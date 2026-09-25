package auction

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
)

var (
	campaignOSVersionRE = regexp.MustCompile(`(?i)^(ios|android)\s+([0-9]+)(?:\.([0-9]+))?$`)
	requestOSVersionRE  = regexp.MustCompile(`^[0-9]+(?:[._][0-9]+)*$`)
)

type osVersionTarget struct {
	os         string
	components []uint64
	special    string
	canonical  string
}

type osVersionCampaignFilter struct {
	Apply       bool
	IsWhiteList bool
	Targets     []osVersionTarget
}

func parseCampaignOSVersionTarget(raw string) (osVersionTarget, error) {
	value := strings.TrimSpace(raw)
	if strings.EqualFold(value, "Android 12L") {
		return osVersionTarget{os: "android", special: "12l", canonical: "android|12l"}, nil
	}
	match := campaignOSVersionRE.FindStringSubmatch(value)
	if match == nil {
		return osVersionTarget{}, fmt.Errorf("expected iOS/Android major or major.minor version")
	}
	components := make([]uint64, 0, 2)
	for _, part := range match[2:] {
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return osVersionTarget{}, fmt.Errorf("invalid numeric component %q", part)
		}
		components = append(components, n)
	}
	osName := strings.ToLower(match[1])
	canonicalParts := make([]string, len(components))
	for i, n := range components {
		canonicalParts[i] = strconv.FormatUint(n, 10)
	}
	return osVersionTarget{
		os:         osName,
		components: components,
		canonical:  osName + "|" + strings.Join(canonicalParts, "."),
	}, nil
}

func parseRequestOSVersion(raw string) ([]uint64, string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "12l" {
		return nil, "12l", true
	}
	if !requestOSVersionRE.MatchString(value) {
		return nil, "", false
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == '_' })
	components := make([]uint64, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, "", false
		}
		components = append(components, n)
	}
	return components, "", len(components) > 0
}

func parseOSVersionCampaignFilter(raw []byte, osFilter *filterV2.Filters) (*osVersionCampaignFilter, error) {
	generic, err := filterV2.GetFiltersFromJSONB(raw)
	if err != nil {
		return nil, err
	}
	if generic == nil || !generic.Apply || len(generic.Objects) == 0 {
		return &osVersionCampaignFilter{}, nil
	}
	if osFilter == nil || !osFilter.Apply || !osFilter.IsWhiteList {
		return nil, fmt.Errorf("os_version requires OS whitelist containing iOS and/or Android")
	}

	out := &osVersionCampaignFilter{Apply: true, IsWhiteList: generic.IsWhiteList, Targets: make([]osVersionTarget, 0, len(generic.Objects))}
	seen := make(map[string]struct{}, len(generic.Objects))
	for rawTarget := range generic.Objects {
		target, err := parseCampaignOSVersionTarget(rawTarget)
		if err != nil {
			return nil, fmt.Errorf("invalid os_version %q: %w", rawTarget, err)
		}
		if !osFilter.Objects[target.os] {
			return nil, fmt.Errorf("os_version %q requires %s in OS whitelist", rawTarget, target.os)
		}
		if _, duplicate := seen[target.canonical]; duplicate {
			continue
		}
		seen[target.canonical] = struct{}{}
		out.Targets = append(out.Targets, target)
	}
	out.Apply = len(out.Targets) > 0
	return out, nil
}

func cloneOSVersionFilter(filter *osVersionCampaignFilter) *osVersionCampaignFilter {
	if filter == nil {
		return nil
	}
	clone := &osVersionCampaignFilter{Apply: filter.Apply, IsWhiteList: filter.IsWhiteList, Targets: make([]osVersionTarget, len(filter.Targets))}
	for i, target := range filter.Targets {
		clone.Targets[i] = target
		clone.Targets[i].components = append([]uint64(nil), target.components...)
	}
	return clone
}

func osVersionTargetMatches(target osVersionTarget, requestOS, requestVersion string) bool {
	if target.os != normalizeOS(requestOS) {
		return false
	}
	components, special, ok := parseRequestOSVersion(requestVersion)
	if !ok {
		return false
	}
	if target.special != "" {
		return special == target.special
	}
	if special != "" || len(components) < len(target.components) {
		return false
	}
	for i, expected := range target.components {
		if components[i] != expected {
			return false
		}
	}
	return true
}

func osVersionFilterAllowed(filter *osVersionCampaignFilter, requestOS, requestVersion *string) (allowed, matched bool) {
	if filter == nil || !filter.Apply {
		return true, false
	}
	if requestOS == nil || requestVersion == nil {
		return !filter.IsWhiteList, false
	}
	for _, target := range filter.Targets {
		if osVersionTargetMatches(target, *requestOS, *requestVersion) {
			matched = true
			break
		}
	}
	if filter.IsWhiteList {
		return matched, matched
	}
	return !matched, matched
}

func osVersionRequestValue(osName, version *string) string {
	if osName == nil || version == nil {
		return "<nil>"
	}
	return normalizeOS(*osName) + "|" + strings.TrimSpace(*version)
}
