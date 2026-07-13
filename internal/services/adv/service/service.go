package auction

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
)

const SlotDuration = 5 * time.Minute
const CampaignStatusActive = "active"

type TimeRange struct{ Start, End time.Time }
type Creative struct {
	ID, CampaignID, ADMURL, Name, CreativeName, Title, Description string
	TrackersMacros                                                 map[string]bool
	W, H                                                           int
}
type TrackerMacroValues struct{ Device, Browser, DeviceOS string }

type Campaign struct {
	ID, UserID, Status, PricingModel, Format, CampaignName, BrandName, TrafficType, QualitySegment, DSPURL string
	H, W                                                                                                   int
	Vertical                                                                                               json.RawMessage
	BasePrice, PlatformFeePercent, GoalTotalDollars                                                        float64
	EvennessBySlotMode                                                                                     bool
	StartTS, EndTS                                                                                         time.Time
	CountryFilter, LanguageFilter, DeviceTypeFilter, OSFilter, BrowserFilter, SiteIDFilter, IPFilter       *filterV2.Filters
	ActiveIntervals                                                                                        []TimeRange
	Creatives                                                                                              map[string]*Creative
}

func (c *Campaign) IsValid() bool {
	return c != nil && c.ID != "" && c.UserID != "" && strings.EqualFold(c.Status, CampaignStatusActive) && c.GoalTotalDollars > 0 && c.BasePrice >= 0 && !c.StartTS.IsZero() && !c.EndTS.IsZero() && c.StartTS.Before(c.EndTS)
}
func (c *Campaign) Clone() *Campaign {
	if c == nil {
		return nil
	}
	out := *c
	out.Vertical = append(json.RawMessage(nil), c.Vertical...)
	out.ActiveIntervals = append([]TimeRange(nil), c.ActiveIntervals...)
	out.Creatives = make(map[string]*Creative, len(c.Creatives))
	for id, cr := range c.Creatives {
		if cr != nil {
			cc := *cr
			cc.TrackersMacros = cloneBoolMap(cr.TrackersMacros)
			out.Creatives[id] = &cc
		}
	}
	return &out
}
func cloneBoolMap(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func (c *Campaign) ChargePriceForFormat(format string) float64 {
	if strings.TrimSpace(format) == "" {
		format = c.Format
	}
	return ChargePrice(c.BasePrice, c.PricingModel, format)
}
func (c *Campaign) GetAuctionPriceForFormat(format string) float64 {
	if strings.TrimSpace(format) == "" {
		format = c.Format
	}
	return EffectivePrice(c.BasePrice, c.PricingModel, format, c.PlatformFeePercent)
}
func (c *Campaign) IsActiveInIntervals(now time.Time) bool {
	if len(c.ActiveIntervals) == 0 {
		return true
	}
	for _, in := range c.ActiveIntervals {
		if (now.Equal(in.Start) || now.After(in.Start)) && now.Before(in.End) {
			return true
		}
	}
	return false
}
func (c *Campaign) IsActiveGlobal(now time.Time) bool {
	return c != nil && strings.EqualFold(c.Status, CampaignStatusActive) && !now.Before(c.StartTS) && now.Before(c.EndTS) && c.IsActiveInIntervals(now)
}
func (c *Campaign) RandomCreativeForSize(w, h int) *Creative {
	list := make([]*Creative, 0, len(c.Creatives))
	for _, cr := range c.Creatives {
		if cr == nil {
			continue
		}
		if w > 0 && h > 0 && (cr.W != w || cr.H != h) {
			continue
		}
		list = append(list, cr)
	}
	if len(list) == 0 {
		return nil
	}
	return list[rand.Intn(len(list))]
}

var weekdayIndex = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

func ParseActiveIntervalSchedule(schedule [][]string, windowStart, windowEnd time.Time) ([]TimeRange, error) {
	if len(schedule) == 0 || !windowStart.Before(windowEnd) {
		return nil, nil
	}
	weekStart := startOfWeek(windowStart.UTC())
	expandedUntil := windowEnd.UTC().Add(7 * 24 * time.Hour)
	var intervals []TimeRange
	for _, pair := range schedule {
		if len(pair) != 2 {
			return nil, fmt.Errorf("invalid schedule interval %v", pair)
		}
		so, err := parseWeekOffset(pair[0])
		if err != nil {
			return nil, err
		}
		eo, err := parseWeekOffset(pair[1])
		if err != nil {
			return nil, err
		}
		if eo < so {
			eo += 7 * 24 * time.Hour
		}
		eo += time.Hour
		for base := weekStart; base.Before(expandedUntil); base = base.Add(7 * 24 * time.Hour) {
			st := maxTime(base.Add(so), windowStart.UTC())
			en := minTime(base.Add(eo), windowEnd.UTC())
			if st.Before(en) {
				intervals = append(intervals, TimeRange{st, en})
			}
		}
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].Start.Before(intervals[j].Start) })
	return intervals, nil
}
func parseWeekOffset(value string) (time.Duration, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid schedule point %q", value)
	}
	day, ok := weekdayIndex[strings.ToLower(strings.TrimSpace(parts[0]))]
	if !ok {
		return 0, fmt.Errorf("invalid weekday %q", parts[0])
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || hour < 0 || hour > 23 {
		return 0, fmt.Errorf("invalid hour %q", parts[1])
	}
	return time.Duration(day*24+hour) * time.Hour, nil
}
func startOfWeek(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.Add(-time.Duration(d.Weekday()) * 24 * time.Hour)
}

const getCampaignsQuery = `SELECT user_id,device_type,os,browser,site_id,ip,campaign_id,h,w,vertical,base_price,goal_total_dollars,evenness_by_slot_mode,start_ts,end_ts,active_intervals,country,language,campaign_name,format_type,brand_name,quality_type,pricing_model,status,traffic_type FROM campaigns WHERE status = 'active'`
const getCreativesByCampaignQuery = `SELECT id,campaign_id,trackers_macros,w,h,name,creative_name,link,title,description FROM creatives WHERE campaign_id = $1`
const getUserGoalsQuery = `SELECT id, goal FROM users WHERE id=ANY($1)`

type campaignRow struct {
	UserID, CampaignID, BasePrice, GoalTotalDollars, CampaignName, FormatType, BrandName, QualityType, PricingModel, Status, TrafficType sql.NullString
	DeviceType, OS, Browser, SiteID, IP, Vertical, ActiveIntervals, Country, Language                                                    []byte
	H, W                                                                                                                                 sql.NullInt64
	EvennessBySlotMode                                                                                                                   sql.NullBool
	StartTS, EndTS                                                                                                                       sql.NullTime
}
type creativeRow struct {
	ID, CampaignID, Name, CreativeName, ADMURL, Title, Description sql.NullString
	TrackersMacros                                                 []byte
	W, H                                                           sql.NullInt64
}

func GetCampaignsFromPostgres(ctx context.Context, db *sql.DB) (map[string]*Campaign, error) {
	campaigns, users, err := GetCampaignsAndUserGoalsFromPostgres(ctx, db)
	_ = users
	return campaigns, err
}
func GetCampaignsAndUserGoalsFromPostgres(ctx context.Context, db *sql.DB) (map[string]*Campaign, map[string]float64, error) {
	if db == nil {
		return nil, nil, fmt.Errorf("postgres db is nil")
	}
	rows, err := db.QueryContext(ctx, getCampaignsQuery)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	campaigns := map[string]*Campaign{}
	userSet := map[string]struct{}{}
	for rows.Next() {
		var r campaignRow
		if err := rows.Scan(&r.UserID, &r.DeviceType, &r.OS, &r.Browser, &r.SiteID, &r.IP, &r.CampaignID, &r.H, &r.W, &r.Vertical, &r.BasePrice, &r.GoalTotalDollars, &r.EvennessBySlotMode, &r.StartTS, &r.EndTS, &r.ActiveIntervals, &r.Country, &r.Language, &r.CampaignName, &r.FormatType, &r.BrandName, &r.QualityType, &r.PricingModel, &r.Status, &r.TrafficType); err != nil {
			return nil, nil, err
		}
		c, err := r.toCampaign()
		if err != nil {
			continue
		}
		if c.IsValid() {
			campaigns[c.ID] = c
			userSet[c.UserID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for id, c := range campaigns {
		cr, err := getCreativesForCampaignFromPostgres(ctx, db, id)
		if err != nil {
			return nil, nil, err
		}
		c.Creatives = cr
	}
	goals := map[string]float64{}
	if len(userSet) > 0 {
		ids := make([]string, 0, len(userSet))
		for id := range userSet {
			ids = append(ids, id)
		}
		gr, err := db.QueryContext(ctx, getUserGoalsQuery, pq.Array(ids))
		if err != nil {
			return nil, nil, err
		}
		defer gr.Close()
		for gr.Next() {
			var id string
			var goal float64
			if err := gr.Scan(&id, &goal); err != nil {
				return nil, nil, err
			}
			if validUserGoal(id, goal) {
				goals[strings.TrimSpace(id)] = goal
			}
		}
		if err := gr.Err(); err != nil {
			return nil, nil, err
		}
	}
	pruneCampaignsWithoutUserGoals(campaigns, goals)
	return campaigns, goals, nil
}

func pruneCampaignsWithoutUserGoals(campaigns map[string]*Campaign, goals map[string]float64) {
	for id, c := range campaigns {
		if c == nil {
			delete(campaigns, id)
			continue
		}
		if _, ok := goals[c.UserID]; !ok {
			delete(campaigns, id)
		}
	}
}

func validUserGoal(userID string, goal float64) bool {
	return strings.TrimSpace(userID) != "" && !math.IsNaN(goal) && !math.IsInf(goal, 0) && goal > 0
}

func (r campaignRow) toCampaign() (*Campaign, error) {
	id := nullableString(r.CampaignID)
	if id == "" {
		return nil, fmt.Errorf("campaign_id is empty")
	}
	base, err := parseNullFloat64(r.BasePrice)
	if err != nil {
		return nil, err
	}
	goal, err := parseNullFloat64(r.GoalTotalDollars)
	if err != nil {
		return nil, err
	}
	intervals, err := parseActiveIntervalsJSONB(r.ActiveIntervals, r.StartTS.Time, r.EndTS.Time)
	if err != nil {
		return nil, err
	}
	return &Campaign{ID: id, UserID: nullableString(r.UserID), Status: nullableString(r.Status), PricingModel: nullableString(r.PricingModel), Format: normalizedAuctionFormat(nullableString(r.FormatType)), CampaignName: nullableString(r.CampaignName), BrandName: nullableString(r.BrandName), QualitySegment: strings.ToLower(nullableString(r.QualityType)), TrafficType: nullableString(r.TrafficType), H: int(r.H.Int64), W: int(r.W.Int64), Vertical: append(json.RawMessage(nil), r.Vertical...), BasePrice: base, GoalTotalDollars: goal, EvennessBySlotMode: r.EvennessBySlotMode.Valid && r.EvennessBySlotMode.Bool, StartTS: r.StartTS.Time, EndTS: r.EndTS.Time, CountryFilter: mustFilter(r.Country), LanguageFilter: mustFilter(r.Language), DeviceTypeFilter: mustFilter(r.DeviceType), OSFilter: mustFilter(r.OS), BrowserFilter: mustFilter(r.Browser), SiteIDFilter: mustFilter(r.SiteID), IPFilter: mustFilter(r.IP), ActiveIntervals: intervals, Creatives: map[string]*Creative{}}, nil
}
func mustFilter(raw []byte) *filterV2.Filters { f, _ := filterV2.GetFiltersFromJSONB(raw); return f }
func nullableString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}
func parseNullFloat64(v sql.NullString) (float64, error) {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return 0, nil
	}
	return strconv.ParseFloat(v.String, 64)
}
func parseActiveIntervalsJSONB(raw []byte, start, end time.Time) ([]TimeRange, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return nil, nil
	}
	var sched [][]string
	if err := json.Unmarshal(raw, &sched); err == nil {
		return ParseActiveIntervalSchedule(sched, start, end)
	}
	var ranges []TimeRange
	if err := json.Unmarshal(raw, &ranges); err != nil {
		return nil, err
	}
	return ranges, nil
}
func getCreativesForCampaignFromPostgres(ctx context.Context, db *sql.DB, campaignID string) (map[string]*Creative, error) {
	rows, err := db.QueryContext(ctx, getCreativesByCampaignQuery, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*Creative{}
	for rows.Next() {
		var r creativeRow
		if err := rows.Scan(&r.ID, &r.CampaignID, &r.TrackersMacros, &r.W, &r.H, &r.Name, &r.CreativeName, &r.ADMURL, &r.Title, &r.Description); err != nil {
			return nil, err
		}
		cr, err := r.toCreative()
		if err != nil {
			return nil, err
		}
		out[cr.ID] = cr
	}
	return out, rows.Err()
}
func (r creativeRow) toCreative() (*Creative, error) {
	id := nullableString(r.ID)
	if id == "" {
		return nil, fmt.Errorf("creative id empty")
	}
	macros, _ := parseTrackersMacrosJSONB(r.TrackersMacros)
	return &Creative{ID: id, CampaignID: nullableString(r.CampaignID), TrackersMacros: macros, W: int(r.W.Int64), H: int(r.H.Int64), Name: nullableString(r.Name), CreativeName: nullableString(r.CreativeName), ADMURL: nullableString(r.ADMURL), Title: nullableString(r.Title), Description: nullableString(r.Description)}, nil
}
func parseTrackersMacrosJSONB(raw []byte) (map[string]bool, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil, nil
	}
	var m map[string]bool
	return m, json.Unmarshal(raw, &m)
}

type AuctionService struct {
	snapshots       *snapshotStore
	filterProcessor *filter.OptimizedFilterProcessor
	quality         *QualityStore
	percents        *PercentStore
	runtime         *RuntimeStore
}

func NewAuctionService(fp *filter.OptimizedFilterProcessor) *AuctionService {
	q := NewQualityStore()
	_ = q.Replace(map[string][]string{})
	return &AuctionService{snapshots: newSnapshotStore(), filterProcessor: fp, quality: q, percents: NewPercentStore("", ""), runtime: NewRuntimeStore(nil, 0)}
}
func (s *AuctionService) SetRuntimeRedis(client *redis.Client, ttl time.Duration) {
	s.runtime = NewRuntimeStore(client, ttl)
}
func (s *AuctionService) SetQualityStore(q *QualityStore) {
	if q != nil {
		s.quality = q
	}
}
func (s *AuctionService) SetPercentStore(p *PercentStore) {
	if p != nil {
		s.percents = p
	}
}
func (s *AuctionService) ReplaceCampaigns(campaigns map[string]*Campaign) {
	s.snapshots.Store(BuildSnapshot(campaigns, map[string]float64{}))
}
func (s *AuctionService) ReplaceSnapshot(campaigns map[string]*Campaign, userGoals map[string]float64) {
	s.snapshots.Store(BuildSnapshot(campaigns, userGoals))
}
func (s *AuctionService) AddCampaign(c *Campaign) {
	snap := s.snapshots.Load()
	nextC := make(map[string]*Campaign, len(snap.Campaigns)+1)
	for k, v := range snap.Campaigns {
		nextC[k] = v
	}
	nextG := make(map[string]float64, len(snap.UserGoals))
	for k, v := range snap.UserGoals {
		nextG[k] = v
	}
	if c != nil {
		nextC[c.ID] = c
		if c.GoalTotalDollars > 0 {
			nextG[c.UserID] = c.GoalTotalDollars
		}
	}
	s.ReplaceSnapshot(nextC, nextG)
}
func (s *AuctionService) GetCampaign(id string) *Campaign { return s.snapshots.Load().Campaigns[id] }
func (s *AuctionService) SetCampaignCreatives(campaignID string, creatives map[string]*Creative) {
	snap := s.snapshots.Load()
	campaigns := map[string]*Campaign{}
	for id, c := range snap.Campaigns {
		campaigns[id] = c.Clone()
	}
	if c := campaigns[campaignID]; c != nil {
		c.Creatives = creatives
	}
	s.ReplaceSnapshot(campaigns, snap.UserGoals)
}
func (s *AuctionService) StartPostgresRefreshTicker(ctx context.Context, db *sql.DB, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	refresh := func() {
		c, g, err := GetCampaignsAndUserGoalsFromPostgres(ctx, db)
		if err != nil {
			log.Printf("[Auction] refresh failed: %v", err)
			return
		}
		s.ReplaceSnapshot(c, g)
	}
	refresh()
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refresh()
			}
		}
	}()
}

type AuctionRequestOptions struct{ Format, TrafficType, SSPDomain string }
type AuctionResult struct {
	Campaign     *Campaign
	Creative     *Creative
	AuctionPrice float64
	ADM          string
}

func (s *AuctionService) SetPercentMaps(a, m map[string]map[string]map[string]*types.PercentAndBidfloor) {
	_ = s.percents.Update("ADULT", PercentMap(a))
	_ = s.percents.Update("MAINSTREAM", PercentMap(m))
}
func (s *AuctionService) SelectAuction(req *ortb_V2_5.BidRequest, now time.Time, options AuctionRequestOptions) *AuctionResult {
	return s.SelectAuctionWithContext(context.Background(), req, now, options)
}

func (s *AuctionService) SelectAuctionWithContext(ctx context.Context, req *ortb_V2_5.BidRequest, now time.Time, options AuctionRequestOptions) *AuctionResult {
	cs := s.SelectAuctionCandidates(ctx, req, now, options)
	if len(cs) == 0 {
		return nil
	}
	return cs[0]
}
func (s *AuctionService) SelectAuctionCandidates(ctx context.Context, req *ortb_V2_5.BidRequest, now time.Time, options AuctionRequestOptions) []*AuctionResult {
	if req == nil {
		return nil
	}
	qseg := s.quality.Segment(options.SSPDomain)
	if qseg == "" {
		return nil
	}
	uaValues := ua.ParseUA("")
	if req.GetDevice() != nil {
		uaValues = ua.ParseUA(req.GetDevice().GetUa())
	}
	macroValues := TrackerMacroValues{Device: uaValues.Device, Browser: uaValues.Browser, DeviceOS: uaValues.OS}
	snap := s.snapshots.Load()
	out := []*AuctionResult{}
	for _, imp := range req.GetImp() {
		if imp == nil {
			continue
		}
		format := impressionFormat(imp, options.Format)
		bw, bh := bannerSize(imp)
		for _, c := range snap.Campaigns {
			if !s.eligible(ctx, snap, c, req, imp, now, options, format, qseg) {
				continue
			}
			creative := c.RandomCreativeForSize(bw, bh)
			if creative == nil {
				continue
			}
			effective := c.GetAuctionPriceForFormat(format) * s.percents.Lookup(options.TrafficType, options.SSPDomain, bidRequestCountry(req), c.UserID)
			if effective <= 0 {
				continue
			}
			adm := appendTrackerMacrosToADMURL(creative.ADMURL, creative.TrackersMacros, c.ID, creative.ID, req, macroValues)
			out = append(out, &AuctionResult{Campaign: c, Creative: creative, AuctionPrice: effective, ADM: adm})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AuctionPrice > out[j].AuctionPrice })
	return out
}
func (s *AuctionService) eligible(ctx context.Context, snap *Snapshot, c *Campaign, req *ortb_V2_5.BidRequest, imp *ortb_V2_5.Imp, now time.Time, opt AuctionRequestOptions, format, qseg string) bool {
	if c == nil || !c.IsActiveGlobal(now) || !strings.EqualFold(c.QualitySegment, qseg) {
		return false
	}
	if opt.TrafficType != "" && !strings.EqualFold(c.TrafficType, opt.TrafficType) {
		return false
	}
	if format != "" && c.Format != "" && c.Format != format {
		return false
	}
	if !s.passesFilters(c, req) {
		return false
	}
	charge := c.ChargePriceForFormat(format)
	if charge <= 0 {
		return false
	}
	userGoal := snap.UserGoals[c.UserID]
	if userGoal == 0 {
		userGoal = c.GoalTotalDollars
	}
	us, err := s.runtime.UserSpent(ctx, c.UserID)
	if err != nil {
		return false
	}
	cs, err := s.runtime.CampaignSpent(ctx, c.ID)
	if err != nil {
		return false
	}
	if userGoal-us < charge || c.GoalTotalDollars-cs < charge {
		return false
	}
	if c.GoalTotalDollars > 0 && cs/c.GoalTotalDollars > 0.95 {
		return false
	}
	ok, err := PacingEligible(ctx, s.runtime, c, now, cs)
	return err == nil && ok
}
func impressionFormat(imp *ortb_V2_5.Imp, fallback string) string {
	if imp != nil && imp.GetBanner() != nil {
		return "ban"
	}
	if imp != nil && imp.GetNative() != nil {
		return "nat"
	}
	return normalizedAuctionFormat(fallback)
}
func bannerSize(imp *ortb_V2_5.Imp) (int, int) {
	if imp == nil || imp.GetBanner() == nil {
		return 0, 0
	}
	return int(imp.GetBanner().GetW()), int(imp.GetBanner().GetH())
}
func bidRequestCountry(req *ortb_V2_5.BidRequest) string {
	if req != nil && req.GetDevice() != nil && req.GetDevice().GetGeo() != nil {
		return req.GetDevice().GetGeo().GetCountry()
	}
	return ""
}
func (s *AuctionService) passesFilters(c *Campaign, req *ortb_V2_5.BidRequest) bool {
	if s.filterProcessor == nil || c.DSPURL == "" {
		return true
	}
	return s.filterProcessor.ProcessRequestForDSPV25(c.DSPURL, req).Allowed
}
func (s *AuctionService) SlotTick(now time.Time) {
	ctx := context.Background()
	snap := s.snapshots.Load()
	for _, c := range snap.Campaigns {
		if c.IsActiveGlobal(now) {
			_ = s.runtime.SetCurrentSlot(ctx, c.ID, SlotID(now))
		} else {
			_ = s.runtime.ClearCurrentSlot(ctx, c.ID)
		}
	}
}

func (s *AuctionService) StartPacingTicker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = SlotDuration
	}
	s.SlotTick(time.Now())
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				s.SlotTick(now)
			}
		}
	}()
}

func (s *AuctionService) GetActiveCampaignsCount(now time.Time) int {
	n := 0
	for _, c := range s.snapshots.Load().Campaigns {
		if c.IsActiveGlobal(now) {
			n++
		}
	}
	return n
}

func appendTrackerMacrosToADMURL(admURL string, macros map[string]bool, campaignID, creativeID string, req *ortb_V2_5.BidRequest, values TrackerMacroValues) string {
	if len(macros) == 0 {
		return admURL
	}
	u, err := url.Parse(admURL)
	if err != nil {
		return admURL
	}
	q := u.Query()
	for key, en := range macros {
		if en {
			q.Set(key, trackerMacroValue(key, campaignID, creativeID, req, values))
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func trackerMacroValue(key, campaignID, creativeID string, req *ortb_V2_5.BidRequest, values TrackerMacroValues) string {
	switch key {
	case "campaign_id":
		return campaignID
	case "creative_id":
		return creativeID
	case "country_code":
		if req != nil && req.GetDevice() != nil && req.GetDevice().GetGeo() != nil {
			return req.GetDevice().GetGeo().GetCountry()
		}
	case "ip_address":
		if req != nil && req.GetDevice() != nil {
			return req.GetDevice().GetIp()
		}
	case "device_os":
		if values.DeviceOS != "" {
			return values.DeviceOS
		}
		if req != nil && req.GetDevice() != nil {
			return req.GetDevice().GetOs()
		}
	case "site_id":
		if req != nil && req.GetSite() != nil {
			return req.GetSite().GetId()
		}
	case "browser":
		return values.Browser
	case "device":
		return values.Device
	case "click_id":
		if req != nil {
			return req.GetId()
		}
	}
	return "unknown"
}
func GetSSPDomainsForQualitySegment(segment string) []string {
	return NewQualityStore().Domains(segment)
}
func QualitySegmentBySSPDomain(domain string) string { return "" }
