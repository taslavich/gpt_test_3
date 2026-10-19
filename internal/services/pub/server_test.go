package pub

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Server, c.Code) {
	t.Helper()
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	r := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { r.Close() })
	if e := r.Ping(context.Background()).Err(); e != nil {
		t.Fatal(e)
	}
	r.FlushDB(context.Background())
	code := c.Code{ID: uuid.NewString(), UserID: uuid.NewString(), ShortCode: "tb_test", Name: "test", Format: "popunder", TrafficType: "mainstream", Device: "all", Active: true, TriggerClick: 1, FrequencyMinutes: .1, Epoch: "test"}
	r.Set(context.Background(), c.ReadyKey, "test", time.Minute)
	b, _ := json.Marshal(code)
	r.HSet(context.Background(), c.CodeKey(code.ShortCode), "data", b)
	r.HSet(context.Background(), c.Prefix+"{control}:blocks", "epoch", "test")
	pipe := r.Pipeline()
	for slot := 0; slot < 16384; slot++ {
		pipe.Eval(context.Background(), c.InitGuardLua, []string{c.GuardKeyForSlot(slot)}, "test")
	}
	if _, e := pipe.Exec(context.Background()); e != nil {
		t.Fatal(e)
	}
	cfg := Config{PublicURL: "https://pub.example.test", InternalSecret: strings.Repeat("x", 32), DeliverySecret: strings.Repeat("y", 32), RTBAdultURL: "https://rtb.example.test/bid_v_2_5", RTBMainstreamURL: "https://rtb.example.test/bid_v_2_5_mc", AdultFeed: "adult", MainstreamFeed: "mainstream", CallbackOrigins: "https://adm.example.test", RedisAddrs: addr, ORTBAddrs: addr, ORTBDB: 14, ORTBReadySet: "ortb:ready", TelemetryMaxBytes: 1 << 20, MaxInflight: 10, RTBTimeout: time.Second, ConfirmedOpenEnabled: true}
	s, e := NewServer(cfg, r, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.CloseORTB() })
	s.ORTB[0].FlushDB(context.Background())
	spool, e := NewTelemetry(t.TempDir(), s.ORTB, cfg.ORTBReadySet, cfg.TelemetryMaxBytes)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { spool.Close() })
	s.Telemetry = spool
	return s, code
}
func input(code c.Code) Request {
	in := Request{ID: uuid.NewString(), Code: code.ShortCode, PageURL: "https://example.test/", SessionID: uuid.NewString(), VisitorID: uuid.NewString(), Click: 1}
	in.SessionToken = sessionToken(strings.Repeat("y", 32), code.ShortCode, in.VisitorID, in.SessionID)
	return in
}
func send(s *Server, in Request) *httptest.ResponseRecorder {
	b, _ := json.Marshal(in)
	r := httptest.NewRequest("POST", "/request", strings.NewReader(string(b)))
	r.Header.Set("User-Agent", "Mozilla/5.0 test browser desktop")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestInvalidURLIsRecordedWithoutRTB(t *testing.T) {
	s, code := fixture(t)
	in := input(code)
	in.PageURL = ""
	w := send(s, in)
	if w.Code != 400 || s.RTBCalls.Load() != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	in.PageURL = "http://127.0.0.1"
	w = send(s, in)
	if w.Code != 400 || s.RTBCalls.Load() != 0 {
		t.Fatal(w.Code)
	}
	in.PageURL = "https://example.test/"
	s.Redis.Del(context.Background(), c.ReadyKey)
	w = send(s, in)
	if w.Code != 503 || s.RTBCalls.Load() != 0 {
		t.Fatal("Redis loss bypassed", w.Code)
	}
}
func TestNoFillReleasesReservation(t *testing.T) {
	s, code := fixture(t)
	s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})
	in := input(code)
	w := send(s, in)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "no_fill") {
		t.Fatal(w.Code, w.Body.String())
	}
	site := c.SiteID("example.test")
	a, b, _ := keys(c.ZoneID(site, code.ID), in.VisitorID, in.SessionID, in.ID)
	if s.Redis.HExists(context.Background(), a, "last").Val() || s.Redis.Exists(context.Background(), b).Val() != 0 {
		t.Fatal("no_fill consumed quota")
	}
	in.ID = uuid.NewString()
	if w = send(s, in); w.Code != 200 {
		t.Fatal(w.Code)
	}
}
func TestConcurrentServersShareReservation(t *testing.T) {
	s, code := fixture(t)
	s2, e := NewServer(s.Config, s.Redis, s.Telemetry)
	if e != nil {
		t.Fatal(e)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(entered)
		<-release
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})
	t.Cleanup(func() { s2.CloseORTB() })
	s2.HTTP = s.HTTP
	in := input(code)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); send(s, in) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not enter RTB")
	}
	other := in
	other.ID = uuid.NewString()
	w := send(s2, other)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("double auction")
	}
}
func TestConfirmedStateExactlyOneCountAndMissingReservationDenied(t *testing.T) {
	s, code := fixture(t)
	in := input(code)
	zone := c.ZoneID(c.SiteID("example.test"), code.ID)
	if v, e := s.reserve(context.Background(), code, zone, in); e != nil || v != "reserved" {
		t.Fatal(v, e)
	}
	d := Delivery{ZoneID: zone, RequestID: in.ID, VisitorID: in.VisitorID, SessionID: in.SessionID, StateTTL: stateTTL(code), Epoch: "test"}
	a, b, _ := keys(zone, in.VisitorID, in.SessionID, in.ID)
	for i := 0; i < 2; i++ {
		if v, e := s.confirmState(context.Background(), d); e != nil || v == 0 {
			t.Fatal(v, e)
		}
	}
	if s.Redis.Get(context.Background(), b).Val() != "1" {
		t.Fatal("duplicate counted")
	}
	in.ID = uuid.NewString()
	s.Redis.Del(context.Background(), a)
	d.RequestID = in.ID
	if v, e := s.confirmState(context.Background(), d); e != nil || v != 0 {
		t.Fatal("lost pending state accepted", v, e)
	}
}
func TestUnsupportedDestinationAndPrivateCallback(t *testing.T) {
	s, _ := fixture(t)
	if _, e := s.destination("https://attacker.test/adm?url=https://example.test"); e == nil {
		t.Fatal("untrusted wrapper")
	}
	if _, e := s.destination("https://adm.example.test/adm?url=http%3A%2F%2F10.0.0.1"); e == nil {
		t.Fatal("private destination")
	}
}

func TestPartialShardLossAndRestartGuardFailsClosed(t *testing.T) {
	s, code := fixture(t)
	in := input(code)
	zone := c.ZoneID(c.SiteID("example.test"), code.ID)
	s.Redis.Del(context.Background(), stateGuard(zone, in.VisitorID))
	if result, e := s.reserve(context.Background(), code, zone, in); e != nil || result != "state_generation_lost" {
		t.Fatal(result, e)
	}
	s.Redis.Set(context.Background(), stateGuard(zone, in.VisitorID), "test:old-server-run-id", 0)
	if result, e := s.reserve(context.Background(), code, zone, in); e != nil || result != "state_generation_lost" {
		t.Fatal(result, e)
	}
}
