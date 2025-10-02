package orchestratorWeb

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	orchestratorGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_4"
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

func (s *Server) GetWinnerBid_V2_4(
	ctx context.Context,
	req *orchestratorGrpc.OrchestratorRequest_V2_4,
) (
	resp *orchestratorGrpc.OrchestratorResponse_V2_4,
	funcErr error,
) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetWinnerBid_V2_4: %v", r)
			log.Printf(err.Error())

			grpcCode := codes.Internal

			resp = nil
			funcErr = status.Errorf(grpcCode, err.Error())
		}
	}()
	getBidsReqCtx, cancel := context.WithTimeout(ctx, s.getBidsTimeout)
	defer cancel()
	bids, err := s.dspRouterGrpcClient.GetBids_V2_4(
		getBidsReqCtx,
		&dspRouterGrpc.DspRouterRequest_V2_4{
			BidRequest:  req.BidRequest,
			SppEndpoint: req.SppEndpoint,
			GlobalId:    req.GlobalId,
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
		return &orchestratorGrpc.OrchestratorResponse_V2_4{
			BidResponse: &ortb_V2_4.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: &ortb_V2_4.SeatBid{
					Bid: []*ortb_V2_4.Bid{},
				},
			},
		}, nil
	}

	getWinnerBidReqCtx, cancel := context.WithTimeout(ctx, s.getWinnerBidTimeout)
	defer cancel()

	winner, err := s.bidEngineGrpcClient.GetWinnerBid_V2_4(
		getWinnerBidReqCtx,
		&bidEngineGrpc.BidEngineRequest_V2_4{
			BidRequest:   bids.BidRequest,
			BidResponses: bids.BidResponses,
			GlobalId:     bids.GlobalId,
		},
	)
	if err != nil {
		newErr := fmt.Errorf("Can not GetWinnerBid_V2_4 from bidEngine in GetWinnerBid because got uknown error %w", err)

		grpcCode := codes.Unknown

		st, ok := status.FromError(err)
		if !ok {
			grpcCode = st.Code()
			newErr = fmt.Errorf("Can not GetWinnerBid_V2_4 from bidEngine in GetWinnerBid because got error: %w", st.Err())
		}

		return nil, status.Errorf(grpcCode, newErr.Error())
	}

	return &orchestratorGrpc.OrchestratorResponse_V2_4{
		BidResponse: winner.BidResponse,
	}, nil
}
