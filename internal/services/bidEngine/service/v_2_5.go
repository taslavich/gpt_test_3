package bidEngine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"

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
	ImpIdUuid map[string]string,
	percentMapAdult *map[string]map[string]map[string]*types.PercentAndBidfloor,
	percentMapMainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
	logged bool,
	typic string,
	admDomain string,
) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse) {
	if len(req.BidResponses) == 0 {
		return &ortb_V2_5.BidResponse{
			Id: req.BidRequest.Id,
			Seatbid: []*ortb_V2_5.SeatBid{
				{
					Bid: []*ortb_V2_5.Bid{},
				},
			},
		}, clickhouse_types.GetEmpty(ImpIdUuid)
	}

	type bidWithDomain struct {
		bid        *ortb_V2_5.Bid
		finalPrice float32
		domain     string
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

		globalUuid := func() string {
			for _, uuid := range ImpIdUuid {
				return uuid
			}

			return ""
		}()

		log.Printf("Got len of impBids = 0, global id %s, bid responses %s", globalUuid, string(jsonData))
		return &ortb_V2_5.BidResponse{
			Id: req.BidRequest.Id,
			Seatbid: []*ortb_V2_5.SeatBid{
				{
					Bid: []*ortb_V2_5.Bid{},
				},
			},
		}, clickhouse_types.GetEmpty(ImpIdUuid)
	}

	seatBid := []*ortb_V2_5.SeatBid{
		{
			Bid: []*ortb_V2_5.Bid{},
		},
	}
	clickhouseSeatBid := clickhouse_types.GetEmpty(ImpIdUuid)

	for impID, bids := range impBids {
		if len(bids) == 0 {
			continue // добавляем защиту
		}

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

		newBids := make([]*bidWithDomain, 0)
		for k := range bids {
			value := utils.GetValueFomSspGeoDspMap(req.SspDomain, req.BidRequest.Device.Geo.GetCountry(), bids[k].domain, percentMap, &types.PercentAndBidfloor{
				Percent:  profitPercent,
				Bidfloor: true,
			})

			finalPrice, err := applyPriceConstraintsAndPercent(
				bids[k].bid.GetPrice(),
				bidFloor,
				value.Percent,
				value.Bidfloor,
			)
			if err != nil {
				continue
			}

			bids[k].finalPrice = finalPrice
			newBids = append(newBids, bids[k])

		}

		sort.Slice(newBids, func(i, j int) bool {
			return newBids[i].finalPrice > newBids[j].finalPrice
		})

		winner := newBids[0]

		var finalBid *ortb_V2_5.Bid

		wrappedNurl := utils.WrapURL(admDomain, winner.bid.GetNurl(), ImpIdUuid[impID], utils.NURL, req.Format)

		if logged {
			wrappedAdm := utils.WrapURL(admDomain, winner.bid.GetAdm(), ImpIdUuid[impID], utils.ADM, req.Format)
			finalBid = &ortb_V2_5.Bid{
				Id:    winner.bid.Id,
				Impid: winner.bid.Impid,
				Price: &winner.finalPrice,
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
				Price: &winner.finalPrice,
				Adm:   winner.bid.Adm,
				Adid:  winner.bid.Adid,
				Nurl:  &wrappedNurl,
				Cid:   winner.bid.Cid,
				Crid:  winner.bid.Crid,
			}
		}

		userId := ""
		cid := winner.bid.Cid
		if cid == nil {
			cid = &userId
		}
		crid := winner.bid.Crid
		if crid == nil {
			crid = &userId
		}

		clickhouseBid := &clickhouse_types.Bid{
			WinDspDomain: &winner.domain,
			WinPrice:     &winner.finalPrice,
			WinDspPrice:  winner.bid.Price,
			WinCid:       cid,
			WinCrid:      crid,
			WinUserId:    &userId,
		}

		////

		seatBid[0].Bid = append(seatBid[0].Bid, finalBid)
		clickhouseSeatBid[ImpIdUuid[impID]] = clickhouseBid
	}

	bidResponse := &ortb_V2_5.BidResponse{
		Id:      req.BidRequest.Id,
		Seatbid: seatBid,
	}

	return bidResponse, clickhouseSeatBid
}

func applyPriceConstraintsAndPercent(dspPrice, bidFloor, profitPercent float32, needed bool) (
	finalDspPrice float32,
	err error,
) {
	if needed && dspPrice < bidFloor {
		return 0, fmt.Errorf("DSP Price is lower thand bid floor")
	}

	finalDspPrice = dspPrice - dspPrice*profitPercent

	if needed && finalDspPrice < bidFloor {
		finalDspPrice = bidFloor
	}

	return finalDspPrice, nil
}
