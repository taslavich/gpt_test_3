package orchestratorWeb

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	orchestratorGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	bidEngineGrpcClient bidEngineGrpc.BidEngineServiceClient
	dspRouterGrpcClient dspRouterGrpc.DspRouterServiceClient

	getBidsTimeout      time.Duration
	getWinnerBidTimeout time.Duration

	orchestratorGrpc.UnimplementedOrchestratorServiceServer
}

func NewServer(
	bidEngineGrpcClient bidEngineGrpc.BidEngineServiceClient,
	dspRouterGrpcClient dspRouterGrpc.DspRouterServiceClient,
	getBidsTimeout,
	getWinnerBidTimeout time.Duration,
) *Server {
	return &Server{
		bidEngineGrpcClient: bidEngineGrpcClient,
		dspRouterGrpcClient: dspRouterGrpcClient,
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
			SspDomain:  req.SspDomain,
			Logged:     req.Logged,
			Typic:      req.Typic,
			Format:     req.Format,
			ImpIdUuid:  req.ImpIdUuid,
			SspUrl:     req.SspUrl,
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

	if bids.Code == 703 {
		return &orchestratorGrpc.OrchestratorResponse_V2_5{
			BidResponse: nil,
			Code:        bids.Code,
		}, nil
	}

	getWinnerBidReqCtx, cancel := context.WithTimeout(ctx, s.getWinnerBidTimeout)
	defer cancel()

	winner, err := s.bidEngineGrpcClient.GetWinnerBid_V2_5(
		getWinnerBidReqCtx,
		&bidEngineGrpc.BidEngineRequest_V2_5{
			BidRequest:   bids.BidRequest,
			BidResponses: bids.BidResponses,
			SspDomain:    bids.SspDomain,
			Logged:       req.Logged,
			Typic:        req.Typic,
			Format:       req.Format,
			ImpIdUuid:    req.ImpIdUuid,
			SspUrl:       req.SspUrl,
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
		Code: winner.Code,
	}, nil
}
