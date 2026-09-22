package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

func TestADVPercentMapRoutesPreserveGroupedConfigAndExpandRuntime(t *testing.T) {
	filename := t.TempDir() + "/adv_percent_map.json"
	if err := os.WriteFile(filename, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := auction.NewPercentStore(filename)
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	InitHttpRoutes(router, store, nil, nil, NewWorkController())

	put := httptest.NewRequest(http.MethodPut, GetADVPercentMapURL, strings.NewReader(`{"123, 456,789":0.25}`))
	putRecorder := httptest.NewRecorder()
	router.ServeHTTP(putRecorder, put)
	if putRecorder.Code != http.StatusNoContent {
		t.Fatalf("PUT status=%d body=%s", putRecorder.Code, putRecorder.Body.String())
	}

	getPersisted := httptest.NewRequest(http.MethodGet, GetADVPercentMapURL, nil)
	persistedRecorder := httptest.NewRecorder()
	router.ServeHTTP(persistedRecorder, getPersisted)
	if persistedRecorder.Code != http.StatusOK {
		t.Fatalf("persisted GET status=%d body=%s", persistedRecorder.Code, persistedRecorder.Body.String())
	}
	var persisted auction.PercentMap
	if err := json.Unmarshal(persistedRecorder.Body.Bytes(), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted["123, 456,789"] != 0.25 || persisted[auction.PercentMapDefaultKey] != 0.20 || persisted[auction.PercentMapRTBDefaultKey] != 0.30 {
		t.Fatalf("unexpected persisted map: %#v", persisted)
	}
	if _, expanded := persisted["123"]; expanded {
		t.Fatalf("persisted map must retain grouped key: %#v", persisted)
	}

	getRuntime := httptest.NewRequest(http.MethodGet, GetDebugADVPercentMapURL, nil)
	runtimeRecorder := httptest.NewRecorder()
	router.ServeHTTP(runtimeRecorder, getRuntime)
	if runtimeRecorder.Code != http.StatusOK {
		t.Fatalf("runtime GET status=%d body=%s", runtimeRecorder.Code, runtimeRecorder.Body.String())
	}
	var runtime auction.PercentMap
	if err := json.Unmarshal(runtimeRecorder.Body.Bytes(), &runtime); err != nil {
		t.Fatal(err)
	}
	for _, campaignID := range []string{"123", "456", "789"} {
		if runtime[campaignID] != 0.25 {
			t.Fatalf("runtime[%s]=%v want 0.25; map=%#v", campaignID, runtime[campaignID], runtime)
		}
	}
}
