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
-- DROP MATERIALIZED VIEWS BEFORE RECREATION
-- ============================================================

DROP VIEW IF EXISTS {db}.mv_fact_conversions_to_agg_stats SYNC;
DROP VIEW IF EXISTS {db}.mv_fact_clicks_to_agg_stats SYNC;
DROP VIEW IF EXISTS {db}.mv_fact_impressions_to_agg_stats SYNC;

DROP VIEW IF EXISTS {db}.mv_conversions_to_fact SYNC;
DROP VIEW IF EXISTS {db}.mv_clicks_to_fact SYNC;
DROP VIEW IF EXISTS {db}.mv_impressions_to_fact SYNC;

DROP VIEW IF EXISTS {db}.mv_ortb_minute_metrics SYNC;

DROP VIEW IF EXISTS {db}.mv_ip_limit_ipv6 SYNC;
DROP VIEW IF EXISTS {db}.mv_ip_limit_ipv4 SYNC;
DROP VIEW IF EXISTS {db}.mv_user_dsp_price_sum SYNC;
DROP VIEW IF EXISTS ads.mv_ortb_traffic_hourly SYNC;

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

    ip                Nullable(IPv4),
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
-- IP LIMIT IPV4
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.ip_limit_ipv4
(
    ip         String,
    created_at DateTime64(3, 'UTC')
)
ENGINE = MergeTree
ORDER BY (ip, created_at)
TTL created_at + INTERVAL 12 HOUR DELETE
SETTINGS index_granularity = 8192;


CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_ip_limit_ipv4
REFRESH EVERY 1 MINUTE
APPEND TO {db}.ip_limit_ipv4
AS
WITH now64(3, 'UTC') AS batch_created_at
SELECT
    ip,
    batch_created_at AS created_at
FROM
(
    SELECT
        IPv4NumToString(ip) AS ip,
        count() AS cnt
    FROM {db}.ortb
    WHERE ip IS NOT NULL
    GROUP BY ip
    HAVING cnt > 300
)
WHERE ip NOT IN
(
    SELECT ip
    FROM {db}.ip_limit_ipv4
);


-- ============================================================
-- IP LIMIT IPV6
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.ip_limit_ipv6
(
    ip         String,
    created_at DateTime64(3, 'UTC')
)
ENGINE = MergeTree
ORDER BY (ip, created_at)
TTL created_at + INTERVAL 12 HOUR DELETE
SETTINGS index_granularity = 8192;


CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_ip_limit_ipv6
REFRESH EVERY 1 MINUTE
APPEND TO {db}.ip_limit_ipv6
AS
WITH now64(3, 'UTC') AS batch_created_at
SELECT
    ip,
    batch_created_at AS created_at
FROM
(
    SELECT
        IPv6NumToString(ipv6) AS ip,
        count() AS cnt
    FROM {db}.ortb
    WHERE ipv6 IS NOT NULL
    GROUP BY ip
    HAVING cnt > 300
)
WHERE ip NOT IN
(
    SELECT ip
    FROM {db}.ip_limit_ipv6
);


-- ============================================================
-- INPUT TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.impressions_in
(
    event_time_impressions DateTime64(3, 'UTC') DEFAULT now64(3),
    created_at             DateTime64(3, 'UTC') DEFAULT now64(3),

    ad_format                 LowCardinality(String) DEFAULT '',

    uuid UUID,
    impressions_uuid UUID,

    INDEX idx_impressions_in_impressions_uuid impressions_uuid TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
ORDER BY (event_time_impressions, uuid)
TTL created_at + INTERVAL 1 HOUR DELETE;


CREATE TABLE IF NOT EXISTS {db}.clicks_in
(
    event_time_clicks DateTime64(3, 'UTC') DEFAULT now64(3),
    created_at        DateTime64(3, 'UTC') DEFAULT now64(3),

    ad_format            LowCardinality(String) DEFAULT '',

    uuid UUID,
    clicks_uuid UUID,

    INDEX idx_clicks_in_clicks_uuid clicks_uuid TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
ORDER BY (event_time_clicks, uuid)
TTL created_at + INTERVAL 1 HOUR DELETE;

CREATE TABLE IF NOT EXISTS {db}.conversions_in
(
    created_at        DateTime64(3, 'UTC') DEFAULT now64(3),
    conversions_event_time DateTime64(3, 'UTC') DEFAULT now64(3),
    conversions_uuid UUID,
    clicks_uuid UUID,
    payout Float64 DEFAULT 0,
    status LowCardinality(String) DEFAULT '',

    INDEX idx_conversions_in_clicks_uuid clicks_uuid TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
ORDER BY (created_at, clicks_uuid)
TTL created_at + INTERVAL 24 HOUR DELETE;

ALTER TABLE {db}.impressions_in
    ADD COLUMN IF NOT EXISTS impressions_uuid UUID AFTER uuid;

ALTER TABLE {db}.clicks_in
    ADD COLUMN IF NOT EXISTS clicks_uuid UUID AFTER uuid;

ALTER TABLE {db}.impressions_in
    ADD INDEX IF NOT EXISTS idx_impressions_in_impressions_uuid impressions_uuid TYPE bloom_filter(0.01) GRANULARITY 1;

ALTER TABLE {db}.clicks_in
    ADD INDEX IF NOT EXISTS idx_clicks_in_clicks_uuid clicks_uuid TYPE bloom_filter(0.01) GRANULARITY 1;

ALTER TABLE {db}.conversions_in
    ADD COLUMN IF NOT EXISTS conversions_uuid UUID AFTER conversions_event_time;

ALTER TABLE {db}.conversions_in
    ADD COLUMN IF NOT EXISTS conversions_event_time DateTime64(3, 'UTC')
    DEFAULT now64(3)
    AFTER created_at;

ALTER TABLE {db}.conversions_in
    ADD COLUMN IF NOT EXISTS status LowCardinality(String)
    DEFAULT ''
    AFTER payout;


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
    impressions_uuid UUID,
    code                   UInt16 DEFAULT 0,

    format                 LowCardinality(String),
    typic                  LowCardinality(String),

    spp_domain             Nullable(String),

    ip                     Nullable(IPv4),
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
    clicks_uuid       UUID,
    code              UInt16 DEFAULT 0,

    format            LowCardinality(String),
    typic             LowCardinality(String),

    spp_domain        Nullable(String),

    ip                Nullable(IPv4),
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


CREATE TABLE IF NOT EXISTS {db}.fact_conversions
(
    conversions_event_time DateTime64(3, 'UTC'),
    event_time        DateTime64(3, 'UTC'),
    event_date        Date,
    event_hour        DateTime('UTC'),

    uuid              UUID,
    clicks_uuid       UUID,
    code              UInt16 DEFAULT 0,

    format            LowCardinality(String),
    typic             LowCardinality(String),

    spp_domain        Nullable(String),

    ip                Nullable(IPv4),
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

    payout            Float64 DEFAULT 0,
    status            LowCardinality(String) DEFAULT '',
    approved          UInt8 DEFAULT 0,
    created_at        DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(event_date)
ORDER BY event_time
TTL event_date + INTERVAL 6 MONTH DELETE
SETTINGS index_granularity = 8192;

ALTER TABLE {db}.fact_impressions
    ADD COLUMN IF NOT EXISTS impressions_uuid UUID AFTER uuid;

ALTER TABLE {db}.fact_clicks
    ADD COLUMN IF NOT EXISTS clicks_uuid UUID AFTER uuid;

ALTER TABLE {db}.fact_conversions
    ADD COLUMN IF NOT EXISTS conversions_event_time DateTime64(3, 'UTC')
    DEFAULT now64(3);

ALTER TABLE {db}.fact_conversions
    ADD COLUMN IF NOT EXISTS status LowCardinality(String)
    DEFAULT ''
    AFTER payout;

ALTER TABLE {db}.fact_conversions
    ADD COLUMN IF NOT EXISTS approved UInt8
    DEFAULT 0
    AFTER status;


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
    conversions         UInt64,
    payout Float64,

    conversions_approved UInt64,
    payout_approved Float64,

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

ALTER TABLE {db}.agg_stats
    ADD COLUMN IF NOT EXISTS conversions UInt64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS payout Float64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS conversions_approved UInt64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS payout_approved Float64 DEFAULT 0;


CREATE TABLE IF NOT EXISTS {db}.user_dsp_price_sum
(
    created_at DateTime DEFAULT now(),
    user_id String,
    sum_cum_per_period Float64
)
ENGINE = ReplacingMergeTree(created_at)
PARTITION BY toYYYYMMDD(created_at)
ORDER BY (created_at, user_id)
TTL created_at + INTERVAL 24 HOUR DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW {db}.mv_user_dsp_price_sum
REFRESH EVERY 1 MINUTE
APPEND TO {db}.user_dsp_price_sum
AS
WITH now('UTC') AS batch_created_at
SELECT
    batch_created_at AS created_at,
    user_id,
    sum(spend) AS sum_cum_per_period
FROM
(
    /* NAT, BAN, POP оплачиваются по показам: CPM / 1000 */
    SELECT
        argMax(win_user_id, created_at) AS user_id,
        argMax(win_dsp_price, created_at) / 1000 AS spend
    FROM {db}.fact_impressions
    WHERE
        created_at >= batch_created_at - INTERVAL 5 MINUTE
        AND created_at < batch_created_at
        AND format IN ('NAT', 'BAN', 'POP')
        AND notEmpty(trimBoth(win_user_id))
    GROUP BY impressions_uuid

    UNION ALL

    /* IPP оплачивается по кликам: CPC без деления */
    SELECT
        argMax(win_user_id, created_at) AS user_id,
        argMax(win_dsp_price, created_at) AS spend
    FROM {db}.fact_clicks
    WHERE
        created_at >= batch_created_at - INTERVAL 5 MINUTE
        AND created_at < batch_created_at
        AND format = 'IPP'
        AND notEmpty(trimBoth(win_user_id))
    GROUP BY clicks_uuid

    UNION ALL

    /*
       Техническая строка нужна, чтобы новый batch создавался,
       даже если за последние 5 минут вообще не было событий.
    */
    SELECT
        '' AS user_id,
        toFloat64(0) AS spend
)
GROUP BY user_id;

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

CREATE TABLE IF NOT EXISTS {db}.traffic_volume_hourly
(
    event_hour DateTime('UTC'),

    format LowCardinality(String),
    typic LowCardinality(String),
    geo LowCardinality(String),
    lang LowCardinality(String),
    device LowCardinality(String),
    os LowCardinality(String),
    browser LowCardinality(String),
    site_id LowCardinality(String),

    requests UInt64,

    nonzero_win_dsp_price_sum Float64,
    nonzero_win_dsp_price_count UInt64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_hour)
ORDER BY
(
    event_hour,
    format,
    typic,
    geo,
    lang,
    device,
    os,
    browser,
    site_id
)
TTL event_hour + INTERVAL 10 DAY DELETE
SETTINGS index_granularity = 8192;


CREATE MATERIALIZED VIEW {db}.mv_ortb_traffic_hourly
REFRESH EVERY 1 HOUR OFFSET 1 MINUTE
APPEND TO {db}.traffic_volume_hourly
EMPTY
AS
WITH
    toStartOfHour(now('UTC')) AS current_hour,
    current_hour - INTERVAL 1 HOUR AS previous_hour
SELECT
    event_hour,

    format,
    typic,
    geo,
    lang,
    device,
    os,
    browser,
    site_id,
    count() AS requests,

    sumIf(
        win_dsp_price,
        win_dsp_price > 0
    ) AS nonzero_win_dsp_price_sum,

    countIf(
        win_dsp_price > 0
    ) AS nonzero_win_dsp_price_count

FROM
(
    SELECT
        toStartOfHour(toDateTime(event_time, 'UTC')) AS event_hour,

        format,
        typic,

        ifNull(geo, '') AS geo,
        ifNull(lang, '') AS lang,
        ifNull(device, '') AS device,
        ifNull(os, '') AS os,
        ifNull(browser, '') AS browser,
        ifNull(toString(site_id), '') AS site_id,

        toFloat64(ifNull(win_dsp_price, 0)) AS win_dsp_price

    FROM {db}.ortb

    WHERE
        toStartOfHour(toDateTime(event_time, 'UTC')) = previous_hour
)
GROUP BY
    event_hour,
    format,
    typic,
    geo,
    lang,
    device,
    os,
    browser,
    site_id;


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
    a.impressions_uuid AS impressions_uuid,
    o.code AS code,

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
WHERE a.impressions_uuid NOT IN (
    SELECT impressions_uuid
    FROM {db}.fact_impressions
    WHERE created_at >= now() - INTERVAL 65 MINUTE
)
AND a.created_at >= now() - INTERVAL 60 MINUTE;


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
    a.clicks_uuid AS clicks_uuid,
    o.code AS code,

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
WHERE a.clicks_uuid NOT IN (
    SELECT clicks_uuid
    FROM {db}.fact_clicks
    WHERE created_at >= now() - INTERVAL 65 MINUTE
)
AND a.created_at >= now() - INTERVAL 60 MINUTE;

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_conversions_to_fact
REFRESH EVERY 1 MINUTE
APPEND TO {db}.fact_conversions
AS
SELECT
    a.conversions_event_time AS conversions_event_time,
    o.event_time AS event_time,
    toDate(o.event_time) AS event_date,
    toStartOfHour(toDateTime(o.event_time, 'UTC')) AS event_hour,

    o.uuid AS uuid,
    a.clicks_uuid AS clicks_uuid,
    o.code AS code,

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
    ifNull(o.device_type, '') AS device_type,

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
    o.win_user_id AS win_user_id,
    a.payout AS payout,
    a.status AS status,
    a.approved AS approved
FROM
(
    SELECT
        created_at,
        conversions_event_time,
        clicks_uuid,
        payout,

        upperUTF8(ifNull(status, '')) AS status,

        toUInt8(
            upperUTF8(ifNull(status, '')) = 'APPROVED'
        ) AS approved

    FROM {db}.conversions_in
) AS a
ANY INNER JOIN {db}.fact_clicks AS o
    ON a.clicks_uuid = o.clicks_uuid
WHERE a.status IN ('', 'PENDING', 'APPROVED')
  AND a.created_at >= now() - toIntervalMinute(1440)
  AND (
      a.clicks_uuid,
      a.conversions_event_time,
      a.approved
  ) NOT IN
  (
      SELECT
          clicks_uuid,
          conversions_event_time,
          approved
      FROM {db}.fact_conversions
      WHERE created_at >= now() - toIntervalMinute(1445)
  );
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

    toUInt64(0)  as conversions,
    toFloat64(0) AS payout,
    toUInt64(0) AS conversions_approved,
    toFloat64(0) AS payout_approved,

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

    toUInt64(0)  as conversions,
    toFloat64(0) AS payout,
    toUInt64(0) AS conversions_approved,
    toFloat64(0) AS payout_approved,

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

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_conversions_to_agg_stats
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
    toUInt64(0) AS clicks,

    toUInt64(countIf(approved != 1)) AS conversions,
    toFloat64(sumIf(conversions_payout, approved != 1)) AS payout,

    toUInt64(countIf(approved = 1)) AS conversions_approved,
    toFloat64(sumIf(conversions_payout, approved = 1)) AS payout_approved,

    toFloat64(0) AS spend_clicks_table,
    toFloat64(0) AS spend_views_table
FROM
(
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
        approved,
        payout AS conversions_payout
    FROM {db}.fact_conversions
) AS source
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
