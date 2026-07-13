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
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
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
	admDomain          string
	work               *WorkController
}

func NewServer(auctionService *auction.AuctionService, runtimeRedisClient, winnerRedisClient *redis.Client, winnerTTL time.Duration, admDomain ...string) *Server {
	if winnerTTL <= 0 {
		winnerTTL = 45 * time.Minute
	}
	domain := ""
	if len(admDomain) > 0 {
		domain = admDomain[0]
	}
	return &Server{auctionService: auctionService, runtimeRedisClient: runtimeRedisClient, winnerRedisClient: winnerRedisClient, winnerTTL: winnerTTL, admDomain: domain, work: NewWorkController(true)}
}
func (s *Server) WorkController() *WorkController { return s.work }

func (s *Server) DoAuction(ctx context.Context, req *advGrpc.DoAuctionRequest) (*advGrpc.DoAuctionResponse, error) {
	if s.work != nil && !s.work.Enabled() {
		return nil, status.Error(codes.Unavailable, "ADV service is temporarily disabled after Redis write failure")
	}
	if req == nil || req.GetBidRequest() == nil {
		return &advGrpc.DoAuctionResponse{Code: http.StatusBadRequest}, nil
	}
	if len(req.GetBidRequest().GetImp()) == 0 {
		return &advGrpc.DoAuctionResponse{Code: http.StatusNoContent}, nil
	}
	if s.auctionService == nil || s.winnerRedisClient == nil {
		return &advGrpc.DoAuctionResponse{Code: http.StatusServiceUnavailable}, nil
	}

	br := &ortb_V2_5.BidResponse{}
	id := uuid.NewString()
	cur := "USD"
	br.Id = &id
	br.Cur = &cur
	br.Seatbid = []*ortb_V2_5.SeatBid{{}}
	winnerUserIDs := make(map[string]string)
	for _, imp := range req.GetBidRequest().GetImp() {
		if imp == nil {
			continue
		}
		impID := imp.GetId()
		impUUID := req.GetImpIdUuid()[impID]
		if impUUID == "" {
			continue
		}
		bidReq := req.GetBidRequest()
		if cloned, ok := proto.Clone(req.GetBidRequest()).(*ortb_V2_5.BidRequest); ok {
			cloned.Imp = []*ortb_V2_5.Imp{imp}
			bidReq = cloned
		}
		candidates := s.auctionService.SelectAuctionCandidates(ctx, bidReq, time.Now(), auction.AuctionRequestOptions{Format: req.GetFormat(), TrafficType: req.GetTrafficType(), SSPDomain: req.GetSspDomain()})
		for _, result := range candidates {
			if result == nil || result.Campaign == nil || result.Creative == nil {
				continue
			}
			charge := result.Campaign.ChargePriceForFormat(req.GetFormat())
			if charge <= 0 {
				continue
			}
			fields := map[string]any{"price": strconv.FormatFloat(charge, 'f', -1, 64), "user_id": result.Campaign.UserID, "campaign_id": result.Campaign.ID, "format": req.GetFormat()}
			if err := s.winnerRedisClient.HSet(ctx, impUUID, fields).Err(); err != nil {
				continue
			}
			if err := s.winnerRedisClient.Expire(ctx, impUUID, s.winnerTTL).Err(); err != nil {
				_ = s.winnerRedisClient.Del(ctx, impUUID).Err()
				continue
			}
			adm, nurl, burl := s.wrapBidURLs(result, impUUID, req)
			bid := buildBid(req.GetBidRequest(), imp, result.Campaign, result.Creative, adm, result.Campaign.BasePrice)
			if nurl != "" {
				bid.Nurl = &nurl
			}
			if burl != "" {
				bid.Burl = &burl
			}
			br.Seatbid[0].Bid = append(br.Seatbid[0].Bid, bid)
			winnerUserIDs[impID] = result.Campaign.UserID
			break
		}
	}
	if len(br.GetSeatbid()) == 0 || len(br.GetSeatbid()[0].GetBid()) == 0 {
		return &advGrpc.DoAuctionResponse{Code: http.StatusNoContent}, nil
	}
	return &advGrpc.DoAuctionResponse{Selected: true, Code: http.StatusOK, BidResponse: br, WinnerUserIds: winnerUserIDs}, nil
}

func (s *Server) wrapBidURLs(result *auction.AuctionResult, impUUID string, req *advGrpc.DoAuctionRequest) (adm, nurl, burl string) {
	adm = result.ADM
	format := req.GetFormat()
	if s.admDomain == "" {
		return adm, "", ""
	}
	if adm != "" && (format == "ipp" || format == "in-page-push" || format == "in_page_push") {
		adm = utils.WrapURL(s.admDomain, adm, impUUID, format)
	}
	if result.Creative != nil && result.Creative.ADMURL != "" {
		nurl = utils.WrapNurlURL(s.admDomain, result.Creative.ADMURL, impUUID, req.GetSspDomain(), format)
	}
	if format == "pop" || format == "popunder" || format == "nat" || format == "native" || format == "ban" || format == "banner" {
		burl = utils.WrapBurlURL(s.admDomain, impUUID, format)
	}
	return adm, nurl, burl
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
