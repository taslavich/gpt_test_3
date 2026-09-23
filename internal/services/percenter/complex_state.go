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
	TypeModelComplex = 2

	ComplexPhaseBenchmark      = "complex_benchmark"
	ComplexPhaseSSPSearch      = "complex_ssp_search"
	ComplexPhaseMarginBaseline = "complex_margin_baseline"
	ComplexPhaseMarginSearch   = "complex_margin_search"

	ComplexStateKeyPrefix   = "percenter:complex:segment:"
	ComplexStateIndexKey    = "percenter:complex:segments"
	ComplexPointVersionBase = uint64(1) << 63
)

type ComplexPolicy struct {
	BuyoutRetention       float64
	EfficiencyRetention   float64
	MinImpressions        uint64
	OptimizeInterval      time.Duration
	RebenchmarkInterval   time.Duration
	SSPSearchStepsPercent []float64
	MarginSearchStepsPP   []float64
	MaxMargin             float64
	StateTTL              time.Duration
	PendingHistoryTTL     time.Duration
}

func (p ComplexPolicy) Normalize() ComplexPolicy {
	if p.BuyoutRetention <= 0 || p.BuyoutRetention > 1 {
		p.BuyoutRetention = 0.80
	}
	if p.EfficiencyRetention <= 0 || p.EfficiencyRetention > 1 {
		p.EfficiencyRetention = 0.80
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
	p.SSPSearchStepsPercent = normalizePositiveSteps(p.SSPSearchStepsPercent, []float64{10, 5, 2, 1})
	p.MarginSearchStepsPP = normalizePositiveSteps(p.MarginSearchStepsPP, []float64{10, 5, 2, 1})
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

func normalizePositiveSteps(raw, fallback []float64) []float64 {
	clean := make([]float64, 0, len(raw))
	for _, step := range raw {
		if step > 0 && !math.IsNaN(step) && !math.IsInf(step, 0) {
			clean = append(clean, step)
		}
	}
	if len(clean) == 0 {
		return append([]float64(nil), fallback...)
	}
	return clean
}

type ComplexDecision struct {
	At                         time.Time `json:"at"`
	Phase                      string    `json:"phase"`
	Reason                     string    `json:"reason"`
	Requests                   uint64    `json:"requests"`
	Impressions                uint64    `json:"impressions"`
	Wins                       uint64    `json:"wins"`
	ObservedBuyout             float64   `json:"observed_buyout"`
	BaselineBuyout             float64   `json:"baseline_buyout"`
	AdvertiserSpend            float64   `json:"advertiser_spend"`
	ObservedEfficiency         float64   `json:"observed_efficiency"`
	BaselineEfficiency         float64   `json:"baseline_efficiency"`
	TargetProfitPerOpportunity float64   `json:"target_profit_per_relevant_opportunity"`
	ActualProfitPerOpportunity float64   `json:"actual_profit_per_relevant_opportunity"`
	BuyoutThresholdPassed      bool      `json:"buyout_threshold_passed"`
	EfficiencyThresholdPassed  bool      `json:"efficiency_threshold_passed"`
	ProfitImproved             bool      `json:"profit_improved"`
	OldSSPBid                  float64   `json:"old_ssp_bid"`
	NewSSPBid                  float64   `json:"new_ssp_bid"`
	OldMargin                  float64   `json:"old_margin"`
	NewMargin                  float64   `json:"new_margin"`
	OldAdvertiserPrice         float64   `json:"old_advertiser_price"`
	NewAdvertiserPrice         float64   `json:"new_advertiser_price"`
	OldPointVersion            uint64    `json:"old_point_version"`
	NewPointVersion            uint64    `json:"new_point_version"`
}

type ComplexState struct {
	SegmentHash         string            `json:"segment_hash"`
	CampaignID          string            `json:"campaign_id"`
	TypeModel           int               `json:"type_model"`
	Phase               string            `json:"phase"`
	OriginalBid         float64           `json:"original_bid"`
	AdvertiserPrice     float64           `json:"advertiser_price"`
	SSPBid              float64           `json:"ssp_bid"`
	Margin              float64           `json:"margin"`
	EffectiveMin        float64           `json:"effective_min"`
	MapSource           string            `json:"map_source,omitempty"`
	MaxMargin           float64           `json:"max_margin"`
	BaselineBuyout      float64           `json:"baseline_buyout"`
	BaselineEfficiency  float64           `json:"baseline_efficiency"`
	LastGoodSSPBid      float64           `json:"last_good_ssp_bid"`
	LastGoodMargin      float64           `json:"last_good_margin"`
	LastConfirmedProfit float64           `json:"last_confirmed_profit_per_relevant_opportunity"`
	SSPStepIndex        int               `json:"ssp_step_index"`
	MarginStepIndex     int               `json:"margin_step_index"`
	AwaitingProbe       bool              `json:"awaiting_probe"`
	SearchComplete      bool              `json:"search_complete"`
	PointVersion        uint64            `json:"point_version"`
	LastOptimizeAt      time.Time         `json:"last_optimize_at"`
	LastRebenchmarkAt   time.Time         `json:"last_rebenchmark_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	DecisionHistory     []ComplexDecision `json:"decision_history,omitempty"`
	PendingHistory      *HistoryEvent     `json:"pending_history,omitempty"`
}

type ComplexPricing struct {
	SegmentHash     string
	AdvertiserPrice float64
	Margin          float64
	SSPBid          float64
	PointVersion    uint64
}

func NewComplexState(segmentHash, campaignID string, originalBid, effectiveMin float64, policy ComplexPolicy, now time.Time, mapSource ...string) ComplexState {
	policy = policy.Normalize()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	effectiveMin = clampMargin(effectiveMin, 0, policy.MaxMargin)
	sspBid := priceAfterMargin(originalBid, effectiveMin)
	source := ""
	if len(mapSource) > 0 {
		source = strings.TrimSpace(mapSource[0])
	}
	return ComplexState{
		SegmentHash:     strings.TrimSpace(segmentHash),
		CampaignID:      strings.TrimSpace(campaignID),
		TypeModel:       TypeModelComplex,
		Phase:           ComplexPhaseBenchmark,
		OriginalBid:     originalBid,
		AdvertiserPrice: originalBid,
		SSPBid:          sspBid,
		Margin:          effectiveMin,
		EffectiveMin:    effectiveMin,
		MapSource:       source,
		MaxMargin:       policy.MaxMargin,
		LastGoodSSPBid:  sspBid,
		LastGoodMargin:  effectiveMin,
		SSPStepIndex:    0,
		MarginStepIndex: 0,
		PointVersion:    ComplexPointVersionBase + 1,
		UpdatedAt:       now,
	}
}

func (s ComplexState) Pricing() ComplexPricing {
	return ComplexPricing{
		SegmentHash:     s.SegmentHash,
		AdvertiserPrice: s.AdvertiserPrice,
		Margin:          s.Margin,
		SSPBid:          s.SSPBid,
		PointVersion:    s.PointVersion,
	}
}

func ComplexStateKey(segmentHash string) string {
	return ComplexStateKeyPrefix + strings.TrimSpace(segmentHash)
}

func (s ComplexState) Compatible(segmentHash, campaignID string, originalBid, effectiveMin float64, policy ComplexPolicy) bool {
	policy = policy.Normalize()
	return strings.TrimSpace(s.SegmentHash) == strings.TrimSpace(segmentHash) &&
		strings.TrimSpace(s.CampaignID) == strings.TrimSpace(campaignID) &&
		s.TypeModel == TypeModelComplex &&
		approximatelyEqual(s.OriginalBid, originalBid) &&
		approximatelyEqual(s.EffectiveMin, effectiveMin) &&
		approximatelyEqual(s.MaxMargin, policy.MaxMargin)
}

func RepairComplexState(state ComplexState, originalBid, effectiveMin float64, policy ComplexPolicy, now time.Time) (ComplexState, bool) {
	policy = policy.Normalize()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	phaseOK := state.Phase == ComplexPhaseBenchmark || state.Phase == ComplexPhaseSSPSearch || state.Phase == ComplexPhaseMarginBaseline || state.Phase == ComplexPhaseMarginSearch
	invalid := strings.TrimSpace(state.SegmentHash) == "" || strings.TrimSpace(state.CampaignID) == "" || state.TypeModel != TypeModelComplex ||
		!finiteComplexValue(state.OriginalBid) || !finiteComplexValue(state.SSPBid) || !finiteComplexValue(state.AdvertiserPrice) || !finiteComplexValue(state.LastGoodSSPBid) ||
		state.SSPBid > state.AdvertiserPrice+1e-12 || math.IsNaN(state.Margin) || math.IsInf(state.Margin, 0) || math.IsNaN(state.LastGoodMargin) || math.IsInf(state.LastGoodMargin, 0) ||
		math.IsNaN(state.BaselineBuyout) || math.IsInf(state.BaselineBuyout, 0) || state.BaselineBuyout < 0 ||
		math.IsNaN(state.BaselineEfficiency) || math.IsInf(state.BaselineEfficiency, 0) || state.BaselineEfficiency < 0 ||
		state.PointVersion == 0 || !phaseOK || state.SSPStepIndex < 0 || state.SSPStepIndex >= len(policy.SSPSearchStepsPercent) ||
		state.MarginStepIndex < 0 || state.MarginStepIndex >= len(policy.MarginSearchStepsPP)
	if invalid {
		reset := NewComplexState(state.SegmentHash, state.CampaignID, originalBid, effectiveMin, policy, now, state.MapSource)
		reset.PointVersion = nextPointVersion(state.PointVersion)
		return reset, true
	}

	newMin := clampMargin(effectiveMin, 0, policy.MaxMargin)
	if !approximatelyEqual(state.OriginalBid, originalBid) || !approximatelyEqual(state.EffectiveMin, newMin) || !approximatelyEqual(state.MaxMargin, policy.MaxMargin) ||
		state.Margin < newMin-1e-12 || state.Margin > policy.MaxMargin+1e-12 || state.LastGoodMargin < newMin-1e-12 || state.LastGoodMargin > policy.MaxMargin+1e-12 {
		reset := NewComplexState(state.SegmentHash, state.CampaignID, originalBid, newMin, policy, now, state.MapSource)
		reset.PointVersion = nextPointVersion(state.PointVersion)
		reset.appendDecision(ComplexDecision{
			At: now, Phase: ComplexPhaseBenchmark, Reason: "state_repaired_rebenchmark",
			OldSSPBid: state.SSPBid, NewSSPBid: reset.SSPBid,
			OldMargin: state.Margin, NewMargin: reset.Margin,
			OldAdvertiserPrice: state.AdvertiserPrice, NewAdvertiserPrice: reset.AdvertiserPrice,
			OldPointVersion: state.PointVersion, NewPointVersion: reset.PointVersion,
		})
		return reset, true
	}
	return state, false
}

func RebenchmarkComplexState(state ComplexState, policy ComplexPolicy, now time.Time, reason string) ComplexState {
	policy = policy.Normalize()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	old := state
	state.Phase = ComplexPhaseBenchmark
	state.AdvertiserPrice = state.OriginalBid
	state.Margin = clampMargin(state.EffectiveMin, 0, policy.MaxMargin)
	state.SSPBid = priceAfterMargin(state.OriginalBid, state.Margin)
	state.BaselineBuyout = 0
	state.BaselineEfficiency = 0
	state.LastGoodSSPBid = state.SSPBid
	state.LastGoodMargin = state.Margin
	state.LastConfirmedProfit = 0
	state.SSPStepIndex = 0
	state.MarginStepIndex = 0
	state.AwaitingProbe = false
	state.SearchComplete = false
	state.PointVersion = nextPointVersion(state.PointVersion)
	state.LastOptimizeAt = now
	state.LastRebenchmarkAt = now
	state.UpdatedAt = now
	state.appendDecision(ComplexDecision{
		At: now, Phase: state.Phase, Reason: reason,
		OldSSPBid: old.SSPBid, NewSSPBid: state.SSPBid,
		OldMargin: old.Margin, NewMargin: state.Margin,
		OldAdvertiserPrice: old.AdvertiserPrice, NewAdvertiserPrice: state.AdvertiserPrice,
		OldPointVersion: old.PointVersion, NewPointVersion: state.PointVersion,
	})
	return state
}

func (s *ComplexState) appendDecision(decision ComplexDecision) {
	if s == nil {
		return
	}
	const maxHistory = 64
	s.DecisionHistory = append(s.DecisionHistory, decision)
	if len(s.DecisionHistory) > maxHistory {
		s.DecisionHistory = append([]ComplexDecision(nil), s.DecisionHistory[len(s.DecisionHistory)-maxHistory:]...)
	}
}

func finiteComplexValue(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func advertiserPriceForMargin(sspBid, margin float64) float64 {
	if !finiteComplexValue(sspBid) || margin < 0 || margin >= 1 || math.IsNaN(margin) || math.IsInf(margin, 0) {
		return 0
	}
	return sspBid / (1 - margin)
}

type ComplexStateStore struct {
	redis  *redis.Client
	policy ComplexPolicy
}

func NewComplexStateStore(client *redis.Client, policy ComplexPolicy) *ComplexStateStore {
	return &ComplexStateStore{redis: client, policy: policy.Normalize()}
}

func (s *ComplexStateStore) Policy() ComplexPolicy {
	if s == nil {
		return ComplexPolicy{}.Normalize()
	}
	return s.policy.Normalize()
}

func (s *ComplexStateStore) Get(ctx context.Context, segmentHash string) (ComplexState, error) {
	if s == nil || s.redis == nil {
		return ComplexState{}, errors.New("complex percenter Redis store is not configured")
	}
	raw, err := s.redis.Get(ctx, ComplexStateKey(segmentHash)).Bytes()
	if err != nil {
		return ComplexState{}, err
	}
	var state ComplexState
	if err := json.Unmarshal(raw, &state); err != nil {
		return ComplexState{}, fmt.Errorf("decode complex percenter state: %w", err)
	}
	return state, nil
}

func (s *ComplexStateStore) GetOrInitPricing(ctx context.Context, segmentHash, campaignID string, originalBid, effectiveMin float64, now time.Time, mapSource ...string) (ComplexPricing, error) {
	if s == nil || s.redis == nil {
		return ComplexPricing{}, errors.New("complex percenter Redis store is not configured")
	}
	policy := s.policy.Normalize()
	source := ""
	if len(mapSource) > 0 {
		source = strings.TrimSpace(mapSource[0])
	}
	key := ComplexStateKey(segmentHash)
	var result ComplexState
	for attempt := 0; attempt < 4; attempt++ {
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, key).Bytes()
			if errors.Is(err, redis.Nil) {
				result = NewComplexState(segmentHash, campaignID, originalBid, effectiveMin, policy, now, source)
				return saveComplexStateTx(ctx, tx, key, result, policy.StateTTL, policy.PendingHistoryTTL, true)
			}
			if err != nil {
				return err
			}
			var state ComplexState
			if err := json.Unmarshal(raw, &state); err != nil {
				result = NewComplexState(segmentHash, campaignID, originalBid, effectiveMin, policy, now, source)
				return saveComplexStateTx(ctx, tx, key, result, policy.StateTTL, policy.PendingHistoryTTL, true)
			}
			if !state.Compatible(segmentHash, campaignID, originalBid, effectiveMin, policy) {
				oldPointVersion := state.PointVersion
				state = NewComplexState(segmentHash, campaignID, originalBid, effectiveMin, policy, now, source)
				state.PointVersion = nextPointVersion(oldPointVersion)
				result = state
				return saveComplexStateTx(ctx, tx, key, state, policy.StateTTL, policy.PendingHistoryTTL, true)
			}
			repaired, changed := RepairComplexState(state, originalBid, effectiveMin, policy, now)
			if source != "" && repaired.MapSource != source {
				repaired.MapSource = source
				changed = true
			}
			result = repaired
			if !changed {
				return nil
			}
			return saveComplexStateTx(ctx, tx, key, repaired, policy.StateTTL, policy.PendingHistoryTTL, true)
		}, key)
		if err == nil {
			return result.Pricing(), nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return ComplexPricing{}, err
		}
	}
	return ComplexPricing{}, redis.TxFailedErr
}

func (s *ComplexStateStore) SaveCAS(ctx context.Context, state ComplexState, expectedPointVersion uint64) (bool, error) {
	if s == nil || s.redis == nil {
		return false, errors.New("complex percenter Redis store is not configured")
	}
	key := ComplexStateKey(state.SegmentHash)
	policy := s.policy.Normalize()
	for attempt := 0; attempt < 4; attempt++ {
		saved := false
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			current, err := tx.Get(ctx, key).Bytes()
			if err != nil {
				return err
			}
			var persisted ComplexState
			if err := json.Unmarshal(current, &persisted); err != nil {
				return err
			}
			if persisted.PointVersion != expectedPointVersion {
				return nil
			}
			if err := saveComplexStateTx(ctx, tx, key, state, policy.StateTTL, policy.PendingHistoryTTL, true); err != nil {
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

func saveComplexStateTx(ctx context.Context, tx *redis.Tx, key string, state ComplexState, stateTTL, pendingHistoryTTL time.Duration, addIndex bool) error {
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
			pipe.SAdd(ctx, ComplexStateIndexKey, state.SegmentHash)
		}
		return nil
	})
	return err
}

func (s *ComplexStateStore) SegmentHashes(ctx context.Context) ([]string, error) {
	if s == nil || s.redis == nil {
		return nil, errors.New("complex percenter Redis store is not configured")
	}
	return s.redis.SMembers(ctx, ComplexStateIndexKey).Result()
}

func (s *ComplexStateStore) States(ctx context.Context) ([]ComplexState, error) {
	hashes, err := s.SegmentHashes(ctx)
	if err != nil {
		return nil, err
	}
	const batchSize = 512
	states := make([]ComplexState, 0, len(hashes))
	for start := 0; start < len(hashes); start += batchSize {
		end := start + batchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		keys := make([]string, 0, end-start)
		for _, hash := range hashes[start:end] {
			keys = append(keys, ComplexStateKey(hash))
		}
		values, err := s.redis.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, err
		}
		for offset, value := range values {
			hash := hashes[start+offset]
			if value == nil {
				if err := s.removeStaleIndexMember(ctx, hash); err != nil {
					return nil, fmt.Errorf("remove stale complex state index member %q: %w", hash, err)
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
			var state ComplexState
			if err := json.Unmarshal(raw, &state); err != nil {
				return nil, fmt.Errorf("decode complex percenter state list item: %w", err)
			}
			states = append(states, state)
		}
	}
	return states, nil
}

func (s *ComplexStateStore) removeStaleIndexMember(ctx context.Context, segmentHash string) error {
	if s == nil || s.redis == nil {
		return errors.New("complex percenter Redis store is not configured")
	}
	key := ComplexStateKey(segmentHash)
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
				pipe.SRem(ctx, ComplexStateIndexKey, segmentHash)
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
