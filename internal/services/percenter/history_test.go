package percenter

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryOutboxPersistsAndDeduplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := OpenHistoryOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	event := HistoryEvent{EventID: "e1", EventTime: time.Now().UTC(), EventType: "state_updated", StateSegmentHash: "s1"}
	if err := store.Save(event); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(event); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenHistoryOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events, err := store.List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "e1" {
		t.Fatalf("events=%+v", events)
	}
	if err := store.Delete("e1"); err != nil {
		t.Fatal(err)
	}
	events, err = store.List(10)
	if err != nil || len(events) != 0 {
		t.Fatalf("after delete events=%+v err=%v", events, err)
	}
}

func TestStateUpdateHistoryDoesNotMutatePricing(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	before := BaselineStateForCampaign("hash", "42", 1, 0.2, 1, TypeModelSmart, ProfitModelImpression, now.Add(-time.Minute))
	before.PointVersion = 7
	before.SSPBid = .5
	before.AdvertiserPrice = .8
	before.Margin = .375
	after := before
	after.PointVersion = 8
	after.AdvertiserPrice = .85
	after.Margin = marginFor(after.AdvertiserPrice, after.SSPBid)
	metrics := Metrics{Requests: 20, Wins: 6, Clicks: 2, TwinBidProfit: .4}
	event := StateUpdateHistoryEvent(before, after, metrics, "state_updated", now)
	if event.EventID == "" || event.OldPointVersion != 7 || event.NewPointVersion != 8 {
		t.Fatalf("bad event: %+v", event)
	}
	if before.PointVersion != 7 || before.AdvertiserPrice != .8 || before.SSPBid != .5 {
		t.Fatalf("builder mutated state: %+v", before)
	}
}

func TestInitializedHistoryEventCapturesFirstState(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	state := BaselineStateForCampaign("hash-init", "77", 1.25, 0.2, 3, TypeModelSimple, ProfitModelImpression, now)
	event := InitializedHistoryEvent(state, now)
	if event.EventType != "initialized" || event.EventID == "" {
		t.Fatalf("bad initialized event: %+v", event)
	}
	if event.StateSegmentHash != state.SegmentHash || event.EffectiveSegmentHash != state.SegmentHash {
		t.Fatalf("wrong hashes in initialized event: %+v", event)
	}
	if event.OldPointVersion != 0 || event.NewPointVersion != state.PointVersion {
		t.Fatalf("wrong point versions in initialized event: %+v", event)
	}
	if event.NewAdvertiserPrice != state.AdvertiserPrice || event.NewSSPBid != state.SSPBid || event.NewMargin != state.Margin {
		t.Fatalf("wrong initial pricing in event: %+v", event)
	}
}

func TestPendingHistoryDoesNotChangeBusinessStateComparison(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	base := BaselineStateForCampaign("seg", "campaign", 1, 0.2, 3, TypeModelSmart, ProfitModelImpression, now)
	withPending := base
	withPending.PendingHistory = []HistoryEvent{InitializedHistoryEvent(base, now)}
	if !statesEqualIgnoringPendingHistory(base, withPending) {
		t.Fatal("pending history must not participate in business-state CAS comparison")
	}
	withPending.SSPBid -= 0.01
	if statesEqualIgnoringPendingHistory(base, withPending) {
		t.Fatal("pricing changes must participate in business-state CAS comparison")
	}
}

func TestHistoryTransportKeysStayOutsideBusinessState(t *testing.T) {
	segmentHash := "segment-123"
	if HistoryOutboxKey(segmentHash) == SegmentKey(segmentHash) {
		t.Fatal("history outbox must not reuse the business-state key")
	}
	if HistoryPendingKey(segmentHash) == SegmentKey(segmentHash) {
		t.Fatal("history pending marker must not reuse the business-state key")
	}
	if got := historySegmentHashFromPendingKey(HistoryPendingKey(segmentHash)); got != segmentHash {
		t.Fatalf("pending marker round-trip=%q want=%q", got, segmentHash)
	}
}
