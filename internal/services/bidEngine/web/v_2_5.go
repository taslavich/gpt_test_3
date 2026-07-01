package bidEngineWeb

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	for key := range req.ImpIdUuid {
		if strings.TrimSpace(req.ImpIdUuid[key]) == "" {
			log.Printf("Empty uuid jjj")
		}
	}

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetWinnerBid_V2_5: %v, %s", r, string(debug.Stack()))
			log.Printf("Error: %v", err.Error())

			grpcCode := codes.Internal

			resp = nil
			funcErr = status.Error(grpcCode, err.Error())
		}
	}()

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
			s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
		}
	}

	for _, uuid := range burlUUIDs {
		if err := utils.WriteUUIDKeyToRedis(ctx, s.redisBurlClient, uuid, s.redisUUIDKeyTTL); err != nil {
			log.Printf("failed to write BURL UUID key in GetWinnerBidInternal: %v", err)
			s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
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
			s.redisWriteErrorMonitor.RecordForURL(err, req.SspUrl)
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

	if len(bidResponse.Seatbid[0].Bid) == 0 {
		return &bidEngineGrpc.BidEngineResponse_V2_5{
			BidResponse: nil,
			Code:        http.StatusNoContent,
		}, nil
	}

	return &bidEngineGrpc.BidEngineResponse_V2_5{
		BidResponse:    bidResponse,
		Code:           http.StatusOK,
		FailedImpIds:   failedImpIds,
		ImpIdUuidClone: impIdUuidClone,
	}, nil
}
