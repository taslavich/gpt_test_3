package billing

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

func newReplayRecord(id string) outbox.Record {
	needs := true
	return outbox.Record{
		Kind: outbox.KindBilling, RequiresADVRecovery: &needs,
		EventID: id, UserID: "user", CampaignID: "campaign", Price: 0.25,
		Format: "POP", Source: "burl", CreatedAt: time.Now().Add(-time.Hour), Attempts: 1,
	}
}

func openReplayTestOutbox(t *testing.T) *outbox.Store {
	t.Helper()
	store, err := outbox.Open(filepath.Join(t.TempDir(), "replay.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestADVOutboxDrainsSuccessfulEventsDespiteFailedNeighbor(t *testing.T) {
	store := openReplayTestOutbox(t)
	if err := store.Save(newReplayRecord("good")); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(newReplayRecord("broken")); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(nil, store, time.Nanosecond, time.Nanosecond, []string{"adv"})
	blocked := true
	enabled := 0
	worker.processOverride = func(_ context.Context, record outbox.Record) (bool, error) {
		if record.EventID == "broken" && blocked {
			return true, errors.New("redis timeout")
		}
		return false, nil
	}
	worker.statusOverride = func(_ context.Context, _ []string, on bool) map[string]int {
		if on {
			enabled++
		}
		return map[string]int{"adv": http.StatusOK}
	}
	worker.run(context.Background())
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].EventID != "broken" || enabled != 0 {
		t.Fatalf("expected independent ACK without enabling ADV: records=%+v enabled=%d", records, enabled)
	}
	flag, err := store.ADVRecoveryPending()
	if err != nil || !flag {
		t.Fatalf("lost durable recovery flag: %t %v", flag, err)
	}
	blocked = false
	worker.run(context.Background())
	records, err = store.List()
	if err != nil || len(records) != 0 || enabled != 1 {
		t.Fatalf("ADV was not restored after final event: records=%+v enabled=%d err=%v", records, enabled, err)
	}
	flag, err = store.ADVRecoveryPending()
	if err != nil || flag {
		t.Fatalf("recovery latch not cleared: %t %v", flag, err)
	}
}

func TestADVRecoveryLatchSurvivesEmptyOutboxAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.db")
	store, err := outbox.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(newReplayRecord("good")); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(nil, store, time.Nanosecond, time.Nanosecond, []string{"adv"})
	worker.processOverride = func(context.Context, outbox.Record) (bool, error) { return false, nil }
	worker.statusOverride = func(context.Context, []string, bool) map[string]int {
		return map[string]int{"adv": http.StatusServiceUnavailable}
	}
	worker.run(context.Background())
	remaining, err := store.List()
	if err != nil || len(remaining) != 0 {
		t.Fatalf("success was not ACKed: %v %v", remaining, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = outbox.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.ADVRecoveryPending()
	if err != nil || !pending {
		t.Fatalf("lost recovery latch after restart: %t %v", pending, err)
	}
	next := NewWorker(nil, store, time.Nanosecond, time.Nanosecond, []string{"adv"})
	enabled := 0
	next.statusOverride = func(context.Context, []string, bool) map[string]int {
		enabled++
		return map[string]int{"adv": http.StatusOK}
	}
	next.run(context.Background())
	pending, err = store.ADVRecoveryPending()
	if err != nil || pending || enabled != 1 {
		t.Fatalf("restart failed to re-enable ADV: pending=%t enabled=%d err=%v", pending, enabled, err)
	}
}

func TestADVOutboxMissingWinnerIsRetainedAndRateLimited(t *testing.T) {
	store := openReplayTestOutbox(t)
	if err := store.Save(newReplayRecord("orphan")); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(nil, store, time.Nanosecond, time.Nanosecond, []string{"adv"})
	attempts := 0
	worker.processOverride = func(context.Context, outbox.Record) (bool, error) {
		attempts++
		return false, ErrWinnerUnavailable
	}
	worker.statusOverride = func(context.Context, []string, bool) map[string]int {
		t.Fatal("must never enable ADV with unresolved financial event")
		return nil
	}
	worker.run(context.Background())
	worker.run(context.Background())
	rows, err := store.List()
	if err != nil || len(rows) != 1 || rows[0].ErrorClass != "winner_missing" || attempts != 1 {
		t.Fatalf("winner missing was not preserved and backoff applied: %+v attempts=%d err=%v", rows, attempts, err)
	}
}
