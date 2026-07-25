package auction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	filterV2 "gitlab.com/twinbid-exchange/RTB-exchange/internal/filterV2"
)

var weekdayIndex = map[string]int{
	"sun": 0,
	"mon": 1,
	"tue": 2,
	"wed": 3,
	"thu": 4,
	"fri": 5,
	"sat": 6,
}

// ParseActiveIntervalSchedule expands weekly inclusive-hour intervals into
// concrete UTC half-open ranges clipped to the campaign window.
func ParseActiveIntervalSchedule(schedule [][]string, windowStart, windowEnd time.Time) ([]TimeRange, error) {
	if len(schedule) == 0 || !windowStart.Before(windowEnd) {
		return nil, nil
	}
	windowStart = windowStart.UTC()
	windowEnd = windowEnd.UTC()
	weekStart := startOfWeek(windowStart)
	expandedUntil := windowEnd.Add(7 * 24 * time.Hour)
	intervals := make([]TimeRange, 0, len(schedule))

	for _, pair := range schedule {
		if len(pair) != 2 {
			return nil, fmt.Errorf("invalid active interval %v", pair)
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
		endOffset += time.Hour // schedule endpoint is an inclusive hour

		for base := weekStart.Add(-7 * 24 * time.Hour); base.Before(expandedUntil); base = base.Add(7 * 24 * time.Hour) {
			start := maxTime(base.Add(startOffset), windowStart)
			end := minTime(base.Add(endOffset), windowEnd)
			if start.Before(end) {
				intervals = append(intervals, TimeRange{Start: start, End: end})
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
	t = t.UTC()
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return dayStart.Add(-time.Duration(dayStart.Weekday()) * 24 * time.Hour)
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

func (s *AuctionService) RefreshFromPostgres(ctx context.Context, db *sql.DB) error {
	snapshot, err := LoadSnapshotFromPostgres(ctx, db)
	if err != nil {
		return err
	}
	return s.PublishSnapshot(snapshot)
}

func (s *AuctionService) StartPostgresRefreshTicker(ctx context.Context, db *sql.DB, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.RefreshFromPostgres(ctx, db); err != nil && onError != nil {
					onError(err)
				}
			}
		}
	}()
}

func LoadSnapshotFromPostgres(ctx context.Context, db *sql.DB) (*Snapshot, error) {
	if db == nil {
		return nil, errors.New("postgres db is nil")
	}
	query := `
	SELECT
		user_id::text,
		campaign_id::text,
		base_price::text,
		evenness_by_slot_mode,
		start_ts,
		end_ts,
		active_intervals,
		country,
		language,
		device_type,
		os,
		browser,
		site_id,
		ip,
		format_type,
		quality_type,
		pricing_model,
		status,
		traffic_type,
		goal_total_dollars::text,
		traffic_reset_version,
		updated_at
	FROM campaigns
	WHERE status = 'active'
`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query active campaigns: %w", err)
	}
	defer rows.Close()

	campaigns := make([]*Campaign, 0)
	campaignByID := make(map[string]*Campaign)
	userSet := make(map[string]struct{})
	activeRows := 0
	for rows.Next() {
		activeRows++
		var row campaignDBRow
		if err := rows.Scan(
			&row.UserID, &row.CampaignID, &row.BasePrice,
			&row.Evenness, &row.StartTS, &row.EndTS, &row.ActiveIntervals,
			&row.Country, &row.Language, &row.DeviceType, &row.OS, &row.Browser, &row.SiteID, &row.IP,
			&row.Format, &row.Quality, &row.PricingModel, &row.Status, &row.TrafficType,
			&row.GoalTotalDollars,
			&row.TrafficResetVersion, &row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan active campaign: %w", err)
		}
		campaign, err := row.campaign()
		if err != nil {
			log.Printf("ADV snapshot: skipping invalid active campaign: %v", err)
			continue
		}
		if _, duplicate := campaignByID[campaign.ID]; duplicate {
			log.Printf("ADV snapshot: skipping duplicate active campaign_id %s", campaign.ID)
			continue
		}
		campaigns = append(campaigns, campaign)
		campaignByID[campaign.ID] = campaign
		userSet[campaign.UserID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active campaigns: %w", err)
	}

	campaignIDs := make([]string, 0, len(campaignByID))
	for id := range campaignByID {
		campaignIDs = append(campaignIDs, id)
	}
	if err := loadCreativesBatch(ctx, db, campaignIDs, campaignByID); err != nil {
		return nil, err
	}

	userIDs := make([]string, 0, len(userSet))
	for id := range userSet {
		userIDs = append(userIDs, id)
	}
	userGoals, err := loadUserGoalsBatch(ctx, db, userIDs)
	if err != nil {
		return nil, err
	}
	validCampaigns := make([]*Campaign, 0, len(campaigns))
	for _, campaign := range campaigns {
		if campaign == nil {
			continue
		}
		if _, ok := userGoals[campaign.UserID]; !ok {
			log.Printf(
				"ADV snapshot: skipping campaign %s because users.goal_total_dollars is missing or invalid for owner %s",
				campaign.ID,
				campaign.UserID,
			)
			continue
		}
		validCampaigns = append(validCampaigns, campaign)
	}
	if activeRows > 0 && len(validCampaigns) == 0 {
		return nil, fmt.Errorf("all %d active campaign rows were invalid; previous snapshot must be retained", activeRows)
	}
	return &Snapshot{Campaigns: validCampaigns, UserGoals: userGoals}, nil
}

type campaignDBRow struct {
	UserID              sql.NullString
	CampaignID          sql.NullString
	BasePrice           sql.NullString
	GoalTotalDollars    sql.NullString
	TrafficResetVersion sql.NullInt64
	UpdatedAt           sql.NullTime

	Format       sql.NullString
	Quality      sql.NullString
	PricingModel sql.NullString
	Status       sql.NullString
	TrafficType  sql.NullString

	Evenness sql.NullBool

	StartTS sql.NullTime
	EndTS   sql.NullTime

	ActiveIntervals []byte
	Country         []byte
	Language        []byte
	DeviceType      []byte
	OS              []byte
	Browser         []byte
	SiteID          []byte
	IP              []byte
}

func (r campaignDBRow) campaign() (*Campaign, error) {
	id := strings.TrimSpace(r.CampaignID.String)
	userID := strings.TrimSpace(r.UserID.String)
	if !r.CampaignID.Valid || !r.UserID.Valid || id == "" || userID == "" {
		return nil, errors.New("active campaign has empty campaign_id or user_id")
	}
	basePrice, err := parseFiniteNonNegative(r.BasePrice.String)
	if err != nil {
		return nil, fmt.Errorf("campaign %s base_price: %w", id, err)
	}
	goalTotalDollars, err := parseFiniteNonNegative(
		r.GoalTotalDollars.String,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"campaign %s goal_total_dollars: %w",
			id,
			err,
		)
	}

	if basePrice <= 0 {
		return nil, fmt.Errorf("campaign %s base_price must be positive", id)
	}
	status := strings.ToLower(strings.TrimSpace(r.Status.String))
	pricingModel := strings.ToUpper(strings.TrimSpace(r.PricingModel.String))
	format := normalizeFormat(r.Format.String)
	trafficType := normalizeTraffic(r.TrafficType.String)
	quality := strings.ToLower(strings.TrimSpace(r.Quality.String))
	if status != CampaignStatusActive {
		return nil, fmt.Errorf("campaign %s status is not active", id)
	}
	if pricingModel != PricingModelCPM && pricingModel != PricingModelCPC {
		return nil, fmt.Errorf("campaign %s has invalid pricing_model", id)
	}
	if format == "" || trafficType == "" {
		return nil, fmt.Errorf("campaign %s has invalid format or traffic_type", id)
	}
	if _, ok := validQualitySegments[quality]; !ok {
		return nil, fmt.Errorf("campaign %s has invalid quality_type", id)
	}
	if !r.StartTS.Valid || !r.EndTS.Valid || !r.StartTS.Time.Before(r.EndTS.Time) {
		return nil, fmt.Errorf("campaign %s has invalid start_ts/end_ts", id)
	}
	activeIntervals, err := parseActiveIntervalsJSONB(r.ActiveIntervals, r.StartTS.Time, r.EndTS.Time)
	if err != nil {
		return nil, fmt.Errorf("campaign %s active_intervals: %w", id, err)
	}
	parseFilter := func(raw []byte, field string) (*filterV2.Filters, error) {
		value, err := filterV2.GetFiltersFromJSONB(raw)
		if err != nil {
			return nil, fmt.Errorf("campaign %s %s filter: %w", id, field, err)
		}
		return value, nil
	}
	country, err := parseFilter(r.Country, "country")
	if err != nil {
		return nil, err
	}
	language, err := parseFilter(r.Language, "language")
	if err != nil {
		return nil, err
	}
	normalizeFilterObjects(language, normalizeLanguage)

	deviceType, err := parseFilter(r.DeviceType, "device_type")
	if err != nil {
		return nil, err
	}
	normalizeFilterObjects(deviceType, normalizeDeviceType)
	osFilter, err := parseFilter(r.OS, "os")
	if err != nil {
		return nil, err
	}
	browser, err := parseFilter(r.Browser, "browser")
	if err != nil {
		return nil, err
	}
	siteID, err := parseFilter(r.SiteID, "site_id")
	if err != nil {
		return nil, err
	}
	ip, err := parseFilter(r.IP, "ip")
	if err != nil {
		return nil, err
	}

	return &Campaign{
		ID: id, UserID: userID, Status: status,
		PricingModel: pricingModel,
		Format:       format, TrafficType: trafficType,
		QualitySegment: quality,
		BasePrice:      basePrice, GoalTotalDollars: goalTotalDollars, EvennessBySlotMode: r.Evenness.Valid && r.Evenness.Bool,
		StartTS: r.StartTS.Time.UTC(), EndTS: r.EndTS.Time.UTC(), ActiveIntervals: activeIntervals,
		CountryFilter: country, LanguageFilter: language, DeviceTypeFilter: deviceType,
		OSFilter: osFilter, BrowserFilter: browser, SiteIDFilter: siteID, IPFilter: ip,
		Creatives:           []*Creative{},
		TrafficResetVersion: r.TrafficResetVersion.Int64,
		UpdatedAt: func() time.Time {
			if r.UpdatedAt.Valid {
				return r.UpdatedAt.Time.UTC()
			}
			return time.Time{}
		}(),
	}, nil
}

func normalizeFilterObjects(filters *filterV2.Filters, normalize func(string) string) {
	if filters == nil || len(filters.Objects) == 0 || normalize == nil {
		return
	}

	normalized := make(map[string]bool, len(filters.Objects))
	for value := range filters.Objects {
		value = normalize(value)
		if value != "" {
			normalized[value] = true
		}
	}
	filters.Objects = normalized
	filters.Apply = len(normalized) > 0
}

func loadCreativesBatch(ctx context.Context, db *sql.DB, campaignIDs []string, campaigns map[string]*Campaign) error {
	if len(campaignIDs) == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT cr.id::text, cr.campaign_id::text, cr.trackers_macros, cr.w, cr.h,
		       cr.adm, COALESCE(ci.mime_type, cr.file_format), cr.banner_type,
		       cr.name, cr.creative_name, cr.title, cr.description
		FROM creatives cr
		LEFT JOIN creative_images ci ON ci.creative_id=cr.id
		WHERE cr.campaign_id::text = ANY($1)`, pq.Array(campaignIDs))
	if err != nil {
		return fmt.Errorf("batch query creatives: %w", err)
	}
	defer rows.Close()
	creativeIDs := make(map[string]map[string]struct{}, len(campaigns))
	for rows.Next() {
		var id, campaignID, adm, fileFormat, bannerType, name, creativeName, title, description sql.NullString
		var trackers []byte
		var w, h sql.NullInt64
		if err := rows.Scan(&id, &campaignID, &trackers, &w, &h, &adm, &fileFormat, &bannerType, &name, &creativeName, &title, &description); err != nil {
			return fmt.Errorf("scan creative: %w", err)
		}
		campaign := campaigns[strings.TrimSpace(campaignID.String)]
		if campaign == nil {
			continue
		}
		creativeID := strings.TrimSpace(id.String)
		admURL := strings.TrimSpace(adm.String)
		if creativeID == "" || admURL == "" {
			log.Printf("ADV snapshot: skipping invalid creative for campaign %s", campaign.ID)
			continue
		}
		if normalizeFormat(campaign.Format) == "BAN" && (!w.Valid || !h.Valid || w.Int64 <= 0 || h.Int64 <= 0) {
			log.Printf("ADV snapshot: skipping banner creative %s without valid dimensions for campaign %s", creativeID, campaign.ID)
			continue
		}
		seen := creativeIDs[campaign.ID]
		if seen == nil {
			seen = make(map[string]struct{})
			creativeIDs[campaign.ID] = seen
		}
		if _, duplicate := seen[creativeID]; duplicate {
			log.Printf("ADV snapshot: skipping duplicate creative %s for campaign %s", creativeID, campaign.ID)
			continue
		}
		macros, err := parseTrackersMacrosJSONB(trackers)
		if err != nil {
			log.Printf("ADV snapshot: skipping creative %s with invalid trackers_macros: %v", creativeID, err)
			continue
		}
		seen[creativeID] = struct{}{}
		creativeW, creativeH := 0, 0
		if w.Valid && w.Int64 > 0 {
			creativeW = int(w.Int64)
		}
		if h.Valid && h.Int64 > 0 {
			creativeH = int(h.Int64)
		}
		campaign.Creatives = append(campaign.Creatives, &Creative{
			ID: creativeID, CampaignID: campaign.ID, ADMURL: admURL,
			FileFormat: strings.TrimSpace(fileFormat.String), BannerType: strings.TrimSpace(bannerType.String),
			TrackersMacros: macros, W: creativeW, H: creativeH, Name: name.String,
			CreativeName: creativeName.String, Title: title.String, Description: description.String,
		})
	}
	return rows.Err()
}

func loadUserGoalsBatch(ctx context.Context, db *sql.DB, userIDs []string) (map[string]float64, error) {
	goals := make(map[string]float64, len(userIDs))
	if len(userIDs) == 0 {
		return goals, nil
	}
	rows, err := db.QueryContext(
		ctx,
		`
		SELECT
			id::text,
			goal_total_dollars::text
		FROM users
		WHERE id::text = ANY($1)
	`,
		pq.Array(userIDs),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"batch query users.goal_total_dollars: %w",
			err,
		)
	}
	defer rows.Close()
	for rows.Next() {
		var id, rawGoal string
		if err := rows.Scan(&id, &rawGoal); err != nil {
			return nil, fmt.Errorf(
				"scan users.goal_total_dollars: %w",
				err,
			)
		}
		id = strings.TrimSpace(id)
		goal, err := parseFiniteNonNegative(rawGoal)
		if id == "" || err != nil {
			log.Printf(
				"ADV snapshot: skipping invalid users.goal_total_dollars row for user %q: %v",
				id,
				err,
			)
			continue
		}
		goals[id] = goal
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return goals, nil
}

func parseFiniteNonNegative(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty numeric value")
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, errors.New("invalid non-negative finite number")
	}
	return parsed, nil
}

func parseTrackersMacrosJSONB(raw []byte) (map[string]bool, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil, nil
	}
	var result map[string]bool
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
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
	out := make([]TimeRange, 0, len(ranges))
	for i := range ranges {
		start := maxTime(ranges[i].Start.UTC(), windowStart.UTC())
		end := minTime(ranges[i].End.UTC(), windowEnd.UTC())
		if start.Before(end) {
			out = append(out, TimeRange{Start: start, End: end})
		}
	}
	return out, nil
}
