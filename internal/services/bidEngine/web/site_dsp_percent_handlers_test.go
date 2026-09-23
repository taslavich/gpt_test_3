package bidEngineWeb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	bidEngine "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
)

func TestSiteIDDspPercentHTTPPutGetAndDebug(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "site_id_dsp_percents.json")
	if err := os.WriteFile(filename, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := bidEngine.NewStore(filename)
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	InitHttpRoutes(router, nil, store)

	body := `{"site-a, site-b":{"dsp-x, dsp-y":0.25,"dsp-z":0.4}}`
	put := httptest.NewRequest(http.MethodPut, PutSiteIDDspPercentsMapUrl, strings.NewReader(body))
	put.Header.Set("Content-Type", "application/json")
	putRecorder := httptest.NewRecorder()
	router.ServeHTTP(putRecorder, put)
	if putRecorder.Code != http.StatusNoContent {
		t.Fatalf("PUT status=%d body=%s", putRecorder.Code, putRecorder.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, GetSiteIDDspPercentsMapUrl, nil)
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRecorder.Code, getRecorder.Body.String())
	}
	var raw bidEngine.Map
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if got := raw["site-a, site-b"]["dsp-x, dsp-y"]; got != 0.25 {
		t.Fatalf("raw grouped percent=%v want 0.25", got)
	}

	debug := httptest.NewRequest(http.MethodGet, GetDebugSiteIDDspPercentsMapUrl, nil)
	debugRecorder := httptest.NewRecorder()
	router.ServeHTTP(debugRecorder, debug)
	if debugRecorder.Code != http.StatusOK {
		t.Fatalf("debug GET status=%d body=%s", debugRecorder.Code, debugRecorder.Body.String())
	}
	var expanded bidEngine.Map
	if err := json.Unmarshal(debugRecorder.Body.Bytes(), &expanded); err != nil {
		t.Fatal(err)
	}
	if got := expanded["site-b"]["dsp-y"]; got != 0.25 {
		t.Fatalf("expanded percent=%v want 0.25", got)
	}
}

func TestSiteIDDspPercentHTTPRejectsWholeNumberPercent(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "site_id_dsp_percents.json")
	if err := os.WriteFile(filename, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := bidEngine.NewStore(filename)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	InitHttpRoutes(router, nil, store)

	put := httptest.NewRequest(http.MethodPut, PutSiteIDDspPercentsMapUrl, strings.NewReader(`{"site":{"dsp":25}}`))
	put.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, put)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT invalid percent status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
