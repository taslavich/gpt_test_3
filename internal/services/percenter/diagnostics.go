package percenter

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// LoadSegmentWindowMetrics is the targeted diagnostic equivalent of
// LoadWindowMetrics. It uses exactly the same metric definitions but limits the
// query to one segment and one point_version so analyst requests do not scan and
// aggregate every active segment.
func LoadSegmentWindowMetrics(
	ctx context.Context,
	conn clickhouse.Conn,
	database, ortbTable, impressionsTable, clicksTable string,
	segmentHash string,
	pointVersion uint64,
	window time.Duration,
) (Metrics, bool, error) {
	metric := Metrics{SegmentHash: segmentHash, PointVersion: pointVersion}
	if conn == nil {
		return metric, false, fmt.Errorf("clickhouse connection is nil")
	}
	if segmentHash == "" || pointVersion == 0 {
		return metric, false, nil
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
  AND o.segment_hash = ?
  AND o.percenter_point_version = ?
GROUP BY o.segment_hash, o.percenter_point_version
SETTINGS join_use_nulls = 1
`, database, ortbTable, database, impressionsTable, seconds, database, clicksTable, seconds, seconds)

	rows, err := conn.Query(ctx, query, segmentHash, pointVersion)
	if err != nil {
		return metric, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return metric, false, err
		}
		return metric, false, nil
	}
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
		return metric, false, err
	}
	if err := rows.Err(); err != nil {
		return metric, false, err
	}
	return metric, true, nil
}
