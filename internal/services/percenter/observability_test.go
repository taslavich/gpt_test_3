package percenter

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

func TestObservabilityOutboxSurvivesRestartAndDeduplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.db")
	outbox, err := OpenObservabilityOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	record := ObservabilityRecord{
		ID:        "stable-event-id",
		Kind:      ObservabilityKindHistory,
		CreatedAt: time.Unix(100, 0).UTC(),
		Payload:   []byte(`{"event":"one"}`),
	}
	if err := outbox.Put(record); err != nil {
		t.Fatal(err)
	}
	// Same id + same payload is an idempotent retry, not a duplicate.
	if err := outbox.Put(record); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenObservabilityOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	records, err := reopened.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("expected one unacked record after restart, got %#v", records)
	}
}

func TestRelayFailureKeepsLocalOutboxRecord(t *testing.T) {
	outbox, err := OpenObservabilityOutbox(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()
	if err := outbox.Put(ObservabilityRecord{
		ID: "event-1", Kind: ObservabilityKindTelemetry, CreatedAt: time.Now().UTC(), Payload: []byte(`{"counter":"x"}`),
	}); err != nil {
		t.Fatal(err)
	}

	client := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  10 * time.Millisecond,
		ReadTimeout:  10 * time.Millisecond,
		WriteTimeout: 10 * time.Millisecond,
		MaxRetries:   -1,
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := RelayObservabilityOutbox(ctx, outbox, client); err == nil {
		t.Fatal("expected Redis relay failure")
	}
	count, err := outbox.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("failed delivery must keep durable local record, count=%d", count)
	}
}

func TestADVTelemetryFlushUsesStableAttributedBucket(t *testing.T) {
	outbox, err := OpenObservabilityOutbox(filepath.Join(t.TempDir(), "adv.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()

	telemetry := NewADVTelemetry("adv", "adv-1", outbox)
	bucket := time.Date(2026, 9, 23, 1, 2, 0, 0, time.UTC)
	telemetry.recordAt(bucket.Add(10*time.Second), "state_init_failed", "campaign-1", "request-exact", "campaign-segment", TypeModelSimple, 7)
	telemetry.recordAt(bucket.Add(20*time.Second), "state_init_failed", "campaign-1", "request-exact", "campaign-segment", TypeModelSimple, 7)
	if err := telemetry.Flush(bucket.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	records, err := outbox.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one aggregated minute record, got %d", len(records))
	}
	var event TelemetryEvent
	if err := json.Unmarshal(records[0].Payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.Value != 2 || event.PointVersion != 7 || event.ExactSegmentHash != "request-exact" || event.SegmentHash != "campaign-segment" {
		t.Fatalf("unexpected telemetry attribution: %#v", event)
	}
	if !event.Bucket.Equal(bucket) {
		t.Fatalf("unexpected bucket: %s", event.Bucket)
	}
}

func TestADVTelemetryOpenMinuteFlushAndShutdownPersistOneCumulativeEvent(t *testing.T) {
	outbox, err := OpenObservabilityOutbox(filepath.Join(t.TempDir(), "adv-minute.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()

	telemetry := NewADVTelemetry("adv", "adv-1", outbox)
	bucket := time.Date(2026, 9, 23, 2, 12, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		telemetry.recordAt(bucket.Add(10*time.Second), "state_init_failed", "campaign-1", "exact", "segment", TypeModelSimple, 9)
	}
	// A regular flush inside the same open minute must not publish a partial
	// delta with the minute's stable EventID.
	if err := telemetry.Flush(bucket.Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if records, err := outbox.List(); err != nil {
		t.Fatal(err)
	} else if len(records) != 0 {
		t.Fatalf("open minute must not be persisted partially, got %d records", len(records))
	}

	for i := 0; i < 30; i++ {
		telemetry.recordAt(bucket.Add(40*time.Second), "state_init_failed", "campaign-1", "exact", "segment", TypeModelSimple, 9)
	}
	if err := telemetry.FlushAll(bucket.Add(50 * time.Second)); err != nil {
		t.Fatal(err)
	}
	// Repeated shutdown flush is idempotent and must not conflict with payload.
	if err := telemetry.FlushAll(bucket.Add(55 * time.Second)); err != nil {
		t.Fatal(err)
	}

	records, err := outbox.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one logical durable event for the minute, got %d", len(records))
	}
	var event TelemetryEvent
	if err := json.Unmarshal(records[0].Payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.Value != 130 {
		t.Fatalf("minute telemetry lost or duplicated events: got %d want 130", event.Value)
	}
	wantID := stableObservabilityID("telemetry", bucket.Format(time.RFC3339), "adv", "adv-1", "campaign-1", "exact", "segment", "1", "9", "state_init_failed")
	if event.EventID != wantID || records[0].ID != wantID {
		t.Fatalf("unexpected stable minute event id: event=%q envelope=%q want=%q", event.EventID, records[0].ID, wantID)
	}
}

func TestHistoryAttributionUsesMetricsPointNotNewState(t *testing.T) {
	at := time.Date(2026, 9, 23, 2, 0, 0, 0, time.UTC)
	previous := SimpleState{
		SegmentHash: "final-segment", CampaignID: "campaign-1", TypeModel: TypeModelSimple,
		ReferenceOriginalBid: 1, EffectiveMin: .2, MapSource: "ALL", Margin: .2, SSPBid: .8,
		BaselineWinRate: .5, PointVersion: 11, Phase: SimplePhaseSearch,
	}
	next := previous
	next.PointVersion = 12
	next.Margin = .25
	next.SSPBid = .75
	next.DecisionHistory = []SimpleDecision{{
		At: at, Reason: "candidate_accepted_probe", OldMargin: .2, NewMargin: .25,
		OldSSPBid: .8, NewSSPBid: .75, OldPointVersion: 11, NewPointVersion: 12,
	}}
	metric := SimpleMetrics{
		ExactSegmentHash: "request-exact", SegmentHash: "final-segment", PointVersion: 11,
		Requests: 100, Impressions: 50, TwinBidProfit: 1,
	}
	event, ok := BuildSimpleHistoryEvent(previous, next, metric)
	if !ok {
		t.Fatal("expected history event")
	}
	if event.PointVersion != 11 || event.ExactSegmentHash != "request-exact" || event.SegmentHash != "final-segment" {
		t.Fatalf("history must be attributed to the point that produced metrics: %#v", event)
	}
	if math.Abs(event.CandidateMargin-.2) > 1e-12 || math.Abs(event.ResultMargin-.25) > 1e-12 {
		t.Fatalf("history must separate evaluated .20 from next untested .25: %#v", event)
	}

	metric.PointVersion = 12
	if _, ok := BuildSimpleHistoryEvent(previous, next, metric); ok {
		t.Fatal("metrics from a new point_version must not be attributed to the old decision")
	}
}

func TestComplexHistoryUsesExactRequestHashAndRealStep(t *testing.T) {
	at := time.Date(2026, 9, 23, 2, 5, 0, 0, time.UTC)
	previous := ComplexState{
		SegmentHash: "final-complex", CampaignID: "campaign-2", TypeModel: TypeModelComplex,
		Phase: ComplexPhaseSSPSearch, OriginalBid: 10, AdvertiserPrice: 1.3125, SSPBid: 1.05,
		Margin: .2, EffectiveMin: .2, MapSource: "campaign", PointVersion: ComplexPointVersionBase + 9,
		BaselineBuyout: .5, LastGoodSSPBid: 1.1666666666666667, LastGoodMargin: .2,
	}
	next := previous
	next.PointVersion++
	next.SSPBid = .945
	next.AdvertiserPrice = .945 / .8
	next.DecisionHistory = []ComplexDecision{{
		At: at, Phase: ComplexPhaseSSPSearch, Reason: "ssp_candidate_accepted_probe",
		OldSSPBid: 1.05, NewSSPBid: .945, OldMargin: .2, NewMargin: .2,
		OldPointVersion: previous.PointVersion, NewPointVersion: next.PointVersion,
		BuyoutThresholdPassed: true,
	}}
	metric := ComplexMetrics{
		ExactSegmentHash: "request-complex", SegmentHash: previous.SegmentHash, PointVersion: previous.PointVersion,
		Requests: 100, Impressions: 40, AdvertiserSpend: 1, TwinBidProfit: .1,
	}
	event, ok := BuildComplexHistoryEvent(previous, next, metric)
	if !ok {
		t.Fatal("expected complex history event")
	}
	if event.ExactSegmentHash != "request-complex" || event.PointVersion != previous.PointVersion {
		t.Fatalf("unexpected complex attribution: %#v", event)
	}
	if event.Step < 9.999999 || event.Step > 10.000001 {
		t.Fatalf("expected 10%% relative SSP step, got %v", event.Step)
	}
}

func TestDependencyTransitionsOneErrorOneRecovered(t *testing.T) {
	transitions := NewDependencyTransitions()
	if got := transitions.Update("redis", true); got != "ERROR" {
		t.Fatalf("first failure: got %q", got)
	}
	if got := transitions.Update("redis", true); got != "" {
		t.Fatalf("continuing failure must be silent, got %q", got)
	}
	if got := transitions.Update("redis", false); got != "RECOVERED" {
		t.Fatalf("recovery: got %q", got)
	}
	if got := transitions.Update("redis", false); got != "" {
		t.Fatalf("continuing healthy state must be silent, got %q", got)
	}
}

func TestObservabilityKafkaOutageThenRecoveryKeepsLogicalDeliveryPending(t *testing.T) {
	kafkaDone := false
	clickHouseDone := false
	kafkaWrites := 0
	clickHouseWrites := 0
	kafkaErr := errors.New("kafka unavailable")

	err := runObservabilityStages(observabilityStageHooks{
		KafkaEnabled: true, KafkaDone: kafkaDone, ClickHouseEnabled: true, ClickHouseDone: clickHouseDone,
		WriteKafka:      func() error { kafkaWrites++; return kafkaErr },
		MarkKafka:       func() error { kafkaDone = true; return nil },
		WriteClickHouse: func() error { clickHouseWrites++; return nil },
		MarkClickHouse:  func() error { clickHouseDone = true; return nil },
	})
	if !errors.Is(err, kafkaErr) || kafkaDone || clickHouseDone || kafkaWrites != 1 || clickHouseWrites != 0 {
		t.Fatalf("Kafka outage must leave event pending and stop only delivery stages: err=%v kafkaDone=%t chDone=%t kafkaWrites=%d chWrites=%d", err, kafkaDone, clickHouseDone, kafkaWrites, clickHouseWrites)
	}

	err = runObservabilityStages(observabilityStageHooks{
		KafkaEnabled: true, KafkaDone: kafkaDone, ClickHouseEnabled: true, ClickHouseDone: clickHouseDone,
		WriteKafka:      func() error { kafkaWrites++; return nil },
		MarkKafka:       func() error { kafkaDone = true; return nil },
		WriteClickHouse: func() error { clickHouseWrites++; return nil },
		MarkClickHouse:  func() error { clickHouseDone = true; return nil },
	})
	if err != nil || !kafkaDone || !clickHouseDone || kafkaWrites != 2 || clickHouseWrites != 1 {
		t.Fatalf("recovery must drain pending stages: err=%v kafkaDone=%t chDone=%t kafkaWrites=%d chWrites=%d", err, kafkaDone, clickHouseDone, kafkaWrites, clickHouseWrites)
	}
}

func TestObservabilityClickHouseOutageThenRecoveryDoesNotReplayCompletedKafka(t *testing.T) {
	kafkaDone := false
	clickHouseDone := false
	kafkaWrites := 0
	clickHouseWrites := 0
	clickHouseErr := errors.New("clickhouse unavailable")

	err := runObservabilityStages(observabilityStageHooks{
		KafkaEnabled: true, KafkaDone: kafkaDone, ClickHouseEnabled: true, ClickHouseDone: clickHouseDone,
		WriteKafka:      func() error { kafkaWrites++; return nil },
		MarkKafka:       func() error { kafkaDone = true; return nil },
		WriteClickHouse: func() error { clickHouseWrites++; return clickHouseErr },
		MarkClickHouse:  func() error { clickHouseDone = true; return nil },
	})
	if !errors.Is(err, clickHouseErr) || !kafkaDone || clickHouseDone || kafkaWrites != 1 || clickHouseWrites != 1 {
		t.Fatalf("ClickHouse outage must leave only CH stage pending: err=%v kafkaDone=%t chDone=%t", err, kafkaDone, clickHouseDone)
	}

	err = runObservabilityStages(observabilityStageHooks{
		KafkaEnabled: true, KafkaDone: kafkaDone, ClickHouseEnabled: true, ClickHouseDone: clickHouseDone,
		WriteKafka:      func() error { kafkaWrites++; return nil },
		MarkKafka:       func() error { kafkaDone = true; return nil },
		WriteClickHouse: func() error { clickHouseWrites++; return nil },
		MarkClickHouse:  func() error { clickHouseDone = true; return nil },
	})
	if err != nil || kafkaWrites != 1 || clickHouseWrites != 2 || !clickHouseDone {
		t.Fatalf("ClickHouse recovery must not replay completed Kafka stage: err=%v kafkaWrites=%d chWrites=%d", err, kafkaWrites, clickHouseWrites)
	}
}

func TestKafkaCrashAfterPublishBeforeMarkerReplaysSameLogicalEventID(t *testing.T) {
	const eventID = "stable-event-42"
	kafkaDone := false
	publishedKeys := make([]string, 0, 2)
	crash := errors.New("crash before kafka marker")

	err := runObservabilityStages(observabilityStageHooks{
		KafkaEnabled: true, KafkaDone: kafkaDone,
		WriteKafka: func() error { publishedKeys = append(publishedKeys, eventID); return nil },
		MarkKafka:  func() error { return crash },
	})
	if !errors.Is(err, crash) || kafkaDone {
		t.Fatalf("expected simulated crash before marker, err=%v kafkaDone=%t", err, kafkaDone)
	}

	err = runObservabilityStages(observabilityStageHooks{
		KafkaEnabled: true, KafkaDone: kafkaDone,
		WriteKafka: func() error { publishedKeys = append(publishedKeys, eventID); return nil },
		MarkKafka:  func() error { kafkaDone = true; return nil },
	})
	if err != nil || !kafkaDone || len(publishedKeys) != 2 || publishedKeys[0] != eventID || publishedKeys[1] != eventID {
		t.Fatalf("Kafka physical replay must preserve one logical event identity: err=%v keys=%v", err, publishedKeys)
	}
}

func TestClickHouseLogicalIdempotencySurvivesCrashBeforeDeliveryMarker(t *testing.T) {
	const eventID = "history-stable-1"
	logicalRows := map[string]bool{}
	physicalInsertCalls := 0
	insert := func() error {
		physicalInsertCalls++
		logicalRows[eventID] = true
		return nil
	}
	exists := func(id string) (bool, error) { return logicalRows[id], nil }

	// First process writes ClickHouse and then crashes before Redis delivery marker.
	if err := ensureLogicalObservabilityEvent(eventID, exists, insert); err != nil {
		t.Fatal(err)
	}
	// Recovery retries the same stable event_id. Existence check must collapse it
	// before a second INSERT, independent of background ReplacingMergeTree merge.
	if err := ensureLogicalObservabilityEvent(eventID, exists, insert); err != nil {
		t.Fatal(err)
	}
	if physicalInsertCalls != 1 || len(logicalRows) != 1 {
		t.Fatalf("ClickHouse retry created duplicate logical row: inserts=%d rows=%v", physicalInsertCalls, logicalRows)
	}
}

func TestCrashAfterClickHouseMarkerBeforeFinalAckSkipsAllCompletedStages(t *testing.T) {
	kafkaWrites := 0
	clickHouseWrites := 0
	// Simulates restart after Kafka+ClickHouse stage markers were persisted but
	// before the final ACK/ready-queue cleanup transaction completed.
	if err := runObservabilityStages(observabilityStageHooks{
		KafkaEnabled: true, KafkaDone: true, ClickHouseEnabled: true, ClickHouseDone: true,
		WriteKafka:      func() error { kafkaWrites++; return nil },
		MarkKafka:       func() error { return nil },
		WriteClickHouse: func() error { clickHouseWrites++; return nil },
		MarkClickHouse:  func() error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if kafkaWrites != 0 || clickHouseWrites != 0 {
		t.Fatalf("completed delivery stages must not replay before final ACK retry: kafka=%d clickhouse=%d", kafkaWrites, clickHouseWrites)
	}
}

func TestSimpleHistoryAcceptedProbeSeparatesEvaluatedAndNextPoint(t *testing.T) {
	at := time.Date(2026, 9, 23, 3, 0, 0, 0, time.UTC)
	previous := SimpleState{
		SegmentHash: "s", CampaignID: "c", TypeModel: TypeModelSimple, Phase: SimplePhaseSearch,
		ReferenceOriginalBid: 1, EffectiveMin: .20, MaxMargin: .90,
		LastConfirmedMargin: .20, Margin: .25, SSPBid: .75, BaselineWinRate: .50, PointVersion: 10,
	}
	next := previous
	next.LastConfirmedMargin = .25
	next.Margin = .30
	next.SSPBid = .70
	next.PointVersion = 11
	next.DecisionHistory = []SimpleDecision{{At: at, Reason: "candidate_accepted_probe"}}
	metric := SimpleMetrics{ExactSegmentHash: "x", SegmentHash: "s", PointVersion: 10, Requests: 100, Impressions: 60, TwinBidProfit: 2}
	event, ok := BuildSimpleHistoryEvent(previous, next, metric)
	if !ok {
		t.Fatal("expected history event")
	}
	if math.Abs(event.PreviousMargin-.20) > 1e-12 || math.Abs(event.CandidateMargin-.25) > 1e-12 || math.Abs(event.ResultMargin-.30) > 1e-12 {
		t.Fatalf("wrong accepted-point semantics: previous=%v candidate=%v result=%v", event.PreviousMargin, event.CandidateMargin, event.ResultMargin)
	}
	if event.Decision != "accepted" || event.PointVersion != 10 {
		t.Fatalf("accepted decision must describe evaluated point_version 10: %#v", event)
	}
}

func TestSimpleHistoryRollbackSeparatesRejectedCandidateAndResult(t *testing.T) {
	at := time.Date(2026, 9, 23, 3, 5, 0, 0, time.UTC)
	previous := SimpleState{
		SegmentHash: "s", CampaignID: "c", TypeModel: TypeModelSimple, Phase: SimplePhaseSearch,
		ReferenceOriginalBid: 1, EffectiveMin: .20, MaxMargin: .90,
		LastConfirmedMargin: .20, Margin: .25, SSPBid: .75, BaselineWinRate: .50, PointVersion: 20,
	}
	next := previous
	next.Margin = .20
	next.SSPBid = .80
	next.PointVersion = 21
	next.DecisionHistory = []SimpleDecision{{At: at, Reason: "winrate_guard_rollback"}}
	metric := SimpleMetrics{ExactSegmentHash: "x", SegmentHash: "s", PointVersion: 20, Requests: 100, Impressions: 20, TwinBidProfit: 1}
	event, ok := BuildSimpleHistoryEvent(previous, next, metric)
	if !ok {
		t.Fatal("expected history event")
	}
	if math.Abs(event.CandidateMargin-.25) > 1e-12 || math.Abs(event.ResultMargin-.20) > 1e-12 || event.Decision != "rollback" {
		t.Fatalf("rollback must reject evaluated .25 and result in .20: %#v", event)
	}
}

func TestComplexHistoryAcceptedProbeDoesNotMarkNextProbeAccepted(t *testing.T) {
	at := time.Date(2026, 9, 23, 3, 10, 0, 0, time.UTC)
	previous := ComplexState{
		SegmentHash: "cs", CampaignID: "cc", TypeModel: TypeModelComplex, Phase: ComplexPhaseSSPSearch,
		OriginalBid: 10, EffectiveMin: .20, MaxMargin: .90,
		LastGoodSSPBid: 1.0, LastGoodMargin: .20,
		SSPBid: .90, Margin: .20, AdvertiserPrice: .90 / .80, PointVersion: ComplexPointVersionBase + 20,
	}
	next := previous
	next.LastGoodSSPBid = .90
	next.SSPBid = .81
	next.AdvertiserPrice = .81 / .80
	next.PointVersion++
	next.DecisionHistory = []ComplexDecision{{At: at, Phase: ComplexPhaseSSPSearch, Reason: "ssp_candidate_accepted_probe", BuyoutThresholdPassed: true}}
	metric := ComplexMetrics{ExactSegmentHash: "cx", SegmentHash: "cs", PointVersion: previous.PointVersion, Requests: 100, Impressions: 50, Clicks: 3, AdvertiserSpend: 1, TwinBidProfit: .1}
	event, ok := BuildComplexHistoryEvent(previous, next, metric)
	if !ok {
		t.Fatal("expected history event")
	}
	if math.Abs(event.PreviousSSPBid-1.0) > 1e-12 || math.Abs(event.CandidateSSPBid-.90) > 1e-12 || math.Abs(event.ResultSSPBid-.81) > 1e-12 {
		t.Fatalf("wrong complex point semantics: %#v", event)
	}
	if event.Clicks != 3 || event.Decision != "accepted" {
		t.Fatalf("unexpected complex history: %#v", event)
	}
}

func TestComplexHistoryRollbackKeepsRejectedCandidateSeparateFromResult(t *testing.T) {
	at := time.Date(2026, 9, 23, 3, 15, 0, 0, time.UTC)
	previous := ComplexState{
		SegmentHash: "cs", CampaignID: "cc", TypeModel: TypeModelComplex, Phase: ComplexPhaseMarginSearch,
		OriginalBid: 1, EffectiveMin: .20, MaxMargin: .90,
		LastGoodSSPBid: .70, LastGoodMargin: .20,
		SSPBid: .70, Margin: .30, AdvertiserPrice: 1.0, PointVersion: ComplexPointVersionBase + 30,
	}
	next := previous
	next.Margin = .20
	next.AdvertiserPrice = .70 / .80
	next.PointVersion++
	next.DecisionHistory = []ComplexDecision{{At: at, Phase: ComplexPhaseMarginSearch, Reason: "margin_profit_regressed_rollback", EfficiencyThresholdPassed: true}}
	metric := ComplexMetrics{ExactSegmentHash: "cx", SegmentHash: "cs", PointVersion: previous.PointVersion, Requests: 100, Impressions: 50, AdvertiserSpend: 1, TwinBidProfit: .01}
	event, ok := BuildComplexHistoryEvent(previous, next, metric)
	if !ok {
		t.Fatal("expected history event")
	}
	if math.Abs(event.PreviousMargin-.20) > 1e-12 || math.Abs(event.CandidateMargin-.30) > 1e-12 || math.Abs(event.ResultMargin-.20) > 1e-12 || event.Decision != "rollback" {
		t.Fatalf("wrong complex rollback semantics: %#v", event)
	}
}

func TestCommittedPendingHistoryRecoversAfterCrashExactlyOnceLogically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-recovery.db")
	at := time.Date(2026, 9, 23, 3, 20, 0, 0, time.UTC)
	event := HistoryEvent{EventID: "history-committed-1", Timestamp: at, Bucket: at.Truncate(time.Minute), SegmentHash: "s", PointVersion: 7}

	// Simulate the state value after a successful SaveCAS and a crash before the
	// local history Put. pending_history is part of the committed Redis JSON.
	committed := SimpleState{SegmentHash: "s", CampaignID: "c", TypeModel: TypeModelSimple, PointVersion: 8, PendingHistory: &event}
	raw, err := json.Marshal(committed)
	if err != nil {
		t.Fatal(err)
	}
	var restarted SimpleState
	if err := json.Unmarshal(raw, &restarted); err != nil {
		t.Fatal(err)
	}
	if restarted.PendingHistory == nil || restarted.PendingHistory.EventID != event.EventID {
		t.Fatalf("pending history did not survive committed-state restart: %#v", restarted.PendingHistory)
	}

	outbox, err := OpenObservabilityOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := PutHistoryEvent(outbox, *restarted.PendingHistory); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Close(); err != nil {
		t.Fatal(err)
	}

	// A second restart can replay the pending marker if clearing it crashed. The
	// same event_id/payload must still produce one logical durable record.
	outbox, err = OpenObservabilityOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()
	if err := PutHistoryEvent(outbox, *restarted.PendingHistory); err != nil {
		t.Fatal(err)
	}
	records, err := outbox.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != event.EventID {
		t.Fatalf("pending-history recovery duplicated/lost logical transition: %#v", records)
	}
}

type atomicKafkaObservabilityTestApplier struct {
	mu         sync.Mutex
	applied    map[string][]byte
	applyCalls int
}

func (a *atomicKafkaObservabilityTestApplier) ApplyObservabilityEventOnce(eventID string, payload []byte) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.applied == nil {
		a.applied = make(map[string][]byte)
	}
	if _, exists := a.applied[eventID]; exists {
		return false, nil
	}

	// This critical section models the required consumer-side storage
	// transaction: logical mutation and dedupe marker become visible together.
	a.applyCalls++
	a.applied[eventID] = append([]byte(nil), payload...)
	return true, nil
}

func TestKafkaDuplicateMessagesConcurrentApplyOneLogicalObservabilityEvent(t *testing.T) {
	event := HistoryEvent{EventID: "kafka-history-1", Timestamp: time.Now().UTC(), SegmentHash: "s", PointVersion: 9}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	messages := []kafka.Message{
		{Key: []byte(event.EventID), Value: payload},
		{Key: []byte(event.EventID), Value: payload},
	}
	applier := &atomicKafkaObservabilityTestApplier{}
	start := make(chan struct{})
	errs := make(chan error, len(messages))
	var wg sync.WaitGroup
	for _, message := range messages {
		message := message
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- ApplyKafkaObservabilityMessageIdempotent(message, applier)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	applier.mu.Lock()
	defer applier.mu.Unlock()
	if applier.applyCalls != 1 || len(applier.applied) != 1 {
		t.Fatalf("concurrent duplicate Kafka messages must yield one logical mutation: calls=%d applied=%v", applier.applyCalls, applier.applied)
	}
	if _, ok := applier.applied[event.EventID]; !ok {
		t.Fatalf("logical event %q was not applied", event.EventID)
	}
}

func TestComplexMetricsIndexCarriesClicks(t *testing.T) {
	index := NewComplexMetricsIndex([]ComplexMetrics{
		{ExactSegmentHash: "x", SegmentHash: "s", PointVersion: 7, Requests: 2, Impressions: 1, Clicks: 3},
		{ExactSegmentHash: "x", SegmentHash: "s", PointVersion: 7, Requests: 4, Impressions: 2, Clicks: 5},
	})
	metric, ok := index.ForState(ComplexState{SegmentHash: "s", PointVersion: 7})
	if !ok || metric.Clicks != 8 {
		t.Fatalf("complex metrics clicks were not loaded/indexed: ok=%t metric=%#v", ok, metric)
	}
}

func TestObservabilityOutboxRejectsCorruptFileWithoutTruncatingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.db")
	original := []byte("this-is-not-a-bbolt-database-and-must-not-be-overwritten")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	outbox, err := OpenObservabilityOutbox(path)
	if err == nil {
		if outbox != nil {
			_ = outbox.Close()
		}
		t.Fatal("corrupt bbolt outbox must fail closed instead of being silently recreated")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("corrupt outbox was modified during failed startup: got=%q want=%q", got, original)
	}
}

func TestObservabilityOutboxRejectsDirectoryAsFilePath(t *testing.T) {
	path := t.TempDir()
	outbox, err := OpenObservabilityOutbox(path)
	if err == nil {
		if outbox != nil {
			_ = outbox.Close()
		}
		t.Fatal("directory path must not be accepted as bbolt outbox file")
	}
}
