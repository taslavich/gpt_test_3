package dspRouterWeb

import (
	"errors"
	"testing"

	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestSuccessfulADVBidResponseErrorAlwaysWins(t *testing.T) {
	response := &advGrpc.DoAuctionResponse{BidResponse: &ortb.BidResponse{}}
	if got := successfulADVBidResponse(response, errors.New("ADV failed")); got != nil {
		t.Fatal("ADV response must be ignored when the call returned an error")
	}
}

func TestSuccessfulADVBidResponseReturnsBidOnlyWithoutError(t *testing.T) {
	want := &ortb.BidResponse{}
	response := &advGrpc.DoAuctionResponse{BidResponse: want}
	if got := successfulADVBidResponse(response, nil); got != want {
		t.Fatal("successful ADV response was not returned")
	}
	if got := successfulADVBidResponse(nil, nil); got != nil {
		t.Fatal("nil ADV response must fall through to DSP")
	}
}
