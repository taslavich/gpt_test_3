package auction

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
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

type debugLogFunc func(format string, args ...any)

func shouldLogSSPDomain(value string) bool {
	switch normalizeDomain(value) {
	case "adl_test", "mc_test", "1":
		return true
	default:
		return false
	}
}

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
	snapshot atomic.Pointer[Snapshot]
	runtime  *RuntimeStore
	winners  *WinnerStore
	percents *PercentStore
	quality  *QualityStore
}

func NewAuctionService(runtime *RuntimeStore, winners *WinnerStore, percents *PercentStore, quality *QualityStore) *AuctionService {
	s := &AuctionService{runtime: runtime, winners: winners, percents: percents, quality: quality}
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
	requestedFormat := normalizeFormat(options.Format)
	trafficType := normalizeTraffic(options.TrafficType)
	sspDomain := normalizeDomain(options.SSPDomain)
	requestID := ""
	if req != nil {
		requestID = strings.TrimSpace(req.GetId())
	}

	logEnabled := shouldLogSSPDomain(sspDomain)
	logf := debugLogFunc(func(format string, args ...any) {
		if logEnabled {
			log.Printf(format, args...)
		}
	})

	logf(
		"[ADV][AUCTION_START] request_id=%q format=%q traffic_type=%q ssp_domain=%q impressions=%d imp_uuid_count=%d",
		requestID,
		requestedFormat,
		trafficType,
		sspDomain,
		len(req.GetImp()),
		len(options.ImpIDUUID),
	)

	if s == nil || !validAuctionInput(req, options.ImpIDUUID) {
		logf(
			"[ADV][AUCTION_REJECT] request_id=%q reason=invalid_input request_nil=%t impressions=%d imp_uuid_count=%d",
			requestID,
			req == nil,
			len(req.GetImp()),
			len(options.ImpIDUUID),
		)
		return nil, ErrInvalidAuctionRequest
	}
	if s.runtime == nil || s.winners == nil || s.percents == nil || s.quality == nil {
		logf(
			"[ADV][AUCTION_ERROR] request_id=%q reason=dependencies_not_initialized runtime_nil=%t winners_nil=%t percents_nil=%t quality_nil=%t",
			requestID,
			s.runtime == nil,
			s.winners == nil,
			s.percents == nil,
			s.quality == nil,
		)
		return nil, errors.New("auction dependencies are not initialized")
	}

	if requestedFormat == "" || trafficType == "" || sspDomain == "" {
		logf(
			"[ADV][AUCTION_REJECT] request_id=%q reason=invalid_format_traffic_or_domain raw_format=%q raw_traffic_type=%q raw_ssp_domain=%q normalized_format=%q normalized_traffic_type=%q normalized_ssp_domain=%q",
			requestID,
			options.Format,
			options.TrafficType,
			options.SSPDomain,
			requestedFormat,
			trafficType,
			sspDomain,
		)
		return nil, ErrInvalidAuctionRequest
	}

	if !s.quality.ContainsAny(sspDomain) {
		logf(
			"[ADV][AUCTION_NO_BID] request_id=%q reason=ssp_not_in_any_quality_map ssp_domain=%q",
			requestID,
			sspDomain,
		)
		return &AuctionOutcome{WinnerUserIDs: map[string]string{}}, nil
	}

	snapshot := s.currentSnapshot()
	if snapshot == nil {
		logf(
			"[ADV][AUCTION_ERROR] request_id=%q reason=campaign_snapshot_unavailable",
			requestID,
		)
		return nil, errors.New("campaign snapshot is unavailable")
	}

	logf(
		"[ADV][SNAPSHOT] request_id=%q campaigns=%d user_goals=%d",
		requestID,
		len(snapshot.Campaigns),
		len(snapshot.UserGoals),
	)

	seat := &ortb.SeatBid{Bid: make([]*ortb.Bid, 0, len(req.GetImp()))}
	winnerUsers := make(map[string]string)
	infrastructureErrors := 0

	for idx, imp := range req.GetImp() {
		if imp == nil {
			logf(
				"[ADV][IMP_REJECT] request_id=%q imp_index=%d reason=impression_nil",
				requestID,
				idx,
			)
			continue
		}

		impID := strings.TrimSpace(imp.GetId())
		if impID == "" {
			logf(
				"[ADV][IMP_REJECT] request_id=%q imp_index=%d reason=imp_id_empty",
				requestID,
				idx,
			)
			continue
		}

		if !impressionMatchesFormat(imp, requestedFormat) {
			logf(
				"[ADV][IMP_REJECT] request_id=%q imp_id=%q reason=impression_format_mismatch requested_format=%q has_banner=%t has_native=%t banner_ext=%q",
				requestID,
				impID,
				requestedFormat,
				imp.GetBanner() != nil,
				imp.GetNative() != nil,
				bannerExtValues(imp),
			)
			continue
		}

		winnerUUID := strings.TrimSpace(options.ImpIDUUID[impID])
		if winnerUUID == "" {
			logf(
				"[ADV][IMP_REJECT] request_id=%q imp_id=%q reason=winner_uuid_missing",
				requestID,
				impID,
			)
			continue
		}

		logf(
			"[ADV][IMP_START] request_id=%q imp_id=%q winner_uuid=%q campaigns_to_check=%d",
			requestID,
			impID,
			winnerUUID,
			len(snapshot.Campaigns),
		)

		candidates := make([]candidate, 0, len(snapshot.Campaigns))
		for _, campaign := range snapshot.Campaigns {
			cand, eligible, infraErr := s.evaluateCampaign(
				ctx,
				snapshot,
				campaign,
				req,
				imp,
				now,
				requestedFormat,
				trafficType,
				sspDomain,
				logf,
			)
			if infraErr != nil {
				infrastructureErrors++
				campaignID := ""
				userID := ""
				if campaign != nil {
					campaignID = campaign.ID
					userID = campaign.UserID
				}
				logf(
					"[ADV][CAMPAIGN_ERROR] request_id=%q imp_id=%q campaign_id=%q user_id=%q error=%v",
					requestID,
					impID,
					campaignID,
					userID,
					infraErr,
				)
				continue
			}
			if eligible {
				candidates = append(candidates, cand)
			}
		}

		logf(
			"[ADV][IMP_CANDIDATES] request_id=%q imp_id=%q eligible=%d checked=%d infrastructure_errors=%d",
			requestID,
			impID,
			len(candidates),
			len(snapshot.Campaigns),
			infrastructureErrors,
		)
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].effectivePrice > candidates[j].effectivePrice })

		for _, cand := range candidates {
			if len(cand.creatives) == 0 {
				logf(
					"[ADV][CANDIDATE_SKIP] request_id=%q imp_id=%q campaign_id=%q reason=eligible_candidate_has_no_creatives",
					requestID,
					impID,
					cand.campaign.ID,
				)
				continue
			}
			creative := cand.creatives[rand.Intn(len(cand.creatives))]
			if creative == nil {
				logf(
					"[ADV][CANDIDATE_SKIP] request_id=%q imp_id=%q campaign_id=%q reason=random_creative_nil matched_creatives=%d",
					requestID,
					impID,
					cand.campaign.ID,
					len(cand.creatives),
				)
				continue
			}
			bid := s.buildBid(req, imp, cand.campaign, creative, cand.effectivePrice)
			if bid == nil {
				logf(
					"[ADV][CANDIDATE_SKIP] request_id=%q imp_id=%q campaign_id=%q creative_id=%q reason=bid_build_failed adm_url_empty=%t",
					requestID,
					impID,
					cand.campaign.ID,
					creative.ID,
					strings.TrimSpace(creative.ADMURL) == "",
				)
				continue
			}

			winner := WinnerRecord{
				Price:      cand.chargePrice,
				UserID:     cand.campaign.UserID,
				CampaignID: cand.campaign.ID,
				Format:     requestedFormat,
			}
			if err := s.winners.Put(ctx, winnerUUID, winner); err != nil {
				logf(
					"[ADV][WINNER_REDIS_ERROR] request_id=%q imp_id=%q winner_uuid=%q campaign_id=%q creative_id=%q user_id=%q error=%v",
					requestID,
					impID,
					winnerUUID,
					cand.campaign.ID,
					creative.ID,
					cand.campaign.UserID,
					err,
				)
				infrastructureErrors++
				continue
			}

			seat.Bid = append(seat.Bid, bid)
			winnerUsers[impID] = cand.campaign.UserID
			logf(
				"[ADV][WINNER] request_id=%q imp_id=%q winner_uuid=%q campaign_id=%q creative_id=%q user_id=%q charge_price=%.12f effective_price=%.12f matched_creatives=%d",
				requestID,
				impID,
				winnerUUID,
				cand.campaign.ID,
				creative.ID,
				cand.campaign.UserID,
				cand.chargePrice,
				cand.effectivePrice,
				len(cand.creatives),
			)
			break
		}
	}

	if len(seat.Bid) == 0 {
		if infrastructureErrors > 0 {
			logf(
				"[ADV][AUCTION_ERROR] request_id=%q reason=redis_operations_failed errors=%d",
				requestID,
				infrastructureErrors,
			)
			return nil, fmt.Errorf("ADV Redis operations failed for %d candidates", infrastructureErrors)
		}
		logf(
			"[ADV][AUCTION_NO_BID] request_id=%q impressions=%d winner_user_ids=%d",
			requestID,
			len(req.GetImp()),
			len(winnerUsers),
		)
		return &AuctionOutcome{WinnerUserIDs: winnerUsers}, nil
	}

	responseID := uuid.NewString()
	currency := "USD"
	logf(
		"[ADV][AUCTION_SUCCESS] request_id=%q response_id=%q bids=%d winner_user_ids=%d",
		requestID,
		responseID,
		len(seat.Bid),
		len(winnerUsers),
	)
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

func (s *AuctionService) evaluateCampaign(
	ctx context.Context,
	snapshot *Snapshot,
	campaign *Campaign,
	req *ortb.BidRequest,
	imp *ortb.Imp,
	now time.Time,
	requestedFormat, trafficType, sspDomain string,
	logf debugLogFunc,
) (candidate, bool, error) {
	requestID := ""
	if req != nil {
		requestID = strings.TrimSpace(req.GetId())
	}
	impID := ""
	if imp != nil {
		impID = strings.TrimSpace(imp.GetId())
	}

	if campaign == nil {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q reason=campaign_nil",
			requestID,
			impID,
		)
		return candidate{}, false, nil
	}

	campaignID := strings.TrimSpace(campaign.ID)
	userID := strings.TrimSpace(campaign.UserID)

	if !s.quality.Contains(campaign.QualitySegment, sspDomain) {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=quality_mismatch campaign_quality=%q ssp_domain=%q",
			requestID,
			impID,
			campaignID,
			userID,
			campaign.QualitySegment,
			sspDomain,
		)
		return candidate{}, false, nil
	}
	if normalizeFormat(campaign.Format) != requestedFormat {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=format_mismatch campaign_format=%q requested_format=%q",
			requestID,
			impID,
			campaignID,
			userID,
			normalizeFormat(campaign.Format),
			requestedFormat,
		)
		return candidate{}, false, nil
	}
	if normalizeTraffic(campaign.TrafficType) != trafficType {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=traffic_type_mismatch campaign_traffic_type=%q requested_traffic_type=%q",
			requestID,
			impID,
			campaignID,
			userID,
			normalizeTraffic(campaign.TrafficType),
			trafficType,
		)
		return candidate{}, false, nil
	}
	if !campaignActiveAt(campaign, now) {
		logCampaignActivityRejection(logf, requestID, impID, campaign, now)
		return candidate{}, false, nil
	}

	chargePrice := CalculateChargePrice(campaign.BasePrice, campaign.PricingModel, requestedFormat)
	if chargePrice <= 0 || math.IsNaN(chargePrice) || math.IsInf(chargePrice, 0) {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=invalid_charge_price base_price=%.12f pricing_model=%q requested_format=%q charge_price=%.12f",
			requestID,
			impID,
			campaignID,
			userID,
			campaign.BasePrice,
			campaign.PricingModel,
			requestedFormat,
			chargePrice,
		)
		return candidate{}, false, nil
	}

	campaignSpent, err := s.runtime.CampaignSpent(ctx, campaign.ID)
	if err != nil {
		logf(
			"[ADV][CAMPAIGN_ERROR] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=campaign_spent_read_failed error=%v",
			requestID,
			impID,
			campaignID,
			userID,
			err,
		)
		return candidate{}, false, err
	}
	campaignRemaining := campaign.GoalTotalDollars - campaignSpent
	if campaignRemaining < chargePrice {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=campaign_balance_insufficient campaign_goal_total_dollars=%.12f campaign_spent=%.12f campaign_remaining=%.12f charge_price=%.12f",
			requestID,
			impID,
			campaignID,
			userID,
			campaign.GoalTotalDollars,
			campaignSpent,
			campaignRemaining,
			chargePrice,
		)
		return candidate{}, false, nil
	}

	if campaign.EvennessBySlotMode {
		eligible, err := s.runtime.PacingEligible(ctx, campaign, now, campaignSpent)
		if err != nil {
			logf(
				"[ADV][CAMPAIGN_ERROR] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=pacing_check_failed campaign_spent=%.12f error=%v",
				requestID,
				impID,
				campaignID,
				userID,
				campaignSpent,
				err,
			)
			return candidate{}, false, err
		}
		if !eligible {
			s.logPacingRejection(ctx, campaign, now, campaignSpent, requestID, impID, logf)
			return candidate{}, false, nil
		}
	}

	userGoal, ok := snapshot.UserGoals[campaign.UserID]
	if !ok || userGoal < 0 || math.IsNaN(userGoal) || math.IsInf(userGoal, 0) {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=user_goal_total_dollars_missing_or_invalid user_goal_exists=%t user_goal_total_dollars=%.12f",
			requestID,
			impID,
			campaignID,
			userID,
			ok,
			userGoal,
		)
		return candidate{}, false, nil
	}
	userSpent, err := s.runtime.UserSpent(ctx, campaign.UserID)
	if err != nil {
		logf(
			"[ADV][CAMPAIGN_ERROR] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=user_spent_read_failed error=%v",
			requestID,
			impID,
			campaignID,
			userID,
			err,
		)
		return candidate{}, false, err
	}
	userRemaining := userGoal - userSpent
	if userRemaining < chargePrice {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=user_balance_insufficient user_goal_total_dollars=%.12f user_spent=%.12f user_remaining=%.12f charge_price=%.12f",
			requestID,
			impID,
			campaignID,
			userID,
			userGoal,
			userSpent,
			userRemaining,
			chargePrice,
		)
		return candidate{}, false, nil
	}

	creatives := matchingCreatives(campaign.Creatives, imp, requestedFormat)
	if len(creatives) == 0 {
		logCreativeRejections(logf, requestID, impID, campaign, imp, requestedFormat)
		return candidate{}, false, nil
	}

	if !campaignPassesFiltersWithDebug(campaign, req, requestID, impID, logf) {
		return candidate{}, false, nil
	}

	deduction := s.percents.Lookup(campaign.UserID)
	effective := CalculateEffectiveAuctionPrice(chargePrice, deduction)
	if effective <= 0 || math.IsNaN(effective) || math.IsInf(effective, 0) {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=effective_price_non_positive charge_price=%.12f deduction=%.12f effective_price=%.12f",
			requestID,
			impID,
			campaignID,
			userID,
			chargePrice,
			deduction,
			effective,
		)
		return candidate{}, false, nil
	}

	logf(
		"[ADV][CAMPAIGN_ELIGIBLE] request_id=%q imp_id=%q campaign_id=%q user_id=%q creatives=%d charge_price=%.12f deduction=%.12f effective_price=%.12f campaign_remaining=%.12f user_remaining=%.12f",
		requestID,
		impID,
		campaignID,
		userID,
		len(creatives),
		chargePrice,
		deduction,
		effective,
		campaignRemaining,
		userRemaining,
	)
	return candidate{campaign: campaign, creatives: creatives, chargePrice: chargePrice, effectivePrice: effective}, true, nil
}

func logCampaignActivityRejection(logf debugLogFunc, requestID, impID string, campaign *Campaign, now time.Time) {
	if campaign == nil {
		return
	}

	reason := "outside_active_intervals"
	switch {
	case !strings.EqualFold(strings.TrimSpace(campaign.Status), CampaignStatusActive):
		reason = "campaign_status_not_active"
	case campaign.StartTS.IsZero() || campaign.EndTS.IsZero() || !campaign.StartTS.Before(campaign.EndTS):
		reason = "campaign_time_window_invalid"
	case now.Before(campaign.StartTS):
		reason = "campaign_not_started"
	case !now.Before(campaign.EndTS):
		reason = "campaign_ended"
	}

	logf(
		"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=%s status=%q now=%q start=%q end=%q active_intervals=%d",
		requestID,
		impID,
		campaign.ID,
		campaign.UserID,
		reason,
		campaign.Status,
		now.UTC().Format(time.RFC3339Nano),
		campaign.StartTS.UTC().Format(time.RFC3339Nano),
		campaign.EndTS.UTC().Format(time.RFC3339Nano),
		len(campaign.ActiveIntervals),
	)

	if reason == "outside_active_intervals" {
		for index, interval := range campaign.ActiveIntervals {
			logf(
				"[ADV][ACTIVE_INTERVAL] request_id=%q imp_id=%q campaign_id=%q index=%d start=%q end=%q now_inside=%t",
				requestID,
				impID,
				campaign.ID,
				index,
				interval.Start.UTC().Format(time.RFC3339Nano),
				interval.End.UTC().Format(time.RFC3339Nano),
				!now.Before(interval.Start) && now.Before(interval.End),
			)
		}
	}
}

func (s *AuctionService) logPacingRejection(
	ctx context.Context,
	campaign *Campaign,
	now time.Time,
	campaignSpent float64,
	requestID, impID string,
	logf debugLogFunc,
) {
	if s == nil || s.runtime == nil || s.runtime.client == nil || campaign == nil {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q reason=pacing_runtime_unavailable",
			requestID,
			impID,
			campaignID(campaign),
		)
		return
	}

	currentKey := pacingCurrentPrefix + campaign.ID
	slotKey, err := s.runtime.client.Get(ctx, currentKey).Result()
	if errors.Is(err, redis.Nil) {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=pacing_current_slot_key_missing current_key=%q",
			requestID,
			impID,
			campaign.ID,
			campaign.UserID,
			currentKey,
		)
		return
	}
	if err != nil {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=pacing_current_slot_read_failed current_key=%q error=%v",
			requestID,
			impID,
			campaign.ID,
			campaign.UserID,
			currentKey,
			err,
		)
		return
	}
	if !strings.HasPrefix(slotKey, pacingSpentPrefix+campaign.ID+":") {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=pacing_current_slot_key_invalid current_key=%q slot_key=%q expected_prefix=%q",
			requestID,
			impID,
			campaign.ID,
			campaign.UserID,
			currentKey,
			slotKey,
			pacingSpentPrefix+campaign.ID+":",
		)
		return
	}

	slotSpent, err := s.runtime.floatValue(ctx, slotKey)
	if err != nil {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=pacing_slot_spent_read_failed slot_key=%q error=%v",
			requestID,
			impID,
			campaign.ID,
			campaign.UserID,
			slotKey,
			err,
		)
		return
	}
	slotTarget, err := pacingSlotTarget(campaign, now, campaignSpent, slotSpent)
	if err != nil {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=pacing_slot_target_failed campaign_spent=%.12f slot_spent=%.12f error=%v",
			requestID,
			impID,
			campaign.ID,
			campaign.UserID,
			campaignSpent,
			slotSpent,
			err,
		)
		return
	}

	reason := "pacing_slot_limit_reached"
	if slotTarget <= 0 {
		reason = "pacing_slot_target_non_positive"
	}
	logf(
		"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=%s campaign_spent=%.12f slot_spent=%.12f slot_target=%.12f remaining_to_slot_target=%.12f current_slot_fraction=%.12f active_slots_left=%.12f slot_key=%q",
		requestID,
		impID,
		campaign.ID,
		campaign.UserID,
		reason,
		campaignSpent,
		slotSpent,
		slotTarget,
		slotTarget-slotSpent,
		CurrentSlotActiveFraction(campaign, now),
		ActiveSlotsLeft(campaign, now.UTC().Truncate(SlotDuration)),
		slotKey,
	)
}

func campaignID(campaign *Campaign) string {
	if campaign == nil {
		return ""
	}
	return campaign.ID
}

func logCreativeRejections(
	logf debugLogFunc,
	requestID, impID string,
	campaign *Campaign,
	imp *ortb.Imp,
	format string,
) {
	if campaign == nil {
		return
	}
	logf(
		"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=no_matching_creatives format=%q creatives_total=%d requested_banner_sizes=%q",
		requestID,
		impID,
		campaign.ID,
		campaign.UserID,
		format,
		len(campaign.Creatives),
		formatBannerSizes(bannerSizes(imp)),
	)

	for index, creative := range campaign.Creatives {
		if creative == nil {
			logf(
				"[ADV][CREATIVE_REJECT] request_id=%q imp_id=%q campaign_id=%q creative_index=%d reason=creative_nil",
				requestID,
				impID,
				campaign.ID,
				index,
			)
			continue
		}
		if strings.TrimSpace(creative.ID) == "" {
			logf(
				"[ADV][CREATIVE_REJECT] request_id=%q imp_id=%q campaign_id=%q creative_index=%d reason=creative_id_empty",
				requestID,
				impID,
				campaign.ID,
				index,
			)
			continue
		}
		if strings.TrimSpace(creative.ADMURL) == "" {
			logf(
				"[ADV][CREATIVE_REJECT] request_id=%q imp_id=%q campaign_id=%q creative_id=%q reason=adm_url_empty",
				requestID,
				impID,
				campaign.ID,
				creative.ID,
			)
			continue
		}
		if normalizeFormat(format) == "BAN" {
			if creative.W <= 0 || creative.H <= 0 {
				logf(
					"[ADV][CREATIVE_REJECT] request_id=%q imp_id=%q campaign_id=%q creative_id=%q reason=creative_dimensions_invalid width=%d height=%d",
					requestID,
					impID,
					campaign.ID,
					creative.ID,
					creative.W,
					creative.H,
				)
				continue
			}
			if !bannerSizes(imp)[[2]int{creative.W, creative.H}] {
				logf(
					"[ADV][CREATIVE_REJECT] request_id=%q imp_id=%q campaign_id=%q creative_id=%q reason=banner_size_mismatch creative_width=%d creative_height=%d requested_banner_sizes=%q",
					requestID,
					impID,
					campaign.ID,
					creative.ID,
					creative.W,
					creative.H,
					formatBannerSizes(bannerSizes(imp)),
				)
			}
		}
	}
}

func formatBannerSizes(sizes map[[2]int]bool) string {
	if len(sizes) == 0 {
		return ""
	}
	items := make([]string, 0, len(sizes))
	for size := range sizes {
		items = append(items, fmt.Sprintf("%dx%d", size[0], size[1]))
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func bannerExtValues(imp *ortb.Imp) string {
	if imp == nil || imp.GetBanner() == nil {
		return ""
	}
	return strings.Join(imp.GetBanner().GetExt(), ",")
}

func (s *AuctionService) buildBid(req *ortb.BidRequest, imp *ortb.Imp, campaign *Campaign, creative *Creative, bidPrice float64) *ortb.Bid {
	if s == nil || imp == nil || campaign == nil || creative == nil || bidPrice <= 0 || math.IsNaN(bidPrice) || math.IsInf(bidPrice, 0) {
		return nil
	}
	originalADM := appendTrackerMacros(creative.ADMURL, creative.TrackersMacros, campaign.ID, creative.ID, req)
	if strings.TrimSpace(originalADM) == "" {
		return nil
	}
	id, impID, cid, crid := creative.ID, imp.GetId(), campaign.ID, creative.ID
	price := float32(bidPrice)
	w, h := int32(creative.W), int32(creative.H)
	return &ortb.Bid{Id: &id, Impid: &impID, Price: &price, Adm: &originalADM, Cid: &cid, Crid: &crid, W: &w, H: &h}
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

type requestFilterValues struct {
	country    *string
	language   *string
	deviceType *string
	osName     *string
	browser    *string
	siteID     *string
	ip         *string
}

func extractRequestFilterValues(req *ortb.BidRequest) requestFilterValues {
	values := requestFilterValues{}
	if req == nil {
		return values
	}
	if device := req.GetDevice(); device != nil {
		if geo := device.GetGeo(); geo != nil {
			values.country = nonEmptyStringPtr(geo.GetCountry())
		}
		values.language = nonEmptyStringPtr(normalizeLanguage(device.GetLanguage()))
		values.osName = nonEmptyStringPtr(device.GetOs())
		values.ip = nonEmptyStringPtr(device.GetIp())

		rawUA := strings.TrimSpace(device.GetUa())
		if rawUA != "" {
			parsed := ua.ParseUA(rawUA)
			values.deviceType = nonEmptyStringPtr(normalizeDeviceType(parsed.Device))
			values.browser = nonEmptyStringPtr(parsed.Browser)
			if values.osName == nil {
				values.osName = nonEmptyStringPtr(parsed.OS)
			}
		} else if device.DeviceType != nil {
			values.deviceType = nonEmptyStringPtr(normalizeDeviceType(strconv.Itoa(int(device.GetDeviceType()))))
		}
	}
	if site := req.GetSite(); site != nil {
		values.siteID = nonEmptyStringPtr(site.GetId())
	}
	return values
}

type campaignFilterCheck struct {
	name   string
	filter *filterV2.Filters
	value  *string
}

func campaignPassesFilters(c *Campaign, req *ortb.BidRequest) bool {
	return campaignPassesFiltersWithDebug(c, req, "", "", func(string, ...any) {})
}

func campaignPassesFiltersWithDebug(
	c *Campaign,
	req *ortb.BidRequest,
	requestID, impID string,
	logf debugLogFunc,
) bool {
	if c == nil {
		return false
	}
	values := extractRequestFilterValues(req)
	checks := []campaignFilterCheck{
		{name: "country", filter: c.CountryFilter, value: values.country},
		{name: "language", filter: c.LanguageFilter, value: values.language},
		{name: "device_type", filter: c.DeviceTypeFilter, value: values.deviceType},
		{name: "os", filter: c.OSFilter, value: values.osName},
		{name: "browser", filter: c.BrowserFilter, value: values.browser},
		{name: "site_id", filter: c.SiteIDFilter, value: values.siteID},
		{name: "ip", filter: c.IPFilter, value: values.ip},
	}

	allAllowed := true
	for _, check := range checks {
		if allowed(check.filter, check.value) {
			continue
		}
		allAllowed = false

		value := "<nil>"
		listed := false
		if check.value != nil {
			value = *check.value
			if check.filter != nil {
				listed = check.filter.Objects[value]
			}
		}
		mode := "disabled"
		configuredObjects := 0
		if check.filter != nil {
			configuredObjects = len(check.filter.Objects)
			if check.filter.IsWhiteList {
				mode = "whitelist"
			} else {
				mode = "blacklist"
			}
		}
		logf(
			"[ADV][FILTER_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q filter=%q value=%q mode=%q listed=%t configured_objects=%d",
			requestID,
			impID,
			c.ID,
			c.UserID,
			check.name,
			value,
			mode,
			listed,
			configuredObjects,
		)
	}

	if !allAllowed {
		logf(
			"[ADV][CAMPAIGN_REJECT] request_id=%q imp_id=%q campaign_id=%q user_id=%q reason=filter_rejected",
			requestID,
			impID,
			c.ID,
			c.UserID,
		)
	}
	return allAllowed
}

func allowed(filter *filterV2.Filters, value *string) bool {
	if filter == nil {
		return true
	}
	return filter.Allowed(value)
}

func normalizeLanguage(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeDeviceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "4", "mobile", "phone", "smartphone":
		return "mobile"
	case "5", "tablet":
		return "tablet"
	case "2", "desktop", "pc":
		return "desktop"
	case "bot":
		return "bot"
	case "3", "6", "7", "other", "connected_tv", "connected device", "set_top_box":
		return "other"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
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
