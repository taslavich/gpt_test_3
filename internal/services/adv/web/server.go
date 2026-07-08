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

	campaign := s.auctionService.SelectCampaign(req.GetBidRequest(), time.Now())
	if campaign == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusNoContent}, nil
	}

	if err := s.ensurePositiveBalances(ctx, campaign.UserID, campaign.ID); err != nil {
		if errors.Is(err, redis.Nil) || errors.Is(err, errUserBalanceNotPositive) {
			return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusNoContent}, nil
		}

		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusServiceUnavailable}, nil
	}

	var creativeID, adm string
	if creative := campaign.FirstCreative(); creative != nil {
		creativeID = creative.ID
		adm = creative.ADMURL
	}

	return &advGrpc.DoAuctionResponse{
		Selected:     true,
		CampaignId:   campaign.ID,
		CreativeId:   creativeID,
		Adm:          adm,
		AuctionPrice: campaign.GetAuctionPrice(),
		Code:         http.StatusOK,
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
