package percenter

import (
	"strings"
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
		HistoryPendingTTL:          30 * 24 * time.Hour,
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

func TestPoliciesFromConfigRejectsExplicitInvalidValuesBeforeNormalize(t *testing.T) {
	cases := []func(*config.PercenterPolicyConfig){
		func(c *config.PercenterPolicyConfig) { c.SimpleOptimizeInterval = 0 },
		func(c *config.PercenterPolicyConfig) { c.SimpleWinRateRetention = 2 },
		func(c *config.PercenterPolicyConfig) { c.SimpleMaxMargin = 0 },
		func(c *config.PercenterPolicyConfig) { c.ComplexBuyoutRetention = 0 },
		func(c *config.PercenterPolicyConfig) { c.ComplexMaxMargin = 1 },
		func(c *config.PercenterPolicyConfig) { c.HistoryPendingTTL = 0 },
	}
	for i, mutate := range cases {
		cfg := canonicalPolicyConfig()
		mutate(&cfg)
		if _, _, err := PoliciesFromConfig(cfg); err == nil {
			t.Fatalf("case %d: explicit invalid config must be rejected, not normalized to default", i)
		}
	}
}

func TestPolicyFingerprintCanonicalizesStepFormattingAndTTLParity(t *testing.T) {
	cfgA := canonicalPolicyConfig()
	cfgB := canonicalPolicyConfig()
	cfgB.SimpleMarginSearchSteps = " 5, 2, 1 "
	cfgB.ComplexSSPSearchSteps = "10, 5, 2, 1"
	sA, cA, err := PoliciesFromConfig(cfgA)
	if err != nil {
		t.Fatal(err)
	}
	sB, cB, err := PoliciesFromConfig(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if PolicyFingerprint(sA, cA) != PolicyFingerprint(sB, cB) {
		t.Fatal("equivalent policy inputs must have identical fingerprint")
	}
	if sA.StateTTL != sB.StateTTL || cA.StateTTL != cB.StateTTL || sA.PendingHistoryTTL != cA.PendingHistoryTTL {
		t.Fatal("policy TTL parity broken")
	}
}

func TestTTLOnlyChangeDoesNotInvalidateOptimizerPoint(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	cfg := canonicalPolicyConfig()
	s1, c1, err := PoliciesFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sState := NewSimpleState("s", "c", 1, .2, false, s1, now)
	cState := NewComplexState("x", "c", 1, .2, c1, now)
	cfg.SimpleStateTTL = 24 * time.Hour
	cfg.ComplexStateTTL = 24 * time.Hour
	s2, c2, err := PoliciesFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !sState.Compatible("s", "c", 1, .2, false, s2) {
		t.Fatal("Simple TTL-only change must not invalidate state")
	}
	if !cState.Compatible("x", "c", 1, .2, c2) {
		t.Fatal("Complex TTL-only change must not invalidate state")
	}
	if s1.StateTTL == s2.StateTTL || c1.StateTTL == c2.StateTTL {
		t.Fatal("test did not change TTL")
	}
}

func TestPendingHistoryRetentionIndependentFromStateTTL(t *testing.T) {
	cfg := canonicalPolicyConfig()
	cfg.SimpleStateTTL = 24 * time.Hour
	cfg.ComplexStateTTL = 24 * time.Hour
	cfg.HistoryPendingTTL = 30 * 24 * time.Hour
	s, c, err := PoliciesFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.PendingHistoryTTL != 30*24*time.Hour || c.PendingHistoryTTL != 30*24*time.Hour {
		t.Fatal("pending history retention must be shared")
	}
	if s.PendingHistoryTTL <= s.StateTTL || c.PendingHistoryTTL <= c.StateTTL {
		t.Fatal("pending history must survive shorter optimizer TTL")
	}
}

func TestPoliciesFromConfigErrorsNameActualAndExpectedInvariant(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.PercenterPolicyConfig)
		want   []string
	}{
		{name: "simple retention", mutate: func(c *config.PercenterPolicyConfig) { c.SimpleWinRateRetention = .7 }, want: []string{"SIMPLE_WIN_RATE_RETENTION=0.7", "requires 0.5"}},
		{name: "simple steps", mutate: func(c *config.PercenterPolicyConfig) { c.SimpleMarginSearchSteps = "5,1" }, want: []string{"SIMPLE_MARGIN_SEARCH_STEPS=\"5,1\"", "requires 5,2,1"}},
		{name: "simple cadence", mutate: func(c *config.PercenterPolicyConfig) { c.SimpleOptimizeInterval = time.Minute }, want: []string{"SIMPLE_OPTIMIZE_INTERVAL=1m0s", "requires 5m"}},
		{name: "complex buyout", mutate: func(c *config.PercenterPolicyConfig) { c.ComplexBuyoutRetention = .7 }, want: []string{"COMPLEX_BUYOUT_RETENTION=0.7", "requires 0.8"}},
		{name: "complex efficiency", mutate: func(c *config.PercenterPolicyConfig) { c.ComplexEfficiencyRetention = .7 }, want: []string{"COMPLEX_EFFICIENCY_RETENTION=0.7", "requires 0.8"}},
		{name: "complex steps", mutate: func(c *config.PercenterPolicyConfig) { c.ComplexSSPSearchSteps = "10,5,2" }, want: []string{"COMPLEX_SSP_SEARCH_STEPS=\"10,5,2\"", "requires 10,5,2,1"}},
		{name: "history ttl", mutate: func(c *config.PercenterPolicyConfig) { c.HistoryPendingTTL = 0 }, want: []string{"HISTORY_PENDING_TTL=0s", "must be >0"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := canonicalPolicyConfig()
			tt.mutate(&cfg)
			_, _, err := PoliciesFromConfig(cfg)
			if err == nil {
				t.Fatal("expected validation error")
			}
			for _, part := range tt.want {
				if !strings.Contains(err.Error(), part) {
					t.Fatalf("error %q does not contain %q", err, part)
				}
			}
		})
	}
}

func TestPoliciesFromConfigMalformedStepErrorsNameEnvActualAndCanonicalValue(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.PercenterPolicyConfig)
		want   []string
	}{
		{
			name:   "simple margin steps",
			mutate: func(c *config.PercenterPolicyConfig) { c.SimpleMarginSearchSteps = "5,x,1" },
			want:   []string{`SIMPLE_MARGIN_SEARCH_STEPS="5,x,1"`, "production invariant requires 5,2,1"},
		},
		{
			name:   "complex ssp steps",
			mutate: func(c *config.PercenterPolicyConfig) { c.ComplexSSPSearchSteps = "10,x,2,1" },
			want:   []string{`COMPLEX_SSP_SEARCH_STEPS="10,x,2,1"`, "production invariant requires 10,5,2,1"},
		},
		{
			name:   "complex margin steps",
			mutate: func(c *config.PercenterPolicyConfig) { c.ComplexMarginSearchSteps = "10,5,x,1" },
			want:   []string{`COMPLEX_MARGIN_SEARCH_STEPS="10,5,x,1"`, "production invariant requires 10,5,2,1"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := canonicalPolicyConfig()
			tt.mutate(&cfg)
			_, _, err := PoliciesFromConfig(cfg)
			if err == nil {
				t.Fatal("expected malformed step config to fail")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}
