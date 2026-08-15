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

func TestADVPercentMapRoutesUseOneFlatMapWithoutTypic(t *testing.T) {
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

	put := httptest.NewRequest(http.MethodPut, GetADVPercentMapURL, strings.NewReader(`{" USER-ID ":0.25}`))
	putRecorder := httptest.NewRecorder()
	router.ServeHTTP(putRecorder, put)
	if putRecorder.Code != http.StatusNoContent {
		t.Fatalf("PUT status=%d body=%s", putRecorder.Code, putRecorder.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, GetDebugADVPercentMapURL, nil)
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRecorder.Code, getRecorder.Body.String())
	}

	var got auction.PercentMap
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["user-id"] != 0.25 {
		t.Fatalf("unexpected map: %#v", got)
	}
}
