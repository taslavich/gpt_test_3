package bidEngineWeb

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/redis/go-redis/v9"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	bidEngine "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	ProfitPercent              float32
	redisClients               []*redis.Client
	redisAdmClient             *redis.Client
	redisBurlClient            *redis.Client
	redisUUIDKeyTTL            time.Duration
	redisSetOrtb               string
	timeout                    time.Duration
	percentFilename_adult      string
	percentFilename_mainstream string
	percentMap_adult           *map[string]map[string]map[string]*types.PercentAndBidfloor
	percentMap_mainstream      *map[string]map[string]map[string]*types.PercentAndBidfloor
	admDomain                  string
	redisWriteErrorMonitor     *services.RedisWriteErrorMonitor

	GetWinnerBidInternal_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		ImpIdUuid map[string]string,
		percentMapAdult *map[string]map[string]map[string]*types.PercentAndBidfloor,
		percentMapMainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
		logged bool,
		typic string,
		admDomain string,
	) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string)

	pb.BidEngineServiceServer
}

func NewServer(
	ProfitPercent float32,
	redisClients []*redis.Client,
	redisAdmClient *redis.Client,
	redisBurlClient *redis.Client,
	redisUUIDKeyTTL time.Duration,
	redisSetOrtb string,
	GetWinnerBidInternal_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		ImpIdUuid map[string]string,
		percentMapAdult *map[string]map[string]map[string]*types.PercentAndBidfloor,
		percentMapMainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
		logged bool,
		typic string,
		admDomain string,
	) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string),
	percentFilename_adult string,
	percentMap_adult *map[string]map[string]map[string]*types.PercentAndBidfloor,

	percentFilename_mainstream string,
	percentMap_mainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,

	admDomain string,
	redisWriteErrorMonitor *services.RedisWriteErrorMonitor,
) *Server {
	return &Server{
		ProfitPercent:              ProfitPercent,
		redisClients:               redisClients,
		redisAdmClient:             redisAdmClient,
		redisBurlClient:            redisBurlClient,
		redisUUIDKeyTTL:            redisUUIDKeyTTL,
		redisSetOrtb:               redisSetOrtb,
		GetWinnerBidInternal_V_2_5: GetWinnerBidInternal_V_2_5,
		percentFilename_adult:      percentFilename_adult,
		percentFilename_mainstream: percentFilename_mainstream,
		percentMap_adult:           percentMap_adult,
		percentMap_mainstream:      percentMap_mainstream,
		admDomain:                  admDomain,
		redisWriteErrorMonitor:     redisWriteErrorMonitor,
	}
}

func (s *Server) GetWinnerBid_V2_5(
	ctx context.Context,
	req *bidEngineGrpc.BidEngineRequest_V2_5,
) (
	resp *bidEngineGrpc.BidEngineResponse_V2_5,
	funcErr error,
) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetWinnerBid_V2_5: %v, %s", r, string(debug.Stack()))
			log.Printf("Error: %v", err.Error())
			resp = nil
			funcErr = status.Error(codes.Internal, err.Error())
		}
	}()
	if s == nil {
		return nil, status.Error(codes.Unavailable, "bid engine service is unavailable")
	}
	if req == nil || req.GetBidRequest() == nil {
		return nil, status.Error(codes.InvalidArgument, "bid engine request is empty")
	}
	if req.GetRekl() {
		return s.handleReadyADVResponse(ctx, req), nil
	}
	if s.GetWinnerBidInternal_V_2_5 == nil {
		return nil, status.Error(codes.Internal, "bid engine auction function is nil")
	}

	bidResponse, clickhouseBid, burlUUIDs, admUUIDs := s.GetWinnerBidInternal_V_2_5(
		ctx,
		req,
		s.ProfitPercent,
		req.ImpIdUuid,
		s.percentMap_adult,
		s.percentMap_mainstream,
		req.Logged,
		req.Typic,
		s.admDomain,
	)

	for _, uuid := range admUUIDs {
		if err := utils.WriteUUIDKeyToRedis(ctx, s.redisAdmClient, uuid, s.redisUUIDKeyTTL); err != nil {
			log.Printf("failed to write ADM UUID key in GetWinnerBidInternal: %v", err)
			if s.redisWriteErrorMonitor != nil {
				s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
			}
		}
	}

	for _, uuid := range burlUUIDs {
		if err := utils.WriteUUIDKeyToRedis(ctx, s.redisBurlClient, uuid, s.redisUUIDKeyTTL); err != nil {
			log.Printf("failed to write BURL UUID key in GetWinnerBidInternal: %v", err)
			if s.redisWriteErrorMonitor != nil {
				s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
			}
		}
	}

	impIdUuidClone := make(map[string]string, len(req.ImpIdUuid))
	for impID, uuid := range req.ImpIdUuid {
		impIdUuidClone[impID] = uuid
	}

	failedImpIds := make([]string, 0)
	for impID, uuid := range req.ImpIdUuid {
		if err := utils.WriteWinStats(ctx, s.redisClients, uuid, clickhouseBid[uuid], req.Logged); err != nil {
			log.Printf("failed to WriteJsonToRedis Bid BID_RESPONSE_WINNER in GetWinnerBidInternal: %v", err)
			if s.redisWriteErrorMonitor != nil {
				s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
			}
			failedImpIds = append(failedImpIds, impID)
			delete(impIdUuidClone, impID)
			continue
		}
	}

	if len(impIdUuidClone) == 0 {
		return &bidEngineGrpc.BidEngineResponse_V2_5{
			BidResponse: nil,
			Code:        http.StatusNoContent,
		}, nil
	}

	if !bidResponseHasBids(bidResponse) {
		return &bidEngineGrpc.BidEngineResponse_V2_5{
			BidResponse: nil,
			Code:        http.StatusNoContent,
			Rekl:        false,
		}, nil
	}

	return &bidEngineGrpc.BidEngineResponse_V2_5{
		BidResponse:    bidResponse,
		Code:           http.StatusOK,
		FailedImpIds:   failedImpIds,
		ImpIdUuidClone: impIdUuidClone,
		Rekl:           false,
		WinnerUserIds:  map[string]string{},
	}, nil
}

func (s *Server) handleReadyADVResponse(ctx context.Context, req *bidEngineGrpc.BidEngineRequest_V2_5) *bidEngineGrpc.BidEngineResponse_V2_5 {
	ready := req.GetReadyBidResponse()
	if !bidResponseHasBids(ready) {
		return &bidEngineGrpc.BidEngineResponse_V2_5{Code: http.StatusNoContent, Rekl: true, WinnerUserIds: map[string]string{}}
	}
	cloned, ok := proto.Clone(ready).(*ortb_V2_5.BidResponse)
	if !ok || cloned == nil {
		return &bidEngineGrpc.BidEngineResponse_V2_5{Code: http.StatusNoContent, Rekl: true, WinnerUserIds: map[string]string{}}
	}
	resultSeat := &ortb_V2_5.SeatBid{Bid: []*ortb_V2_5.Bid{}}
	impUUID := make(map[string]string)
	winnerUsers := make(map[string]string)
	failed := make([]string, 0)
	advDomain := "adv"

	for _, seat := range cloned.GetSeatbid() {
		if seat == nil {
			continue
		}
		for _, bid := range seat.GetBid() {
			if bid == nil || bid.GetImpid() == "" {
				continue
			}
			impID := bid.GetImpid()
			uuid := req.GetImpIdUuid()[impID]
			userID := req.GetWinnerUserIds()[impID]
			if uuid == "" || userID == "" {
				failed = append(failed, impID)
				continue
			}
			finalBid, ok := bidEngine.FinalizeBidCallbacks(
				bid,
				s.admDomain,
				uuid,
				req.GetSspDomain(),
				req.GetFormat(),
				true,
				bidEngine.ADVUsesBURL(req.GetFormat()),
			)
			if !ok {
				failed = append(failed, impID)
				continue
			}
			price := finalBid.GetPrice()
			cid, crid := finalBid.GetCid(), finalBid.GetCrid()
			clickhouseBid := &clickhouse_types.Bid{
				WinDspDomain: &advDomain,
				WinPrice:     &price,
				WinDspPrice:  &price,
				WinCid:       &cid,
				WinCrid:      &crid,
				WinUserId:    &userID,
			}
			if err := utils.WriteWinStats(ctx, s.redisClients, uuid, clickhouseBid, req.GetLogged()); err != nil {
				log.Printf("failed to WriteWinStats for ADV bid: %v", err)
				if s.redisWriteErrorMonitor != nil {
					s.redisWriteErrorMonitor.RecordForURL(err, req.GetSspUrl())
				}
				failed = append(failed, impID)
				continue
			}
			resultSeat.Bid = append(resultSeat.Bid, finalBid)
			impUUID[impID] = uuid
			winnerUsers[impID] = userID
		}
	}
	if len(resultSeat.Bid) == 0 {
		return &bidEngineGrpc.BidEngineResponse_V2_5{Code: http.StatusNoContent, Rekl: true, FailedImpIds: failed, WinnerUserIds: map[string]string{}}
	}
	cloned.Seatbid = []*ortb_V2_5.SeatBid{resultSeat}
	return &bidEngineGrpc.BidEngineResponse_V2_5{
		BidResponse:    cloned,
		Code:           http.StatusOK,
		FailedImpIds:   failed,
		ImpIdUuidClone: impUUID,
		Rekl:           true,
		WinnerUserIds:  winnerUsers,
	}
}

func bidResponseHasBids(response *ortb_V2_5.BidResponse) bool {
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
