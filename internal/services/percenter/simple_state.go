package percenter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TypeModelSimple = 1

	SimplePhaseBaseline = "simple_baseline"
	SimplePhaseSearch   = "simple_search"
	SimplePhaseSettled  = "simple_settled"

	SimpleStateKeyPrefix = "percenter:simple:segment:"
	SimpleStateIndexKey  = "percenter:simple:segments"
)

type SimplePolicy struct {
	WinRateRetention    float64
	MinImpressions      uint64
	OptimizeInterval    time.Duration
	RebenchmarkInterval time.Duration
	SearchStepsPP       []float64
	MaxMargin           float64
	StateTTL            time.Duration
	PendingHistoryTTL   time.Duration
}

func (p SimplePolicy) Normalize() SimplePolicy {
	if p.WinRateRetention <= 0 || p.WinRateRetention > 1 {
		p.WinRateRetention = 0.50
	}
	if p.MinImpressions == 0 {
		p.MinImpressions = 5
	}
	if p.OptimizeInterval <= 0 {
		p.OptimizeInterval = 5 * time.Minute
	}
	if p.RebenchmarkInterval <= 0 {
		p.RebenchmarkInterval = 6 * time.Hour
	}
	if len(p.SearchStepsPP) == 0 {
		p.SearchStepsPP = []float64{5, 2, 1}
	}
	clean := make([]float64, 0, len(p.SearchStepsPP))
	for _, step := range p.SearchStepsPP {
		if step > 0 && !math.IsNaN(step) && !math.IsInf(step, 0) {
			clean = append(clean, step)
		}
	}
	if len(clean) == 0 {
		clean = []float64{5, 2, 1}
	}
	p.SearchStepsPP = clean
	if p.MaxMargin <= 0 || p.MaxMargin >= 1 {
		p.MaxMargin = 0.90
	}
	if p.StateTTL <= 0 {
		p.StateTTL = 7 * 24 * time.Hour
	}
	if p.PendingHistoryTTL <= 0 {
		p.PendingHistoryTTL = DefaultPendingHistoryTTL
	}
	return p
}

type SimpleDecision struct {
	At                 time.Time `json:"at"`
	Reason             string    `json:"reason"`
	BaselineWinRate    float64   `json:"baseline_winrate"`
	ObservedWinRate    float64   `json:"observed_winrate"`
	TargetProfitPerReq float64   `json:"target_profit_per_request"`
	ActualProfitPerReq float64   `json:"actual_profit_per_request"`
	OldMargin          float64   `json:"old_margin"`
	NewMargin          float64   `json:"new_margin"`
	OldSSPBid          float64   `json:"old_ssp_bid"`
	NewSSPBid          float64   `json:"new_ssp_bid"`
	Impressions        uint64    `json:"impressions"`
	Requests           uint64    `json:"requests"`
	OldPointVersion    uint64    `json:"old_point_version"`
	NewPointVersion    uint64    `json:"new_point_version"`
}

type SimpleState struct {
	SegmentHash          string           `json:"segment_hash"`
	CampaignID           string           `json:"campaign_id"`
	TypeModel            int              `json:"type_model"`
	RTB                  bool             `json:"rtb"`
	ReferenceOriginalBid float64          `json:"reference_original_bid"`
	EffectiveMin         float64          `json:"effective_min"`
	MapSource            string           `json:"map_source,omitempty"`
	MaxMargin            float64          `json:"max_margin"`
	Margin               float64          `json:"margin"`
	SSPBid               float64          `json:"ssp_bid"`
	BaselineWinRate      float64          `json:"baseline_winrate"`
	LastConfirmedMargin  float64          `json:"last_confirmed_margin"`
	LastConfirmedProfit  float64          `json:"last_confirmed_profit_per_request"`
	LastRequests         uint64           `json:"last_requests"`
	LastImpressions      uint64           `json:"last_impressions"`
	StepIndex            int              `json:"step_index"`
	AwaitingProbe        bool             `json:"awaiting_probe"`
	PointVersion         uint64           `json:"point_version"`
	Phase                string           `json:"phase"`
	LastOptimizeAt       time.Time        `json:"last_optimize_at"`
	LastRebenchmarkAt    time.Time        `json:"last_rebenchmark_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
	DecisionHistory      []SimpleDecision `json:"decision_history,omitempty"`
	PendingHistory       *HistoryEvent    `json:"pending_history,omitempty"`
}

func NewSimpleState(segmentHash, campaignID string, originalBid, effectiveMin float64, rtb bool, policy SimplePolicy, now time.Time, mapSource ...string) SimpleState {
	policy = policy.Normalize()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	effectiveMin = clampMargin(effectiveMin, 0, policy.MaxMargin)
	source := ""
	if len(mapSource) > 0 {
		source = strings.TrimSpace(mapSource[0])
	}
	state := SimpleState{
		SegmentHash:          strings.TrimSpace(segmentHash),
		CampaignID:           strings.TrimSpace(campaignID),
		TypeModel:            TypeModelSimple,
		RTB:                  rtb,
		ReferenceOriginalBid: originalBid,
		EffectiveMin:         effectiveMin,
		MapSource:            source,
		MaxMargin:            policy.MaxMargin,
		Margin:               effectiveMin,
		BaselineWinRate:      0,
		LastConfirmedMargin:  effectiveMin,
		StepIndex:            0,
		PointVersion:         1,
		Phase:                SimplePhaseBaseline,
		UpdatedAt:            now,
	}
	state.SSPBid = priceAfterMargin(originalBid, state.Margin)
	return state
}

func (s SimpleState) PricingForOriginalBid(originalBid float64) SimplePricing {
	return SimplePricing{
		SegmentHash:  s.SegmentHash,
		Margin:       s.Margin,
		SSPBid:       priceAfterMargin(originalBid, s.Margin),
		PointVersion: s.PointVersion,
	}
}

type SimplePricing struct {
	SegmentHash  string
	Margin       float64
	SSPBid       float64
	PointVersion uint64
}

func SimpleStateKey(segmentHash string) string {
	return SimpleStateKeyPrefix + strings.TrimSpace(segmentHash)
}

func (s SimpleState) Compatible(segmentHash, campaignID string, originalBid, effectiveMin float64, rtb bool, policy SimplePolicy) bool {
	policy = policy.Normalize()
	if strings.TrimSpace(s.SegmentHash) != strings.TrimSpace(segmentHash) || strings.TrimSpace(s.CampaignID) != strings.TrimSpace(campaignID) {
		return false
	}
	if s.TypeModel != TypeModelSimple || s.RTB != rtb {
		return false
	}
	if !approximatelyEqual(s.EffectiveMin, effectiveMin) || !approximatelyEqual(s.MaxMargin, policy.MaxMargin) {
		return false
	}
	// RTB external bids are dynamic. They must never invalidate a Simple state.
	if !rtb && !approximatelyEqual(s.ReferenceOriginalBid, originalBid) {
		return false
	}
	return true
}

func RepairSimpleState(state SimpleState, originalBid, effectiveMin float64, rtb bool, policy SimplePolicy, now time.Time) (SimpleState, bool) {
	policy = policy.Normalize()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	invalid := strings.TrimSpace(state.SegmentHash) == "" || strings.TrimSpace(state.CampaignID) == "" || state.TypeModel != TypeModelSimple ||
		math.IsNaN(state.Margin) || math.IsInf(state.Margin, 0) || math.IsNaN(state.EffectiveMin) || math.IsInf(state.EffectiveMin, 0) ||
		state.PointVersion == 0 || state.StepIndex < 0 || state.StepIndex >= len(policy.SearchStepsPP) ||
		(state.Phase != SimplePhaseBaseline && state.Phase != SimplePhaseSearch && state.Phase != SimplePhaseSettled)
	if invalid {
		reset := NewSimpleState(state.SegmentHash, state.CampaignID, originalBid, effectiveMin, rtb, policy, now, state.MapSource)
		reset.PointVersion = nextPointVersion(state.PointVersion)
		return reset, true
	}

	changed := false
	originalMargin := state.Margin
	state.RTB = rtb
	state.MaxMargin = policy.MaxMargin
	state.EffectiveMin = clampMargin(effectiveMin, 0, policy.MaxMargin)
	margin := clampMargin(state.Margin, state.EffectiveMin, state.MaxMargin)
	if !approximatelyEqual(margin, state.Margin) {
		state.Margin = margin
		changed = true
	}
	confirmed := clampMargin(state.LastConfirmedMargin, state.EffectiveMin, state.MaxMargin)
	if !approximatelyEqual(confirmed, state.LastConfirmedMargin) {
		state.LastConfirmedMargin = confirmed
		changed = true
	}
	if !rtb && !approximatelyEqual(state.ReferenceOriginalBid, originalBid) {
		state.ReferenceOriginalBid = originalBid
		changed = true
	}
	if rtb && state.ReferenceOriginalBid <= 0 {
		state.ReferenceOriginalBid = originalBid
		changed = true
	}
	state.SSPBid = priceAfterMargin(originalBid, state.Margin)
	if changed {
		oldVersion := state.PointVersion
		state.PointVersion = nextPointVersion(state.PointVersion)
		state.BaselineWinRate = 0
		state.LastConfirmedProfit = 0
		state.StepIndex = 0
		state.AwaitingProbe = false
		state.Phase = SimplePhaseBaseline
		state.LastRebenchmarkAt = time.Time{}
		state.LastOptimizeAt = time.Time{}
		state.UpdatedAt = now
		state.appendDecision(SimpleDecision{At: now, Reason: "state_repaired_rebenchmark", OldMargin: originalMargin, NewMargin: state.Margin, OldSSPBid: priceAfterMargin(originalBid, originalMargin), NewSSPBid: state.SSPBid, OldPointVersion: oldVersion, NewPointVersion: state.PointVersion})
	}
	return state, changed
}

func RebenchmarkSimpleState(state SimpleState, policy SimplePolicy, now time.Time, reason string) SimpleState {
	policy = policy.Normalize()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	oldMargin := state.Margin
	oldVersion := state.PointVersion
	state.Margin = clampMargin(state.EffectiveMin, 0, policy.MaxMargin)
	state.SSPBid = priceAfterMargin(state.ReferenceOriginalBid, state.Margin)
	state.BaselineWinRate = 0
	state.LastConfirmedMargin = state.Margin
	state.LastConfirmedProfit = 0
	state.StepIndex = 0
	state.AwaitingProbe = false
	state.PointVersion = nextPointVersion(state.PointVersion)
	state.Phase = SimplePhaseBaseline
	state.LastOptimizeAt = now
	state.LastRebenchmarkAt = now
	state.UpdatedAt = now
	state.appendDecision(SimpleDecision{At: now, Reason: reason, OldMargin: oldMargin, NewMargin: state.Margin, OldSSPBid: priceAfterMargin(state.ReferenceOriginalBid, oldMargin), NewSSPBid: state.SSPBid, OldPointVersion: oldVersion, NewPointVersion: state.PointVersion})
	return state
}

func (s *SimpleState) appendDecision(decision SimpleDecision) {
	if s == nil {
		return
	}
	const maxHistory = 64
	s.DecisionHistory = append(s.DecisionHistory, decision)
	if len(s.DecisionHistory) > maxHistory {
		s.DecisionHistory = append([]SimpleDecision(nil), s.DecisionHistory[len(s.DecisionHistory)-maxHistory:]...)
	}
}

func nextPointVersion(current uint64) uint64 {
	if current == ^uint64(0) {
		return current
	}
	return current + 1
}

func clampMargin(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func priceAfterMargin(originalBid, margin float64) float64 {
	if originalBid <= 0 || math.IsNaN(originalBid) || math.IsInf(originalBid, 0) {
		return 0
	}
	return originalBid * (1 - margin)
}

func approximatelyEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-12
}

type SimpleStateStore struct {
	redis  *redis.Client
	policy SimplePolicy
}

func NewSimpleStateStore(client *redis.Client, policy SimplePolicy) *SimpleStateStore {
	return &SimpleStateStore{redis: client, policy: policy.Normalize()}
}

func (s *SimpleStateStore) Policy() SimplePolicy {
	if s == nil {
		return SimplePolicy{}.Normalize()
	}
	return s.policy.Normalize()
}

func (s *SimpleStateStore) Get(ctx context.Context, segmentHash string) (SimpleState, error) {
	if s == nil || s.redis == nil {
		return SimpleState{}, errors.New("simple percenter Redis store is not configured")
	}
	raw, err := s.redis.Get(ctx, SimpleStateKey(segmentHash)).Bytes()
	if err != nil {
		return SimpleState{}, err
	}
	var state SimpleState
	if err := json.Unmarshal(raw, &state); err != nil {
		return SimpleState{}, fmt.Errorf("decode simple percenter state: %w", err)
	}
	return state, nil
}

func (s *SimpleStateStore) GetOrInitPricing(ctx context.Context, segmentHash, campaignID string, originalBid, effectiveMin float64, rtb bool, now time.Time, mapSource ...string) (SimplePricing, error) {
	if s == nil || s.redis == nil {
		return SimplePricing{}, errors.New("simple percenter Redis store is not configured")
	}
	policy := s.policy.Normalize()
	source := ""
	if len(mapSource) > 0 {
		source = strings.TrimSpace(mapSource[0])
	}
	key := SimpleStateKey(segmentHash)
	var result SimpleState
	for attempt := 0; attempt < 4; attempt++ {
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, key).Bytes()
			if errors.Is(err, redis.Nil) {
				result = NewSimpleState(segmentHash, campaignID, originalBid, effectiveMin, rtb, policy, now, source)
				return saveSimpleStateTx(ctx, tx, key, result, policy.StateTTL, policy.PendingHistoryTTL, true)
			}
			if err != nil {
				return err
			}
			var state SimpleState
			if err := json.Unmarshal(raw, &state); err != nil {
				// Corrupt state is replaced by a safe baseline rather than used for pricing.
				result = NewSimpleState(segmentHash, campaignID, originalBid, effectiveMin, rtb, policy, now, source)
				return saveSimpleStateTx(ctx, tx, key, result, policy.StateTTL, policy.PendingHistoryTTL, true)
			}
			if !state.Compatible(segmentHash, campaignID, originalBid, effectiveMin, rtb, policy) {
				oldPointVersion := state.PointVersion
				state = NewSimpleState(segmentHash, campaignID, originalBid, effectiveMin, rtb, policy, now, source)
				state.PointVersion = nextPointVersion(oldPointVersion)
				result = state
				return saveSimpleStateTx(ctx, tx, key, state, policy.StateTTL, policy.PendingHistoryTTL, true)
			}
			repaired, changed := RepairSimpleState(state, originalBid, effectiveMin, rtb, policy, now)
			if source != "" && repaired.MapSource != source {
				repaired.MapSource = source
				changed = true
			}
			result = repaired
			if !changed {
				return nil
			}
			return saveSimpleStateTx(ctx, tx, key, repaired, policy.StateTTL, policy.PendingHistoryTTL, true)
		}, key)
		if err == nil {
			return result.PricingForOriginalBid(originalBid), nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return SimplePricing{}, err
		}
	}
	return SimplePricing{}, redis.TxFailedErr
}

func (s *SimpleStateStore) SaveCAS(ctx context.Context, state SimpleState, expectedPointVersion uint64) (bool, error) {
	if s == nil || s.redis == nil {
		return false, errors.New("simple percenter Redis store is not configured")
	}
	key := SimpleStateKey(state.SegmentHash)
	policy := s.policy.Normalize()
	for attempt := 0; attempt < 4; attempt++ {
		saved := false
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			current, err := tx.Get(ctx, key).Bytes()
			if err != nil {
				return err
			}
			var persisted SimpleState
			if err := json.Unmarshal(current, &persisted); err != nil {
				return err
			}
			if persisted.PointVersion != expectedPointVersion {
				return nil
			}
			if err := saveSimpleStateTx(ctx, tx, key, state, policy.StateTTL, policy.PendingHistoryTTL, true); err != nil {
				return err
			}
			saved = true
			return nil
		}, key)
		if err == nil {
			return saved, nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return false, err
		}
	}
	return false, redis.TxFailedErr
}

func saveSimpleStateTx(ctx context.Context, tx *redis.Tx, key string, state SimpleState, stateTTL, pendingHistoryTTL time.Duration, addIndex bool) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, key, payload, stateTTL)
		if err := stagePendingHistoryRedis(ctx, pipe, state.PendingHistory, pendingHistoryTTL); err != nil {
			return err
		}
		if addIndex {
			pipe.SAdd(ctx, SimpleStateIndexKey, state.SegmentHash)
		}
		return nil
	})
	return err
}

func (s *SimpleStateStore) SegmentHashes(ctx context.Context) ([]string, error) {
	if s == nil || s.redis == nil {
		return nil, errors.New("simple percenter Redis store is not configured")
	}
	return s.redis.SMembers(ctx, SimpleStateIndexKey).Result()
}

func (s *SimpleStateStore) States(ctx context.Context) ([]SimpleState, error) {
	hashes, err := s.SegmentHashes(ctx)
	if err != nil {
		return nil, err
	}
	const batchSize = 512
	states := make([]SimpleState, 0, len(hashes))
	for start := 0; start < len(hashes); start += batchSize {
		end := start + batchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		keys := make([]string, 0, end-start)
		for _, hash := range hashes[start:end] {
			keys = append(keys, SimpleStateKey(hash))
		}
		values, err := s.redis.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, err
		}
		for offset, value := range values {
			hash := hashes[start+offset]
			if value == nil {
				if err := s.removeStaleIndexMember(ctx, hash); err != nil {
					return nil, fmt.Errorf("remove stale simple state index member %q: %w", hash, err)
				}
				continue
			}
			var raw []byte
			switch v := value.(type) {
			case string:
				raw = []byte(v)
			case []byte:
				raw = v
			default:
				continue
			}
			var state SimpleState
			if err := json.Unmarshal(raw, &state); err != nil {
				return nil, fmt.Errorf("decode simple percenter state list item: %w", err)
			}
			states = append(states, state)
		}
	}
	return states, nil
}

func (s *SimpleStateStore) removeStaleIndexMember(ctx context.Context, segmentHash string) error {
	if s == nil || s.redis == nil {
		return errors.New("simple percenter Redis store is not configured")
	}
	key := SimpleStateKey(segmentHash)
	for attempt := 0; attempt < 4; attempt++ {
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			_, err := tx.Get(ctx, key).Result()
			if err == nil {
				return nil
			}
			if !errors.Is(err, redis.Nil) {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.SRem(ctx, SimpleStateIndexKey, segmentHash)
				return nil
			})
			return err
		}, key)
		if err == nil {
			return nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}
	return redis.TxFailedErr
}
