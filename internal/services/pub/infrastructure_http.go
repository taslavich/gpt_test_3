package pub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

func (s *Server) infrastructureConfig(w http.ResponseWriter, r *http.Request) {
	if s.Infrastructure == nil {
		respond(w, 200, infraManifest{infraSettings: s.InfraSettings, Status: "disabled"})
		return
	}
	m, e := s.Infrastructure.manifest()
	if e != nil {
		respond(w, 503, map[string]string{"error": e.Error()})
		return
	}
	respond(w, 200, m)
}
func (s *Server) infrastructureHealth(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]string{"fingerprint": s.InfraSettings.Fingerprint})
}
func (s *Server) sdk(w http.ResponseWriter, r *http.Request) {
	b, e := runtimeFiles.ReadFile("sdk.prod.js")
	if e != nil {
		http.Error(w, "SDK unavailable", 503)
		return
	}
	settings, _ := json.Marshal(s.InfraSettings)
	b = []byte(strings.Replace(string(b), "__TWINBID_BOOTSTRAP_SETTINGS__", string(settings), 1))
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public,max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}
func (s *Server) routedRuntime(w http.ResponseWriter, r *http.Request) {
	b, e := runtimeFiles.ReadFile("runtime.prod.js")
	if e != nil {
		http.Error(w, "runtime unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public,max-age=60")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}

var infrastructureComponents = map[string]bool{"bootstrap": true, "infrastructure": true, "runtime": true, "config": true, "request": true, "delivery": true, "confirm": true, "popup": true}
var infrastructureReasons = map[string]bool{"loaded": true, "network": true, "invalid_config": true, "untrusted_delivery": true, "blocked": true, "business_refusal": true, "unavailable": true}

func (s *Server) infrastructureEvent(w http.ResponseWriter, r *http.Request) {
	if s.Infrastructure == nil {
		w.WriteHeader(204)
		return
	}
	var event struct {
		Component string `json:"component"`
		Reason    string `json:"reason"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
	d.DisallowUnknownFields()
	if d.Decode(&event) != nil || !infrastructureComponents[event.Component] || !infrastructureReasons[event.Reason] {
		http.Error(w, "invalid diagnostic", 400)
		return
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		http.Error(w, "invalid diagnostic", 400)
		return
	}
	counter, _ := s.Infrastructure.events.LoadOrStore(event.Component+":"+event.Reason, &atomic.Uint64{})
	counter.(*atomic.Uint64).Add(1)
	w.WriteHeader(204)
}
func (s *Server) infrastructureMetrics(w io.Writer) {
	if s.Infrastructure == nil {
		fmt.Fprintln(w, "pub_infrastructure_enabled 0")
		return
	}
	a := s.Infrastructure
	a.mu.RLock()
	snapshot := a.snapshot
	fault := a.fault
	a.mu.RUnlock()
	fmt.Fprintf(w, "pub_infrastructure_enabled 1\npub_infrastructure_checked_at_seconds %d\npub_infrastructure_refresh_failures_total %d\npub_infrastructure_state_changes_total %d\n", snapshot.CheckedAt, a.failures.Load(), a.rotations.Load())
	healthy := 0
	if fault == "" {
		healthy = 1
	}
	fmt.Fprintf(w, "pub_infrastructure_control_valid %d\n", healthy)
	for _, d := range snapshot.Domains {
		n := 0
		if d.State == "available" {
			n = 1
		}
		fmt.Fprintf(w, "pub_infrastructure_domain_available{origin=%q,state=%q} %d\n", d.Origin, d.State, n)
	}
	fmt.Fprintf(w, "pub_infrastructure_stable_state{state=%q} 1\n", snapshot.StableState)
	if manifest, e := a.manifest(); e == nil {
		for _, entry := range []struct {
			component string
			origins   []string
		}{{"runtime", manifest.RuntimeOrigins}, {"api", manifest.APIOrigins}, {"delivery", manifest.DeliveryOrigins}} {
			fmt.Fprintf(w, "pub_infrastructure_selected{component=%q,origin=%q} 1\n", entry.component, entry.origins[0])
		}
	}
	a.events.Range(func(k, v any) bool {
		parts := strings.Split(k.(string), ":")
		fmt.Fprintf(w, "pub_infrastructure_client_events_total{component=%q,reason=%q} %d\n", parts[0], parts[1], v.(*atomic.Uint64).Load())
		return true
	})
}
func (s *Server) requestDeliveryPrefix(ctx context.Context, r *http.Request) (string, error) {
	legacy := strings.TrimRight(s.Config.PublicURL, "/") + "/delivery/"
	header := r.Header.Get("X-TwinBid-Infrastructure")
	if header == "" || s.Infrastructure == nil {
		return legacy, nil
	}
	if s.Infrastructure == nil || header != s.InfraSettings.Fingerprint {
		return "", errors.New("invalid_infrastructure_generation")
	}
	m, e := s.Infrastructure.manifest()
	if e != nil {
		return "", e
	}
	return m.DeliveryOrigins[0] + m.Paths.Delivery, nil
}
func (s *Server) registerInfrastructureRoutes(m *http.ServeMux) {
	p := s.InfraSettings.Paths
	m.HandleFunc("GET "+p.SDK, s.sdk)
	m.HandleFunc("GET "+p.Infrastructure, s.infrastructureConfig)
	m.HandleFunc("GET "+p.Runtime, s.routedRuntime)
	m.HandleFunc("GET "+p.Health, s.infrastructureHealth)
	m.HandleFunc("POST "+p.Diagnostics, s.infrastructureEvent)
	if p.Config != "/config" {
		m.HandleFunc("GET "+p.Config, s.publicConfig)
	}
	if p.Request != "/request" {
		m.HandleFunc("POST "+p.Request, s.request)
	}
	if p.Delivery != "/delivery/" {
		m.HandleFunc("GET "+p.Delivery+"{token}", s.delivery)
	}
	if p.Confirm != "/confirm/" {
		m.HandleFunc("POST "+p.Confirm+"{token}", s.confirm)
	}
}
