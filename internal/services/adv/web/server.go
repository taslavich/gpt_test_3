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
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
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

	if utils.ShouldSSPDomain(req.SspDomain) {
		return nil, status.Error(codes.Unavailable, disabledMessage)
	}

	requestID := ""
	if req != nil && req.GetBidRequest() != nil {
		requestID = req.GetBidRequest().GetId()
	}

	log.Printf(
		"[ADV][REQUEST] request_id=%q format=%q traffic_type=%q ssp_domain=%q impressions=%d imp_uuid_count=%d",
		requestID,
		req.GetFormat(),
		req.GetTrafficType(),
		req.GetSspDomain(),
		len(req.GetBidRequest().GetImp()),
		len(req.GetImpIdUuid()),
	)

	if s == nil || s.work == nil || !s.work.Enabled() {
		log.Printf("[ADV][REQUEST_REJECT] request_id=%q reason=service_disabled", requestID)
		return nil, status.Error(codes.Unavailable, disabledMessage)
	}

	if req == nil || req.GetBidRequest() == nil || len(req.GetBidRequest().GetImp()) == 0 {
		log.Printf("[ADV][REQUEST_REJECT] request_id=%q reason=no_impressions", requestID)
		return nil, status.Error(codes.InvalidArgument, "ADV auction request must contain at least one impression")
	}

	if s.auctionService == nil {
		log.Printf("[ADV][REQUEST_ERROR] request_id=%q reason=auction_service_unavailable", requestID)
		return nil, status.Error(codes.Unavailable, "ADV auction service is unavailable")
	}

	outcome, err := s.auctionService.Auction(ctx, req.GetBidRequest(), time.Now().UTC(), auction.AuctionRequestOptions{
		Format:      req.GetFormat(),
		TrafficType: req.GetTrafficType(),
		SSPDomain:   req.GetSspDomain(),
		ImpIDUUID:   req.GetImpIdUuid(),
	})

	if errors.Is(err, auction.ErrInvalidAuctionRequest) {
		log.Printf("[ADV][REQUEST_REJECT] request_id=%q reason=invalid_auction_request error=%v", requestID, err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err != nil {
		log.Printf("[ADV][REQUEST_ERROR] request_id=%q reason=auction_failed error=%v", requestID, err)
		return nil, status.Error(codes.Unavailable, fmt.Sprintf("ADV auction failed: %v", err))
	}

	if outcome == nil || !hasBids(outcome.BidResponse) {
		winnerUsers := 0
		if outcome != nil {
			winnerUsers = len(outcome.WinnerUserIDs)
		}
		log.Printf(
			"[ADV][RESPONSE_NO_BID] request_id=%q winner_user_ids=%d",
			requestID,
			winnerUsers,
		)
		return &advGrpc.DoAuctionResponse{
			WinnerUserIds: map[string]string{},
		}, nil
	}

	log.Printf(
		"[ADV][RESPONSE_SUCCESS] request_id=%q seatbids=%d bids=%d winner_user_ids=%d",
		requestID,
		len(outcome.BidResponse.GetSeatbid()),
		countBids(outcome.BidResponse),
		len(outcome.WinnerUserIDs),
	)
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

func countBids(response *ortb.BidResponse) int {
	if response == nil {
		return 0
	}
	count := 0
	for _, seat := range response.GetSeatbid() {
		if seat != nil {
			count += len(seat.GetBid())
		}
	}
	return count
}
