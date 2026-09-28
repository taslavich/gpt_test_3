package billing

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type crashTestApplier struct {
	applied map[string]bool
	count   int
}

func newCrashTestApplier() *crashTestApplier {
	return &crashTestApplier{applied: make(map[string]bool)}
}

func (a *crashTestApplier) Apply(_ context.Context, record outbox.Record) error {
	if !a.applied[record.EventID] {
		a.applied[record.EventID] = true
		a.count++
	}
	return nil
}

func billingCrashRecord() outbox.Record {
	requiresRecovery := true
	return outbox.Record{
		Kind:                outbox.KindBilling,
		RequiresADVRecovery: &requiresRecovery,
		EventID:             CallbackEventID("burl", "POP", "winner-1"),
		UserID:              "user-1",
		CampaignID:          "campaign-1",
		TypeModel:           2,
		Price:               0.25,
		Format:              "POP",
		Source:              "burl",
		Attempts:            1,
	}
}

func reopenOutbox(t *testing.T, path string, store *outbox.Store) *outbox.Store {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := outbox.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return reopened
}

func assertApplyCount(t *testing.T, applier *crashTestApplier, want int) {
	t.Helper()
	if applier.count != want {
		t.Fatalf("side effects=%d want %d", applier.count, want)
	}
}

func TestBillingCrashAfterDurableIntentBeforeRedisRecoversExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	store, err := outbox.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := billingCrashRecord()
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	store = reopenOutbox(t, path, store)
	defer store.Close()

	applier := newCrashTestApplier()
	if err := ApplyDurableIntent(context.Background(), store, applier, record); err != nil {
		t.Fatal(err)
	}
	assertApplyCount(t, applier, 1)
	if records, err := store.List(); err != nil || len(records) != 0 {
		t.Fatalf("completed intent remained after recovery: records=%#v err=%v", records, err)
	}
}

func TestBillingCrashAfterRedisBeforeACKRecoversExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	store, err := outbox.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := billingCrashRecord()
	applier := newCrashTestApplier()
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	if err := applier.Apply(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	assertApplyCount(t, applier, 1)

	store = reopenOutbox(t, path, store)
	defer store.Close()
	if err := ApplyDurableIntent(context.Background(), store, applier, record); err != nil {
		t.Fatal(err)
	}
	assertApplyCount(t, applier, 1)
}

func TestCompletedBillingIntentCanBeReplayedWithoutDoubleSpend(t *testing.T) {
	store, err := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record := billingCrashRecord()
	applier := newCrashTestApplier()

	for i := 0; i < 2; i++ {
		if err := ApplyDurableIntent(context.Background(), store, applier, record); err != nil {
			t.Fatal(err)
		}
	}
	assertApplyCount(t, applier, 1)
}

func TestCallbackEventIDIsStableAndSeparatesLogicalEvents(t *testing.T) {
	first := CallbackEventID("burl", "POP", "winner-1")
	second := CallbackEventID(" BURL ", "pop", " winner-1 ")
	if first == "" || first != second {
		t.Fatalf("stable callback event IDs differ: %q %q", first, second)
	}
	if first == CallbackEventID("adm", "IPP", "winner-1") {
		t.Fatal("different callback kinds must not share an idempotency key")
	}
	if first == CallbackEventID("burl", "POP", "winner-2") {
		t.Fatal("different upstream winner IDs must not share an idempotency key")
	}
}

type contextAwareApplier struct {
	applied int
}

func (a *contextAwareApplier) Apply(ctx context.Context, _ outbox.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.applied++
	return nil
}

func TestApplyDurableCallbackIntentSurvivesCanceledRequestContext(t *testing.T) {
	store, err := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	applier := &contextAwareApplier{}
	record := billingCrashRecord()

	if err := ApplyDurableCallbackIntent(requestCtx, store, applier, record); err != nil {
		t.Fatalf("callback billing inherited request cancellation: %v", err)
	}
	if applier.applied != 1 {
		t.Fatalf("applied=%d want 1", applier.applied)
	}
	if records, err := store.List(); err != nil || len(records) != 0 {
		t.Fatalf("completed callback intent remained in outbox: records=%#v err=%v", records, err)
	}
}

func TestApplyDurableIntentStillHonorsCallerCancellation(t *testing.T) {
	store, err := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	applier := &contextAwareApplier{}
	record := billingCrashRecord()

	if err := ApplyDurableIntent(ctx, store, applier, record); !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplyDurableIntent error=%v want context.Canceled", err)
	}
	if applier.applied != 0 {
		t.Fatalf("applied=%d want 0", applier.applied)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].EventID != record.EventID {
		t.Fatalf("durable intent was not retained after cancellation: %#v", records)
	}
}
