package auction

import (
	jsoniter "github.com/json-iterator/go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/rtbrules"
	"strings"
)

// Rules apply only to copies created for RTB campaigns. BannerPartner's BAN
// normalization is deliberately left exactly as before and takes precedence.
func marshalRTBRequestForCampaignWithRules(req *ortb.BidRequest, campaign *Campaign, format string, store *rtbrules.Store) ([]byte, error) {
	if campaign == nil {
		return jsoniter.Marshal(req)
	}
	isBannerPartnerBAN := strings.TrimSpace(campaign.UserID) == rtbBannerPartnerUserID && normalizeFormat(format) == constants.BAN
	if isBannerPartnerBAN {
		body, err := jsoniter.Marshal(req)
		if err != nil {
			return nil, err
		}
		var payload map[string]any
		if err = jsoniter.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		normalizeBannerPartnerBANRTBRequest(payload)
		return jsoniter.Marshal(payload)
	}
	if !campaign.RTB || store == nil || !store.Has(strings.TrimSpace(campaign.ID)) {
		return jsoniter.Marshal(req)
	}
	return store.ApplyToRequest(strings.TrimSpace(campaign.ID), req)
}
