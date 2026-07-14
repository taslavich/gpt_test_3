package bidEngine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	clickhouse_types "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/protobuf/proto"
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
) (*ortb_V2_5.BidResponse, clickhouse_types.UuidImpBidResponse, []string, []string) {
	if req == nil || req.GetBidRequest() == nil {
		return &ortb_V2_5.BidResponse{Seatbid: []*ortb_V2_5.SeatBid{{Bid: []*ortb_V2_5.Bid{}}}}, clickhouse_types.GetEmpty(ImpIdUuid), nil, nil
	}
	for l := range req.ImpIdUuid {
		if strings.TrimSpace(req.ImpIdUuid[l]) == "" {
			log.Printf("Empty uuid jjj")
		}
	}

	if len(req.BidResponses) == 0 {
		return &ortb_V2_5.BidResponse{
			Id: req.BidRequest.Id,
			Seatbid: []*ortb_V2_5.SeatBid{
				{
					Bid: []*ortb_V2_5.Bid{},
				},
			},
		}, clickhouse_types.GetEmpty(ImpIdUuid), nil, nil
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
		}, clickhouse_types.GetEmpty(ImpIdUuid), nil, nil
	}

	seatBid := []*ortb_V2_5.SeatBid{
		{
			Bid: []*ortb_V2_5.Bid{},
		},
	}
	clickhouseSeatBid := clickhouse_types.GetEmpty(ImpIdUuid)
	burlUUIDs := make([]string, 0, len(ImpIdUuid))
	admUUIDs := make([]string, 0, len(ImpIdUuid))

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
			if percentMapAdult != nil {
				percentMap = *percentMapAdult
			}
		case sppAdapterWeb.MAINSTREAM:
			if percentMapMainstream != nil {
				percentMap = *percentMapMainstream
			}
		}
		country := ""
		if req.GetBidRequest().GetDevice() != nil && req.GetBidRequest().GetDevice().GetGeo() != nil {
			country = req.GetBidRequest().GetDevice().GetGeo().GetCountry()
		}

		newBids := make([]*bidWithDomain, 0)
		for k := range bids {
			value := utils.GetValueFomSspGeoDspMap(req.SspDomain, country, bids[k].domain, percentMap, &types.PercentAndBidfloor{
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

		if len(newBids) == 0 {
			continue
		}
		sort.Slice(newBids, func(i, j int) bool {
			return newBids[i].finalPrice > newBids[j].finalPrice
		})

		winner := newBids[0]

		baseBid := &ortb_V2_5.Bid{
			Id:    winner.bid.Id,
			Impid: winner.bid.Impid,
			Price: &winner.finalPrice,
			Adm:   winner.bid.Adm,
			Nurl:  winner.bid.Nurl,
			Adid:  winner.bid.Adid,
			Cid:   winner.bid.Cid,
			Crid:  winner.bid.Crid,
		}
		finalBid, ok := FinalizeBidCallbacks(
			baseBid,
			admDomain,
			ImpIdUuid[impID],
			req.SspDomain,
			req.Format,
			logged,
			true,
		)
		if !ok {
			continue
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

		uuid := ImpIdUuid[impID]
		seatBid[0].Bid = append(seatBid[0].Bid, finalBid)
		clickhouseSeatBid[uuid] = clickhouseBid
		burlUUIDs = append(burlUUIDs, uuid)
		if logged {
			admUUIDs = append(admUUIDs, uuid)
		}
	}

	bidResponse := &ortb_V2_5.BidResponse{
		Id:      req.BidRequest.Id,
		Seatbid: seatBid,
	}

	return bidResponse, clickhouseSeatBid, burlUUIDs, admUUIDs
}

// FinalizeBidCallbacks is the single BidEngine callback-finalization path for
// both auctioned DSP bids and preselected ADV bids. The caller controls whether
// ADM and BURL wrappers are required, while NURL is wrapped whenever the source
// bid contains one.
func FinalizeBidCallbacks(
	source *ortb_V2_5.Bid,
	admDomain, globalID, sspDomain, format string,
	wrapADM, wrapBURL bool,
) (*ortb_V2_5.Bid, bool) {
	if source == nil || strings.TrimSpace(admDomain) == "" || strings.TrimSpace(globalID) == "" || strings.TrimSpace(format) == "" {
		return nil, false
	}
	finalBid, ok := proto.Clone(source).(*ortb_V2_5.Bid)
	if !ok || finalBid == nil {
		return nil, false
	}

	// Never pass callback URLs supplied by the source unchanged.
	finalBid.Nurl = nil
	finalBid.Burl = nil

	if wrapADM {
		wrappedADM := utils.WrapURL(admDomain, source.GetAdm(), globalID, format)
		if wrappedADM == "" {
			return nil, false
		}
		finalBid.Adm = &wrappedADM
	}
	if source.GetNurl() != "" {
		wrappedNURL := utils.WrapNurlURL(admDomain, source.GetNurl(), globalID, sspDomain, format)
		if wrappedNURL == "" {
			return nil, false
		}
		finalBid.Nurl = &wrappedNURL
	}
	if wrapBURL {
		wrappedBURL := utils.WrapBurlURL(admDomain, globalID, format)
		if wrappedBURL == "" {
			return nil, false
		}
		finalBid.Burl = &wrappedBURL
	}
	return finalBid, true
}

// ADVUsesBURL preserves ADV billing semantics: NAT, BAN and POP are charged by
// BURL, while IPP is charged by the ADM callback.
func ADVUsesBURL(format string) bool {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case constants.NAT, constants.BAN, constants.POP:
		return true
	default:
		return false
	}
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
