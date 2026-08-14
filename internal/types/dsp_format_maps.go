package types

import (
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

type GeoDspPercentMap map[string]map[string]map[string]*PercentAndBidfloor

type FormatPercentRouteV25 struct {
	AdultFilename      string
	MainstreamFilename string
	AdultMap           *GeoDspPercentMap
	MainstreamMap      *GeoDspPercentMap
}

type FormatPercentRoutesV25 struct {
	POP FormatPercentRouteV25
	BAN FormatPercentRouteV25
	NAT FormatPercentRouteV25
	IPP FormatPercentRouteV25
}

func normalizeFormatForPercentRoute(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		// Preserve the legacy BidEngine behavior for callers that predate format.
		return constants.POP
	}
	switch normalized {
	case constants.POP:
		return constants.POP
	case constants.BAN:
		return constants.BAN
	case constants.NAT:
		return constants.NAT
	case constants.IPP:
		return constants.IPP
	default:
		return ""
	}
}

func (r *FormatPercentRoutesV25) Route(format string) *FormatPercentRouteV25 {
	if r == nil {
		return nil
	}
	switch normalizeFormatForPercentRoute(format) {
	case constants.POP:
		return &r.POP
	case constants.BAN:
		return &r.BAN
	case constants.NAT:
		return &r.NAT
	case constants.IPP:
		return &r.IPP
	default:
		return nil
	}
}

func (r *FormatPercentRoutesV25) Select(format, trafficType string) (string, *GeoDspPercentMap) {
	route := r.Route(format)
	if route == nil {
		return "", nil
	}
	switch strings.ToUpper(strings.TrimSpace(trafficType)) {
	case "ADULT":
		return route.AdultFilename, route.AdultMap
	case "MAINSTREAM":
		return route.MainstreamFilename, route.MainstreamMap
	default:
		return "", nil
	}
}
