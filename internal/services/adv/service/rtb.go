package auction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/protobuf/proto"
)

const (
	rtbCodeNetworkError = "1"
	rtbCodeInvalidJSON  = "3"
	rtbCodeReadError    = "4"
	rtbCodeRequestError = "55"
	rtbCodeInvalidADM   = "800"
)

type rtbCampaignResult struct {
	campaignID string
	impIDs     []string
	bids       map[string]*ortb.Bid
	codes      map[string]string
}

func newSafeRTBHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          1024,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 0,
		ExpectContinueTimeout: time.Second,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, item := range ips {
			if unsafeRTBIP(item.IP) {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(item.IP.String(), port))
		}
		return nil, fmt.Errorf("RTB endpoint %q resolves only to private/reserved addresses", host)
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("RTB redirects are disabled")
		},
	}
}

var rtbBlockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func unsafeRTBIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	for _, prefix := range rtbBlockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func (s *AuctionService) SetStatsRedisClients(clients []*redis.Client) {
	if s != nil {
		s.statsRedisClients = clients
	}
}

func (s *AuctionService) fetchRTBCampaignBids(
	ctx context.Context,
	req *ortb.BidRequest,
	snapshot *Snapshot,
	now time.Time,
	options AuctionRequestOptions,
	requestedFormat, trafficType, sspDomain, siteIDQualityValue string,
	antiState *AntiPerekrutState,
	requestIsVPN bool,
	vpnClassificationErr error,
	logf debugLogFunc,
) map[string]map[string]*ortb.Bid {
	out := make(map[string]map[string]*ortb.Bid)
	if s == nil || req == nil || snapshot == nil || s.rtbHTTPClient == nil {
		return out
	}

	type job struct {
		campaign *Campaign
		imps     []*ortb.Imp
	}
	jobs := make([]job, 0)
	rtbCampaignsSeen := 0
	for _, campaign := range snapshot.Campaigns {
		if campaign == nil || !campaign.RTB {
			continue
		}
		rtbCampaignsSeen++
		log.Printf("[ADV][RTB_CAMPAIGN_SEEN] request_id=%q campaign_id=%q user_id=%q dsp_link=%q request_imps=%d", strings.TrimSpace(req.GetId()), campaign.ID, campaign.UserID, campaign.DSPLink, len(req.GetImp()))
		if strings.TrimSpace(campaign.DSPLink) == "" {
			log.Printf("[ADV][RTB_CAMPAIGN_SKIP] request_id=%q campaign_id=%q reason=dsp_link_empty", strings.TrimSpace(req.GetId()), campaign.ID)
			continue
		}
		eligible := make([]*ortb.Imp, 0, len(req.GetImp()))
		for _, imp := range req.GetImp() {
			if imp == nil || strings.TrimSpace(imp.GetId()) == "" {
				continue
			}
			durableUserBlocked := snapshot.UserAntiPerekrutBlocked[campaign.UserID]
			preflightOK, rejectReason := s.rtbPreflightEligible(campaign, req, imp, now, requestedFormat, trafficType, sspDomain, siteIDQualityValue,
				options.ImpIDUUID[imp.GetId()], antiState, durableUserBlocked, requestIsVPN, vpnClassificationErr, logf)
			if preflightOK {
				eligible = append(eligible, imp)
				log.Printf("[ADV][RTB_PREFLIGHT_PASS] request_id=%q campaign_id=%q imp_id=%q", strings.TrimSpace(req.GetId()), campaign.ID, imp.GetId())
			} else {
				log.Printf("[ADV][RTB_PREFLIGHT_REJECT] request_id=%q campaign_id=%q imp_id=%q reason=%q", strings.TrimSpace(req.GetId()), campaign.ID, imp.GetId(), rejectReason)
			}
		}
		if len(eligible) > 0 {
			jobs = append(jobs, job{campaign: campaign, imps: eligible})
			log.Printf("[ADV][RTB_JOB_QUEUED] request_id=%q campaign_id=%q eligible_imps=%d", strings.TrimSpace(req.GetId()), campaign.ID, len(eligible))
		}
	}
	log.Printf("[ADV][RTB_DISCOVERY] request_id=%q rtb_campaigns=%d jobs=%d", strings.TrimSpace(req.GetId()), rtbCampaignsSeen, len(jobs))
	if len(jobs) == 0 {
		s.writeRTBResponseStats(ctx, options.ImpIDUUID, nil, logf)
		return out
	}

	results := make(chan rtbCampaignResult, len(jobs))
	var wg sync.WaitGroup
	for _, item := range jobs {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.callRTBCampaign(ctx, req, item.campaign, item.imps, requestedFormat, logf)
		}()
	}
	wg.Wait()
	close(results)

	statsByImp := make(map[string]map[string]string)
	for result := range results {
		for impID, code := range result.codes {
			if statsByImp[impID] == nil {
				statsByImp[impID] = make(map[string]string)
			}
			statsByImp[impID][result.campaignID] = code
		}
		for impID, bid := range result.bids {
			if out[impID] == nil {
				out[impID] = make(map[string]*ortb.Bid)
			}
			out[impID][result.campaignID] = bid
		}
	}
	s.writeRTBResponseStats(ctx, options.ImpIDUUID, statsByImp, logf)
	return out
}

func (s *AuctionService) rtbPreflightEligible(
	campaign *Campaign,
	req *ortb.BidRequest,
	imp *ortb.Imp,
	now time.Time,
	requestedFormat, trafficType, sspDomain, siteIDQualityValue, hashFallback string,
	antiState *AntiPerekrutState,
	durableUserBlocked bool,
	requestIsVPN bool,
	vpnClassificationErr error,
	logf debugLogFunc,
) (bool, string) {
	if campaign == nil {
		return false, "campaign_nil"
	}
	if imp == nil {
		return false, "imp_nil"
	}
	if !campaign.RTB {
		return false, "campaign_not_rtb"
	}
	if normalizeFormat(campaign.Format) != requestedFormat {
		return false, "format_mismatch"
	}
	if !trafficMatches(campaign.TrafficType, trafficType) {
		return false, "traffic_type_mismatch"
	}
	if !campaignActiveAt(campaign, now) {
		return false, "campaign_inactive_at_request_time"
	}
	if campaign.BlockVPN && vpnClassificationErr != nil {
		return false, "vpn_classification_error"
	}
	if campaign.BlockVPN && requestIsVPN {
		return false, "vpn_blocked"
	}
	if s.antiperekrutEnabled {
		if s.antiperekrut == nil {
			return false, "antiperekrut_unavailable"
		}
		if !s.antiperekrut.CampaignAllowed(antiState, campaign) {
			return false, "antiperekrut_campaign_rejected"
		}
		hashID := strings.TrimSpace(req.GetId())
		if hashID == "" {
			hashID = hashFallback
		}
		if !trafficHashPass(hashID, campaign.ID, s.antiperekrut.EffectiveTrafficLimit(antiState, campaign, now)) {
			return false, "traffic_limit_hash_rejected"
		}
		_ = durableUserBlocked // durable state is already part of CampaignAllowed
	}
	if !s.quality.Contains(campaign.QualitySegment, sspDomain) {
		return false, "quality_segment_mismatch"
	}
	if !s.siteIDQuality.allowsNormalized(campaign.QualitySegment, siteIDQualityValue) {
		return false, "site_id_quality_mismatch"
	}
	if !campaignPassesFiltersWithDebug(campaign, req, strings.TrimSpace(req.GetId()), imp.GetId(), logf) {
		return false, "campaign_filters_rejected"
	}
	return true, ""
}

func (s *AuctionService) callRTBCampaign(ctx context.Context, source *ortb.BidRequest, campaign *Campaign, imps []*ortb.Imp, format string, logf debugLogFunc) rtbCampaignResult {
	result := rtbCampaignResult{campaignID: campaign.ID, bids: make(map[string]*ortb.Bid), codes: make(map[string]string)}
	for _, imp := range imps {
		if imp != nil && strings.TrimSpace(imp.GetId()) != "" {
			result.impIDs = append(result.impIDs, imp.GetId())
		}
	}
	setAll := func(code string) {
		for _, impID := range result.impIDs {
			result.codes[impID] = code
		}
	}

	u, err := url.Parse(strings.TrimSpace(campaign.DSPLink))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		setAll(rtbCodeRequestError)
		return result
	}
	cloned, ok := proto.Clone(source).(*ortb.BidRequest)
	if !ok || cloned == nil {
		setAll(rtbCodeRequestError)
		return result
	}
	allowed := make(map[string]struct{}, len(result.impIDs))
	for _, id := range result.impIDs {
		allowed[id] = struct{}{}
	}
	filtered := make([]*ortb.Imp, 0, len(result.impIDs))
	for _, imp := range cloned.GetImp() {
		if imp == nil {
			continue
		}
		if _, exists := allowed[imp.GetId()]; exists {
			filtered = append(filtered, imp)
		}
	}
	cloned.Imp = filtered
	stripInternalADVFormatMarkers(cloned)
	body, err := jsoniter.Marshal(cloned)
	if err != nil {
		setAll(rtbCodeRequestError)
		return result
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, campaign.DSPLink, strings.NewReader(string(body)))
	if err != nil {
		setAll(rtbCodeRequestError)
		return result
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Connection", "keep-alive")
	httpReq.Header.Set("X-Openrtb-Version", "2.5")
	startedAt := time.Now()
	log.Printf("[ADV][RTB_HTTP_ATTEMPT] request_id=%q campaign_id=%q url=%q imp_count=%d imp_ids=%v payload_bytes=%d", strings.TrimSpace(source.GetId()), campaign.ID, campaign.DSPLink, len(result.impIDs), result.impIDs, len(body))
	resp, err := s.rtbHTTPClient.Do(httpReq)
	if err != nil {
		log.Printf("[ADV][RTB_HTTP_ERROR] request_id=%q campaign_id=%q url=%q imp_ids=%v duration=%s error=%v", strings.TrimSpace(source.GetId()), campaign.ID, campaign.DSPLink, result.impIDs, time.Since(startedAt), err)
		setAll(rtbCodeNetworkError)
		return result
	}
	defer resp.Body.Close()
	log.Printf("[ADV][RTB_HTTP_RESULT] request_id=%q campaign_id=%q url=%q imp_ids=%v status=%d duration=%s", strings.TrimSpace(source.GetId()), campaign.ID, campaign.DSPLink, result.impIDs, resp.StatusCode, time.Since(startedAt))
	if resp.StatusCode != http.StatusOK {
		setAll(fmt.Sprintf("%d", resp.StatusCode))
		return result
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		setAll(rtbCodeReadError)
		return result
	}
	var bidResponse ortb.BidResponse
	if err := jsoniter.Unmarshal(payload, &bidResponse); err != nil {
		setAll(rtbCodeInvalidJSON)
		return result
	}

	bidsByImp := make(map[string][]*ortb.Bid)
	for _, seat := range bidResponse.GetSeatbid() {
		if seat == nil {
			continue
		}
		for _, bid := range seat.GetBid() {
			if bid == nil {
				continue
			}
			if _, exists := allowed[bid.GetImpid()]; !exists {
				continue
			}
			bidsByImp[bid.GetImpid()] = append(bidsByImp[bid.GetImpid()], bid)
		}
	}
	for _, impID := range result.impIDs {
		result.codes[impID] = fmt.Sprintf("%d", http.StatusOK)
		items := bidsByImp[impID]
		if len(items) == 0 {
			continue // successful no-bid
		}
		var best *ortb.Bid
		invalidADMSeen := false
		for _, bid := range items {
			if !validRTBADM(format, bid.GetAdm()) {
				invalidADMSeen = true
				continue
			}
			price := bid.GetPrice()
			if price <= 0 || math.IsNaN(float64(price)) || math.IsInf(float64(price), 0) {
				continue
			}
			if best == nil || price > best.GetPrice() || (price == best.GetPrice() && bid.GetId() < best.GetId()) {
				best = bid
			}
		}
		if best != nil {
			result.bids[impID] = best
			log.Printf("[ADV][RTB_BID_SELECTED] request_id=%q campaign_id=%q imp_id=%q bid_id=%q price=%.12f total_bids_for_imp=%d", strings.TrimSpace(source.GetId()), campaign.ID, impID, best.GetId(), float64(best.GetPrice()), len(items))
			continue
		}
		if invalidADMSeen {
			result.codes[impID] = rtbCodeInvalidADM
			log.Printf("[ADV][RTB_BID_REJECT] request_id=%q campaign_id=%q imp_id=%q reason=no_valid_bid invalid_adm_seen=true bids=%d", strings.TrimSpace(source.GetId()), campaign.ID, impID, len(items))
		} else if len(items) > 0 {
			log.Printf("[ADV][RTB_BID_REJECT] request_id=%q campaign_id=%q imp_id=%q reason=no_valid_positive_price bids=%d", strings.TrimSpace(source.GetId()), campaign.ID, impID, len(items))
		} else {
			log.Printf("[ADV][RTB_NO_BID] request_id=%q campaign_id=%q imp_id=%q", strings.TrimSpace(source.GetId()), campaign.ID, impID)
		}
	}
	return result
}

func validRTBADM(format, adm string) bool {
	if strings.TrimSpace(adm) == "" {
		return false
	}
	switch normalizeFormat(format) {
	case constants.NAT, constants.IPP:
		return json.Valid([]byte(adm))
	default:
		return true
	}
}

func (s *AuctionService) evaluateRTBCandidate(ctx context.Context, campaign *Campaign, bid *ortb.Bid, imp *ortb.Imp, now time.Time, requestedFormat string) (candidate, bool, error) {
	if campaign == nil || bid == nil || imp == nil {
		return candidate{}, false, nil
	}
	rawPrice := float64(bid.GetPrice())
	if rawPrice <= 0 || math.IsNaN(rawPrice) || math.IsInf(rawPrice, 0) {
		return candidate{}, false, nil
	}
	chargePrice := CalculateChargePrice(rawPrice, campaign.PricingModel, requestedFormat)
	if !finitePositive(chargePrice) {
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
		clone := *campaign
		clone.BasePrice = rawPrice
		eligible, _, err := s.runtime.PacingEligibility(ctx, &clone, now, campaignSpent)
		if err != nil || !eligible {
			return candidate{}, false, err
		}
	}
	deduction := s.percents.Lookup(campaign.UserID)
	effective := CalculateEffectiveAuctionPrice(rawPrice, deduction)
	if !finitePositive(effective) {
		return candidate{}, false, nil
	}
	return candidate{campaign: campaign, chargePrice: chargePrice, effectivePrice: effective, basePrice: rawPrice, externalBid: bid}, true, nil
}

func buildExternalADVBid(cand candidate) *ortb.Bid {
	if cand.externalBid == nil || cand.campaign == nil || !finitePositive(cand.effectivePrice) {
		return nil
	}
	bid, ok := proto.Clone(cand.externalBid).(*ortb.Bid)
	if !ok || bid == nil {
		return nil
	}
	price := float32(cand.effectivePrice)
	cid := cand.campaign.ID
	marker := constants.ExternalADVBidMarker
	bid.Price = &price
	bid.Cid = &cid
	if bid.Ext == nil {
		bid.Ext = &ortb.BidExt{}
	}
	// Cwin is an internal transport marker only. BidEngine replaces it with the
	// real clicks_wins callback before the bid leaves the exchange, so the
	// external bidder's Adid/Crid remain intact.
	bid.Ext.Cwin = &marker
	// Downstream BURL follows the same semantics as ordinary DSP: it is never
	// proxied. BidEngine synthesizes the exchange BURL when required.
	bid.Burl = nil
	return bid
}

func stripInternalADVFormatMarkers(req *ortb.BidRequest) {
	if req == nil {
		return
	}
	for _, imp := range req.GetImp() {
		if imp == nil || imp.GetBanner() == nil || len(imp.GetBanner().GetExt()) == 0 {
			continue
		}
		ext := imp.Banner.GetExt()[:0]
		for _, value := range imp.Banner.GetExt() {
			if strings.HasPrefix(strings.TrimSpace(value), constants.ADVImpressionFormatMarkerPrefix) {
				continue
			}
			ext = append(ext, value)
		}
		imp.Banner.Ext = ext
	}
}

func (s *AuctionService) writeRTBResponseStats(_ context.Context, impUUID map[string]string, stats map[string]map[string]string, logf debugLogFunc) {
	if s == nil || len(s.statsRedisClients) == 0 {
		return
	}
	for impID, rawUUID := range impUUID {
		uuid := strings.TrimSpace(rawUUID)
		if uuid == "" {
			continue
		}
		items := stats[impID]
		if items == nil {
			items = map[string]string{}
		}
		payload, err := proto.Marshal(&eventspb.BidResponses{Items: items})
		if err != nil {
			log.Printf("[ADV][RTB_STATS_ERROR] imp_id=%q error=%v", impID, err)
			continue
		}

		// Use the same Redis writer contract as the other ORTB statistics writers:
		// HSET creates the UUID hash if it does not exist yet, and the SSP adapter
		// can safely add the remaining fields before publishing the UUID as ready.
		log.Printf("[ADV][RTB_STATS_WRITE_ATTEMPT] imp_id=%q uuid=%q items=%v", impID, uuid, items)

		writeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		err = utils.WriteBytesToRedis(
			writeCtx,
			s.statsRedisClients,
			uuid,
			constants.ADV_RTB_RESPONSES_COLUMN,
			payload,
			true,
		)
		cancel()
		if err != nil {
			log.Printf("[ADV][RTB_STATS_ERROR] imp_id=%q uuid=%q error=%v", impID, uuid, err)
			continue
		}

		log.Printf("[ADV][RTB_STATS_WRITE_OK] imp_id=%q uuid=%q items=%v", impID, uuid, items)
	}
}
