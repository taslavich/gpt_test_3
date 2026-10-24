package pub

import (
	"net"
	"net/http/httptest"
	"testing"
)

func TestPublisherOnlyAcceptsTrustedNginx(t *testing.T) {
	_, cidr, err := net.ParseCIDR("138.226.222.12/32")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{proxies: []*net.IPNet{cidr}}
	for _, tc := range []struct {
		name      string
		peer      string
		forwarded string
		want      int
		wantIP    string
	}{
		{"nginx", "138.226.222.12:50000", "203.0.113.45", 200, "203.0.113.45"},
		{"other host with forged X-Real-IP", "203.0.113.12:50000", "203.0.113.45", 403, ""},
		{"other host without header", "203.0.113.12:50000", "", 403, ""},
		{"missing remote IP", "bad-address", "203.0.113.45", 403, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/health", nil)
			request.RemoteAddr = tc.peer
			request.Header.Set("X-Real-IP", tc.forwarded)
			recorder := httptest.NewRecorder()
			s.Handler().ServeHTTP(recorder, request)
			if recorder.Code != tc.want {
				t.Fatalf("status %d, want %d", recorder.Code, tc.want)
			}
			if tc.wantIP != "" && s.clientIP(request) != tc.wantIP {
				t.Fatalf("client ip = %q, want %q", s.clientIP(request), tc.wantIP)
			}
		})
	}
}

func TestPublisherDoesNotUseProxyIPAsVisitorIP(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "/health", nil)
	r.RemoteAddr = "138.226.222.12:1234"
	if got := s.clientIP(r); got != "" {
		t.Fatalf("missing X-Real-IP: %q", got)
	}
	r.Header.Set("X-Real-IP", "not-an-ip")
	if got := s.clientIP(r); got != "" {
		t.Fatalf("invalid X-Real-IP: %q", got)
	}
	r.Header.Set("X-Real-IP", "2001:db8::7")
	if got := s.clientIP(r); got != "2001:db8::7" {
		t.Fatalf("IPv6 X-Real-IP: %q", got)
	}
}
