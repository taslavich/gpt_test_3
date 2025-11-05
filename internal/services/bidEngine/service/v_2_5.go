package bidEngine

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync/atomic"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

func getRandomProfitPercent() float32 {
	percentages := [11]float32{0.20, 0.21, 0.22, 0.23, 0.24, 0.25, 0.26, 0.27, 0.28, 0.29, 0.30} // 20% - 30% (1)
	randomIndex := rand.Intn(11)                                                                 // 0 - 10 (1)
	return percentages[randomIndex]
}

func shouldPass(counter *uint64) bool {
	return atomic.AddUint64(counter, 1)%100 < 5
}

func GetWinnerBidInternal_V_2_5(
	ctx context.Context,
	req *bidEngineGrpc.BidEngineRequest_V2_5,
	profitPercent float32,
	globalId string,
	hostname string,
	counter *uint64,
) (*ortb_V2_5.BidResponse, *clickhouse_types.BidResponse) {
	////////////
	///profitPercent = getRandomProfitPercent()
	//////////////
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

	/*if geoMap, ok := SspGeoPercents[req.GetBidRequest().GetSspDomain()]; ok {
		if percent, hasGeo := geoMap[req.GetBidRequest().GetDevice().GetGeo().GetCountry()]; hasGeo {
			profitPercent = percent
		}
	}*/

	var errStr string = "None"
	for impID, bids := range impBids {
		if len(bids) == 0 {
			continue // добавляем защиту
		}
		sort.Slice(bids, func(i, j int) bool {
			return bids[i].bid.GetPrice() > bids[j].bid.GetPrice()
		})

		winningBidWithDomain := bids[0] // ← ИЗМЕНЕНО
		winningBid := winningBidWithDomain.bid
		winningDomain := winningBidWithDomain.domain

		var bidFloor float32 = 0
		for _, imp := range req.BidRequest.Imp {
			if imp.GetId() == impID {
				bidFloor = imp.GetBidfloor()
				break
			}
		}

		if winningDomain == "http://ortbtwinbidexadlt.hilltopadsfeed.com/ask" {
			profitPercent = 0.70
		}

		if winningDomain == "http://pop.zog.link/bid-request?token=h6dKfdh544FHD83" && req.SspDomain == "galaksion.com" {
			profitPercent = 0.30
		}

		finalPrice, _, err := applyPriceConstraintsAndPercent(
			winningBid.GetPrice(),
			bidFloor,
			profitPercent,
		)
		if err != nil {
			errStr = err.Error()
			continue
		}

		var finalBid *ortb_V2_5.Bid

		wrappedNurl := utils.WrapURL(hostname, winningBid.GetNurl(), globalId, utils.NURL)

		if shouldPass(counter) {
			wrappedAdm := utils.WrapURL(hostname, winningBid.GetAdm(), globalId, utils.ADM)
			finalBid = &ortb_V2_5.Bid{
				Id:    winningBid.Id,
				Impid: winningBid.Impid,
				Price: &finalPrice,
				Adm:   &wrappedAdm,
				Adid:  winningBid.Adid,
				Nurl:  &wrappedNurl,
			}
		} else {
			finalBid = &ortb_V2_5.Bid{
				Id:    winningBid.Id,
				Impid: winningBid.Impid,
				Price: &finalPrice,
				Adm:   winningBid.Adm,
				Adid:  winningBid.Adid,
				Nurl:  &wrappedNurl,
			}
		}

		clickhouseBid := &clickhouse_types.Bid{
			DspDomain: &winningDomain,
			Id:        winningBid.Id,
			Impid:     winningBid.Impid,
			Price:     &finalPrice,
			DspPrice:  winningBid.Price,
			Adid:      winningBid.Adid,
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

func applyPriceConstraintsAndPercent(dspPrice, bidFloor, profitPercent float32) (
	finalDspPrice float32,
	finalProfitPercent float32,
	err error,
) {
	if bidFloor == constants.NEGATIVE_BIDFLOOR {
		bidFloor = constants.ZERO_BIDFLOOR
	}

	if dspPrice < bidFloor {
		return 0, 0, fmt.Errorf("DSP Price is lower thand bid floor")
	}

	finalDspPrice = dspPrice - dspPrice*profitPercent
	finalProfitPercent = profitPercent

	if finalDspPrice < bidFloor {
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
