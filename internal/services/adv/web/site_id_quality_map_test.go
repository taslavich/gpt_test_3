package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

func TestSiteIDQualityMapPutReturnsDetailedValidationErrorAndPreservesState(t *testing.T) {
	path := t.TempDir() + "/site_id_quality.json"
	initial := []byte(`{
		"usual":{"isWhiteList":false,"siteIds":[]},
		"high":{"isWhiteList":true,"siteIds":["site-high"]},
		"ultra":{"isWhiteList":false,"siteIds":[]}
	}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := auction.NewSiteIDQualityStore(path)
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	InitHttpRoutes(router, nil, nil, store, NewWorkController())

	invalid := []byte(`{"usual":{"isWhiteList":false,"siteIds":[]},"high":{"isWhiteList":false,"siteIds":[]}}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, PutSiteIDQualityMapURL, bytes.NewReader(invalid))
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response must be JSON: %v body=%s", err, recorder.Body.String())
	}
	if !strings.Contains(response["error"], `map "ultra" is missing`) {
		t.Fatalf("validation reason missing from response body: %q", response["error"])
	}
	if !store.Allows("high", "site-high") || store.Allows("high", "site-other") {
		t.Fatal("invalid PUT changed runtime state")
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(persisted) != string(initial) {
		t.Fatal("invalid PUT changed persisted file")
	}
}

func TestSiteIDQualityMapPutReplacesAllThreeSegments(t *testing.T) {
	path := t.TempDir() + "/site_id_quality.json"
	initial := []byte(`{
		"usual":{"isWhiteList":false,"siteIds":[]},
		"high":{"isWhiteList":false,"siteIds":[]},
		"ultra":{"isWhiteList":false,"siteIds":[]}
	}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := auction.NewSiteIDQualityStore(path)
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	InitHttpRoutes(router, nil, nil, store, NewWorkController())
	valid := []byte(`{
		"usual":{"isWhiteList":true,"siteIds":["u1"]},
		"high":{"isWhiteList":true,"siteIds":["h1"]},
		"ultra":{"isWhiteList":false,"siteIds":["bad"]}
	}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, PutSiteIDQualityMapURL, bytes.NewReader(valid))
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !store.Allows("usual", "u1") || store.Allows("usual", "other") {
		t.Fatal("usual whitelist was not replaced")
	}
	if !store.Allows("high", "h1") || store.Allows("high", "other") {
		t.Fatal("high whitelist was not replaced")
	}
	if store.Allows("ultra", "bad") || !store.Allows("ultra", "good") {
		t.Fatal("ultra blacklist was not replaced")
	}
}
