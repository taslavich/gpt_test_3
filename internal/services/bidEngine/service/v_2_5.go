package bidEngine

import (
	"context"
	"sort"

	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
)

func GetWinnerBidInternal_V_2_5(
	ctx context.Context,
	req *bidEngineGrpc.BidEngineRequest_V2_5,
	profitPercent float32,
	globalId string,
	hostname string,
) (*pb.BidResponse, *pb.BidResponse) {
	if len(req.BidResponses) == 0 {
		return &pb.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: []*ortb_V2_5.SeatBid{
					{
						Bid: []*ortb_V2_5.Bid{},
					},
				},
			}, &pb.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: []*ortb_V2_5.SeatBid{
					{
						Bid: []*ortb_V2_5.Bid{},
					},
				},
			}
	}

	impBids := make(map[string][]*pb.Bid)
	for _, bidResponse := range req.BidResponses {
		if bidResponse == nil || bidResponse.Seatbid == nil {
			continue
		}
		for _, seatbid := range bidResponse.Seatbid {
			if seatbid == nil {
				continue
			}
			for _, bid := range seatbid.Bid {
				if bid == nil {
					continue
				}
				impID := bid.GetImpid()
				if impID == "" {
					continue
				}
				impBids[impID] = append(impBids[impID], bid)
			}
		}
	}

	if len(impBids) == 0 {
		return &pb.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: []*ortb_V2_5.SeatBid{
					{
						Bid: []*ortb_V2_5.Bid{},
					},
				},
			}, &pb.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: []*ortb_V2_5.SeatBid{
					{
						Bid: []*ortb_V2_5.Bid{},
					},
				},
			}
	}

	seatBid := []*pb.SeatBid{}
	seatBidByDspPrice := []*pb.SeatBid{}

	for impID, bids := range impBids {
		sort.Slice(bids, func(i, j int) bool {
			return bids[i].GetPrice() > bids[j].GetPrice()
		})

		winningBid := bids[0]

		var bidFloor float32 = 0
		for _, imp := range req.BidRequest.Imp {
			if imp.GetId() == impID {
				bidFloor = imp.GetBidFloor()
				break
			}
		}

		finalPrice, _, err := applyPriceConstraintsAndPercent(
			winningBid.GetPrice(),
			bidFloor,
			profitPercent,
		)
		if err != nil {
			continue
		}

		wrappedNurl := utils.WrapURL(hostname, winningBid.GetNurl(), globalId, utils.NURL)
		var wrappedBurl *string
		if winningBid.Burl != nil {
			bufBur := utils.WrapURL(hostname, winningBid.GetBurl(), globalId, utils.BURL)
			wrappedBurl = &bufBur
		}
		finalBid := &pb.Bid{
			Id:    winningBid.Id,
			Impid: winningBid.Impid,
			Price: &finalPrice,
			Adid:  winningBid.Adid,
			Nurl:  &wrappedNurl,
			Burl:  wrappedBurl,
		}

		bidByDspPrice := &pb.Bid{
			Id:    winningBid.Id,
			Impid: winningBid.Impid,
			Price: winningBid.Price,
		}

		seatBid[0].Bid = append(seatBid[0].Bid, finalBid)
		seatBidByDspPrice[0].Bid = append(seatBidByDspPrice[0].Bid, bidByDspPrice)
	}

	bidResponse := &pb.BidResponse{
		Id:      req.BidRequest.Id,
		Seatbid: seatBid,
	}

	bidResponseByDspPrice := &pb.BidResponse{
		Id:      req.BidRequest.Id,
		Seatbid: seatBidByDspPrice,
	}

	return bidResponse, bidResponseByDspPrice
}
