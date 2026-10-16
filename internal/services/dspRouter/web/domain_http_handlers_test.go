package dspRouterWeb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func testDomainConfigFile(t *testing.T) (string, *filter.DomainChangersBoxChanger) {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "domains.json")
	if err := os.WriteFile(filename, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	box, err := filter.NewDomainChangersBoxChanger(filename)
	if err != nil {
		t.Fatal(err)
	}
	return filename, box
}

func TestDspDomainsMapHTTPEndpoints(t *testing.T) {
	adlFile, adl := testDomainConfigFile(t)
	mcFile, mc := testDomainConfigFile(t)
	router := chi.NewRouter()
	InitHttpRoutes(router, nil, "", "", nil, nil, "", "", nil, nil,
		adlFile, mcFile, adl, mc, "", "", nil, nil)

	body := `{"adl_test": {"apply":true,"siteIdBoxes":{"123, 456":{"toChange":true,"domain":"verified.example"}}}}`
	put := httptest.NewRequest(http.MethodPut, PutDspDomainsMapUrl+"?typic=ADULT", strings.NewReader(body))
	put.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, put)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("PUT status=%d body=%q", resp.Code, resp.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, GetDspDomainsMapUrl+"?typic=ADULT", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, get)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", resp.Code, resp.Body.String())
	}
	var raw filter.DomainChangerType
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["adl_test"] == nil || raw["adl_test"].SiteIdBoxes["123, 456"] == nil {
		t.Fatalf("GET should show original grouped IDs: %s", resp.Body.String())
	}

	debug := httptest.NewRequest(http.MethodGet, GetDebugDspDomainsMapUrl+"?typic=ADULT", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, debug)
	var expanded filter.DomainChangerType
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &expanded) != nil ||
		expanded["adl_test"] == nil || expanded["adl_test"].SiteIdBoxes["456"] == nil || expanded["adl_test"].SiteIdBoxes["123, 456"] != nil {
		t.Fatalf("debug GET must show expanded in-memory map: status=%d body=%s", resp.Code, resp.Body.String())
	}
	if got := mc.Snapshot(); len(got) != 0 {
		t.Fatalf("ADULT PUT changed MAINSTREAM: %v", got)
	}

	bad := httptest.NewRequest(http.MethodPut, PutDspDomainsMapUrl+"?typic=UNKNOWN", strings.NewReader(body))
	bad.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, bad)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid typic status=%d", resp.Code)
	}
}

func TestRouterAppliesDomainBeforeIDAndIsolatesDSP(t *testing.T) {
	originalID := "123"
	originalDomain := "original.example"
	page := "https://original.example/path"
	impID := "imp-1"
	requestID := "test-id"

	// Real outbound HTTP endpoints decode what was actually sent to each DSP.
	makeDSP := func(capture chan ortb.BidRequest) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			var got ortb.BidRequest
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			capture <- got
			w.WriteHeader(http.StatusNoContent)
		}))
	}
	oneCh := make(chan ortb.BidRequest, 1)
	twoCh := make(chan ortb.BidRequest, 1)
	plainCh := make(chan ortb.BidRequest, 1)
	idOnlyCh := make(chan ortb.BidRequest, 1)
	one := makeDSP(oneCh)
	two := makeDSP(twoCh)
	plain := makeDSP(plainCh)
	idOnly := makeDSP(idOnlyCh)
	defer one.Close()
	defer two.Close()
	defer plain.Close()
	defer idOnly.Close()

	file, domainChanger := testDomainConfigFile(t)
	if err := domainChanger.Rewrite(filter.DomainChangerType{
		"adl_one": {Apply: true, SiteIdBoxes: map[string]*filter.DomainBoxChanger{
			"123": {ToChange: true, Domain: "first.example"},
		}},
		"adl_two": {Apply: true, SiteIdBoxes: map[string]*filter.DomainBoxChanger{
			"123": {ToChange: true, Domain: "second.example"},
		}},
	}, file); err != nil {
		t.Fatal(err)
	}
	siteIDChanger := &filter.ChangersBoxChanger{Changers: filter.ToManyChangersMap(filter.ChangerType{
		"adl_one": {Apply: true, SiteIdBoxes: map[string]*filter.SiteIdBoxChanger{
			"123": {ToChange: true, SiteId: "replacement"},
		}},
		"adl_idonly": {Apply: true, SiteIdBoxes: map[string]*filter.SiteIdBoxChanger{
			"123": {ToChange: true, SiteId: "replacement"},
		}},
	})}
	routes := &FormatRoutesV25{POP: FormatRouteV25{
		AdultEndpoints: config.MapStringToString{
			one.URL: "adl_one", two.URL: "adl_two", plain.URL: "adl_plain", idOnly.URL: "adl_idonly",
		},
	}}
	server := NewServer(nil, nil, nil, routes, nil, time.Second, InitSspHttpClients(routes.EndpointSets()...),
		nil, nil, nil, nil, siteIDChanger, nil, config.MapStringToDuration{"ssp.example": time.Second}, nil, nil)
	server.SetDomainChangers(domainChanger, nil)
	req := &dspRouterGrpc.DspRouterRequest_V2_5{
		BidRequest: &ortb.BidRequest{
			Id: &requestID, Imp: []*ortb.Imp{{Id: &impID}},
			Site: &ortb.Site{Id: &originalID, Domain: &originalDomain, Page: &page},
		},
		TrafficType: "ADULT", Format: constants.POP, SspDomain: "adl_ssp.example",
	}
	if _, err := server.GetBids_V2_5(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	first, second, third, fourth := <-oneCh, <-twoCh, <-plainCh, <-idOnlyCh
	if first.Site.GetId() != "replacement" || first.Site.GetDomain() != "first.example" {
		t.Fatalf("domain->ID sequence failed for first DSP: %+v", first.Site)
	}
	if second.Site.GetId() != "123" || second.Site.GetDomain() != "second.example" {
		t.Fatalf("domain-only update missing: %+v", second.Site)
	}
	if third.Site.GetId() != "123" || third.Site.GetDomain() != "original.example" {
		t.Fatalf("unchanged DSP contaminated: %+v", third.Site)
	}
	if fourth.Site.GetId() != "replacement" || fourth.Site.GetDomain() != "original.example" {
		t.Fatalf("site_id-only update affected domain: %+v", fourth.Site)
	}
	if first.Site.GetPage() != page || second.Site.GetPage() != page || third.Site.GetPage() != page ||
		req.BidRequest.Site.GetId() != "123" || req.BidRequest.Site.GetDomain() != "original.example" {
		t.Fatal("unexpected change to site.page or original request")
	}
}
