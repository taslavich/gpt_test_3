package web

import (
	"context"
	"net/http"
	"time"

	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

// Server exposes the ADV auction service over gRPC.
type Server struct {
	advGrpc.UnimplementedAdvServiceServer
	auctionService *auction.AuctionService
}

// NewServer creates a gRPC server that delegates auction decisions to AuctionService.
func NewServer(auctionService *auction.AuctionService) *Server {
	return &Server{auctionService: auctionService}
}

// DoAuction accepts an OpenRTB bid request and runs the ADV auction selection logic.
func (s *Server) DoAuction(ctx context.Context, req *advGrpc.DoAuctionRequest) (*advGrpc.DoAuctionResponse, error) {
	if req == nil || req.GetBidRequest() == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusBadRequest}, nil
	}
	if s.auctionService == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusServiceUnavailable}, nil
	}

	campaign := s.auctionService.SelectCampaign(req.GetBidRequest(), time.Now())
	if campaign == nil {
		return &advGrpc.DoAuctionResponse{Selected: false, Code: http.StatusNoContent}, nil
	}

	var creativeID, adm string
	if creative := campaign.FirstCreative(); creative != nil {
		creativeID = creative.ID
		adm = creative.ADMURL
	}

	return &advGrpc.DoAuctionResponse{
		Selected:     true,
		CampaignId:   campaign.ID,
		CreativeId:   creativeID,
		Adm:          adm,
		AuctionPrice: campaign.GetAuctionPrice(),
		Code:         http.StatusOK,
	}, nil
}
