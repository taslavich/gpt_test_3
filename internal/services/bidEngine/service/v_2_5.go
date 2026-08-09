package bidEngine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"

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
	for impID := range req.ImpIdUuid {
		if strings.TrimSpace(req.ImpIdUuid[impID]) == "" {
			log.Printf("Empty uuid jjj")
		}
	}

	type bidWithDomain struct {
		bid        *ortb_V2_5.Bid
		finalPrice float32
		domain     string
	}

	// ADV has already run its own auction in the Router. Keep the selected ADV
	// bid by impression here; BidEngine only finalizes that winner and never
	// compares it by price with downstream DSPs.
	advBids := make(map[string]*ortb_V2_5.Bid)
	if ready := req.GetReadyBidResponse(); ready != nil {
		for _, seat := range ready.GetSeatbid() {
			if seat == nil {
				continue
			}
			for _, bid := range seat.GetBid() {
				if bid == nil || strings.TrimSpace(bid.GetImpid()) == "" {
					continue
				}
				// ADV is expected to return at most one selected bid per impression.
				// Keep the first one if malformed input contains duplicates.
				if _, exists := advBids[bid.GetImpid()]; !exists {
					advBids[bid.GetImpid()] = bid
				}
			}
		}
	}

	// Group downstream DSP bids by impression exactly as before. The Router
	// sends DSPs only impressions that did not receive an ADV winner, but the
	// ADV lookup below is still authoritative if a bad DSP responds for another
	// impression.
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

	if len(req.BidResponses) > 0 && len(impBids) == 0 {
		jsonData, err := json.MarshalIndent(req.BidResponses, "", "  ")
		if err != nil {
			log.Printf("Got len of impBids = 0, Error marshaling: %v", err)
		}
		globalUuid := ""
		for _, uuid := range ImpIdUuid {
			globalUuid = uuid
			break
		}
		log.Printf("Got len of impBids = 0, global id %s, bid responses %s", globalUuid, string(jsonData))
	}

	seatBid := []*ortb_V2_5.SeatBid{{Bid: []*ortb_V2_5.Bid{}}}
	clickhouseSeatBid := clickhouse_types.GetEmpty(ImpIdUuid)
	burlUUIDs := make([]string, 0, len(ImpIdUuid))
	admUUIDs := make([]string, 0, len(ImpIdUuid))

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
	advDomain := "adv"

	for _, imp := range req.GetBidRequest().GetImp() {
		if imp == nil || strings.TrimSpace(imp.GetId()) == "" {
			continue
		}
		impID := imp.GetId()

		// ADV is a per-impression priority winner. If it exists, do not run the
		// downstream DSP auction for this impression.
		if advBid := advBids[impID]; advBid != nil {
			uuid := ImpIdUuid[impID]
			userID := req.GetWinnerUserIds()[impID]
			if strings.TrimSpace(uuid) == "" || strings.TrimSpace(userID) == "" {
				continue
			}

			basePriceValue, basePriceExists := req.GetWinnerBasePrices()[impID]
			if !basePriceExists || basePriceValue <= 0 || math.IsNaN(basePriceValue) || math.IsInf(basePriceValue, 0) {
				continue
			}

			finalBid, ok := FinalizeADVCallbacks(
				advBid,
				admDomain,
				uuid,
				req.GetSspDomain(),
				req.GetFormat(),
			)
			if !ok || finalBid == nil {
				continue
			}

			effectivePrice := finalBid.GetPrice()
			basePrice := float32(basePriceValue)
			cid, crid := finalBid.GetCid(), finalBid.GetCrid()
			clickhouseSeatBid[uuid] = &clickhouse_types.Bid{
				WinDspDomain: &advDomain,
				WinPrice:     &effectivePrice,
				WinDspPrice:  &basePrice,
				WinCid:       &cid,
				WinCrid:      &crid,
				WinUserId:    &userID,
			}
			seatBid[0].Bid = append(seatBid[0].Bid, finalBid)
			continue
		}

		// No ADV winner for this impression: run the existing DSP auction with
		// the same bidfloor/percent/finalPrice selection logic as before.
		bids := impBids[impID]
		if len(bids) == 0 {
			continue
		}

		bidFloor := imp.GetBidfloor()
		newBids := make([]*bidWithDomain, 0, len(bids))
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
			Ext:   winner.bid.Ext,
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

	// Preserve the legacy ADV-only response envelope. In mixed or DSP-only
	// requests the response envelope remains the legacy BidEngine one.
	if allRequestedImpressionsHaveADV(req.GetBidRequest(), advBids) && req.GetReadyBidResponse() != nil {
		if cloned, ok := proto.Clone(req.GetReadyBidResponse()).(*ortb_V2_5.BidResponse); ok && cloned != nil {
			cloned.Seatbid = seatBid
			return cloned, clickhouseSeatBid, burlUUIDs, admUUIDs
		}
	}

	return &ortb_V2_5.BidResponse{
		Id:      req.BidRequest.Id,
		Seatbid: seatBid,
	}, clickhouseSeatBid, burlUUIDs, admUUIDs
}

func allRequestedImpressionsHaveADV(request *ortb_V2_5.BidRequest, advBids map[string]*ortb_V2_5.Bid) bool {
	if request == nil || len(request.GetImp()) == 0 {
		return false
	}
	seen := 0
	for _, imp := range request.GetImp() {
		if imp == nil || strings.TrimSpace(imp.GetId()) == "" {
			continue
		}
		seen++
		if advBids[imp.GetId()] == nil {
			return false
		}
	}
	return seen > 0
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

	if !setClicksWinsCallback(finalBid, admDomain, globalID) {
		return nil, false
	}

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

// FinalizeADVCallbacks finalizes a preselected ADV winner without treating ADV
// as a downstream DSP. Native and image-banner click destinations are wrapped
// inside their ADM payloads; iframe banner ADM stays unchanged. NURL and BURL
// point only to this exchange and never contain an embedded downstream URL.
func FinalizeADVCallbacks(
	source *ortb_V2_5.Bid,
	admDomain, globalID, sspDomain, format string,
) (*ortb_V2_5.Bid, bool) {
	if source == nil {
		return nil, false
	}
	if isADVNativeFormat(format) {
		finalBid, ok := finalizeADVNativeCallbacks(source, admDomain, globalID, format)
		if !ok || !setClicksWinsCallback(finalBid, admDomain, globalID) {
			return nil, false
		}
		return finalBid, true
	}
	if isADVBannerFormat(format) {
		finalBid, ok := finalizeADVBannerCallbacks(source, admDomain, globalID, sspDomain, format)
		if !ok || !setClicksWinsCallback(finalBid, admDomain, globalID) {
			return nil, false
		}
		return finalBid, true
	}

	cleanSource, ok := proto.Clone(source).(*ortb_V2_5.Bid)
	if !ok || cleanSource == nil {
		return nil, false
	}
	// ADV cannot supply DSP callbacks. Discard them even if malformed input
	// contains such fields, then synthesize exchange-only NURL/BURL below.
	cleanSource.Nurl = nil
	cleanSource.Burl = nil

	finalBid, ok := FinalizeBidCallbacks(
		cleanSource,
		admDomain,
		globalID,
		sspDomain,
		format,
		true,
		true,
	)
	if !ok || finalBid == nil {
		return nil, false
	}

	wrappedNURL := utils.WrapADVNurlURL(admDomain, globalID, sspDomain, format)
	if wrappedNURL == "" {
		return nil, false
	}
	finalBid.Nurl = &wrappedNURL
	return finalBid, true
}

func setClicksWinsCallback(bid *ortb_V2_5.Bid, admDomain, globalID string) bool {
	if bid == nil {
		return false
	}
	cwin := utils.WrapClicksWinsURL(admDomain, globalID)
	if strings.TrimSpace(cwin) == "" {
		return false
	}
	if bid.Ext == nil {
		bid.Ext = &ortb_V2_5.BidExt{}
	}
	bid.Ext.Cwin = &cwin
	return true
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
