package clickhouse_loader

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
)

const ddlTemplate = `

CREATE DATABASE IF NOT EXISTS {db};

-- Stop every POP recovery writer before any migration work starts.  This is
-- intentionally the first cut-over action: the legacy view cannot race schema
-- preparation and append an unmarked recovery row, while the transition
-- catch-up below covers the short recovery-off interval.
DROP VIEW IF EXISTS {db}.mv_recover_pop_impressions SYNC;
DROP VIEW IF EXISTS {db}.mv_recover_pop_impressions_v2 SYNC;
-- Stop the legacy cumulative recovery-spend writer as part of the same cut-over.
-- Existing agg_stats.pop_recovered_spend data is left intact for rollback/audit,
-- but no new recovery spend is appended there after this migration.
DROP VIEW IF EXISTS {db}.mv_fact_pop_recovered_to_agg_stats SYNC;

-- ============================================================
-- DROP MATERIALIZED VIEWS BEFORE RECREATION
-- ============================================================

-- Stop the fact producer BEFORE disconnecting its registry consumer. Resume it
-- only at the very end, so routine Loader restart cannot create a registry gap.
DROP VIEW IF EXISTS {db}.mv_clicks_to_fact SYNC;
DROP VIEW IF EXISTS {db}.mv_publisher_pop_revenue_checkpoint SYNC;
DROP VIEW IF EXISTS {db}.mv_publisher_pop_revenue_to_agg_stats SYNC;
DROP VIEW IF EXISTS {db}.mv_fact_clicks_to_publisher_pop_revenue SYNC;
DROP VIEW IF EXISTS {db}.mv_fact_conversions_to_agg_stats SYNC;
DROP VIEW IF EXISTS {db}.mv_fact_clicks_to_agg_stats SYNC;
DROP VIEW IF EXISTS {db}.mv_fact_clicks_wins_to_agg_stats SYNC;
-- Keep mv_fact_impressions_to_agg_stats alive while POP recovery is replaced.
-- The recovery writers were stopped before migration work, and the transition
-- catch-up must still flow through the ordinary impression spend/statistics MV.

DROP VIEW IF EXISTS {db}.mv_conversions_to_fact SYNC;
DROP VIEW IF EXISTS {db}.mv_clicks_to_fact SYNC;
DROP VIEW IF EXISTS {db}.mv_clicks_wins_to_fact SYNC;
DROP VIEW IF EXISTS {db}.mv_impressions_to_fact SYNC;

DROP VIEW IF EXISTS {db}.mv_ortb_minute_metrics SYNC;

DROP VIEW IF EXISTS {db}.mv_ip_limit_ipv6 SYNC;
DROP VIEW IF EXISTS {db}.mv_ip_limit_ipv4 SYNC;
DROP VIEW IF EXISTS {db}.mv_user_dsp_price_sum SYNC;
DROP VIEW IF EXISTS {db}.mv_ortb_traffic_hourly SYNC;
DROP VIEW IF EXISTS {db}.mv_campaign_dsp_price_sum SYNC;
DROP VIEW IF EXISTS {db}.mv_recover_clicks_from_clicks_wins SYNC;
DROP VIEW IF EXISTS {db}.mv_ortb_to_publisher_inventory SYNC;

-- ============================================================
-- ORTB TABLE
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.ortb
(
    uuid              UUID,
    logical_event_id  String DEFAULT toString(uuid),

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
    adv_rtb_responses_raw String DEFAULT '',

    win_dsp_domain    Nullable(String),

    win_final_price   Float64 DEFAULT 0,
    win_dsp_price     Float64 DEFAULT 0,

    win_cid           String DEFAULT '',
    win_crid          String DEFAULT '',
    win_user_id       String DEFAULT '',
    pub_user_id String DEFAULT '',
    ad_code_id String DEFAULT '',
    zone_id String DEFAULT '',
    request_id String DEFAULT '',
    site_page String DEFAULT '',
    request_error UInt8 DEFAULT 0,
    error_reason String DEFAULT '',

    exact_segment_hash       String DEFAULT '',
    segment_hash             String DEFAULT '',
    percenter_point_version  UInt64 DEFAULT 0
)
ENGINE = MergeTree
PARTITION BY toStartOfHour(created_at)
ORDER BY (created_at, uuid)
TTL created_at + INTERVAL 1 HOUR DELETE
SETTINGS index_granularity = 8192;

-- Lookup by auction UUID for clicks_wins -> ORTB enrichment.
-- ALTER is kept for compatibility with already existing tables.
ALTER TABLE {db}.ortb
    ADD COLUMN IF NOT EXISTS logical_event_id String DEFAULT toString(uuid) AFTER uuid;

ALTER TABLE {db}.ortb
    ADD COLUMN IF NOT EXISTS adv_rtb_responses_raw String DEFAULT '' AFTER bid_responses_raw;

ALTER TABLE {db}.ortb
    ADD COLUMN IF NOT EXISTS exact_segment_hash String DEFAULT '' AFTER win_user_id;

ALTER TABLE {db}.ortb
    ADD COLUMN IF NOT EXISTS segment_hash String DEFAULT '' AFTER exact_segment_hash;

ALTER TABLE {db}.ortb
    ADD COLUMN IF NOT EXISTS percenter_point_version UInt64 DEFAULT 0 AFTER segment_hash;

ALTER TABLE {db}.ortb
    ADD INDEX IF NOT EXISTS idx_ortb_uuid
    uuid TYPE bloom_filter(0.01) GRANULARITY 1;

-- Kafka offset commit happens after the ClickHouse insert, so a process crash
-- can replay the same ORTB message. Keep the physical append-only table, but
-- expose one row per stable logical event synchronously at read time; unlike a
-- ReplacingMergeTree-only solution this does not wait for a background merge.
ALTER TABLE {db}.ortb ADD COLUMN IF NOT EXISTS pub_user_id String DEFAULT '';
ALTER TABLE {db}.ortb ADD COLUMN IF NOT EXISTS ad_code_id String DEFAULT '';
ALTER TABLE {db}.ortb ADD COLUMN IF NOT EXISTS zone_id String DEFAULT '';
ALTER TABLE {db}.ortb ADD COLUMN IF NOT EXISTS request_id String DEFAULT '';
ALTER TABLE {db}.ortb ADD COLUMN IF NOT EXISTS site_page String DEFAULT '';
ALTER TABLE {db}.ortb ADD COLUMN IF NOT EXISTS request_error UInt8 DEFAULT 0;
ALTER TABLE {db}.ortb ADD COLUMN IF NOT EXISTS error_reason String DEFAULT '';


CREATE VIEW {db}.ortb_logical AS
SELECT *
FROM {db}.ortb
ORDER BY created_at ASC
LIMIT 1 BY logical_event_id;


-- ============================================================
-- PERCENTER OBSERVABILITY / DECISION HISTORY
-- ============================================================

-- Durable percenter decision history. event_id is stable across retries.
-- ReplacingMergeTree + FINAL logical view make physical replay logically
-- idempotent without relying on background merge timing.

-- Lifetime discovery only; min/max are idempotent under retries. This does not
-- assign Site IDs and does not keep wide auction payloads or compute revenue.
CREATE TABLE IF NOT EXISTS {db}.publisher_inventory
(
 pub_user_id String,site_id String,ad_code_id String,zone_id String,hostname String,
 first_request_at SimpleAggregateFunction(min,DateTime64(3,'UTC')),
 last_request_at SimpleAggregateFunction(max,DateTime64(3,'UTC'))
)
ENGINE=AggregatingMergeTree
ORDER BY (pub_user_id,site_id,ad_code_id,zone_id,hostname);
CREATE MATERIALIZED VIEW {db}.mv_ortb_to_publisher_inventory TO {db}.publisher_inventory AS
SELECT pub_user_id,ifNull(site_id,'') AS site_id,ad_code_id,zone_id,
 ifNull(site_domain,'') AS hostname,min(event_time) AS first_request_at,max(event_time) AS last_request_at
FROM {db}.ortb WHERE pub_user_id!='' AND site_id!='' AND zone_id!=''
GROUP BY pub_user_id,site_id,ad_code_id,zone_id,hostname;

CREATE TABLE IF NOT EXISTS {db}.percenter_state_history
(
    event_id String,
    timestamp DateTime64(3, 'UTC'),
    bucket DateTime64(3, 'UTC'),
    campaign_id String,
    exact_segment_hash String,
    segment_hash String,
    type_model UInt8,
    phase LowCardinality(String),
    point_version UInt64,
    original_bid Float64,
    advertiser_price Float64,
    ssp_bid Float64,
    margin Float64,
    effective_min Float64,
    map_source LowCardinality(String),
    baseline_buyout Float64,
    baseline_winrate Float64,
    baseline_efficiency Float64,
    requests UInt64,
    impressions UInt64,
    wins UInt64,
    clicks UInt64,
    advertiser_spend Float64,
    revenue Float64,
    buyout Float64,
    winrate Float64,
    efficiency Float64,
    profit Float64,
    profit_per_relevant_opportunity Float64,
    previous_ssp_bid Float64,
    previous_margin Float64,
    candidate_ssp_bid Float64,
    candidate_margin Float64,
    result_ssp_bid Float64,
    result_margin Float64,
    decision LowCardinality(String),
    reason String,
    step Float64,
    threshold_passed Bool,
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(inserted_at)
ORDER BY event_id;

-- Compatibility with installations that already have an older
-- percenter_state_history schema. CREATE TABLE IF NOT EXISTS does not evolve an
-- existing table, so explicitly add every column used by the current history
-- writer that may be absent. Do not use AFTER here: schema-drifted installations
-- may not have the referenced predecessor column yet, which makes ClickHouse
-- reject an otherwise idempotent ADD COLUMN IF NOT EXISTS migration.
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS timestamp DateTime64(3, 'UTC');
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS bucket DateTime64(3, 'UTC');
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS campaign_id String;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS exact_segment_hash String;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS segment_hash String;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS type_model UInt8;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS phase LowCardinality(String);
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS point_version UInt64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS original_bid Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS advertiser_price Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS ssp_bid Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS margin Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS effective_min Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS map_source LowCardinality(String);
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS baseline_buyout Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS baseline_winrate Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS baseline_efficiency Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS requests UInt64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS impressions UInt64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS wins UInt64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS clicks UInt64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS advertiser_spend Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS revenue Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS buyout Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS winrate Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS efficiency Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS profit Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS profit_per_relevant_opportunity Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS previous_ssp_bid Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS previous_margin Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS candidate_ssp_bid Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS candidate_margin Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS result_ssp_bid Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS result_margin Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS decision LowCardinality(String);
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS reason String;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS step Float64;
ALTER TABLE {db}.percenter_state_history
    ADD COLUMN IF NOT EXISTS threshold_passed Bool;

CREATE TABLE IF NOT EXISTS {db}.percenter_telemetry
(
    event_id String,
    bucket DateTime64(3, 'UTC'),
    service LowCardinality(String),
    instance String,
    campaign_id String,
    exact_segment_hash String,
    segment_hash String,
    type_model UInt8,
    point_version UInt64,
    counter LowCardinality(String),
    value UInt64,
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(inserted_at)
ORDER BY event_id;

-- Apply the same schema-drift protection to telemetry. These statements are
-- metadata-only when the columns already exist and let an older installation
-- converge to the writer contract without dropping telemetry data.
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS bucket DateTime64(3, 'UTC');
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS service LowCardinality(String);
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS instance String;
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS campaign_id String;
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS exact_segment_hash String;
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS segment_hash String;
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS type_model UInt8;
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS point_version UInt64;
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS counter LowCardinality(String);
ALTER TABLE {db}.percenter_telemetry
    ADD COLUMN IF NOT EXISTS value UInt64;

-- Analytics must read logical views (or otherwise deduplicate by event_id),
-- so retries/crash replays cannot double-apply one logical observation.
CREATE VIEW IF NOT EXISTS {db}.percenter_state_history_logical AS
SELECT *
FROM {db}.percenter_state_history FINAL;

CREATE VIEW IF NOT EXISTS {db}.percenter_telemetry_logical AS
SELECT *
FROM {db}.percenter_telemetry FINAL;


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
    FROM {db}.ortb_logical
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
    FROM {db}.ortb_logical
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
    is_iframe            Nullable(UInt8) DEFAULT NULL,

    uuid UUID,
    clicks_uuid UUID,

    INDEX idx_clicks_in_clicks_uuid clicks_uuid TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
ORDER BY (event_time_clicks, uuid)
TTL created_at + INTERVAL 1 HOUR DELETE;

CREATE TABLE IF NOT EXISTS {db}.clicks_wins_in
(
    event_time_clicks_wins DateTime64(3, 'UTC') DEFAULT now64(3),
    created_at             DateTime64(3, 'UTC') DEFAULT now64(3),

    uuid UUID,
    clicks_wins_uuid UUID,

    INDEX idx_clicks_wins_in_clicks_wins_uuid clicks_wins_uuid TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
ORDER BY (event_time_clicks_wins, uuid)
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

ALTER TABLE {db}.clicks_in
    ADD COLUMN IF NOT EXISTS is_iframe Nullable(UInt8) DEFAULT NULL AFTER ad_format;

ALTER TABLE {db}.clicks_wins_in
    ADD COLUMN IF NOT EXISTS clicks_wins_uuid UUID AFTER uuid;

ALTER TABLE {db}.impressions_in
    ADD INDEX IF NOT EXISTS idx_impressions_in_impressions_uuid impressions_uuid TYPE bloom_filter(0.01) GRANULARITY 1;

ALTER TABLE {db}.clicks_in
    ADD INDEX IF NOT EXISTS idx_clicks_in_clicks_uuid clicks_uuid TYPE bloom_filter(0.01) GRANULARITY 1;

ALTER TABLE {db}.clicks_wins_in
    ADD INDEX IF NOT EXISTS idx_clicks_wins_in_clicks_wins_uuid clicks_wins_uuid TYPE bloom_filter(0.01) GRANULARITY 1;

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
    adv_rtb_responses_raw      String DEFAULT '',

    win_dsp_domain         LowCardinality(String),

    win_final_price        Float64,
    win_dsp_price          Float64,

    win_cid                String DEFAULT '',
    win_crid               String DEFAULT '',
    win_user_id            String DEFAULT '',
    pub_user_id String DEFAULT '',
    ad_code_id String DEFAULT '',
    zone_id String DEFAULT '',

    is_pop_recovered       UInt8 DEFAULT 0,
    pop_recovered_at       Nullable(DateTime64(3, 'UTC')) DEFAULT NULL,
    pop_recovery_key       String DEFAULT '',
    pop_recovery_source_key String DEFAULT '',

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
    is_iframe         Nullable(UInt8) DEFAULT NULL,
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
    adv_rtb_responses_raw String DEFAULT '',

    win_dsp_domain    LowCardinality(String),

    win_final_price   Float64,
    win_dsp_price     Float64,

    win_cid           String DEFAULT '',
    win_crid          String DEFAULT '',
    win_user_id       String DEFAULT '',
    pub_user_id String DEFAULT '',
    ad_code_id String DEFAULT '',
    zone_id String DEFAULT '',
    created_at        DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(event_date)
ORDER BY event_time
TTL event_date + INTERVAL 6 MONTH DELETE
SETTINGS index_granularity = 8192;


-- ============================================================
-- FACT CLICKS WINS
-- ============================================================

CREATE TABLE IF NOT EXISTS {db}.fact_clicks_wins
(
    event_time_clicks_wins DateTime64(3, 'UTC'),
    event_time             DateTime64(3, 'UTC'),
    event_date             Date,
    event_hour             DateTime('UTC'),

    uuid                   UUID,
    clicks_wins_uuid       UUID,
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
    adv_rtb_responses_raw      String DEFAULT '',

    win_dsp_domain         LowCardinality(String),

    win_final_price        Float64,
    win_dsp_price          Float64,

    win_cid                String DEFAULT '',
    win_crid               String DEFAULT '',
    win_user_id            String DEFAULT '',
    pub_user_id String DEFAULT '',
    ad_code_id String DEFAULT '',
    zone_id String DEFAULT '',
    created_at             DateTime64(3, 'UTC') DEFAULT now64(3)
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
    adv_rtb_responses_raw String DEFAULT '',

    win_dsp_domain    LowCardinality(String),

    win_final_price   Float64,
    win_dsp_price     Float64,

    win_cid           String DEFAULT '',
    win_crid          String DEFAULT '',
    win_user_id       String DEFAULT '',
    pub_user_id String DEFAULT '',
    ad_code_id String DEFAULT '',
    zone_id String DEFAULT '',

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

ALTER TABLE {db}.fact_impressions
    ADD COLUMN IF NOT EXISTS is_pop_recovered UInt8 DEFAULT 0 AFTER win_user_id;

ALTER TABLE {db}.fact_impressions
    ADD COLUMN IF NOT EXISTS pop_recovered_at Nullable(DateTime64(3, 'UTC')) DEFAULT NULL AFTER is_pop_recovered;

ALTER TABLE {db}.fact_impressions
    ADD COLUMN IF NOT EXISTS pop_recovery_key String DEFAULT '' AFTER pop_recovered_at;

ALTER TABLE {db}.fact_impressions
    ADD COLUMN IF NOT EXISTS pop_recovery_source_key String DEFAULT '' AFTER pop_recovery_key;

ALTER TABLE {db}.fact_clicks
    ADD COLUMN IF NOT EXISTS clicks_uuid UUID AFTER uuid;

ALTER TABLE {db}.fact_clicks
    ADD COLUMN IF NOT EXISTS is_iframe Nullable(UInt8) DEFAULT NULL AFTER clicks_uuid;

ALTER TABLE {db}.fact_clicks_wins
    ADD COLUMN IF NOT EXISTS clicks_wins_uuid UUID AFTER uuid;

ALTER TABLE {db}.fact_impressions
    ADD COLUMN IF NOT EXISTS adv_rtb_responses_raw String DEFAULT '' AFTER bid_responses_raw;

ALTER TABLE {db}.fact_clicks
    ADD COLUMN IF NOT EXISTS adv_rtb_responses_raw String DEFAULT '' AFTER bid_responses_raw;

ALTER TABLE {db}.fact_clicks_wins
    ADD COLUMN IF NOT EXISTS adv_rtb_responses_raw String DEFAULT '' AFTER bid_responses_raw;

ALTER TABLE {db}.fact_conversions
    ADD COLUMN IF NOT EXISTS adv_rtb_responses_raw String DEFAULT '' AFTER bid_responses_raw;

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

-- Lookup indexes used by the corrected refreshable MVs.
-- Existing parts require a one-time MATERIALIZE INDEX command
-- new parts receive these indexes automatically.
ALTER TABLE {db}.fact_clicks
    ADD INDEX IF NOT EXISTS idx_fact_clicks_clicks_uuid
    clicks_uuid TYPE bloom_filter(0.01) GRANULARITY 1;

ALTER TABLE {db}.fact_clicks_wins
    ADD INDEX IF NOT EXISTS idx_fact_clicks_wins_created_at
    created_at TYPE minmax GRANULARITY 1;

ALTER TABLE {db}.fact_conversions
    ADD INDEX IF NOT EXISTS idx_fact_conversions_created_at
    created_at TYPE minmax GRANULARITY 1;


-- ============================================================
-- AGGREGATED STATS
-- browser_version здесь специально НЕ используется
-- ============================================================


-- Publisher attribution: backwards-compatible defaults.
ALTER TABLE {db}.fact_impressions ADD COLUMN IF NOT EXISTS pub_user_id String DEFAULT '';
ALTER TABLE {db}.fact_impressions ADD COLUMN IF NOT EXISTS ad_code_id String DEFAULT '';
ALTER TABLE {db}.fact_impressions ADD COLUMN IF NOT EXISTS zone_id String DEFAULT '';
ALTER TABLE {db}.fact_clicks ADD COLUMN IF NOT EXISTS pub_user_id String DEFAULT '';
ALTER TABLE {db}.fact_clicks ADD COLUMN IF NOT EXISTS ad_code_id String DEFAULT '';
ALTER TABLE {db}.fact_clicks ADD COLUMN IF NOT EXISTS zone_id String DEFAULT '';
ALTER TABLE {db}.fact_clicks_wins ADD COLUMN IF NOT EXISTS pub_user_id String DEFAULT '';
ALTER TABLE {db}.fact_clicks_wins ADD COLUMN IF NOT EXISTS ad_code_id String DEFAULT '';
ALTER TABLE {db}.fact_clicks_wins ADD COLUMN IF NOT EXISTS zone_id String DEFAULT '';
ALTER TABLE {db}.fact_conversions ADD COLUMN IF NOT EXISTS pub_user_id String DEFAULT '';
ALTER TABLE {db}.fact_conversions ADD COLUMN IF NOT EXISTS ad_code_id String DEFAULT '';
ALTER TABLE {db}.fact_conversions ADD COLUMN IF NOT EXISTS zone_id String DEFAULT '';

CREATE TABLE IF NOT EXISTS {db}.agg_stats
(
    win_user_id         String DEFAULT '',
    win_cid             String DEFAULT '',
    win_crid            String DEFAULT '',

    event_date          Date,

    device_type         LowCardinality(String),
    os                  LowCardinality(String),
    os_version          LowCardinality(String),

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
    spend_views_table   Float64,
    pop_recovered_spend Float64 DEFAULT 0,
    pub_user_id String DEFAULT '',
    ad_code_id String DEFAULT '',
    zone_id String DEFAULT '',
    ssp_revenue_views Float64 DEFAULT 0,
    pop_revenue_bucket UInt64 DEFAULT 0
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
    typic,
    os_version,
    pub_user_id,
    ad_code_id,
    zone_id,
    pop_revenue_bucket
)
SETTINGS index_granularity = 8192;

-- os_version must be added to the SummingMergeTree sorting key in the same
-- ALTER that adds the column. ClickHouse only permits extending an existing
-- sorting key with a newly added column; keeping both operations atomic also
-- avoids a partial migration where the column exists but is not in ORDER BY.
-- Normal/recovery fact writers must be paused for deployment; no historical
-- advertiser aggregate is replayed for this schema migration.
DROP VIEW IF EXISTS {db}.mv_fact_impressions_to_agg_stats SYNC;
ALTER TABLE {db}.agg_stats
    ADD COLUMN IF NOT EXISTS conversions UInt64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS payout Float64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS conversions_approved UInt64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS payout_approved Float64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pop_recovered_spend Float64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS os_version LowCardinality(String),
    ADD COLUMN IF NOT EXISTS pub_user_id String,
    ADD COLUMN IF NOT EXISTS ad_code_id String,
    ADD COLUMN IF NOT EXISTS zone_id String,
    ADD COLUMN IF NOT EXISTS ssp_revenue_views Float64 DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pop_revenue_bucket UInt64,
    MODIFY ORDER BY
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
        typic,
        os_version,
        pub_user_id,
        ad_code_id,
        zone_id,
        pop_revenue_bucket
    );

-- ADD+ORDER BY requires no DEFAULT on newly added key columns. Set their
-- explicit empty defaults only AFTER the atomic key extension.
ALTER TABLE {db}.agg_stats
    MODIFY COLUMN pub_user_id String DEFAULT '',
    MODIFY COLUMN ad_code_id String DEFAULT '',
    MODIFY COLUMN zone_id String DEFAULT '',
    MODIFY COLUMN pop_revenue_bucket UInt64 DEFAULT 0;

-- ============================================================
-- No historical IPP DELETE/INSERT cutover runs from ClickHouse schema DDL.
-- Existing billing is authoritative and unchanged by this Publisher upgrade.

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
REFRESH EVERY 1 MINUTE OFFSET 5 SECOND
APPEND TO {db}.user_dsp_price_sum
AS
WITH now('UTC') AS batch_created_at
SELECT
    batch_created_at AS created_at,
    user_id,
    sum(spend) AS sum_cum_per_period
FROM
(
    /* NAT, BAN, POP, VID оплачиваются по показам: CPM / 1000 */
    SELECT
        argMax(win_user_id, created_at) AS user_id,
        argMax(win_dsp_price, created_at) / 1000 AS spend
    FROM {db}.fact_impressions
    WHERE
        created_at >= batch_created_at - INTERVAL 1 MINUTE
        AND created_at < batch_created_at
        AND format IN ('NAT', 'BAN', 'POP', 'VID')
        AND notEmpty(trimBoth(win_user_id))
    GROUP BY impressions_uuid

    UNION ALL

    /* IPP оплачивается только по подтверждённым clicks_wins: CPC без деления */
    SELECT
        argMax(win_user_id, created_at) AS user_id,
        argMax(win_dsp_price, created_at) AS spend
    FROM {db}.fact_clicks_wins
    WHERE
        created_at >= batch_created_at - INTERVAL 1 MINUTE
        AND created_at < batch_created_at
        AND format = 'IPP'
        AND notEmpty(trimBoth(win_user_id))
    GROUP BY clicks_wins_uuid

    UNION ALL

    /*
       Техническая строка отмечает последний завершённый batch.
       ADV считает отсутствие конкретного user_id в этом batch нулевым расходом,
       поэтому старое ненулевое значение больше не переиспользуется.
    */
    SELECT
        '' AS user_id,
        toFloat64(0) AS spend
)
GROUP BY user_id;

CREATE TABLE IF NOT EXISTS {db}.campaign_dsp_price_sum
(
    created_at        DateTime('UTC') DEFAULT now('UTC'),
    cid               String,
    sum_cum_per_period Float64
)
ENGINE = ReplacingMergeTree(created_at)
PARTITION BY toYYYYMMDD(created_at)
ORDER BY (created_at, cid)
TTL created_at + INTERVAL 14 DAY DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW {db}.mv_campaign_dsp_price_sum
REFRESH EVERY 1 MINUTE OFFSET 5 SECOND
APPEND TO {db}.campaign_dsp_price_sum
AS
WITH now('UTC') AS batch_created_at
SELECT
    batch_created_at AS created_at,
    cid,
    sum(spend) AS sum_cum_per_period
FROM
(
    /* NAT, BAN, POP, VID оплачиваются по показам: CPM / 1000 */
    SELECT
        argMax(win_cid, event_time) AS cid,
        argMax(win_dsp_price, event_time) / 1000 AS spend
    FROM {db}.fact_impressions
    WHERE
        event_time >= toStartOfMinute(batch_created_at - INTERVAL 1 MINUTE)
        AND event_time < toStartOfMinute(batch_created_at)
        AND format IN ('NAT', 'BAN', 'POP', 'VID')
        AND notEmpty(trimBoth(win_cid))
    GROUP BY impressions_uuid

    UNION ALL

    /*
       IPP оплачивается только по подтверждённым clicks_wins: CPC без деления.
       Используем created_at, а не auction event_time: click-win может прийти позже
       исходного аукциона и всё равно должен попасть в ближайший минутный batch.
    */
    SELECT
        argMax(win_cid, created_at) AS cid,
        argMax(win_dsp_price, created_at) AS spend
    FROM {db}.fact_clicks_wins
    WHERE
        created_at >= batch_created_at - INTERVAL 1 MINUTE
        AND created_at < batch_created_at
        AND format = 'IPP'
        AND notEmpty(trimBoth(win_cid))
    GROUP BY clicks_wins_uuid

    UNION ALL

    /*
       Техническая строка отмечает последний завершённый batch.
       ADV считает отсутствие разрешённой кампании в этом batch нулевым расходом.
    */
    SELECT
        '' AS cid,
        toFloat64(0) AS spend
)
GROUP BY cid;

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
    os_version LowCardinality(String),
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
    site_id,
    os_version
)
TTL event_hour + INTERVAL 10 DAY DELETE
SETTINGS index_granularity = 8192;

ALTER TABLE {db}.traffic_volume_hourly
    ADD COLUMN IF NOT EXISTS os_version LowCardinality(String),
    MODIFY ORDER BY
    (
        event_hour,
        format,
        typic,
        geo,
        lang,
        device,
        os,
        browser,
        site_id,
        os_version
    );


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
    os_version,
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
        ifNull(os_version, '') AS os_version,
        ifNull(browser, '') AS browser,
        ifNull(toString(site_id), '') AS site_id,

        toFloat64(ifNull(win_dsp_price, 0)) AS win_dsp_price

    FROM {db}.ortb_logical

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
    os_version,
    browser,
    site_id;


-- ============================================================
-- MV: IMPRESSIONS INPUT -> FACT IMPRESSIONS
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_impressions_to_fact
REFRESH EVERY 1 MINUTE
APPEND TO {db}.fact_impressions
AS
WITH
    pending_impressions AS
    (
        /*
         * Keep the original retry-deduplication semantics: only callbacks not
         * already materialized in fact_impressions are eligible, and physical
         * retries of the same impressions_uuid collapse to the earliest row.
         *
         * IMPORTANT: the POP late-arrival/double-billing guard is intentionally
         * removed here. Recovery remains responsible for creating recovered
         * fact rows, but a later real callback is no longer cross-checked
         * against fact_clicks/fact_impressions recovery state in this minute MV.
         */
        SELECT
            event_time_impressions,
            created_at,
            ad_format,
            uuid,
            impressions_uuid
        FROM {db}.impressions_in
        PREWHERE created_at >= now() - INTERVAL 60 MINUTE
        WHERE impressions_uuid NOT IN
        (
            SELECT impressions_uuid
            FROM {db}.fact_impressions
            PREWHERE created_at >= now() - INTERVAL 65 MINUTE
        )
        ORDER BY created_at ASC, impressions_uuid ASC
        LIMIT 1 BY impressions_uuid
    ),
    ortb_for_pending AS
    (
        /*
         * Do not build the ANY JOIN from the full wide ORTB window.  The two
         * raw response columns are large, so reading them for every ORTB row
         * can exhaust memory even when only a small set of impression UUIDs
         * is actually pending.
         *
         * PREWHERE evaluates the UUID membership first.  The wide/raw columns
         * below are therefore read only for auctions referenced by the current
         * pending set.  There is deliberately no additional time predicate:
         * matching semantics stay identical to the previous join against the
         * whole {db}.ortb table.
         */
        SELECT
            uuid,
            event_time,
            code,
            format,
            typic,
            spp_domain,
            ip,
            ipv6,
            lang,
            browser,
            browser_version,
            os,
            os_version,
            device,
            site_id,
            site_domain,
            bid_floor,
            geo,
            city_id,
            bid_responses_raw,
            adv_rtb_responses_raw,
            win_dsp_domain,
            win_final_price,
            win_dsp_price,
            win_cid,
            win_crid,
            win_user_id,
            pub_user_id,
            ad_code_id,
            zone_id
        FROM {db}.ortb
        PREWHERE uuid IN
        (
            SELECT uuid
            FROM pending_impressions
        )
    )

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
    o.adv_rtb_responses_raw AS adv_rtb_responses_raw,

    ifNull(o.win_dsp_domain, '') AS win_dsp_domain,

    o.win_final_price AS win_final_price,
    o.win_dsp_price AS win_dsp_price,

    o.win_cid AS win_cid,
    o.win_crid AS win_crid,
    o.win_user_id AS win_user_id,
    o.pub_user_id AS pub_user_id,
    o.ad_code_id AS ad_code_id,
    o.zone_id AS zone_id,

    toUInt8(0) AS is_pop_recovered,
    CAST(NULL AS Nullable(DateTime64(3, 'UTC'))) AS pop_recovered_at,
    if(
        o.format = 'POP',
        concat('pop-auction:', toString(o.uuid)),
        ''
    ) AS pop_recovery_key,
    '' AS pop_recovery_source_key
FROM pending_impressions AS a
ANY INNER JOIN ortb_for_pending AS o
    ON a.uuid = o.uuid;


-- ============================================================
-- REFRESHABLE MV: CLICKS INPUT -> FACT CLICKS
-- Запускается раз в минуту
-- Берёт только uuid, которых нет в fact_clicks за последние 5 минут
-- ============================================================



CREATE MATERIALIZED VIEW {db}.mv_clicks_wins_to_fact
REFRESH EVERY 1 MINUTE OFFSET 20 SECOND
APPEND TO {db}.fact_clicks_wins
AS
WITH pending_clicks_wins AS
(
    SELECT
        event_time_clicks_wins,
        clicks_wins_uuid,
        uuid
    FROM {db}.clicks_wins_in
    PREWHERE created_at >= now() - INTERVAL 60 MINUTE
    WHERE clicks_wins_uuid NOT IN
    (
        SELECT clicks_wins_uuid
        FROM {db}.fact_clicks_wins
        PREWHERE created_at >= now() - INTERVAL 65 MINUTE
    )
)
SELECT
    a.event_time_clicks_wins AS event_time_clicks_wins,
    o.event_time AS event_time,
    toDate(o.event_time) AS event_date,
    toStartOfHour(toDateTime(o.event_time, 'UTC')) AS event_hour,

    o.uuid AS uuid,
    a.clicks_wins_uuid AS clicks_wins_uuid,
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
    o.adv_rtb_responses_raw AS adv_rtb_responses_raw,

    ifNull(o.win_dsp_domain, '') AS win_dsp_domain,

    o.win_final_price AS win_final_price,
    o.win_dsp_price AS win_dsp_price,

    o.win_cid AS win_cid,
    o.win_crid AS win_crid,
    o.win_user_id AS win_user_id,
    o.pub_user_id AS pub_user_id,
    o.ad_code_id AS ad_code_id,
    o.zone_id AS zone_id
FROM
(
    /*
     * ORTB is the streamed (left) side.
     * Only UUIDs present in the small pending set are read, and one
     * deterministic ORTB row is retained for every auction UUID.
     */
    SELECT
        event_time,
        uuid,
        code,
        format,
        typic,
        spp_domain,
        ip,
        ipv6,
        lang,
        browser,
        browser_version,
        os,
        os_version,
        device,
        site_id,
        site_domain,
        bid_floor,
        geo,
        city_id,
        bid_responses_raw,
        adv_rtb_responses_raw,
        win_dsp_domain,
        win_final_price,
        win_dsp_price,
        win_cid,
        win_crid,
        win_user_id,
        pub_user_id,
        ad_code_id,
        zone_id
    FROM {db}.ortb
    PREWHERE created_at >= now() - INTERVAL 65 MINUTE
    WHERE uuid IN
    (
        SELECT uuid
        FROM pending_clicks_wins
    )
    ORDER BY created_at DESC
    LIMIT 1 BY uuid
) AS o
ALL INNER JOIN pending_clicks_wins AS a
    ON o.uuid = a.uuid
SETTINGS query_plan_join_swap_table = 'false';

CREATE MATERIALIZED VIEW {db}.mv_conversions_to_fact
REFRESH EVERY 1 MINUTE OFFSET 40 SECOND
APPEND TO {db}.fact_conversions
AS
WITH pending_conversions AS
(
    SELECT
        created_at,
        conversions_event_time,
        clicks_uuid,
        payout,
        upperUTF8(ifNull(status, '')) AS status,
        toUInt8(upperUTF8(ifNull(status, '')) = 'APPROVED') AS approved
    FROM {db}.conversions_in
    PREWHERE created_at >= now() - toIntervalMinute(1440)
    WHERE upperUTF8(ifNull(status, '')) IN
    (
        '',
        'PENDING',
        'APPROVED',
        '{STATUS}'
    )
    AND
    (
        clicks_uuid,
        conversions_event_time,
        toUInt8(upperUTF8(ifNull(status, '')) = 'APPROVED')
    ) NOT IN
    (
        SELECT
            clicks_uuid,
            conversions_event_time,
            approved
        FROM {db}.fact_conversions
        PREWHERE created_at >= now() - toIntervalMinute(1445)
    )
)
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
    o.adv_rtb_responses_raw AS adv_rtb_responses_raw,

    ifNull(o.win_dsp_domain, '') AS win_dsp_domain,

    o.win_final_price AS win_final_price,
    o.win_dsp_price AS win_dsp_price,

    o.win_cid AS win_cid,
    o.win_crid AS win_crid,
    o.win_user_id AS win_user_id,
    o.pub_user_id AS pub_user_id,
    o.ad_code_id AS ad_code_id,
    o.zone_id AS zone_id,

    a.payout AS payout,
    a.status AS status,
    a.approved AS approved
FROM
(
    /*
     * fact_clicks is streamed from the left. The hash table is built only
     * from the small pending_conversions set on the right.
     */
    SELECT
        event_time,
        uuid,
        clicks_uuid,
        code,
        format,
        typic,
        spp_domain,
        ip,
        ipv6,
        lang,
        browser,
        browser_version,
        os,
        os_version,
        device_type,
        site_id,
        site_domain,
        bid_floor,
        geo,
        city_id,
        bid_responses_raw,
        adv_rtb_responses_raw,
        win_dsp_domain,
        win_final_price,
        win_dsp_price,
        win_cid,
        win_crid,
        win_user_id,
        pub_user_id,
        ad_code_id,
        zone_id
    FROM {db}.fact_clicks
    WHERE clicks_uuid IN
    (
        SELECT clicks_uuid
        FROM pending_conversions
    )
    ORDER BY created_at DESC
    LIMIT 1 BY clicks_uuid
) AS o
ALL INNER JOIN pending_conversions AS a
    ON o.clicks_uuid = a.clicks_uuid
SETTINGS query_plan_join_swap_table = 'false';

-- ============================================================
-- POP RECOVERY EVENT LOG (DELTA SOURCE FOR BACKEND)
-- ============================================================
--
-- Do not maintain another ever-growing cumulative money counter.  Every newly
-- recovered POP impression emits one immutable source event.  The backend can
-- scan only rows after its last composite marker
-- (recovery_at, recovery_source_key), group those rows by user/campaign, and
-- turn only that new slice into Redis adjustments.
--
-- recovery_source_key is already the stable idempotency key used by the POP
-- recovery writer (pop-click:<clicks_uuid>).  recovery_at orders newly emitted
-- events; the source key breaks ties inside one recovery batch.
CREATE TABLE IF NOT EXISTS {db}.pop_recovery_events
(
    recovery_at             DateTime64(3, 'UTC'),
    recovery_source_key     String,
    recovery_key            String DEFAULT '',
    auction_uuid            UUID,

    user_id                 String DEFAULT '',
    campaign_id             String DEFAULT '',
    spend                   Float64,

    event_time              DateTime64(3, 'UTC'),
    event_date              Date
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(recovery_at)
ORDER BY (recovery_at, recovery_source_key)
TTL recovery_at + INTERVAL 6 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_fact_pop_recovered_to_events
TO {db}.pop_recovery_events
AS
SELECT
    assumeNotNull(pop_recovered_at) AS recovery_at,
    pop_recovery_source_key AS recovery_source_key,
    pop_recovery_key AS recovery_key,
    uuid AS auction_uuid,

    win_user_id AS user_id,
    win_cid AS campaign_id,
    win_dsp_price / 1000 AS spend,

    event_time AS event_time,
    event_date AS event_date
FROM {db}.fact_impressions
WHERE format = 'POP'
  AND is_pop_recovered = 1
  AND pop_recovered_at IS NOT NULL
  AND pop_recovery_source_key != '';


-- ============================================================
-- MV: FACT IMPRESSIONS -> AGG STATS
-- browser_version здесь специально НЕ группируется
-- ============================================================

CREATE MATERIALIZED VIEW {db}.mv_fact_impressions_to_agg_stats
TO {db}.agg_stats
AS
SELECT
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    os_version,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic,
    pub_user_id,
    ad_code_id,
    zone_id,

    count() AS impressions,
    toUInt64(0) AS clicks,

    toUInt64(0)  as conversions,
    toFloat64(0) AS payout,
    toUInt64(0) AS conversions_approved,
    toFloat64(0) AS payout_approved,

    toFloat64(0) AS spend_clicks_table,

    sum(win_dsp_price / 1000) AS spend_views_table,
    toFloat64(0) AS ssp_revenue_views
FROM {db}.fact_impressions
GROUP BY
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    os_version,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic,
    pub_user_id,
    ad_code_id,
    zone_id;



-- ============================================================
-- POP RECOVERY TRANSITION CATCH-UP
-- ============================================================
--
-- Re-scan two fully closed hours after all recovery writers were stopped.
-- Existing fact rows reduce the deficit and already-used source keys are
-- excluded, so restart/retry is idempotent.  The same full-missing vs partial
-- deficit semantics as the legacy production recovery are used here.
INSERT INTO {db}.fact_impressions
(
    event_time_impressions,
    event_time,
    event_date,
    event_hour,
    uuid,
    impressions_uuid,
    code,
    format,
    typic,
    spp_domain,
    ip,
    ipv6,
    lang,
    browser,
    browser_version,
    os,
    os_version,
    device_type,
    site_id,
    site_domain,
    bid_floor,
    geo,
    city_id,
    bid_responses_raw,
    adv_rtb_responses_raw,
    win_dsp_domain,
    win_final_price,
    win_dsp_price,
    win_cid,
    win_crid,
    win_user_id,
    pub_user_id,
    ad_code_id,
    zone_id,
    is_pop_recovered,
    pop_recovered_at,
    pop_recovery_key,
    pop_recovery_source_key,
    created_at
)
WITH
    toStartOfHour(now('UTC') - INTERVAL 2 HOUR) AS dt_from,
    toStartOfHour(now('UTC')) AS dt_to,

    pop_clicks AS
    (
        SELECT
            event_time_clicks,
            event_time,
            event_date,
            event_hour,
            uuid,
            clicks_uuid,
            code,
            format,
            typic,
            spp_domain,
            ip,
            ipv6,
            lang,
            browser,
            browser_version,
            os,
            os_version,
            device_type,
            site_id,
            site_domain,
            bid_floor,
            geo,
            city_id,
            bid_responses_raw,
            adv_rtb_responses_raw,
            win_dsp_domain,
            win_final_price,
            win_dsp_price,
            win_cid,
            win_crid,
            win_user_id,
            pub_user_id,
            ad_code_id,
            zone_id,
            created_at
        FROM {db}.fact_clicks
        WHERE event_time >= dt_from
          AND event_time < dt_to
          AND format = 'POP'
        ORDER BY created_at ASC, clicks_uuid ASC
        LIMIT 1 BY clicks_uuid
    ),

    pop_impressions AS
    (
        SELECT
            event_time_impressions,
            event_time,
            event_date,
            event_hour,
            uuid,
            impressions_uuid,
            code,
            format,
            typic,
            spp_domain,
            ip,
            ipv6,
            lang,
            browser,
            browser_version,
            os,
            os_version,
            device_type,
            site_id,
            site_domain,
            bid_floor,
            geo,
            city_id,
            bid_responses_raw,
            adv_rtb_responses_raw,
            win_dsp_domain,
            win_final_price,
            win_dsp_price,
            win_cid,
            win_crid,
            win_user_id,
            pub_user_id,
            ad_code_id,
            zone_id,
            created_at
        FROM {db}.fact_impressions
        WHERE event_time >= dt_from
          AND event_time < dt_to
          AND format = 'POP'
    ),

    click_counts AS
    (
        SELECT
            uuid,
            count() AS click_cnt
        FROM pop_clicks
        GROUP BY uuid
    ),

    impression_counts AS
    (
        SELECT
            uuid,
            count() AS impression_cnt
        FROM pop_impressions
        GROUP BY uuid
    ),

    deficits AS
    (
        SELECT
            c.uuid,
            c.click_cnt,
            ifNull(i.impression_cnt, 0) AS impression_cnt,
            c.click_cnt - ifNull(i.impression_cnt, 0) AS missing_cnt
        FROM click_counts AS c
        LEFT JOIN impression_counts AS i USING (uuid)
        WHERE c.click_cnt > ifNull(i.impression_cnt, 0)
    ),

    unused_clicks AS
    (
        SELECT
            c.*,
            concat('pop-click:', toString(c.clicks_uuid)) AS recovery_source_key
        FROM pop_clicks AS c
        WHERE concat('pop-click:', toString(c.clicks_uuid)) NOT IN
        (
            SELECT pop_recovery_source_key
            FROM {db}.fact_impressions
            WHERE event_time >= dt_from
              AND event_time < dt_to
              AND format = 'POP'
              AND is_pop_recovered = 1
              AND pop_recovery_source_key != ''
        )
    ),

    full_missing_candidates AS
    (
        SELECT
            c.*,
            d.missing_cnt,
            row_number() OVER
            (
                PARTITION BY c.uuid
                ORDER BY c.event_time_clicks ASC, c.clicks_uuid ASC
            ) AS recovery_rank
        FROM unused_clicks AS c
        INNER JOIN deficits AS d USING (uuid)
        WHERE d.impression_cnt = 0
    ),

    partial_templates AS
    (
        SELECT
            i.*
        FROM pop_impressions AS i
        INNER JOIN deficits AS d USING (uuid)
        WHERE d.impression_cnt > 0
        ORDER BY i.created_at ASC, i.impressions_uuid ASC
        LIMIT 1 BY uuid
    ),

    partial_recovery_sources AS
    (
        SELECT
            c.uuid,
            c.recovery_source_key,
            d.missing_cnt,
            row_number() OVER
            (
                PARTITION BY c.uuid
                ORDER BY c.event_time_clicks ASC, c.clicks_uuid ASC
            ) AS recovery_rank
        FROM unused_clicks AS c
        INNER JOIN deficits AS d USING (uuid)
        WHERE d.impression_cnt > 0
    )

-- Legacy full-missing path: business fields come from fact_clicks.
SELECT
    c.event_time_clicks AS event_time_impressions,
    c.event_time AS event_time,
    c.event_date AS event_date,
    c.event_hour AS event_hour,
    c.uuid AS uuid,
    c.clicks_uuid AS impressions_uuid,
    c.code AS code,
    c.format AS format,
    c.typic AS typic,
    c.spp_domain AS spp_domain,
    c.ip AS ip,
    c.ipv6 AS ipv6,
    c.lang AS lang,
    c.browser AS browser,
    c.browser_version AS browser_version,
    c.os AS os,
    c.os_version AS os_version,
    c.device_type AS device_type,
    c.site_id AS site_id,
    c.site_domain AS site_domain,
    c.bid_floor AS bid_floor,
    c.geo AS geo,
    c.city_id AS city_id,
    c.bid_responses_raw AS bid_responses_raw,
    c.adv_rtb_responses_raw AS adv_rtb_responses_raw,
    c.win_dsp_domain AS win_dsp_domain,
    c.win_final_price AS win_final_price,
    c.win_dsp_price AS win_dsp_price,
    c.win_cid AS win_cid,
    c.win_crid AS win_crid,
    c.win_user_id AS win_user_id,
    c.pub_user_id AS pub_user_id,
    c.ad_code_id AS ad_code_id,
    c.zone_id AS zone_id,
    toUInt8(1) AS is_pop_recovered,
    now64(3, 'UTC') AS pop_recovered_at,
    concat('pop-auction:', toString(c.uuid)) AS pop_recovery_key,
    c.recovery_source_key AS pop_recovery_source_key,
    c.created_at AS created_at
FROM full_missing_candidates AS c
WHERE c.recovery_rank <= c.missing_cnt

UNION ALL

-- Legacy partial-deficit path: clone the existing impression exactly for all
-- business/attribution fields. Only the new recovery metadata differs.
SELECT
    i.event_time_impressions AS event_time_impressions,
    i.event_time AS event_time,
    i.event_date AS event_date,
    i.event_hour AS event_hour,
    i.uuid AS uuid,
    i.impressions_uuid AS impressions_uuid,
    i.code AS code,
    i.format AS format,
    i.typic AS typic,
    i.spp_domain AS spp_domain,
    i.ip AS ip,
    i.ipv6 AS ipv6,
    i.lang AS lang,
    i.browser AS browser,
    i.browser_version AS browser_version,
    i.os AS os,
    i.os_version AS os_version,
    i.device_type AS device_type,
    i.site_id AS site_id,
    i.site_domain AS site_domain,
    i.bid_floor AS bid_floor,
    i.geo AS geo,
    i.city_id AS city_id,
    i.bid_responses_raw AS bid_responses_raw,
    i.adv_rtb_responses_raw AS adv_rtb_responses_raw,
    i.win_dsp_domain AS win_dsp_domain,
    i.win_final_price AS win_final_price,
    i.win_dsp_price AS win_dsp_price,
    i.win_cid AS win_cid,
    i.win_crid AS win_crid,
    i.win_user_id AS win_user_id,
    i.pub_user_id AS pub_user_id,
    i.ad_code_id AS ad_code_id,
    i.zone_id AS zone_id,
    toUInt8(1) AS is_pop_recovered,
    now64(3, 'UTC') AS pop_recovered_at,
    concat('pop-auction:', toString(i.uuid)) AS pop_recovery_key,
    s.recovery_source_key AS pop_recovery_source_key,
    i.created_at AS created_at
FROM partial_templates AS i
INNER JOIN partial_recovery_sources AS s USING (uuid)
WHERE s.recovery_rank <= s.missing_cnt;


-- ============================================================
-- REFRESHABLE MV: POP CLICK DEFICIT -> RECOVERED IMPRESSIONS
-- ============================================================
--
-- Recovery is enabled only after the transition catch-up has completed.
-- Full absence keeps the legacy fact_clicks-based path. Partial deficit keeps
-- the legacy clone-an-existing-impression path; the new recovery metadata is
-- layered on top without changing the recovered row's business fields.
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_recover_pop_impressions_v2
REFRESH EVERY 1 HOUR OFFSET 10 MINUTE
APPEND TO {db}.fact_impressions
(
    event_time_impressions,
    event_time,
    event_date,
    event_hour,
    uuid,
    impressions_uuid,
    code,
    format,
    typic,
    spp_domain,
    ip,
    ipv6,
    lang,
    browser,
    browser_version,
    os,
    os_version,
    device_type,
    site_id,
    site_domain,
    bid_floor,
    geo,
    city_id,
    bid_responses_raw,
    adv_rtb_responses_raw,
    win_dsp_domain,
    win_final_price,
    win_dsp_price,
    win_cid,
    win_crid,
    win_user_id,
    pub_user_id,
    ad_code_id,
    zone_id,
    is_pop_recovered,
    pop_recovered_at,
    pop_recovery_key,
    pop_recovery_source_key,
    created_at
)
EMPTY
AS
WITH
    toStartOfHour(now('UTC') - INTERVAL 1 HOUR) AS dt_from,
    toStartOfHour(now('UTC')) AS dt_to,

    pop_clicks AS
    (
        SELECT
            event_time_clicks,
            event_time,
            event_date,
            event_hour,
            uuid,
            clicks_uuid,
            code,
            format,
            typic,
            spp_domain,
            ip,
            ipv6,
            lang,
            browser,
            browser_version,
            os,
            os_version,
            device_type,
            site_id,
            site_domain,
            bid_floor,
            geo,
            city_id,
            bid_responses_raw,
            adv_rtb_responses_raw,
            win_dsp_domain,
            win_final_price,
            win_dsp_price,
            win_cid,
            win_crid,
            win_user_id,
            pub_user_id,
            ad_code_id,
            zone_id,
            created_at
        FROM {db}.fact_clicks
        WHERE event_time >= dt_from
          AND event_time < dt_to
          AND format = 'POP'
        ORDER BY created_at ASC, clicks_uuid ASC
        LIMIT 1 BY clicks_uuid
    ),

    pop_impressions AS
    (
        SELECT
            event_time_impressions,
            event_time,
            event_date,
            event_hour,
            uuid,
            impressions_uuid,
            code,
            format,
            typic,
            spp_domain,
            ip,
            ipv6,
            lang,
            browser,
            browser_version,
            os,
            os_version,
            device_type,
            site_id,
            site_domain,
            bid_floor,
            geo,
            city_id,
            bid_responses_raw,
            adv_rtb_responses_raw,
            win_dsp_domain,
            win_final_price,
            win_dsp_price,
            win_cid,
            win_crid,
            win_user_id,
            pub_user_id,
            ad_code_id,
            zone_id,
            created_at
        FROM {db}.fact_impressions
        WHERE event_time >= dt_from
          AND event_time < dt_to
          AND format = 'POP'
    ),

    click_counts AS
    (
        SELECT
            uuid,
            count() AS click_cnt
        FROM pop_clicks
        GROUP BY uuid
    ),

    impression_counts AS
    (
        SELECT
            uuid,
            count() AS impression_cnt
        FROM pop_impressions
        GROUP BY uuid
    ),

    deficits AS
    (
        SELECT
            c.uuid,
            c.click_cnt,
            ifNull(i.impression_cnt, 0) AS impression_cnt,
            c.click_cnt - ifNull(i.impression_cnt, 0) AS missing_cnt
        FROM click_counts AS c
        LEFT JOIN impression_counts AS i USING (uuid)
        WHERE c.click_cnt > ifNull(i.impression_cnt, 0)
    ),

    unused_clicks AS
    (
        SELECT
            c.*,
            concat('pop-click:', toString(c.clicks_uuid)) AS recovery_source_key
        FROM pop_clicks AS c
        WHERE concat('pop-click:', toString(c.clicks_uuid)) NOT IN
        (
            SELECT pop_recovery_source_key
            FROM {db}.fact_impressions
            WHERE event_time >= dt_from
              AND event_time < dt_to
              AND format = 'POP'
              AND is_pop_recovered = 1
              AND pop_recovery_source_key != ''
        )
    ),

    full_missing_candidates AS
    (
        SELECT
            c.*,
            d.missing_cnt,
            row_number() OVER
            (
                PARTITION BY c.uuid
                ORDER BY c.event_time_clicks ASC, c.clicks_uuid ASC
            ) AS recovery_rank
        FROM unused_clicks AS c
        INNER JOIN deficits AS d USING (uuid)
        WHERE d.impression_cnt = 0
    ),

    partial_templates AS
    (
        SELECT
            i.*
        FROM pop_impressions AS i
        INNER JOIN deficits AS d USING (uuid)
        WHERE d.impression_cnt > 0
        ORDER BY i.created_at ASC, i.impressions_uuid ASC
        LIMIT 1 BY uuid
    ),

    partial_recovery_sources AS
    (
        SELECT
            c.uuid,
            c.recovery_source_key,
            d.missing_cnt,
            row_number() OVER
            (
                PARTITION BY c.uuid
                ORDER BY c.event_time_clicks ASC, c.clicks_uuid ASC
            ) AS recovery_rank
        FROM unused_clicks AS c
        INNER JOIN deficits AS d USING (uuid)
        WHERE d.impression_cnt > 0
    )

-- Legacy full-missing path: business fields come from fact_clicks.
SELECT
    c.event_time_clicks AS event_time_impressions,
    c.event_time AS event_time,
    c.event_date AS event_date,
    c.event_hour AS event_hour,
    c.uuid AS uuid,
    c.clicks_uuid AS impressions_uuid,
    c.code AS code,
    c.format AS format,
    c.typic AS typic,
    c.spp_domain AS spp_domain,
    c.ip AS ip,
    c.ipv6 AS ipv6,
    c.lang AS lang,
    c.browser AS browser,
    c.browser_version AS browser_version,
    c.os AS os,
    c.os_version AS os_version,
    c.device_type AS device_type,
    c.site_id AS site_id,
    c.site_domain AS site_domain,
    c.bid_floor AS bid_floor,
    c.geo AS geo,
    c.city_id AS city_id,
    c.bid_responses_raw AS bid_responses_raw,
    c.adv_rtb_responses_raw AS adv_rtb_responses_raw,
    c.win_dsp_domain AS win_dsp_domain,
    c.win_final_price AS win_final_price,
    c.win_dsp_price AS win_dsp_price,
    c.win_cid AS win_cid,
    c.win_crid AS win_crid,
    c.win_user_id AS win_user_id,
    c.pub_user_id AS pub_user_id,
    c.ad_code_id AS ad_code_id,
    c.zone_id AS zone_id,
    toUInt8(1) AS is_pop_recovered,
    now64(3, 'UTC') AS pop_recovered_at,
    concat('pop-auction:', toString(c.uuid)) AS pop_recovery_key,
    c.recovery_source_key AS pop_recovery_source_key,
    c.created_at AS created_at
FROM full_missing_candidates AS c
WHERE c.recovery_rank <= c.missing_cnt

UNION ALL

-- Legacy partial-deficit path: clone the existing impression exactly for all
-- business/attribution fields. Only the new recovery metadata differs.
SELECT
    i.event_time_impressions AS event_time_impressions,
    i.event_time AS event_time,
    i.event_date AS event_date,
    i.event_hour AS event_hour,
    i.uuid AS uuid,
    i.impressions_uuid AS impressions_uuid,
    i.code AS code,
    i.format AS format,
    i.typic AS typic,
    i.spp_domain AS spp_domain,
    i.ip AS ip,
    i.ipv6 AS ipv6,
    i.lang AS lang,
    i.browser AS browser,
    i.browser_version AS browser_version,
    i.os AS os,
    i.os_version AS os_version,
    i.device_type AS device_type,
    i.site_id AS site_id,
    i.site_domain AS site_domain,
    i.bid_floor AS bid_floor,
    i.geo AS geo,
    i.city_id AS city_id,
    i.bid_responses_raw AS bid_responses_raw,
    i.adv_rtb_responses_raw AS adv_rtb_responses_raw,
    i.win_dsp_domain AS win_dsp_domain,
    i.win_final_price AS win_final_price,
    i.win_dsp_price AS win_dsp_price,
    i.win_cid AS win_cid,
    i.win_crid AS win_crid,
    i.win_user_id AS win_user_id,
    i.pub_user_id AS pub_user_id,
    i.ad_code_id AS ad_code_id,
    i.zone_id AS zone_id,
    toUInt8(1) AS is_pop_recovered,
    now64(3, 'UTC') AS pop_recovered_at,
    concat('pop-auction:', toString(i.uuid)) AS pop_recovery_key,
    s.recovery_source_key AS pop_recovery_source_key,
    i.created_at AS created_at
FROM partial_templates AS i
INNER JOIN partial_recovery_sources AS s USING (uuid)
WHERE s.recovery_rank <= s.missing_cnt;


-- ============================================================
-- MV: FACT CLICKS -> AGG STATS
-- IPP intentionally excluded: partner-confirmed fact_clicks_wins is the
-- authoritative source for IPP clicks and click spend.
-- browser_version здесь специально НЕ группируется
-- ============================================================

CREATE MATERIALIZED VIEW {db}.mv_fact_clicks_to_agg_stats
TO {db}.agg_stats
AS
SELECT
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    os_version,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic,
    pub_user_id,
    ad_code_id,
    zone_id,

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
    toFloat64(0) AS spend_views_table,
    toFloat64(0) AS ssp_revenue_views
FROM {db}.fact_clicks
WHERE format != 'IPP'
GROUP BY
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    os_version,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic,
    pub_user_id,
    ad_code_id,
    zone_id;

-- ============================================================
-- MV: FACT CLICKS WINS -> AGG STATS (IPP ONLY)
-- For IPP the partner-confirmed click-win is the billable/statistical click.
-- ============================================================

CREATE MATERIALIZED VIEW {db}.mv_fact_clicks_wins_to_agg_stats
TO {db}.agg_stats
AS
SELECT
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    os_version,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic,
    pub_user_id,
    ad_code_id,
    zone_id,

    toUInt64(0) AS impressions,
    count() AS clicks,

    toUInt64(0) AS conversions,
    toFloat64(0) AS payout,
    toUInt64(0) AS conversions_approved,
    toFloat64(0) AS payout_approved,

    sum(win_dsp_price) AS spend_clicks_table,
    toFloat64(0) AS spend_views_table,
    toFloat64(0) AS ssp_revenue_views
FROM {db}.fact_clicks_wins
WHERE format = 'IPP'
GROUP BY
    win_user_id,
    win_cid,
    win_crid,

    event_date,

    device_type,
    os,
    os_version,
    event_hour,

    browser,
    geo,
    site_id,

    format,
    typic,
    pub_user_id,
    ad_code_id,
    zone_id;

CREATE MATERIALIZED VIEW {db}.mv_fact_conversions_to_agg_stats
TO {db}.agg_stats
AS
SELECT
    win_user_id,
    win_cid,
    win_crid,
    event_date,
    device_type,
    os,
    os_version,
    event_hour,
    browser,
    geo,
    site_id,
    format,
    typic,
    pub_user_id,
    ad_code_id,
    zone_id,

    toUInt64(0) AS impressions,
    toUInt64(0) AS clicks,

    toUInt64(countIf(approved != 1)) AS conversions,
    toFloat64(sumIf(conversions_payout, approved != 1)) AS payout,

    toUInt64(countIf(approved = 1)) AS conversions_approved,
    toFloat64(sumIf(conversions_payout, approved = 1)) AS payout_approved,

    toFloat64(0) AS spend_clicks_table,
    toFloat64(0) AS spend_views_table,
    toFloat64(0) AS ssp_revenue_views
FROM
(
    SELECT
        win_user_id,
        win_cid,
        win_crid,
        event_date,
        device_type,
        os,
        os_version,
        event_hour,
        browser,
        geo,
        site_id,
        format,
        typic,
        pub_user_id,
        ad_code_id,
        zone_id,
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
    os_version,
    event_hour,
    browser,
    geo,
    site_id,
    format,
    typic,
    pub_user_id,
    ad_code_id,
    zone_id;

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
        FROM {db}.ortb_logical
        WHERE event_time >= metric_minute
          AND event_time < current_minute
        GROUP BY code_str
    )
) AS m

CROSS JOIN
(
    /*
       Keep the diagnostic click count on the same source semantics as billing:
       IPP comes from partner-confirmed fact_clicks_wins, other formats from
       fact_clicks.
    */
    SELECT sum(cnt) AS cnt_clicks_5m
    FROM
    (
        SELECT count() AS cnt
        FROM {db}.fact_clicks
        WHERE event_time >= ratio_from
          AND event_time < ratio_to
          AND format != 'IPP'

        UNION ALL

        SELECT count() AS cnt
        FROM {db}.fact_clicks_wins
        WHERE event_time >= ratio_from
          AND event_time < ratio_to
          AND format = 'IPP'
    )
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
    FROM {db}.ortb_logical
    WHERE event_time >= ratio_from
      AND event_time < ratio_to
) AS o;
-- UUID-derived buckets are immutable even if a replay has conflicting event_time.
-- New Publisher POP clicks use UUIDv7 (one-hour buckets); UUIDv4 IDs map
-- to 256 fixed buckets. No registry/checkpoint TTL: replays remain duplicates.
-- Publisher starts with no past revenue; no legacy registry or import is needed.
CREATE TABLE IF NOT EXISTS {db}.publisher_pop_revenue_events_v2
(
 clicks_uuid UUID,
 bucket UInt64 MATERIALIZED if(substring(toString(clicks_uuid),15,1)='7',1000000+intDiv(toUInt64(toUnixTimestamp64Milli(UUIDv7ToDateTime(clicks_uuid,'UTC'))),3600000),1+cityHash64(clicks_uuid)%256),
 event AggregateFunction(argMin,Tuple(String,String,String,Date,String,String,String,DateTime('UTC'),String,String,String,String,String,String,String,String,Float64),DateTime64(3,'UTC')),
 seen SimpleAggregateFunction(sum,UInt64),
 PROJECTION generations (SELECT bucket,sum(seen) GROUP BY bucket)
)
ENGINE=AggregatingMergeTree PARTITION BY intDiv(bucket,24) ORDER BY (bucket,clicks_uuid)
SETTINGS deduplicate_merge_projection_mode='rebuild';

CREATE TABLE IF NOT EXISTS {db}.publisher_pop_revenue_checkpoints
(bucket UInt64,generation UInt64)
ENGINE=ReplacingMergeTree(generation) ORDER BY bucket;
-- The projection is maintained atomically in the same parts as the money rows.
-- Filtering on bucket prunes historical money groups independently of event_date.
ALTER TABLE {db}.agg_stats MODIFY SETTING deduplicate_merge_projection_mode='rebuild';
ALTER TABLE {db}.agg_stats ADD PROJECTION IF NOT EXISTS publisher_pop_by_bucket
(SELECT pop_revenue_bucket,win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id,sum(ssp_revenue_views) GROUP BY pop_revenue_bucket,win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id);


CREATE MATERIALIZED VIEW {db}.mv_fact_clicks_to_publisher_pop_revenue
TO {db}.publisher_pop_revenue_events_v2
AS SELECT clicks_uuid,argMinState(tuple(win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id,win_final_price),created_at) AS event,count() AS seen
FROM {db}.fact_clicks
WHERE format='POP' AND pub_user_id!='' AND clicks_uuid!=toUUID('00000000-0000-0000-0000-000000000000')
GROUP BY clicks_uuid;

CREATE VIEW {db}.publisher_pop_revenue_dirty AS
SELECT bucket FROM
(SELECT bucket,sum(seen) AS generation FROM {db}.publisher_pop_revenue_events_v2 GROUP BY bucket) AS source
LEFT JOIN
(SELECT bucket,max(generation) AS done FROM {db}.publisher_pop_revenue_checkpoints GROUP BY bucket) AS progress USING bucket
WHERE generation!=ifNull(done,0);

-- Only changed buckets are read. Generation and entitlement are captured in
-- ONE registry scan, so a concurrent late insert cannot be acknowledged unseen.
-- Freeze dirty membership in one non-correlated scalar per query. Repeating
-- dirty-table subqueries (or a CTE) in the UNION branches can observe a checkpoint
-- between the two reads and mistake already-paid revenue for a missing target.
-- ClickHouse caches the scalar by query-tree hash; do not replace it with a CTE.
CREATE VIEW {db}.publisher_pop_revenue_delta AS
WITH (SELECT groupArray(bucket) FROM {db}.publisher_pop_revenue_dirty) AS dirty_buckets,
canonical AS
(
 SELECT bucket,clicks_uuid,argMinMerge(event) AS e,sum(seen) AS n
 FROM {db}.publisher_pop_revenue_events_v2
 WHERE bucket IN (SELECT arrayJoin(dirty_buckets))
 GROUP BY bucket,clicks_uuid
), snapshot AS
(
 SELECT *,sum(n) OVER (PARTITION BY bucket) AS generation FROM canonical
), changes AS
(
 SELECT bucket AS pop_revenue_bucket,e.1 AS win_user_id,e.2 AS win_cid,e.3 AS win_crid,e.4 AS event_date,e.5 AS device_type,e.6 AS os,e.7 AS os_version,e.8 AS event_hour,e.9 AS browser,e.10 AS geo,e.11 AS site_id,e.12 AS format,e.13 AS typic,e.14 AS pub_user_id,e.15 AS ad_code_id,e.16 AS zone_id,generation,toDecimal128(toString(e.17),18)/1000 AS delta FROM snapshot
 UNION ALL
 SELECT pop_revenue_bucket,win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id,toUInt64(0) AS generation,-toDecimal128(toString(ssp_revenue_views),18) AS delta
 FROM {db}.agg_stats
 WHERE format='POP' AND pub_user_id!='' AND pop_revenue_bucket IN (SELECT arrayJoin(dirty_buckets))
)
SELECT pop_revenue_bucket,win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id,max(generation) AS generation,toFloat64(round(sum(delta),12)) AS ssp_revenue_views
FROM changes GROUP BY pop_revenue_bucket,win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id
SETTINGS enable_scalar_subquery_optimization=1;

-- New Publisher POP revenue is calculated from newly ingested fact_clicks only.
-- There is no historical Publisher POP import on normal startup.
CREATE MATERIALIZED VIEW {db}.mv_publisher_pop_revenue_to_agg_stats
REFRESH EVERY 1 MINUTE APPEND TO {db}.agg_stats AS
SELECT pop_revenue_bucket,win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id,ssp_revenue_views
FROM {db}.publisher_pop_revenue_delta WHERE abs(ssp_revenue_views)>0.000000000001;

-- This is a repair checkpoint, NOT a claim that an INSERT succeeded. A bucket
-- is clean only after a fresh snapshot proves every target group equals its
-- canonical entitlement. Partial/unknown inserts are reconciled on the next run.
CREATE MATERIALIZED VIEW {db}.mv_publisher_pop_revenue_checkpoint
REFRESH EVERY 1 MINUTE DEPENDS ON {db}.mv_publisher_pop_revenue_to_agg_stats
APPEND TO {db}.publisher_pop_revenue_checkpoints AS
SELECT pop_revenue_bucket AS bucket,max(generation) AS generation
FROM {db}.publisher_pop_revenue_delta GROUP BY bucket
HAVING generation>0 AND countIf(abs(ssp_revenue_views)>0.000000000001)=0;

-- Registry and aggregate writers are installed before new facts can arrive.
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
    a.is_iframe AS is_iframe,
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
    o.adv_rtb_responses_raw AS adv_rtb_responses_raw,

    ifNull(o.win_dsp_domain, '') AS win_dsp_domain,

    o.win_final_price AS win_final_price,
    o.win_dsp_price AS win_dsp_price,

    o.win_cid AS win_cid,
    o.win_crid AS win_crid,
    o.win_user_id AS win_user_id,
    o.pub_user_id AS pub_user_id,
    o.ad_code_id AS ad_code_id,
    o.zone_id AS zone_id
FROM {db}.clicks_in AS a
ANY INNER JOIN {db}.ortb AS o
    ON a.uuid = o.uuid
WHERE a.clicks_uuid NOT IN (
    SELECT clicks_uuid
    FROM {db}.fact_clicks
    WHERE created_at >= now() - INTERVAL 65 MINUTE
)
AND a.created_at >= now() - INTERVAL 60 MINUTE;

`

func CreateDB(ctx context.Context, ch clickhouse.Conn, database string) error {
	database = strings.TrimSpace(database)
	if !diagnosticDatabase.MatchString(database) {
		return fmt.Errorf("invalid ClickHouse database name")
	}
	// Full bootstrap DDL is only valid on a fresh database. Existing tables,
	// advertiser balances, and historical data are never replaced or deleted.
	exists, err := tableExists(ctx, ch, database, "agg_stats")
	if err != nil {
		return err
	}
	if exists {
		return EnsureLoaderSchema(ctx, ch, database)
	}

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

	// Mark a successfully created *new* Publisher schema. No old financial
	// data existed and no existing financial MV was ever dropped.
	if err := ch.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+database+".publisher_schema_versions (version UInt64) ENGINE=ReplacingMergeTree ORDER BY version"); err != nil {
		return err
	}
	if err := ch.Exec(ctx, "INSERT INTO "+database+".publisher_schema_versions VALUES(?)", publisherSchemaVersion); err != nil {
		return err
	}
	return ApplyPublisherZoneDiagnostics(ctx, ch, database)
}

// Exceptional repair after a failed ClickHouse source->MV write. This is an
// explicit maintenance operation, never a recurring full-history refresh. It
// streams retained facts/old registry states; it cannot pay a UUID twice.
func RepairPublisherPOPRevenueRegistry(ctx context.Context, ch clickhouse.Conn, db string) error {
	for _, query := range publisherPOPRegistryImportSQL(db) {
		if err := ch.Exec(ctx, query); err != nil {
			return err
		}
	}
	return nil
}
func publisherPOPRegistryImportSQL(db string) []string {
	return []string{
		"INSERT INTO " + db + ".publisher_pop_revenue_events_v2(clicks_uuid,event,seen) SELECT clicks_uuid,initializeAggregation('argMinState',tuple(win_user_id,win_cid,win_crid,event_date,device_type,os,os_version,event_hour,browser,geo,site_id,format,typic,pub_user_id,ad_code_id,zone_id,win_final_price),created_at),toUInt64(1) FROM " + db + ".fact_clicks WHERE format='POP' AND pub_user_id!='' AND clicks_uuid!=toUUID('00000000-0000-0000-0000-000000000000') ",
	}
}

// splitClickHouseStatements splits only on executable SQL delimiters. A raw
// strings.Split(sql, ";") is unsafe because comments and quoted literals may
// legitimately contain semicolons.
func splitClickHouseStatements(sql string) []string {
	statements := make([]string, 0, strings.Count(sql, ";")+1)
	var current strings.Builder
	hasExecutableSQL := false
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false

	flush := func() {
		statement := strings.TrimSpace(current.String())
		if hasExecutableSQL && statement != "" {
			statements = append(statements, statement)
		}
		current.Reset()
		hasExecutableSQL = false
	}

	for i := 0; i < len(sql); i++ {
		c := sql[i]
		var next byte
		if i+1 < len(sql) {
			next = sql[i+1]
		}

		if inLineComment {
			current.WriteByte(c)
			if c == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			current.WriteByte(c)
			if c == '*' && next == '/' {
				current.WriteByte(next)
				i++
				inBlockComment = false
			}
			continue
		}

		if inSingleQuote {
			current.WriteByte(c)
			if c == '\\' && i+1 < len(sql) {
				i++
				current.WriteByte(sql[i])
				continue
			}
			if c == '\'' {
				if next == '\'' {
					current.WriteByte(next)
					i++
				} else {
					inSingleQuote = false
				}
			}
			continue
		}

		if inDoubleQuote {
			current.WriteByte(c)
			if c == '\\' && i+1 < len(sql) {
				i++
				current.WriteByte(sql[i])
				continue
			}
			if c == '"' {
				if next == '"' {
					current.WriteByte(next)
					i++
				} else {
					inDoubleQuote = false
				}
			}
			continue
		}

		if inBacktick {
			current.WriteByte(c)
			if c == '`' {
				if next == '`' {
					current.WriteByte(next)
					i++
				} else {
					inBacktick = false
				}
			}
			continue
		}

		switch {
		case c == '-' && next == '-':
			current.WriteByte(c)
			current.WriteByte(next)
			i++
			inLineComment = true
		case c == '/' && next == '*':
			current.WriteByte(c)
			current.WriteByte(next)
			i++
			inBlockComment = true
		case c == '\'':
			current.WriteByte(c)
			inSingleQuote = true
			hasExecutableSQL = true
		case c == '"':
			current.WriteByte(c)
			inDoubleQuote = true
			hasExecutableSQL = true
		case c == '`':
			current.WriteByte(c)
			inBacktick = true
			hasExecutableSQL = true
		case c == ';':
			flush()
		default:
			current.WriteByte(c)
			if !strings.ContainsRune(" \t\r\n", rune(c)) {
				hasExecutableSQL = true
			}
		}
	}

	flush()
	return statements
}
