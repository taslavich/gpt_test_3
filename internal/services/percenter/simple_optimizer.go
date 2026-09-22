package percenter

import (
	"math"
	"time"
)

type SimpleMetrics struct {
	SegmentHash   string
	PointVersion  uint64
	Requests      uint64
	Impressions   uint64
	TwinBidProfit float64
}

func (m SimpleMetrics) WinRate() float64 {
	if m.Requests == 0 {
		return 0
	}
	return float64(m.Impressions) / float64(m.Requests)
}

func (m SimpleMetrics) ProfitPerRequest() float64 {
	if m.Requests == 0 {
		return 0
	}
	return m.TwinBidProfit / float64(m.Requests)
}

func AdvanceSimple(state SimpleState, metrics SimpleMetrics, policy SimplePolicy, now time.Time) (SimpleState, bool) {
	policy = policy.Normalize()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if metrics.PointVersion != state.PointVersion || metrics.SegmentHash != state.SegmentHash {
		return state, false
	}
	if metrics.Impressions < policy.MinImpressions || metrics.Requests == 0 {
		return state, false
	}
	if !state.LastOptimizeAt.IsZero() && now.Sub(state.LastOptimizeAt) < policy.OptimizeInterval {
		return state, false
	}
	if !state.LastRebenchmarkAt.IsZero() && now.Sub(state.LastRebenchmarkAt) >= policy.RebenchmarkInterval {
		return RebenchmarkSimpleState(state, policy, now, "scheduled_6h_rebenchmark"), true
	}

	old := state
	winRate := metrics.WinRate()
	profit := metrics.ProfitPerRequest()
	state.LastRequests = metrics.Requests
	state.LastImpressions = metrics.Impressions

	switch state.Phase {
	case SimplePhaseBaseline, "":
		state.BaselineWinRate = winRate
		state.LastConfirmedMargin = clampMargin(state.Margin, state.EffectiveMin, policy.MaxMargin)
		state.LastConfirmedProfit = profit
		state.StepIndex = 0
		state.AwaitingProbe = false
		state.LastRebenchmarkAt = now
		state.LastOptimizeAt = now
		state.Phase = SimplePhaseSearch
		if next, ok := nextSimpleProbe(state, policy); ok {
			state.Margin = next
			state.SSPBid = priceAfterMargin(state.ReferenceOriginalBid, state.Margin)
			state.PointVersion = nextPointVersion(state.PointVersion)
			state.UpdatedAt = now
			state.appendDecision(decisionFrom(old, state, metrics, "baseline_established_probe", winRate, profit, state.LastConfirmedProfit, now))
			return state, true
		}
		state.Phase = SimplePhaseSettled
		state.UpdatedAt = now
		state.appendDecision(decisionFrom(old, state, metrics, "baseline_established_at_cap", winRate, profit, state.LastConfirmedProfit, now))
		return state, true

	case SimplePhaseSearch:
		if state.AwaitingProbe {
			state.LastOptimizeAt = now
			state.AwaitingProbe = false
			if next, ok := nextSimpleProbe(state, policy); ok {
				state.Margin = next
				state.SSPBid = priceAfterMargin(state.ReferenceOriginalBid, state.Margin)
				state.PointVersion = nextPointVersion(state.PointVersion)
				state.UpdatedAt = now
				state.appendDecision(decisionFrom(old, state, metrics, "smaller_step_probe", winRate, profit, state.LastConfirmedProfit, now))
				return state, true
			}
			state.Phase = SimplePhaseSettled
			state.Margin = state.LastConfirmedMargin
			state.SSPBid = priceAfterMargin(state.ReferenceOriginalBid, state.Margin)
			state.PointVersion = nextPointVersion(state.PointVersion)
			state.UpdatedAt = now
			state.appendDecision(decisionFrom(old, state, metrics, "no_legal_probe_settled", winRate, profit, state.LastConfirmedProfit, now))
			return state, true
		}

		guardOK := state.BaselineWinRate > 0 && winRate >= policy.WinRateRetention*state.BaselineWinRate
		profitOK := profit > state.LastConfirmedProfit+1e-12
		state.LastOptimizeAt = now
		if guardOK && profitOK {
			// Preserve the target that was in force while this probe was evaluated.
			// History compares the accepted probe to the previous confirmed point.
			previousConfirmedProfit := state.LastConfirmedProfit
			state.LastConfirmedMargin = state.Margin
			state.LastConfirmedProfit = profit
			if next, ok := nextSimpleProbe(state, policy); ok {
				state.Margin = next
				state.SSPBid = priceAfterMargin(state.ReferenceOriginalBid, state.Margin)
				state.PointVersion = nextPointVersion(state.PointVersion)
				state.UpdatedAt = now
				state.appendDecision(decisionFrom(old, state, metrics, "candidate_accepted_probe", winRate, profit, previousConfirmedProfit, now))
				return state, true
			}
			state.Phase = SimplePhaseSettled
			state.UpdatedAt = now
			state.appendDecision(decisionFrom(old, state, metrics, "candidate_accepted_at_cap", winRate, profit, previousConfirmedProfit, now))
			return state, true
		}

		// Guardrail/profit failure: return to the last confirmed point first.
		// The following 5m decision will probe the next smaller step.
		state.Margin = clampMargin(state.LastConfirmedMargin, state.EffectiveMin, policy.MaxMargin)
		state.SSPBid = priceAfterMargin(state.ReferenceOriginalBid, state.Margin)
		if state.StepIndex < len(policy.SearchStepsPP)-1 {
			state.StepIndex++
			state.AwaitingProbe = true
			state.Phase = SimplePhaseSearch
		} else {
			state.AwaitingProbe = false
			state.Phase = SimplePhaseSettled
		}
		state.PointVersion = nextPointVersion(state.PointVersion)
		state.UpdatedAt = now
		reason := "profit_regressed_rollback"
		if !guardOK {
			reason = "winrate_guard_rollback"
		}
		state.appendDecision(decisionFrom(old, state, metrics, reason, winRate, profit, old.LastConfirmedProfit, now))
		return state, true

	case SimplePhaseSettled:
		state.LastOptimizeAt = now
		return state, false
	default:
		return RebenchmarkSimpleState(state, policy, now, "invalid_phase_rebenchmark"), true
	}
}

func nextSimpleProbe(state SimpleState, policy SimplePolicy) (float64, bool) {
	policy = policy.Normalize()
	idx := state.StepIndex
	if idx < 0 || idx >= len(policy.SearchStepsPP) {
		return 0, false
	}
	base := state.LastConfirmedMargin
	if base < state.EffectiveMin {
		base = state.EffectiveMin
	}
	candidate := base + policy.SearchStepsPP[idx]/100.0
	if candidate > policy.MaxMargin+1e-12 {
		return 0, false
	}
	candidate = clampMargin(candidate, state.EffectiveMin, policy.MaxMargin)
	if math.Abs(candidate-base) <= 1e-12 {
		return 0, false
	}
	return candidate, true
}

func decisionFrom(old, next SimpleState, metrics SimpleMetrics, reason string, observedWinRate, actualProfit, targetProfit float64, now time.Time) SimpleDecision {
	return SimpleDecision{
		At:                 now,
		Reason:             reason,
		BaselineWinRate:    next.BaselineWinRate,
		ObservedWinRate:    observedWinRate,
		TargetProfitPerReq: targetProfit,
		ActualProfitPerReq: actualProfit,
		OldMargin:          old.Margin,
		NewMargin:          next.Margin,
		OldSSPBid:          old.SSPBid,
		NewSSPBid:          next.SSPBid,
		Impressions:        metrics.Impressions,
		Requests:           metrics.Requests,
		OldPointVersion:    old.PointVersion,
		NewPointVersion:    next.PointVersion,
	}
}
