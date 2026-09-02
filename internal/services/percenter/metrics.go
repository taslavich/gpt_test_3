package percenter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func LoadWindowMetrics(ctx context.Context, conn clickhouse.Conn, database, ortbTable, impressionsTable, clicksTable string, window time.Duration) ([]Metrics, error) {
	if conn == nil {
		return nil, fmt.Errorf("clickhouse connection is nil")
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	database = quoteIdentifier(database)
	ortbTable = quoteIdentifier(ortbTable)
	impressionsTable = quoteIdentifier(impressionsTable)
	clicksTable = quoteIdentifier(clicksTable)
	seconds := int64(window / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	query := fmt.Sprintf(`
SELECT
    o.segment_hash,
    o.percenter_point_version,
    count() AS requests,
    countIf(isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS wins,
    sumIf(ifNull(c.click_count, 0), isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS clicks,
    sumIf(o.win_dsp_price / 1000.0, isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS advertiser_spend,
    sumIf((o.win_dsp_price - o.win_final_price) / 1000.0, isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS twinbid_profit,
    sumIf((o.win_dsp_price - o.win_final_price) * ifNull(c.click_count, 0), isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS click_twinbid_profit
FROM %s.%s AS o
LEFT JOIN
(
    SELECT DISTINCT uuid
    FROM %s.%s
    WHERE event_time_impressions >= now64(3) - toIntervalSecond(%d)
) AS i ON o.uuid = i.uuid
LEFT JOIN
(
    SELECT uuid, uniqExact(clicks_uuid) AS click_count
    FROM %s.%s
    WHERE event_time_clicks >= now64(3) - toIntervalSecond(%d)
    GROUP BY uuid
) AS c ON o.uuid = c.uuid
WHERE o.event_time >= now64(3) - toIntervalSecond(%d)
  AND o.segment_hash != ''
  AND o.percenter_point_version > 0
GROUP BY o.segment_hash, o.percenter_point_version
SETTINGS join_use_nulls = 1
`, database, ortbTable, database, impressionsTable, seconds, database, clicksTable, seconds, seconds)

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Metrics, 0)
	for rows.Next() {
		var metric Metrics
		if err := rows.Scan(
			&metric.SegmentHash,
			&metric.PointVersion,
			&metric.Requests,
			&metric.Wins,
			&metric.Clicks,
			&metric.AdvertiserSpend,
			&metric.TwinBidProfit,
			&metric.ClickTwinBidProfit,
		); err != nil {
			return nil, err
		}
		result = append(result, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func quoteIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "``"
	}
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

// LoadFallbackTraffic reconstructs exact segment dimensions from ORTB and
// counts actual impression callbacks over the fixed fallback window. The
// segment_hash stored in ORTB may already be a parent hash, therefore routing
// decisions are based on raw dimensions + win_cid rather than on segment_hash.
func LoadFallbackTraffic(ctx context.Context, conn clickhouse.Conn, database, ortbTable, impressionsTable string, window time.Duration) ([]SegmentTraffic, error) {
	if conn == nil {
		return nil, fmt.Errorf("clickhouse connection is nil")
	}
	if window <= 0 {
		window = FallbackWindow
	}
	database = quoteIdentifier(database)
	ortbTable = quoteIdentifier(ortbTable)
	impressionsTable = quoteIdentifier(impressionsTable)
	seconds := int64(window / time.Second)
	if seconds < 1 {
		seconds = 1
	}

	query := fmt.Sprintf(`
SELECT
    ifNull(o.spp_domain, '') AS ssp_domain,
    ifNull(o.geo, '') AS geo,
    ifNull(o.browser, '') AS browser,
    ifNull(o.device, '') AS device,
    ifNull(o.os, '') AS os,
    ifNull(o.site_id, '') AS site_id,
    o.win_cid AS campaign_id,
    countIf(isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS impressions
FROM %s.%s AS o
LEFT JOIN
(
    SELECT DISTINCT uuid
    FROM %s.%s
    WHERE event_time_impressions >= now64(3) - toIntervalSecond(%d)
) AS i ON o.uuid = i.uuid
WHERE o.event_time >= now64(3) - toIntervalSecond(%d)
  AND o.segment_hash != ''
  AND o.percenter_point_version > 0
  AND o.win_cid != ''
GROUP BY
    ssp_domain,
    geo,
    browser,
    device,
    os,
    site_id,
    campaign_id
SETTINGS join_use_nulls = 1
`, database, ortbTable, database, impressionsTable, seconds, seconds)

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]SegmentTraffic, 0)
	for rows.Next() {
		var item SegmentTraffic
		if err := rows.Scan(
			&item.Segment.SSPDomain,
			&item.Segment.Geo,
			&item.Segment.Browser,
			&item.Segment.Device,
			&item.Segment.OS,
			&item.Segment.SiteID,
			&item.Segment.CampaignID,
			&item.Impressions,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
