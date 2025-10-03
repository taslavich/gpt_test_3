package orchestratorWeb

import (
	"context"
	"fmt"
	"log"

	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	orchestratorGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetWinnerBid_V2_5(
	ctx context.Context,
	req *orchestratorGrpc.OrchestratorRequest_V2_5,
) (
	resp *orchestratorGrpc.OrchestratorResponse_V2_5,
	funcErr error,
) {
	//start := time.Now()
	defer func() {
		//elapsed := time.Since(start)
		//fmt.Printf("Execution time in ms: %d ms\n", elapsed.Milliseconds())

		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetWinnerBid_V2_5: %v", r)
			log.Printf(err.Error())

			grpcCode := codes.Internal

			resp = nil
			funcErr = status.Errorf(grpcCode, err.Error())
		}
	}()
	getBidsReqCtx, cancel := context.WithTimeout(ctx, s.getBidsTimeout)
	defer cancel()
	log.Println("GetBids_V2_5")
	fmt.Println("GetBids_V2_5")
	bids, err := s.dspRouterGrpcClient.GetBids_V2_5(
		getBidsReqCtx,
		&dspRouterGrpc.DspRouterRequest_V2_5{
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

	log.Println("END GetBids_V2_5")
	fmt.Println("END GetBids_V2_5")

	if len(bids.BidResponses) == 0 {
		return &orchestratorGrpc.OrchestratorResponse_V2_5{
			BidResponse: &ortb_V2_5.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: &ortb_V2_5.SeatBid{
					Bid: []*ortb_V2_5.Bid{},
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
