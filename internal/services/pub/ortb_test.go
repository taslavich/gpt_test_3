package pub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrewriteAdapterAndSingleCommit(t *testing.T) {
	for _, status := range []int{200, 204} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s, code := fixture(t)
			in := input(code)
			var id string
			s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
				var request ortb.BidRequest
				if e := json.NewDecoder(r.Body).Decode(&request); e != nil {
					t.Fatal(e)
				}
				id = r.Header.Get(pubuuid.Header)
				sig := r.Header.Get(pubuuid.SignatureHeader)
				if e := pubuuid.Validate(s.Config.InternalSecret, id, r.URL.Query().Get("feed"), r.Header.Get(pubuuid.TimeHeader), sig, &request, time.Now()); e != nil {
					t.Fatal(e)
				}
				fields := s.ORTB[0].HGetAll(context.Background(), id).Val()
				if fields["pub_user_id"] != code.UserID || fields["request_id"] != in.ID || fields["request_error"] != "0" || fields["SITE_ID"] == "" || fields["FORMAT"] != "POP" {
					t.Fatal("prewrite missing", fields)
				}
				if s.ORTB[0].SCard(context.Background(), s.Config.ORTBReadySet).Val() != 0 {
					t.Fatal("premature commit")
				}
				if e := pubuuid.Claim(context.Background(), s.ORTB, id, sig); e != nil {
					t.Fatal(e)
				}
				if e := utils.WriteStatsOrtb(context.Background(), s.ORTB, id, true, "POP", "MAINSTREAM", "ssp", "1.1.1.1", "", "en", "US", 0, int32(status), ua.UAFields{}, fields["SITE_ID"], "example.test", 0); e != nil {
					t.Fatal(e)
				}
				// Same existing HSET is allowed to enrich advertiser fields before ready.
				s.ORTB[0].HSet(context.Background(), id, "WIN_USER_ID", "advertiser")
				if e := utils.AddUUIDToRedisSet(context.Background(), s.ORTB, s.Config.ORTBReadySet, id, true); e != nil {
					t.Fatal(e)
				}
				body := `{"seatbid":[{"bid":[{"adm":"https://adm.example.test/adm?url=https%3A%2F%2Fexample.test"}]}]}`
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			w := send(s, in)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if s.ORTB[0].SCard(context.Background(), s.Config.ORTBReadySet).Val() != 1 {
				t.Fatal("not one commit")
			}
			if s.ORTB[0].HGet(context.Background(), id, "WIN_USER_ID").Val() != "advertiser" {
				t.Fatal("advertiser overwritten")
			}
			if s.ORTB[0].HGet(context.Background(), id, "request_error").Val() != "0" {
				t.Fatal("false error")
			}
			// Unknown timeout after commit can never change/recommit the HASH.
			n, e := writeError(context.Background(), s.ORTB, s.Config.ORTBReadySet, id, map[string]string{"request_error": "1"})
			if e != nil || n {
				t.Fatal(n, e)
			}
			s.ORTB[0].SPop(context.Background(), s.Config.ORTBReadySet)
			s.ORTB[0].Del(context.Background(), id)
			if e := pubuuid.Claim(context.Background(), s.ORTB, id, "anything"); e == nil {
				t.Fatal("replay after loader deletion")
			}
		})
	}
}
func TestTransportTimeoutBeforeAndAfterAdmission(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		t.Run(fmt.Sprint(claimed), func(t *testing.T) {
			s, code := fixture(t)
			var id string
			s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
				id = r.Header.Get(pubuuid.Header)
				if claimed {
					if e := pubuuid.Claim(context.Background(), s.ORTB, id, r.Header.Get(pubuuid.SignatureHeader)); e != nil {
						t.Fatal(e)
					}
				}
				return nil, errors.New("connection lost")
			})
			in := input(code)
			if w := send(s, in); w.Code != 502 {
				t.Fatal(w.Code, w.Body.String())
			}
			want := int64(1)
			if claimed {
				want = 0
			}
			if n := s.ORTB[0].SCard(context.Background(), s.Config.ORTBReadySet).Val(); n != want {
				t.Fatal(n, want)
			}
			ttl := s.ORTB[0].TTL(context.Background(), id).Val()
			if ttl <= 0 || ttl > 24*time.Hour {
				t.Fatal(ttl)
			}
			if w := send(s, in); w.Code != 200 || !strings.Contains(w.Body.String(), "rtb_failed") || s.RTBCalls.Load() != 1 {
				t.Fatal("retry restarted auction", w.Code, w.Body.String())
			}
		})
	}
}
func TestClaimErrorRace(t *testing.T) {
	s, _ := fixture(t)
	for i := 0; i < 100; i++ {
		id := uuid.NewString()
		if e := s.prepareORTB(context.Background(), id, "sig", map[string]string{"request_error": "0"}); e != nil {
			t.Fatal(e)
		}
		var wins atomic.Int64
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if pubuuid.Claim(context.Background(), s.ORTB, id, "sig") == nil {
				wins.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			if n, e := writeError(context.Background(), s.ORTB, s.Config.ORTBReadySet, id, map[string]string{"request_error": "1"}); e != nil {
				t.Error(e)
			} else if n {
				wins.Add(1)
			}
		}()
		wg.Wait()
		if wins.Load() != 1 {
			t.Fatal("both/neither owners", wins.Load())
		}
	}
}
func TestErrorRecordAndOutboxDurability(t *testing.T) {
	s, code := fixture(t)
	in := input(code)
	in.PageURL = ""
	if w := send(s, in); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := send(s, in); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if s.ORTB[0].SCard(context.Background(), s.Config.ORTBReadySet).Val() != 1 {
		t.Fatal("duplicate error")
	}
	id := s.ORTB[0].SMembers(context.Background(), s.Config.ORTBReadySet).Val()[0]
	fields := s.ORTB[0].HGetAll(context.Background(), id).Val()
	if fields["request_error"] != "1" || fields["error_reason"] != "invalid_page_url" || fields["CODE"] != "400" || fields["EVENT_TIME"] == "" || fields["WIN_USER_ID"] != "" {
		t.Fatal(fields)
	}
	// Durable batched outbox survives close/reopen; retries same Redis script.
	dir := t.TempDir()
	out, e := NewTelemetry(dir, s.ORTB, s.Config.ORTBReadySet, 1024)
	if e != nil {
		t.Fatal(e)
	}
	extra := uuid.NewString()
	if e = out.Record(extra, fields); e != nil {
		t.Fatal(e)
	}
	out.Close()
	out, e = NewTelemetry(dir, s.ORTB, s.Config.ORTBReadySet, 1024)
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	if e = out.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = out.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
	if n := s.ORTB[0].SCard(context.Background(), s.Config.ORTBReadySet).Val(); n != 2 {
		t.Fatal(n)
	}
	if e = out.Record(uuid.NewString(), map[string]string{"large": strings.Repeat("x", 2048)}); e == nil {
		t.Fatal("quota not enforced")
	}
}
func TestConfirmDoesNotCallFinancialCallbacks(t *testing.T) {
	s, code := fixture(t)
	in := input(code)
	zone := strings.Repeat("a", 64)
	if _, e := s.reserve(context.Background(), code, zone, in); e != nil {
		t.Fatal(e)
	}
	d := Delivery{Token: uuid.NewString(), RequestID: in.ID, ZoneID: zone, VisitorID: in.VisitorID, SessionID: in.SessionID, Epoch: code.Epoch, StateTTL: stateTTL(code), Expires: time.Now().Add(time.Minute).Unix()}
	b, _ := json.Marshal(d)
	s.Redis.Set(context.Background(), "twinbid:publisher:v1:delivery:"+d.Token, b, time.Minute)
	s.HTTP.Transport = transport(func(*http.Request) (*http.Response, error) {
		t.Error("confirm made HTTP call")
		return nil, errors.New("unexpected")
	})
	token := d.Token + "." + c.Sign(s.Config.DeliverySecret, d.Token)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/confirm/"+token, nil)
		s.Handler().ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	_, key, _ := keys(zone, in.VisitorID, in.SessionID, in.ID)
	if s.Redis.Get(context.Background(), key).Val() != "1" {
		t.Fatal("confirm counted twice")
	}
}

func TestZoneLimitsAndReservationExpiry(t *testing.T) {
	s, code := fixture(t)
	cap := 1
	code.MaxPerSession = &cap
	in := input(code)
	zone := "zone-one"
	d := Delivery{RequestID: in.ID, ZoneID: zone, VisitorID: in.VisitorID, SessionID: in.SessionID, Epoch: code.Epoch, StateTTL: stateTTL(code)}
	if v, e := s.reserve(context.Background(), code, zone, in); e != nil || v != "reserved" {
		t.Fatal(v, e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n, e := s.confirmState(context.Background(), d); e != nil || n == 0 {
				t.Error(n, e)
			}
		}()
	}
	wg.Wait()
	a, b, _ := keys(zone, in.VisitorID, in.SessionID, in.ID)
	if s.Redis.Get(context.Background(), b).Val() != "1" {
		t.Fatal("concurrent duplicate")
	}
	in.ID = uuid.NewString()
	if v, e := s.reserve(context.Background(), code, zone, in); e != nil || v != "frequency" {
		t.Fatal(v, e)
	}
	s.Redis.HSet(context.Background(), a, "last", 0)
	if v, e := s.reserve(context.Background(), code, zone, in); e != nil || v != "session_cap" {
		t.Fatal(v, e)
	}
	in.SessionID = uuid.NewString()
	if v, e := s.reserve(context.Background(), code, zone, in); e != nil || v != "reserved" {
		t.Fatal(v, e)
	}
	s.Redis.HSet(context.Background(), a, "until", 1)
	d.RequestID = in.ID
	d.SessionID = in.SessionID
	if n, e := s.confirmState(context.Background(), d); e != nil || n != 0 {
		t.Fatal("expired reservation counted", n, e)
	}
	in.ID = uuid.NewString()
	if v, e := s.reserve(context.Background(), code, "zone-two", in); e != nil || v != "reserved" {
		t.Fatal("zone limit leaked", v, e)
	}
}

func TestORTBRedisLossUsesDurableErrorOutbox(t *testing.T) {
	s, code := fixture(t)
	original := s.ORTB
	failed := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 10 * time.Millisecond, MaxRetries: 0})
	defer failed.Close()
	s.ORTB = []*redis.Client{failed}
	in := input(code)
	in.PageURL = ""
	w := send(s, in)
	if w.Code != 400 {
		t.Fatal("durable error was refused", w.Code, w.Body.String())
	}
	s.ORTB = original
	if n := original[0].SCard(context.Background(), s.Config.ORTBReadySet).Val(); n != 0 {
		t.Fatal(n)
	}
	if e := s.Telemetry.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
	if n := original[0].SCard(context.Background(), s.Config.ORTBReadySet).Val(); n != 1 {
		t.Fatal("outbox did not restore same error", n)
	}
}
func TestPrewriteCollisionDoesNotReplaceAnotherHASH(t *testing.T) {
	s, _ := fixture(t)
	id := uuid.NewString()
	s.ORTB[0].HSet(context.Background(), id, "WIN_USER_ID", "old-advertiser")
	if e := s.prepareORTB(context.Background(), id, "signature", map[string]string{"pub_user_id": "attacker"}); e == nil {
		t.Fatal("collision replaced existing HASH")
	}
	if s.ORTB[0].HGet(context.Background(), id, "WIN_USER_ID").Val() != "old-advertiser" || s.ORTB[0].HExists(context.Background(), id, "pub_user_id").Val() {
		t.Fatal("foreign HASH changed")
	}
}

func TestDeliveryKeepsOriginalADMForURLAndXML(t *testing.T) {
	s, _ := fixture(t)
	for _, target := range []string{"https://example.test/landing", `<ad><popunderAd><url><![CDATA[https://example.test/landing]]></url></popunderAd></ad>`} {
		original := "https://adm.example.test/adm?id=original-auction&f=4&url=" + url.QueryEscape(target)
		got, e := s.deliveryADM(original)
		if e != nil {
			t.Fatal(e)
		}
		parsed, e := url.Parse(got)
		if e != nil {
			t.Fatal(e)
		}
		if parsed.Host != "adm.example.test" || parsed.Path != "/adm" || parsed.Query().Get("id") != "original-auction" || parsed.Query().Get("f") != "4" || parsed.Query().Get("url") != "https://example.test/landing" {
			t.Fatal("tracking wrapper bypassed/broken", got)
		}
	}
}

func TestOnlyExplicitPublisherFeedTypes(t *testing.T) {
	s, _ := fixture(t)
	for _, traffic := range []string{"adult", "mainstream"} {
		endpoint, feed, e := s.feed(traffic)
		if e != nil {
			t.Fatal(e)
		}
		if traffic == "adult" && (endpoint != s.Config.RTBAdultURL || feed != s.Config.AdultFeed) {
			t.Fatal("adult feed")
		}
		if traffic == "mainstream" && (endpoint != s.Config.RTBMainstreamURL || feed != s.Config.MainstreamFeed) {
			t.Fatal("mainstream feed")
		}
	}
	for _, traffic := range []string{"", "mixed", "other"} {
		if _, _, e := s.feed(traffic); e == nil {
			t.Fatal("unsupported type routed", traffic)
		}
	}
}
