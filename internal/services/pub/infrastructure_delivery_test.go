package pub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
)

func TestCrossDomainRetryOneAuctionOriginalADMAndReservation(t *testing.T) {
	s, code := fixture(t)
	cfg := s.Config
	cfg.AntiBlockEnabled = true
	cfg.AntiBlockAutoRotate = true
	cfg.AntiBlockStableOrigin = "https://stable.test"
	cfg.AntiBlockDomains = "https://work-a.test,https://work-b.test"
	cfg.AntiBlockDeliveryOrigin = "https://work-b.test"
	cfg.AntiBlockRoutes = `{"request":"/x/query","delivery":"/x/open/","confirm":"/x/done/"}`
	settings, e := infrastructureSettings(cfg)
	if e != nil {
		t.Fatal(e)
	}
	s.Config = cfg
	s.InfraSettings = settings
	s.Infrastructure = newInfrastructure(context.Background(), settings, s.Redis)
	second, e := NewServer(cfg, s.Redis, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { second.CloseORTB() })
	in := input(code)
	adm := "https://adm.example.test/adm?id=original-id&format=POP&url=https%3A%2F%2Fexample.test"
	s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		var request ortb.BidRequest
		if e := json.NewDecoder(r.Body).Decode(&request); e != nil {
			t.Fatal(e)
		}
		id := r.Header.Get(pubuuid.Header)
		sig := r.Header.Get(pubuuid.SignatureHeader)
		if e := pubuuid.Validate(cfg.InternalSecret, id, r.URL.Query().Get("feed"), r.Header.Get(pubuuid.TimeHeader), sig, &request, time.Now()); e != nil {
			t.Fatal(e)
		}
		if e := pubuuid.Claim(context.Background(), s.ORTB, id, sig); e != nil {
			t.Fatal(e)
		}
		fields := s.ORTB[0].HGetAll(context.Background(), id).Val()
		if e := utils.WriteStatsOrtb(context.Background(), s.ORTB, id, true, "POP", "MAINSTREAM", "ssp", "1.1.1.1", "", "en", "US", 0, 200, ua.UAFields{}, fields["SITE_ID"], "example.test", 0); e != nil {
			t.Fatal(e)
		}
		if e := utils.AddUUIDToRedisSet(context.Background(), s.ORTB, cfg.ORTBReadySet, id, true); e != nil {
			t.Fatal(e)
		}
		body, _ := json.Marshal(map[string]any{"seatbid": []any{map[string]any{"bid": []any{map[string]any{"adm": adm}}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	sendDomain := func(server *Server, origin string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(in)
		r := httptest.NewRequest("POST", origin+settings.Paths.Request, strings.NewReader(string(b)))
		r.Header.Set("User-Agent", "Mozilla/5.0 test browser desktop")
		r.Header.Set("X-TwinBid-Infrastructure", settings.Fingerprint)
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w
	}
	first := sendDomain(s, "https://work-a.test")
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	// The browser loses this response AFTER processing. Same ID on another domain/instance reads the cached result.
	repeated := sendDomain(second, "https://work-b.test")
	if repeated.Code != 200 || strings.TrimSpace(repeated.Body.String()) != strings.TrimSpace(first.Body.String()) || s.RTBCalls.Load() != 1 || second.RTBCalls.Load() != 0 || s.ORTB[0].SCard(context.Background(), cfg.ORTBReadySet).Val() != 1 {
		t.Fatal("duplicate auction/ORTB commit", repeated.Code, repeated.Body.String())
	}
	var result Result
	if e := json.Unmarshal(first.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	open, e := url.Parse(result.OpenURL)
	if e != nil || open.Scheme+"://"+open.Host != "https://work-b.test" || !strings.HasPrefix(open.Path, "/x/open/") {
		t.Fatal(result, e)
	}
	zone := c.ZoneID(c.SiteID("example.test"), code.ID)
	_, sessionKey, _ := keys(zone, in.VisitorID, in.SessionID, in.ID)
	for _, method := range []string{"HEAD", "GET"} {
		w := httptest.NewRecorder()
		second.Handler().ServeHTTP(w, httptest.NewRequest(method, result.OpenURL, nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if method == "GET" && (!strings.Contains(w.Body.String(), "/x/done/") || !strings.Contains(w.Body.String(), "original-id")) {
			t.Fatal("ADM/confirm changed", w.Body.String())
		}
	}
	if second.Redis.Exists(context.Background(), sessionKey).Val() != 0 {
		t.Fatal("HEAD/GET consumed quota")
	}
	token := strings.TrimPrefix(open.Path, settings.Paths.Delivery)
	// Client HEAD checks are advisory; the bridge must still reject a changed HMAC
	// on either HEAD or navigation without confirmation, quota or another auction.
	parts := strings.Split(token, ".")
	badSignature := strings.Repeat("0", 64)
	if parts[1] == badSignature {
		badSignature = strings.Repeat("1", 64)
	}
	for _, invalid := range []string{"invalid-token", parts[0] + "." + badSignature} {
		for _, method := range []string{"HEAD", "GET"} {
			w := httptest.NewRecorder()
			second.Handler().ServeHTTP(w, httptest.NewRequest(method, "https://work-b.test"+settings.Paths.Delivery+invalid, nil))
			if w.Code != 410 || second.Redis.Exists(context.Background(), sessionKey).Val() != 0 || s.RTBCalls.Load() != 1 || second.RTBCalls.Load() != 0 {
				t.Fatal("invalid delivery token accepted or consumed quota/auction", method, w.Code)
			}
		}
	}
	for n := 0; n < 2; n++ {
		w := httptest.NewRecorder()
		second.Handler().ServeHTTP(w, httptest.NewRequest("POST", "https://work-a.test"+settings.Paths.Confirm+token, nil))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if second.Redis.Get(context.Background(), sessionKey).Val() != "1" || s.ORTB[0].SCard(context.Background(), cfg.ORTBReadySet).Val() != 1 {
		t.Fatal("confirm duplicated quota/stats")
	}
	// Legacy request still returns the old same-origin delivery URL even with anti-block enabled.
	legacy, _ := second.requestDeliveryPrefix(context.Background(), httptest.NewRequest("POST", "/request", nil))
	if legacy != cfg.PublicURL+"/delivery/" {
		t.Fatal(legacy)
	}
}

func TestInfrastructureEnabledKeepsFrequencyCapAndNoFill(t *testing.T) {
	s, code := fixture(t)
	cfg := testInfraConfig()
	cfg.PublicURL = s.Config.PublicURL
	settings, e := infrastructureSettings(cfg)
	if e != nil {
		t.Fatal(e)
	}
	s.Config.AntiBlockEnabled = true
	s.InfraSettings = settings
	s.Infrastructure = newInfrastructure(context.Background(), settings, s.Redis)
	s.HTTP.Transport = transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})
	in := input(code)
	b, _ := json.Marshal(in)
	r := httptest.NewRequest("POST", "https://work-a.test/request", strings.NewReader(string(b)))
	r.Header.Set("User-Agent", "Mozilla/5.0 test browser desktop")
	r.Header.Set("X-TwinBid-Infrastructure", settings.Fingerprint)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "no_fill") {
		t.Fatal(w.Code, w.Body.String())
	}
	zone := c.ZoneID(c.SiteID("example.test"), code.ID)
	a, sessionKey, _ := keys(zone, in.VisitorID, in.SessionID, in.ID)
	if s.Redis.HExists(context.Background(), a, "last").Val() || s.Redis.Exists(context.Background(), sessionKey).Val() != 0 {
		t.Fatal("no-fill consumed quota")
	}
	cap := 1
	code.MaxPerSession = &cap
	in.ID = "5983019e-ce2c-43ad-83c9-0fcb76792f97"
	if v, e := s.reserve(context.Background(), code, zone, in); e != nil || v != "reserved" {
		t.Fatal(v, e)
	}
	d := Delivery{RequestID: in.ID, ZoneID: zone, VisitorID: in.VisitorID, SessionID: in.SessionID, Epoch: code.Epoch, StateTTL: stateTTL(code)}
	if n, e := s.confirmState(context.Background(), d); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	in.ID = "5983019e-ce2c-43ad-83c9-0fcb76792f98"
	if v, e := s.reserve(context.Background(), code, zone, in); e != nil || v != "frequency" {
		t.Fatal(v, e)
	}
	s.Redis.HSet(context.Background(), a, "last", 0)
	if v, e := s.reserve(context.Background(), code, zone, in); e != nil || v != "session_cap" {
		t.Fatal(v, e)
	}
}
