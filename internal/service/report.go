package service

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type ReportRow struct {
	Group       string  `json:"group"`
	Impressions uint64  `json:"impressions"`
	Clicks      uint64  `json:"clicks"`
	Cost        float64 `json:"cost"`
}

type ReportResponse struct {
	Feed      string      `json:"feed"`
	SspDomain string      `json:"spp_domain"`
	GroupBy   string      `json:"group_by"`
	DateStart string      `json:"date_start"`
	DateEnd   string      `json:"date_end"`
	Rows      []ReportRow `json:"rows"`
}

type ReportService struct {
	conn                 clickhouse.Conn
	feedResolver         *FeedResolver
	factClicksTable      string
	factImpressionsTable string
}

func NewReportService(conn clickhouse.Conn, feedResolver *FeedResolver, factClicksTable, factImpressionsTable string) *ReportService {
	return &ReportService{
		conn:                 conn,
		feedResolver:         feedResolver,
		factClicksTable:      factClicksTable,
		factImpressionsTable: factImpressionsTable,
	}
}

func (s *ReportService) BuildReport(ctx context.Context, feed, groupBy, dateStart, dateEnd string) (*ReportResponse, int, error) {
	if feed == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("feed is required")
	}
	if groupBy == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("group_by is required")
	}
	if dateStart == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("date_start is required")
	}
	if dateEnd == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("date_end is required")
	}

	if _, err := time.Parse("2006-01-02", dateStart); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid date_start, expected YYYY-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", dateEnd); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid date_end, expected YYYY-MM-DD")
	}

	sspDomain, err := s.feedResolver.Resolve(feed)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}

	groupExpr, normalizedGroup, err := normalizeGroupBy(groupBy)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	if err := validateTableName(s.factClicksTable); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if err := validateTableName(s.factImpressionsTable); err != nil {
		return nil, http.StatusInternalServerError, err
	}

	query := fmt.Sprintf(`
WITH
clicks_part AS
(
    SELECT
        %s AS gr_key,
        count(*) AS clicks,
        sumIf(win_dsp_price, format = 'POP') / 1000
            + sumIf(win_dsp_price, format = 'IPP') AS cost
    FROM %s
    WHERE spp_domain = ?
      AND event_date >= toDate(?)
      AND event_date <= toDate(?)
    GROUP BY gr_key
),
impressions_part AS
(
    SELECT
        %s AS gr_key,
        count(*) AS impressions,
        sumIf(win_final_price, format IN ('BAN', 'NAT')) / 1000 AS cost
    FROM %s
    WHERE spp_domain = ?
      AND event_date >= toDate(?)
      AND event_date <= toDate(?)
    GROUP BY gr_key
)
SELECT
    toString(coalesce(c.gr_key, i.gr_key)) AS gr_key,
    ifNull(i.impressions, 0) AS impressions,
    ifNull(c.clicks, 0) AS clicks,
    ifNull(c.cost, 0) + ifNull(i.cost, 0) AS cost
FROM clicks_part AS c
FULL OUTER JOIN impressions_part AS i
    ON c.gr_key = i.gr_key
ORDER BY gr_key`, groupExpr, s.factClicksTable, groupExpr, s.factImpressionsTable)

	rows, err := s.conn.Query(ctx, query, sspDomain, dateStart, dateEnd, sspDomain, dateStart, dateEnd)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("clickhouse query failed: %w", err)
	}
	defer rows.Close()

	result := make([]ReportRow, 0)
	for rows.Next() {
		var row ReportRow
		if err := rows.Scan(&row.Group, &row.Impressions, &row.Clicks, &row.Cost); err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("scan row failed: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("read rows failed: %w", err)
	}

	return &ReportResponse{
		Feed:      feed,
		SspDomain: sspDomain,
		GroupBy:   normalizedGroup,
		DateStart: dateStart,
		DateEnd:   dateEnd,
		Rows:      result,
	}, http.StatusOK, nil
}

func normalizeGroupBy(groupBy string) (string, string, error) {
	switch groupBy {
	case "geo":
		return "geo", "geo", nil
	case "date", "event_date":
		return "event_date", "date", nil
	case "site", "site_id":
		return "site_id", "site", nil
	default:
		return "", "", fmt.Errorf("invalid group_by, allowed: geo, date, site")
	}
}

var tableNameRe = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

func validateTableName(tableName string) error {
	if !tableNameRe.MatchString(tableName) {
		return fmt.Errorf("invalid clickhouse table name: %s", tableName)
	}
	return nil
}
