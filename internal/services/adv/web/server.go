package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	ortb_V2_5 "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

// Server exposes the ADV auction service over gRPC.
type Server struct {
	advGrpc.UnimplementedAdvServiceServer
	auctionService                      *auction.AuctionService
	userBalanceThresholdRedisClient     *redis.Client
	userBalanceSpentRedisClient         *redis.Client
	campaignBalanceThresholdRedisClient *redis.Client
	campaignBalanceSpentRedisClient     *redis.Client
}

// NewServer creates a gRPC server that delegates auction decisions to AuctionService.
func NewServer(
	auctionService *auction.AuctionService,
	userBalanceThresholdRedisClient *redis.Client,
	userBalanceSpentRedisClient *redis.Client,
	campaignBalanceThresholdRedisClient *redis.Client,
	campaignBalanceSpentRedisClient *redis.Client,
) *Server {
	return &Server{
		auctionService:                      auctionService,
		userBalanceThresholdRedisClient:     userBalanceThresholdRedisClient,
		userBalanceSpentRedisClient:         userBalanceSpentRedisClient,
		campaignBalanceThresholdRedisClient: campaignBalanceThresholdRedisClient,
		campaignBalanceSpentRedisClient:     campaignBalanceSpentRedisClient,
	}
}

// DoAuction accepts an OpenRTB bid request and runs the ADV auction selection logic.
func (s *Server) DoAuction(ctx context.Context, req *advGrpc.DoAuctionRequest) (*advGrpc.DoAuctionResponse, error) {
	if req == nil || req.GetBidRequest() == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusBadRequest}, nil
	}
	if s.auctionService == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusServiceUnavailable}, nil
	}

	auctionResult := s.auctionService.SelectAuction(req.GetBidRequest(), time.Now(), auction.AuctionRequestOptions{
		Format:      req.GetFormat(),
		TrafficType: req.GetTrafficType(),
		SSPDomain:   req.GetSspDomain(),
	})
	if auctionResult == nil || auctionResult.Campaign == nil || auctionResult.Creative == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusNoContent}, nil
	}

	campaign := auctionResult.Campaign
	creative := auctionResult.Creative
	if err := s.ensurePositiveBalances(ctx, campaign.UserID, campaign.ID); err != nil {
		if errors.Is(err, redis.Nil) || errors.Is(err, errUserBalanceNotPositive) {
			return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusNoContent}, nil
		}

		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusServiceUnavailable}, nil
	}

	bidResponse := buildBidResponse(req.GetBidRequest(), campaign, creative, auctionResult.ADM, auctionResult.AuctionPrice)

	return &advGrpc.DoAuctionResponse{
		Selected:     true,
		CampaignId:   campaign.ID,
		CreativeId:   creative.ID,
		Adm:          auctionResult.ADM,
		AuctionPrice: auctionResult.AuctionPrice,
		Code:         http.StatusOK,
		BidResponse:  bidResponse,
	}, nil
}

var errUserBalanceNotPositive = errors.New("balance is not positive")

func (s *Server) ensurePositiveBalances(ctx context.Context, userID, campaignID string) error {
	if userID == "" {
		return fmt.Errorf("campaign user_id is empty")
	}
	if campaignID == "" {
		return fmt.Errorf("campaign_id is empty")
	}

	if err := ensurePositiveRemainingBalance(ctx, s.userBalanceThresholdRedisClient, s.userBalanceSpentRedisClient, userID); err != nil {
		return err
	}

	return ensurePositiveRemainingBalance(ctx, s.campaignBalanceThresholdRedisClient, s.campaignBalanceSpentRedisClient, campaignID)
}

func ensurePositiveRemainingBalance(ctx context.Context, thresholdClient, spentClient *redis.Client, balanceKey string) error {
	threshold, err := redisFloatValue(ctx, thresholdClient, balanceKey, "threshold")
	if err != nil {
		return err
	}

	spent, err := redisFloatValue(ctx, spentClient, balanceKey, "spent")
	if err != nil {
		return err
	}

	if threshold-spent <= 0 {
		return errUserBalanceNotPositive
	}

	return nil
}

func redisFloatValue(ctx context.Context, client *redis.Client, key string, valueName string) (float64, error) {
	if client == nil {
		return 0, fmt.Errorf("redis %s client is nil", valueName)
	}

	valueRaw, err := client.Get(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	value, err := strconv.ParseFloat(valueRaw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse redis %s for key %s: %w", valueName, key, err)
	}

	return value, nil
}
func buildBidResponse(req *ortb_V2_5.BidRequest, campaign *auction.Campaign, creative *auction.Creative, adm string, price float64) *ortb_V2_5.BidResponse {
	if req == nil || campaign == nil || creative == nil {
		return nil
	}

	bidID := req.GetId()
	impID := ""
	if len(req.GetImp()) > 0 {
		impID = req.GetImp()[0].GetId()
	}
	price32 := float32(price)
	cid := campaign.ID
	crid := creative.ID
	adomain := []string{campaign.CampaignName}
	w := int32(creative.W)
	h := int32(creative.H)
	cur := "USD"

	return &ortb_V2_5.BidResponse{
		Id:    &bidID,
		Bidid: &bidID,
		Cur:   &cur,
		Seatbid: []*ortb_V2_5.SeatBid{
			{
				Bid: []*ortb_V2_5.Bid{
					{
						Id:      &bidID,
						Impid:   &impID,
						Price:   &price32,
						Adm:     &adm,
						Adomain: adomain,
						Cid:     &cid,
						Crid:    &crid,
						W:       &w,
						H:       &h,
					},
				},
			},
		},
	}
}
