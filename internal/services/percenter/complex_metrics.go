package percenter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func LoadComplexWindowMetrics(ctx context.Context, conn clickhouse.Conn, database, ortbTable, impressionsTable string, window time.Duration) ([]ComplexMetrics, error) {
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
    o.segment_hash,
    o.percenter_point_version,
    count() AS requests,
    countIf(isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS impressions,
    sumIf(o.win_dsp_price / 1000.0, isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS advertiser_spend,
    sumIf((o.win_dsp_price - o.win_final_price) / 1000.0, isNotNull(i.uuid) AND ifNull(o.win_dsp_domain, '') = 'adv') AS twinbid_profit
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
GROUP BY o.segment_hash, o.percenter_point_version
SETTINGS join_use_nulls = 1
`, quoteIdentifier(database), quoteIdentifier(ortbTable), quoteIdentifier(database), quoteIdentifier(impressionsTable), seconds, seconds)

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ComplexMetrics, 0)
	for rows.Next() {
		var metric ComplexMetrics
		if err := rows.Scan(&metric.SegmentHash, &metric.PointVersion, &metric.Requests, &metric.Impressions, &metric.AdvertiserSpend, &metric.TwinBidProfit); err != nil {
			return nil, err
		}
		result = append(result, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type ComplexMetricsIndex struct {
	byPoint map[complexMetricsKey]ComplexMetrics
}

type complexMetricsKey struct {
	segmentHash  string
	pointVersion uint64
}

func NewComplexMetricsIndex(metrics []ComplexMetrics) ComplexMetricsIndex {
	index := ComplexMetricsIndex{byPoint: make(map[complexMetricsKey]ComplexMetrics, len(metrics))}
	for _, metric := range metrics {
		if strings.TrimSpace(metric.SegmentHash) == "" || metric.PointVersion == 0 {
			continue
		}
		key := complexMetricsKey{segmentHash: metric.SegmentHash, pointVersion: metric.PointVersion}
		if current, ok := index.byPoint[key]; ok {
			current.Requests += metric.Requests
			current.Impressions += metric.Impressions
			current.AdvertiserSpend += metric.AdvertiserSpend
			current.TwinBidProfit += metric.TwinBidProfit
			index.byPoint[key] = current
			continue
		}
		index.byPoint[key] = metric
	}
	return index
}

func (i ComplexMetricsIndex) ForState(state ComplexState) (ComplexMetrics, bool) {
	if i.byPoint == nil || strings.TrimSpace(state.SegmentHash) == "" || state.PointVersion == 0 {
		return ComplexMetrics{}, false
	}
	metric, ok := i.byPoint[complexMetricsKey{segmentHash: state.SegmentHash, pointVersion: state.PointVersion}]
	return metric, ok
}
