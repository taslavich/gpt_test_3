package bidEngineWeb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetWinnerBid_V2_5(
	ctx context.Context,
	req *bidEngineGrpc.BidEngineRequest_V2_5,
) (
	resp *bidEngineGrpc.BidEngineResponse_V2_5,
	funcErr error,
) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered from panic in GetWinnerBid_V2_5: %v", r)
			log.Printf(err.Error())

			grpcCode := codes.Internal

			resp = nil
			funcErr = status.Errorf(grpcCode, err.Error())
		}
	}()
	bidResponse, bidResponseByDspPrice := s.GetWinnerBidInternal_V_2_5(
		ctx,
		req,
		s.ProfitPercent,
		req.GlobalId,
		s.hostname,
	)

	data, err := json.Marshal(bidResponse)
	if err != nil {
		fmt.Printf("failed to marshal JSON in GetWinnerBidInternal: %w", err)
	}

	if err := utils.WriteJsonToRedis(ctx, s.redisClient, req.GlobalId, constants.BID_RESPONSE_WINNER_COLUMN, data); err != nil {
		fmt.Printf("failed to WriteJsonToRedis Bid BID_RESPONSE_WINNER in GetWinnerBidInternal: %w", err)
	}

	dataByDspPrice, err := json.Marshal(bidResponseByDspPrice)
	if err != nil {
		fmt.Printf("failed to marshal JSON in GetWinnerBidInternal: %w", err)
	}

	if err := utils.WriteJsonToRedis(ctx, s.redisClient, req.GlobalId, constants.BID_RESPONSE_WINNER_BY_DSP_PRICE_COLUMN, dataByDspPrice); err != nil {
		fmt.Printf("failed to WriteJsonToRedis Bid BID_RESPONSE_WINNER_BY_DSP_PRICE_COLUMN in GetWinnerBidInternal: %w", err)
	}

	return &bidEngineGrpc.BidEngineResponse_V2_5{
		BidResponse: bidResponse,
	}, nil
}
