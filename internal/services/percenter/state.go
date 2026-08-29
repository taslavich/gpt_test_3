package percenter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	PhaseBenchmark      = "benchmark"
	PhaseSSPSearch      = "ssp_search"
	PhaseMarginBaseline = "margin_baseline"
	PhaseMarginSearch   = "margin_search"
)

type Policy struct {
	BuyoutRetention        float64
	EfficiencyRetention    float64
	DefaultMinMargin       float64
	PromoMinMargin         float64
	MaxMargin              float64
	SSPSearchPrecision     float64
	MarginSearchStepsPP    []float64
	SSPReoptimizeInterval  time.Duration
	MarginOptimizeInterval time.Duration
	SegmentStateTTL        time.Duration
	ADVCacheTTL            time.Duration
}

func (p Policy) Normalize() Policy {
	if p.BuyoutRetention <= 0 || p.BuyoutRetention > 1 {
		p.BuyoutRetention = 0.80
	}
	if p.EfficiencyRetention <= 0 || p.EfficiencyRetention > 1 {
		p.EfficiencyRetention = 0.80
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
	SegmentHash     string  `json:"segment_hash"`
	CampaignID      string  `json:"campaign_id"`
	CampaignVersion int64   `json:"campaign_version"`
	PointVersion    uint64  `json:"point_version"`
	OriginalBid     float64 `json:"original_bid"`
	MinMargin       float64 `json:"min_margin"`

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

	LastChangeAt        time.Time `json:"last_change_at"`
	LastSSPReoptimizeAt time.Time `json:"last_ssp_reoptimize_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func BaselineState(segmentHash, campaignID string, originalBid, minMargin float64, campaignRevision int64, now time.Time) State {
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
	return State{
		SegmentHash:     segmentHash,
		CampaignID:      campaignID,
		CampaignVersion: campaignRevision,
		PointVersion:    1,
		OriginalBid:     originalBid,
		MinMargin:       minMargin,
		AdvertiserPrice: originalBid,
		SSPBid:          ssp,
		Margin:          minMargin,
		Phase:           PhaseBenchmark,
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
	return finitePositive(s.OriginalBid) && finitePositive(s.AdvertiserPrice) && finitePositive(s.SSPBid) &&
		approximatelyEqual(s.OriginalBid, originalBid) && approximatelyEqual(s.MinMargin, minMargin) &&
		s.CampaignVersion == campaignRevision
}

func (s State) sspReoptimizationDue(now time.Time, interval time.Duration) bool {
	if interval <= 0 || s.LastSSPReoptimizeAt.IsZero() {
		return false
	}
	return !now.Before(s.LastSSPReoptimizeAt.Add(interval))
}

type Pricing struct {
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
}

func NewStateStore(client *redis.Client, policy Policy) *StateStore {
	return &StateStore{redis: client, policy: policy.Normalize(), cache: make(map[string]cachedState)}
}

func SegmentKey(hash string) string { return "percenter:segment:" + hash }

var compareAndSwapStateScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
if not current then
  return 0
end
if current ~= ARGV[1] then
  return 0
end
redis.call("SET", KEYS[1], ARGV[2], "PX", ARGV[3])
return 1
`)

func (s *StateStore) GetOrInitPricing(ctx context.Context, segmentHash, campaignID string, originalBid, minMargin float64, campaignRevision int64, now time.Time) (Pricing, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	baseline := BaselineState(segmentHash, campaignID, originalBid, minMargin, campaignRevision, now)
	fallback := pricingFromState(baseline)
	fallback.FromFallback = true
	if s == nil || s.redis == nil {
		return fallback, errors.New("percenter redis is unavailable")
	}

	if state, ok := s.cached(segmentHash, originalBid, minMargin, campaignRevision, now); ok {
		if state.sspReoptimizationDue(now, s.policy.SSPReoptimizeInterval) {
			baseline.PointVersion = nextPointVersion(state.PointVersion)
			if err := s.Save(ctx, baseline); err != nil {
				return pricingFromState(state), err
			}
			return pricingFromState(baseline), nil
		}
		return pricingFromState(state), nil
	}

	state, err := s.Load(ctx, segmentHash)
	if err != nil && !errors.Is(err, redis.Nil) {
		return fallback, err
	}
	if errors.Is(err, redis.Nil) || !state.ValidFor(originalBid, minMargin, campaignRevision) {
		if !errors.Is(err, redis.Nil) && state.PointVersion > 0 {
			baseline.PointVersion = nextPointVersion(state.PointVersion)
		}
		state = baseline
		if err := s.Save(ctx, state); err != nil {
			return fallback, err
		}
	} else if state.sspReoptimizationDue(now, s.policy.SSPReoptimizeInterval) {
		previous := state
		baseline.PointVersion = nextPointVersion(state.PointVersion)
		state = baseline
		if err := s.Save(ctx, state); err != nil {
			return pricingFromState(previous), err
		}
	}
	s.putCache(state, now)
	return pricingFromState(state), nil
}

func (s *StateStore) cached(hash string, originalBid, minMargin float64, revision int64, now time.Time) (State, bool) {
	if s == nil || s.policy.ADVCacheTTL <= 0 {
		return State{}, false
	}
	s.mu.RLock()
	item, ok := s.cache[hash]
	s.mu.RUnlock()
	if !ok || now.Sub(item.loadedAt) > s.policy.ADVCacheTTL || !item.state.ValidFor(originalBid, minMargin, revision) {
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

func (s *StateStore) SaveIfCurrent(ctx context.Context, previous, next State) (bool, error) {
	if s == nil || s.redis == nil {
		return false, errors.New("percenter redis is unavailable")
	}
	previousRaw, err := json.Marshal(previous)
	if err != nil {
		return false, fmt.Errorf("encode previous percenter state %s: %w", previous.SegmentHash, err)
	}
	next.UpdatedAt = time.Now().UTC()
	nextRaw, err := json.Marshal(next)
	if err != nil {
		return false, fmt.Errorf("encode next percenter state %s: %w", next.SegmentHash, err)
	}
	ttlMS := s.policy.SegmentStateTTL.Milliseconds()
	if ttlMS <= 0 {
		ttlMS = (7 * 24 * time.Hour).Milliseconds()
	}
	result, err := compareAndSwapStateScript.Run(
		ctx,
		s.redis,
		[]string{SegmentKey(previous.SegmentHash)},
		string(previousRaw),
		string(nextRaw),
		fmt.Sprintf("%d", ttlMS),
	).Int()
	if err != nil {
		return false, fmt.Errorf("compare-and-swap percenter state %s: %w", previous.SegmentHash, err)
	}
	if result != 1 {
		return false, nil
	}
	s.putCache(next, next.UpdatedAt)
	return true, nil
}

func pricingFromState(state State) Pricing {
	return Pricing{AdvertiserPrice: state.AdvertiserPrice, SSPBid: state.SSPBid, Margin: state.Margin, Phase: state.Phase, PointVersion: state.PointVersion}
}

func CampaignVersionKey(campaignID string) string {
	return "percenter:campaign:" + strings.TrimSpace(campaignID) + ":version"
}

var ensureCampaignVersionScript = redis.NewScript(`
local current_fp = redis.call("HGET", KEYS[1], "fingerprint")
local current_version = tonumber(redis.call("HGET", KEYS[1], "version") or "0")
if not current_fp then
  redis.call("HSET", KEYS[1], "fingerprint", ARGV[1], "version", 1)
  return 1
end
if current_fp ~= ARGV[1] then
  local next_version = current_version + 1
  if next_version < 1 then next_version = 1 end
  redis.call("HSET", KEYS[1], "fingerprint", ARGV[1], "version", next_version)
  return next_version
end
if current_version < 1 then
  redis.call("HSET", KEYS[1], "version", 1)
  return 1
end
return current_version
`)

func campaignFingerprint(typeModel int, originalBid, minMargin float64) string {
	return fmt.Sprintf("%d|%.12g|%.12g", typeModel, originalBid, minMargin)
}

func (s *StateStore) EnsureCampaignVersion(ctx context.Context, campaignID string, typeModel int, originalBid, minMargin float64) (int64, error) {
	if s == nil || s.redis == nil {
		return 0, errors.New("percenter redis is unavailable")
	}
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return 0, errors.New("campaign id is empty")
	}
	fingerprint := campaignFingerprint(typeModel, originalBid, minMargin)
	version, err := ensureCampaignVersionScript.Run(ctx, s.redis, []string{CampaignVersionKey(campaignID)}, fingerprint).Int64()
	if err != nil {
		return 0, fmt.Errorf("ensure percenter campaign version %s: %w", campaignID, err)
	}
	if version < 1 {
		version = 1
	}
	return version, nil
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
