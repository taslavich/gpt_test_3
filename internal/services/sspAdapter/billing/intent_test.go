package billing

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type crashTestApplier struct {
	regularApplied map[string]bool
	promoApplied   map[string]bool
	regularCount   int
	promoCount     int
	failPromoOnce  bool
}

func newCrashTestApplier() *crashTestApplier {
	return &crashTestApplier{
		regularApplied: make(map[string]bool),
		promoApplied:   make(map[string]bool),
	}
}

func (a *crashTestApplier) Apply(_ context.Context, record outbox.Record) error {
	if !a.regularApplied[record.EventID] {
		a.regularApplied[record.EventID] = true
		a.regularCount++
	}
	if a.failPromoOnce {
		a.failPromoOnce = false
		return errors.New("injected crash between Redis and PostgreSQL")
	}
	if !a.promoApplied[record.EventID] {
		a.promoApplied[record.EventID] = true
		a.promoCount++
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
		PromoStateCaptured:  true,
		PromoActive:         true,
		PromoGeneration:     4,
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

func assertCrashCounts(t *testing.T, applier *crashTestApplier, regular, promo int) {
	t.Helper()
	if applier.regularCount != regular || applier.promoCount != promo {
		t.Fatalf("side effects regular=%d promo=%d want regular=%d promo=%d", applier.regularCount, applier.promoCount, regular, promo)
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
	assertCrashCounts(t, applier, 1, 1)
	if records, err := store.List(); err != nil || len(records) != 0 {
		t.Fatalf("completed intent remained after recovery: records=%#v err=%v", records, err)
	}
}

func TestBillingCrashAfterRedisBeforePostgresRecoversExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	store, err := outbox.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := billingCrashRecord()
	applier := newCrashTestApplier()
	applier.failPromoOnce = true
	if err := ApplyDurableIntent(context.Background(), store, applier, record); err == nil {
		t.Fatal("expected injected failure")
	}
	assertCrashCounts(t, applier, 1, 0)

	store = reopenOutbox(t, path, store)
	defer store.Close()
	if err := ApplyDurableIntent(context.Background(), store, applier, record); err != nil {
		t.Fatal(err)
	}
	assertCrashCounts(t, applier, 1, 1)
}

func TestBillingCrashAfterPostgresBeforeACKRecoversExactlyOnce(t *testing.T) {
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
	assertCrashCounts(t, applier, 1, 1)

	store = reopenOutbox(t, path, store)
	defer store.Close()
	if err := ApplyDurableIntent(context.Background(), store, applier, record); err != nil {
		t.Fatal(err)
	}
	assertCrashCounts(t, applier, 1, 1)
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
	assertCrashCounts(t, applier, 1, 1)
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
