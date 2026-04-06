package orchestratorWeb

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
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	orchestratorGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	bidEngineGrpcClient bidEngineGrpc.BidEngineServiceClient
	dspRouterGrpcClient dspRouterGrpc.DspRouterServiceClient

	redisClient *redis.Client

	getBidsTimeout      time.Duration
	getWinnerBidTimeout time.Duration

	orchestratorGrpc.UnimplementedOrchestratorServiceServer
}

func NewServer(
	bidEngineGrpcClient bidEngineGrpc.BidEngineServiceClient,
	dspRouterGrpcClient dspRouterGrpc.DspRouterServiceClient,
	redisClient *redis.Client,
	getBidsTimeout,
	getWinnerBidTimeout time.Duration,
) *Server {
	return &Server{
		bidEngineGrpcClient: bidEngineGrpcClient,
		dspRouterGrpcClient: dspRouterGrpcClient,
		redisClient:         redisClient,
		getBidsTimeout:      getBidsTimeout,
		getWinnerBidTimeout: getWinnerBidTimeout,
	}
}

func (s *Server) GetWinnerBid_V2_5(
	ctx context.Context,
	req *orchestratorGrpc.OrchestratorRequest_V2_5,
) (
	resp *orchestratorGrpc.OrchestratorResponse_V2_5,
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
	getBidsReqCtx, cancel := context.WithTimeout(ctx, s.getBidsTimeout)
	defer cancel()

	bids, err := s.dspRouterGrpcClient.GetBids_V2_5(
		getBidsReqCtx,
		&dspRouterGrpc.DspRouterRequest_V2_5{
			BidRequest: req.BidRequest,
			GlobalId:   req.GlobalId,
			SspDomain:  req.SspDomain,
			Logged:     req.Logged,
			Typic:      req.Typic,
			Format:     req.Format,
		},
	)
	if err != nil {
		newErr := fmt.Errorf("Can not get bids from router in GetWinnerBid because got uknown error: %w", err)

		grpcCode := codes.Unknown

		st, ok := status.FromError(err)
		if !ok {
			grpcCode = st.Code()
			newErr = fmt.Errorf("Can not get bids from router in  GetWinnerBid because got error: %w", st.Err())
		}

		return nil, status.Errorf(grpcCode, newErr.Error())
	}

	if len(bids.BidResponses) == 0 {
		clickhouseData, err := json.Marshal(&clickhouse_types.BidResponse{
			Id: req.BidRequest.Id,
			Seatbid: []*clickhouse_types.SeatBid{
				{
					Bid: []*clickhouse_types.Bid{},
				},
			},
			Error: fmt.Sprintf("Got len of BidResponses = 0, bid request id: %s", req.BidRequest.GetId()),
		})
		if err != nil {
			log.Printf("failed to marshal JSON in GetWinnerBidInternal: %w", err)
		}

		if err := utils.WriteJsonToRedis(ctx, s.redisClient, req.GlobalId, constants.BID_RESPONSE_WINNER_COLUMN, clickhouseData, req.Logged); err != nil {
			log.Printf("failed to WriteJsonToRedis Bid BID_RESPONSE_WINNER in GetWinnerBidInternal: %w", err)
		}

		return &orchestratorGrpc.OrchestratorResponse_V2_5{
			BidResponse: &ortb_V2_5.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: []*ortb_V2_5.SeatBid{
					{
						Bid: []*ortb_V2_5.Bid{},
					},
				},
			},
		}, nil
	}

	getWinnerBidReqCtx, cancel := context.WithTimeout(ctx, s.getWinnerBidTimeout)
	defer cancel()

	winner, err := s.bidEngineGrpcClient.GetWinnerBid_V2_5(
		getWinnerBidReqCtx,
		&bidEngineGrpc.BidEngineRequest_V2_5{
			BidRequest:   bids.BidRequest,
			BidResponses: bids.BidResponses,
			GlobalId:     bids.GlobalId,
			SspDomain:    bids.SspDomain,
			Logged:       req.Logged,
			Typic:        req.Typic,
			Format:       req.Format,
		},
	)
	if err != nil {
		newErr := fmt.Errorf("Can not GetWinnerBid_V2_5 from bidEngine in GetWinnerBid because got uknown error %w", err)

		grpcCode := codes.Unknown

		st, ok := status.FromError(err)
		if !ok {
			grpcCode = st.Code()
			newErr = fmt.Errorf("Can not GetWinnerBid_V2_5 from bidEngine in GetWinnerBid because got error: %w", st.Err())
		}

		return nil, status.Errorf(grpcCode, newErr.Error())
	}

	return &orchestratorGrpc.OrchestratorResponse_V2_5{
		BidResponse: winner.BidResponse,
	}, nil
}
