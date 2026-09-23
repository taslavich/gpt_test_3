package percenter

import (
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

func canonicalPolicyConfig() config.PercenterPolicyConfig {
	return config.PercenterPolicyConfig{
		SimpleOptimizeInterval:     5 * time.Minute,
		SimpleRebenchmarkInterval:  6 * time.Hour,
		SimpleStateTTL:             7 * 24 * time.Hour,
		SimpleMinImpressions:       5,
		SimpleWinRateRetention:     0.50,
		SimpleMarginSearchSteps:    "5,2,1",
		SimpleMaxMargin:            0.90,
		ComplexOptimizeInterval:    5 * time.Minute,
		ComplexRebenchmarkInterval: 6 * time.Hour,
		ComplexStateTTL:            7 * 24 * time.Hour,
		ComplexMinImpressions:      5,
		ComplexBuyoutRetention:     0.80,
		ComplexEfficiencyRetention: 0.80,
		ComplexSSPSearchSteps:      "10,5,2,1",
		ComplexMarginSearchSteps:   "10,5,2,1",
		ComplexMaxMargin:           0.90,
	}
}

func TestPoliciesFromConfigKeepsADVAndDaemonStateContractIdentical(t *testing.T) {
	cfg := canonicalPolicyConfig()
	simple, complex, err := PoliciesFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if simple.OptimizeInterval != 5*time.Minute || simple.RebenchmarkInterval != 6*time.Hour || simple.StateTTL != 7*24*time.Hour || simple.MaxMargin != .90 {
		t.Fatalf("unexpected Simple production policy: %#v", simple)
	}
	if complex.OptimizeInterval != 5*time.Minute || complex.RebenchmarkInterval != 6*time.Hour || complex.StateTTL != 7*24*time.Hour || complex.MaxMargin != .90 {
		t.Fatalf("unexpected Complex production policy: %#v", complex)
	}
}

func TestPoliciesFromConfigRejectsADVDaemonDrift(t *testing.T) {
	cfg := canonicalPolicyConfig()
	cfg.SimpleOptimizeInterval = 4 * time.Minute
	if _, _, err := PoliciesFromConfig(cfg); err == nil {
		t.Fatal("non-canonical Simple cadence must be rejected before ADV/daemon can disagree")
	}

	cfg = canonicalPolicyConfig()
	cfg.ComplexMarginSearchSteps = "10,5,1"
	if _, _, err := PoliciesFromConfig(cfg); err == nil {
		t.Fatal("non-canonical Complex search steps must be rejected")
	}
}
