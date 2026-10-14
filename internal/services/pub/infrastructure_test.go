package pub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testInfraConfig() Config {
	return Config{PublicURL: "https://legacy.test", AntiBlockEnabled: true, AntiBlockDomains: "https://work-a.test,https://work-b.test", AntiBlockStableOrigin: "https://stable.test", AntiBlockAutoRotate: true}
}
func TestInfrastructurePinsAndRoutes(t *testing.T) {
	c := testInfraConfig()
	c.AntiBlockRuntimeOrigin = "https://work-b.test"
	c.AntiBlockAPIOrigin = "https://work-b.test"
	c.AntiBlockDeliveryOrigin = "https://work-b.test"
	s, e := infrastructureSettings(c)
	if e != nil || s.RuntimePreferred != s.APIPreferred || s.RuntimePreferred != s.DeliveryPreferred {
		t.Fatal(s, e)
	}
	manager := newInfrastructure(context.Background(), s, nil)
	manager.settings.Rotate = false
	if m, e := manager.manifest(); e != nil || m.RuntimeOrigins[0] != "https://work-b.test" || m.APIOrigins[0] != "https://work-b.test" || m.DeliveryOrigins[0] != "https://work-b.test" {
		t.Fatal("manual preferences", m, e)
	}
	if s.CheckInterval != 24*time.Hour || s.PollInterval != 15*time.Second || s.Timeout != 10*time.Second || s.MaxBytes != 16<<20 {
		t.Fatal(s)
	}
	for _, raw := range []string{"http://work.test", "https://work.test:444", "https://user@work.test", "https://work.test/a", "https://work.test?x=1", "https://127.0.0.1", "https://localhost", "https://work..test", "https://work.test,https://work.test"} {
		c := testInfraConfig()
		c.AntiBlockDomains = raw
		if _, e := infrastructureSettings(c); e == nil {
			t.Fatal("accepted", raw)
		}
	}
	for _, raw := range []string{`{"request":"/metrics"}`, `{"request":"/config"}`, `{"delivery":"/x/","request":"/x/inside"}`, `{"request":"/x?redirect=1"}`, `{"request":"//other.test"}`, `{"unknown":"/x"}`, `{} junk`} {
		c := testInfraConfig()
		c.AntiBlockRoutes = raw
		if _, e := infrastructureSettings(c); e == nil {
			t.Fatal("accepted routes", raw)
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254", "::1", "::ffff:127.0.0.1", "fc00::1", "2001:db8::1", "64:ff9b::7f00:1"} {
		if publicIP(netip.MustParseAddr(raw)) {
			t.Fatal("SSRF IP accepted", raw)
		}
	}
	if !publicIP(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public IP refused")
	}
}
func TestInfrastructureSharedSnapshotRestartAndFailures(t *testing.T) {
	server, _ := fixture(t)
	cfg, e := infrastructureSettings(testInfraConfig())
	if e != nil {
		t.Fatal(e)
	}
	a := newInfrastructure(context.Background(), cfg, server.Redis)
	calls := atomic.Int64{}
	failed := false
	a.client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if failed {
			return nil, errors.New("list network failure")
		}
		body := "[Adblock Plus 2.0]\n! fixture valid list\n||blocked.test^\n"
		if r.URL.Host == "pgl.yoyo.org" {
			body = "blocked.test\nanother-blocked.test\n"
		}
		if r.URL.Path == cfg.Paths.Health {
			body = `{"fingerprint":"` + cfg.Fingerprint + `"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	a.match = func(_ context.Context, _ []string, requests []filterRequest) ([]filterFinding, error) {
		var out []filterFinding
		for _, r := range requests {
			out = append(out, filterFinding{Origin: r.Origin, Component: r.Component, Blocked: r.Origin == "https://work-a.test" && r.Component == "runtime"})
		}
		return out, nil
	}
	if e = a.refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	m, e := a.manifest()
	if e != nil || !reflect.DeepEqual(m.APIOrigins, []string{"https://work-b.test"}) {
		t.Fatal(m, e)
	}
	count := calls.Load()
	if e = a.refresh(context.Background()); e != nil || calls.Load() != count {
		t.Fatal("daily refresh repeated", e)
	}
	restored := newInfrastructure(context.Background(), cfg, server.Redis)
	m, e = restored.manifest()
	if e != nil || len(m.APIOrigins) != 1 || m.APIOrigins[0] != "https://work-b.test" {
		t.Fatal("restart lost state", m, e)
	}
	// A failed update never publishes a partial/empty configuration.
	a.mu.Lock()
	a.snapshot.CheckedAt = 0
	before := a.snapshot
	a.mu.Unlock()
	b, _ := json.Marshal(before)
	server.Redis.Set(context.Background(), infraSnapshotKey, b, 0)
	server.Redis.Del(context.Background(), infraAttemptKey)
	failed = true
	if e = a.refresh(context.Background()); e == nil {
		t.Fatal("failed source accepted")
	}
	a.mu.RLock()
	after := a.snapshot
	a.mu.RUnlock()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failure replaced last state")
	}
	count = calls.Load()
	if e = a.refresh(context.Background()); e != nil || calls.Load() != count {
		t.Fatal("failed update retried more than configured daily", e)
	}
	// A stale lease cannot publish and a different fleet configuration cannot load it.
	server.Redis.Set(context.Background(), infraLeaseKey, "new-owner", time.Minute)
	if n, e := server.Redis.Eval(context.Background(), infraPublishLua, []string{infraLeaseKey, infraSnapshotKey}, "old-owner", `{}`).Int(); e != nil || n != 0 {
		t.Fatal("stale publisher accepted", n, e)
	}
	cfg.Fingerprint = "other"
	bad := newInfrastructure(context.Background(), cfg, server.Redis)
	if _, e = bad.manifest(); e == nil {
		t.Fatal("fleet mismatch accepted")
	}
	restored.mu.Lock()
	for i := range restored.snapshot.Domains {
		restored.snapshot.Domains[i].State = "blocked"
	}
	restored.mu.Unlock()
	if _, e = restored.manifest(); e == nil {
		t.Fatal("all blocked pool accepted")
	}
	restored.settings.Rotate = false
	if m, e = restored.manifest(); e != nil || m.APIOrigins[0] != "https://work-a.test" || len(m.APIOrigins) != 1 {
		t.Fatal("manual mode rotated", m, e)
	}
}
func TestInfrastructureDownloadBoundsAndTwoProbes(t *testing.T) {
	cfg, _ := infrastructureSettings(testInfraConfig())
	a := newInfrastructure(context.Background(), cfg, nil)
	for _, tc := range []struct {
		status int
		body   string
	}{{302, "redirect"}, {200, "<html>not a filter list</html>padding"}, {200, strings.Repeat("x", int(cfg.MaxBytes)+1)}} {
		a.client = &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}
		if _, e := a.download(context.Background(), infraSources[0].URL); e == nil {
			t.Fatal("invalid response accepted", tc.status)
		}
	}
	n := 0
	a.client = &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) { n++; return nil, errors.New("unreachable") })}
	if a.probe(context.Background(), cfg.Origins[0]) || n != 2 {
		t.Fatal(n)
	}
	if _, e := normalizeHosts("bad.test\nhttps://evil.test/path"); e == nil {
		t.Fatal("invalid host list accepted")
	}
}
func TestInfrastructureRealFilterProcess(t *testing.T) {
	script := os.Getenv("PUB_TEST_FILTER_SCRIPT")
	if script == "" {
		t.Skip("PUB_TEST_FILTER_SCRIPT unset")
	}
	cfg, _ := infrastructureSettings(testInfraConfig())
	cfg.FilterScript = script
	a := newInfrastructure(context.Background(), cfg, nil)
	requests := a.filterRequests()
	found, e := a.matchFilters(context.Background(), []string{"[Adblock Plus 2.0]\n||work-a.test/assets/*$script,third-party\n||work-b.test/request$xmlhttprequest,method=POST"}, requests)
	if e != nil {
		t.Fatal(e)
	}
	n := 0
	for _, f := range found {
		if f.Blocked {
			n++
		}
	}
	if n != 2 {
		t.Fatal("actual paths/types not checked", n, found)
	}
}
func TestInfrastructureAliasesCORSAndLegacySDK(t *testing.T) {
	s, _ := fixture(t)
	c := testInfraConfig()
	c.AntiBlockRoutes = `{"config":"/x/settings","request":"/x/query","delivery":"/x/open/","confirm":"/x/done/"}`
	settings, e := infrastructureSettings(c)
	if e != nil {
		t.Fatal(e)
	}
	s.Config.AntiBlockEnabled = true
	s.InfraSettings = settings
	s.Infrastructure = newInfrastructure(context.Background(), settings, s.Redis)
	h := s.Handler()
	for _, path := range []string{"/static/adManager.js", "/sdk.js", "/assets/runtime.js", "/infra.json", "/infra/health"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
		if path == "/sdk.js" && strings.Contains(w.Body.String(), "__TWINBID_BOOTSTRAP_SETTINGS__") {
			t.Fatal("unreplaced SDK pins")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("OPTIONS", "/x/query", nil))
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-TwinBid-Infrastructure") || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal(w.Code, w.Header())
	}
}

func TestInfrastructureLiveOfficialLists(t *testing.T) {
	if os.Getenv("PUB_TEST_LIVE_LISTS") != "true" {
		t.Skip("PUB_TEST_LIVE_LISTS unset")
	}
	cfg, _ := infrastructureSettings(testInfraConfig())
	cfg.Timeout = 3 * time.Second
	cfg.FilterScript = os.Getenv("PUB_TEST_FILTER_SCRIPT")
	if cfg.FilterScript == "" {
		t.Skip("filter helper path unset")
	}
	a := newInfrastructure(context.Background(), cfg, nil)
	var lists []string
	for _, source := range infraSources {
		text, e := a.download(context.Background(), source.URL)
		if e != nil {
			t.Skipf("official source not reachable here: %s: %v", source.Name, e)
		}
		if source.Hosts {
			text, e = normalizeHosts(text)
			if e != nil {
				t.Fatal(e)
			}
		}
		lists = append(lists, text)
		t.Log(source.Name, len(text), "bytes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, e := a.matchFilters(ctx, lists, a.filterRequests()); e != nil {
		t.Fatal(e)
	}
}
