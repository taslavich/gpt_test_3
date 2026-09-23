package auction

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
)

const (
	TypeModelSimple  = 1
	TypeModelComplex = 2
	TypeModelMapOnly = 3

	PromoPercenterMinMargin = 0.30
	UnknownSegmentValue     = "__unknown__"
)

type PercentRoutingMode uint8

const (
	PercentRoutingPercenterFloor PercentRoutingMode = iota + 1
	PercentRoutingMapOnly
	PercentRoutingRTBComplexFallback
)

type PricingDecision struct {
	Mode       PercentRoutingMode
	MapPercent float64
	MapSource  string
	HardMin    float64
	MinMargin  float64
	MaxMargin  float64
	Percent    float64
}

func normalizeTypeModel(value int) int {
	if value == 0 {
		return TypeModelSimple
	}
	return value
}

func validTypeModel(value int) bool {
	value = normalizeTypeModel(value)
	return value == TypeModelSimple || value == TypeModelComplex || value == TypeModelMapOnly
}

func businessHardMin(campaign *Campaign) float64 {
	if campaign != nil && campaign.PromoSpendRemaining > 0 {
		return PromoPercenterMinMargin
	}
	// Outside an active promo there is no hard-coded business minimum. The
	// campaign-specific map entry, or ALL/ALL_RTB fallback from the map, is the
	// only floor.
	return 0
}

func (s *AuctionService) ResolvePricingDecision(campaign *Campaign) (PricingDecision, error) {
	if campaign == nil {
		return PricingDecision{}, fmt.Errorf("campaign is nil")
	}
	model := normalizeTypeModel(campaign.TypeModel)
	if !validTypeModel(model) {
		return PricingDecision{}, fmt.Errorf("campaign %s has invalid type_model %d", campaign.ID, campaign.TypeModel)
	}

	if s == nil || s.percents == nil {
		return PricingDecision{}, fmt.Errorf("campaign %s percent map is not configured", campaign.ID)
	}
	mapPercent, mapSource := s.percents.LookupForCampaignWithSource(campaign.ID, campaign.RTB)
	if mapSource == "" {
		return PricingDecision{}, fmt.Errorf("campaign %s percent map has no required fallback", campaign.ID)
	}
	if !finitePercent(mapPercent) {
		return PricingDecision{}, fmt.Errorf("campaign %s resolved invalid percent %.12f", campaign.ID, mapPercent)
	}

	effectiveCampaign := *campaign
	effectiveCampaign.PromoSpendRemaining = s.effectivePromoSpendRemaining(campaign)
	hardMin := businessHardMin(&effectiveCampaign)
	effectiveMapPercent := math.Max(mapPercent, hardMin)
	if effectiveMapPercent > MaxAdvertiserMargin {
		return PricingDecision{}, fmt.Errorf("campaign %s minimum margin %.12f exceeds maximum %.12f", campaign.ID, effectiveMapPercent, MaxAdvertiserMargin)
	}

	decision := PricingDecision{
		MapPercent: mapPercent,
		MapSource:  mapSource,
		HardMin:    hardMin,
		MinMargin:  effectiveMapPercent,
		MaxMargin:  MaxAdvertiserMargin,
	}

	if model == TypeModelMapOnly {
		decision.Mode = PercentRoutingMapOnly
		decision.Percent = effectiveMapPercent
		return decision, nil
	}
	if campaign.RTB && model == TypeModelComplex {
		decision.Mode = PercentRoutingRTBComplexFallback
		decision.Percent = effectiveMapPercent
		return decision, nil
	}

	decision.Mode = PercentRoutingPercenterFloor
	decision.Percent = decision.MinMargin
	// The safe baseline working point is exactly the resolved map/promo floor;
	// optimizer modes may move Percent upward but can never go below MinMargin
	// or above MaxMargin.
	return decision, nil
}

func finitePercent(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= MaxAdvertiserMargin
}

type PercenterSegment struct {
	SSPDomain  string
	Geo        string
	Browser    string
	Device     string
	OS         string
	SiteID     string
	CampaignID string
}

func BuildPercenterSegment(req *ortb.BidRequest, sspDomain, campaignID string) PercenterSegment {
	segment := PercenterSegment{
		SSPDomain:  segmentStringValue(sspDomain),
		Geo:        UnknownSegmentValue,
		Browser:    UnknownSegmentValue,
		Device:     UnknownSegmentValue,
		OS:         UnknownSegmentValue,
		SiteID:     UnknownSegmentValue,
		CampaignID: segmentStringValue(campaignID),
	}
	if req == nil {
		return segment
	}

	if device := req.GetDevice(); device != nil {
		if geo := device.GetGeo(); geo != nil {
			segment.Geo = segmentNormalizedStringPointer(geo.Country, normalizeCountry)
		}

		// An explicitly present empty OS is different from an absent OS. Only
		// derive OS from UA when the OpenRTB OS field is actually absent.
		if device.Os != nil {
			segment.OS = normalizeOS(*device.Os)
		}

		if device.Ua != nil {
			rawUA := strings.TrimSpace(*device.Ua)
			if rawUA == "" {
				segment.Browser = ""
				if device.DeviceType == nil {
					segment.Device = ""
				}
			} else {
				parsed := ua.ParseUA(rawUA)
				segment.Browser = normalizeBrowser(parsed.Browser)
				segment.Device = normalizeDeviceType(parsed.Device)
				if device.Os == nil {
					segment.OS = normalizeOS(parsed.OS)
				}
			}
		}
		if device.DeviceType != nil && (device.Ua == nil || strings.TrimSpace(device.GetUa()) == "") {
			segment.Device = normalizeDeviceType(strconv.Itoa(int(device.GetDeviceType())))
		}
	}

	if site := req.GetSite(); site != nil {
		segment.SiteID = segmentNormalizedStringPointer(site.Id, strings.TrimSpace)
	}
	return segment
}

func segmentStringValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		// These call-site dimensions are plain strings, so absent and explicit
		// empty cannot be distinguished. Treat the ambiguous blank as missing
		// rather than inventing a false empty-vs-missing distinction.
		return UnknownSegmentValue
	}
	return value
}

func segmentNormalizedStringPointer(value *string, normalize func(string) string) string {
	if value == nil {
		return UnknownSegmentValue
	}
	return normalize(*value)
}

func (s PercenterSegment) Hash() string {
	parts := []string{s.SSPDomain, s.Geo, s.Browser, s.Device, s.OS, s.SiteID, s.CampaignID}
	h := sha256.New()
	for _, part := range parts {
		// Length-prefix every component so different field boundaries cannot
		// produce the same byte stream.
		_, _ = fmt.Fprintf(h, "%d:%s|", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func BuildPercenterSegmentHash(req *ortb.BidRequest, sspDomain, campaignID string) string {
	return BuildPercenterSegment(req, sspDomain, campaignID).Hash()
}

// BuildPercenterRequestHash is the pre-campaign exact request identity used for
// Stage 04 attribution. It intentionally omits campaign_id; the final
// BuildPercenterSegmentHash remains campaign-aware and is used for optimizer state.
func BuildPercenterRequestHash(req *ortb.BidRequest, sspDomain string) string {
	return BuildPercenterSegment(req, sspDomain, "").Hash()
}
