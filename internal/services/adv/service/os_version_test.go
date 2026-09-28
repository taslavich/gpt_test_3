package auction

import (
	"testing"

	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
)

func TestOSVersionTargetBranchMatching(t *testing.T) {
	ios187, err := parseCampaignOSVersionTarget("iOS 18.7")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"18.7", "18.7.1", "18_7_10"} {
		if !osVersionTargetMatches(ios187, "iPhone OS", version) {
			t.Fatalf("expected iOS 18.7 to match %q", version)
		}
	}
	for _, version := range []string{"18.3", "18.70", "19.7", "x86_64"} {
		if osVersionTargetMatches(ios187, "iOS", version) {
			t.Fatalf("expected iOS 18.7 not to match %q", version)
		}
	}
	if osVersionTargetMatches(ios187, "Android", "18.7.1") {
		t.Fatal("OS and version must be matched together")
	}
}

func TestOSVersionMajorMatchesDescendants(t *testing.T) {
	target, err := parseCampaignOSVersionTarget("Android 8")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"8", "8.0", "8.1", "8.1.0"} {
		if !osVersionTargetMatches(target, "Android OS", version) {
			t.Fatalf("expected Android 8 to match %q", version)
		}
	}
	if osVersionTargetMatches(target, "Android", "80") {
		t.Fatal("major version must match on a component boundary")
	}
}

func TestAndroid12LIsSeparateBranch(t *testing.T) {
	target, err := parseCampaignOSVersionTarget("Android 12L")
	if err != nil {
		t.Fatal(err)
	}
	if !osVersionTargetMatches(target, "android", "12L") {
		t.Fatal("Android 12L must match raw 12L")
	}
	for _, version := range []string{"12", "12.0", "12.1", "12L.0"} {
		if osVersionTargetMatches(target, "android", version) {
			t.Fatalf("Android 12L must not match %q", version)
		}
	}
}

func TestOSVersionFilterUnknownSemantics(t *testing.T) {
	white := &osVersionCampaignFilter{Apply: true, IsWhiteList: true, Targets: []osVersionTarget{{os: "ios", components: []uint64{18}}}}
	black := &osVersionCampaignFilter{Apply: true, IsWhiteList: false, Targets: []osVersionTarget{{os: "ios", components: []uint64{18}}}}
	if allowed, _ := osVersionFilterAllowed(white, nil, nil); allowed {
		t.Fatal("unknown request must fail a whitelist")
	}
	if allowed, _ := osVersionFilterAllowed(black, nil, nil); !allowed {
		t.Fatal("unknown request must pass a blacklist")
	}
}

func TestParseOSVersionCampaignFilterRequiresMatchingOSWhitelist(t *testing.T) {
	osFilter := filterV2.NewFilters(true, true, []string{"ios"})
	if _, err := parseOSVersionCampaignFilter([]byte(`{"isWhiteList":true,"objects":["Android 8.1"]}`), osFilter); err == nil {
		t.Fatal("expected Android version to require Android OS whitelist")
	}
	filter, err := parseOSVersionCampaignFilter([]byte(`{"isWhiteList":false,"objects":["iOS 18.7"]}`), osFilter)
	if err != nil {
		t.Fatalf("parse valid filter: %v", err)
	}
	if filter.IsWhiteList || len(filter.Targets) != 1 {
		t.Fatalf("unexpected parsed filter: %#v", filter)
	}
}

func TestNormalizeOSSynonyms(t *testing.T) {
	for _, raw := range []string{"iOS", "iPhone OS", "iPad OS", "iPhoneOS", "iPad", "IPHONE"} {
		if got := normalizeOS(raw); got != "ios" {
			t.Fatalf("normalizeOS(%q)=%q", raw, got)
		}
	}
	for _, raw := range []string{"Android", "Android OS", "AndroidOS"} {
		if got := normalizeOS(raw); got != "android" {
			t.Fatalf("normalizeOS(%q)=%q", raw, got)
		}
	}
}

func TestOSVersionDebugLogValues(t *testing.T) {
	osFilter := filterV2.NewFilters(true, true, []string{"ios"})
	filter, err := parseOSVersionCampaignFilter([]byte(`{"isWhiteList":true,"objects":["iOS 18.7","iOS 17"]}`), osFilter)
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}

	requestOS := "iPhone OS"
	requestVersion := "18_7_1"
	if got := osVersionTargetsLogValue(filter); got != "iOS 17=>ios|17,iOS 18.7=>ios|18.7" {
		t.Fatalf("unexpected db target log value: %q", got)
	}
	if got := osVersionOSLogValue(&requestOS); got != "ios" {
		t.Fatalf("unexpected normalized request OS: %q", got)
	}
	if got := osVersionNormalizedRequestVersion(&requestVersion); got != "18.7.1" {
		t.Fatalf("unexpected normalized request version: %q", got)
	}
	if got := osVersionComparisonLogValue(filter, &requestOS, &requestVersion); got != "iOS 17:false,iOS 18.7:true" {
		t.Fatalf("unexpected comparison log value: %q", got)
	}
	allowed, matched := osVersionFilterAllowed(filter, &requestOS, &requestVersion)
	if !matched || !allowed {
		t.Fatalf("expected matching whitelist request to pass: matched=%t allowed=%t", matched, allowed)
	}
}

func TestOSVersionDebugLogUnknownRequest(t *testing.T) {
	filter := &osVersionCampaignFilter{
		Apply:       true,
		IsWhiteList: true,
		Targets: []osVersionTarget{{
			os:         "ios",
			components: []uint64{18},
			canonical:  "ios|18",
			raw:        "iOS 18",
		}},
	}
	if got := osVersionComparisonLogValue(filter, nil, nil); got != "<request_unknown>" {
		t.Fatalf("unexpected unknown-request comparison: %q", got)
	}
	if got := osVersionNormalizedRequestVersion(nil); got != "<nil>" {
		t.Fatalf("unexpected nil version log value: %q", got)
	}
}
