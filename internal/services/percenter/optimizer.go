package percenter

import (
	"math"
	"time"
)

type Metrics struct {
	SegmentHash        string
	PointVersion       uint64
	Requests           uint64
	Wins               uint64
	Clicks             uint64
	AdvertiserSpend    float64
	TwinBidProfit      float64
	ClickTwinBidProfit float64
}

func (m Metrics) Buyout() float64 {
	if m.Requests == 0 {
		return 0
	}
	return float64(m.Wins) / float64(m.Requests)
}

func (m Metrics) Efficiency() float64 {
	if m.AdvertiserSpend <= 0 || m.Wins == 0 {
		return 0
	}
	return float64(m.Wins) / m.AdvertiserSpend
}

func (m Metrics) ProfitPerRequest() float64 {
	return m.ProfitPerRequestFor(ProfitModelImpression)
}

func (m Metrics) ProfitPerRequestFor(profitModel string) float64 {
	if m.Requests == 0 {
		return 0
	}
	profit := m.TwinBidProfit
	if normalizeProfitModel(profitModel) == ProfitModelClick {
		profit = m.ClickTwinBidProfit
	}
	return profit / float64(m.Requests)
}

func Advance(state State, metrics Metrics, policy Policy, now time.Time) (State, bool) {
	policy = policy.Normalize()
	if metrics.Requests == 0 {
		return state, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if !state.LastChangeAt.IsZero() && now.Sub(state.LastChangeAt) < policy.MarginOptimizeInterval {
		return state, false
	}

	if normalizeTypeModel(state.TypeModel) == TypeModelSimple {
		if state.simpleBaselineReoptimizationDue(now, policy.SimpleBaselineReoptimizeInterval) {
			reset := resetState(state, now)
			return reset, true
		}
		return advanceSimple(state, metrics, policy, now)
	}

	if !state.LastSSPReoptimizeAt.IsZero() && now.Sub(state.LastSSPReoptimizeAt) >= policy.SSPReoptimizeInterval {
		reset := resetState(state, now)
		return reset, true
	}

	switch state.Phase {
	case PhaseBenchmark, "":
		buyout := metrics.Buyout()
		if buyout <= 0 {
			return state, false
		}
		state.BenchmarkBuyout = buyout
		state.SSPLow = 0
		state.SSPHigh = state.SSPBid
		candidate := (state.SSPLow + state.SSPHigh) / 2
		applySSPTestPoint(&state, candidate, now)
		state.Phase = PhaseSSPSearch
		return state, true

	case PhaseSSPSearch:
		if state.BenchmarkBuyout <= 0 || state.SSPHigh <= 0 {
			reset := resetState(state, now)
			return reset, true
		}
		retention := metrics.Buyout() / state.BenchmarkBuyout
		if retention >= policy.BuyoutRetention {
			state.SSPHigh = state.SSPBid
		} else {
			state.SSPLow = state.SSPBid
		}
		if state.SSPHigh <= 0 || (state.SSPHigh-state.SSPLow)/state.SSPHigh <= policy.SSPSearchPrecision {
			applySSPTestPoint(&state, state.SSPHigh, now)
			state.Margin = marginFor(state.AdvertiserPrice, state.SSPBid)
			state.Phase = PhaseMarginBaseline
			state.LastSSPReoptimizeAt = now
			return state, true
		}
		candidate := (state.SSPLow + state.SSPHigh) / 2
		applySSPTestPoint(&state, candidate, now)
		return state, true

	case PhaseMarginBaseline:
		efficiency := metrics.Efficiency()
		if efficiency <= 0 {
			return state, false
		}
		state.BaselineEfficiency = efficiency
		state.BestProfitPerReq = metrics.ProfitPerRequest()
		state.BestMargin = state.Margin
		state.BestAdvertiserPrice = state.AdvertiserPrice
		state.MarginStepIndex = 0
		state.MarginDirection = 1
		if next, ok := nextMargin(state, policy, true); ok {
			applyMarginPoint(&state, next, now)
			state.Phase = PhaseMarginSearch
			return state, true
		}
		return state, false

	case PhaseMarginSearch:
		validEfficiency := state.BaselineEfficiency > 0 && metrics.Efficiency()/state.BaselineEfficiency >= policy.EfficiencyRetention
		profit := metrics.ProfitPerRequest()
		improved := validEfficiency && profit > state.BestProfitPerReq
		if improved {
			state.BestProfitPerReq = profit
			state.BestMargin = state.Margin
			state.BestAdvertiserPrice = state.AdvertiserPrice
			if next, ok := nextMargin(state, policy, true); ok {
				applyMarginPoint(&state, next, now)
				return state, true
			}
		}

		// Overshot the local maximum or violated the advertiser-efficiency guard.
		// Reduce the search step around the best known point; at the final 1pp
		// step keep alternating around the optimum so the percentage never freezes.
		if state.MarginStepIndex < len(policy.MarginSearchStepsPP)-1 {
			state.MarginStepIndex++
			if next, ok := nextMarginFromBest(state, policy, state.MarginDirection); ok {
				applyMarginPoint(&state, next, now)
				return state, true
			}
		}
		state.MarginDirection *= -1
		if state.MarginDirection == 0 {
			state.MarginDirection = -1
		}
		if next, ok := nextMarginFromBest(state, policy, state.MarginDirection); ok {
			applyMarginPoint(&state, next, now)
			return state, true
		}
		// No neighbour is feasible: return to the best known legal point.
		applyMarginPoint(&state, state.BestMargin, now)
		return state, true
	default:
		reset := resetState(state, now)
		return reset, true
	}
}

func resetState(state State, now time.Time) State {
	reset := BaselineStateForCampaign(
		state.SegmentHash,
		state.CampaignID,
		state.OriginalBid,
		state.MinMargin,
		state.CampaignVersion,
		state.TypeModel,
		state.ProfitModel,
		now,
	)
	reset.PointVersion = nextPointVersion(state.PointVersion)
	return reset
}

func advanceSimple(state State, metrics Metrics, policy Policy, now time.Time) (State, bool) {
	switch state.Phase {
	case PhaseSimpleBaseline, "":
		buyout := metrics.Buyout()
		if buyout <= 0 {
			return state, false
		}
		state.BenchmarkBuyout = buyout
		state.BestProfitPerReq = metrics.ProfitPerRequestFor(state.ProfitModel)
		state.BestMargin = state.MinMargin
		state.BestAdvertiserPrice = state.OriginalBid
		state.MarginStepIndex = 0
		state.MarginDirection = 1
		state.LastSimpleBaselineAt = now
		if next, ok := nextSimpleMarginFromBest(state, policy, 1); ok {
			applySimpleMarginPoint(&state, next, now)
			state.Phase = PhaseSimpleMarginSearch
			return state, true
		}
		return state, false

	case PhaseSimpleMarginSearch:
		validWinRate := state.BenchmarkBuyout > 0 && metrics.Buyout()/state.BenchmarkBuyout >= policy.SimpleWinRateRetention
		profit := metrics.ProfitPerRequestFor(state.ProfitModel)
		improved := validWinRate && profit > state.BestProfitPerReq
		if improved {
			state.BestProfitPerReq = profit
			state.BestMargin = state.Margin
			state.BestAdvertiserPrice = state.OriginalBid
			if next, ok := nextSimpleMarginFromBest(state, policy, state.MarginDirection); ok {
				applySimpleMarginPoint(&state, next, now)
				return state, true
			}
		}

		if state.MarginStepIndex < len(policy.MarginSearchStepsPP)-1 {
			state.MarginStepIndex++
			if next, ok := nextSimpleMarginFromBest(state, policy, state.MarginDirection); ok {
				applySimpleMarginPoint(&state, next, now)
				return state, true
			}
		}
		state.MarginDirection *= -1
		if state.MarginDirection == 0 {
			state.MarginDirection = -1
		}
		if next, ok := nextSimpleMarginFromBest(state, policy, state.MarginDirection); ok {
			applySimpleMarginPoint(&state, next, now)
			return state, true
		}
		applySimpleMarginPoint(&state, state.BestMargin, now)
		return state, true

	default:
		reset := resetState(state, now)
		return reset, true
	}
}

func applySimpleMarginPoint(state *State, margin float64, now time.Time) {
	if state == nil {
		return
	}
	margin = math.Max(state.MinMargin, math.Min(margin, 0.999999))
	state.PointVersion = nextPointVersion(state.PointVersion)
	state.AdvertiserPrice = state.OriginalBid
	state.Margin = margin
	state.SSPBid = state.OriginalBid * (1 - margin)
	state.LastChangeAt = now
}

func nextSimpleMarginFromBest(state State, policy Policy, direction int) (float64, bool) {
	if len(policy.MarginSearchStepsPP) == 0 {
		return 0, false
	}
	idx := state.MarginStepIndex
	if idx < 0 {
		idx = 0
	}
	if idx >= len(policy.MarginSearchStepsPP) {
		idx = len(policy.MarginSearchStepsPP) - 1
	}
	if direction == 0 {
		direction = 1
	}
	step := policy.MarginSearchStepsPP[idx] / 100
	base := state.BestMargin
	if base <= 0 {
		base = state.Margin
	}
	candidate := base + float64(direction)*step
	maxMargin := math.Max(state.MinMargin, policy.MaxMargin)
	if candidate < state.MinMargin-1e-12 || candidate > maxMargin+1e-12 {
		return 0, false
	}
	return math.Max(state.MinMargin, math.Min(candidate, maxMargin)), true
}

func applySSPTestPoint(state *State, ssp float64, now time.Time) {
	if state == nil {
		return
	}
	if ssp < 0 {
		ssp = 0
	}
	state.PointVersion = nextPointVersion(state.PointVersion)
	state.SSPBid = ssp
	state.AdvertiserPrice = ssp / (1 - state.MinMargin)
	if state.AdvertiserPrice > state.OriginalBid {
		state.AdvertiserPrice = state.OriginalBid
	}
	state.Margin = state.MinMargin
	state.LastChangeAt = now
}

func applyMarginPoint(state *State, margin float64, now time.Time) {
	if state == nil {
		return
	}
	margin = math.Max(state.MinMargin, math.Min(margin, 0.999999))
	state.PointVersion = nextPointVersion(state.PointVersion)
	state.Margin = margin
	state.AdvertiserPrice = state.SSPBid / (1 - margin)
	if state.AdvertiserPrice > state.OriginalBid {
		state.AdvertiserPrice = state.OriginalBid
		state.Margin = marginFor(state.AdvertiserPrice, state.SSPBid)
	}
	state.LastChangeAt = now
}

func nextMargin(state State, policy Policy, forward bool) (float64, bool) {
	direction := state.MarginDirection
	if !forward {
		direction *= -1
	}
	return nextMarginFromBest(state, policy, direction)
}

func nextMarginFromBest(state State, policy Policy, direction int) (float64, bool) {
	if len(policy.MarginSearchStepsPP) == 0 {
		return 0, false
	}
	idx := state.MarginStepIndex
	if idx < 0 {
		idx = 0
	}
	if idx >= len(policy.MarginSearchStepsPP) {
		idx = len(policy.MarginSearchStepsPP) - 1
	}
	step := policy.MarginSearchStepsPP[idx] / 100
	if direction == 0 {
		direction = 1
	}
	base := state.BestMargin
	if base <= 0 {
		base = state.Margin
	}
	candidate := base + float64(direction)*step
	maxFeasible := policy.MaxMargin
	if state.OriginalBid > 0 {
		byOriginalBid := 1 - state.SSPBid/state.OriginalBid
		if byOriginalBid < maxFeasible {
			maxFeasible = byOriginalBid
		}
	}
	maxFeasible = math.Max(state.MinMargin, maxFeasible)
	if candidate < state.MinMargin-1e-12 || candidate > maxFeasible+1e-12 {
		return 0, false
	}
	return math.Max(state.MinMargin, math.Min(candidate, maxFeasible)), true
}

func marginFor(advertiserPrice, sspBid float64) float64 {
	if advertiserPrice <= 0 {
		return 0
	}
	return 1 - sspBid/advertiserPrice
}
