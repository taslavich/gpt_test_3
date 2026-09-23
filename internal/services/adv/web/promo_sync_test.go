package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

func TestPromoSpendRemainingControlEndpoint(t *testing.T) {
	service := auction.NewAuctionService(nil, nil, nil, nil, nil)
	router := chi.NewRouter()
	InitHttpRoutes(router, nil, nil, nil, NewWorkController(), AntiPerekrutHTTPConfig{AuctionService: service})

	req := httptest.NewRequest(http.MethodPut, PutPromoSpendRemainingURL, bytes.NewBufferString(`{"user_id":"user-1","remaining":0,"revision":11,"generation":4}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}

	badReq := httptest.NewRequest(http.MethodPut, PutPromoSpendRemainingURL, bytes.NewBufferString(`{"user_id":"","remaining":0,"revision":11,"generation":4}`))
	badResponse := httptest.NewRecorder()
	router.ServeHTTP(badResponse, badReq)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("bad status=%d body=%q", badResponse.Code, badResponse.Body.String())
	}
}
