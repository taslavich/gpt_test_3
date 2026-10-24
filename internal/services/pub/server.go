package pub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	shard "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Request struct {
	ORTBID       string `json:"-"`
	SessionToken string `json:"session_token"`
	ID           string `json:"request_id"`
	Code         string `json:"code"`
	PageURL      string `json:"page_url"`
	SessionID    string `json:"page_session_id"`
	VisitorID    string `json:"visitor_id"`
	Click        int    `json:"page_click"`
}
type Delivery struct {
	Token       string
	RequestID   string
	ZoneID      string
	VisitorID   string
	SessionID   string
	Epoch       string
	Destination string
	StateTTL    int64
	Expires     int64
	CodeID      string
}
type Result struct {
	Status    string `json:"status"`
	RequestID string `json:"request_id"`
	OpenURL   string `json:"open_url,omitempty"`
	Error     bool   `json:"error"`
	Reason    string `json:"reason,omitempty"`
}
type Server struct {
	safetyUntil atomic.Int64

	Config         Config
	InfraSettings  infraSettings
	Infrastructure *infrastructure
	ORTB           []*redis.Client
	Redis          redis.UniversalClient
	HTTP           *http.Client
	Telemetry      *Telemetry
	slots          chan struct{}
	proxies        []*net.IPNet
	lastMu         sync.Mutex
	last           map[string]int64
	Requests       atomic.Uint64
	Rejected       atomic.Uint64
	RTBCalls       atomic.Uint64
	breakerUntil   atomic.Int64
	failures       atomic.Int64
}

func NewServer(cfg Config, r redis.UniversalClient, t *Telemetry) (*Server, error) {
	if e := cfg.Validate(); e != nil {
		return nil, e
	}
	s := &Server{Config: cfg, Redis: r, Telemetry: t, slots: make(chan struct{}, cfg.MaxInflight), last: map[string]int64{}, HTTP: &http.Client{Timeout: cfg.RTBTimeout, Transport: &http.Transport{MaxIdleConns: 4096, MaxIdleConnsPerHost: 2048, MaxConnsPerHost: cfg.MaxInflight, IdleConnTimeout: 90 * time.Second}, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
	s.InfraSettings, _ = infrastructureSettings(cfg)
	if cfg.AntiBlockEnabled {
		initCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		s.Infrastructure = newInfrastructure(initCtx, s.InfraSettings, r)
		cancel()
	}
	for _, raw := range strings.Split(cfg.TrustedProxies, ",") {
		if raw == "" {
			continue
		}
		_, n, e := net.ParseCIDR(strings.TrimSpace(raw))
		if e != nil {
			return nil, fmt.Errorf("PUB_TRUSTED_PROXIES: %w", e)
		}
		s.proxies = append(s.proxies, n)
	}
	var e error
	s.ORTB, e = shard.NewRedisShardedClientsForDB(strings.Split(cfg.ORTBAddrs, ","), cfg.ORTBPassword, cfg.ORTBDB, cfg.ORTBTLS, 256, 16)
	if e != nil {
		return nil, e
	}
	check, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.RefreshSafety(check)
	return s, nil
}
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]bool{"alive": true}) })
	m.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		_, e := s.ready(r.Context())
		if e != nil || !s.Config.ConfirmedOpenEnabled {
			respond(w, 503, map[string]bool{"ready": false})
			return
		}
		respond(w, 200, map[string]bool{"ready": true})
	})
	m.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "pub_requests_total %d\npub_rejected_total %d\npub_rtb_calls_total %d\n", s.Requests.Load(), s.Rejected.Load(), s.RTBCalls.Load())
		s.infrastructureMetrics(w)
	})
	m.HandleFunc("GET /config", s.publicConfig)
	m.HandleFunc("POST /request", s.request)
	m.HandleFunc("GET /delivery/{token}", s.delivery)
	m.HandleFunc("POST /confirm/{token}", s.confirm)
	m.HandleFunc("GET /static/adManager.js", s.script)
	s.registerInfrastructureRoutes(m)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if s.Config.AntiBlockEnabled || s.Config.AntiBlockDomains != "" {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-TwinBid-Infrastructure")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		m.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (s *Server) code(ctx context.Context, short, epoch string) (c.Code, error) {
	var out c.Code
	if len(short) > 80 || !strings.HasPrefix(short, "tb_") {
		return out, errors.New("unknown_code")
	}
	b, e := s.Redis.HGet(ctx, c.CodeKey(short), "data").Bytes()
	if e != nil {
		return out, errors.New("code_unavailable")
	}
	if json.Unmarshal(b, &out) != nil || out.Epoch != epoch || out.Validate() != nil {
		return out, errors.New("code_generation_mismatch")
	}
	return out, nil
}
func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	epoch, e := s.ready(r.Context())
	if e != nil {
		respond(w, 503, map[string]string{"error": "config_unready"})
		return
	}
	code, e := s.code(r.Context(), r.URL.Query().Get("code"), epoch)
	if e != nil {
		respond(w, 404, map[string]string{"error": e.Error()})
		return
	}
	session := ""
	visitor := r.URL.Query().Get("visitor_id")
	pageSessionID := r.URL.Query().Get("page_session_id")
	if _, e := uuid.Parse(visitor); e == nil {
		if _, e = uuid.Parse(pageSessionID); e == nil {
			session = sessionToken(s.Config.DeliverySecret, code.ShortCode, visitor, pageSessionID)
		}
	}
	respond(w, 200, map[string]any{"session_token": session, "trigger_click": code.TriggerClick, "frequency_minutes": code.FrequencyMinutes, "max_per_session": code.MaxPerSession, "device": code.Device, "active": code.Active && !code.Archived})
}
func (s *Server) clientIP(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	trusted := false
	for _, p := range s.proxies {
		if p.Contains(peer) {
			trusted = true
			break
		}
	}
	if trusted {
		forwarded := r.Header.Get("X-Real-IP")
		if ip := net.ParseIP(forwarded); ip != nil {
			return ip.String()
		}
	}
	if peer == nil {
		return ""
	}
	return peer.String()
}
func (s *Server) negative(w http.ResponseWriter, r *http.Request, in Request, code c.Code, host, site, zone, reason string, status int) {
	s.Rejected.Add(1)
	if _, e := uuid.Parse(in.ID); e != nil {
		in.ID = uuid.NewString()
	}
	// After RTB dispatch, the request already has ONE object. Never emit another.
	if in.ORTBID == "" {
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(c.Sign(s.Config.InternalSecret, []string{in.Code, code.UserID, in.VisitorID, in.SessionID, in.ID}))).String()
		fields := s.publisherFields(r, in, code, host, site, zone, reason, status)
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
		_, e := writeError(ctx, s.ORTB, s.Config.ORTBReadySet, id, fields)
		cancel()
		if e != nil && (s.Telemetry == nil || s.Telemetry.Record(id, fields) != nil) {
			respond(w, 503, Result{Status: "error", RequestID: in.ID, Error: true, Reason: "telemetry_unavailable"})
			return
		}
	}
	respond(w, status, Result{Status: "error", RequestID: in.ID, Error: true, Reason: reason})
}
func (s *Server) request(w http.ResponseWriter, r *http.Request) {
	s.Requests.Add(1)
	in := Request{}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(&in); e != nil {
		s.negative(w, r, in, c.Code{}, "", "", "", "invalid_request", 400)
		return
	}
	if _, e := uuid.Parse(in.ID); e != nil {
		s.negative(w, r, in, c.Code{}, "", "", "", "invalid_request_id", 400)
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		s.negative(w, r, in, c.Code{}, "", "", "", "overload", 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.Config.RTBTimeout+3*time.Second)
	defer cancel()
	epoch, e := s.ready(ctx)
	if e != nil {
		s.negative(w, r, in, c.Code{}, "", "", "", "config_unready", 503)
		return
	}
	code, e := s.code(ctx, in.Code, epoch)
	if e != nil {
		s.negative(w, r, in, code, "", "", "", e.Error(), 404)
		return
	}
	host, page, e := c.Page(in.PageURL)
	if e != nil {
		s.negative(w, r, in, code, "", "", "", "invalid_page_url", 400)
		return
	}
	site := c.SiteID(host)
	zone := c.ZoneID(site, code.ID)
	in.PageURL = page
	reject := func(reason string, status int) { s.negative(w, r, in, code, host, site, zone, reason, status) }
	if !code.Active || code.Archived {
		reject("code_paused", 403)
		return
	}
	if in.Click < code.TriggerClick || in.Click > 100000 {
		reject("trigger_click", 403)
		return
	}
	if _, e = uuid.Parse(in.VisitorID); e != nil {
		reject("invalid_visitor_id", 400)
		return
	}
	if _, e = uuid.Parse(in.SessionID); e != nil {
		reject("invalid_page_session_id", 400)
		return
	}
	if e = validSession(s.Config.DeliverySecret, in); e != nil {
		reject(e.Error(), 403)
		return
	}
	mobile := strings.Contains(strings.ToLower(r.UserAgent()), "mobile") || strings.Contains(strings.ToLower(r.UserAgent()), "android")
	if (code.Device == "desktop" && mobile) || (code.Device == "mobile" && !mobile) {
		reject("device", 403)
		return
	}
	if len(r.UserAgent()) < 20 || s.clientIP(r) == "" {
		reject("invalid_client_context", 400)
		return
	}
	// A generation-tagged global policy hash ensures a missing policy store fails closed.
	policy, e := s.Redis.HMGet(ctx, c.Prefix+"{control}:blocks", "epoch", host).Result()
	if e != nil || len(policy) != 2 || policy[0] != epoch {
		reject("policy_unready", 503)
		return
	}
	if policy[1] == "1" {
		reject("domain_blocked", 403)
		return
	}
	if !s.Config.ConfirmedOpenEnabled {
		reject("confirmed_open_contract_unverified", 503)
		return
	}
	if time.Now().UnixMilli() < s.breakerUntil.Load() {
		reject("rtb_circuit_open", 503)
		return
	}
	deliveryPrefix, e := s.requestDeliveryPrefix(ctx, r)
	if e != nil {
		reject("infrastructure_unavailable", 503)
		return
	}
	reservation, e := s.reserve(ctx, code, zone, in)
	if e != nil {
		reject("frequency_unavailable", 503)
		return
	}
	if reservation != "reserved" {
		if strings.HasPrefix(reservation, "{") {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, reservation)
			return
		}
		reject(reservation, 409)
		return
	}
	delivery := Delivery{RequestID: in.ID, ZoneID: zone, VisitorID: in.VisitorID, SessionID: in.SessionID, Epoch: epoch, StateTTL: stateTTL(code), CodeID: code.ID, Expires: time.Now().Add(45 * time.Second).Unix()}
	// Pin the request across a client disconnect; the bounded server operation owns its lifetime.
	work, cancelWork := context.WithTimeout(context.WithoutCancel(ctx), s.Config.RTBTimeout+2*time.Second)
	defer cancelWork()
	result, err := s.auction(work, r, &in, code, host, site, zone, epoch)
	if err != nil {
		s.finish(work, delivery, `{"status":"error","error":true,"reason":"rtb_failed"}`)
		reject("rtb_failed", 502)
		return
	}
	s.lastMu.Lock()
	if len(s.last) < 100000 {
		s.last[code.ID+":"+zone] = time.Now().UnixMilli()
	}
	s.lastMu.Unlock()
	if result == nil {
		out := Result{Status: "no_fill", RequestID: in.ID}
		b, _ := json.Marshal(out)
		if e = s.finish(work, delivery, string(b)); e != nil {
			reject("reservation_release_failed", 503)
			return
		}
		respond(w, 200, out)
		return
	}
	delivery.Destination = result.ADM
	delivery.Token = uuid.NewString()
	b, _ := json.Marshal(delivery)
	token := delivery.Token + "." + c.Sign(s.Config.DeliverySecret, delivery.Token)
	if e = s.Redis.Set(work, c.Prefix+"delivery:"+delivery.Token, b, 45*time.Second).Err(); e != nil {
		s.finish(work, delivery, `{"status":"error","error":true,"reason":"delivery_unavailable"}`)
		reject("delivery_unavailable", 503)
		return
	}
	out := Result{Status: "fill", RequestID: in.ID, OpenURL: deliveryPrefix + token}
	outJSON, _ := json.Marshal(out)
	// Keep the reservation until confirmation; cache the response idempotently.
	_, _, key := keys(zone, in.VisitorID, in.SessionID, in.ID)
	if e = s.Redis.Set(work, key, outJSON, 5*time.Minute).Err(); e != nil {
		reject("request_cache_failed", 503)
		return
	}
	respond(w, 200, out)
}

type bid struct {
	ADM  string `json:"adm"`
	BURL string `json:"burl"`
}

func (s *Server) auction(ctx context.Context, r *http.Request, in *Request, code c.Code, host, site, zone, epoch string) (*bid, error) {
	endpoint, feed, err := s.feed(code.TrafficType)
	if err != nil {
		return nil, err
	}
	u, e := url.Parse(endpoint)
	if e != nil {
		return nil, e
	}
	q := u.Query()
	q.Set("feed", feed)
	u.RawQuery = q.Encode()
	ip := s.clientIP(r)
	device := map[string]any{"ua": r.UserAgent()}
	if strings.Contains(ip, ":") {
		device["ipv6"] = ip
	} else {
		device["ip"] = ip
	}
	payload := map[string]any{"id": in.ID, "imp": []any{map[string]any{"id": "1", "banner": map[string]any{}, "bidfloor": 0, "secure": 1}}, "site": map[string]any{"id": site, "domain": host, "page": in.PageURL}, "device": device, "cur": []string{"USD"}}
	body, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(body))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	var parsed ortb.BidRequest
	if e = json.Unmarshal(body, &parsed); e != nil {
		return nil, e
	}
	id := uuid.NewString()
	stamp := fmt.Sprint(time.Now().Unix())
	signature := pubuuid.Sign(s.Config.InternalSecret, id, feed, stamp, &parsed)
	if e = s.prepareORTB(ctx, id, signature, s.publisherFields(r, *in, code, host, site, zone, "", 0)); e != nil {
		return nil, e
	}
	in.ORTBID = id
	req.Header.Set(pubuuid.Header, id)
	req.Header.Set(pubuuid.TimeHeader, stamp)
	req.Header.Set(pubuuid.SignatureHeader, signature)
	s.RTBCalls.Add(1)
	res, e := s.HTTP.Do(req)
	if e != nil {
		// If no adapter claimed the UUID, atomically refuse future admission and
		// commit an error. If claimed (or Redis unavailable), do not guess its outcome.
		settle, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		fields := s.publisherFields(r, *in, code, host, site, zone, "rtb_transport_before_admission", 502)
		_, _ = writeError(settle, s.ORTB, s.Config.ORTBReadySet, id, fields)
		cancel()
		if s.failures.Add(1) >= 5 {
			s.breakerUntil.Store(time.Now().Add(5 * time.Second).UnixMilli())
		}
		return nil, e
	}
	defer res.Body.Close()
	if res.StatusCode == 204 {
		s.failures.Store(0)
		return nil, nil
	}
	if res.StatusCode != 200 {
		fields := s.publisherFields(r, *in, code, host, site, zone, "rtb_refused_before_admission", res.StatusCode)
		_, _ = writeError(ctx, s.ORTB, s.Config.ORTBReadySet, id, fields)
		if s.failures.Add(1) >= 5 {
			s.breakerUntil.Store(time.Now().Add(5 * time.Second).UnixMilli())
		}
		return nil, fmt.Errorf("RTB status %d", res.StatusCode)
	}
	s.failures.Store(0)
	var response struct {
		Seatbid []struct {
			Bid []bid `json:"bid"`
		} `json:"seatbid"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&response); e != nil {
		return nil, e
	}
	for _, seat := range response.Seatbid {
		for _, b := range seat.Bid {
			if b.ADM != "" {
				return &b, nil
			}
		}
	}
	return nil, nil
}

func (s *Server) loadDelivery(ctx context.Context, token string) (Delivery, error) {
	parts := strings.Split(token, ".")
	var d Delivery
	if len(parts) != 2 || !c.Verify(s.Config.DeliverySecret, parts[0], parts[1]) {
		return d, errors.New("invalid token")
	}
	b, e := s.Redis.Get(ctx, c.Prefix+"delivery:"+parts[0]).Bytes()
	if e != nil {
		return d, e
	}
	if e = json.Unmarshal(b, &d); e != nil {
		return d, e
	}
	epoch, e := s.ready(ctx)
	if e != nil || epoch != d.Epoch || time.Now().Unix() > d.Expires {
		return d, errors.New("expired delivery/generation")
	}
	return d, nil
}
func (s *Server) delivery(w http.ResponseWriter, r *http.Request) {
	d, e := s.loadDelivery(r.Context(), r.PathValue("token"))
	if e != nil || !s.Config.ConfirmedOpenEnabled {
		http.Error(w, "delivery unavailable", 410)
		return
	}
	dest, _ := json.Marshal(d.Destination)
	confirmPrefix := "/confirm/"
	if s.InfraSettings.Paths.Delivery != "/delivery/" && strings.HasPrefix(r.URL.Path, s.InfraSettings.Paths.Delivery) {
		confirmPrefix = s.InfraSettings.Paths.Confirm
	}
	if s.Config.AntiBlockEnabled && s.InfraSettings.Paths.Delivery == "/delivery/" {
		confirmPrefix = s.InfraSettings.Paths.Confirm
	}
	confirm, _ := json.Marshal(confirmPrefix + r.PathValue("token"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; connect-src 'self'")
	if s.Config.AntiBlockEnabled {
		eventURL, _ := json.Marshal(s.InfraSettings.Paths.Diagnostics)
		fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><script>const report=(component,reason)=>{try{void fetch(%s,{method:'POST',credentials:'omit',redirect:'error',headers:{'Content-Type':'application/json'},body:JSON.stringify({component,reason}),signal:AbortSignal.timeout(2000)}).catch(()=>{})}catch{}};window.addEventListener('load',async()=>{report('delivery','loaded');let stage='confirm';try{let r=await fetch(%s,{method:'POST',credentials:'omit',redirect:'error',signal:AbortSignal.timeout(5000)});if(r.ok){stage='delivery';location.replace(%s)}else{report('confirm','business_refusal');window.close()}}catch{report(stage,'network');window.close()}},{once:true});</script>`, eventURL, confirm, dest)
		return
	}
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><script>window.addEventListener('load',async()=>{try{let r=await fetch(%s,{method:'POST',credentials:'omit'});if(r.ok)location.replace(%s);else window.close()}catch(e){window.close()}},{once:true});</script>`, confirm, dest)
}
func (s *Server) confirm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	d, e := s.loadDelivery(ctx, r.PathValue("token"))
	if e != nil || !s.Config.ConfirmedOpenEnabled {
		http.Error(w, "invalid delivery", 410)
		return
	}
	n, e := s.confirmState(ctx, d)
	if e != nil || n == 0 {
		http.Error(w, "reservation unavailable", 409)
		return
	}
	// No HTTP callback, impression, click or money event here. The browser
	// follows the original ADM; its original handlers own trackers and billing.
	w.WriteHeader(204)
}
func (s *Server) FlushLast(ctx context.Context) error {
	s.lastMu.Lock()
	snapshot := s.last
	s.last = map[string]int64{}
	s.lastMu.Unlock()
	for key, stamp := range snapshot {
		if e := s.Redis.Eval(ctx, `local old=tonumber(redis.call('GET',KEYS[1]) or '0');if tonumber(ARGV[1])>old then redis.call('SET',KEYS[1],ARGV[1],'EX',604800) end;return 1`, []string{c.Prefix + "last:" + key}, stamp).Err(); e != nil {
			s.lastMu.Lock()
			for k, v := range snapshot {
				if v > s.last[k] {
					s.last[k] = v
				}
			}
			s.lastMu.Unlock()
			return e
		}
	}
	return nil
}
