package web

import (
	"context"
	"net/http"
	"testing"

	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDoAuctionNilRequestReturns400(t *testing.T) {
	s := NewServer(nil, nil, nil, 0)
	resp, err := s.DoAuction(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCode() != http.StatusBadRequest {
		t.Fatalf("code=%d", resp.GetCode())
	}
}

func TestDoAuctionDisabledUnavailable(t *testing.T) {
	s := NewServer(nil, nil, nil, 0)
	_ = s.WorkController().Set(false)
	_, err := s.DoAuction(context.Background(), &advGrpc.DoAuctionRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}
