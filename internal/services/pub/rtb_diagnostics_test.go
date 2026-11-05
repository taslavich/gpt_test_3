package pub

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubdiagnostic"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
)

func TestPublisherRTBDiagnosticReasonAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		status                     int
		header, body, reason, text string
	}{
		{403, "ssp_site_blocked", "", "ssp_site_blocked", "Site ID blocked"},
		{503, "ssp_ortb_write_failed", "private-token", "ssp_ortb_write_failed", "auction stats in Redis"},
		{504, "ssp_orchestrator_timeout", "", "ssp_orchestrator_timeout", "Orchestrator timed out"},
		{403, "", "Ip is limited: 192.0.2.42", "ssp_ip_limited", "Visitor IP rate-limited"},
		{403, "", "Ipv6 is limited: 2001:db8::2", "ssp_ip_limited", "Visitor IP rate-limited"},
		{403, "", "Busy", "ssp_feed_unknown", "Feed missing from SSP config"},
		{403, "arbitrary-header", "Ip is limited: private-token", "upstream_unclassified", "Request refused"},
		{429, "", "<html>private-token</html>", "upstream_unclassified", "Request rate limited"},
		{502, "", "", "upstream_unclassified", "Gateway got no valid response"},
	} {
		f := classifyRTBHTTPFailure(tc.status, tc.header, tc.body)
		line := (rtbAlertGroup{reason: f.reason, status: f.status, count: 1}).line()
		if f.reason != tc.reason || !strings.Contains(line, tc.text) || strings.Contains(line, "private-token") || strings.Contains(line, "192.0.2.42") || strings.Contains(line, "2001:db8::2") {
			t.Errorf("bad diagnostic for %#v: %s", tc, line)
		}
	}
	for _, tc := range []struct {
		err    error
		reason string
	}{
		{context.Canceled, "transport_canceled"},
		{context.DeadlineExceeded, "transport_timeout"},
		{&net.DNSError{Err: "lookup failed"}, "transport_dns"},
		{&url.Error{Op: "Post", URL: "https://rtb.test/?feed=private-token", Err: syscall.ECONNREFUSED}, "transport_refused"},
		{syscall.ECONNRESET, "transport_reset"},
		{errors.New("private-token"), "transport_failed"},
	} {
		if got := classifyRTBTransportFailure(tc.err); got.reason != tc.reason || strings.Contains(got.Error(), "private-token") {
			t.Errorf("bad transport diagnostic: %#v", got)
		}
	}
}

func TestPublisherForwardsTypedSSPFailureOnlyToOperators(t *testing.T) {
	s, code := fixture(t)
	var message, ortbID string
	s.Alerts = NewSystemAlerts("test-pub", func(_ context.Context, text string) error {
		message = text
		return nil
	})
	s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		ortbID = r.Header.Get(pubuuid.Header)
		headers := make(http.Header)
		headers.Set(pubdiagnostic.Header, "ssp_site_blocked")
		return &http.Response{StatusCode: 403, Header: headers, Body: io.NopCloser(strings.NewReader("private-upstream-content"))}, nil
	})
	in := input(code)
	w := send(s, in)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "rtb_failed") || strings.Contains(w.Body.String(), "ssp_site_blocked") {
		t.Fatal("publisher response changed:", w.Code, w.Body.String())
	}
	if !s.Alerts.flush(context.Background()) || ortbID == "" ||
		!strings.Contains(message, "RTB HTTP 403: Site ID blocked ×1") ||
		strings.Contains(message, in.ID) || strings.Contains(message, ortbID) ||
		strings.Contains(message, "private-upstream-content") || strings.Contains(message, "?feed=") {
		t.Fatalf("bad operator alert: %q", message)
	}
}
