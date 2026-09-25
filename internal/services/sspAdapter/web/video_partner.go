package sppAdapterWeb

import (
	"encoding/json"
	"strconv"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// postBidPayloadV25 keeps the regular OpenRTB request plus the one partner
// extension that must be normalized before the adapter overwrites imp.ext with
// its internal subid. Partner-specific values never leave the SSP adapter.
type postBidPayloadV25 struct {
	*ortb.BidRequest
	partnerPlacementByIndex map[int]int32
}

func (p *postBidPayloadV25) UnmarshalJSON(data []byte) error {
	var request ortb.BidRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return err
	}

	var raw struct {
		Imp []struct {
			Ext json.RawMessage `json:"ext"`
		} `json:"imp"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	placements := make(map[int]int32)
	for i, imp := range raw.Imp {
		if len(imp.Ext) == 0 || string(imp.Ext) == "null" {
			continue
		}
		var ext map[string]json.RawMessage
		if err := json.Unmarshal(imp.Ext, &ext); err != nil {
			// Unknown/malformed partner extension is not a malformed OpenRTB bid
			// request. Treat it as unknown placement and let VIDEO eligibility
			// fail closed later if a concrete video_format is required.
			continue
		}
		rawPL, ok := ext["pl"]
		if !ok {
			continue
		}
		var numeric int32
		if err := json.Unmarshal(rawPL, &numeric); err == nil {
			placements[i] = numeric
			continue
		}
		var text string
		if err := json.Unmarshal(rawPL, &text); err == nil {
			value, parseErr := strconv.ParseInt(strings.TrimSpace(text), 10, 32)
			if parseErr == nil {
				placements[i] = int32(value)
			}
		}
	}

	p.BidRequest = &request
	p.partnerPlacementByIndex = placements
	return nil
}

func isMyBidSSP(sspDomain string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(sspDomain)), "mybid")
}

func normalizePartnerVideoPlacement(payload *postBidPayloadV25, sspDomain, format string) {
	if payload == nil || payload.BidRequest == nil || !strings.EqualFold(strings.TrimSpace(format), constants.VID) || !isMyBidSSP(sspDomain) {
		return
	}
	for i, imp := range payload.BidRequest.GetImp() {
		if imp == nil || imp.GetVideo() == nil {
			continue
		}
		video := imp.GetVideo()
		// Standard OpenRTB placement always has priority over partner data.
		if video.Placement != nil && video.GetPlacement() > 0 {
			continue
		}
		pl, ok := payload.partnerPlacementByIndex[i]
		if !ok {
			// For MyBid the agreed fallback signal is ext.pl, not plcmt. If
			// neither standard placement nor ext.pl can classify the inventory,
			// keep it unknown so a concrete video_format fails closed.
			video.Plcmt = nil
			continue
		}
		var placement int32
		switch pl {
		case 61:
			placement = 1 // instream
		case 306, 308:
			placement = 3 // outstream (OpenRTB 2.5 in-article mapping)
		default:
			video.Plcmt = nil
			continue
		}
		video.Placement = &placement
	}
}
