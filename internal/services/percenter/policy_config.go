package percenter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

const DefaultPendingHistoryTTL = 30 * 24 * time.Hour

// PoliciesFromConfig is the single production builder/validator used by both
// ADV and the percenter daemon. Fixed Stage 02/03 business constants are
// validated on the raw config BEFORE any Normalize call can replace an
// explicitly invalid value with a default. State TTLs and history retention are
// configurable, but must be positive.
func PoliciesFromConfig(cfg config.PercenterPolicyConfig) (SimplePolicy, ComplexPolicy, error) {
	simpleSteps, err := parsePolicySteps(cfg.SimpleMarginSearchSteps)
	if err != nil {
		return SimplePolicy{}, ComplexPolicy{}, fmt.Errorf(
			"SIMPLE_MARGIN_SEARCH_STEPS=%q is invalid: production invariant requires 5,2,1: %w",
			cfg.SimpleMarginSearchSteps, err,
		)
	}
	complexSSPSteps, err := parsePolicySteps(cfg.ComplexSSPSearchSteps)
	if err != nil {
		return SimplePolicy{}, ComplexPolicy{}, fmt.Errorf(
			"COMPLEX_SSP_SEARCH_STEPS=%q is invalid: production invariant requires 10,5,2,1: %w",
			cfg.ComplexSSPSearchSteps, err,
		)
	}
	complexMarginSteps, err := parsePolicySteps(cfg.ComplexMarginSearchSteps)
	if err != nil {
		return SimplePolicy{}, ComplexPolicy{}, fmt.Errorf(
			"COMPLEX_MARGIN_SEARCH_STEPS=%q is invalid: production invariant requires 10,5,2,1: %w",
			cfg.ComplexMarginSearchSteps, err,
		)
	}

	if err := validateRawPolicyConfig(cfg, simpleSteps, complexSSPSteps, complexMarginSteps); err != nil {
		return SimplePolicy{}, ComplexPolicy{}, err
	}

	simple := SimplePolicy{
		WinRateRetention: cfg.SimpleWinRateRetention, MinImpressions: cfg.SimpleMinImpressions,
		OptimizeInterval: cfg.SimpleOptimizeInterval, RebenchmarkInterval: cfg.SimpleRebenchmarkInterval,
		SearchStepsPP: append([]float64(nil), simpleSteps...), MaxMargin: cfg.SimpleMaxMargin,
		StateTTL: cfg.SimpleStateTTL, PendingHistoryTTL: cfg.HistoryPendingTTL,
	}
	complex := ComplexPolicy{
		BuyoutRetention: cfg.ComplexBuyoutRetention, EfficiencyRetention: cfg.ComplexEfficiencyRetention,
		MinImpressions: cfg.ComplexMinImpressions, OptimizeInterval: cfg.ComplexOptimizeInterval,
		RebenchmarkInterval:   cfg.ComplexRebenchmarkInterval,
		SSPSearchStepsPercent: append([]float64(nil), complexSSPSteps...), MarginSearchStepsPP: append([]float64(nil), complexMarginSteps...),
		MaxMargin: cfg.ComplexMaxMargin, StateTTL: cfg.ComplexStateTTL, PendingHistoryTTL: cfg.HistoryPendingTTL,
	}
	return simple, complex, nil
}

func validateRawPolicyConfig(cfg config.PercenterPolicyConfig, simpleSteps, complexSSPSteps, complexMarginSteps []float64) error {
	if !policyStepsEqual(simpleSteps, []float64{5, 2, 1}) {
		return fmt.Errorf("SIMPLE_MARGIN_SEARCH_STEPS=%q is invalid: production invariant requires 5,2,1", cfg.SimpleMarginSearchSteps)
	}
	if cfg.SimpleWinRateRetention != .50 {
		return fmt.Errorf("SIMPLE_WIN_RATE_RETENTION=%v is invalid: production invariant requires 0.5", cfg.SimpleWinRateRetention)
	}
	if cfg.SimpleMinImpressions != 5 {
		return fmt.Errorf("SIMPLE_MIN_IMPRESSIONS=%d is invalid: production invariant requires 5", cfg.SimpleMinImpressions)
	}
	if cfg.SimpleOptimizeInterval != 5*time.Minute {
		return fmt.Errorf("SIMPLE_OPTIMIZE_INTERVAL=%s is invalid: production invariant requires 5m", cfg.SimpleOptimizeInterval)
	}
	if cfg.SimpleRebenchmarkInterval != 6*time.Hour {
		return fmt.Errorf("SIMPLE_REBENCHMARK_INTERVAL=%s is invalid: production invariant requires 6h", cfg.SimpleRebenchmarkInterval)
	}
	if cfg.SimpleMaxMargin != .90 {
		return fmt.Errorf("SIMPLE_MAX_MARGIN=%v is invalid: production invariant requires 0.9", cfg.SimpleMaxMargin)
	}
	if cfg.SimpleStateTTL <= 0 {
		return fmt.Errorf("SIMPLE_STATE_TTL=%s is invalid: value must be >0", cfg.SimpleStateTTL)
	}

	if !policyStepsEqual(complexSSPSteps, []float64{10, 5, 2, 1}) {
		return fmt.Errorf("COMPLEX_SSP_SEARCH_STEPS=%q is invalid: production invariant requires 10,5,2,1", cfg.ComplexSSPSearchSteps)
	}
	if !policyStepsEqual(complexMarginSteps, []float64{10, 5, 2, 1}) {
		return fmt.Errorf("COMPLEX_MARGIN_SEARCH_STEPS=%q is invalid: production invariant requires 10,5,2,1", cfg.ComplexMarginSearchSteps)
	}
	if cfg.ComplexBuyoutRetention != .80 {
		return fmt.Errorf("COMPLEX_BUYOUT_RETENTION=%v is invalid: production invariant requires 0.8", cfg.ComplexBuyoutRetention)
	}
	if cfg.ComplexEfficiencyRetention != .80 {
		return fmt.Errorf("COMPLEX_EFFICIENCY_RETENTION=%v is invalid: production invariant requires 0.8", cfg.ComplexEfficiencyRetention)
	}
	if cfg.ComplexMinImpressions != 5 {
		return fmt.Errorf("COMPLEX_MIN_IMPRESSIONS=%d is invalid: production invariant requires 5", cfg.ComplexMinImpressions)
	}
	if cfg.ComplexOptimizeInterval != 5*time.Minute {
		return fmt.Errorf("COMPLEX_OPTIMIZE_INTERVAL=%s is invalid: production invariant requires 5m", cfg.ComplexOptimizeInterval)
	}
	if cfg.ComplexRebenchmarkInterval != 6*time.Hour {
		return fmt.Errorf("COMPLEX_REBENCHMARK_INTERVAL=%s is invalid: production invariant requires 6h", cfg.ComplexRebenchmarkInterval)
	}
	if cfg.ComplexMaxMargin != .90 {
		return fmt.Errorf("COMPLEX_MAX_MARGIN=%v is invalid: production invariant requires 0.9", cfg.ComplexMaxMargin)
	}
	if cfg.ComplexStateTTL <= 0 {
		return fmt.Errorf("COMPLEX_STATE_TTL=%s is invalid: value must be >0", cfg.ComplexStateTTL)
	}
	if cfg.HistoryPendingTTL <= 0 {
		return fmt.Errorf("HISTORY_PENDING_TTL=%s is invalid: value must be >0 and is independent from SIMPLE_STATE_TTL/COMPLEX_STATE_TTL", cfg.HistoryPendingTTL)
	}
	return nil
}

// PolicyFingerprint is diagnostic-only. Equal normalized policies produce the
// same digest regardless of ENV string formatting (for example spaces in step
// lists), because it hashes canonical typed values in fixed field order.
func PolicyFingerprint(simple SimplePolicy, complex ComplexPolicy) string {
	canonical := strings.Join([]string{
		simple.OptimizeInterval.String(), simple.RebenchmarkInterval.String(), simple.StateTTL.String(), simple.PendingHistoryTTL.String(),
		strconv.FormatUint(simple.MinImpressions, 10), strconv.FormatFloat(simple.WinRateRetention, 'g', -1, 64), formatPolicySteps(simple.SearchStepsPP), strconv.FormatFloat(simple.MaxMargin, 'g', -1, 64),
		complex.OptimizeInterval.String(), complex.RebenchmarkInterval.String(), complex.StateTTL.String(), complex.PendingHistoryTTL.String(),
		strconv.FormatUint(complex.MinImpressions, 10), strconv.FormatFloat(complex.BuyoutRetention, 'g', -1, 64), strconv.FormatFloat(complex.EfficiencyRetention, 'g', -1, 64),
		formatPolicySteps(complex.SSPSearchStepsPercent), formatPolicySteps(complex.MarginSearchStepsPP), strconv.FormatFloat(complex.MaxMargin, 'g', -1, 64),
	}, "|")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func formatPolicySteps(steps []float64) string {
	parts := make([]string, len(steps))
	for i, v := range steps {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	return strings.Join(parts, ",")
}

func parsePolicySteps(raw string) ([]float64, error) {
	parts := strings.Split(raw, ",")
	steps := make([]float64, 0, len(parts))
	for _, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("invalid percenter search steps %q: expected a comma-separated list of positive numbers", raw)
		}
		steps = append(steps, v)
	}
	return steps, nil
}

func policyStepsEqual(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
