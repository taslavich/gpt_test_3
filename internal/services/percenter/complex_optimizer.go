package percenter

import (
	"math"
	"time"
)

type ComplexMetrics struct {
	ExactSegmentHash string
	SegmentHash      string
	PointVersion     uint64
	Requests         uint64
	Impressions      uint64
	Clicks           uint64
	AdvertiserSpend  float64
	TwinBidProfit    float64
}

func (m ComplexMetrics) Buyout() float64 {
	if m.Requests == 0 {
		return 0
	}
	return float64(m.Impressions) / float64(m.Requests)
}

func (m ComplexMetrics) Efficiency() float64 {
	if m.AdvertiserSpend <= 0 || math.IsNaN(m.AdvertiserSpend) || math.IsInf(m.AdvertiserSpend, 0) {
		return 0
	}
	return float64(m.Impressions) / m.AdvertiserSpend
}

func (m ComplexMetrics) ProfitPerRelevantOpportunity() float64 {
	if m.Requests == 0 {
		return 0
	}
	return m.TwinBidProfit / float64(m.Requests)
}

func AdvanceComplex(state ComplexState, metrics ComplexMetrics, policy ComplexPolicy, now time.Time) (ComplexState, bool) {
	policy = policy.Normalize()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if metrics.SegmentHash != state.SegmentHash || metrics.PointVersion != state.PointVersion {
		return state, false
	}
	if metrics.Impressions < policy.MinImpressions || metrics.Requests == 0 {
		return state, false
	}
	if !state.LastOptimizeAt.IsZero() && now.Sub(state.LastOptimizeAt) < policy.OptimizeInterval {
		return state, false
	}
	if !state.LastRebenchmarkAt.IsZero() && now.Sub(state.LastRebenchmarkAt) >= policy.RebenchmarkInterval {
		return RebenchmarkComplexState(state, policy, now, "scheduled_6h_rebenchmark"), true
	}

	old := state
	buyout := metrics.Buyout()
	efficiency := metrics.Efficiency()
	profit := metrics.ProfitPerRelevantOpportunity()

	switch state.Phase {
	case ComplexPhaseBenchmark, "":
		state.BaselineBuyout = buyout
		state.BaselineEfficiency = 0
		state.LastGoodSSPBid = state.SSPBid
		state.LastGoodMargin = state.EffectiveMin
		state.LastConfirmedProfit = profit
		state.SSPStepIndex = 0
		state.MarginStepIndex = 0
		state.AwaitingProbe = false
		state.SearchComplete = false
		state.LastOptimizeAt = now
		state.LastRebenchmarkAt = now
		state.Phase = ComplexPhaseSSPSearch
		if next, ok := nextComplexSSPProbe(state, policy); ok {
			state.SSPBid = next
			state.Margin = state.EffectiveMin
			state.AdvertiserPrice = advertiserPriceForMargin(state.SSPBid, state.Margin)
			state.PointVersion = nextPointVersion(state.PointVersion)
			state.UpdatedAt = now
			state.appendDecision(complexDecisionFrom(old, state, metrics, "benchmark_established_ssp_probe", buyout, efficiency, profit, profit, true, true, true, now))
			return state, true
		}
		state = enterComplexMarginBaseline(state, policy, now)
		state.appendDecision(complexDecisionFrom(old, state, metrics, "benchmark_established_no_ssp_probe", buyout, efficiency, profit, profit, true, true, true, now))
		return state, true

	case ComplexPhaseSSPSearch:
		if state.AwaitingProbe {
			state.LastOptimizeAt = now
			state.AwaitingProbe = false
			if next, ok := nextComplexSSPProbe(state, policy); ok {
				state.SSPBid = next
				state.Margin = state.EffectiveMin
				state.AdvertiserPrice = advertiserPriceForMargin(state.SSPBid, state.Margin)
				state.PointVersion = nextPointVersion(state.PointVersion)
				state.UpdatedAt = now
				state.appendDecision(complexDecisionFrom(old, state, metrics, "ssp_smaller_step_probe", buyout, efficiency, profit, state.LastConfirmedProfit, true, true, false, now))
				return state, true
			}
			state = enterComplexMarginBaseline(state, policy, now)
			state.appendDecision(complexDecisionFrom(old, state, metrics, "ssp_search_exhausted_margin_baseline", buyout, efficiency, profit, state.LastConfirmedProfit, true, true, false, now))
			return state, true
		}

		guardOK := state.BaselineBuyout > 0 && buyout+1e-12 >= policy.BuyoutRetention*state.BaselineBuyout
		profitOK := profit > state.LastConfirmedProfit+1e-12
		state.LastOptimizeAt = now
		if guardOK {
			previousProfit := state.LastConfirmedProfit
			state.LastGoodSSPBid = state.SSPBid
			state.LastConfirmedProfit = profit
			if next, ok := nextComplexSSPProbe(state, policy); ok {
				state.SSPBid = next
				state.Margin = state.EffectiveMin
				state.AdvertiserPrice = advertiserPriceForMargin(state.SSPBid, state.Margin)
				state.PointVersion = nextPointVersion(state.PointVersion)
				state.UpdatedAt = now
				state.appendDecision(complexDecisionFrom(old, state, metrics, "ssp_candidate_accepted_probe", buyout, efficiency, profit, previousProfit, guardOK, true, profitOK, now))
				return state, true
			}
			state = enterComplexMarginBaseline(state, policy, now)
			state.appendDecision(complexDecisionFrom(old, state, metrics, "ssp_candidate_accepted_margin_baseline", buyout, efficiency, profit, previousProfit, guardOK, true, profitOK, now))
			return state, true
		}

		state.SSPBid = state.LastGoodSSPBid
		state.Margin = state.EffectiveMin
		state.AdvertiserPrice = advertiserPriceForMargin(state.SSPBid, state.Margin)
		if state.SSPStepIndex < len(policy.SSPSearchStepsPercent)-1 {
			state.SSPStepIndex++
			state.AwaitingProbe = true
			state.PointVersion = nextPointVersion(state.PointVersion)
			state.UpdatedAt = now
			reason := "ssp_buyout_guard_rollback"
			state.appendDecision(complexDecisionFrom(old, state, metrics, reason, buyout, efficiency, profit, old.LastConfirmedProfit, guardOK, true, profitOK, now))
			return state, true
		}
		state = enterComplexMarginBaseline(state, policy, now)
		reason := "ssp_final_buyout_rollback_margin_baseline"
		state.appendDecision(complexDecisionFrom(old, state, metrics, reason, buyout, efficiency, profit, old.LastConfirmedProfit, guardOK, true, profitOK, now))
		return state, true

	case ComplexPhaseMarginBaseline:
		state.BaselineEfficiency = efficiency
		state.LastGoodMargin = clampMargin(state.Margin, state.EffectiveMin, policy.MaxMargin)
		state.LastConfirmedProfit = profit
		state.MarginStepIndex = 0
		state.AwaitingProbe = false
		state.SearchComplete = false
		state.LastOptimizeAt = now
		state.Phase = ComplexPhaseMarginSearch
		if next, ok := nextComplexMarginProbe(state, policy); ok {
			state.Margin = next
			state.AdvertiserPrice = advertiserPriceForMargin(state.LastGoodSSPBid, state.Margin)
			state.SSPBid = state.LastGoodSSPBid
			state.PointVersion = nextPointVersion(state.PointVersion)
			state.UpdatedAt = now
			state.appendDecision(complexDecisionFrom(old, state, metrics, "margin_baseline_established_probe", buyout, efficiency, profit, profit, true, true, true, now))
			return state, true
		}
		state.SearchComplete = true
		state.UpdatedAt = now
		state.appendDecision(complexDecisionFrom(old, state, metrics, "margin_baseline_at_cap", buyout, efficiency, profit, profit, true, true, true, now))
		return state, true

	case ComplexPhaseMarginSearch:
		if state.SearchComplete {
			return state, false
		}
		if state.AwaitingProbe {
			state.LastOptimizeAt = now
			state.AwaitingProbe = false
			if next, ok := nextComplexMarginProbe(state, policy); ok {
				state.Margin = next
				state.AdvertiserPrice = advertiserPriceForMargin(state.LastGoodSSPBid, state.Margin)
				state.SSPBid = state.LastGoodSSPBid
				state.PointVersion = nextPointVersion(state.PointVersion)
				state.UpdatedAt = now
				state.appendDecision(complexDecisionFrom(old, state, metrics, "margin_smaller_step_probe", buyout, efficiency, profit, state.LastConfirmedProfit, true, true, false, now))
				return state, true
			}
			state.SearchComplete = true
			state.UpdatedAt = now
			state.appendDecision(complexDecisionFrom(old, state, metrics, "margin_search_exhausted", buyout, efficiency, profit, state.LastConfirmedProfit, true, true, false, now))
			return state, true
		}

		efficiencyOK := state.BaselineEfficiency > 0 && efficiency+1e-12 >= policy.EfficiencyRetention*state.BaselineEfficiency
		profitOK := profit > state.LastConfirmedProfit+1e-12
		state.LastOptimizeAt = now
		if efficiencyOK && profitOK {
			previousProfit := state.LastConfirmedProfit
			state.LastGoodMargin = state.Margin
			state.LastConfirmedProfit = profit
			if next, ok := nextComplexMarginProbe(state, policy); ok {
				state.Margin = next
				state.AdvertiserPrice = advertiserPriceForMargin(state.LastGoodSSPBid, state.Margin)
				state.SSPBid = state.LastGoodSSPBid
				state.PointVersion = nextPointVersion(state.PointVersion)
				state.UpdatedAt = now
				state.appendDecision(complexDecisionFrom(old, state, metrics, "margin_candidate_accepted_probe", buyout, efficiency, profit, previousProfit, true, efficiencyOK, profitOK, now))
				return state, true
			}
			state.SearchComplete = true
			state.UpdatedAt = now
			state.appendDecision(complexDecisionFrom(old, state, metrics, "margin_candidate_accepted_at_cap", buyout, efficiency, profit, previousProfit, true, efficiencyOK, profitOK, now))
			return state, true
		}

		state.Margin = clampMargin(state.LastGoodMargin, state.EffectiveMin, policy.MaxMargin)
		state.SSPBid = state.LastGoodSSPBid
		state.AdvertiserPrice = advertiserPriceForMargin(state.SSPBid, state.Margin)
		if state.MarginStepIndex < len(policy.MarginSearchStepsPP)-1 {
			state.MarginStepIndex++
			state.AwaitingProbe = true
		} else {
			state.SearchComplete = true
			state.AwaitingProbe = false
		}
		state.PointVersion = nextPointVersion(state.PointVersion)
		state.UpdatedAt = now
		reason := "margin_profit_regressed_rollback"
		if !efficiencyOK {
			reason = "margin_efficiency_guard_rollback"
		}
		state.appendDecision(complexDecisionFrom(old, state, metrics, reason, buyout, efficiency, profit, old.LastConfirmedProfit, true, efficiencyOK, profitOK, now))
		return state, true

	default:
		return RebenchmarkComplexState(state, policy, now, "invalid_phase_rebenchmark"), true
	}
}

func nextComplexSSPProbe(state ComplexState, policy ComplexPolicy) (float64, bool) {
	policy = policy.Normalize()
	idx := state.SSPStepIndex
	if idx < 0 || idx >= len(policy.SSPSearchStepsPercent) || !finiteComplexValue(state.LastGoodSSPBid) {
		return 0, false
	}
	step := policy.SSPSearchStepsPercent[idx] / 100.0
	candidate := state.LastGoodSSPBid * (1 - step)
	if !finiteComplexValue(candidate) || math.Abs(candidate-state.LastGoodSSPBid) <= 1e-12 {
		return 0, false
	}
	return candidate, true
}

func nextComplexMarginProbe(state ComplexState, policy ComplexPolicy) (float64, bool) {
	policy = policy.Normalize()
	idx := state.MarginStepIndex
	if idx < 0 || idx >= len(policy.MarginSearchStepsPP) {
		return 0, false
	}
	base := clampMargin(state.LastGoodMargin, state.EffectiveMin, policy.MaxMargin)
	candidate := base + policy.MarginSearchStepsPP[idx]/100.0
	candidate = clampMargin(candidate, state.EffectiveMin, policy.MaxMargin)
	if math.Abs(candidate-base) <= 1e-12 {
		return 0, false
	}
	return candidate, true
}

func enterComplexMarginBaseline(state ComplexState, policy ComplexPolicy, now time.Time) ComplexState {
	policy = policy.Normalize()
	state.Phase = ComplexPhaseMarginBaseline
	state.SSPBid = state.LastGoodSSPBid
	state.Margin = clampMargin(state.EffectiveMin, 0, policy.MaxMargin)
	state.AdvertiserPrice = advertiserPriceForMargin(state.SSPBid, state.Margin)
	state.BaselineEfficiency = 0
	state.LastGoodMargin = state.Margin
	state.MarginStepIndex = 0
	state.AwaitingProbe = false
	state.SearchComplete = false
	state.PointVersion = nextPointVersion(state.PointVersion)
	state.LastOptimizeAt = now
	state.UpdatedAt = now
	return state
}

func complexDecisionFrom(old, next ComplexState, metrics ComplexMetrics, reason string, buyout, efficiency, actualProfit, targetProfit float64, buyoutOK, efficiencyOK, profitOK bool, now time.Time) ComplexDecision {
	return ComplexDecision{
		At:                         now,
		Phase:                      next.Phase,
		Reason:                     reason,
		Requests:                   metrics.Requests,
		Impressions:                metrics.Impressions,
		Wins:                       metrics.Impressions,
		ObservedBuyout:             buyout,
		BaselineBuyout:             next.BaselineBuyout,
		AdvertiserSpend:            metrics.AdvertiserSpend,
		ObservedEfficiency:         efficiency,
		BaselineEfficiency:         next.BaselineEfficiency,
		TargetProfitPerOpportunity: targetProfit,
		ActualProfitPerOpportunity: actualProfit,
		BuyoutThresholdPassed:      buyoutOK,
		EfficiencyThresholdPassed:  efficiencyOK,
		ProfitImproved:             profitOK,
		OldSSPBid:                  old.SSPBid,
		NewSSPBid:                  next.SSPBid,
		OldMargin:                  old.Margin,
		NewMargin:                  next.Margin,
		OldAdvertiserPrice:         old.AdvertiserPrice,
		NewAdvertiserPrice:         next.AdvertiserPrice,
		OldPointVersion:            old.PointVersion,
		NewPointVersion:            next.PointVersion,
	}
}
