package auction

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
)

// InvalidCampaignIPFilterError marks an invalid value inside campaigns.ip.
// Snapshot loading uses this typed error to exclude only the affected campaign
// and send a precise bot notification without taking the whole ADV snapshot down.
type InvalidCampaignIPFilterError struct {
	Value string
	Cause error
}

func (e *InvalidCampaignIPFilterError) Error() string {
	if e == nil {
		return "invalid campaign IP filter"
	}
	if e.Value == "" {
		return fmt.Sprintf("invalid campaign IP filter: %v", e.Cause)
	}
	return fmt.Sprintf("invalid campaign IP filter value %q: %v", e.Value, e.Cause)
}

func (e *InvalidCampaignIPFilterError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// parseIPv4CampaignFilter parses the existing JSONB campaign IP filter. Exact
// IPv4 addresses stay in Filters.Objects while CIDR networks are compiled once
// into netip.Prefix values for the snapshot hot path.
func parseIPv4CampaignFilter(raw []byte) (*filterV2.Filters, []netip.Prefix, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return filterV2.NewFilters(false, false, nil), nil, nil
	}

	var payload filterV2.FiltersJson
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, &InvalidCampaignIPFilterError{Cause: fmt.Errorf("invalid JSON: %w", err)}
	}
	if len(payload.Objects) == 0 {
		return filterV2.NewFilters(false, payload.IsWhiteList, nil), nil, nil
	}

	exact := make(map[string]bool, len(payload.Objects))
	prefixes := make([]netip.Prefix, 0, len(payload.Objects))
	seenPrefixes := make(map[netip.Prefix]struct{}, len(payload.Objects))

	for _, configured := range payload.Objects {
		value := strings.TrimSpace(configured)
		if value == "" {
			return nil, nil, &InvalidCampaignIPFilterError{Value: configured, Cause: fmt.Errorf("empty value")}
		}

		if strings.Contains(value, "/") {
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				return nil, nil, &InvalidCampaignIPFilterError{Value: configured, Cause: fmt.Errorf("invalid IPv4 CIDR")}
			}
			if !prefix.Addr().Is4() {
				return nil, nil, &InvalidCampaignIPFilterError{Value: configured, Cause: fmt.Errorf("IPv6 is not allowed in the IPv4 IP filter")}
			}
			prefix = prefix.Masked()
			if _, duplicate := seenPrefixes[prefix]; duplicate {
				continue
			}
			seenPrefixes[prefix] = struct{}{}
			prefixes = append(prefixes, prefix)
			continue
		}

		addr, err := netip.ParseAddr(value)
		if err != nil {
			return nil, nil, &InvalidCampaignIPFilterError{Value: configured, Cause: fmt.Errorf("invalid IPv4 address")}
		}
		if !addr.Is4() {
			return nil, nil, &InvalidCampaignIPFilterError{Value: configured, Cause: fmt.Errorf("IPv6 is not allowed in the IPv4 IP filter")}
		}
		exact[addr.String()] = true
	}

	return &filterV2.Filters{
		Apply:       true,
		IsWhiteList: payload.IsWhiteList,
		Objects:     exact,
	}, prefixes, nil
}

// ipv4CampaignFilterAllowed preserves the existing whitelist/blacklist
// semantics while extending a match from exact IPv4 values to configured CIDR
// prefixes. The returned matched flag is used only for diagnostics/logging.
func ipv4CampaignFilterAllowed(filter *filterV2.Filters, prefixes []netip.Prefix, rawIP *string) (allowedResult bool, matched bool) {
	if filter == nil || !filter.Apply {
		return true, false
	}
	if rawIP == nil {
		return !filter.IsWhiteList, false
	}

	addr, err := netip.ParseAddr(strings.TrimSpace(*rawIP))
	if err == nil && addr.Is4() {
		if filter.Objects[addr.String()] {
			matched = true
		} else {
			for _, prefix := range prefixes {
				if prefix.Contains(addr) {
					matched = true
					break
				}
			}
		}
	}

	if filter.IsWhiteList {
		return matched, matched
	}
	return !matched, matched
}
