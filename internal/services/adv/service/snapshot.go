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

		for base := weekStart; base.Before(expandedUntil); base = base.Add(7 * 24 * time.Hour) {
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

// MigrateADVSchema performs all ADV schema changes atomically. The DO blocks
// keep compatibility with installations that still have a legacy balance or
// budget column.
func MigrateADVSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("postgres db is nil")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ADV schema migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'goal'
			) THEN
				ALTER TABLE users ADD COLUMN goal NUMERIC NOT NULL DEFAULT 0;
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'balance'
				) THEN
					EXECUTE 'UPDATE users SET goal = balance WHERE balance IS NOT NULL';
				END IF;
			END IF;
		END $$`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS spent NUMERIC NOT NULL DEFAULT 0`,
		`ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS spent NUMERIC NOT NULL DEFAULT 0`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = 'campaigns'
				AND column_name IN ('goal_total', 'goal', 'budget')
			) THEN
				ALTER TABLE campaigns ADD COLUMN goal_total NUMERIC NOT NULL DEFAULT 0;
			END IF;
		END $$`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ADV schema migration failed: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ADV schema migration: %w", err)
	}
	return nil
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
	goalColumn, err := campaignGoalColumn(ctx, db)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT
			user_id::text, campaign_id::text, base_price::text,
			evenness_by_slot_mode, start_ts, end_ts, active_intervals,
			country, language, device_type, os, browser, site_id, ip,
			format_type, quality_type, pricing_model, status, traffic_type,
			%s::text
		FROM campaigns
		WHERE status = 'active'`, goalColumn)
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
			&row.GoalTotal,
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
			log.Printf("ADV snapshot: skipping campaign %s because users.goal is missing or invalid for owner %s", campaign.ID, campaign.UserID)
			continue
		}
		validCampaigns = append(validCampaigns, campaign)
	}
	if activeRows > 0 && len(validCampaigns) == 0 {
		return nil, fmt.Errorf("all %d active campaign rows were invalid; previous snapshot must be retained", activeRows)
	}
	return &Snapshot{Campaigns: validCampaigns, UserGoals: userGoals}, nil
}

func campaignGoalColumn(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'campaigns'
		  AND column_name IN ('goal_total', 'goal', 'budget')`)
	if err != nil {
		return "", fmt.Errorf("discover campaign goal column: %w", err)
	}
	defer rows.Close()
	available := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		available[name] = true
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate campaign goal columns: %w", err)
	}
	for _, preferred := range []string{"goal_total", "goal", "budget"} {
		if available[preferred] {
			return preferred, nil
		}
	}
	return "", errors.New("campaign goal column is missing")
}

type campaignDBRow struct {
	UserID, CampaignID, BasePrice, GoalTotal           sql.NullString
	Format, Quality, PricingModel, Status, TrafficType sql.NullString
	Evenness                                           sql.NullBool
	StartTS, EndTS                                     sql.NullTime
	ActiveIntervals, Country, Language, DeviceType, OS []byte
	Browser, SiteID, IP                                []byte
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
	goal, err := parseFiniteNonNegative(r.GoalTotal.String)
	if err != nil {
		return nil, fmt.Errorf("campaign %s goal: %w", id, err)
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
	deviceType, err := parseFilter(r.DeviceType, "device_type")
	if err != nil {
		return nil, err
	}
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
		BasePrice:      basePrice, GoalTotalDollars: goal, EvennessBySlotMode: r.Evenness.Valid && r.Evenness.Bool,
		StartTS: r.StartTS.Time.UTC(), EndTS: r.EndTS.Time.UTC(), ActiveIntervals: activeIntervals,
		CountryFilter: country, LanguageFilter: language, DeviceTypeFilter: deviceType,
		OSFilter: osFilter, BrowserFilter: browser, SiteIDFilter: siteID, IPFilter: ip,
		Creatives: []*Creative{},
	}, nil
}

func loadCreativesBatch(ctx context.Context, db *sql.DB, campaignIDs []string, campaigns map[string]*Campaign) error {
	if len(campaignIDs) == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id::text, campaign_id::text, trackers_macros, w, h,
		       link, name, creative_name, title, description
		FROM creatives
		WHERE campaign_id::text = ANY($1)`, pq.Array(campaignIDs))
	if err != nil {
		return fmt.Errorf("batch query creatives: %w", err)
	}
	defer rows.Close()
	creativeIDs := make(map[string]map[string]struct{}, len(campaigns))
	for rows.Next() {
		var id, campaignID, adm, name, creativeName, title, description sql.NullString
		var trackers []byte
		var w, h sql.NullInt64
		if err := rows.Scan(&id, &campaignID, &trackers, &w, &h, &adm, &name, &creativeName, &title, &description); err != nil {
			return fmt.Errorf("scan creative: %w", err)
		}
		campaign := campaigns[strings.TrimSpace(campaignID.String)]
		if campaign == nil {
			continue
		}
		creativeID := strings.TrimSpace(id.String)
		admURL := strings.TrimSpace(adm.String)
		if creativeID == "" || admURL == "" || !w.Valid || !h.Valid || w.Int64 <= 0 || h.Int64 <= 0 {
			log.Printf("ADV snapshot: skipping invalid creative for campaign %s", campaign.ID)
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
		campaign.Creatives = append(campaign.Creatives, &Creative{
			ID: creativeID, CampaignID: campaign.ID, ADMURL: admURL,
			TrackersMacros: macros, W: int(w.Int64), H: int(h.Int64), Name: name.String,
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
	rows, err := db.QueryContext(ctx, `SELECT id::text, goal::text FROM users WHERE id::text = ANY($1)`, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("batch query user goals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, rawGoal string
		if err := rows.Scan(&id, &rawGoal); err != nil {
			return nil, fmt.Errorf("scan user goal: %w", err)
		}
		id = strings.TrimSpace(id)
		goal, err := parseFiniteNonNegative(rawGoal)
		if id == "" || err != nil {
			log.Printf("ADV snapshot: skipping invalid users.goal row for user %q: %v", id, err)
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
