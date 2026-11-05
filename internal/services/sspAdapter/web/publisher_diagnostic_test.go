package sppAdapterWeb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubdiagnostic"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPublisherDiagnosticIsRestrictedToPublisherFeed(t *testing.T) {
	t.Setenv("PUB_UUID_FEEDS", "test-pub")
	for _, tc := range []struct {
		feed, format, reason, want string
		header                     bool
	}{
		{"test-pub", "POP", "ssp_ip_limited", "ssp_ip_limited", true},
		{"normal", "POP", "ssp_ip_limited", "", true},
		{"test-pub", "BAN", "ssp_ip_limited", "", true},
		{"test-pub", "POP", "arbitrary-body", "", true},
		{"test-pub", "POP", "ssp_ip_limited", "", false},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/bid_v_2_5_mc?feed="+tc.feed, nil)
		if tc.header {
			r.Header.Set(pubuuid.Header, "sample")
		}
		markPublisherRTBFailure(w, r, tc.format, tc.reason)
		if got := w.Header().Get(pubdiagnostic.Header); got != tc.want {
			t.Errorf("got %q, want %q for %#v", got, tc.want, tc)
		}
	}
}

func TestPublisherOrchestratorDiagnosticDistinguishesRPCFailures(t *testing.T) {
	for _, tc := range []struct {
		code codes.Code
		want string
	}{
		{codes.DeadlineExceeded, "ssp_orchestrator_timeout"},
		{codes.Unavailable, "ssp_orchestrator_unavailable"},
		{codes.InvalidArgument, "ssp_orchestrator_rejected"},
		{codes.Canceled, "ssp_orchestrator_canceled"},
		{codes.Internal, "ssp_orchestrator_error"},
	} {
		if got := publisherOrchestratorFailureReason(status.Error(tc.code, "private details")); got != tc.want {
			t.Errorf("%v: %s", tc.code, got)
		}
	}
	if got := publisherOrchestratorFailureReason(context.DeadlineExceeded); got != "ssp_orchestrator_timeout" {
		t.Fatal(got)
	}
}
