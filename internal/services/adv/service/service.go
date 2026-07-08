package auction

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// SlotDuration задаёт длительность одного временного слота (5 минут)
const SlotDuration = 5 * time.Minute

const (
	CampaignStatusActive       = "active"
	CampaignStatusPaused       = "paused"
	CampaignStatusDraft        = "draft"
	CampaignStatusModeration   = "moderation"
	CampaignStatusCompleted    = "completed"
	CampaignStatusNoBudget     = "no_budget"
	CampaignStatusPendingStart = "pending_start"

	PricingModelCPM = "CPM"
	PricingModelCPC = "CPC"

	EventTypeImpression = "impression"
	EventTypeClick      = "click"
)

// QualitySegment определяет сегмент качества трафика.
type QualitySegment string

const (
	QualitySegmentUsual QualitySegment = "usual"
	QualitySegmentHigh  QualitySegment = "high"
	QualitySegmentUltra QualitySegment = "ultra"
)

// qualitySegmentSSPMap хранит SSP-домены по сегментам качества.
// Домены являются локальной бизнес-настройкой; при наличии внешнего хранилища
// этот маппинг должен загружаться из конфигурации/БД.
var qualitySegmentSSPMap = map[QualitySegment][]string{
	QualitySegmentUsual: {"usual.ssp.local"},
	QualitySegmentHigh:  {"high.ssp.local"},
	QualitySegmentUltra: {"ultra.ssp.local"},
}

// TimeRange представляет интервал времени с точными датой и временем (UTC)
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Creative хранит минимальные данные креатива, которые нужны аукциону для ADM-ссылки.
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

// TrackerMacroValues содержит значения макросов, которых нет или пока может не быть в BidRequest.
type TrackerMacroValues struct {
	Device   string
	Browser  string
	DeviceOS string
}

var weekdayIndex = map[string]int{
	"sun": 0,
	"mon": 1,
	"tue": 2,
	"wed": 3,
	"thu": 4,
	"fri": 5,
	"sat": 6,
}

// ParseActiveIntervalSchedule разворачивает недельное расписание из БД вида:
// [["mon,4", "mon,4"], ["mon,6", "tue,10"], ["tue,18", "sun,23"]]
// в конкретные UTC TimeRange внутри окна кампании. Точки расписания задают
// включительные часы, поэтому End в TimeRange выставляется на следующий час
// после конечной точки и остаётся эксклюзивным.
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
		startOffset, err := parseWeekOffset(pair[0])
		if err != nil {
			return nil, err
		}
		endOffset, err := parseWeekOffset(pair[1])
		if err != nil {
			return nil, err
		}
		if endOffset < startOffset {
			endOffset += 7 * 24 * time.Hour
		}
		endExclusiveOffset := endOffset + time.Hour

		for base := weekStart; base.Before(expandedUntil); base = base.Add(7 * 24 * time.Hour) {
			start := base.Add(startOffset)
			end := base.Add(endExclusiveOffset)
			clippedStart := maxTime(start, windowStart.UTC())
			clippedEnd := minTime(end, windowEnd.UTC())
			if clippedStart.Before(clippedEnd) {
				intervals = append(intervals, TimeRange{Start: clippedStart, End: clippedEnd})
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
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return dayStart.Add(-time.Duration(dayStart.Weekday()) * 24 * time.Hour)
}

const getCampaignsQuery = `
	SELECT
		user_id,
		device_type,
		os,
		browser,
		site_id,
		ip,
		campaign_id,
		h,
		w,
		vertical,
		base_price,
		evenness_by_slot_mode,
		start_ts,
		end_ts,
		active_intervals,
		country,
		language,
		campaign_name,
		format_type,
		brand_name,
		quality_type,
		pricing_model,
		status,
		traffic_type
	FROM campaigns
`

const getCreativesByCampaignQuery = `
	SELECT
		id,
		campaign_id,
		trackers_macros,
		w,
		h,
		name,
		creative_name,
		link,
		title,
		description
	FROM creatives
	WHERE campaign_id = $1
`

// GetCampaignsFromPostgres загружает все кампании из Postgres и возвращает мапу
// campaign_id -> Campaign. Для каждой кампании дополнительно загружаются креативы.
func GetCampaignsFromPostgres(ctx context.Context, db *sql.DB) (map[string]*Campaign, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres db is nil")
	}

	rows, err := db.QueryContext(ctx, getCampaignsQuery)
	if err != nil {
		return nil, fmt.Errorf("query campaigns: %w", err)
	}
	defer rows.Close()

	campaigns := make(map[string]*Campaign)
	for rows.Next() {
		var row campaignRow
		if err := rows.Scan(
			&row.UserID, &row.DeviceType, &row.OS, &row.Browser, &row.SiteID, &row.IP,
			&row.CampaignID, &row.H, &row.W, &row.Vertical, &row.BasePrice,
			&row.EvennessBySlotMode, &row.StartTS, &row.EndTS, &row.ActiveIntervals,
			&row.Country, &row.Language, &row.CampaignName, &row.FormatType, &row.BrandName,
			&row.QualityType, &row.PricingModel, &row.Status, &row.TrafficType,
		); err != nil {
			return nil, fmt.Errorf("scan campaign row: %w", err)
		}

		campaign, err := row.toCampaign()
		if err != nil {
			return nil, err
		}
		campaigns[campaign.ID] = campaign
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaign rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close campaign rows: %w", err)
	}

	for campaignID, campaign := range campaigns {
		creatives, err := getCreativesForCampaignFromPostgres(ctx, db, campaignID)
		if err != nil {
			return nil, err
		}
		campaign.Creatives = creatives
	}

	return campaigns, nil
}

func getCreativesForCampaignFromPostgres(ctx context.Context, db *sql.DB, campaignID string) (map[string]*Creative, error) {
	rows, err := db.QueryContext(ctx, getCreativesByCampaignQuery, campaignID)
	if err != nil {
		return nil, fmt.Errorf("query creatives for campaign_id=%s: %w", campaignID, err)
	}
	defer rows.Close()

	creatives := make(map[string]*Creative)
	for rows.Next() {
		var row creativeRow
		if err := rows.Scan(
			&row.ID, &row.CampaignID, &row.TrackersMacros, &row.W, &row.H,
			&row.Name, &row.CreativeName, &row.ADMURL, &row.Title, &row.Description,
		); err != nil {
			return nil, fmt.Errorf("scan creative row for campaign_id=%s: %w", campaignID, err)
		}

		creative, err := row.toCreative()
		if err != nil {
			return nil, fmt.Errorf("parse creative for campaign_id=%s: %w", campaignID, err)
		}
		creatives[creative.ID] = creative
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate creatives for campaign_id=%s: %w", campaignID, err)
	}
	return creatives, nil
}

type creativeRow struct {
	ID, CampaignID, Name, CreativeName, ADMURL, Title, Description sql.NullString
	TrackersMacros                                                 []byte
	W, H                                                           sql.NullInt64
}

func (r creativeRow) toCreative() (*Creative, error) {
	creativeID := nullableString(r.ID)
	if creativeID == "" {
		return nil, fmt.Errorf("creative id is empty")
	}
	trackersMacros, err := parseTrackersMacrosJSONB(r.TrackersMacros)
	if err != nil {
		return nil, fmt.Errorf("parse trackers_macros for creative_id=%s: %w", creativeID, err)
	}
	return &Creative{
		ID:             creativeID,
		CampaignID:     nullableString(r.CampaignID),
		TrackersMacros: trackersMacros,
		W:              int(r.W.Int64),
		H:              int(r.H.Int64),
		Name:           nullableString(r.Name),
		CreativeName:   nullableString(r.CreativeName),
		ADMURL:         nullableString(r.ADMURL),
		Title:          nullableString(r.Title),
		Description:    nullableString(r.Description),
	}, nil
}

func parseTrackersMacrosJSONB(raw []byte) (map[string]bool, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil, nil
	}
	var macros map[string]bool
	if err := json.Unmarshal(raw, &macros); err != nil {
		return nil, err
	}
	return macros, nil
}

type campaignRow struct {
	UserID, CampaignID, BasePrice, CampaignName, FormatType        sql.NullString
	BrandName, QualityType, PricingModel, Status, TrafficType      sql.NullString
	DeviceType, OS, Browser, SiteID, IP, Vertical, ActiveIntervals []byte
	Country, Language                                              []byte
	H, W                                                           sql.NullInt64
	EvennessBySlotMode                                             sql.NullBool
	StartTS, EndTS                                                 sql.NullTime
}

func (r campaignRow) toCampaign() (*Campaign, error) {
	campaignID := nullableString(r.CampaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("campaign_id is empty")
	}
	basePrice, err := parseNullFloat64(r.BasePrice)
	if err != nil {
		return nil, fmt.Errorf("parse base_price for campaign_id=%s: %w", campaignID, err)
	}
	activeIntervals, err := parseActiveIntervalsJSONB(r.ActiveIntervals, r.StartTS.Time, r.EndTS.Time)
	if err != nil {
		return nil, fmt.Errorf("parse active_intervals for campaign_id=%s: %w", campaignID, err)
	}
	countryFilter, err := parseCampaignFilter(r.Country, campaignID, "country")
	if err != nil {
		return nil, err
	}
	languageFilter, err := parseCampaignFilter(r.Language, campaignID, "language")
	if err != nil {
		return nil, err
	}
	deviceTypeFilter, err := parseCampaignFilter(r.DeviceType, campaignID, "device_type")
	if err != nil {
		return nil, err
	}
	osFilter, err := parseCampaignFilter(r.OS, campaignID, "os")
	if err != nil {
		return nil, err
	}
	browserFilter, err := parseCampaignFilter(r.Browser, campaignID, "browser")
	if err != nil {
		return nil, err
	}
	siteIDFilter, err := parseCampaignFilter(r.SiteID, campaignID, "site_id")
	if err != nil {
		return nil, err
	}
	ipFilter, err := parseCampaignFilter(r.IP, campaignID, "ip")
	if err != nil {
		return nil, err
	}

	campaign := &Campaign{
		ID: campaignID, UserID: nullableString(r.UserID), Status: nullableString(r.Status), PricingModel: nullableString(r.PricingModel),
		Format: nullableString(r.FormatType), CampaignName: nullableString(r.CampaignName), BrandName: nullableString(r.BrandName),
		QualitySegment: nullableString(r.QualityType), TrafficType: nullableString(r.TrafficType), H: int(r.H.Int64), W: int(r.W.Int64),
		Vertical: cloneJSONRawMessage(r.Vertical), BasePrice: basePrice, EvennessBySlotMode: r.EvennessBySlotMode.Valid && r.EvennessBySlotMode.Bool,
		StartTS: r.StartTS.Time, EndTS: r.EndTS.Time, CountryFilter: countryFilter, LanguageFilter: languageFilter,
		DeviceTypeFilter: deviceTypeFilter, OSFilter: osFilter, BrowserFilter: browserFilter, SiteIDFilter: siteIDFilter, IPFilter: ipFilter,
		ActiveIntervals: activeIntervals, Creatives: make(map[string]*Creative),
	}
	campaign.normalizeDefaultsLocked()
	return campaign, nil
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func parseNullFloat64(value sql.NullString) (float64, error) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value.String, 64)
}

func cloneJSONRawMessage(raw []byte) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func parseCampaignFilter(raw []byte, campaignID, field string) (*filterV2.Filters, error) {
	filters, err := filterV2.GetFiltersFromJSONB(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s filter for campaign_id=%s: %w", field, campaignID, err)
	}
	return filters, nil
}

func parseActiveIntervalsJSONB(raw []byte, windowStart, windowEnd time.Time) ([]TimeRange, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return nil, nil
	}
	var schedule [][]string
	if err := json.Unmarshal(raw, &schedule); err == nil {
		return ParseActiveIntervalSchedule(schedule, windowStart, windowEnd)
	}
	var ranges []TimeRange
	if err := json.Unmarshal(raw, &ranges); err != nil {
		return nil, err
	}
	return ranges, nil
}

// Campaign представляет рекламную кампанию
type Campaign struct {
	// Основные поля
	ID                  string
	UserID              string
	Status              string
	PricingModel        string
	Format              string
	CampaignName        string
	BrandName           string
	TrafficType         string
	H                   int
	W                   int
	Vertical            json.RawMessage
	BasePrice           float64
	PlatformFeePercent  float64
	EvennessBySlotMode  bool
	GoalTotalDollars    float64
	Budget              float64
	DailyBudget         *float64
	DailySpent          float64
	DailySpentResetDate time.Time
	QualitySegment      string
	CumDoneDollars      float64
	SlotDoneDollars     float64
	StartTS             time.Time
	EndTS               time.Time

	// Связь с фильтрами
	DSPURL string // URL DSP, которому принадлежит кампания

	CountryFilter    *filterV2.Filters
	LanguageFilter   *filterV2.Filters
	DeviceTypeFilter *filterV2.Filters
	OSFilter         *filterV2.Filters
	BrowserFilter    *filterV2.Filters
	SiteIDFilter     *filterV2.Filters
	IPFilter         *filterV2.Filters

	// Активные интервалы (nil или пустой = всегда активна)
	ActiveIntervals []TimeRange

	// Креативы кампании: первый уровень мапы находится в AuctionService.creativesByCampaignID,
	// а это локальная ссылка для удобного доступа из Campaign.
	Creatives map[string]*Creative

	// Защита от конкурентного доступа
	mu sync.RWMutex
}

func (c *Campaign) normalizeDefaultsLocked() {
	if c.Status == "" {
		c.Status = CampaignStatusActive
	}
	if c.PricingModel == "" {
		c.PricingModel = PricingModelCPM
	}
	if c.Budget == 0 {
		c.Budget = c.GoalTotalDollars
	}
	if c.QualitySegment == "" {
		c.QualitySegment = string(QualitySegmentUsual)
	}
	if c.Creatives == nil {
		c.Creatives = make(map[string]*Creative)
	}
}

// GetCPMEquivalent возвращает CPM-эквивалент для сортировки в аукционе.
func (c *Campaign) GetCPMEquivalent() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BasePrice
}

// GetCost возвращает стоимость события в зависимости от типа события и модели оплаты.
func (c *Campaign) GetCost(eventType string) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	switch eventType {
	case EventTypeImpression:
		if strings.EqualFold(c.PricingModel, PricingModelCPM) {
			return c.BasePrice / 1000
		}
	case EventTypeClick:
		if strings.EqualFold(c.PricingModel, PricingModelCPC) {
			return c.BasePrice
		}
		if strings.EqualFold(c.PricingModel, PricingModelCPM) && (c.Format == "popunder" || c.Format == "in-page-push") {
			return c.BasePrice / 1000
		}
	}

	return 0
}

// GetAuctionPrice возвращает цену после вычета процента платформы.
func (c *Campaign) GetAuctionPrice() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	fee := math.Max(0, math.Min(c.PlatformFeePercent, 100))
	return c.BasePrice * (1 - fee/100)
}

// IsActiveInIntervals проверяет, попадает ли текущее время в один из заданных интервалов.
func (c *Campaign) IsActiveInIntervals(now time.Time) bool {
	if len(c.ActiveIntervals) == 0 {
		return true
	}
	for _, interval := range c.ActiveIntervals {
		if (now.Equal(interval.Start) || now.After(interval.Start)) && now.Before(interval.End) {
			return true
		}
	}
	return false
}

// IsActiveGlobal проверяет, активна ли кампания глобально.
func (c *Campaign) IsActiveGlobal(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeDefaultsLocked()
	if c.Status != CampaignStatusActive || now.Before(c.StartTS) || !now.Before(c.EndTS) {
		return false
	}
	if !c.isActiveInIntervalsLocked(now) {
		return false
	}
	return c.CumDoneDollars < c.goalLocked() && c.checkDailyBudgetLocked(now)
}

func (c *Campaign) goalLocked() float64 {
	if c.Budget > 0 {
		return c.Budget
	}
	return c.GoalTotalDollars
}

func (c *Campaign) isActiveInIntervalsLocked(now time.Time) bool {
	if len(c.ActiveIntervals) == 0 {
		return true
	}
	for _, interval := range c.ActiveIntervals {
		if (now.Equal(interval.Start) || now.After(interval.Start)) && now.Before(interval.End) {
			return true
		}
	}
	return false
}

// calculateActiveSeconds возвращает количество секунд работы кампании от now до end с учётом интервалов.
func calculateActiveSeconds(now time.Time, intervals []TimeRange, start, end time.Time) int64 {
	from := maxTime(now, start)
	if !from.Before(end) {
		return 0
	}
	if len(intervals) == 0 {
		return int64(end.Sub(from).Seconds())
	}
	var seconds int64
	for _, interval := range intervals {
		intervalStart := maxTime(from, interval.Start)
		intervalEnd := minTime(end, interval.End)
		if intervalStart.Before(intervalEnd) {
			seconds += int64(intervalEnd.Sub(intervalStart).Seconds())
		}
	}
	return seconds
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// SlotsLeft возвращает количество полных активных слотов до окончания кампании.
func (c *Campaign) SlotsLeft(now time.Time) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	seconds := calculateActiveSeconds(now, c.ActiveIntervals, c.StartTS, c.EndTS)
	if seconds <= 0 {
		return 0
	}
	return int(math.Ceil(float64(seconds) / SlotDuration.Seconds()))
}

// SlotTarget вычисляет целевую сумму на текущий слот.
func (c *Campaign) SlotTarget(now time.Time) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	remaining := c.goalLocked() - c.CumDoneDollars
	if remaining <= 0 {
		return 0
	}
	seconds := calculateActiveSeconds(now, c.ActiveIntervals, c.StartTS, c.EndTS)
	if seconds <= 0 {
		return 0
	}
	return remaining / math.Ceil(float64(seconds)/SlotDuration.Seconds())
}

// IsEligibleByMinThreshold проверяет минимальный порог участия 20%.
func (c *Campaign) IsEligibleByMinThreshold() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	goal := c.goalLocked()
	if goal <= 0 {
		return false
	}
	remainingShare := (goal - c.CumDoneDollars) / goal
	return remainingShare >= 0.20
}

// ShouldParticipateInSlot определяет, может ли кампания участвовать в слоте.
func (c *Campaign) ShouldParticipateInSlot(now time.Time) bool {
	if !c.IsActiveGlobal(now) || !c.IsEligibleByMinThreshold() {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.EvennessBySlotMode {
		return true
	}
	target := c.slotTargetLocked(now)
	return target > 0 && c.SlotDoneDollars < target
}

func (c *Campaign) slotTargetLocked(now time.Time) float64 {
	remaining := c.goalLocked() - c.CumDoneDollars
	seconds := calculateActiveSeconds(now, c.ActiveIntervals, c.StartTS, c.EndTS)
	if remaining <= 0 || seconds <= 0 {
		return 0
	}
	return remaining / math.Ceil(float64(seconds)/SlotDuration.Seconds())
}

// CheckDailyBudget проверяет дневной лимит. Сброс счётчика между днями в памяти;
// для нескольких инстансов сервиса требуется внешнее хранилище дневных расходов.
func (c *Campaign) CheckDailyBudget(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checkDailyBudgetLocked(now)
}

func (c *Campaign) checkDailyBudgetLocked(now time.Time) bool {
	if c.DailyBudget == nil {
		return true
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if c.DailySpentResetDate.IsZero() || !c.DailySpentResetDate.Equal(day) {
		c.DailySpentResetDate = day
		c.DailySpent = 0
	}
	return c.DailySpent < *c.DailyBudget
}

// RecordEvent учитывает событие показа или клика и обновляет бюджетные статусы.
func (c *Campaign) RecordEvent(eventType string) {
	now := time.Now()
	cost := c.GetCost(eventType)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalizeDefaultsLocked()
	if now.After(c.EndTS) {
		c.Status = CampaignStatusCompleted
		return
	}
	if !c.checkDailyBudgetLocked(now) {
		log.Printf("[Auction] daily budget reached: campaign=%s", c.ID)
		return
	}
	if cost <= 0 {
		return
	}
	c.CumDoneDollars += cost
	c.SlotDoneDollars += cost
	if c.DailyBudget != nil {
		c.DailySpent += cost
	}
	if c.CumDoneDollars >= c.goalLocked() {
		c.Status = CampaignStatusNoBudget
	}
	if now.After(c.EndTS) {
		c.Status = CampaignStatusCompleted
	}
	log.Printf("[Auction] recorded %s: campaign=%s cost=%.6f total=%.6f status=%s", eventType, c.ID, cost, c.CumDoneDollars, c.Status)
}

// ResetSlotDone обнуляет счётчик слота.
func (c *Campaign) ResetSlotDone() { c.mu.Lock(); defer c.mu.Unlock(); c.SlotDoneDollars = 0 }

func (c *Campaign) GetCumDone() float64 { c.mu.RLock(); defer c.mu.RUnlock(); return c.CumDoneDollars }
func (c *Campaign) GetSlotDone() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SlotDoneDollars
}

// FirstCreative возвращает первый доступный креатив кампании.
func (c *Campaign) FirstCreative() *Creative {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, creative := range c.Creatives {
		return creative
	}
	return nil
}

// AuctionService - сервис аукциона
type AuctionService struct {
	campaigns             *map[string]*Campaign
	creativesByCampaignID *map[string]map[string]*Creative
	filterProcessor       *filter.OptimizedFilterProcessor
	mu                    sync.RWMutex
}

func NewAuctionService(filterProcessor *filter.OptimizedFilterProcessor) *AuctionService {
	return &AuctionService{
		campaigns:             &map[string]*Campaign{},
		creativesByCampaignID: &map[string]map[string]*Creative{},
		filterProcessor:       filterProcessor,
	}
}

func (s *AuctionService) ReplaceCampaigns(campaigns map[string]*Campaign) {
	if campaigns == nil {
		campaigns = make(map[string]*Campaign)
	}

	creativesByCampaignID := make(map[string]map[string]*Creative, len(campaigns))
	for campaignID, campaign := range campaigns {
		if campaign == nil {
			continue
		}
		campaign.mu.Lock()
		campaign.normalizeDefaultsLocked()
		campaign.mu.Unlock()
		creativesByCampaignID[campaignID] = campaign.Creatives
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.campaigns = &campaigns
	s.creativesByCampaignID = &creativesByCampaignID
}

func (s *AuctionService) StartPostgresRefreshTicker(ctx context.Context, db *sql.DB, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}

	refresh := func() {
		campaigns, err := GetCampaignsFromPostgres(ctx, db)
		if err != nil {
			log.Printf("[Auction] failed to refresh campaigns: %v", err)
			return
		}
		s.ReplaceCampaigns(campaigns)
	}

	refresh()
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}

func (s *AuctionService) AddCampaign(campaign *Campaign) {
	campaign.mu.Lock()
	campaign.normalizeDefaultsLocked()
	if campaign.DailyBudget != nil && *campaign.DailyBudget > campaign.goalLocked() {
		*campaign.DailyBudget = campaign.goalLocked()
	}
	campaign.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	(*s.campaigns)[campaign.ID] = campaign
	(*s.creativesByCampaignID)[campaign.ID] = campaign.Creatives
}

func (s *AuctionService) GetCampaign(id string) *Campaign {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return (*s.campaigns)[id]
}

// SetCampaignCreatives задаёт креативы кампании в формате:
// campaignID -> creativeID -> Creative. ADMURL берётся из Creative.ADMURL.
func (s *AuctionService) SetCampaignCreatives(campaignID string, creatives map[string]*Creative) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if creatives == nil {
		creatives = make(map[string]*Creative)
	}
	(*s.creativesByCampaignID)[campaignID] = creatives
	if campaign := (*s.campaigns)[campaignID]; campaign != nil {
		campaign.mu.Lock()
		campaign.Creatives = creatives
		campaign.mu.Unlock()
	}
}

// ApplyCreativeTrackerMacros читает из Postgres таблицу creatives (id, trackers_macros)
// и добавляет в ADM-ссылки креативов включённые макросы как query-параметры.
func (s *AuctionService) ApplyCreativeTrackerMacros(
	ctx context.Context,
	db *sql.DB,
	campaignID string,
	req *ortb_V2_5.BidRequest,
	values TrackerMacroValues,
) error {
	if db == nil {
		return fmt.Errorf("postgres db is nil")
	}

	rows, err := db.QueryContext(ctx, `SELECT id, trackers_macros FROM creatives`)
	if err != nil {
		return fmt.Errorf("query creatives trackers_macros: %w", err)
	}
	defer rows.Close()

	s.mu.RLock()
	campaignCreatives := (*s.creativesByCampaignID)[campaignID]
	s.mu.RUnlock()
	if len(campaignCreatives) == 0 {
		return nil
	}

	for rows.Next() {
		var creativeID string
		var rawMacros []byte
		if err := rows.Scan(&creativeID, &rawMacros); err != nil {
			return fmt.Errorf("scan creative trackers_macros: %w", err)
		}

		creative := campaignCreatives[creativeID]
		if creative == nil {
			continue
		}

		var macros map[string]bool
		if len(rawMacros) > 0 {
			if err := json.Unmarshal(rawMacros, &macros); err != nil {
				return fmt.Errorf("unmarshal trackers_macros for creative %s: %w", creativeID, err)
			}
		}
		creative.TrackersMacros = macros
		creative.ADMURL = appendTrackerMacrosToADMURL(creative.ADMURL, macros, campaignID, creativeID, req, values)
	}

	return rows.Err()
}

func appendTrackerMacrosToADMURL(
	admURL string,
	macros map[string]bool,
	campaignID string,
	creativeID string,
	req *ortb_V2_5.BidRequest,
	values TrackerMacroValues,
) string {
	if len(macros) == 0 {
		return admURL
	}

	parsedURL, err := url.Parse(admURL)
	if err != nil {
		return admURL
	}

	query := parsedURL.Query()
	for key, enabled := range macros {
		if !enabled {
			continue
		}
		query.Set(key, trackerMacroValue(key, campaignID, creativeID, req, values))
	}
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

func trackerMacroValue(
	key string,
	campaignID string,
	creativeID string,
	req *ortb_V2_5.BidRequest,
	values TrackerMacroValues,
) string {
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
	return ""
}

func (s *AuctionService) passesFilters(c *Campaign, req *ortb_V2_5.BidRequest) bool {
	if s.filterProcessor == nil {
		return true
	}
	return s.filterProcessor.ProcessRequestForDSPV25(c.DSPURL, req).Allowed
}

func chooseMostExpensive(campaigns []*Campaign) *Campaign {
	if len(campaigns) == 0 {
		return nil
	}
	sort.SliceStable(campaigns, func(i, j int) bool { return campaigns[i].GetAuctionPrice() > campaigns[j].GetAuctionPrice() })
	return campaigns[0]
}

func (s *AuctionService) chooseFirstFilteredByAuctionPrice(campaigns []*Campaign, req *ortb_V2_5.BidRequest) *Campaign {
	sortedCampaigns := append([]*Campaign(nil), campaigns...)
	chooseMostExpensive(sortedCampaigns)
	for _, campaign := range sortedCampaigns {
		if s.passesFilters(campaign, req) {
			return campaign
		}
	}
	return nil
}

// GetSSPDomainsForQualitySegment возвращает SSP-фиды для сегмента качества.
func GetSSPDomainsForQualitySegment(segment string) []string {
	domains := qualitySegmentSSPMap[QualitySegment(segment)]
	if len(domains) == 0 {
		domains = qualitySegmentSSPMap[QualitySegmentUsual]
	}
	return append([]string(nil), domains...)
}

// SelectCampaign выбирает кампанию для BidRequest.
func (s *AuctionService) SelectCampaign(req *ortb_V2_5.BidRequest, now time.Time) *Campaign {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var priority, regular, active []*Campaign
	for _, c := range *s.campaigns {
		auctionPrice := c.GetAuctionPrice()
		if auctionPrice <= 0 || !c.IsActiveGlobal(now) || !c.IsEligibleByMinThreshold() {
			continue
		}

		active = append(active, c)
		c.mu.RLock()
		even := c.EvennessBySlotMode
		c.mu.RUnlock()
		if !even {
			priority = append(priority, c)
		} else if c.ShouldParticipateInSlot(now) {
			regular = append(regular, c)
		}
	}
	var selected *Campaign
	mode := "fallback"
	if len(priority) > 0 {
		selected = s.chooseFirstFilteredByAuctionPrice(priority, req)
		mode = "priority"
	} else if len(regular) > 0 {
		selected = s.chooseFirstFilteredByAuctionPrice(regular, req)
		mode = "even"
	} else {
		selected = s.chooseFirstFilteredByAuctionPrice(active, req)
	}
	if selected == nil {
		return nil
	}
	auctionPrice := selected.GetAuctionPrice()
	log.Printf("[Auction] Selected campaign: ID=%s, auction_price=%.2f, DSP=%s, segment=%s, feeds=%v, mode=%s, slot_done=%.2f, slot_target=%.2f",
		selected.ID, auctionPrice, selected.DSPURL, selected.QualitySegment,
		GetSSPDomainsForQualitySegment(selected.QualitySegment), mode, selected.GetSlotDone(), selected.SlotTarget(now))
	return selected
}

// CheckUserBalance обновляет статусы кампаний пользователя по внешнему сигналу баланса.
// В текущей in-memory модели userID не хранится, поэтому вызывающий код должен передать кампании нужного пользователя.
func CheckUserBalance(campaigns []*Campaign, hasPositiveBalance bool) {
	for _, c := range campaigns {
		c.mu.Lock()
		if hasPositiveBalance && c.Status == CampaignStatusPaused {
			c.Status = CampaignStatusActive
		}
		if !hasPositiveBalance && c.Status == CampaignStatusActive {
			c.Status = CampaignStatusPaused
		}
		c.mu.Unlock()
	}
}

func (s *AuctionService) SlotTick(now time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	log.Printf("[Auction] New slot started at %v", now.Truncate(SlotDuration))
	for _, campaign := range *s.campaigns {
		campaign.ResetSlotDone()
	}
}

func (s *AuctionService) GetActiveCampaignsCount(now time.Time) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, campaign := range *s.campaigns {
		if campaign.IsActiveGlobal(now) {
			count++
		}
	}
	return count
}
