package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPPromoSyncFanout(t *testing.T) {
	type requestObservation struct {
		method string
		path   string
		input  promoSyncRequest
		err    error
	}
	observed := make(chan requestObservation, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input promoSyncRequest
		err := json.NewDecoder(r.Body).Decode(&input)
		observed <- requestObservation{method: r.Method, path: r.URL.Path, input: input, err: err}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	syncPromo := NewHTTPPromoSync([]string{server.URL})
	if err := syncPromo(context.Background(), "user-1", 0, 11); err != nil {
		t.Fatal(err)
	}
	got := <-observed
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.method != http.MethodPut || got.path != promoSyncPath {
		t.Fatalf("request=%s %s", got.method, got.path)
	}
	if got.input.UserID != "user-1" || got.input.Remaining != 0 || got.input.Revision != 11 {
		t.Fatalf("payload=%+v", got.input)
	}
}

func TestHTTPPromoSyncRequiresEveryADVInstance(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer okServer.Close()
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer badServer.Close()

	syncPromo := NewHTTPPromoSync([]string{okServer.URL, badServer.URL})
	if err := syncPromo(context.Background(), "user-1", 1, 12); err == nil {
		t.Fatal("expected fanout failure when one ADV instance rejects the update")
	}
}
