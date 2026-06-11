package clickhouse_loader

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func CreateDB(ctx context.Context, ch clickhouse.Conn, database string) error {
	database = strings.TrimSpace(database)
	if database == "" {
		return fmt.Errorf("clickhouse database name is empty")
	}

	const ddlTemplate = `

CREATE DATABASE IF NOT EXISTS {db};

-- ============================================================
-- ORTB TABLE
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.ortb
(
    uuid              UUID,

    event_time        DateTime64(3, 'UTC'),
    created_at        DateTime64(3, 'UTC') DEFAULT now64(3),

    code              UInt16 DEFAULT 0,

    format            LowCardinality(String),
    typic             LowCardinality(String),

    spp_domain        Nullable(String),

    ip                Nullable(IPv6),
    ipv6              Nullable(IPv6),

    lang              Nullable(String),
    browser           Nullable(String),
    browser_version   Nullable(String),
    os                Nullable(String),
    os_version        Nullable(String),
    device            Nullable(String),

    site_id           Nullable(String),
    site_domain       Nullable(String),

    bid_floor         Float64 DEFAULT 0,

    geo               Nullable(String),
    city_id           Nullable(Int32),

    bid_responses_raw String DEFAULT '',

    win_dsp_domain    Nullable(String),

    win_final_price   Float64 DEFAULT 0,
    win_dsp_price     Float64 DEFAULT 0,

    win_cid           String DEFAULT '',
    win_crid          String DEFAULT '',
    win_user_id       String DEFAULT ''
)
ENGINE = MergeTree
PARTITION BY toStartOfHour(created_at)
ORDER BY (created_at, uuid)
TTL created_at + INTERVAL 1 HOUR DELETE
SETTINGS index_granularity = 8192;


-- ============================================================
-- INPUT TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.impressions_in
(
    event_time_impressions DateTime64(3, 'UTC') DEFAULT now64(3),
    created_at             DateTime64(3, 'UTC') DEFAULT now64(3),

    uuid UUID
)
ENGINE = MergeTree
ORDER BY (event_time_impressions, uuid)
TTL created_at + INTERVAL 1 HOUR DELETE;


CREATE TABLE IF NOT EXISTS {db}.clicks_in
(
    event_time_clicks DateTime64(3, 'UTC') DEFAULT now64(3),
    created_at        DateTime64(3, 'UTC') DEFAULT now64(3),

    uuid UUID
)
ENGINE = MergeTree
ORDER BY (event_time_clicks, uuid)
TTL created_at + INTERVAL 1 HOUR DELETE;


-- ============================================================
-- FACT IMPRESSIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.fact_impressions
(
    event_time_impressions DateTime64(3, 'UTC'),
    event_time             DateTime64(3, 'UTC'),
    event_date             Date,
    event_hour             DateTime('UTC'),

    uuid                   UUID,

    format                 LowCardinality(String),
    typic                  LowCardinality(String),

    spp_domain             Nullable(String),

    ip                     Nullable(IPv6),
    ipv6                   Nullable(IPv6),

    lang                   LowCardinality(String),
    browser                LowCardinality(String),
    browser_version        LowCardinality(String),
    os                     LowCardinality(String),
    os_version             LowCardinality(String),
    device_type            LowCardinality(String),

    site_id                LowCardinality(String),
    site_domain            LowCardinality(String),

    bid_floor              Float64,

    geo                    LowCardinality(String),
    city_id                Nullable(Int32),

    bid_responses_raw      String DEFAULT '',

    win_dsp_domain         LowCardinality(String),

    win_final_price        Float64,
    win_dsp_price          Float64,

    win_cid                String DEFAULT '',
    win_crid               String DEFAULT '',
    win_user_id            String DEFAULT '',

    created_at             DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(event_date)
ORDER BY event_time
TTL event_date + INTERVAL 6 MONTH DELETE
SETTINGS index_granularity = 8192;


-- ============================================================
-- FACT CLICKS
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.fact_clicks
(
    event_time_clicks DateTime64(3, 'UTC'),
    event_time        DateTime64(3, 'UTC'),
    event_date        Date,
    event_hour        DateTime('UTC'),

    uuid              UUID,

    format            LowCardinality(String),
    typic             LowCardinality(String),

    spp_domain        Nullable(String),

    ip                Nullable(IPv6),
    ipv6              Nullable(IPv6),

    lang              LowCardinality(String),
    browser           LowCardinality(String),
    browser_version   LowCardinality(String),
    os                LowCardinality(String),
    os_version        LowCardinality(String),
    device_type       LowCardinality(String),

    site_id           LowCardinality(String),
    site_domain       LowCardinality(String),

    bid_floor         Float64,

    geo               LowCardinality(String),
    city_id           Nullable(Int32),

    bid_responses_raw String DEFAULT '',

    win_dsp_domain    LowCardinality(String),

    win_final_price   Float64,
    win_dsp_price     Float64,

    win_cid           String DEFAULT '',
    win_crid          String DEFAULT '',
    win_user_id       String DEFAULT '',
    created_at        DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(event_date)
ORDER BY event_time
TTL event_date + INTERVAL 6 MONTH DELETE
SETTINGS index_granularity = 8192;


-- ============================================================
-- AGGREGATED STATS
-- browser_version здесь специально НЕ используется
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.agg_stats
(
    win_user_id         String DEFAULT '',
    win_cid             String DEFAULT '',
    win_crid            String DEFAULT '',

    event_date          Date,

    device_type         LowCardinality(String),
    os                  LowCardinality(String),

    event_hour          DateTime('UTC'),

    browser             LowCardinality(String),
    geo                 LowCardinality(String),
    site_id             LowCardinality(String),

    format              LowCardinality(String),
    typic               LowCardinality(String),

    impressions         UInt64,
    clicks              UInt64,

    spend_clicks_table  Float64,
    spend_views_table   Float64
)
ENGINE = SummingMergeTree
PARTITION BY toYYYYMMDD(event_date)
ORDER BY
(
    win_user_id,
    win_cid,
    win_crid,
    event_date,
    device_type,
    os,
    event_hour,
    browser,
    geo,
    site_id,
    format,
    typic
)
SETTINGS index_granularity = 8192;

-- ============================================================
-- ORTB MINUTE METRICS
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.ortb_minute_metrics
(
    minute                  DateTime('UTC'),

    total_ortb              UInt64,

    code_counts             Map(String, UInt64),
    code_percents           Map(String, Float64),

    cnt_clicks_5m           UInt64,
    click_ortb_ratio        Float64,

    cnt_impressions_5m      UInt64,
    impression_ortb_ratio   Float64,

    cnt_ortb_5m             UInt64,

    created_at              DateTime('UTC') DEFAULT now('UTC')
)
ENGINE = ReplacingMergeTree(created_at)
PARTITION BY toYYYYMMDD(minute)
ORDER BY minute
TTL minute + INTERVAL 32 DAY DELETE
SETTINGS index_granularity = 8192;


-- ============================================================
-- MV: IMPRESSIONS INPUT -> FACT IMPRESSIONS
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_impressions_to_fact
REFRESH EVERY 1 MINUTE
APPEND TO {db}.fact_impressions
AS
SELECT
    a.event_time_impressions AS event_time_impressions,
    o.event_time AS event_time,
    toDate(o.event_time) AS event_date,
    toStartOfHour(toDateTime(o.event_time, 'UTC')) AS event_hour,

    o.uuid AS uuid,

    o.format AS format,
    o.typic AS typic,

    o.spp_domain AS spp_domain,

    o.ip AS ip,
    o.ipv6 AS ipv6,

    ifNull(o.lang, '') AS lang,
    ifNull(o.browser, '') AS browser,
    ifNull(o.browser_version, '') AS browser_version,
    ifNull(o.os, '') AS os,
    ifNull(o.os_version, '') AS os_version,
    ifNull(o.device, '') AS device_type,

    ifNull(o.site_id, '') AS site_id,
    ifNull(o.site_domain, '') AS site_domain,

    o.bid_floor AS bid_floor,

    ifNull(o.geo, '') AS geo,
    o.city_id AS city_id,

    o.bid_responses_raw AS bid_responses_raw,

    ifNull(o.win_dsp_domain, '') AS win_dsp_domain,

    o.win_final_price AS win_final_price,
    o.win_dsp_price AS win_dsp_price,

    o.win_cid AS win_cid,
    o.win_crid AS win_crid,
    o.win_user_id AS win_user_id
FROM {db}.impressions_in AS a
ANY INNER JOIN {db}.ortb AS o
    ON a.uuid = o.uuid
WHERE a.uuid NOT IN (
    SELECT uuid
    FROM {db}.fact_impressions
    WHERE created_at >= now() - INTERVAL 6 MINUTE
)
AND a.created_at >= now() - INTERVAL 5 MINUTE;


-- ============================================================
-- REFRESHABLE MV: CLICKS INPUT -> FACT CLICKS
-- Запускается раз в минуту
-- Берёт только uuid, которых нет в fact_clicks за последние 5 минут
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_clicks_to_fact
REFRESH EVERY 1 MINUTE
APPEND TO {db}.fact_clicks
AS
SELECT
    a.event_time_clicks AS event_time_clicks,
    o.event_time AS event_time,
    toDate(o.event_time) AS event_date,
    toStartOfHour(toDateTime(o.event_time, 'UTC')) AS event_hour,

    o.uuid AS uuid,

    o.format AS format,
    o.typic AS typic,

    o.spp_domain AS spp_domain,

    o.ip AS ip,
    o.ipv6 AS ipv6,

    ifNull(o.lang, '') AS lang,
    ifNull(o.browser, '') AS browser,
    ifNull(o.browser_version, '') AS browser_version,
    ifNull(o.os, '') AS os,
    ifNull(o.os_version, '') AS os_version,
    ifNull(o.device, '') AS device_type,

    ifNull(o.site_id, '') AS site_id,
    ifNull(o.site_domain, '') AS site_domain,

    o.bid_floor AS bid_floor,

    ifNull(o.geo, '') AS geo,
    o.city_id AS city_id,

    o.bid_responses_raw AS bid_responses_raw,

    ifNull(o.win_dsp_domain, '') AS win_dsp_domain,

    o.win_final_price AS win_final_price,
    o.win_dsp_price AS win_dsp_price,

    o.win_cid AS win_cid,
    o.win_crid AS win_crid,
    o.win_user_id AS win_user_id
FROM {db}.clicks_in AS a
ANY INNER JOIN {db}.ortb AS o
    ON a.uuid = o.uuid
WHERE a.uuid NOT IN (
    SELECT uuid
    FROM {db}.fact_clicks
    WHERE created_at >= now() - INTERVAL 6 MINUTE
)
AND a.created_at >= now() - INTERVAL 5 MINUTE;
-- ============================================================
-- MV: FACT IMPRESSIONS -> AGG STATS
-- browser_version здесь специально НЕ группируется
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_impressions_to_agg_stats
TO {db}.agg_stats
AS
SELECT
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic,

    count() AS impressions,
    toUInt64(0) AS clicks,

    toFloat64(0) AS spend_clicks_table,

    sum(win_dsp_price / 1000) AS spend_views_table
FROM {db}.fact_impressions
GROUP BY
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic;


-- ============================================================
-- MV: FACT CLICKS -> AGG STATS
-- browser_version здесь специально НЕ группируется
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_clicks_to_agg_stats
TO {db}.agg_stats
AS
SELECT
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic,

    toUInt64(0) AS impressions,
    count() AS clicks,

    CASE 
        WHEN format = 'POP' THEN sum(win_dsp_price) / 1000
        ELSE sum(win_dsp_price)
    END AS spend_clicks_table,
    toFloat64(0) AS spend_views_table
FROM {db}.fact_clicks
GROUP BY
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic;

-- ============================================================
-- REFRESHABLE MV: ORTB MINUTE METRICS
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_ortb_minute_metrics
REFRESH EVERY 1 MINUTE
APPEND TO {db}.ortb_minute_metrics
AS
WITH
    toStartOfMinute(now('UTC')-  INTERVAL 1 MINUTE) AS current_minute,

    -- последняя полностью закрытая минута
    current_minute - INTERVAL 1 MINUTE AS metric_minute,

    -- окно для коэффициентов: последние 5 полных минут без текущей минуты
    current_minute - INTERVAL 6 MINUTE AS ratio_from,
    current_minute - INTERVAL 1 MINUTE AS ratio_to
SELECT
    metric_minute AS minute,

    m.total_ortb AS total_ortb,

    mapFromArrays(m.codes, m.counts) AS code_counts,

    mapFromArrays(
        m.codes,
        arrayMap(
            x -> if(
                m.total_ortb = 0,
                toFloat64(0),
                round(toFloat64(x) / toFloat64(m.total_ortb) * 100, 4)
            ),
            m.counts
        )
    ) AS code_percents,

    c.cnt_clicks_5m AS cnt_clicks_5m,

    if(
        o.cnt_ortb_5m = 0,
        toFloat64(0),
        round(toFloat64(c.cnt_clicks_5m) / toFloat64(o.cnt_ortb_5m), 6)
    ) AS click_ortb_ratio,

    i.cnt_impressions_5m AS cnt_impressions_5m,

    if(
        o.cnt_ortb_5m = 0,
        toFloat64(0),
        round(toFloat64(i.cnt_impressions_5m) / toFloat64(o.cnt_ortb_5m), 6)
    ) AS impression_ortb_ratio,

    o.cnt_ortb_5m AS cnt_ortb_5m,

    now('UTC') AS created_at

FROM
(
    SELECT
        sum(cnt) AS total_ortb,
        groupArray(code_str) AS codes,
        groupArray(cnt) AS counts
    FROM
    (
        SELECT
            toString(code) AS code_str,
            count() AS cnt
        FROM {db}.ortb
        WHERE event_time >= metric_minute
          AND event_time < current_minute
        GROUP BY code_str
    )
) AS m

CROSS JOIN
(
    SELECT count() AS cnt_clicks_5m
    FROM {db}.fact_clicks
    WHERE event_time >= ratio_from
      AND event_time < ratio_to
) AS c

CROSS JOIN
(
    SELECT count() AS cnt_impressions_5m
    FROM {db}.fact_impressions
    WHERE event_time >= ratio_from
      AND event_time < ratio_to
) AS i

CROSS JOIN
(
    SELECT count() AS cnt_ortb_5m
    FROM {db}.ortb
    WHERE event_time >= ratio_from
      AND event_time < ratio_to
) AS o;
`

	ddl := strings.ReplaceAll(ddlTemplate, "{db}", database)
	statements := splitClickHouseStatements(ddl)

	for i, statement := range statements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}

		if err := ch.Exec(ctx, statement); err != nil {
			return fmt.Errorf("failed to execute DDL statement #%d: %w\nSQL:\n%s", i+1, err, statement)
		}
	}

	return nil
}

func splitClickHouseStatements(sql string) []string {
	rawStatements := strings.Split(sql, ";")
	statements := make([]string, 0, len(rawStatements))

	for _, statement := range rawStatements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}

		statements = append(statements, statement)
	}

	return statements
}
