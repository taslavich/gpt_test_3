package percenter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	bolt "go.etcd.io/bbolt"
)

const (
	ObservabilityKindHistory   = "history"
	ObservabilityKindTelemetry = "telemetry"

	ObservabilityReadyKey         = "percenter:observability:ready"
	observabilityRecordPrefix     = "percenter:observability:record:"
	observabilityAckPrefix        = "percenter:observability:acked:"
	observabilityKafkaPrefix      = "percenter:observability:kafka:"
	observabilityClickHousePrefix = "percenter:observability:clickhouse:"
	pendingHistoryReadyKey        = "percenter:history:pending:ready"
	pendingHistoryRecordPrefix    = "percenter:history:pending:record:"
	observabilityAckTTL           = 7 * 24 * time.Hour
	observabilityRecordTTL        = 7 * 24 * time.Hour
)

var observabilityBucket = []byte("percenter_observability")

// ObservabilityRecord is the crash-safe envelope shared by ADV minute telemetry
// and optimizer decision history. ID is stable and is used at every hop for
// retry deduplication.
type ObservabilityRecord struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	CreatedAt     time.Time `json:"created_at"`
	Payload       []byte    `json:"payload"`
	Attempts      uint64    `json:"attempts,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	LastAttemptAt time.Time `json:"last_attempt_at,omitempty"`
}

type ObservabilityOutbox struct {
	db *bolt.DB
}

func OpenObservabilityOutbox(path string) (*ObservabilityOutbox, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("percenter observability outbox path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create percenter outbox directory: %w", err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open percenter observability outbox: %w", err)
	}
	outbox := &ObservabilityOutbox{db: db}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(observabilityBucket)
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize percenter observability outbox: %w", err)
	}
	return outbox, nil
}

func (o *ObservabilityOutbox) Close() error {
	if o == nil || o.db == nil {
		return nil
	}
	return o.db.Close()
}

func (o *ObservabilityOutbox) Put(record ObservabilityRecord) error {
	if o == nil || o.db == nil {
		return errors.New("percenter observability outbox is not initialized")
	}
	record.ID = strings.TrimSpace(record.ID)
	record.Kind = strings.TrimSpace(record.Kind)
	if record.ID == "" || record.Kind == "" || len(record.Payload) == 0 {
		return errors.New("invalid percenter observability record")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	} else {
		record.CreatedAt = record.CreatedAt.UTC()
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return o.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(observabilityBucket)
		if current := bucket.Get([]byte(record.ID)); current != nil {
			var existing ObservabilityRecord
			if err := json.Unmarshal(append([]byte(nil), current...), &existing); err != nil {
				return err
			}
			if existing.Kind != record.Kind || string(existing.Payload) != string(record.Payload) {
				return fmt.Errorf("observability record %s conflicts with existing payload", record.ID)
			}
			return nil
		}
		return bucket.Put([]byte(record.ID), encoded)
	})
}

func (o *ObservabilityOutbox) List() ([]ObservabilityRecord, error) {
	if o == nil || o.db == nil {
		return nil, errors.New("percenter observability outbox is not initialized")
	}
	result := make([]ObservabilityRecord, 0)
	err := o.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(observabilityBucket).ForEach(func(_, raw []byte) error {
			var record ObservabilityRecord
			if err := json.Unmarshal(append([]byte(nil), raw...), &record); err != nil {
				return err
			}
			result = append(result, record)
			return nil
		})
	})
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, err
}

func (o *ObservabilityOutbox) Delete(id string) error {
	if o == nil || o.db == nil {
		return errors.New("percenter observability outbox is not initialized")
	}
	return o.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(observabilityBucket).Delete([]byte(strings.TrimSpace(id)))
	})
}

func (o *ObservabilityOutbox) Failure(id string, cause error) error {
	if o == nil || o.db == nil {
		return errors.New("percenter observability outbox is not initialized")
	}
	return o.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(observabilityBucket)
		raw := bucket.Get([]byte(strings.TrimSpace(id)))
		if raw == nil {
			return nil
		}
		var record ObservabilityRecord
		if err := json.Unmarshal(append([]byte(nil), raw...), &record); err != nil {
			return err
		}
		record.Attempts++
		record.LastAttemptAt = time.Now().UTC()
		if cause != nil {
			record.LastError = cause.Error()
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(record.ID), encoded)
	})
}

func (o *ObservabilityOutbox) Count() (int, error) {
	if o == nil || o.db == nil {
		return 0, errors.New("percenter observability outbox is not initialized")
	}
	count := 0
	err := o.db.View(func(tx *bolt.Tx) error {
		count = tx.Bucket(observabilityBucket).Stats().KeyN
		return nil
	})
	return count, err
}

func stableObservabilityID(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strconv.Itoa(len(part))))
		_, _ = h.Write([]byte{':'})
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{'|'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// RelayObservabilityOutbox persists local records in Redis DB7 without deleting
// them locally. Local deletion only happens after the final sink writes an ACK.
func RelayObservabilityOutbox(ctx context.Context, outbox *ObservabilityOutbox, client *redis.Client) error {
	if outbox == nil || client == nil {
		return errors.New("observability relay is not configured")
	}
	records, err := outbox.List()
	if err != nil {
		return err
	}
	var firstErr error
	for _, record := range records {
		acked, ackErr := client.Exists(ctx, observabilityAckPrefix+record.ID).Result()
		if ackErr != nil {
			_ = outbox.Failure(record.ID, ackErr)
			if firstErr == nil {
				firstErr = ackErr
			}
			continue
		}
		if acked > 0 {
			if err := outbox.Delete(record.ID); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		encoded, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			_ = outbox.Failure(record.ID, marshalErr)
			if firstErr == nil {
				firstErr = marshalErr
			}
			continue
		}
		pipe := client.TxPipeline()
		pipe.SetNX(ctx, observabilityRecordPrefix+record.ID, encoded, observabilityRecordTTL)
		pipe.SAdd(ctx, ObservabilityReadyKey, record.ID)
		if _, err := pipe.Exec(ctx); err != nil {
			_ = outbox.Failure(record.ID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type TelemetryEvent struct {
	EventID          string    `json:"event_id"`
	Bucket           time.Time `json:"bucket"`
	Service          string    `json:"service"`
	Instance         string    `json:"instance"`
	CampaignID       string    `json:"campaign_id"`
	ExactSegmentHash string    `json:"exact_segment_hash"`
	SegmentHash      string    `json:"segment_hash"`
	TypeModel        int       `json:"type_model"`
	PointVersion     uint64    `json:"point_version"`
	Counter          string    `json:"counter"`
	Value            uint64    `json:"value"`
}

type telemetryCounter struct {
	key   telemetryCounterKey
	value atomic.Uint64
}

type telemetryCounterKey struct {
	BucketUnix       int64
	CampaignID       string
	ExactSegmentHash string
	SegmentHash      string
	TypeModel        int
	PointVersion     uint64
	Counter          string
}

// ADVTelemetry is safe for the auction hot path: Record only touches RAM.
// The short RWMutex section makes minute cut-over atomic with respect to Record;
// bbolt/Redis/network work only happens after the lock has been released.
type ADVTelemetry struct {
	service  string
	instance string
	outbox   *ObservabilityOutbox
	mu       sync.RWMutex
	closed   bool
	counters sync.Map // map[telemetryCounterKey]*telemetryCounter
}

func NewADVTelemetry(service, instance string, outbox *ObservabilityOutbox) *ADVTelemetry {
	return &ADVTelemetry{service: strings.TrimSpace(service), instance: strings.TrimSpace(instance), outbox: outbox}
}

func (t *ADVTelemetry) Record(counter, campaignID, exactSegmentHash, segmentHash string, typeModel int, pointVersion uint64) {
	if t == nil || strings.TrimSpace(counter) == "" {
		return
	}
	// Capture wall time only after entering the read-side cut-over lock. If a
	// minute-closing Flush already owns the write lock, this Record resumes after
	// cut-over and is therefore attributed to the new minute rather than
	// recreating an already-persisted old bucket.
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.closed {
		return
	}
	t.recordAtLocked(time.Now().UTC(), counter, campaignID, exactSegmentHash, segmentHash, typeModel, pointVersion)
}

func (t *ADVTelemetry) recordAt(at time.Time, counter, campaignID, exactSegmentHash, segmentHash string, typeModel int, pointVersion uint64) {
	if t == nil || strings.TrimSpace(counter) == "" {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.closed {
		return
	}
	t.recordAtLocked(at, counter, campaignID, exactSegmentHash, segmentHash, typeModel, pointVersion)
}

func (t *ADVTelemetry) recordAtLocked(at time.Time, counter, campaignID, exactSegmentHash, segmentHash string, typeModel int, pointVersion uint64) {
	bucket := at.UTC().Truncate(time.Minute)
	key := telemetryCounterKey{
		BucketUnix: bucket.Unix(), CampaignID: strings.TrimSpace(campaignID), ExactSegmentHash: strings.TrimSpace(exactSegmentHash),
		SegmentHash: strings.TrimSpace(segmentHash), TypeModel: typeModel, PointVersion: pointVersion, Counter: strings.TrimSpace(counter),
	}
	value, _ := t.counters.LoadOrStore(key, &telemetryCounter{key: key})
	value.(*telemetryCounter).value.Add(1)
}

// Flush persists only closed minute buckets. The currently open minute remains
// in RAM so one logical key can never be emitted as multiple delta payloads.
func (t *ADVTelemetry) Flush(now time.Time) error {
	return t.flush(now, false)
}

// FlushAll is for orderly shutdown: it atomically closes Record and persists
// all still-open counters, including the current minute, exactly once.
func (t *ADVTelemetry) FlushAll(now time.Time) error {
	return t.flush(now, true)
}

type telemetryFlushItem struct {
	key   telemetryCounterKey
	value uint64
}

func (t *ADVTelemetry) flush(now time.Time, includeOpen bool) error {
	if t == nil || t.outbox == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.UTC().Truncate(time.Minute).Unix()
	items := make([]telemetryFlushItem, 0)

	// Wait for in-flight Record calls, detach complete buckets, then release the
	// lock before touching bbolt. New auction requests never wait for disk I/O.
	t.mu.Lock()
	if includeOpen {
		t.closed = true
	}
	t.counters.Range(func(rawKey, raw any) bool {
		key := rawKey.(telemetryCounterKey)
		if !includeOpen && key.BucketUnix >= cutoff {
			return true
		}
		counter := raw.(*telemetryCounter)
		value := counter.value.Swap(0)
		if value > 0 {
			items = append(items, telemetryFlushItem{key: key, value: value})
		}
		t.counters.Delete(rawKey)
		return true
	})
	t.mu.Unlock()

	var firstErr error
	for _, item := range items {
		bucket := time.Unix(item.key.BucketUnix, 0).UTC()
		event := TelemetryEvent{
			Bucket: bucket, Service: t.service, Instance: t.instance,
			CampaignID: item.key.CampaignID, ExactSegmentHash: item.key.ExactSegmentHash,
			SegmentHash: item.key.SegmentHash, TypeModel: item.key.TypeModel,
			PointVersion: item.key.PointVersion, Counter: item.key.Counter, Value: item.value,
		}
		event.EventID = stableObservabilityID(
			"telemetry", bucket.Format(time.RFC3339), event.Service, event.Instance, event.CampaignID,
			event.ExactSegmentHash, event.SegmentHash, strconv.Itoa(event.TypeModel), strconv.FormatUint(event.PointVersion, 10), event.Counter,
		)
		payload, err := json.Marshal(event)
		if err == nil {
			err = t.outbox.Put(ObservabilityRecord{ID: event.EventID, Kind: ObservabilityKindTelemetry, CreatedAt: bucket, Payload: payload})
		}
		if err != nil {
			// A failed local persist is retriable by the next periodic flush. On
			// shutdown this also leaves the data in memory until process exit, while
			// the error is surfaced to the caller instead of silently dropping it.
			restored, _ := t.counters.LoadOrStore(item.key, &telemetryCounter{key: item.key})
			restored.(*telemetryCounter).value.Add(item.value)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type HistoryEvent struct {
	EventID                      string    `json:"event_id"`
	Timestamp                    time.Time `json:"timestamp"`
	Bucket                       time.Time `json:"bucket"`
	CampaignID                   string    `json:"campaign_id"`
	ExactSegmentHash             string    `json:"exact_segment_hash"`
	SegmentHash                  string    `json:"segment_hash"`
	TypeModel                    int       `json:"type_model"`
	Phase                        string    `json:"phase"`
	PointVersion                 uint64    `json:"point_version"`
	OriginalBid                  float64   `json:"original_bid"`
	AdvertiserPrice              float64   `json:"advertiser_price"`
	SSPBid                       float64   `json:"ssp_bid"`
	Margin                       float64   `json:"margin"`
	EffectiveMin                 float64   `json:"effective_min"`
	MapSource                    string    `json:"map_source"`
	BaselineBuyout               float64   `json:"baseline_buyout"`
	BaselineWinRate              float64   `json:"baseline_winrate"`
	BaselineEfficiency           float64   `json:"baseline_efficiency"`
	Requests                     uint64    `json:"requests"`
	Impressions                  uint64    `json:"impressions"`
	Wins                         uint64    `json:"wins"`
	Clicks                       uint64    `json:"clicks"`
	AdvertiserSpend              float64   `json:"advertiser_spend"`
	Revenue                      float64   `json:"revenue"`
	Buyout                       float64   `json:"buyout"`
	WinRate                      float64   `json:"winrate"`
	Efficiency                   float64   `json:"efficiency"`
	Profit                       float64   `json:"profit"`
	ProfitPerRelevantOpportunity float64   `json:"profit_per_relevant_opportunity"`
	PreviousSSPBid               float64   `json:"previous_ssp_bid"`
	PreviousMargin               float64   `json:"previous_margin"`
	CandidateSSPBid              float64   `json:"candidate_ssp_bid"`
	CandidateMargin              float64   `json:"candidate_margin"`
	ResultSSPBid                 float64   `json:"result_ssp_bid"`
	ResultMargin                 float64   `json:"result_margin"`
	Decision                     string    `json:"decision"`
	Reason                       string    `json:"reason"`
	Step                         float64   `json:"step"`
	ThresholdPassed              bool      `json:"threshold_passed"`
}

func historyDecisionLabel(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	switch {
	case strings.Contains(reason, "rollback"), strings.Contains(reason, "rejected"):
		return "rollback"
	case strings.Contains(reason, "accepted"), strings.Contains(reason, "baseline"), strings.Contains(reason, "rebenchmark"):
		return "accepted"
	default:
		return "updated"
	}
}

func BuildSimpleHistoryEvent(previous, next SimpleState, metric SimpleMetrics) (HistoryEvent, bool) {
	if len(next.DecisionHistory) == 0 || strings.TrimSpace(previous.SegmentHash) == "" ||
		metric.SegmentHash != previous.SegmentHash || metric.PointVersion == 0 || metric.PointVersion != previous.PointVersion {
		return HistoryEvent{}, false
	}
	decision := next.DecisionHistory[len(next.DecisionHistory)-1]
	exactSegmentHash := strings.TrimSpace(metric.ExactSegmentHash)
	if exactSegmentHash == "" {
		exactSegmentHash = previous.SegmentHash
	}

	// metric.PointVersion belongs to previous: that is the point actually
	// evaluated by this decision. next can already be the following, untested
	// probe, so it must never be labelled as the accepted/rejected candidate.
	previousConfirmedMargin := clampMargin(previous.LastConfirmedMargin, previous.EffectiveMin, previous.MaxMargin)
	if previousConfirmedMargin <= 0 {
		previousConfirmedMargin = previous.Margin
	}
	previousConfirmedSSPBid := priceAfterMargin(previous.ReferenceOriginalBid, previousConfirmedMargin)
	candidateMargin := previous.Margin
	candidateSSPBid := previous.SSPBid
	resultMargin := next.Margin
	resultSSPBid := next.SSPBid
	step := math.Abs(candidateMargin-previousConfirmedMargin) * 100

	event := HistoryEvent{
		Timestamp: decision.At.UTC(), Bucket: decision.At.UTC().Truncate(time.Minute), CampaignID: previous.CampaignID,
		ExactSegmentHash: exactSegmentHash, SegmentHash: previous.SegmentHash, TypeModel: TypeModelSimple, Phase: previous.Phase,
		PointVersion: metric.PointVersion, OriginalBid: previous.ReferenceOriginalBid, AdvertiserPrice: previous.ReferenceOriginalBid,
		SSPBid: previous.SSPBid, Margin: previous.Margin, EffectiveMin: previous.EffectiveMin, MapSource: previous.MapSource,
		BaselineWinRate: previous.BaselineWinRate, Requests: metric.Requests, Impressions: metric.Impressions, Wins: metric.Impressions, Clicks: metric.Clicks,
		AdvertiserSpend: metric.AdvertiserSpend, Revenue: metric.AdvertiserSpend, WinRate: metric.WinRate(), Profit: metric.TwinBidProfit, ProfitPerRelevantOpportunity: metric.ProfitPerRequest(),
		PreviousSSPBid: previousConfirmedSSPBid, PreviousMargin: previousConfirmedMargin,
		CandidateSSPBid: candidateSSPBid, CandidateMargin: candidateMargin,
		ResultSSPBid: resultSSPBid, ResultMargin: resultMargin,
		Decision: historyDecisionLabel(decision.Reason), Reason: decision.Reason, Step: step,
		ThresholdPassed: metric.WinRate()+1e-12 >= previous.BaselineWinRate*0.50,
	}
	event.EventID = stableObservabilityID("history", event.CampaignID, event.SegmentHash, strconv.Itoa(event.TypeModel), strconv.FormatUint(metric.PointVersion, 10), strconv.FormatUint(next.PointVersion, 10), event.Timestamp.Format(time.RFC3339Nano), event.Reason)
	return event, true
}

func complexPreviousConfirmedPoint(state ComplexState) (sspBid, margin float64) {
	switch state.Phase {
	case ComplexPhaseSSPSearch:
		sspBid = state.LastGoodSSPBid
		margin = state.EffectiveMin
	case ComplexPhaseMarginBaseline, ComplexPhaseMarginSearch:
		sspBid = state.LastGoodSSPBid
		margin = state.LastGoodMargin
	default:
		sspBid = state.SSPBid
		margin = state.Margin
	}
	if !finiteComplexValue(sspBid) {
		sspBid = state.SSPBid
	}
	margin = clampMargin(margin, state.EffectiveMin, state.MaxMargin)
	return sspBid, margin
}

func BuildComplexHistoryEvent(previous, next ComplexState, metric ComplexMetrics) (HistoryEvent, bool) {
	if len(next.DecisionHistory) == 0 || strings.TrimSpace(previous.SegmentHash) == "" ||
		metric.SegmentHash != previous.SegmentHash || metric.PointVersion == 0 || metric.PointVersion != previous.PointVersion {
		return HistoryEvent{}, false
	}
	decision := next.DecisionHistory[len(next.DecisionHistory)-1]
	exactSegmentHash := strings.TrimSpace(metric.ExactSegmentHash)
	if exactSegmentHash == "" {
		exactSegmentHash = previous.SegmentHash
	}

	previousConfirmedSSPBid, previousConfirmedMargin := complexPreviousConfirmedPoint(previous)
	candidateSSPBid := previous.SSPBid
	candidateMargin := previous.Margin
	resultSSPBid := next.SSPBid
	resultMargin := next.Margin
	step := math.Abs(candidateMargin-previousConfirmedMargin) * 100
	if math.Abs(candidateSSPBid-previousConfirmedSSPBid) > 1e-12 && previousConfirmedSSPBid > 0 {
		step = math.Abs(candidateSSPBid-previousConfirmedSSPBid) / previousConfirmedSSPBid * 100
	}

	event := HistoryEvent{
		Timestamp: decision.At.UTC(), Bucket: decision.At.UTC().Truncate(time.Minute), CampaignID: previous.CampaignID,
		ExactSegmentHash: exactSegmentHash, SegmentHash: previous.SegmentHash, TypeModel: TypeModelComplex, Phase: previous.Phase,
		PointVersion: metric.PointVersion, OriginalBid: previous.OriginalBid, AdvertiserPrice: previous.AdvertiserPrice,
		SSPBid: previous.SSPBid, Margin: previous.Margin, EffectiveMin: previous.EffectiveMin, MapSource: previous.MapSource,
		BaselineBuyout: previous.BaselineBuyout, BaselineEfficiency: previous.BaselineEfficiency,
		Requests: metric.Requests, Impressions: metric.Impressions, Wins: metric.Impressions, Clicks: metric.Clicks,
		AdvertiserSpend: metric.AdvertiserSpend, Revenue: metric.AdvertiserSpend, Buyout: metric.Buyout(), Efficiency: metric.Efficiency(),
		Profit: metric.TwinBidProfit, ProfitPerRelevantOpportunity: metric.ProfitPerRelevantOpportunity(),
		PreviousSSPBid: previousConfirmedSSPBid, PreviousMargin: previousConfirmedMargin,
		CandidateSSPBid: candidateSSPBid, CandidateMargin: candidateMargin,
		ResultSSPBid: resultSSPBid, ResultMargin: resultMargin,
		Decision: historyDecisionLabel(decision.Reason), Reason: decision.Reason, Step: step,
		ThresholdPassed: decision.BuyoutThresholdPassed || decision.EfficiencyThresholdPassed,
	}
	event.EventID = stableObservabilityID("history", event.CampaignID, event.SegmentHash, strconv.Itoa(event.TypeModel), strconv.FormatUint(metric.PointVersion, 10), strconv.FormatUint(next.PointVersion, 10), event.Timestamp.Format(time.RFC3339Nano), event.Phase, event.Reason)
	return event, true
}

func PutHistoryEvent(outbox *ObservabilityOutbox, event HistoryEvent) error {
	if outbox == nil || strings.TrimSpace(event.EventID) == "" {
		return errors.New("invalid history outbox event")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return outbox.Put(ObservabilityRecord{ID: event.EventID, Kind: ObservabilityKindHistory, CreatedAt: event.Timestamp, Payload: payload})
}

// stagePendingHistoryRedis writes the pending transition into Redis in the same
// MULTI/EXEC as optimizer state. This closes the SaveCAS -> local outbox crash
// window: once state commit succeeds, the history payload is independently
// recoverable by stable event_id.
func stagePendingHistoryRedis(ctx context.Context, pipe redis.Pipeliner, event *HistoryEvent, ttl time.Duration) error {
	if event == nil {
		return nil
	}
	if strings.TrimSpace(event.EventID) == "" {
		return errors.New("pending history event_id is empty")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = observabilityRecordTTL
	}
	pipe.Set(ctx, pendingHistoryRecordPrefix+event.EventID, payload, ttl)
	pipe.SAdd(ctx, pendingHistoryReadyKey, event.EventID)
	return nil
}

// RecoverPendingHistory moves Redis-committed history transitions into the local
// durable bbolt outbox. The pending Redis marker is written in the same MULTI
// transaction as optimizer state, so a successful SaveCAS cannot be followed by
// permanent history loss even if the process dies before touching local disk.
func RecoverPendingHistory(ctx context.Context, client *redis.Client, outbox *ObservabilityOutbox, limit int64) (int, error) {
	if client == nil || outbox == nil {
		return 0, errors.New("pending history recovery is not configured")
	}
	ids, err := client.SMembers(ctx, pendingHistoryReadyKey).Result()
	if err != nil {
		return 0, err
	}
	if limit > 0 && int64(len(ids)) > limit {
		ids = ids[:limit]
	}
	recovered := 0
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		raw, err := client.Get(ctx, pendingHistoryRecordPrefix+id).Bytes()
		if errors.Is(err, redis.Nil) {
			_ = client.SRem(ctx, pendingHistoryReadyKey, id).Err()
			continue
		}
		if err != nil {
			return recovered, err
		}
		var event HistoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return recovered, err
		}
		if strings.TrimSpace(event.EventID) != id {
			return recovered, fmt.Errorf("pending history key %q does not match payload event_id %q", id, event.EventID)
		}
		if err := PutHistoryEvent(outbox, event); err != nil {
			return recovered, err
		}
		if _, err := client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Del(ctx, pendingHistoryRecordPrefix+id)
			pipe.SRem(ctx, pendingHistoryReadyKey, id)
			return nil
		}); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

// PersistSimplePendingHistory makes a committed Simple transition durable in
// bbolt, removes the Redis pending marker, and then best-effort clears the copy
// embedded in state. Replays remain idempotent by event_id.
func PersistSimplePendingHistory(ctx context.Context, store *SimpleStateStore, outbox *ObservabilityOutbox, state SimpleState) error {
	if store == nil || store.redis == nil {
		return errors.New("simple pending history Redis store is not configured")
	}
	if state.PendingHistory == nil {
		return nil
	}
	if err := PutHistoryEvent(outbox, *state.PendingHistory); err != nil {
		return err
	}
	if _, err := store.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, pendingHistoryRecordPrefix+state.PendingHistory.EventID)
		pipe.SRem(ctx, pendingHistoryReadyKey, state.PendingHistory.EventID)
		return nil
	}); err != nil {
		return err
	}
	_, err := store.clearPendingHistory(ctx, state.SegmentHash, state.PointVersion, state.PendingHistory.EventID)
	return err
}

// PersistComplexPendingHistory is the Complex equivalent of
// PersistSimplePendingHistory.
func PersistComplexPendingHistory(ctx context.Context, store *ComplexStateStore, outbox *ObservabilityOutbox, state ComplexState) error {
	if store == nil || store.redis == nil {
		return errors.New("complex pending history Redis store is not configured")
	}
	if state.PendingHistory == nil {
		return nil
	}
	if err := PutHistoryEvent(outbox, *state.PendingHistory); err != nil {
		return err
	}
	if _, err := store.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, pendingHistoryRecordPrefix+state.PendingHistory.EventID)
		pipe.SRem(ctx, pendingHistoryReadyKey, state.PendingHistory.EventID)
		return nil
	}); err != nil {
		return err
	}
	_, err := store.clearPendingHistory(ctx, state.SegmentHash, state.PointVersion, state.PendingHistory.EventID)
	return err
}

func (s *SimpleStateStore) clearPendingHistory(ctx context.Context, segmentHash string, pointVersion uint64, eventID string) (bool, error) {
	if s == nil || s.redis == nil {
		return false, errors.New("simple percenter Redis store is not configured")
	}
	key := SimpleStateKey(segmentHash)
	for attempt := 0; attempt < 4; attempt++ {
		cleared := false
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, key).Bytes()
			if err != nil {
				return err
			}
			var current SimpleState
			if err := json.Unmarshal(raw, &current); err != nil {
				return err
			}
			if current.PointVersion != pointVersion || current.PendingHistory == nil || current.PendingHistory.EventID != eventID {
				return nil
			}
			current.PendingHistory = nil
			if err := saveSimpleStateTx(ctx, tx, key, current, s.policy.Normalize().StateTTL, true); err != nil {
				return err
			}
			cleared = true
			return nil
		}, key)
		if err == nil {
			return cleared, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return false, err
		}
	}
	return false, redis.TxFailedErr
}

func (s *ComplexStateStore) clearPendingHistory(ctx context.Context, segmentHash string, pointVersion uint64, eventID string) (bool, error) {
	if s == nil || s.redis == nil {
		return false, errors.New("complex percenter Redis store is not configured")
	}
	key := ComplexStateKey(segmentHash)
	for attempt := 0; attempt < 4; attempt++ {
		cleared := false
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, key).Bytes()
			if err != nil {
				return err
			}
			var current ComplexState
			if err := json.Unmarshal(raw, &current); err != nil {
				return err
			}
			if current.PointVersion != pointVersion || current.PendingHistory == nil || current.PendingHistory.EventID != eventID {
				return nil
			}
			current.PendingHistory = nil
			if err := saveComplexStateTx(ctx, tx, key, current, s.policy.Normalize().StateTTL, true); err != nil {
				return err
			}
			cleared = true
			return nil
		}, key)
		if err == nil {
			return cleared, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return false, err
		}
	}
	return false, redis.TxFailedErr
}

type ObservabilitySink struct {
	Redis          *redis.Client
	Kafka          *kafka.Writer
	ClickHouse     clickhouse.Conn
	HistoryTable   string
	TelemetryTable string
}

type observabilityStageHooks struct {
	KafkaEnabled      bool
	KafkaDone         bool
	ClickHouseEnabled bool
	ClickHouseDone    bool
	WriteKafka        func() error
	MarkKafka         func() error
	WriteClickHouse   func() error
	MarkClickHouse    func() error
}

// runObservabilityStages is deliberately small and deterministic so outage and
// crash-boundary behavior can be regression tested without external services.
// Transport is at-least-once. Stable event_id/message key provides logical
// identity; each sink must make replays idempotent.
func runObservabilityStages(h observabilityStageHooks) error {
	if h.KafkaEnabled && !h.KafkaDone {
		if h.WriteKafka == nil || h.MarkKafka == nil {
			return errors.New("Kafka observability stage is not configured")
		}
		if err := h.WriteKafka(); err != nil {
			return err
		}
		if err := h.MarkKafka(); err != nil {
			return err
		}
	}
	if h.ClickHouseEnabled && !h.ClickHouseDone {
		if h.WriteClickHouse == nil || h.MarkClickHouse == nil {
			return errors.New("ClickHouse observability stage is not configured")
		}
		if err := h.WriteClickHouse(); err != nil {
			return err
		}
		if err := h.MarkClickHouse(); err != nil {
			return err
		}
	}
	return nil
}

func (s *ObservabilitySink) DrainOnce(ctx context.Context, limit int64) (int, error) {
	if s == nil || s.Redis == nil {
		return 0, errors.New("observability sink Redis is not configured")
	}
	ids, err := s.Redis.SMembers(ctx, ObservabilityReadyKey).Result()
	if err != nil {
		return 0, err
	}
	if limit > 0 && int64(len(ids)) > limit {
		ids = ids[:limit]
	}
	processed := 0
	for _, id := range ids {
		ok, err := s.drainOne(ctx, strings.TrimSpace(id))
		if err != nil {
			return processed, err
		}
		if ok {
			processed++
		}
	}
	return processed, nil
}

func (s *ObservabilitySink) drainOne(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	if acked, err := s.Redis.Exists(ctx, observabilityAckPrefix+id).Result(); err != nil {
		return false, err
	} else if acked > 0 {
		_, _ = s.Redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.SRem(ctx, ObservabilityReadyKey, id)
			pipe.Del(ctx, observabilityRecordPrefix+id)
			return nil
		})
		return true, nil
	}
	raw, err := s.Redis.Get(ctx, observabilityRecordPrefix+id).Bytes()
	if errors.Is(err, redis.Nil) {
		_ = s.Redis.SRem(ctx, ObservabilityReadyKey, id).Err()
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var record ObservabilityRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return false, err
	}

	kafkaDone := true
	if s.Kafka != nil {
		published, err := s.Redis.Exists(ctx, observabilityKafkaPrefix+id).Result()
		if err != nil {
			return false, err
		}
		kafkaDone = published > 0
	}
	clickHouseDone := true
	if s.ClickHouse != nil {
		persisted, err := s.Redis.Exists(ctx, observabilityClickHousePrefix+id).Result()
		if err != nil {
			return false, err
		}
		clickHouseDone = persisted > 0
	}

	err = runObservabilityStages(observabilityStageHooks{
		KafkaEnabled:      s.Kafka != nil,
		KafkaDone:         kafkaDone,
		ClickHouseEnabled: s.ClickHouse != nil,
		ClickHouseDone:    clickHouseDone,
		WriteKafka: func() error {
			// kafka-go cannot atomically commit a Redis delivery marker with the
			// broker write. A crash in that gap can physically replay the message.
			// The stable event ID is therefore also the Kafka key and payload ID.
			return s.Kafka.WriteMessages(ctx, kafka.Message{Key: []byte(id), Value: record.Payload, Time: record.CreatedAt})
		},
		MarkKafka: func() error {
			return s.Redis.Set(ctx, observabilityKafkaPrefix+id, "1", observabilityAckTTL).Err()
		},
		WriteClickHouse: func() error {
			return s.writeClickHouseRecordIdempotent(ctx, record)
		},
		MarkClickHouse: func() error {
			return s.Redis.Set(ctx, observabilityClickHousePrefix+id, "1", observabilityAckTTL).Err()
		},
	})
	if err != nil {
		return false, err
	}

	_, err = s.Redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, observabilityAckPrefix+id, "1", observabilityAckTTL)
		pipe.SRem(ctx, ObservabilityReadyKey, id)
		pipe.Del(ctx, observabilityRecordPrefix+id)
		return nil
	})
	return err == nil, err
}

func (s *ObservabilitySink) writeClickHouseRecordIdempotent(ctx context.Context, record ObservabilityRecord) error {
	if s == nil || s.ClickHouse == nil {
		return nil
	}
	var table string
	switch record.Kind {
	case ObservabilityKindHistory:
		table = s.HistoryTable
	case ObservabilityKindTelemetry:
		table = s.TelemetryTable
	default:
		return fmt.Errorf("unsupported observability kind %q", record.Kind)
	}
	return ensureLogicalObservabilityEvent(
		record.ID,
		func(eventID string) (bool, error) {
			return clickHouseObservabilityEventExists(ctx, s.ClickHouse, table, eventID)
		},
		func() error {
			switch record.Kind {
			case ObservabilityKindHistory:
				var event HistoryEvent
				if err := json.Unmarshal(record.Payload, &event); err != nil {
					return err
				}
				if event.EventID != record.ID {
					return fmt.Errorf("history event_id %q does not match envelope id %q", event.EventID, record.ID)
				}
				return insertHistoryEvent(ctx, s.ClickHouse, table, event)
			case ObservabilityKindTelemetry:
				var event TelemetryEvent
				if err := json.Unmarshal(record.Payload, &event); err != nil {
					return err
				}
				if event.EventID != record.ID {
					return fmt.Errorf("telemetry event_id %q does not match envelope id %q", event.EventID, record.ID)
				}
				return insertTelemetryEvent(ctx, s.ClickHouse, table, event)
			}
			return nil
		},
	)
}

// KafkaObservabilityAtomicApplier is the downstream-consumer storage contract
// for the percenter observability topic. Kafka transport is at-least-once, so a
// physical message can be replayed after crash/rebalance or processed by
// concurrent workers. ApplyObservabilityEventOnce MUST atomically commit both
// the logical mutation and the event_id dedupe marker in the same storage
// transaction (or an equivalent atomic primitive). A crash must therefore make
// either both visible or neither visible; a permanent claim written before the
// logical mutation is not a valid implementation.
type KafkaObservabilityAtomicApplier interface {
	ApplyObservabilityEventOnce(eventID string, payload []byte) (applied bool, err error)
}

// ApplyKafkaObservabilityMessageIdempotent validates the stable Kafka identity
// and delegates logical idempotency to consumer storage through one atomic
// operation. This repository does not contain the final Kafka consumer storage,
// so this helper intentionally does not implement a non-atomic exists->apply
// sequence and does not claim exactly-once physical Kafka delivery.
func ApplyKafkaObservabilityMessageIdempotent(message kafka.Message, applier KafkaObservabilityAtomicApplier) error {
	if applier == nil {
		return errors.New("invalid Kafka observability atomic applier")
	}
	var identity struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(message.Value, &identity); err != nil {
		return fmt.Errorf("decode Kafka observability event identity: %w", err)
	}
	identity.EventID = strings.TrimSpace(identity.EventID)
	if identity.EventID == "" {
		return errors.New("Kafka observability event_id is empty")
	}
	if len(message.Key) > 0 && strings.TrimSpace(string(message.Key)) != identity.EventID {
		return fmt.Errorf("Kafka observability key %q does not match event_id %q", string(message.Key), identity.EventID)
	}
	_, err := applier.ApplyObservabilityEventOnce(identity.EventID, message.Value)
	return err
}

func ensureLogicalObservabilityEvent(eventID string, exists func(string) (bool, error), insert func() error) error {
	if strings.TrimSpace(eventID) == "" || exists == nil || insert == nil {
		return errors.New("invalid logical observability delivery")
	}
	alreadyDelivered, err := exists(eventID)
	if err != nil {
		return err
	}
	if alreadyDelivered {
		return nil
	}
	return insert()
}

func clickHouseObservabilityEventExists(ctx context.Context, conn clickhouse.Conn, table, eventID string) (bool, error) {
	if conn == nil {
		return false, errors.New("ClickHouse observability connection is nil")
	}
	if strings.TrimSpace(table) == "" {
		return false, errors.New("ClickHouse observability table is empty")
	}
	var count uint64
	query := fmt.Sprintf(`SELECT count() FROM %s WHERE event_id = ?`, quoteIdentifier(table))
	if err := conn.QueryRow(ctx, query, strings.TrimSpace(eventID)).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func insertHistoryEvent(ctx context.Context, conn clickhouse.Conn, table string, event HistoryEvent) error {
	if strings.TrimSpace(table) == "" {
		return errors.New("percenter history table is empty")
	}
	query := fmt.Sprintf(`INSERT INTO %s (event_id, timestamp, bucket, campaign_id, exact_segment_hash, segment_hash, type_model, phase, point_version, original_bid, advertiser_price, ssp_bid, margin, effective_min, map_source, baseline_buyout, baseline_winrate, baseline_efficiency, requests, impressions, wins, clicks, advertiser_spend, revenue, buyout, winrate, efficiency, profit, profit_per_relevant_opportunity, previous_ssp_bid, previous_margin, candidate_ssp_bid, candidate_margin, result_ssp_bid, result_margin, decision, reason, step, threshold_passed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, quoteIdentifier(table))
	return conn.Exec(ctx, query, event.EventID, event.Timestamp, event.Bucket, event.CampaignID, event.ExactSegmentHash, event.SegmentHash, event.TypeModel, event.Phase, event.PointVersion, event.OriginalBid, event.AdvertiserPrice, event.SSPBid, event.Margin, event.EffectiveMin, event.MapSource, event.BaselineBuyout, event.BaselineWinRate, event.BaselineEfficiency, event.Requests, event.Impressions, event.Wins, event.Clicks, event.AdvertiserSpend, event.Revenue, event.Buyout, event.WinRate, event.Efficiency, event.Profit, event.ProfitPerRelevantOpportunity, event.PreviousSSPBid, event.PreviousMargin, event.CandidateSSPBid, event.CandidateMargin, event.ResultSSPBid, event.ResultMargin, event.Decision, event.Reason, event.Step, event.ThresholdPassed)
}

func insertTelemetryEvent(ctx context.Context, conn clickhouse.Conn, table string, event TelemetryEvent) error {
	if strings.TrimSpace(table) == "" {
		return errors.New("percenter telemetry table is empty")
	}
	query := fmt.Sprintf(`INSERT INTO %s (event_id, bucket, service, instance, campaign_id, exact_segment_hash, segment_hash, type_model, point_version, counter, value) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, quoteIdentifier(table))
	return conn.Exec(ctx, query, event.EventID, event.Bucket, event.Service, event.Instance, event.CampaignID, event.ExactSegmentHash, event.SegmentHash, event.TypeModel, event.PointVersion, event.Counter, event.Value)
}

func RedisObservabilityBacklog(ctx context.Context, client *redis.Client) int64 {
	if client == nil {
		return 0
	}
	value, err := client.SCard(ctx, ObservabilityReadyKey).Result()
	if err != nil {
		return 0
	}
	return value
}

type DependencyTransitions struct {
	mu     sync.Mutex
	active map[string]bool
}

func NewDependencyTransitions() *DependencyTransitions {
	return &DependencyTransitions{active: make(map[string]bool)}
}

// Update returns "ERROR" only on healthy->failed and "RECOVERED" only on
// failed->healthy. Repeated observations in the same state are silent.
func (d *DependencyTransitions) Update(name string, failed bool) string {
	if d == nil {
		return ""
	}
	name = strings.TrimSpace(name)
	d.mu.Lock()
	defer d.mu.Unlock()
	wasFailed := d.active[name]
	if failed == wasFailed {
		return ""
	}
	d.active[name] = failed
	if failed {
		return "ERROR"
	}
	return "RECOVERED"
}

func (d *DependencyTransitions) Active() []string {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]string, 0)
	for name, failed := range d.active {
		if failed {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}
