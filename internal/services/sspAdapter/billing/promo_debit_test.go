package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPPromoDebitUsesCabinetEndpointAndStableEventID(t *testing.T) {
	var got cabinetPromoSpendRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != cabinetPromoSpendPath {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Bot-Secret") != "secret" {
			t.Fatalf("missing internal secret")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errorMsg":"","data":{"remaining":12.5,"revision":7,"generation":4}}`))
	}))
	defer server.Close()

	debit := NewHTTPPromoDebit(server.URL, "secret")
	state, err := debit(context.Background(), "billing:v1:event", "user", "campaign", 4, 0.75)
	if err != nil {
		t.Fatalf("promo debit: %v", err)
	}
	if got.EventID != "billing:v1:event" || got.UserID != "user" || got.CampaignID != "campaign" || got.PromoGeneration != 4 || got.SpendDelta != 0.75 {
		t.Fatalf("unexpected request payload: %+v", got)
	}
	if state.Remaining != 12.5 || state.Revision != 7 || state.Generation != 4 {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestHTTPPromoDebitFailsOnCabinetError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"success":false,"errorMsg":"event payload conflict"}`))
	}))
	defer server.Close()

	debit := NewHTTPPromoDebit(server.URL, "secret")
	if _, err := debit(context.Background(), "event", "user", "campaign", 4, 1); err == nil {
		t.Fatal("expected cabinet error")
	}
}
