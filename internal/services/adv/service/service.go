package auction

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
)

const (
	SlotDuration         = 5 * time.Minute
	CampaignStatusActive = "active"
	PricingModelCPM      = "CPM"
	PricingModelCPC      = "CPC"
	TrafficAdult         = "ADULT"
	TrafficMainstream    = "MAINSTREAM"
	unknownTrackerValue  = "unknown"
)

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type Creative struct {
	ID             string
	CampaignID     string
	ADMURL         string
	TrackersMacros map[string]bool
	W              int
	H              int
	Name           string
	CreativeName   string
	Title          string
	Description    string
}

type Campaign struct {
	ID                 string
	UserID             string
	Status             string
	PricingModel       string
	Format             string
	TrafficType        string
	QualitySegment     string
	BasePrice          float64
	GoalTotalDollars   float64
	EvennessBySlotMode bool
	StartTS            time.Time
	EndTS              time.Time
	ActiveIntervals    []TimeRange

	CountryFilter    *filterV2.Filters
	LanguageFilter   *filterV2.Filters
	DeviceTypeFilter *filterV2.Filters
	OSFilter         *filterV2.Filters
	BrowserFilter    *filterV2.Filters
	SiteIDFilter     *filterV2.Filters
	IPFilter         *filterV2.Filters

	Creatives []*Creative
}

type Snapshot struct {
	Campaigns []*Campaign
	UserGoals map[string]float64
}

type AuctionRequestOptions struct {
	Format      string
	TrafficType string
	SSPDomain   string
	ImpIDUUID   map[string]string
}

type AuctionOutcome struct {
	BidResponse   *ortb.BidResponse
	WinnerUserIDs map[string]string
}

type candidate struct {
	campaign       *Campaign
	creatives      []*Creative
	chargePrice    float64
	effectivePrice float64
}

type AuctionService struct {
	snapshot  atomic.Pointer[Snapshot]
	runtime   *RuntimeStore
	winners   *WinnerStore
	percents  *PercentStore
	quality   *QualityStore
	admDomain string
}

func NewAuctionService(runtime *RuntimeStore, winners *WinnerStore, percents *PercentStore, quality *QualityStore, admDomain string) *AuctionService {
	s := &AuctionService{runtime: runtime, winners: winners, percents: percents, quality: quality, admDomain: strings.TrimSpace(admDomain)}
	s.snapshot.Store(&Snapshot{Campaigns: []*Campaign{}, UserGoals: map[string]float64{}})
	return s
}

func (s *AuctionService) Snapshot() *Snapshot {
	if s == nil {
		return nil
	}
	current := s.snapshot.Load()
	if current == nil {
		return nil
	}
	cloned, err := cloneAndValidateSnapshot(current)
	if err != nil {
		return nil
	}
	return cloned
}

func (s *AuctionService) currentSnapshot() *Snapshot {
	if s == nil {
		return nil
	}
	return s.snapshot.Load()
}

func (s *AuctionService) PublishSnapshot(snapshot *Snapshot) error {
	if s == nil {
		return errors.New("auction service is nil")
	}
	cloned, err := cloneAndValidateSnapshot(snapshot)
	if err != nil {
		return err
	}
	s.snapshot.Store(cloned)
	return nil
}

func cloneAndValidateSnapshot(src *Snapshot) (*Snapshot, error) {
	if src == nil {
		return nil, errors.New("snapshot is nil")
	}
	out := &Snapshot{
		Campaigns: make([]*Campaign, 0, len(src.Campaigns)),
		UserGoals: make(map[string]float64, len(src.UserGoals)),
	}
	for rawID, goal := range src.UserGoals {
		id := strings.TrimSpace(rawID)
		if id == "" || !finiteNonNegative(goal) {
			return nil, fmt.Errorf("invalid user goal for %q", rawID)
		}
		if _, duplicate := out.UserGoals[id]; duplicate {
			return nil, fmt.Errorf("duplicate normalized user id %q", id)
		}
		out.UserGoals[id] = goal
	}

	campaignIDs := make(map[string]struct{}, len(src.Campaigns))
	for _, campaign := range src.Campaigns {
		if campaign == nil {
			return nil, errors.New("snapshot contains nil campaign")
		}
		clone := *campaign
		clone.ID = strings.TrimSpace(clone.ID)
		clone.UserID = strings.TrimSpace(clone.UserID)
		clone.Status = strings.ToLower(strings.TrimSpace(clone.Status))
		clone.PricingModel = strings.ToUpper(strings.TrimSpace(clone.PricingModel))
		clone.Format = normalizeFormat(clone.Format)
		clone.TrafficType = normalizeTraffic(clone.TrafficType)
		clone.QualitySegment = strings.ToLower(strings.TrimSpace(clone.QualitySegment))

		if clone.ID == "" || clone.UserID == "" {
			return nil, errors.New("campaign has empty id or user_id")
		}
		if _, duplicate := campaignIDs[clone.ID]; duplicate {
			return nil, fmt.Errorf("duplicate campaign id %s", clone.ID)
		}
		campaignIDs[clone.ID] = struct{}{}
		if clone.Status != CampaignStatusActive {
			return nil, fmt.Errorf("campaign %s is not active", clone.ID)
		}
		if clone.PricingModel != PricingModelCPM && clone.PricingModel != PricingModelCPC {
			return nil, fmt.Errorf("campaign %s has invalid pricing model", clone.ID)
		}
		if clone.Format == "" || clone.TrafficType == "" {
			return nil, fmt.Errorf("campaign %s has invalid format or traffic type", clone.ID)
		}
		if _, ok := validQualitySegments[clone.QualitySegment]; !ok {
			return nil, fmt.Errorf("campaign %s has invalid quality segment", clone.ID)
		}
		if !finitePositive(clone.BasePrice) || !finiteNonNegative(clone.GoalTotalDollars) {
			return nil, fmt.Errorf("campaign %s has invalid price or goal", clone.ID)
		}
		if clone.StartTS.IsZero() || clone.EndTS.IsZero() || !clone.StartTS.Before(clone.EndTS) {
			return nil, fmt.Errorf("campaign %s has invalid time window", clone.ID)
		}
		clone.StartTS = clone.StartTS.UTC()
		clone.EndTS = clone.EndTS.UTC()
		clone.ActiveIntervals = make([]TimeRange, 0, len(campaign.ActiveIntervals))
		for _, interval := range campaign.ActiveIntervals {
			interval.Start = interval.Start.UTC()
			interval.End = interval.End.UTC()
			if interval.Start.Before(clone.StartTS) || interval.End.After(clone.EndTS) || !interval.Start.Before(interval.End) {
				return nil, fmt.Errorf("campaign %s has invalid active interval", clone.ID)
			}
			clone.ActiveIntervals = append(clone.ActiveIntervals, interval)
		}
		if _, ok := out.UserGoals[clone.UserID]; !ok {
			return nil, fmt.Errorf("campaign %s has no user goal", clone.ID)
		}

		clone.CountryFilter = cloneFilter(campaign.CountryFilter)
		clone.LanguageFilter = cloneFilter(campaign.LanguageFilter)
		clone.DeviceTypeFilter = cloneFilter(campaign.DeviceTypeFilter)
		clone.OSFilter = cloneFilter(campaign.OSFilter)
		clone.BrowserFilter = cloneFilter(campaign.BrowserFilter)
		clone.SiteIDFilter = cloneFilter(campaign.SiteIDFilter)
		clone.IPFilter = cloneFilter(campaign.IPFilter)
		clone.Creatives = make([]*Creative, 0, len(campaign.Creatives))
		creativeIDs := make(map[string]struct{}, len(campaign.Creatives))
		for _, creative := range campaign.Creatives {
			if creative == nil {
				return nil, fmt.Errorf("campaign %s contains nil creative", clone.ID)
			}
			cc := *creative
			cc.ID = strings.TrimSpace(cc.ID)
			cc.CampaignID = strings.TrimSpace(cc.CampaignID)
			cc.ADMURL = strings.TrimSpace(cc.ADMURL)
			if cc.ID == "" || cc.ADMURL == "" {
				return nil, fmt.Errorf("campaign %s has invalid creative", clone.ID)
			}
			if normalizeFormat(clone.Format) == "BAN" && (cc.W <= 0 || cc.H <= 0) {
				return nil, fmt.Errorf("banner campaign %s has creative %s without valid dimensions", clone.ID, cc.ID)
			}
			if cc.CampaignID == "" {
				cc.CampaignID = clone.ID
			}
			if cc.CampaignID != clone.ID {
				return nil, fmt.Errorf("creative %s belongs to another campaign", cc.ID)
			}
			if _, duplicate := creativeIDs[cc.ID]; duplicate {
				return nil, fmt.Errorf("campaign %s has duplicate creative %s", clone.ID, cc.ID)
			}
			creativeIDs[cc.ID] = struct{}{}
			cc.TrackersMacros = cloneBoolMap(creative.TrackersMacros)
			clone.Creatives = append(clone.Creatives, &cc)
		}
		out.Campaigns = append(out.Campaigns, &clone)
	}
	return out, nil
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func finitePositive(value float64) bool {
	return finiteNonNegative(value) && value > 0
}

func cloneFilter(src *filterV2.Filters) *filterV2.Filters {
	if src == nil {
		return nil
	}
	objects := make(map[string]bool, len(src.Objects))
	for key, value := range src.Objects {
		objects[key] = value
	}
	return &filterV2.Filters{Apply: src.Apply, IsWhiteList: src.IsWhiteList, Objects: objects}
}

func cloneBoolMap(src map[string]bool) map[string]bool {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]bool, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func (s *AuctionService) Auction(ctx context.Context, req *ortb.BidRequest, now time.Time, options AuctionRequestOptions) (*AuctionOutcome, error) {
	if s == nil || !validAuctionInput(req, options.ImpIDUUID) {
		return nil, ErrInvalidAuctionRequest
	}
	if s.runtime == nil || s.winners == nil || s.percents == nil || s.quality == nil {
		return nil, errors.New("auction dependencies are not initialized")
	}
	requestedFormat := normalizeFormat(options.Format)
	trafficType := normalizeTraffic(options.TrafficType)
	sspDomain := normalizeDomain(options.SSPDomain)
	if requestedFormat == "" || trafficType == "" || sspDomain == "" {
		return nil, ErrInvalidAuctionRequest
	}
	if !s.quality.ContainsAny(sspDomain) {
		return &AuctionOutcome{WinnerUserIDs: map[string]string{}}, nil
	}
	snapshot := s.currentSnapshot()
	if snapshot == nil {
		return nil, errors.New("campaign snapshot is unavailable")
	}

	seat := &ortb.SeatBid{Bid: make([]*ortb.Bid, 0, len(req.GetImp()))}
	winnerUsers := make(map[string]string)
	infrastructureErrors := 0

	for _, imp := range req.GetImp() {
		if imp == nil || strings.TrimSpace(imp.GetId()) == "" || !impressionMatchesFormat(imp, requestedFormat) {
			continue
		}
		impID := imp.GetId()
		winnerUUID := strings.TrimSpace(options.ImpIDUUID[impID])
		if winnerUUID == "" {
			continue
		}
		candidates := make([]candidate, 0, len(snapshot.Campaigns))
		for _, campaign := range snapshot.Campaigns {
			cand, eligible, infraErr := s.evaluateCampaign(ctx, snapshot, campaign, req, imp, now, requestedFormat, trafficType, sspDomain)
			if infraErr != nil {
				infrastructureErrors++
				continue
			}
			if eligible {
				candidates = append(candidates, cand)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].effectivePrice > candidates[j].effectivePrice })

		for _, cand := range candidates {
			if len(cand.creatives) == 0 {
				continue
			}
			creative := cand.creatives[rand.Intn(len(cand.creatives))]
			if creative == nil {
				continue
			}
			bid := s.buildBid(req, imp, cand.campaign, creative, winnerUUID, requestedFormat)
			if bid == nil {
				continue
			}
			winner := WinnerRecord{Price: cand.chargePrice, UserID: cand.campaign.UserID, CampaignID: cand.campaign.ID, Format: requestedFormat}
			if err := s.winners.Put(ctx, winnerUUID, winner); err != nil {
				infrastructureErrors++
				continue
			}
			seat.Bid = append(seat.Bid, bid)
			winnerUsers[impID] = cand.campaign.UserID
			break
		}
	}

	if len(seat.Bid) == 0 {
		if infrastructureErrors > 0 {
			return nil, fmt.Errorf("ADV Redis operations failed for %d candidates", infrastructureErrors)
		}
		return &AuctionOutcome{WinnerUserIDs: winnerUsers}, nil
	}
	responseID := uuid.NewString()
	currency := "USD"
	return &AuctionOutcome{
		BidResponse:   &ortb.BidResponse{Id: &responseID, Cur: &currency, Seatbid: []*ortb.SeatBid{seat}},
		WinnerUserIDs: winnerUsers,
	}, nil
}

func validAuctionInput(req *ortb.BidRequest, impIDUUID map[string]string) bool {
	if req == nil || len(req.GetImp()) == 0 || len(impIDUUID) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(req.GetImp()))
	seenUUIDs := make(map[string]struct{}, len(req.GetImp()))
	for _, imp := range req.GetImp() {
		if imp == nil {
			continue
		}
		impID := strings.TrimSpace(imp.GetId())
		if impID == "" {
			return false
		}
		if _, duplicate := seen[impID]; duplicate {
			return false
		}
		seen[impID] = struct{}{}
		winnerUUID := strings.TrimSpace(impIDUUID[impID])
		if winnerUUID != "" {
			if _, duplicate := seenUUIDs[winnerUUID]; duplicate {
				return false
			}
			seenUUIDs[winnerUUID] = struct{}{}
		}
	}
	return len(seen) > 0
}

func (s *AuctionService) evaluateCampaign(ctx context.Context, snapshot *Snapshot, campaign *Campaign, req *ortb.BidRequest, imp *ortb.Imp, now time.Time, requestedFormat, trafficType, sspDomain string) (candidate, bool, error) {
	if campaign == nil || normalizeFormat(campaign.Format) != requestedFormat {
		return candidate{}, false, nil
	}
	if normalizeTraffic(campaign.TrafficType) != trafficType {
		return candidate{}, false, nil
	}
	if !s.quality.Contains(campaign.QualitySegment, sspDomain) {
		return candidate{}, false, nil
	}
	if !campaignActiveAt(campaign, now) {
		return candidate{}, false, nil
	}
	chargePrice := CalculateChargePrice(campaign.BasePrice, campaign.PricingModel, requestedFormat)
	if chargePrice <= 0 || math.IsNaN(chargePrice) || math.IsInf(chargePrice, 0) {
		return candidate{}, false, nil
	}
	campaignSpent, err := s.runtime.CampaignSpent(ctx, campaign.ID)
	if err != nil {
		return candidate{}, false, err
	}
	if campaign.GoalTotalDollars-campaignSpent < chargePrice {
		return candidate{}, false, nil
	}
	if campaign.EvennessBySlotMode {
		eligible, err := s.runtime.PacingEligible(ctx, campaign, now, campaignSpent)
		if err != nil {
			return candidate{}, false, err
		}
		if !eligible {
			return candidate{}, false, nil
		}
	}
	userGoal, ok := snapshot.UserGoals[campaign.UserID]
	if !ok || userGoal < 0 || math.IsNaN(userGoal) || math.IsInf(userGoal, 0) {
		return candidate{}, false, nil
	}
	userSpent, err := s.runtime.UserSpent(ctx, campaign.UserID)
	if err != nil {
		return candidate{}, false, err
	}
	if userGoal-userSpent < chargePrice {
		return candidate{}, false, nil
	}
	creatives := matchingCreatives(campaign.Creatives, imp, requestedFormat)
	if len(creatives) == 0 {
		return candidate{}, false, nil
	}
	if !campaignPassesFilters(campaign, req) {
		return candidate{}, false, nil
	}
	deduction := s.percents.Lookup(trafficType, sspDomain, requestCountry(req), campaign.UserID)
	effective := CalculateEffectiveAuctionPrice(chargePrice, deduction)
	if effective <= 0 {
		return candidate{}, false, nil
	}
	return candidate{campaign: campaign, creatives: creatives, chargePrice: chargePrice, effectivePrice: effective}, true, nil
}

func (s *AuctionService) buildBid(req *ortb.BidRequest, imp *ortb.Imp, campaign *Campaign, creative *Creative, winnerUUID, format string) *ortb.Bid {
	if s == nil || imp == nil || campaign == nil || creative == nil || strings.TrimSpace(s.admDomain) == "" {
		return nil
	}
	originalADM := appendTrackerMacros(creative.ADMURL, creative.TrackersMacros, campaign.ID, creative.ID, req)
	if strings.TrimSpace(originalADM) == "" {
		return nil
	}
	adm := utils.WrapURL(s.admDomain, originalADM, winnerUUID, format)
	if adm == "" {
		return nil
	}
	burl := ""
	switch normalizeFormat(format) {
	case "NAT", "BAN", "POP":
		burl = utils.WrapBurlURL(s.admDomain, winnerUUID, format)
	}
	id, impID, cid, crid := creative.ID, imp.GetId(), campaign.ID, creative.ID
	price := float32(campaign.BasePrice)
	w, h := int32(creative.W), int32(creative.H)
	bid := &ortb.Bid{Id: &id, Impid: &impID, Price: &price, Adm: &adm, Cid: &cid, Crid: &crid, W: &w, H: &h}
	if burl != "" {
		bid.Burl = &burl
	}
	return bid
}

func campaignActiveAt(campaign *Campaign, now time.Time) bool {
	if campaign == nil || !strings.EqualFold(strings.TrimSpace(campaign.Status), CampaignStatusActive) {
		return false
	}
	if campaign.StartTS.IsZero() || campaign.EndTS.IsZero() || now.Before(campaign.StartTS) || !now.Before(campaign.EndTS) {
		return false
	}
	if len(campaign.ActiveIntervals) == 0 {
		return true
	}
	for _, interval := range campaign.ActiveIntervals {
		if !now.Before(interval.Start) && now.Before(interval.End) {
			return true
		}
	}
	return false
}

func matchingCreatives(creatives []*Creative, imp *ortb.Imp, format string) []*Creative {
	if normalizeFormat(format) != "BAN" {
		out := make([]*Creative, 0, len(creatives))
		for _, creative := range creatives {
			if creative != nil && strings.TrimSpace(creative.ID) != "" && strings.TrimSpace(creative.ADMURL) != "" {
				out = append(out, creative)
			}
		}
		return out
	}
	sizes := bannerSizes(imp)
	if len(sizes) == 0 {
		return nil
	}
	out := make([]*Creative, 0, len(creatives))
	for _, creative := range creatives {
		if creative == nil || creative.W <= 0 || creative.H <= 0 {
			continue
		}
		if sizes[[2]int{creative.W, creative.H}] {
			out = append(out, creative)
		}
	}
	return out
}

func bannerSizes(imp *ortb.Imp) map[[2]int]bool {
	if imp == nil || imp.GetBanner() == nil {
		return nil
	}
	banner := imp.GetBanner()
	out := make(map[[2]int]bool)
	for _, size := range banner.GetFormat() {
		if size != nil && size.GetW() > 0 && size.GetH() > 0 {
			out[[2]int{int(size.GetW()), int(size.GetH())}] = true
		}
	}
	if len(out) == 0 && banner.GetW() > 0 && banner.GetH() > 0 {
		out[[2]int{int(banner.GetW()), int(banner.GetH())}] = true
	}
	return out
}

func impressionMatchesFormat(imp *ortb.Imp, format string) bool {
	if imp == nil {
		return false
	}
	normalized := normalizeFormat(format)
	switch normalized {
	case "BAN", "IPP":
		banner := imp.GetBanner()
		if banner == nil || imp.GetNative() != nil {
			return false
		}
		expected := constants.ADVImpressionFormatMarkerPrefix + normalized
		for _, value := range banner.GetExt() {
			if strings.EqualFold(strings.TrimSpace(value), expected) {
				return true
			}
		}
		return false
	case "NAT":
		return imp.GetNative() != nil && imp.GetBanner() == nil
	case "POP":
		// OpenRTB 2.5 has no dedicated popunder object. In this project POP is
		// represented by an impression without banner/native objects.
		return imp.GetBanner() == nil && imp.GetNative() == nil
	default:
		return false
	}
}

func normalizeFormat(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "POP", "POPUNDER", "POP_UNDER":
		return "POP"
	case "BAN", "BANNER":
		return "BAN"
	case "NAT", "NATIVE":
		return "NAT"
	case "IPP", "IN-PAGE-PUSH", "IN_PAGE_PUSH":
		return "IPP"
	default:
		return ""
	}
}

func normalizeTraffic(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case TrafficAdult:
		return TrafficAdult
	case TrafficMainstream:
		return TrafficMainstream
	default:
		return ""
	}
}

func requestCountry(req *ortb.BidRequest) string {
	if req != nil && req.GetDevice() != nil && req.GetDevice().GetGeo() != nil {
		return strings.ToUpper(strings.TrimSpace(req.GetDevice().GetGeo().GetCountry()))
	}
	return ""
}

func campaignPassesFilters(c *Campaign, req *ortb.BidRequest) bool {
	if c == nil {
		return false
	}
	var country, language, deviceType, osName, browser, siteID, ip *string
	if req != nil {
		if device := req.GetDevice(); device != nil {
			if geo := device.GetGeo(); geo != nil {
				country = nonEmptyStringPtr(geo.GetCountry())
			}
			language = nonEmptyStringPtr(device.GetLanguage())
			if device.DeviceType != nil {
				value := strconv.Itoa(int(device.GetDeviceType()))
				deviceType = &value
			}
			osName = nonEmptyStringPtr(device.GetOs())
			ip = nonEmptyStringPtr(device.GetIp())
			parsed := ua.ParseUA(device.GetUa())
			browser = nonEmptyStringPtr(parsed.Browser)
			if osName == nil {
				osName = nonEmptyStringPtr(parsed.OS)
			}
		}
		if site := req.GetSite(); site != nil {
			siteID = nonEmptyStringPtr(site.GetId())
		}
	}
	return allowed(c.CountryFilter, country) &&
		allowed(c.LanguageFilter, language) &&
		allowed(c.DeviceTypeFilter, deviceType) &&
		allowed(c.OSFilter, osName) &&
		allowed(c.BrowserFilter, browser) &&
		allowed(c.SiteIDFilter, siteID) &&
		allowed(c.IPFilter, ip)
}

func allowed(filter *filterV2.Filters, value *string) bool {
	if filter == nil {
		return true
	}
	return filter.Allowed(value)
}

func nonEmptyStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func appendTrackerMacros(admURL string, macros map[string]bool, campaignID, creativeID string, req *ortb.BidRequest) string {
	if len(macros) == 0 {
		return admURL
	}
	parsed, err := url.Parse(admURL)
	if err != nil {
		return admURL
	}
	q := parsed.Query()
	for key, enabled := range macros {
		if !enabled || !supportedTrackerMacro(key) {
			continue
		}
		q.Set(key, trackerMacroValue(key, campaignID, creativeID, req))
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func supportedTrackerMacro(key string) bool {
	switch key {
	case "device", "browser", "site_id", "device_os", "ip_address", "campaign_id", "creative_id", "country_code":
		return true
	default:
		return false
	}
}

func trackerMacroValue(key, campaignID, creativeID string, req *ortb.BidRequest) string {
	value := ""
	switch key {
	case "campaign_id":
		value = campaignID
	case "creative_id":
		value = creativeID
	case "site_id":
		if req != nil && req.GetSite() != nil {
			value = req.GetSite().GetId()
		}
	case "ip_address":
		if req != nil && req.GetDevice() != nil {
			value = req.GetDevice().GetIp()
		}
	case "country_code":
		if req != nil && req.GetDevice() != nil && req.GetDevice().GetGeo() != nil {
			value = req.GetDevice().GetGeo().GetCountry()
		}
	case "device_os":
		if req != nil && req.GetDevice() != nil {
			value = req.GetDevice().GetOs()
			if strings.TrimSpace(value) == "" {
				value = ua.ParseUA(req.GetDevice().GetUa()).OS
			}
		}
	case "browser":
		if req != nil && req.GetDevice() != nil {
			value = ua.ParseUA(req.GetDevice().GetUa()).Browser
		}
	case "device":
		if req != nil && req.GetDevice() != nil {
			value = ua.ParseUA(req.GetDevice().GetUa()).Device
		}
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return unknownTrackerValue
	}
	return value
}

var ErrInvalidAuctionRequest = errors.New("invalid ADV auction request")
