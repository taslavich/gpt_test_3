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
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
)

func TestDirectLinkAuctionAndDelivery(t *testing.T) {
	s, code := fixture(t)
	ctx := context.Background()
	save := func() {
		t.Helper()
		b, err := json.Marshal(code)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Redis.HSet(ctx, c.CodeKey(code.ShortCode), "data", b).Err(); err != nil {
			t.Fatal(err)
		}
	}
	const adm = "https://adm.example.test/adm?id=10000000-0000-4000-8000-000000000020"
	const source = "https://example.test/watch"
	target := "/go/" + code.ShortCode
	h := s.Handler()
	open := func(method, referer, purpose string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := trustedRequest(method, target, nil)
		r.Header.Set("User-Agent", "Mozilla/5.0 test browser desktop")
		r.Header.Set("Referer", referer)
		r.Header.Set("Sec-Purpose", purpose)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	calls := 0
	fill := true
	s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		calls++
		var request ortb.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		feed := r.URL.Query().Get("feed")
		if r.Method != http.MethodPost || r.URL.Path != "/bid_v_2_5_mc" || feed != "mainstream" ||
			request.GetSite().GetId() != c.SiteID("example.test") ||
			request.GetSite().GetPage() != source || request.GetDevice().GetIp() != "203.0.113.42" ||
			len(request.GetImp()) != 1 || request.GetImp()[0].GetBanner() == nil {
			t.Fatalf("unexpected Publisher request: %s %s %v", r.Method, r.URL, &request)
		}
		id, sig := r.Header.Get(pubuuid.Header), r.Header.Get(pubuuid.SignatureHeader)
		if err := pubuuid.Validate(s.Config.InternalSecret, id, feed, r.Header.Get(pubuuid.TimeHeader), sig, &request, time.Now()); err != nil {
			t.Fatal(err)
		}
		if got := s.ORTB[0].HGet(ctx, id, "ad_code_id").Val(); got != code.ID {
			t.Fatalf("wrong attributed code %q", got)
		}
		if !fill {
			return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"seatbid":[{"bid":[{"adm":"` + adm + `"}]}]}`))}, nil
	})
	if w := open("GET", source, "", nil); w.Code != 404 {
		t.Fatalf("Popunder must not work via /go: %d", w.Code)
	}
	code.Format, code.FrequencyMinutes = "direct_link", 10
	save()
	if w := send(s, input(code)); w.Code != 404 {
		t.Fatalf("Direct Link must not work via /request: %d", w.Code)
	}
	if w := open("HEAD", source, "", nil); w.Code != 405 {
		t.Fatalf("HEAD must not auction: %d", w.Code)
	}
	if w := open("GET", "", "", nil); w.Code != 204 {
		t.Fatalf("missing source: %d", w.Code)
	}
	if w := open("GET", source, "prefetch", nil); w.Code != 204 {
		t.Fatalf("prefetch must not auction: %d", w.Code)
	}
	if err := s.Redis.HSet(ctx, c.SiteBlocksKey, c.SiteID("example.test"), "1").Err(); err != nil {
		t.Fatal(err)
	}
	if w := open("GET", source, "", nil); w.Code != 403 {
		t.Fatalf("site block ignored: %d", w.Code)
	}
	s.Redis.HDel(ctx, c.SiteBlocksKey, c.SiteID("example.test"))
	code.Active = false
	save()
	if w := open("GET", source, "", nil); w.Code != 403 || calls != 0 {
		t.Fatalf("inactive/invalid traffic reached auction: %d, calls=%d", w.Code, calls)
	}
	code.Active = true
	save()

	fill = false
	w := open("GET", source, "", nil)
	if w.Code != 204 || w.Header().Get("Location") != "" || calls != 1 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("no fill: %d, calls=%d, headers=%v", w.Code, calls, w.Header())
	}
	cookie := w.Result().Cookies()[0]
	fill = true
	w = open("GET", source, "", cookie)
	location, err := url.Parse(w.Header().Get("Location"))
	if w.Code != 302 || err != nil || location.Host != "pub.example.test" || !strings.HasPrefix(location.Path, "/delivery/") || calls != 2 {
		t.Fatalf("fill: %d, calls=%d, location=%v, body=%s", w.Code, calls, location, w.Body)
	}
	delivery := httptest.NewRecorder()
	h.ServeHTTP(delivery, trustedRequest("GET", location.String(), nil))
	if delivery.Code != 200 || !strings.Contains(delivery.Body.String(), adm) || !strings.Contains(delivery.Body.String(), "/confirm/") {
		t.Fatalf("unexpected delivery: %d %s", delivery.Code, delivery.Body)
	}
	confirm := httptest.NewRecorder()
	h.ServeHTTP(confirm, trustedRequest("POST", strings.Replace(location.Path, "/delivery/", "/confirm/", 1), nil))
	if confirm.Code != 204 {
		t.Fatalf("confirm rejected: %d %s", confirm.Code, confirm.Body)
	}
	if w := open("GET", source, "", cookie); w.Code != 409 || calls != 2 {
		t.Fatalf("frequency not enforced after confirmation: %d, calls=%d", w.Code, calls)
	}
}
