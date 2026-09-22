package dspRouterWeb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func writeSiteIDDSPMapForTest(t *testing.T, body string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "site_id_dsp_links.json")
	if err := os.WriteFile(filename, []byte(body), 0644); err != nil {
		t.Fatalf("write map: %v", err)
	}
	return filename
}

func TestSiteIDDSPLinkLookupSupportsAllAndCommaKeys(t *testing.T) {
	filename := writeSiteIDDSPMapForTest(t, `{
      "site-a, site-b": {
        "adl_dsp_one, adl_dsp_two": true,
        "ALL": false
      },
      "ALL": {
        "adl_dsp_fallback": true
      }
    }`)
	store, err := NewSiteIDDSPLinkStore(filename)
	if err != nil {
		t.Fatalf("NewSiteIDDSPLinkStore: %v", err)
	}

	tests := []struct {
		site, dsp string
		want      bool
		found     bool
	}{
		{"site-a", "adl_dsp_one", true, true},
		{"site-b", "adl_dsp_two", true, true},
		{"site-a", "unlisted", false, true},
		{"other", "adl_dsp_fallback", true, true},
		{"other", "unlisted", false, false},
	}
	for _, tt := range tests {
		got, found := store.Lookup(tt.site, tt.dsp)
		if got != tt.want || found != tt.found {
			t.Fatalf("Lookup(%q,%q)=(%v,%v), want (%v,%v)", tt.site, tt.dsp, got, found, tt.want, tt.found)
		}
	}
}

func TestSiteIDDSPLinkOverridesLegacyLinkMapBothDirections(t *testing.T) {
	filename := writeSiteIDDSPMapForTest(t, `{
      "site-allow": {"adl_dsp_x": true},
      "site-block": {"adl_dsp_x": false}
    }`)
	store, err := NewSiteIDDSPLinkStore(filename)
	if err != nil {
		t.Fatalf("NewSiteIDDSPLinkStore: %v", err)
	}

	legacyBlock := GeoDspLinkMap{"ssp": {"US": {"adl_dsp_x": false}}}
	if !shouldRouteDSPByMaps("site-allow", "adl_dsp_x", "ssp", "US", store, legacyBlock) {
		t.Fatal("site_id/DSP true must override legacy false")
	}

	legacyAllow := GeoDspLinkMap{"ssp": {"US": {"adl_dsp_x": true}}}
	if shouldRouteDSPByMaps("site-block", "adl_dsp_x", "ssp", "US", store, legacyAllow) {
		t.Fatal("site_id/DSP false must override legacy true")
	}

	if !shouldRouteDSPByMaps("site-missing", "adl_dsp_x", "ssp", "US", store, legacyAllow) {
		t.Fatal("missing site_id/DSP rule must fall back to legacy map")
	}
	if shouldRouteDSPByMaps("site-missing", "adl_dsp_x", "ssp", "US", store, legacyBlock) {
		t.Fatal("missing site_id/DSP rule must preserve legacy block")
	}
}

func TestSiteIDDSPLinkHTTPPutGetAndDebug(t *testing.T) {
	filename := writeSiteIDDSPMapForTest(t, `{}`)
	store, err := NewSiteIDDSPLinkStore(filename)
	if err != nil {
		t.Fatalf("NewSiteIDDSPLinkStore: %v", err)
	}

	router := chi.NewRouter()
	InitHttpRoutes(router, nil, "", "", nil, nil, "", "", nil, nil, "", "", nil, nil, store)

	body := `{"site-a, site-b":{"adl_dsp_x, adl_dsp_y":true,"adl_dsp_blocked":false}}`
	put := httptest.NewRequest(http.MethodPut, PutSiteIDDspLinksMapUrl, strings.NewReader(body))
	putRecorder := httptest.NewRecorder()
	router.ServeHTTP(putRecorder, put)
	if putRecorder.Code != http.StatusNoContent {
		t.Fatalf("PUT status=%d body=%s", putRecorder.Code, putRecorder.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, GetSiteIDDspLinksMapUrl, nil)
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRecorder.Code, getRecorder.Body.String())
	}
	var raw SiteIDDSPLinkMap
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if !raw["site-a, site-b"]["adl_dsp_x, adl_dsp_y"] || raw["site-a, site-b"]["adl_dsp_blocked"] {
		t.Fatalf("unexpected raw map: %#v", raw)
	}

	debug := httptest.NewRequest(http.MethodGet, GetDebugSiteIDDspLinksMapUrl, nil)
	debugRecorder := httptest.NewRecorder()
	router.ServeHTTP(debugRecorder, debug)
	if debugRecorder.Code != http.StatusOK {
		t.Fatalf("debug GET status=%d body=%s", debugRecorder.Code, debugRecorder.Body.String())
	}
	var expanded SiteIDDSPLinkMap
	if err := json.Unmarshal(debugRecorder.Body.Bytes(), &expanded); err != nil {
		t.Fatal(err)
	}
	if !expanded["site-a"]["adl_dsp_x"] || !expanded["site-b"]["adl_dsp_y"] {
		t.Fatalf("unexpected expanded allow map: %#v", expanded)
	}
	if expanded["site-a"]["adl_dsp_blocked"] || expanded["site-b"]["adl_dsp_blocked"] {
		t.Fatalf("unexpected expanded block map: %#v", expanded)
	}
}

func TestSiteIDDSPLinkRejectsNonBooleanJSON(t *testing.T) {
	filename := writeSiteIDDSPMapForTest(t, `{"site-a":{"adl_dsp_x":25}}`)
	if _, err := NewSiteIDDSPLinkStore(filename); err == nil {
		t.Fatal("numeric value must be rejected; site_id/DSP values are boolean only")
	}
}
