package bidEngineWeb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
	"google.golang.org/protobuf/types/known/emptypb"
)

type Server struct {
	ProfitPercent float32
	redisClient   *redis.Client
	timeout       time.Duration
	hostname      string
	counter       *uint64

	GetWinnerBidInternal_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		globalId string,
		hostname string,
		counter *uint64,
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
		counter *uint64,
	) (*ortb_V2_5.BidResponse, *clickhouse_types.BidResponse),
) *Server {
	var counter uint64 = 0
	return &Server{
		ProfitPercent:              ProfitPercent,
		redisClient:                redisClient,
		hostname:                   hostname,
		GetWinnerBidInternal_V_2_5: GetWinnerBidInternal_V_2_5,
		counter:                    &counter,
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
		s.counter,
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

func (s *Server) SetSspGeoPercentsMap(ctx context.Context, req *bidEngineGrpc.SspGeoDspPercentsRequest_V2_5) (*emptypb.Empty, error) {
	log.Printf("=== DEBUG SetSspGeoPercentsMap ===")
	log.Printf("Request: %+v", req)
	log.Printf("JsonData: '%s'", req.JsonData)
	log.Printf("JsonData length: %d", len(req.JsonData))

	if req == nil {
		log.Printf("REQUEST IS NIL!")
		return nil, status.Errorf(codes.InvalidArgument, "request is nil")
	}

	if req.JsonData == "" {
		log.Printf("JSON_DATA IS EMPTY!")
		return nil, status.Errorf(codes.InvalidArgument, "json_data is empty")
	}

	// Парсим JSON напрямую
	var inputMap map[string]map[string]map[string]float32

	err := json.Unmarshal([]byte(req.JsonData), &inputMap)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON format: %v", err)
	}

	// Сохраняем в файл
	fileData, err := json.MarshalIndent(inputMap, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal data for file: %v", err)
	}

	err = os.WriteFile(bidEngine.SspGeoDspPercentsFilePath, fileData, 0644)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write file %s: %v", bidEngine.SspGeoDspPercentsFilePath, err)
	}

	bidEngine.SspGeoPercents = inputMap

	log.Printf("Successfully updated SspGeoPercents with %d SSP entries", len(inputMap))
	return nil, nil
}

func (s *Server) GetSspGeoPercentsMap(context.Context, *emptypb.Empty) (*bidEngineGrpc.SspGeoDspPercentsResponse_V2_5, error) {
	log.Printf("=== DEBUG GetSspGeoPercentsMap ===")
	log.Printf("SspGeoPercents == nil: %t", bidEngine.SspGeoPercents == nil)
	log.Printf("SspGeoPercents length: %d", len(bidEngine.SspGeoPercents))
	log.Printf("SspGeoPercents: %+v", bidEngine.SspGeoPercents)

	if bidEngine.SspGeoPercents == nil {
		log.Printf("SspGeoPercents is NIL!")
		return &bidEngineGrpc.SspGeoDspPercentsResponse_V2_5{
			JsonData: "{}",
		}, nil
	}

	jsonBytes, err := json.Marshal(bidEngine.SspGeoPercents)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal data to JSON: %v", err)
	}

	return &bidEngineGrpc.SspGeoDspPercentsResponse_V2_5{
		JsonData: string(jsonBytes),
	}, nil
}
