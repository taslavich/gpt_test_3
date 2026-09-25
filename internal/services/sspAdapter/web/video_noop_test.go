package sppAdapterWeb

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVideoImpressionNoopIsStateless204(t *testing.T) {
	recorder := httptest.NewRecorder()
	videoImpressionNoop(recorder)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d", recorder.Code, http.StatusNoContent)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("no-op impression unexpectedly returned a body: %q", recorder.Body.String())
	}
}
