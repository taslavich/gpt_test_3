package percenter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TypeModelSimple = 1
	TypeModelSmart  = 2

	ProfitModelImpression = "impression"
	ProfitModelClick      = "click"

	PhaseBenchmark          = "benchmark"
	PhaseSSPSearch          = "ssp_search"
	PhaseMarginBaseline     = "margin_baseline"
	PhaseMarginSearch       = "margin_search"
	PhaseSimpleBaseline     = "simple_baseline"
	PhaseSimpleMarginSearch = "simple_margin_search"
)

type Policy struct {
	BuyoutRetention                  float64
	EfficiencyRetention              float64
	SimpleWinRateRetention           float64
	DefaultMinMargin                 float64
	PromoMinMargin                   float64
	MaxMargin                        float64
	SSPSearchPrecision               float64
	MarginSearchStepsPP              []float64
	SSPReoptimizeInterval            time.Duration
	SimpleBaselineReoptimizeInterval time.Duration
	MarginOptimizeInterval           time.Duration
	SegmentStateTTL                  time.Duration
	ADVCacheTTL                      time.Duration
}

func (p Policy) Normalize() Policy {
	if p.BuyoutRetention <= 0 || p.BuyoutRetention > 1 {
		p.BuyoutRetention = 0.80
	}
	if p.EfficiencyRetention <= 0 || p.EfficiencyRetention > 1 {
		p.EfficiencyRetention = 0.80
	}
	if p.SimpleWinRateRetention <= 0 || p.SimpleWinRateRetention > 1 {
		p.SimpleWinRateRetention = 0.50
	}
	if p.DefaultMinMargin <= 0 || p.DefaultMinMargin >= 1 {
		p.DefaultMinMargin = 0.20
	}
	if p.PromoMinMargin <= 0 || p.PromoMinMargin >= 1 {
		p.PromoMinMargin = 0.30
	}
	if p.MaxMargin <= 0 || p.MaxMargin >= 1 {
		p.MaxMargin = 0.90
	}
	if p.MaxMargin < p.PromoMinMargin {
		p.MaxMargin = p.PromoMinMargin
	}
	if p.SSPSearchPrecision <= 0 || p.SSPSearchPrecision >= 1 {
		p.SSPSearchPrecision = 0.01
	}
	if len(p.MarginSearchStepsPP) == 0 {
		p.MarginSearchStepsPP = []float64{10, 5, 2, 1}
	}
	clean := make([]float64, 0, len(p.MarginSearchStepsPP))
	for _, step := range p.MarginSearchStepsPP {
		if step > 0 && !math.IsNaN(step) && !math.IsInf(step, 0) {
			clean = append(clean, step)
		}
	}
	if len(clean) == 0 {
		clean = []float64{10, 5, 2, 1}
	}
	p.MarginSearchStepsPP = clean
	if p.SSPReoptimizeInterval <= 0 {
		p.SSPReoptimizeInterval = 6 * time.Hour
	}
	if p.SimpleBaselineReoptimizeInterval <= 0 {
		p.SimpleBaselineReoptimizeInterval = 6 * time.Hour
	}
	if p.MarginOptimizeInterval <= 0 {
		p.MarginOptimizeInterval = 5 * time.Minute
	}
	if p.SegmentStateTTL <= 0 {
		p.SegmentStateTTL = 7 * 24 * time.Hour
	}
	if p.ADVCacheTTL <= 0 {
		p.ADVCacheTTL = 5 * time.Second
	}
	return p
}

func (p Policy) MinMargin(promoRemaining float64) float64 {
	p = p.Normalize()
	if promoRemaining > 0 {
		return p.PromoMinMargin
	}
	return p.DefaultMinMargin
}

type State struct {
	SegmentHash         string  `json:"segment_hash"`
	FallbackSegmentHash string  `json:"fallback_segment_hash,omitempty"`
	CampaignID          string  `json:"campaign_id"`
	CampaignVersion     int64   `json:"campaign_version"`
	PointVersion        uint64  `json:"point_version"`
	TypeModel           int     `json:"type_model"`
	ProfitModel         string  `json:"profit_model"`
	OriginalBid         float64 `json:"original_bid"`
	MinMargin           float64 `json:"min_margin"`

	AdvertiserPrice float64 `json:"advertiser_price"`
	SSPBid          float64 `json:"ssp_bid"`
	Margin          float64 `json:"margin"`
	Phase           string  `json:"phase"`

	BenchmarkBuyout     float64 `json:"benchmark_buyout"`
	BaselineEfficiency  float64 `json:"baseline_efficiency"`
	BestProfitPerReq    float64 `json:"best_profit_per_request"`
	BestMargin          float64 `json:"best_margin"`
	BestAdvertiserPrice float64 `json:"best_advertiser_price"`

	SSPLow  float64 `json:"ssp_low"`
	SSPHigh float64 `json:"ssp_high"`

	MarginStepIndex int `json:"margin_step_index"`
	MarginDirection int `json:"margin_direction"`

	LastChangeAt         time.Time `json:"last_change_at"`
	LastSSPReoptimizeAt  time.Time `json:"last_ssp_reoptimize_at"`
	LastSimpleBaselineAt time.Time `json:"last_simple_baseline_at"`
	UpdatedAt            time.Time `json:"updated_at"`

	// PendingHistory is retained only for rolling-upgrade compatibility with
	// proj135 records. New writes keep history in per-segment outbox keys and
	// always persist this field empty, so business-state size stays bounded.
	PendingHistory []HistoryEvent `json:"pending_history,omitempty"`
}

func BaselineState(segmentHash, campaignID string, originalBid, minMargin float64, campaignRevision int64, now time.Time) State {
	return BaselineStateForCampaign(segmentHash, campaignID, originalBid, minMargin, campaignRevision, TypeModelSmart, ProfitModelImpression, now)
}

func BaselineStateForCampaign(segmentHash, campaignID string, originalBid, minMargin float64, campaignRevision int64, typeModel int, profitModel string, now time.Time) State {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if minMargin < 0 {
		minMargin = 0
	}
	if minMargin >= 1 {
		minMargin = 0.99
	}
	ssp := originalBid * (1 - minMargin)
	typeModel = normalizeTypeModel(typeModel)
	profitModel = normalizeProfitModel(profitModel)
	phase := PhaseBenchmark
	if typeModel == TypeModelSimple {
		phase = PhaseSimpleBaseline
	}
	return State{
		SegmentHash:     segmentHash,
		CampaignID:      campaignID,
		CampaignVersion: campaignRevision,
		PointVersion:    1,
		TypeModel:       typeModel,
		ProfitModel:     profitModel,
		OriginalBid:     originalBid,
		MinMargin:       minMargin,
		AdvertiserPrice: originalBid,
		SSPBid:          ssp,
		Margin:          minMargin,
		Phase:           phase,
		SSPLow:          0,
		SSPHigh:         ssp,
		MarginDirection: 1,
		LastChangeAt:    now,
		// Zero means that the current SSP optimization cycle has not completed yet.
		// The 6h cadence starts only after the binary search converges, otherwise a
		// sparse segment could be reset on every rare request and never finish.
		LastSSPReoptimizeAt: time.Time{},
		UpdatedAt:           now,
	}
}

func (s State) ValidFor(originalBid, minMargin float64, campaignRevision int64) bool {
	return s.ValidForCampaign(originalBid, minMargin, campaignRevision, TypeModelSmart, ProfitModelImpression)
}

func (s State) ValidForCampaign(originalBid, minMargin float64, campaignRevision int64, typeModel int, profitModel string) bool {
	typeModel = normalizeTypeModel(typeModel)
	profitModel = normalizeProfitModel(profitModel)
	if !finitePositive(s.OriginalBid) || !finitePositive(s.AdvertiserPrice) || !finitePositive(s.SSPBid) ||
		!approximatelyEqual(s.OriginalBid, originalBid) || !approximatelyEqual(s.MinMargin, minMargin) ||
		s.CampaignVersion != campaignRevision || normalizeTypeModel(s.TypeModel) != typeModel || normalizeProfitModel(s.ProfitModel) != profitModel {
		return false
	}
	if typeModel == TypeModelSimple && !approximatelyEqual(s.AdvertiserPrice, originalBid) {
		return false
	}
	return true
}

func (s State) sspReoptimizationDue(now time.Time, interval time.Duration) bool {
	if interval <= 0 || s.LastSSPReoptimizeAt.IsZero() {
		return false
	}
	return !now.Before(s.LastSSPReoptimizeAt.Add(interval))
}

func (s State) simpleBaselineReoptimizationDue(now time.Time, interval time.Duration) bool {
	if interval <= 0 || s.LastSimpleBaselineAt.IsZero() {
		return false
	}
	return !now.Before(s.LastSimpleBaselineAt.Add(interval))
}

func (s State) reoptimizationDue(now time.Time, policy Policy) bool {
	policy = policy.Normalize()
	if normalizeTypeModel(s.TypeModel) == TypeModelSimple {
		return s.simpleBaselineReoptimizationDue(now, policy.SimpleBaselineReoptimizeInterval)
	}
	return s.sspReoptimizationDue(now, policy.SSPReoptimizeInterval)
}

type Pricing struct {
	SegmentHash     string
	AdvertiserPrice float64
	SSPBid          float64
	Margin          float64
	Phase           string
	PointVersion    uint64
	FromFallback    bool
}

type cachedState struct {
	state    State
	loadedAt time.Time
}

type StateStore struct {
	redis          *redis.Client
	policy         Policy
	mu             sync.RWMutex
	cache          map[string]cachedState
	lastCacheSweep time.Time

	historyDirtyKey   string
	historyNotifyCh   chan string
	historyNotifyOnce sync.Once
}

func NewStateStore(client *redis.Client, policy Policy) *StateStore {
	return &StateStore{
		redis:           client,
		policy:          policy.Normalize(),
		cache:           make(map[string]cachedState),
		historyNotifyCh: make(chan string, 4096),
	}
}

// ConfigureHistoryQueue keeps the shared ready queue out of segment CAS. The
// segment transaction writes only the business-state key plus its per-segment
// history outbox/pending marker. This method derives the best-effort dirty-set
// key used by the asynchronous delivery worker.
func (s *StateStore) ConfigureHistoryQueue(readyKey string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.historyDirtyKey = HistoryDirtyKey(readyKey)
	s.mu.Unlock()
}

func (s *StateStore) historyDirtySetKey() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	key := s.historyDirtyKey
	s.mu.RUnlock()
	return key
}

func SegmentKey(hash string) string { return "percenter:segment:" + hash }

func (s *StateStore) GetOrInitPricing(ctx context.Context, segmentHash, campaignID string, originalBid, minMargin float64, campaignRevision int64, now time.Time) (Pricing, error) {
	return s.GetOrInitPricingForCampaign(ctx, segmentHash, campaignID, originalBid, minMargin, campaignRevision, TypeModelSmart, ProfitModelImpression, now)
}

func (s *StateStore) GetOrInitPricingForCampaign(ctx context.Context, segmentHash, campaignID string, originalBid, minMargin float64, campaignRevision int64, typeModel int, profitModel string, now time.Time) (Pricing, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	typeModel = normalizeTypeModel(typeModel)
	profitModel = normalizeProfitModel(profitModel)

	state, fallback, err := s.getOrInitStateForCampaign(ctx, segmentHash, campaignID, originalBid, minMargin, campaignRevision, typeModel, profitModel, now)
	if err != nil {
		return fallback, err
	}

	targetHash := strings.TrimSpace(state.FallbackSegmentHash)
	if targetHash == "" || targetHash == state.SegmentHash {
		return pricingFromState(state), nil
	}

	targetState, _, targetErr := s.getOrInitStateForCampaign(ctx, targetHash, campaignID, originalBid, minMargin, campaignRevision, typeModel, profitModel, now)
	if targetErr != nil {
		// Keep serving the exact segment's last known state if the selected parent
		// cannot be read/initialized. This avoids falling all the way back to a new
		// baseline because of a transient Redis error.
		pricing := pricingFromState(state)
		pricing.FromFallback = true
		return pricing, targetErr
	}
	pricing := pricingFromState(targetState)
	pricing.FromFallback = true
	return pricing, nil
}

func (s *StateStore) getOrInitStateForCampaign(ctx context.Context, segmentHash, campaignID string, originalBid, minMargin float64, campaignRevision int64, typeModel int, profitModel string, now time.Time) (State, Pricing, error) {
	baseline := BaselineStateForCampaign(segmentHash, campaignID, originalBid, minMargin, campaignRevision, typeModel, profitModel, now)
	fallback := pricingFromState(baseline)
	fallback.FromFallback = true
	if s == nil || s.redis == nil {
		return State{}, fallback, errors.New("percenter redis is unavailable")
	}

	if state, ok := s.cachedForCampaign(segmentHash, originalBid, minMargin, campaignRevision, typeModel, profitModel, now); ok {
		return state, pricingFromState(state), nil
	}

	// Initialization and campaign reinitialization are persisted together with
	// their history event. This is intentionally retried on a CAS race so two
	// concurrent ADV requests cannot create an untracked state transition.
	for attempt := 0; attempt < 4; attempt++ {
		state, err := s.Load(ctx, segmentHash)
		if err != nil && !errors.Is(err, redis.Nil) {
			return State{}, fallback, err
		}
		if errors.Is(err, redis.Nil) {
			initialized := baseline
			event := InitializedHistoryEvent(initialized, now)
			saved, err := s.saveIfAbsentWithHistory(ctx, initialized, event)
			if err != nil {
				return State{}, fallback, err
			}
			if saved {
				return initialized, pricingFromState(initialized), nil
			}
			continue
		}

		if state.ValidForCampaign(originalBid, minMargin, campaignRevision, typeModel, profitModel) {
			s.putCache(state, now)
			return state, pricingFromState(state), nil
		}

		reinitialized := baseline
		if state.PointVersion > 0 {
			reinitialized.PointVersion = nextPointVersion(state.PointVersion)
		}
		event := StateUpdateHistoryEvent(state, reinitialized, Metrics{}, "reinitialized", now)
		saved, err := s.SaveIfCurrentWithHistory(ctx, state, reinitialized, event)
		if err != nil {
			return State{}, fallback, err
		}
		if saved {
			return reinitialized, pricingFromState(reinitialized), nil
		}
	}
	return State{}, fallback, fmt.Errorf("percenter state %s changed concurrently during initialization", segmentHash)
}

func (s *StateStore) cached(hash string, originalBid, minMargin float64, revision int64, now time.Time) (State, bool) {
	return s.cachedForCampaign(hash, originalBid, minMargin, revision, TypeModelSmart, ProfitModelImpression, now)
}

func (s *StateStore) cachedForCampaign(hash string, originalBid, minMargin float64, revision int64, typeModel int, profitModel string, now time.Time) (State, bool) {
	if s == nil || s.policy.ADVCacheTTL <= 0 {
		return State{}, false
	}
	s.mu.RLock()
	item, ok := s.cache[hash]
	s.mu.RUnlock()
	if !ok || now.Sub(item.loadedAt) > s.policy.ADVCacheTTL || !item.state.ValidForCampaign(originalBid, minMargin, revision, typeModel, profitModel) {
		return State{}, false
	}
	return item.state, true
}

func (s *StateStore) putCache(state State, now time.Time) {
	if s == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	// Pending history is transport state, not pricing state. Do not retain it in
	// the hot ADV cache; CAS helpers explicitly merge pending events from Redis.
	state.PendingHistory = nil
	s.mu.Lock()
	s.cache[state.SegmentHash] = cachedState{state: state, loadedAt: now}
	if s.lastCacheSweep.IsZero() || now.Sub(s.lastCacheSweep) >= time.Minute {
		cutoff := now.Add(-s.policy.ADVCacheTTL)
		for hash, item := range s.cache {
			if item.loadedAt.Before(cutoff) {
				delete(s.cache, hash)
			}
		}
		s.lastCacheSweep = now
	}
	s.mu.Unlock()
}

func (s *StateStore) Load(ctx context.Context, segmentHash string) (State, error) {
	if s == nil || s.redis == nil {
		return State{}, errors.New("percenter redis is unavailable")
	}
	raw, err := s.redis.Get(ctx, SegmentKey(segmentHash)).Bytes()
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, fmt.Errorf("decode percenter state %s: %w", segmentHash, err)
	}
	return state, nil
}

func (s *StateStore) Save(ctx context.Context, state State) error {
	if s == nil || s.redis == nil {
		return errors.New("percenter redis is unavailable")
	}
	if len(state.PendingHistory) != 0 {
		return fmt.Errorf("refusing non-transactional save of percenter state %s with legacy pending history", state.SegmentHash)
	}
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode percenter state %s: %w", state.SegmentHash, err)
	}
	if err := s.redis.Set(ctx, SegmentKey(state.SegmentHash), raw, s.policy.SegmentStateTTL).Err(); err != nil {
		return fmt.Errorf("save percenter state %s: %w", state.SegmentHash, err)
	}
	s.putCache(state, state.UpdatedAt)
	return nil
}

func validateHistoryEventForState(state State, event HistoryEvent) error {
	if strings.TrimSpace(event.StateSegmentHash) != strings.TrimSpace(state.SegmentHash) {
		return fmt.Errorf("history state hash %q does not match state %q", event.StateSegmentHash, state.SegmentHash)
	}
	return ValidateHistoryEvent(event)
}

func stateWithoutPendingHistory(state State) State {
	state.PendingHistory = nil
	return state
}

func statesEqualIgnoringPendingHistory(left, right State) bool {
	leftRaw, leftErr := json.Marshal(stateWithoutPendingHistory(left))
	rightRaw, rightErr := json.Marshal(stateWithoutPendingHistory(right))
	return leftErr == nil && rightErr == nil && bytes.Equal(leftRaw, rightRaw)
}

func (s *StateStore) notifyHistoryPending(segmentHash string) {
	if s == nil || strings.TrimSpace(segmentHash) == "" || s.historyNotifyCh == nil || s.historyDirtySetKey() == "" {
		return
	}
	select {
	case s.historyNotifyCh <- segmentHash:
	default:
		// This channel is only an optimization hint. The event is already durable
		// in the per-segment outbox with a pending marker; recovery will find it.
	}
}

func (s *StateStore) saveIfAbsentWithHistory(ctx context.Context, state State, event HistoryEvent) (bool, error) {
	if s == nil || s.redis == nil {
		return false, errors.New("percenter redis is unavailable")
	}
	if err := validateHistoryEventForState(state, event); err != nil {
		return false, err
	}
	state.UpdatedAt = time.Now().UTC()
	state.PendingHistory = nil
	stateRaw, err := json.Marshal(state)
	if err != nil {
		return false, fmt.Errorf("encode initial percenter state %s: %w", state.SegmentHash, err)
	}
	eventRaw, err := MarshalHistoryEvent(event)
	if err != nil {
		return false, fmt.Errorf("encode initialized percenter history %s: %w", state.SegmentHash, err)
	}
	ttl := s.policy.SegmentStateTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	stateKey := SegmentKey(state.SegmentHash)
	outboxKey := HistoryOutboxKey(state.SegmentHash)
	pendingKey := HistoryPendingKey(state.SegmentHash)

	// All watched keys are per-segment. Different segments never invalidate one
	// another, while SET state + RPUSH outbox + SET pending marker are committed
	// in one EXEC without Lua and without the shared ready queue in the CAS.
	for attempt := 0; attempt < historyStorageMaxRetries; attempt++ {
		saved := false
		err = s.redis.Watch(ctx, func(tx *redis.Tx) error {
			stateType, err := validatePerSegmentHistoryKeyTypes(ctx, tx, stateKey, outboxKey, pendingKey)
			if err != nil {
				return err
			}
			if stateType != "none" {
				return nil
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, stateKey, stateRaw, ttl)
				pipe.RPush(ctx, outboxKey, string(eventRaw))
				pipe.Set(ctx, pendingKey, "1", 0)
				return nil
			})
			if err == nil {
				saved = true
			}
			return err
		}, stateKey, outboxKey, pendingKey)
		if err == nil {
			if saved {
				s.putCache(state, state.UpdatedAt)
				s.notifyHistoryPending(state.SegmentHash)
			}
			return saved, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return false, fmt.Errorf("initialize percenter state %s with history outbox: %w", state.SegmentHash, err)
		}
	}
	return false, fmt.Errorf("initialize percenter state %s with history outbox: Redis transaction conflicted after %d retries", state.SegmentHash, historyStorageMaxRetries)
}

func (s *StateStore) SaveIfCurrent(ctx context.Context, previous, next State) (bool, error) {
	if s == nil || s.redis == nil {
		return false, errors.New("percenter redis is unavailable")
	}
	if strings.TrimSpace(previous.SegmentHash) == "" || previous.SegmentHash != next.SegmentHash {
		return false, fmt.Errorf("percenter state hash mismatch: previous=%q next=%q", previous.SegmentHash, next.SegmentHash)
	}
	next.UpdatedAt = time.Now().UTC()
	next.PendingHistory = nil
	ttl := s.policy.SegmentStateTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	stateKey := SegmentKey(previous.SegmentHash)
	outboxKey := HistoryOutboxKey(previous.SegmentHash)
	pendingKey := HistoryPendingKey(previous.SegmentHash)

	for attempt := 0; attempt < historyStorageMaxRetries; attempt++ {
		saved := false
		movedLegacy := false
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			stateType, err := validatePerSegmentHistoryKeyTypes(ctx, tx, stateKey, outboxKey, pendingKey)
			if err != nil {
				return err
			}
			if stateType == "none" {
				return nil
			}
			currentRaw, err := tx.Get(ctx, stateKey).Bytes()
			if errors.Is(err, redis.Nil) {
				return nil
			}
			if err != nil {
				return err
			}
			var current State
			if err := json.Unmarshal(currentRaw, &current); err != nil {
				return fmt.Errorf("decode current percenter state %s: %w", previous.SegmentHash, err)
			}
			if !statesEqualIgnoringPendingHistory(current, previous) {
				return nil
			}
			nextRaw, err := json.Marshal(next)
			if err != nil {
				return fmt.Errorf("encode next percenter state %s: %w", next.SegmentHash, err)
			}
			legacyValues, err := historyEventRawValues(current.PendingHistory)
			if err != nil {
				return err
			}
			movedLegacy = len(legacyValues) > 0
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, stateKey, nextRaw, ttl)
				if len(legacyValues) > 0 {
					pipe.RPush(ctx, outboxKey, legacyValues...)
					pipe.Set(ctx, pendingKey, "1", 0)
				}
				return nil
			})
			if err == nil {
				saved = true
			}
			return err
		}, stateKey, outboxKey, pendingKey)
		if err == nil {
			if saved {
				s.putCache(next, next.UpdatedAt)
				if movedLegacy {
					s.notifyHistoryPending(next.SegmentHash)
				}
			}
			return saved, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return false, fmt.Errorf("compare-and-swap percenter state %s: %w", previous.SegmentHash, err)
		}
	}
	return false, nil
}

// SaveIfCurrentWithHistory commits one business-state transition and its audit
// event atomically using only per-segment Redis keys. History is not embedded in
// the business-state JSON: it is appended to a separate durable outbox LIST and
// a per-segment pending marker is created in the same EXEC. The shared ready
// queue is touched only by the asynchronous worker after the CAS has completed.
func (s *StateStore) SaveIfCurrentWithHistory(ctx context.Context, previous, next State, event HistoryEvent) (bool, error) {
	if s == nil || s.redis == nil {
		return false, errors.New("percenter redis is unavailable")
	}
	if strings.TrimSpace(previous.SegmentHash) == "" || previous.SegmentHash != next.SegmentHash {
		return false, fmt.Errorf("percenter state hash mismatch: previous=%q next=%q", previous.SegmentHash, next.SegmentHash)
	}
	if err := validateHistoryEventForState(next, event); err != nil {
		return false, err
	}
	next.UpdatedAt = time.Now().UTC()
	next.PendingHistory = nil
	ttl := s.policy.SegmentStateTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	stateKey := SegmentKey(previous.SegmentHash)
	outboxKey := HistoryOutboxKey(previous.SegmentHash)
	pendingKey := HistoryPendingKey(previous.SegmentHash)
	eventRaw, err := MarshalHistoryEvent(event)
	if err != nil {
		return false, fmt.Errorf("encode percenter history event %s: %w", event.EventID, err)
	}

	for attempt := 0; attempt < historyStorageMaxRetries; attempt++ {
		saved := false
		err = s.redis.Watch(ctx, func(tx *redis.Tx) error {
			stateType, err := validatePerSegmentHistoryKeyTypes(ctx, tx, stateKey, outboxKey, pendingKey)
			if err != nil {
				return err
			}
			if stateType == "none" {
				return nil
			}
			currentRaw, err := tx.Get(ctx, stateKey).Bytes()
			if errors.Is(err, redis.Nil) {
				return nil
			}
			if err != nil {
				return err
			}
			var current State
			if err := json.Unmarshal(currentRaw, &current); err != nil {
				return fmt.Errorf("decode current percenter state %s: %w", previous.SegmentHash, err)
			}
			if !statesEqualIgnoringPendingHistory(current, previous) {
				return nil
			}
			nextRaw, err := json.Marshal(next)
			if err != nil {
				return fmt.Errorf("encode next percenter state %s: %w", next.SegmentHash, err)
			}
			values, err := historyEventRawValues(current.PendingHistory)
			if err != nil {
				return err
			}
			values = append(values, string(eventRaw))
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, stateKey, nextRaw, ttl)
				pipe.RPush(ctx, outboxKey, values...)
				pipe.Set(ctx, pendingKey, "1", 0)
				return nil
			})
			if err == nil {
				saved = true
			}
			return err
		}, stateKey, outboxKey, pendingKey)
		if err == nil {
			if saved {
				s.putCache(next, next.UpdatedAt)
				s.notifyHistoryPending(next.SegmentHash)
			}
			return saved, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return false, fmt.Errorf("compare-and-swap percenter state %s with history outbox: %w", previous.SegmentHash, err)
		}
	}
	return false, nil
}

func pricingFromState(state State) Pricing {
	return Pricing{SegmentHash: state.SegmentHash, AdvertiserPrice: state.AdvertiserPrice, SSPBid: state.SSPBid, Margin: state.Margin, Phase: state.Phase, PointVersion: state.PointVersion}
}

func CampaignVersionKey(campaignID string) string {
	return "percenter:campaign:" + strings.TrimSpace(campaignID) + ":version"
}

func campaignFingerprint(typeModel int, originalBid, minMargin float64, pricingContext string) string {
	return fmt.Sprintf("%d|%.12g|%.12g|%s", normalizeTypeModel(typeModel), originalBid, minMargin, strings.TrimSpace(pricingContext))
}

func (s *StateStore) EnsureCampaignVersion(ctx context.Context, campaignID string, typeModel int, originalBid, minMargin float64) (int64, error) {
	return s.EnsureCampaignVersionForContext(ctx, campaignID, typeModel, originalBid, minMargin, "")
}

type CampaignVersionRequest struct {
	CampaignID     string
	TypeModel      int
	OriginalBid    float64
	MinMargin      float64
	PricingContext string
}

// EnsureCampaignVersionsForContext batches the read-only fast path for snapshot
// refresh into one Redis pipeline. Only campaigns whose fingerprint/version is
// absent or stale enter the WATCH/MULTI slow path, and those rare updates are
// synchronized with a bounded worker pool. Auction requests never call this.
func (s *StateStore) EnsureCampaignVersionsForContext(ctx context.Context, requests []CampaignVersionRequest) (map[string]int64, error) {
	if s == nil || s.redis == nil {
		return nil, errors.New("percenter redis is unavailable")
	}
	if len(requests) == 0 {
		return map[string]int64{}, nil
	}

	type preparedRequest struct {
		request     CampaignVersionRequest
		fingerprint string
		key         string
	}
	prepared := make(map[string]preparedRequest, len(requests))
	order := make([]string, 0, len(requests))
	for _, request := range requests {
		request.CampaignID = strings.TrimSpace(request.CampaignID)
		if request.CampaignID == "" {
			return nil, errors.New("campaign id is empty")
		}
		fingerprint := campaignFingerprint(request.TypeModel, request.OriginalBid, request.MinMargin, request.PricingContext)
		if old, exists := prepared[request.CampaignID]; exists {
			if old.fingerprint != fingerprint {
				return nil, fmt.Errorf("campaign %s has conflicting percenter fingerprints in one snapshot", request.CampaignID)
			}
			continue
		}
		prepared[request.CampaignID] = preparedRequest{
			request:     request,
			fingerprint: fingerprint,
			key:         CampaignVersionKey(request.CampaignID),
		}
		order = append(order, request.CampaignID)
	}

	commands := make(map[string]*redis.SliceCmd, len(prepared))
	const readBatchSize = 1024
	for start := 0; start < len(order); start += readBatchSize {
		end := start + readBatchSize
		if end > len(order) {
			end = len(order)
		}
		batch := order[start:end]
		if _, err := s.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, campaignID := range batch {
				item := prepared[campaignID]
				commands[campaignID] = pipe.HMGet(ctx, item.key, "fingerprint", "version")
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("pipeline percenter campaign versions batch_start=%d: %w", start, err)
		}
	}

	versions := make(map[string]int64, len(prepared))
	unresolved := make([]CampaignVersionRequest, 0)
	for _, campaignID := range order {
		item := prepared[campaignID]
		fields, err := commands[campaignID].Result()
		if err != nil {
			return nil, fmt.Errorf("read percenter campaign version %s: %w", campaignID, err)
		}
		matched := false
		if len(fields) == 2 {
			currentFingerprint, _ := fields[0].(string)
			rawVersion, _ := fields[1].(string)
			if currentFingerprint == item.fingerprint && strings.TrimSpace(rawVersion) != "" {
				version, parseErr := strconv.ParseInt(rawVersion, 10, 64)
				if parseErr == nil && version >= 1 {
					versions[campaignID] = version
					matched = true
				}
			}
		}
		if !matched {
			unresolved = append(unresolved, item.request)
		}
	}
	if len(unresolved) == 0 {
		return versions, nil
	}

	workers := 32
	if len(unresolved) < workers {
		workers = len(unresolved)
	}
	jobs := make(chan CampaignVersionRequest)
	var wg sync.WaitGroup
	var resultMu sync.Mutex
	var firstErr error
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for request := range jobs {
				version, err := s.EnsureCampaignVersionForContext(
					ctx, request.CampaignID, request.TypeModel, request.OriginalBid, request.MinMargin, request.PricingContext,
				)
				resultMu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
				} else {
					versions[request.CampaignID] = version
				}
				resultMu.Unlock()
			}
		}()
	}
	for _, request := range unresolved {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		case jobs <- request:
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return versions, nil
}

func (s *StateStore) EnsureCampaignVersionForContext(ctx context.Context, campaignID string, typeModel int, originalBid, minMargin float64, pricingContext string) (int64, error) {
	if s == nil || s.redis == nil {
		return 0, errors.New("percenter redis is unavailable")
	}
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return 0, errors.New("campaign id is empty")
	}
	fingerprint := campaignFingerprint(typeModel, originalBid, minMargin, pricingContext)
	key := CampaignVersionKey(campaignID)

	// Fast path: the overwhelmingly common snapshot refresh case performs one
	// read and no WATCH/MULTI when nothing changed.
	fields, err := s.redis.HMGet(ctx, key, "fingerprint", "version").Result()
	if err != nil {
		return 0, fmt.Errorf("read percenter campaign version %s: %w", campaignID, err)
	}
	if len(fields) == 2 {
		currentFingerprint, _ := fields[0].(string)
		rawVersion, _ := fields[1].(string)
		if currentFingerprint == fingerprint && strings.TrimSpace(rawVersion) != "" {
			version, parseErr := strconv.ParseInt(rawVersion, 10, 64)
			if parseErr == nil && version >= 1 {
				return version, nil
			}
		}
	}

	const maxRetries = 8
	for attempt := 0; attempt < maxRetries; attempt++ {
		var version int64
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			fields, err := tx.HGetAll(ctx, key).Result()
			if err != nil {
				return err
			}
			currentFingerprint, hasFingerprint := fields["fingerprint"]
			currentVersion := int64(0)
			if rawVersion := strings.TrimSpace(fields["version"]); rawVersion != "" {
				currentVersion, err = strconv.ParseInt(rawVersion, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid percenter campaign version %q: %w", rawVersion, err)
				}
			}
			if hasFingerprint && currentFingerprint == fingerprint && currentVersion >= 1 {
				version = currentVersion
				return nil
			}
			if !hasFingerprint {
				version = 1
			} else if currentFingerprint != fingerprint {
				version = currentVersion + 1
				if version < 1 {
					version = 1
				}
			} else {
				version = 1
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, key, "fingerprint", fingerprint, "version", version)
				return nil
			})
			return err
		}, key)
		if err == nil {
			return version, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return 0, fmt.Errorf("ensure percenter campaign version %s: %w", campaignID, err)
		}
	}
	return 0, fmt.Errorf("ensure percenter campaign version %s: Redis transaction conflicted after %d retries", campaignID, maxRetries)
}

func normalizeTypeModel(typeModel int) int {
	if typeModel == TypeModelSimple {
		return TypeModelSimple
	}
	return TypeModelSmart
}

func normalizeProfitModel(model string) string {
	if strings.EqualFold(strings.TrimSpace(model), ProfitModelClick) {
		return ProfitModelClick
	}
	return ProfitModelImpression
}

func nextPointVersion(current uint64) uint64 {
	if current == ^uint64(0) {
		return 1
	}
	return current + 1
}

func finitePositive(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

func approximatelyEqual(a, b float64) bool {
	scale := math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
	return math.Abs(a-b) <= 1e-9*scale
}
