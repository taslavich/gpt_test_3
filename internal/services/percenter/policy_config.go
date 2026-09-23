package percenter

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

// PoliciesFromConfig is the single production contract used by both ADV and
// the percenter daemon. Keeping policy construction here prevents the auction
// process and optimizer daemon from validating/expiring the same Redis state
// with different thresholds, cadence, max margin or TTL.
func PoliciesFromConfig(cfg config.PercenterPolicyConfig) (SimplePolicy, ComplexPolicy, error) {
	simpleSteps, err := parsePolicySteps(cfg.SimpleMarginSearchSteps)
	if err != nil {
		return SimplePolicy{}, ComplexPolicy{}, err
	}
	complexSSPSteps, err := parsePolicySteps(cfg.ComplexSSPSearchSteps)
	if err != nil {
		return SimplePolicy{}, ComplexPolicy{}, err
	}
	complexMarginSteps, err := parsePolicySteps(cfg.ComplexMarginSearchSteps)
	if err != nil {
		return SimplePolicy{}, ComplexPolicy{}, err
	}

	simple := SimplePolicy{
		WinRateRetention:    cfg.SimpleWinRateRetention,
		MinImpressions:      cfg.SimpleMinImpressions,
		OptimizeInterval:    cfg.SimpleOptimizeInterval,
		RebenchmarkInterval: cfg.SimpleRebenchmarkInterval,
		SearchStepsPP:       simpleSteps,
		MaxMargin:           cfg.SimpleMaxMargin,
		StateTTL:            cfg.SimpleStateTTL,
	}.Normalize()
	complex := ComplexPolicy{
		BuyoutRetention:       cfg.ComplexBuyoutRetention,
		EfficiencyRetention:   cfg.ComplexEfficiencyRetention,
		MinImpressions:        cfg.ComplexMinImpressions,
		OptimizeInterval:      cfg.ComplexOptimizeInterval,
		RebenchmarkInterval:   cfg.ComplexRebenchmarkInterval,
		SSPSearchStepsPercent: complexSSPSteps,
		MarginSearchStepsPP:   complexMarginSteps,
		MaxMargin:             cfg.ComplexMaxMargin,
		StateTTL:              cfg.ComplexStateTTL,
	}.Normalize()

	if err := validateCanonicalSimplePolicy(simple); err != nil {
		return SimplePolicy{}, ComplexPolicy{}, err
	}
	if err := validateCanonicalComplexPolicy(complex); err != nil {
		return SimplePolicy{}, ComplexPolicy{}, err
	}
	return simple, complex, nil
}

func validateCanonicalSimplePolicy(policy SimplePolicy) error {
	if !policyStepsEqual(policy.SearchStepsPP, []float64{5, 2, 1}) {
		return fmt.Errorf("SIMPLE_MARGIN_SEARCH_STEPS must be exactly 5,2,1")
	}
	if policy.WinRateRetention != 0.50 {
		return fmt.Errorf("SIMPLE_WIN_RATE_RETENTION must be 0.5")
	}
	if policy.MinImpressions != 5 {
		return fmt.Errorf("SIMPLE_MIN_IMPRESSIONS must be 5")
	}
	if policy.OptimizeInterval != 5*time.Minute {
		return fmt.Errorf("SIMPLE_OPTIMIZE_INTERVAL must be 5m")
	}
	if policy.RebenchmarkInterval != 6*time.Hour {
		return fmt.Errorf("SIMPLE_REBENCHMARK_INTERVAL must be 6h")
	}
	if policy.MaxMargin != 0.90 {
		return fmt.Errorf("SIMPLE_MAX_MARGIN must be 0.9")
	}
	if policy.StateTTL <= 0 {
		return fmt.Errorf("SIMPLE_STATE_TTL must be positive")
	}
	return nil
}

func validateCanonicalComplexPolicy(policy ComplexPolicy) error {
	if !policyStepsEqual(policy.SSPSearchStepsPercent, []float64{10, 5, 2, 1}) {
		return fmt.Errorf("COMPLEX_SSP_SEARCH_STEPS must be exactly 10,5,2,1")
	}
	if !policyStepsEqual(policy.MarginSearchStepsPP, []float64{10, 5, 2, 1}) {
		return fmt.Errorf("COMPLEX_MARGIN_SEARCH_STEPS must be exactly 10,5,2,1")
	}
	if policy.BuyoutRetention != 0.80 {
		return fmt.Errorf("COMPLEX_BUYOUT_RETENTION must be 0.8")
	}
	if policy.EfficiencyRetention != 0.80 {
		return fmt.Errorf("COMPLEX_EFFICIENCY_RETENTION must be 0.8")
	}
	if policy.MinImpressions != 5 {
		return fmt.Errorf("COMPLEX_MIN_IMPRESSIONS must be 5")
	}
	if policy.OptimizeInterval != 5*time.Minute {
		return fmt.Errorf("COMPLEX_OPTIMIZE_INTERVAL must be 5m")
	}
	if policy.RebenchmarkInterval != 6*time.Hour {
		return fmt.Errorf("COMPLEX_REBENCHMARK_INTERVAL must be 6h")
	}
	if policy.MaxMargin != 0.90 {
		return fmt.Errorf("COMPLEX_MAX_MARGIN must be 0.9")
	}
	if policy.StateTTL <= 0 {
		return fmt.Errorf("COMPLEX_STATE_TTL must be positive")
	}
	return nil
}

func parsePolicySteps(raw string) ([]float64, error) {
	parts := strings.Split(raw, ",")
	steps := make([]float64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid percenter search steps %q", raw)
		}
		steps = append(steps, value)
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
