package bidEngineWeb

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/redis/go-redis/v9"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	ProfitPercent              float32
	redisClient                *redis.Client
	timeout                    time.Duration
	percentFilename_adult      string
	percentFilename_mainstream string
	percentMap_adult           *map[string]map[string]map[string]*types.PercentAndBidfloor
	percentMap_mainstream      *map[string]map[string]map[string]*types.PercentAndBidfloor
	admDomain                  string

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
	) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse)

	pb.BidEngineServiceServer
}

func NewServer(
	ProfitPercent float32,
	redisClient *redis.Client,
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
	) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse),
	percentFilename_adult string,
	percentMap_adult *map[string]map[string]map[string]*types.PercentAndBidfloor,

	percentFilename_mainstream string,
	percentMap_mainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,

	admDomain string,
) *Server {
	return &Server{
		ProfitPercent:              ProfitPercent,
		redisClient:                redisClient,
		GetWinnerBidInternal_V_2_5: GetWinnerBidInternal_V_2_5,
		percentFilename_adult:      percentFilename_adult,
		percentFilename_mainstream: percentFilename_mainstream,
		percentMap_adult:           percentMap_adult,
		percentMap_mainstream:      percentMap_mainstream,
		admDomain:                  admDomain,
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

			grpcCode := codes.Internal

			resp = nil
			funcErr = status.Errorf(grpcCode, err.Error())
		}
	}()
	bidResponse, clickhouseBid := s.GetWinnerBidInternal_V_2_5(
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

	for _, uuid := range req.ImpIdUuid {
		if err := utils.WriteWinStats(ctx, s.redisClient, uuid, clickhouseBid[uuid], req.Logged); err != nil {
			log.Printf("failed to WriteJsonToRedis Bid BID_RESPONSE_WINNER in GetWinnerBidInternal: %w", err)
		}
	}

	return &bidEngineGrpc.BidEngineResponse_V2_5{
		BidResponse: bidResponse,
	}, nil
}
