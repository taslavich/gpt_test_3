package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"sync/atomic"
	"time"

	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const disabledMessage = "ADV service is temporarily disabled after Redis write failure"

type WorkController struct {
	enabled atomic.Bool
}

func NewWorkController() *WorkController {
	controller := &WorkController{}
	controller.enabled.Store(true)
	return controller
}

func (c *WorkController) Set(enabled bool) error {
	if c == nil {
		return errors.New("ADV work controller is nil")
	}
	c.enabled.Store(enabled)
	return nil
}

func (c *WorkController) Enabled() bool {
	return c != nil && c.enabled.Load()
}

type Server struct {
	advGrpc.UnimplementedAdvServiceServer
	auctionService *auction.AuctionService
	work           *WorkController
}

func NewServer(auctionService *auction.AuctionService, work *WorkController) *Server {
	return &Server{auctionService: auctionService, work: work}
}

func (s *Server) DoAuction(ctx context.Context, req *advGrpc.DoAuctionRequest) (resp *advGrpc.DoAuctionResponse, funcErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			resp = nil
			funcErr = status.Error(codes.Internal, fmt.Sprintf("ADV auction panic: %v\n%s", recovered, debug.Stack()))
		}
	}()

	log.Println("[INFO] ADV auction: request received")

	if s == nil || s.work == nil || !s.work.Enabled() {
		log.Println("[INFO] ADV auction: service disabled")
		return nil, status.Error(codes.Unavailable, disabledMessage)
	}

	if req == nil || req.GetBidRequest() == nil || len(req.GetBidRequest().GetImp()) == 0 {
		log.Println("[INFO] ADV auction: invalid request - no impressions")
		return nil, status.Error(codes.InvalidArgument, "ADV auction request must contain at least one impression")
	}

	if s.auctionService == nil {
		log.Println("[INFO] ADV auction: auction service unavailable")
		return nil, status.Error(codes.Unavailable, "ADV auction service is unavailable")
	}

	log.Printf("[INFO] ADV auction: processing request with format=%s, traffic_type=%s, ssp_domain=%s, impressions=%d",
		req.GetFormat(), req.GetTrafficType(), req.GetSspDomain(), len(req.GetBidRequest().GetImp()))

	outcome, err := s.auctionService.Auction(ctx, req.GetBidRequest(), time.Now().UTC(), auction.AuctionRequestOptions{
		Format:      req.GetFormat(),
		TrafficType: req.GetTrafficType(),
		SSPDomain:   req.GetSspDomain(),
		ImpIDUUID:   req.GetImpIdUuid(),
	})

	if errors.Is(err, auction.ErrInvalidAuctionRequest) {
		log.Printf("[INFO] ADV auction: invalid request: %v", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err != nil {
		log.Printf("[INFO] ADV auction: failed: %v", err)
		return nil, status.Error(codes.Unavailable, fmt.Sprintf("ADV auction failed: %v", err))
	}

	if outcome == nil || !hasBids(outcome.BidResponse) {
		log.Println("[INFO] ADV auction: completed - no bids")
		return &advGrpc.DoAuctionResponse{
			WinnerUserIds: map[string]string{},
		}, nil
	}

	log.Printf("[INFO] ADV auction: completed successfully - %d bids", len(outcome.BidResponse.GetSeatbid()))
	return &advGrpc.DoAuctionResponse{
		BidResponse:   outcome.BidResponse,
		WinnerUserIds: outcome.WinnerUserIDs,
	}, nil
}

func hasBids(response *ortb.BidResponse) bool {
	if response == nil {
		return false
	}
	for _, seat := range response.GetSeatbid() {
		if seat != nil && len(seat.GetBid()) > 0 {
			return true
		}
	}
	return false
}
