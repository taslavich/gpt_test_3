package percenter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func LoadSimpleWindowMetrics(ctx context.Context, conn clickhouse.Conn, database, ortbTable, impressionsTable, clicksTable string, window time.Duration) ([]SimpleMetrics, error) {
	if conn == nil {
		return nil, fmt.Errorf("clickhouse connection is nil")
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	seconds := int64(window / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	query := fmt.Sprintf(`
SELECT
    any(o.exact_segment_hash) AS exact_segment_hash,
    o.segment_hash,
    o.percenter_point_version,
    count() AS requests,
    countIf(isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS impressions,
    countIf(isNotNull(c.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS clicks,
    sumIf(o.win_dsp_price / 1000.0, isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS advertiser_spend,
    sumIf((o.win_dsp_price - o.win_final_price) / 1000.0, isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS twinbid_profit
FROM %s AS o
LEFT JOIN
(
    SELECT DISTINCT uuid
    FROM %s.%s
    WHERE event_time_impressions >= now64(3) - toIntervalSecond(%d)
) AS i ON o.uuid = i.uuid
LEFT JOIN
(
    SELECT DISTINCT uuid
    FROM %s.%s
    WHERE event_time_clicks >= now64(3) - toIntervalSecond(%d)
) AS c ON o.uuid = c.uuid
WHERE o.event_time >= now64(3) - toIntervalSecond(%d)
  AND o.segment_hash != ''
  AND o.percenter_point_version > 0
GROUP BY o.segment_hash, o.percenter_point_version
SETTINGS join_use_nulls = 1
`, logicalOrtbSource(database, ortbTable, seconds), quoteIdentifier(database), quoteIdentifier(impressionsTable), seconds, quoteIdentifier(database), quoteIdentifier(clicksTable), seconds, seconds)

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SimpleMetrics, 0)
	for rows.Next() {
		var metric SimpleMetrics
		if err := rows.Scan(&metric.ExactSegmentHash, &metric.SegmentHash, &metric.PointVersion, &metric.Requests, &metric.Impressions, &metric.Clicks, &metric.AdvertiserSpend, &metric.TwinBidProfit); err != nil {
			return nil, err
		}
		result = append(result, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// SimpleMetricsIndex indexes each statistics window strictly by the exact
// segment_hash + point_version pair. Statistics from another segment are never
// used to satisfy the Simple statistical guard.
type SimpleMetricsIndex struct {
	byPoint map[simpleMetricsKey]SimpleMetrics
}

type simpleMetricsKey struct {
	segmentHash  string
	pointVersion uint64
}

func NewSimpleMetricsIndex(metrics []SimpleMetrics) SimpleMetricsIndex {
	index := SimpleMetricsIndex{byPoint: make(map[simpleMetricsKey]SimpleMetrics, len(metrics))}
	for _, metric := range metrics {
		if strings.TrimSpace(metric.SegmentHash) == "" || metric.PointVersion == 0 {
			continue
		}
		key := simpleMetricsKey{segmentHash: metric.SegmentHash, pointVersion: metric.PointVersion}
		if current, ok := index.byPoint[key]; ok {
			if current.ExactSegmentHash == "" {
				current.ExactSegmentHash = metric.ExactSegmentHash
			}
			current.Requests += metric.Requests
			current.Impressions += metric.Impressions
			current.Clicks += metric.Clicks
			current.AdvertiserSpend += metric.AdvertiserSpend
			current.TwinBidProfit += metric.TwinBidProfit
			index.byPoint[key] = current
			continue
		}
		index.byPoint[key] = metric
	}
	return index
}

func (i SimpleMetricsIndex) ForState(state SimpleState) (SimpleMetrics, bool) {
	if i.byPoint == nil || strings.TrimSpace(state.SegmentHash) == "" || state.PointVersion == 0 {
		return SimpleMetrics{}, false
	}
	metric, ok := i.byPoint[simpleMetricsKey{segmentHash: state.SegmentHash, pointVersion: state.PointVersion}]
	return metric, ok
}

// logicalOrtbSource deduplicates Kafka replay synchronously at query time.
// The stable logical_event_id is persisted by clickhouse-loader, so optimizer
// decisions never depend on ReplacingMergeTree/background merge timing.
func logicalOrtbSource(database, table string, seconds int64) string {
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf(`(
    SELECT *
    FROM %s.%s
    WHERE event_time >= now64(3) - toIntervalSecond(%d)
    ORDER BY created_at ASC
    LIMIT 1 BY logical_event_id
)`, quoteIdentifier(database), quoteIdentifier(table), seconds)
}

func quoteIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "``"
	}
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}
