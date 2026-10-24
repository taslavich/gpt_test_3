package pub

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type infraPaths struct {
	SDK            string `json:"sdk"`
	Infrastructure string `json:"infrastructure"`
	Runtime        string `json:"runtime"`
	Config         string `json:"config"`
	Request        string `json:"request"`
	Delivery       string `json:"delivery"`
	Confirm        string `json:"confirm"`
	Diagnostics    string `json:"diagnostics"`
	Health         string `json:"health"`
}

func defaultInfraPaths() infraPaths {
	return infraPaths{"/sdk.js", "/infra.json", "/assets/runtime.js", "/config", "/request", "/delivery/", "/confirm/", "/infra/event", "/infra/health"}
}

type infraSettings struct {
	Enabled           bool          `json:"enabled"`
	Fingerprint       string        `json:"fingerprint"`
	RuntimeIntegrity  string        `json:"runtime_integrity"`
	Stable            string        `json:"stable_origin"`
	Origins           []string      `json:"origins"`
	Paths             infraPaths    `json:"paths"`
	LegacyRuntime     string        `json:"legacy_runtime"`
	RuntimePreferred  string        `json:"-"`
	APIPreferred      string        `json:"-"`
	DeliveryPreferred string        `json:"-"`
	Pages             []string      `json:"-"`
	Rotate            bool          `json:"-"`
	CheckInterval     time.Duration `json:"-"`
	PollInterval      time.Duration `json:"-"`
	Timeout           time.Duration `json:"-"`
	MaxBytes          int64         `json:"-"`
	Node              string        `json:"-"`
	FilterScript      string        `json:"-"`
}

var originHost = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

func publicOrigin(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || (u.Port() != "" && u.Port() != "443") {
		return "", errors.New("expected HTTPS origin with port 443 and no path/query/userinfo")
	}
	host := strings.ToLower(u.Hostname())
	if !originHost.MatchString(host) || !strings.Contains(host, ".") || strings.Contains(host, "..") || strings.HasSuffix(host, ".") || net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return "", errors.New("expected public DNS hostname")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", errors.New("invalid hostname")
		}
	}
	if len(host) > 253 {
		return "", errors.New("invalid hostname")
	}
	return "https://" + host, nil
}

var infraPathPattern = regexp.MustCompile(`^/[A-Za-z0-9_/-]+(?:\.(?:js|json))?/?$`)

func (p infraPaths) validate() error {
	all := []string{p.SDK, p.Infrastructure, p.Runtime, p.Config, p.Request, p.Delivery, p.Confirm, p.Diagnostics, p.Health}
	reserved := map[string]bool{"/health": true, "/ready": true, "/metrics": true, "/static/adManager.js": true}
	seen := map[string]bool{}
	for i, v := range all {
		prefix := i == 5 || i == 6
		if !infraPathPattern.MatchString(v) || strings.Contains(v, "//") || strings.Contains(v, "..") || v == "/" || strings.HasSuffix(v, "/") != prefix || reserved[v] || seen[strings.TrimSuffix(v, "/")] {
			return fmt.Errorf("PUB_ANTIBLOCK_ROUTES_JSON: invalid/conflicting path %q", v)
		}
		// Fixed legacy routes must never be reassigned to another component.
		for legacy, index := range map[string]int{"/config": 3, "/request": 4, "/delivery/": 5, "/confirm/": 6} {
			if v == legacy && i != index {
				return fmt.Errorf("PUB_ANTIBLOCK_ROUTES_JSON: reserved path %q", v)
			}
		}
		seen[strings.TrimSuffix(v, "/")] = true
	}
	for i, a := range all {
		for j, b := range all {
			if i != j && (i == 5 || i == 6) && strings.HasPrefix(b, a) {
				return errors.New("PUB_ANTIBLOCK_ROUTES_JSON: overlapping prefix")
			}
		}
	}
	return nil
}
func infrastructureSettings(c Config) (infraSettings, error) {
	s := infraSettings{Enabled: c.AntiBlockEnabled, Paths: defaultInfraPaths(), LegacyRuntime: strings.TrimRight(c.PublicURL, "/") + "/static/adManager.js", Rotate: c.AntiBlockAutoRotate, CheckInterval: c.AntiBlockCheckInterval, PollInterval: c.AntiBlockPollInterval, Timeout: c.AntiBlockHTTPTimeout, MaxBytes: c.AntiBlockMaxListBytes, FilterScript: c.AntiBlockFilterScript, Node: c.AntiBlockNode}
	if c.AntiBlockRoutes != "" {
		d := json.NewDecoder(strings.NewReader(c.AntiBlockRoutes))
		d.DisallowUnknownFields()
		if e := d.Decode(&s.Paths); e != nil {
			return s, fmt.Errorf("PUB_ANTIBLOCK_ROUTES_JSON: %w", e)
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return s, errors.New("PUB_ANTIBLOCK_ROUTES_JSON: multiple values")
		}
	}
	if e := s.Paths.validate(); e != nil {
		return s, e
	}
	if !s.Enabled {
		s.Stable = strings.TrimRight(c.PublicURL, "/")
		return s, nil
	}
	runtime, err := runtimeFiles.ReadFile("runtime.prod.js")
	if err != nil {
		return s, err
	}
	digest := sha512.Sum384(runtime)
	s.RuntimeIntegrity = "sha384-" + base64.StdEncoding.EncodeToString(digest[:])
	var e error
	s.Stable, e = publicOrigin(c.AntiBlockStableOrigin)
	if e != nil {
		return s, fmt.Errorf("PUB_ANTIBLOCK_STABLE_ORIGIN: %w", e)
	}
	legacy, e := publicOrigin(c.PublicURL)
	if e != nil {
		return s, fmt.Errorf("PUB_PUBLIC_URL: %w", e)
	}
	s.LegacyRuntime = legacy + "/static/adManager.js"
	seen := map[string]bool{}
	for _, raw := range strings.Split(c.AntiBlockDomains, ",") {
		o, e := publicOrigin(strings.TrimSpace(raw))
		if e != nil || seen[o] {
			return s, fmt.Errorf("PUB_ANTIBLOCK_DOMAINS: invalid/duplicate origin %q", raw)
		}
		seen[o] = true
		s.Origins = append(s.Origins, o)
	}
	if len(s.Origins) < 1 || len(s.Origins) > 32 {
		return s, errors.New("PUB_ANTIBLOCK_DOMAINS: use 1..32 explicit origins")
	}
	for _, pref := range []struct {
		raw  string
		dest *string
	}{{c.AntiBlockRuntimeOrigin, &s.RuntimePreferred}, {c.AntiBlockAPIOrigin, &s.APIPreferred}, {c.AntiBlockDeliveryOrigin, &s.DeliveryPreferred}} {
		raw, dest := pref.raw, pref.dest
		if raw == "" {
			continue
		}
		o, e := publicOrigin(raw)
		if e != nil || !seen[o] {
			return s, errors.New("PUB_ANTIBLOCK_*_ORIGIN must belong to PUB_ANTIBLOCK_DOMAINS")
		}
		*dest = o
	}
	if s.CheckInterval == 0 {
		s.CheckInterval = 24 * time.Hour
	}
	if s.PollInterval == 0 {
		s.PollInterval = 15 * time.Second
	}
	if s.Timeout == 0 {
		s.Timeout = 10 * time.Second
	}
	if s.MaxBytes == 0 {
		s.MaxBytes = 16 << 20
	}
	if s.Node == "" {
		s.Node = "node"
	}
	if s.FilterScript == "" {
		s.FilterScript = "scripts/pub-client/filter.cjs"
	}
	if s.CheckInterval < time.Minute || s.CheckInterval > 7*24*time.Hour || s.PollInterval < time.Second || s.PollInterval > time.Minute || s.Timeout < time.Second || s.Timeout > 30*time.Second || s.MaxBytes < 1024 || s.MaxBytes > 32<<20 {
		return s, errors.New("invalid PUB_ANTIBLOCK interval/timeout/list limit")
	}
	pages := c.AntiBlockSamplePages
	if pages == "" {
		pages = "https://publisher.invalid/"
	}
	for _, raw := range strings.Split(pages, ",") {
		u, e := url.Parse(strings.TrimSpace(raw))
		if e != nil || u == nil {
			return s, errors.New("PUB_ANTIBLOCK_SAMPLE_PAGES: invalid URL")
		}
		if _, e = publicOrigin(u.Scheme + "://" + u.Host); e != nil || u.User != nil || u.Fragment != "" {
			return s, errors.New("PUB_ANTIBLOCK_SAMPLE_PAGES: expected public HTTPS page URL")
		}
		s.Pages = append(s.Pages, u.String())
	}
	if len(s.Pages) > 64 {
		return s, errors.New("PUB_ANTIBLOCK_SAMPLE_PAGES: max 64 contexts")
	}
	// This includes operator intent, not only published fields, so mixed fleet ENV fails closed.
	b, _ := json.Marshal(struct {
		Public      infraSettings
		Pool, Pages []string
		Preferences []string
		Rotate      bool
		Interval    int64
	}{s, s.Origins, s.Pages, []string{s.RuntimePreferred, s.APIPreferred, s.DeliveryPreferred}, s.Rotate, int64(s.CheckInterval)})
	h := sha256.Sum256(b)
	s.Fingerprint = hex.EncodeToString(h[:])
	return s, nil
}
