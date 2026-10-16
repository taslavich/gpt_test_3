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
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func writeEndpointMapForTest(t *testing.T, filename string, mapa DSPEndpointMap) {
	t.Helper()
	data, err := json.MarshalIndent(mapa, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filename, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDSPEndpointStoreUpdatePersistsAndPublishesCompleteSnapshot(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "dsp.json")
	writeEndpointMapForTest(t, filename, DSPEndpointMap{
		"https://old.example/bid": "adl_old.example",
	})

	rules := filter.NewRuleManager()
	rules.SetDSPRules("new.example|v25", nil, []*filter.FilterRule{{Field: filter.FieldNativeTitleLen}})
	processor := filter.NewOptimizedFilterProcessor(rules)
	store, err := NewDSPEndpointStore(filename, sppAdapterWeb.ADULT, processor)
	if err != nil {
		t.Fatalf("NewDSPEndpointStore: %v", err)
	}

	updated := DSPEndpointMap{
		"https://z.example/bid": "adl_z.example",
		"https://a.example/bid": "adl_new.example",
	}
	if err := store.Update(updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	snapshot := store.runtimeSnapshot()
	if len(snapshot.ordered) != 2 {
		t.Fatalf("ordered endpoints=%v", snapshot.ordered)
	}
	if snapshot.ordered[0].Domain != "adl_new.example" || snapshot.ordered[0].client == nil {
		t.Fatalf("first runtime endpoint was not fully prepared: %#v", snapshot.ordered[0])
	}
	if snapshot.ordered[1].Domain != "adl_z.example" || snapshot.ordered[1].client == nil {
		t.Fatalf("second runtime endpoint was not fully prepared: %#v", snapshot.ordered[1])
	}
	wantMask := filter.NativeMaskForField(filter.FieldNativeTitleLen)
	if snapshot.nativeMask != wantMask {
		t.Fatalf("native mask=%v want %v", snapshot.nativeMask, wantMask)
	}

	onDisk, err := store.ReadRaw()
	if err != nil {
		t.Fatalf("ReadRaw: %v", err)
	}
	if len(onDisk) != 2 || onDisk["https://a.example/bid"] != "adl_new.example" {
		t.Fatalf("disk map=%v", onDisk)
	}
}

func TestDSPEndpointStoreRejectsInvalidUpdateWithoutChangingDiskOrRuntime(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "dsp.json")
	initial := DSPEndpointMap{"https://old.example/bid": "adl_old.example"}
	writeEndpointMapForTest(t, filename, initial)

	store, err := NewDSPEndpointStore(filename, sppAdapterWeb.ADULT, nil)
	if err != nil {
		t.Fatalf("NewDSPEndpointStore: %v", err)
	}
	if err := store.Update(DSPEndpointMap{"https://bad.example/bid": "mc_wrong.example"}); err == nil {
		t.Fatal("expected typic/domain validation error")
	}

	runtime := store.Snapshot()
	if len(runtime) != 1 || runtime["https://old.example/bid"] != "adl_old.example" {
		t.Fatalf("runtime changed after rejected update: %v", runtime)
	}
	onDisk, err := store.ReadRaw()
	if err != nil {
		t.Fatalf("ReadRaw: %v", err)
	}
	if len(onDisk) != 1 || onDisk["https://old.example/bid"] != "adl_old.example" {
		t.Fatalf("disk changed after rejected update: %v", onDisk)
	}
}

func TestDSPEndpointStoresShareClientsByDomainAcrossFormats(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.json")
	two := filepath.Join(dir, "two.json")
	mapa := DSPEndpointMap{"https://one.example/bid": "adl_shared.example"}
	writeEndpointMapForTest(t, one, mapa)
	writeEndpointMapForTest(t, two, DSPEndpointMap{"https://two.example/bid": "adl_shared.example"})

	registry := NewDSPHTTPClientRegistry()
	storeOne, err := NewDSPEndpointStore(one, sppAdapterWeb.ADULT, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	storeTwo, err := NewDSPEndpointStore(two, sppAdapterWeb.ADULT, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	clientOne := storeOne.runtimeSnapshot().ordered[0].client
	clientTwo := storeTwo.runtimeSnapshot().ordered[0].client
	if clientOne == nil || clientOne != clientTwo {
		t.Fatal("same DSP domain must reuse one HTTP client across format stores")
	}
}

func TestFormatRoutesHotUpdateChangesOnlySelectedFormatAndTypic(t *testing.T) {
	dir := t.TempDir()
	popAdultFile := filepath.Join(dir, "pop-adult.json")
	banAdultFile := filepath.Join(dir, "ban-adult.json")
	writeEndpointMapForTest(t, popAdultFile, DSPEndpointMap{"https://pop-old.example/bid": "adl_pop_old.example"})
	writeEndpointMapForTest(t, banAdultFile, DSPEndpointMap{"https://ban.example/bid": "adl_ban.example"})

	registry := NewDSPHTTPClientRegistry()
	popAdult, err := NewDSPEndpointStore(popAdultFile, sppAdapterWeb.ADULT, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	banAdult, err := NewDSPEndpointStore(banAdultFile, sppAdapterWeb.ADULT, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	routes := &FormatRoutesV25{
		POP: FormatRouteV25{AdultStore: popAdult},
		BAN: FormatRouteV25{AdultStore: banAdult},
	}

	if err := popAdult.Update(DSPEndpointMap{"https://pop-new.example/bid": "adl_pop_new.example"}); err != nil {
		t.Fatal(err)
	}
	pop, _, _ := routes.selectRuntime(constants.POP, sppAdapterWeb.ADULT)
	ban, _, _ := routes.selectRuntime(constants.BAN, sppAdapterWeb.ADULT)
	if len(pop) != 1 || pop[0].Domain != "adl_pop_new.example" {
		t.Fatalf("POP runtime not updated: %v", pop)
	}
	if len(ban) != 1 || ban[0].Domain != "adl_ban.example" {
		t.Fatalf("BAN runtime changed unexpectedly: %v", ban)
	}
}

func TestDSPEndpointsHTTPPutReplacesOnlyRequestedMap(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "pop-adult.json")
	writeEndpointMapForTest(t, filename, DSPEndpointMap{"https://old.example/bid": "adl_old.example"})
	store, err := NewDSPEndpointStore(filename, sppAdapterWeb.ADULT, nil)
	if err != nil {
		t.Fatal(err)
	}
	routes := &FormatRoutesV25{POP: FormatRouteV25{AdultStore: store}}

	router := chi.NewRouter()
	InitHttpRoutes(router, routes, "", "", nil, nil, "", "", nil, nil, "", "", nil, nil, "", "", nil, nil)

	body := strings.NewReader(`{"https://new.example/bid":"adl_new.example"}`)
	req := httptest.NewRequest(http.MethodPut, PutDspEndpointsMapURL+"?typic=ADULT&format=POP", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("PUT status=%d body=%s", resp.Code, resp.Body.String())
	}
	if got := store.Snapshot(); len(got) != 1 || got["https://new.example/bid"] != "adl_new.example" {
		t.Fatalf("runtime after PUT=%v", got)
	}

	debugReq := httptest.NewRequest(http.MethodGet, GetDebugDspEndpointsMapURL+"?typic=ADULT&format=POP", nil)
	debugResp := httptest.NewRecorder()
	router.ServeHTTP(debugResp, debugReq)
	if debugResp.Code != http.StatusOK || !strings.Contains(debugResp.Body.String(), "adl_new.example") {
		t.Fatalf("debug GET status=%d body=%s", debugResp.Code, debugResp.Body.String())
	}
}
