package sppAdapterWeb

import (
	"context"
	"errors"
	"net/http"
	"os"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubdiagnostic"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func markPublisherRTBFailure(w http.ResponseWriter, r *http.Request, format, reason string) {
	if format != "POP" || r.Header.Get(pubuuid.Header) == "" ||
		!pubuuid.FeedAllowed(os.Getenv("PUB_UUID_FEEDS"), r.URL.Query().Get("feed")) {
		return
	}
	if _, ok := pubdiagnostic.SSPDescription(reason); ok {
		w.Header().Set(pubdiagnostic.Header, reason)
	}
}

func publisherOrchestratorFailureReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "ssp_orchestrator_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "ssp_orchestrator_canceled"
	}
	switch status.Code(err) {
	case codes.DeadlineExceeded:
		return "ssp_orchestrator_timeout"
	case codes.Unavailable:
		return "ssp_orchestrator_unavailable"
	case codes.Canceled:
		return "ssp_orchestrator_canceled"
	case codes.InvalidArgument, codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition, codes.ResourceExhausted:
		return "ssp_orchestrator_rejected"
	default:
		return "ssp_orchestrator_error"
	}
}
