package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

func TestAuctionDiagnosticsRoutesReturnLocalSnapshotAndToggleState(t *testing.T) {
	service := auction.NewAuctionService(nil, nil, nil, nil, nil)
	router := chi.NewRouter()
	InitHttpRoutes(router, nil, nil, nil, NewWorkController(), AntiPerekrutHTTPConfig{AuctionService: service})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, GetAuctionDiagnosticsURL, nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected snapshot status: got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var snapshot auction.AuctionDiagnosticsSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode diagnostics response: %v", err)
	}
	if snapshot.Campaigns == nil || snapshot.Codebook["200"].Name != "bid_won" || snapshot.Enabled {
		t.Fatalf("unexpected diagnostics payload: %+v", snapshot)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, AuctionDiagnosticsStatusURL+"?enabled=true", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected enable status: got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var status auction.AuctionDiagnosticsStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode enable response: %v", err)
	}
	if !status.Enabled || status.CoveragePercent != 100 {
		t.Fatalf("unexpected enabled status: %+v", status)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, AuctionDiagnosticsStatusURL+"?enabled=false", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected disable status: got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode disable response: %v", err)
	}
	if status.Enabled {
		t.Fatalf("diagnostics stayed enabled: %+v", status)
	}
}
