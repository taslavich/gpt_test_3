package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	ortb_V2_5 "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	advGrpc.UnimplementedAdvServiceServer
	auctionService     *auction.AuctionService
	runtimeRedisClient *redis.Client
	winnerRedisClient  *redis.Client
	winnerTTL          time.Duration
	work               *WorkController
}

func NewServer(auctionService *auction.AuctionService, runtimeRedisClient, winnerRedisClient *redis.Client, winnerTTL time.Duration) *Server {
	if winnerTTL <= 0 {
		winnerTTL = 45 * time.Minute
	}
	return &Server{auctionService: auctionService, runtimeRedisClient: runtimeRedisClient, winnerRedisClient: winnerRedisClient, winnerTTL: winnerTTL, work: NewWorkController(true)}
}
func (s *Server) WorkController() *WorkController { return s.work }
func (s *Server) DoAuction(ctx context.Context, req *advGrpc.DoAuctionRequest) (*advGrpc.DoAuctionResponse, error) {
	if s.work != nil && !s.work.Enabled() {
		return nil, status.Error(codes.Unavailable, "ADV service is temporarily disabled after Redis write failure")
	}
	if req == nil || req.GetBidRequest() == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusBadRequest}, nil
	}
	if s.auctionService == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusServiceUnavailable}, nil
	}
	br := &ortb_V2_5.BidResponse{}
	id := uuid.NewString()
	cur := "USD"
	br.Id = &id
	br.Cur = &cur
	br.Seatbid = []*ortb_V2_5.SeatBid{{}}
	selectedCampaign := ""
	selectedCreative := ""
	selectedADM := ""
	selectedPrice := 0.0
	winnerUserIDs := make(map[string]string)
	for _, imp := range req.GetBidRequest().GetImp() {
		if imp == nil {
			continue
		}
		impUUID := req.GetImpIdUuid()[imp.GetId()]
		if impUUID == "" {
			continue
		}
		bidReq := req.GetBidRequest()
		if cloned, ok := proto.Clone(req.GetBidRequest()).(*ortb_V2_5.BidRequest); ok {
			cloned.Imp = []*ortb_V2_5.Imp{imp}
			bidReq = cloned
		}
		result := s.auctionService.SelectAuctionWithContext(ctx, bidReq, time.Now(), auction.AuctionRequestOptions{Format: req.GetFormat(), TrafficType: req.GetTrafficType(), SSPDomain: req.GetSspDomain()})
		if result == nil || result.Campaign == nil || result.Creative == nil {
			continue
		}
		charge := result.Campaign.ChargePriceForFormat(req.GetFormat())
		if charge <= 0 {
			continue
		}
		if s.winnerRedisClient != nil {
			if err := s.winnerRedisClient.HSet(ctx, impUUID, map[string]any{"price": strconv.FormatFloat(charge, 'f', -1, 64), "user_id": result.Campaign.UserID, "campaign_id": result.Campaign.ID, "format": req.GetFormat()}).Err(); err != nil {
				continue
			}
			_ = s.winnerRedisClient.Expire(ctx, impUUID, s.winnerTTL).Err()
		}
		bid := buildBid(req.GetBidRequest(), imp, result.Campaign, result.Creative, result.ADM, result.Campaign.BasePrice)
		br.Seatbid[0].Bid = append(br.Seatbid[0].Bid, bid)
		selectedCampaign = result.Campaign.ID
		selectedCreative = result.Creative.ID
		selectedADM = result.ADM
		selectedPrice = charge
		winnerUserIDs[imp.GetId()] = result.Campaign.UserID
	}
	if len(br.GetSeatbid()) == 0 || len(br.GetSeatbid()[0].GetBid()) == 0 {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusNoContent}, nil
	}
	return &advGrpc.DoAuctionResponse{Selected: true, CampaignId: selectedCampaign, CreativeId: selectedCreative, Adm: selectedADM, AuctionPrice: selectedPrice, Code: http.StatusOK, BidResponse: br, WinnerUserIds: winnerUserIDs}, nil
}
func buildBid(req *ortb_V2_5.BidRequest, imp *ortb_V2_5.Imp, campaign *auction.Campaign, creative *auction.Creative, adm string, price float64) *ortb_V2_5.Bid {
	bidID := creative.ID
	impID := imp.GetId()
	price32 := float32(price)
	cid := campaign.ID
	crid := creative.ID
	w := int32(creative.W)
	h := int32(creative.H)
	return &ortb_V2_5.Bid{Id: &bidID, Impid: &impID, Price: &price32, Adm: &adm, Cid: &cid, Crid: &crid, W: &w, H: &h}
}
func buildBidResponse(req *ortb_V2_5.BidRequest, campaign *auction.Campaign, creative *auction.Creative, adm string, price float64) *ortb_V2_5.BidResponse {
	if req == nil || len(req.GetImp()) == 0 {
		return nil
	}
	id := uuid.NewString()
	cur := "USD"
	return &ortb_V2_5.BidResponse{Id: &id, Cur: &cur, Seatbid: []*ortb_V2_5.SeatBid{{Bid: []*ortb_V2_5.Bid{buildBid(req, req.GetImp()[0], campaign, creative, adm, price)}}}}
}
func redisFloatValue(ctx context.Context, client *redis.Client, key string, valueName string) (float64, error) {
	if client == nil {
		return 0, fmt.Errorf("redis %s client is nil", valueName)
	}
	raw, err := client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(raw, 64)
}
