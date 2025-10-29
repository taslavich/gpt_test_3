package bidEngineWeb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	bidEngine "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type Server struct {
	ProfitPercent float32
	redisClient   *redis.Client
	timeout       time.Duration
	hostname      string

	GetWinnerBidInternal_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		globalId string,
		hostname string,
	) (*ortb_V2_5.BidResponse, *clickhouse_types.BidResponse)

	pb.BidEngineServiceServer
}

func NewServer(
	ProfitPercent float32,
	redisClient *redis.Client,
	hostname string,
	GetWinnerBidInternal_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		globalId string,
		hostname string,
	) (*ortb_V2_5.BidResponse, *clickhouse_types.BidResponse),
) *Server {
	return &Server{
		ProfitPercent:              ProfitPercent,
		redisClient:                redisClient,
		hostname:                   hostname,
		GetWinnerBidInternal_V_2_5: GetWinnerBidInternal_V_2_5,
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
	bidResponse, clickhouseBidResponse := s.GetWinnerBidInternal_V_2_5(
		ctx,
		req,
		s.ProfitPercent,
		req.GlobalId,
		s.hostname,
	)

	clickhouseData, err := json.Marshal(clickhouseBidResponse)
	if err != nil {
		log.Printf("failed to marshal JSON in GetWinnerBidInternal: %w", err)
	}

	if err := utils.WriteJsonToRedis(ctx, s.redisClient, req.GlobalId, constants.BID_RESPONSE_WINNER_COLUMN, clickhouseData); err != nil {
		log.Printf("failed to WriteJsonToRedis Bid BID_RESPONSE_WINNER in GetWinnerBidInternal: %w", err)
	}

	return &bidEngineGrpc.BidEngineResponse_V2_5{
		BidResponse: bidResponse,
	}, nil
}

func (s *Server) ChangeSspGeoPercentsMap(ctx context.Context, req *bidEngineGrpc.SspGeoPercentsRequest_V2_5) (*emptypb.Empty, error) {
	for sspDomain, externalGeoMap := range req.Changes {
		if _, ok := bidEngine.SspGeoPercents[sspDomain]; !ok {
			bidEngine.SspGeoPercents[sspDomain] = make(map[string]float32)
		}

		for externalGeo, externalDelta := range externalGeoMap.Values {
			if percent, ok := bidEngine.SspGeoPercents[sspDomain][externalGeo]; ok {
				bidEngine.SspGeoPercents[sspDomain][externalGeo] = percent + externalDelta
				continue
			}

			bidEngine.SspGeoPercents[sspDomain][externalGeo] = s.ProfitPercent + externalDelta
		}
	}

	return nil, nil
}
