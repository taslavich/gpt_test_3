package billing

import (
	"context"
	"testing"
	"time"
)

func TestStoreApplyRejectsInvalidOrUnavailableRuntime(t *testing.T) {
	store := NewStore(nil, time.Hour, 1, time.Millisecond)
	if _, err := store.Apply(context.Background(), Event{EventID: "e1", UserID: "u1", CampaignID: "c1", Format: "IPP", Price: 1}); err == nil {
		t.Fatal("expected nil redis client error")
	}
	store = NewStore(nil, 0, 0, 0)
	if store.markerTTL <= 0 || store.retries <= 0 || store.backoff <= 0 {
		t.Fatalf("defaults not applied: %+v", store)
	}
}
