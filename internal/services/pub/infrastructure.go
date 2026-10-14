package pub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Control metadata only: never ORTB/quotas/ledger, with a separate cluster hash tag.
const infraSnapshotKey = "twinbid:pubinfra:v1:{domains}:snapshot"
const infraAttemptKey = "twinbid:pubinfra:v1:{domains}:attempt"
const infraLeaseKey = "twinbid:pubinfra:v1:{domains}:lease"
const infraPublishLua = `if redis.call('GET',KEYS[1])~=ARGV[1] then return 0 end;redis.call('SET',KEYS[2],ARGV[2]);return 1`
const infraReleaseLua = `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end;return 0`

type infraDomain struct {
	Origin  string   `json:"origin"`
	State   string   `json:"state"`
	Reasons []string `json:"reasons,omitempty"`
}
type infraSnapshot struct {
	Fingerprint string        `json:"fingerprint"`
	CheckedAt   int64         `json:"checked_at"`
	Domains     []infraDomain `json:"domains"`
	StableState string        `json:"stable_state"`
}
type infraManifest struct {
	infraSettings
	RuntimeOrigins  []string `json:"runtime_origins"`
	APIOrigins      []string `json:"api_origins"`
	DeliveryOrigins []string `json:"delivery_origins"`
	CheckedAt       int64    `json:"checked_at"`
	Status          string   `json:"status"`
}
type infrastructure struct {
	settings    infraSettings
	redis       redis.UniversalClient
	client      *http.Client
	mu          sync.RWMutex
	snapshot    infraSnapshot
	fault       string
	initialized bool
	failures    atomic.Uint64
	rotations   atomic.Uint64
	events      sync.Map
	// Test seam; production always uses the pinned, offline filter process.
	match func(context.Context, []string, []filterRequest) ([]filterFinding, error)
}

func newInfrastructure(ctx context.Context, cfg infraSettings, r redis.UniversalClient) *infrastructure {
	a := &infrastructure{settings: cfg, redis: r, client: publicHTTPClient(cfg.Timeout)}
	for _, o := range cfg.Origins {
		a.snapshot.Domains = append(a.snapshot.Domains, infraDomain{Origin: o, State: "unverified"})
	}
	a.snapshot.Fingerprint = cfg.Fingerprint
	a.snapshot.StableState = "unverified"
	a.match = a.matchFilters
	if r != nil {
		b, _ := json.Marshal(a.snapshot)
		// Seed once; later a missing key is not silently replaced over a live fleet.
		if e := r.SetNX(ctx, infraSnapshotKey, b, 0).Err(); e != nil {
			a.fault = "control_unavailable"
		} else {
			a.poll(ctx)
		}
	}
	return a
}
func (a *infrastructure) validSnapshot(s infraSnapshot) bool {
	if s.Fingerprint != a.settings.Fingerprint || len(s.Domains) != len(a.settings.Origins) || s.CheckedAt > time.Now().Add(time.Minute).Unix() {
		return false
	}
	allowed := map[string]bool{"unverified": true, "available": true, "blocked": true, "unavailable": true}
	for i, d := range s.Domains {
		if d.Origin != a.settings.Origins[i] || !allowed[d.State] {
			return false
		}
	}
	return allowed[s.StableState]
}
func (a *infrastructure) poll(ctx context.Context) error {
	b, e := a.redis.Get(ctx, infraSnapshotKey).Bytes()
	if e == redis.Nil {
		a.mu.RLock()
		initial := !a.initialized
		seed := a.snapshot
		a.mu.RUnlock()
		if initial {
			encoded, _ := json.Marshal(seed)
			if err := a.redis.SetNX(ctx, infraSnapshotKey, encoded, 0).Err(); err != nil {
				return err
			}
			b, e = a.redis.Get(ctx, infraSnapshotKey).Bytes()
		}
	}
	if e != nil {
		a.mu.Lock()
		if e == redis.Nil {
			a.fault = "control_state_lost"
		}
		a.mu.Unlock()
		return e // Redis transport errors retain the last validated snapshot.
	}
	var s infraSnapshot
	if len(b) > 256<<10 || json.Unmarshal(b, &s) != nil || !a.validSnapshot(s) {
		a.mu.Lock()
		a.fault = "config_mismatch"
		a.mu.Unlock()
		return errors.New("infrastructure snapshot/config mismatch")
	}
	a.mu.Lock()
	a.snapshot = s
	a.initialized = true
	a.fault = ""
	a.mu.Unlock()
	return nil
}
func ordered(origins []string, preferred string) []string {
	out := append([]string{}, origins...)
	for i, o := range out {
		if o == preferred {
			copy(out[1:i+1], out[:i])
			out[0] = o
			break
		}
	}
	return out
}
func (a *infrastructure) manifest() (infraManifest, error) {
	a.mu.RLock()
	s := a.snapshot
	fault := a.fault
	a.mu.RUnlock()
	m := infraManifest{infraSettings: a.settings, CheckedAt: s.CheckedAt, Status: "ready"}
	if fault != "" {
		return m, errors.New(fault)
	}
	var usable []string
	for _, d := range s.Domains {
		if d.State == "available" || d.State == "unverified" {
			usable = append(usable, d.Origin)
		}
	}
	if !a.settings.Rotate {
		first := a.settings.Origins[0]
		preferred := func(v string) []string {
			if v == "" {
				v = first
			}
			return []string{v}
		}
		m.RuntimeOrigins = preferred(a.settings.RuntimePreferred)
		m.APIOrigins = preferred(a.settings.APIPreferred)
		m.DeliveryOrigins = preferred(a.settings.DeliveryPreferred)
		m.Status = "manual"
		return m, nil
	} else if len(usable) == 0 {
		return m, errors.New("no_usable_domains")
	}
	if s.CheckedAt == 0 {
		m.Status = "unverified"
	} else if time.Now().Unix()-s.CheckedAt > int64(a.settings.CheckInterval.Seconds())*2 {
		m.Status = "stale"
	}
	m.RuntimeOrigins = ordered(usable, a.settings.RuntimePreferred)
	m.APIOrigins = ordered(usable, a.settings.APIPreferred)
	m.DeliveryOrigins = ordered(usable, a.settings.DeliveryPreferred)
	return m, nil
}
func (s *Server) RunInfrastructure(ctx context.Context) {
	if s.Infrastructure == nil {
		return
	}
	a := s.Infrastructure
	tick := time.NewTicker(a.settings.PollInterval)
	defer tick.Stop()
	for {
		check, cancel := context.WithTimeout(ctx, 2*time.Second)
		e := a.poll(check)
		cancel()
		if e == nil {
			a.mu.RLock()
			stamp := a.snapshot.CheckedAt
			a.mu.RUnlock()
			if time.Now().Unix()-stamp >= int64(a.settings.CheckInterval.Seconds()) {
				if e = a.refresh(ctx); e != nil {
					a.failures.Add(1)
					log.Printf("pub infrastructure refresh: %v (last configuration retained)", e)
				}
			}
		} else {
			log.Printf("pub infrastructure control: %v", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Fixed official sources. No request, ENV domain or fetched redirect can turn this into a proxy.
var infraSources = []struct {
	Name, URL string
	Hosts     bool
}{
	{"EasyList", "https://easylist.to/easylist/easylist.txt", false},
	{"EasyPrivacy", "https://easylist.to/easylist/easyprivacy.txt", false},
	{"PeterLowe", "https://pgl.yoyo.org/adservers/serverlist.php?hostformat=plaintext&showintro=0&mimetype=plaintext", true},
	{"AdGuardBase", "https://filters.adtidy.org/extension/ublock/filters/2.txt", false},
	{"AdGuardTracking", "https://filters.adtidy.org/extension/ublock/filters/3.txt", false},
}

func (a *infrastructure) download(ctx context.Context, raw string) (string, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return "", e
	}
	req.Header.Set("User-Agent", "TwinBid-Pub-Infrastructure/1")
	res, e := a.client.Do(req)
	if e != nil {
		return "", e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, a.settings.MaxBytes+1))
	if e != nil {
		return "", e
	}
	if len(b) > int(a.settings.MaxBytes) || len(b) < 32 || bytes.IndexByte(b, 0) >= 0 || strings.HasPrefix(strings.TrimSpace(string(b)), "<") {
		return "", errors.New("invalid or oversized filter response")
	}
	return string(b), nil
}
func normalizeHosts(text string) (string, error) {
	var out strings.Builder
	n := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		if strings.ContainsAny(line, " /\\:$|@") {
			return "", errors.New("invalid Peter Lowe plaintext domain")
		}
		if _, e := publicOrigin("https://" + line); e != nil {
			return "", e
		}
		out.WriteString("||" + line + "^\n")
		n++
	}
	if n < 1 {
		return "", errors.New("empty domain list")
	}
	return out.String(), nil
}
func (a *infrastructure) probe(ctx context.Context, origin string) bool {
	for attempt := 0; attempt < 2; attempt++ {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, _ := http.NewRequestWithContext(probeCtx, "GET", origin+a.settings.Paths.Health, nil)
		res, e := a.client.Do(req)
		if e == nil {
			var health struct {
				Fingerprint string `json:"fingerprint"`
			}
			e = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&health)
			res.Body.Close()
			if res.StatusCode == 200 && e == nil && health.Fingerprint == a.settings.Fingerprint {
				cancel()
				return true
			}
		}
		cancel()
	}
	return false
}

type filterRequest struct {
	URL       string `json:"url"`
	SourceURL string `json:"source_url"`
	Type      string `json:"type"`
	Origin    string `json:"origin"`
	Component string `json:"component"`
}
type filterFinding struct {
	Origin    string `json:"origin"`
	Component string `json:"component"`
	Blocked   bool   `json:"blocked"`
}

func (a *infrastructure) filterRequests() []filterRequest {
	var out []filterRequest
	p := a.settings.Paths
	token := "00000000-0000-4000-8000-000000000001." + strings.Repeat("0", 64)
	for _, page := range a.settings.Pages {
		out = append(out, filterRequest{a.settings.Stable + p.SDK, page, "script", a.settings.Stable, "bootstrap"}, filterRequest{a.settings.Stable + p.Infrastructure, page, "xmlhttprequest", a.settings.Stable, "infrastructure"}, filterRequest{a.settings.Stable + p.Diagnostics, page, "other", a.settings.Stable, "diagnostics_preflight"})
		for _, o := range a.settings.Origins {
			for _, v := range []struct{ Path, Type, Component string }{{p.Runtime, "script", "runtime"}, {p.Config + "?code=tb_probe&visitor_id=00000000-0000-4000-8000-000000000001&page_session_id=00000000-0000-4000-8000-000000000002", "xmlhttprequest", "config"}, {p.Request, "xmlhttprequest", "request"}, {p.Request, "other", "request_preflight"}, {p.Request, "xmlhttprequest", "request_preflight"}, {p.Delivery + token, "main_frame", "delivery"}, {p.Delivery + token, "popup", "delivery"}, {p.Delivery + token, "xmlhttprequest", "delivery_probe"}, {p.Diagnostics, "xmlhttprequest", "diagnostics"}} {
				out = append(out, filterRequest{o + v.Path, page, v.Type, o, v.Component})
			}
			out = append(out, filterRequest{o + p.Confirm + token, o + p.Delivery + token, "xmlhttprequest", o, "confirm"})
		}
	}
	return out
}

// Bounded output buffers also limit a failing helper's stdout/stderr.
type limitedBuffer struct {
	b     bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.b.Len() {
		return 0, errors.New("helper output exceeds limit")
	}
	return b.b.Write(p)
}
func (a *infrastructure) matchFilters(ctx context.Context, lists []string, requests []filterRequest) ([]filterFinding, error) {
	payload, e := json.Marshal(map[string]any{"lists": lists, "requests": requests})
	if e != nil {
		return nil, e
	}
	cmd := exec.CommandContext(ctx, a.settings.Node, "--max-old-space-size=512", a.settings.FilterScript)
	cmd.Stdin = bytes.NewReader(payload)
	out := limitedBuffer{limit: 8 << 20}
	errs := limitedBuffer{limit: 8192}
	cmd.Stdout = &out
	cmd.Stderr = &errs
	if e = cmd.Run(); e != nil {
		return nil, fmt.Errorf("filter helper: %w: %s", e, errs.b.String())
	}
	var findings []filterFinding
	if e = json.Unmarshal(out.b.Bytes(), &findings); e != nil || len(findings) != len(requests) {
		return nil, errors.New("invalid filter helper output")
	}
	for i, f := range findings {
		if f.Origin != requests[i].Origin || f.Component != requests[i].Component {
			return nil, errors.New("filter helper output identity mismatch")
		}
	}
	return findings, nil
}
func (a *infrastructure) refresh(parent context.Context) error {
	// One lease covers this bounded job; publish checks ownership (fencing on expiry).
	ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
	defer cancel()
	lease := uuid.NewString()
	ok, e := a.redis.SetNX(ctx, infraLeaseKey, lease, 5*time.Minute).Result()
	if e != nil || !ok {
		return e
	}
	defer func() {
		release, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
		defer cancel()
		a.redis.Eval(release, infraReleaseLua, []string{infraLeaseKey}, lease)
	}()
	if e = a.poll(ctx); e != nil {
		return e
	}
	a.mu.RLock()
	old := a.snapshot
	a.mu.RUnlock()
	if time.Now().Unix()-old.CheckedAt < int64(a.settings.CheckInterval.Seconds()) {
		return nil
	}
	attempted, e := a.redis.SetNX(ctx, infraAttemptKey, lease, a.settings.CheckInterval).Result()
	if e != nil || !attempted {
		return e
	}
	var lists []string
	for _, source := range infraSources {
		text, e := a.download(ctx, source.URL)
		if e != nil {
			return fmt.Errorf("%s: %w", source.Name, e)
		}
		if source.Hosts {
			text, e = normalizeHosts(text)
			if e != nil {
				return fmt.Errorf("%s: %w", source.Name, e)
			}
		} else if !strings.HasPrefix(strings.TrimSpace(text), "[Adblock") && !strings.HasPrefix(strings.TrimSpace(text), "!") {
			return fmt.Errorf("%s: missing filter header", source.Name)
		}
		lists = append(lists, text)
	}
	findings, e := a.match(ctx, lists, a.filterRequests())
	if e != nil {
		return e
	}
	blocked := map[string][]string{}
	for _, f := range findings {
		if f.Blocked {
			found := false
			for _, reason := range blocked[f.Origin] {
				if reason == f.Component {
					found = true
					break
				}
			}
			if !found {
				blocked[f.Origin] = append(blocked[f.Origin], f.Component)
			}
		}
	}
	next := infraSnapshot{Fingerprint: a.settings.Fingerprint, CheckedAt: time.Now().Unix(), StableState: "available"}
	if len(blocked[a.settings.Stable]) > 0 {
		next.StableState = "blocked"
	} else if !a.probe(ctx, a.settings.Stable) {
		next.StableState = "unavailable"
	}
	availability := map[string]bool{}
	var probeMu sync.Mutex
	var probes sync.WaitGroup
	slots := make(chan struct{}, 4)
	for _, origin := range a.settings.Origins {
		if len(blocked[origin]) > 0 {
			continue
		}
		probes.Add(1)
		go func(o string) {
			defer probes.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			ok := a.probe(ctx, o)
			probeMu.Lock()
			availability[o] = ok
			probeMu.Unlock()
		}(origin)
	}
	probes.Wait()
	for _, o := range a.settings.Origins {
		d := infraDomain{Origin: o, State: "available"}
		if len(blocked[o]) > 0 {
			d.State = "blocked"
			d.Reasons = blocked[o]
			sort.Strings(d.Reasons)
		} else if !availability[o] {
			d.State = "unavailable"
			d.Reasons = []string{"two_failed_health_probes"}
		}
		next.Domains = append(next.Domains, d)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	b, _ := json.Marshal(next)
	n, e := a.redis.Eval(ctx, infraPublishLua, []string{infraLeaseKey, infraSnapshotKey}, lease, b).Int()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("infrastructure lease expired; snapshot not published")
	}
	if !bytes.Equal(mustDomains(old), mustDomains(next)) {
		a.rotations.Add(1)
	}
	a.mu.Lock()
	a.snapshot = next
	a.fault = ""
	a.mu.Unlock()
	log.Printf("pub infrastructure checked=%d stable=%s domains=%s auto_rotate=%t", next.CheckedAt, next.StableState, mustDomains(next), a.settings.Rotate)
	return nil
}
func mustDomains(s infraSnapshot) []byte { b, _ := json.Marshal(s.Domains); return b }

// Pin only globally routable resolved addresses; disable proxy inheritance and redirects.
// This protects both operator-configured pool probes and fixed list downloads from SSRF/rebinding.
func publicHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout, DisableKeepAlives: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil || port != "443" {
			return nil, errors.New("infrastructure dial requires port 443")
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, errors.New("infrastructure DNS unavailable")
		}
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, errors.New("infrastructure DNS resolved a non-public IP")
			}
		}
		var last error
		for _, ip := range ips {
			conn, e := (&net.Dialer{Timeout: timeout}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}}}
}
func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, p := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96"} {
		if netip.MustParsePrefix(p).Contains(ip) {
			return false
		}
	}
	return true
}
