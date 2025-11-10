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
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Server struct {
	ProfitPercent              float32
	redisClient                *redis.Client
	timeout                    time.Duration
	hostname                   string
	percentFilename_adult      string
	percentFilename_mainstream string
	percentMap_adult           map[string]map[string]map[string]*types.PercentAndBidfloor
	percentMap_mainstream      map[string]map[string]map[string]*types.PercentAndBidfloor

	GetWinnerBidInternal_V_2_5 func(
		ctx context.Context,
		req *bidEngineGrpc.BidEngineRequest_V2_5,
		profitPercent float32,
		globalId string,
		hostname string,
		percentMapAdult map[string]map[string]map[string]*types.PercentAndBidfloor,
		percentMapMainstream map[string]map[string]map[string]*types.PercentAndBidfloor,
		logged bool,
		typic string,
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
		percentMapAdult map[string]map[string]map[string]*types.PercentAndBidfloor,
		percentMapMainstream map[string]map[string]map[string]*types.PercentAndBidfloor,
		logged bool,
		typic string,
	) (*ortb_V2_5.BidResponse, *clickhouse_types.BidResponse),
	percentFilename_adult string,
	percentMap_adult map[string]map[string]map[string]*types.PercentAndBidfloor,

	percentFilename_mainstream string,
	percentMap_mainstream map[string]map[string]map[string]*types.PercentAndBidfloor,
) *Server {
	return &Server{
		ProfitPercent:              ProfitPercent,
		redisClient:                redisClient,
		hostname:                   hostname,
		GetWinnerBidInternal_V_2_5: GetWinnerBidInternal_V_2_5,
		percentFilename_adult:      percentFilename_adult,
		percentFilename_mainstream: percentFilename_mainstream,
		percentMap_adult:           percentMap_adult,
		percentMap_mainstream:      percentMap_mainstream,
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
		s.percentMap_adult,
		s.percentMap_mainstream,
		req.Logged,
		req.Typic,
	)

	clickhouseData, err := json.Marshal(clickhouseBidResponse)
	if err != nil {
		log.Printf("failed to marshal JSON in GetWinnerBidInternal: %w", err)
	}

	if err := utils.WriteJsonToRedis(ctx, s.redisClient, req.GlobalId, constants.BID_RESPONSE_WINNER_COLUMN, clickhouseData, req.Logged); err != nil {
		log.Printf("failed to WriteJsonToRedis Bid BID_RESPONSE_WINNER in GetWinnerBidInternal: %w", err)
	}

	return &bidEngineGrpc.BidEngineResponse_V2_5{
		BidResponse: bidResponse,
	}, nil
}

func (s *Server) SetSspGeoPercentsMap(ctx context.Context, req *bidEngineGrpc.SspGeoDspPercentsRequest_V2_5) (*emptypb.Empty, error) {
	var err error

	var typic string
	err = json.Unmarshal([]byte(req.Typic), &typic)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON format: %v", err)
	}

	switch typic {
	case sppAdapterWeb.ADULT:
		s.percentMap_adult, err = utils.RewriteSspGeoDspFile[*types.PercentAndBidfloor](
			req.JsonData,
			s.percentFilename_adult,
		)
	case sppAdapterWeb.MAINSTREAM:
		s.percentMap_mainstream, err = utils.RewriteSspGeoDspFile[*types.PercentAndBidfloor](
			req.JsonData,
			s.percentFilename_mainstream,
		)
	default:
		return nil, status.Errorf(codes.Internal, "Typic is no here! failed to read file: %v", err)
	}
	if err != nil {
		return nil, err
	}
	log.Printf("Successfully updated SspGeoPercents SSP entries")

	return nil, nil
}

func (s *Server) GetSspGeoPercentsMap(ctx context.Context, req *bidEngineGrpc.GetSspGeoDspPercentsRequest_V2_5) (*bidEngineGrpc.GetSspGeoDspPercentsResponse_V2_5, error) {
	var data []byte
	var err error

	var typic string
	err = json.Unmarshal([]byte(req.Typic), &typic)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON format: %v", err)
	}

	switch typic {
	case sppAdapterWeb.ADULT:
		data, err = os.ReadFile(s.percentFilename_adult)
	case sppAdapterWeb.MAINSTREAM:
		data, err = os.ReadFile(s.percentFilename_mainstream)
	default:
		return nil, status.Error(codes.Internal, "Typic is no here! failed to read file")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read file %s: %w", s.percentFilename_adult, err)
	}

	return &bidEngineGrpc.GetSspGeoDspPercentsResponse_V2_5{
		JsonData: string(data),
	}, nil
}
