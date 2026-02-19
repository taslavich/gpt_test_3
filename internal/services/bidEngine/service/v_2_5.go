package bidEngine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

func GetWinnerBidInternal_V_2_5(
	ctx context.Context,
	req *bidEngineGrpc.BidEngineRequest_V2_5,
	profitPercent float32,
	globalId string,
	percentMapAdult *map[string]map[string]map[string]*types.PercentAndBidfloor,
	percentMapMainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
	logged bool,
	typic string,
	admDomain string,
) (*ortb_V2_5.BidResponse, *clickhouse_types.BidResponse) {
	type bidWithDomain struct {
		bid    *ortb_V2_5.Bid
		domain string
	}

	impBids := make(map[string][]*bidWithDomain)
	for domain, bidResponse := range req.BidResponses {
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
				impBids[impID] = append(impBids[impID], &bidWithDomain{
					bid:    bid,
					domain: domain,
				})
			}
		}
	}

	if len(impBids) == 0 {
		jsonData, err := json.MarshalIndent(req.BidResponses, "", "  ")
		if err != nil {
			log.Printf("Got len of impBids = 0, Error marshaling: %v", err)
		}

		log.Printf("Got len of impBids = 0, global id %s, bid responses %s", req.GlobalId, string(jsonData))
		return &ortb_V2_5.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: []*ortb_V2_5.SeatBid{
					{
						Bid: []*ortb_V2_5.Bid{},
					},
				},
			}, &clickhouse_types.BidResponse{
				Id: req.BidRequest.Id,
				Seatbid: []*clickhouse_types.SeatBid{
					{
						Bid: []*clickhouse_types.Bid{},
					},
				},
				Error: fmt.Sprintf("Got len of impBids = 0, bid request id: %s", req.BidRequest.GetId()),
			}
	}

	seatBid := []*ortb_V2_5.SeatBid{
		{
			Bid: []*ortb_V2_5.Bid{},
		},
	}
	clickhouseSeatBid := []*clickhouse_types.SeatBid{
		{
			Bid: []*clickhouse_types.Bid{},
		},
	}

	var errStr string = "None"
	for impID, bids := range impBids {
		if len(bids) == 0 {
			continue // добавляем защиту
		}
		sort.Slice(bids, func(i, j int) bool {
			return bids[i].bid.GetPrice() > bids[j].bid.GetPrice()
		})

		winner := bids[0]

		var bidFloor float32 = 0
		for _, imp := range req.BidRequest.Imp {
			if imp.GetId() == impID {
				bidFloor = imp.GetBidfloor()
				break
			}
		}

		var percentMap map[string]map[string]map[string]*types.PercentAndBidfloor
		switch typic {
		case sppAdapterWeb.ADULT:
			percentMap = *percentMapAdult
		case sppAdapterWeb.MAINSTREAM:
			percentMap = *percentMapMainstream
		}

		value := utils.GetValueFomSspGeoDspMap(req.SspDomain, req.BidRequest.Device.Geo.GetCountry(), winner.domain, percentMap, &types.PercentAndBidfloor{
			Percent:  profitPercent,
			Bidfloor: true,
		})

		finalPrice, _, err := applyPriceConstraintsAndPercent(
			winner.bid.GetPrice(),
			bidFloor,
			value.Percent,
			value.Bidfloor,
		)
		if err != nil {
			errStr = err.Error()
			continue
		}

		var finalBid *ortb_V2_5.Bid

		wrappedNurl := utils.WrapURL(admDomain, winner.bid.GetNurl(), globalId, utils.NURL)

		if logged {
			wrappedAdm := utils.WrapURL(admDomain, winner.bid.GetAdm(), globalId, utils.ADM)
			finalBid = &ortb_V2_5.Bid{
				Id:    winner.bid.Id,
				Impid: winner.bid.Impid,
				Price: &finalPrice,
				Adm:   &wrappedAdm,
				Adid:  winner.bid.Adid,
				Nurl:  &wrappedNurl,
				Cid:   winner.bid.Cid,
				Crid:  winner.bid.Crid,
			}
		} else {
			finalBid = &ortb_V2_5.Bid{
				Id:    winner.bid.Id,
				Impid: winner.bid.Impid,
				Price: &finalPrice,
				Adm:   winner.bid.Adm,
				Adid:  winner.bid.Adid,
				Nurl:  &wrappedNurl,
				Cid:   winner.bid.Cid,
				Crid:  winner.bid.Crid,
			}
		}

		clickhouseBid := &clickhouse_types.Bid{
			DspDomain: &winner.domain,
			Id:        winner.bid.Id,
			Impid:     winner.bid.Impid,
			Price:     &finalPrice,
			DspPrice:  winner.bid.Price,
			Adid:      winner.bid.Adid,
			Cid:       winner.bid.Cid,
			Crid:      winner.bid.Crid,
		}

		seatBid[0].Bid = append(seatBid[0].Bid, finalBid)
		clickhouseSeatBid[0].Bid = append(clickhouseSeatBid[0].Bid, clickhouseBid)
	}

	bidResponse := &ortb_V2_5.BidResponse{
		Id:      req.BidRequest.Id,
		Seatbid: seatBid,
	}

	clickhouseBidResponse := &clickhouse_types.BidResponse{
		Id:      req.BidRequest.Id,
		Seatbid: clickhouseSeatBid,
		Error:   errStr,
	}

	return bidResponse, clickhouseBidResponse
}

func applyPriceConstraintsAndPercent(dspPrice, bidFloor, profitPercent float32, needed bool) (
	finalDspPrice float32,
	finalProfitPercent float32,
	err error,
) {
	if needed && bidFloor == constants.NEGATIVE_BIDFLOOR {
		bidFloor = constants.ZERO_BIDFLOOR
	}

	if needed && dspPrice < bidFloor {
		return 0, 0, fmt.Errorf("DSP Price is lower thand bid floor")
	}

	finalDspPrice = dspPrice - dspPrice*profitPercent
	finalProfitPercent = profitPercent

	if needed && finalDspPrice < bidFloor {
		finalDspPrice, finalProfitPercent = findGoodPriceViaPercent(
			dspPrice,
			bidFloor,
			profitPercent,
		)
	}

	return finalDspPrice, finalProfitPercent, nil
}

func findGoodPriceViaPercent(
	DspPrice, bidFloor, profitPercent float32,
) (
	finalDspPrice float32,
	finalProfitPercent float32,
) {
	finalDspPrice = -1
	for finalProfitPercent = profitPercent; finalProfitPercent >= 0; finalProfitPercent = finalProfitPercent - EPS {
		finalDspPrice = DspPrice - DspPrice*finalProfitPercent
		if finalDspPrice >= bidFloor {
			break
		}
	}

	return finalDspPrice, finalProfitPercent
}
