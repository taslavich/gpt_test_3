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

func trafficTypeFromOrchestratorRequest(req *orchestratorGrpc.OrchestratorRequest_V2_5) string {
	if req == nil {
		return ""
	}
	if req.GetTrafficType() != "" {
		return req.GetTrafficType()
	}
	return req.GetTypic()
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
			funcErr = status.Error(grpcCode, err.Error())
		}
	}()
	if s == nil || req == nil || req.GetBidRequest() == nil {
		return nil, status.Error(codes.InvalidArgument, "orchestrator request or bid request is nil")
	}
	if s.dspRouterGrpcClient == nil || s.bidEngineGrpcClient == nil {
		return nil, status.Error(codes.Unavailable, "orchestrator downstream client is unavailable")
	}

	getBidsTimeout := s.getBidsTimeout
	if getBidsTimeout <= 0 {
		getBidsTimeout = time.Second
	}
	getBidsReqCtx, cancel := context.WithTimeout(ctx, getBidsTimeout)
	defer cancel()

	bids, err := s.dspRouterGrpcClient.GetBids_V2_5(
		getBidsReqCtx,
		&dspRouterGrpc.DspRouterRequest_V2_5{
			BidRequest:  req.BidRequest,
			SspDomain:   req.SspDomain,
			Logged:      req.Logged,
			Typic:       req.Typic,
			Format:      req.Format,
			TrafficType: trafficTypeFromOrchestratorRequest(req),
			ImpIdUuid:   req.ImpIdUuid,
			SspUrl:      req.SspUrl,
			Feed:        req.GetFeed(),
		},
	)
	if err != nil {
		newErr := fmt.Errorf("Can not get bids from router in GetWinnerBid because got uknown error: %w", err)

		grpcCode := codes.Unknown

		if st, ok := status.FromError(err); ok {
			grpcCode = st.Code()
			newErr = fmt.Errorf("Can not get bids from router in GetWinnerBid because got error: %w", st.Err())
		}

		return nil, status.Error(grpcCode, newErr.Error())
	}

	if bids == nil {
		return nil, status.Error(codes.Unavailable, "router returned a nil response")
	}

	if bids.GetCode() == 703 {
		return &orchestratorGrpc.OrchestratorResponse_V2_5{
			BidResponse:   nil,
			Code:          bids.GetCode(),
			Rekl:          false,
			WinnerUserIds: map[string]string{},
		}, nil
	}

	getWinnerBidTimeout := s.getWinnerBidTimeout
	if getWinnerBidTimeout <= 0 {
		getWinnerBidTimeout = time.Second
	}
	getWinnerBidReqCtx, cancel := context.WithTimeout(ctx, getWinnerBidTimeout)
	defer cancel()

	winner, err := s.bidEngineGrpcClient.GetWinnerBid_V2_5(
		getWinnerBidReqCtx,
		&bidEngineGrpc.BidEngineRequest_V2_5{
			BidRequest:       bids.GetBidRequest(),
			BidResponses:     bids.GetBidResponses(),
			SspDomain:        bids.GetSspDomain(),
			Logged:           req.GetLogged(),
			Typic:            req.GetTypic(),
			Format:           req.GetFormat(),
			ImpIdUuid:        bids.GetImpIdUuid(),
			SspUrl:           req.GetSspUrl(),
			Rekl:             bids.GetRekl(),
			ReadyBidResponse: bids.GetReadyBidResponse(),
			WinnerUserIds:    bids.GetWinnerUserIds(),
		},
	)
	if err != nil {
		newErr := fmt.Errorf("Can not GetWinnerBid_V2_5 from bidEngine in GetWinnerBid because got uknown error %w", err)

		grpcCode := codes.Unknown

		if st, ok := status.FromError(err); ok {
			grpcCode = st.Code()
			newErr = fmt.Errorf("Can not GetWinnerBid_V2_5 from bidEngine in GetWinnerBid because got error: %w", st.Err())
		}

		return nil, status.Error(grpcCode, newErr.Error())
	}

	if winner == nil {
		return nil, status.Error(codes.Unavailable, "bid engine returned a nil response")
	}

	return &orchestratorGrpc.OrchestratorResponse_V2_5{
		BidResponse:    winner.GetBidResponse(),
		Code:           winner.GetCode(),
		FailedImpIds:   winner.GetFailedImpIds(),
		ImpIdUuidClone: winner.GetImpIdUuidClone(),
		Rekl:           winner.GetRekl(),
		WinnerUserIds:  winner.GetWinnerUserIds(),
	}, nil
}
