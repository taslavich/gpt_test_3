package dspRouterWeb

import (
	"net/http"
	"sort"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

type GeoDspLinkMap map[string]map[string]map[string]bool

type DSPEndpointV25 struct {
	Endpoint string
	Domain   string
	client   *http.Client
}

// FormatRouteV25 isolates DSP endpoints and SSP/GEO allow-maps for one ad
// format. Adult and mainstream stay independent inside that format.
type FormatRouteV25 struct {
	// Legacy endpoint maps are retained for tests/backwards compatibility.
	// Production uses the atomic stores below.
	AdultEndpoints      config.MapStringToString
	MainstreamEndpoints config.MapStringToString
	AdultStore          *DSPEndpointStore
	MainstreamStore     *DSPEndpointStore

	AdultLinkFilename      string
	MainstreamLinkFilename string
	AdultLinkMap           *GeoDspLinkMap
	MainstreamLinkMap      *GeoDspLinkMap

	// Legacy precompiled runtime used when no atomic endpoint store is configured.
	// Production endpoint stores keep the same data inside immutable snapshots.
	AdultOrdered         []DSPEndpointV25
	MainstreamOrdered    []DSPEndpointV25
	AdultNativeMask      filter.NativeFieldMask
	MainstreamNativeMask filter.NativeFieldMask
}

// FormatRoutesV25 keeps POP deployment backwards-compatible while allowing
// BAN/NAT/IPP/VID to be configured independently.
type FormatRoutesV25 struct {
	POP FormatRouteV25
	BAN FormatRouteV25
	NAT FormatRouteV25
	IPP FormatRouteV25
	VID FormatRouteV25
}

func normalizeDSPFormat(format string) string {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case constants.POP:
		return constants.POP
	case constants.BAN:
		return constants.BAN
	case constants.NAT:
		return constants.NAT
	case constants.IPP:
		return constants.IPP
	case constants.VID:
		return constants.VID
	default:
		return ""
	}
}

func (r *FormatRoutesV25) route(format string) *FormatRouteV25 {
	if r == nil {
		return nil
	}
	switch normalizeDSPFormat(format) {
	case constants.POP:
		return &r.POP
	case constants.BAN:
		return &r.BAN
	case constants.NAT:
		return &r.NAT
	case constants.IPP:
		return &r.IPP
	case constants.VID:
		return &r.VID
	default:
		return nil
	}
}

func (r *FormatRoutesV25) selectRuntime(format, trafficType string) ([]DSPEndpointV25, GeoDspLinkMap, filter.NativeFieldMask) {
	route := r.route(format)
	if route == nil {
		return nil, nil, 0
	}
	switch trafficType {
	case sppAdapterWeb.ADULT:
		ordered, nativeMask := route.AdultOrdered, route.AdultNativeMask
		if route.AdultStore != nil {
			snapshot := route.AdultStore.runtimeSnapshot()
			ordered, nativeMask = snapshot.ordered, snapshot.nativeMask
		}
		if route.AdultLinkMap == nil {
			return ordered, nil, nativeMask
		}
		return ordered, *route.AdultLinkMap, nativeMask
	case sppAdapterWeb.MAINSTREAM:
		ordered, nativeMask := route.MainstreamOrdered, route.MainstreamNativeMask
		if route.MainstreamStore != nil {
			snapshot := route.MainstreamStore.runtimeSnapshot()
			ordered, nativeMask = snapshot.ordered, snapshot.nativeMask
		}
		if route.MainstreamLinkMap == nil {
			return ordered, nil, nativeMask
		}
		return ordered, *route.MainstreamLinkMap, nativeMask
	default:
		return nil, nil, 0
	}
}

func (r *FormatRoutesV25) selectConfig(format, trafficType string) (string, *GeoDspLinkMap) {
	route := r.route(format)
	if route == nil {
		return "", nil
	}
	switch trafficType {
	case sppAdapterWeb.ADULT:
		return route.AdultLinkFilename, route.AdultLinkMap
	case sppAdapterWeb.MAINSTREAM:
		return route.MainstreamLinkFilename, route.MainstreamLinkMap
	default:
		return "", nil
	}
}

func (r *FormatRoutesV25) selectEndpointStore(format, trafficType string) *DSPEndpointStore {
	route := r.route(format)
	if route == nil {
		return nil
	}
	switch trafficType {
	case sppAdapterWeb.ADULT:
		return route.AdultStore
	case sppAdapterWeb.MAINSTREAM:
		return route.MainstreamStore
	default:
		return nil
	}
}

func orderedEndpoints(endpoints config.MapStringToString) []DSPEndpointV25 {
	if len(endpoints) == 0 {
		return nil
	}
	ordered := make([]DSPEndpointV25, 0, len(endpoints))
	for endpoint, domain := range endpoints {
		ordered = append(ordered, DSPEndpointV25{Endpoint: endpoint, Domain: domain})
	}
	sortDSPEndpoints(ordered)
	return ordered
}

func sortDSPEndpoints(ordered []DSPEndpointV25) {
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Domain == ordered[j].Domain {
			return ordered[i].Endpoint < ordered[j].Endpoint
		}
		return ordered[i].Domain < ordered[j].Domain
	})
}

func (r *FormatRoutesV25) prepare(processor *filter.OptimizedFilterProcessor) {
	if r == nil {
		return
	}
	prepareRoute := func(route *FormatRouteV25) {
		if route == nil {
			return
		}
		if route.AdultStore == nil {
			route.AdultOrdered = orderedEndpoints(route.AdultEndpoints)
			route.AdultNativeMask = 0
			if processor != nil {
				for _, dsp := range route.AdultOrdered {
					route.AdultNativeMask |= processor.NativeMaskForDSPV25(DeletePrefix(dsp.Domain))
				}
			}
		}
		if route.MainstreamStore == nil {
			route.MainstreamOrdered = orderedEndpoints(route.MainstreamEndpoints)
			route.MainstreamNativeMask = 0
			if processor != nil {
				for _, dsp := range route.MainstreamOrdered {
					route.MainstreamNativeMask |= processor.NativeMaskForDSPV25(DeletePrefix(dsp.Domain))
				}
			}
		}
	}
	prepareRoute(&r.POP)
	prepareRoute(&r.BAN)
	prepareRoute(&r.NAT)
	prepareRoute(&r.IPP)
	prepareRoute(&r.VID)
}

func (r *FormatRoutesV25) EndpointSets() []config.MapStringToString {
	if r == nil {
		return nil
	}
	return []config.MapStringToString{
		r.POP.AdultEndpoints, r.POP.MainstreamEndpoints,
		r.BAN.AdultEndpoints, r.BAN.MainstreamEndpoints,
		r.NAT.AdultEndpoints, r.NAT.MainstreamEndpoints,
		r.IPP.AdultEndpoints, r.IPP.MainstreamEndpoints,
		r.VID.AdultEndpoints, r.VID.MainstreamEndpoints,
	}
}
