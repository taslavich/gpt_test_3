package dspRouterWeb

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func TestFormatRoutesV25KeepsFormatAndTrafficIndependent(t *testing.T) {
	popAdl := GeoDspLinkMap{"ALL": {"ALL": {"adl_pop": true}}}
	banAdl := GeoDspLinkMap{"ALL": {"ALL": {"adl_ban": true}}}
	natMc := GeoDspLinkMap{"ALL": {"ALL": {"mc_nat": true}}}
	ippMc := GeoDspLinkMap{"ALL": {"ALL": {"mc_ipp": true}}}
	routes := &FormatRoutesV25{
		POP: FormatRouteV25{AdultEndpoints: config.MapStringToString{"pop-url": "adl_pop"}, AdultLinkMap: &popAdl},
		BAN: FormatRouteV25{AdultEndpoints: config.MapStringToString{"ban-url": "adl_ban"}, AdultLinkMap: &banAdl},
		NAT: FormatRouteV25{MainstreamEndpoints: config.MapStringToString{"nat-url": "mc_nat"}, MainstreamLinkMap: &natMc},
		IPP: FormatRouteV25{MainstreamEndpoints: config.MapStringToString{"ipp-url": "mc_ipp"}, MainstreamLinkMap: &ippMc},
	}

	cases := []struct {
		format, typic, endpoint string
	}{
		{constants.POP, sppAdapterWeb.ADULT, "pop-url"},
		{constants.BAN, sppAdapterWeb.ADULT, "ban-url"},
		{constants.NAT, sppAdapterWeb.MAINSTREAM, "nat-url"},
		{constants.IPP, sppAdapterWeb.MAINSTREAM, "ipp-url"},
	}
	routes.prepare(nil)
	for _, tc := range cases {
		endpoints, links, _ := routes.selectRuntime(tc.format, tc.typic)
		if len(endpoints) != 1 || endpoints[0].Endpoint != tc.endpoint {
			t.Fatalf("%s/%s did not select endpoint %q: %v", tc.format, tc.typic, tc.endpoint, endpoints)
		}
		if links == nil {
			t.Fatalf("%s/%s route lost links: %v", tc.format, tc.typic, links)
		}
	}
}

func TestFormatRoutesV25PrecomputesDeterministicDSPOrder(t *testing.T) {
	routes := &FormatRoutesV25{POP: FormatRouteV25{AdultEndpoints: config.MapStringToString{
		"https://z.example":  "z-domain",
		"https://a2.example": "a-domain",
		"https://a1.example": "a-domain",
	}}}
	routes.prepare(nil)
	got, _, _ := routes.selectRuntime(constants.POP, sppAdapterWeb.ADULT)
	want := []DSPEndpointV25{
		{Endpoint: "https://a1.example", Domain: "a-domain"},
		{Endpoint: "https://a2.example", Domain: "a-domain"},
		{Endpoint: "https://z.example", Domain: "z-domain"},
	}
	if len(got) != len(want) {
		t.Fatalf("ordered endpoints=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ordered endpoints[%d]=%v want %v", i, got[i], want[i])
		}
	}
}
