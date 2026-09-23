-- Stage 04 also adds the pre-campaign exact request identity to ORTB attribution.
ALTER TABLE ortb ADD COLUMN IF NOT EXISTS exact_segment_hash String DEFAULT '' AFTER win_user_id;

-- Stage 04: durable percenter state/history/telemetry sinks.
-- event_id is stable across retries. Delivery checks event_id before retry INSERT; ReplacingMergeTree + FINAL logical views are a second-line replay safety net.
CREATE TABLE IF NOT EXISTS percenter_state_history
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

-- Keep migration idempotent for installations that briefly ran an earlier
-- Stage 04 schema before result-point fields were added.
ALTER TABLE percenter_state_history ADD COLUMN IF NOT EXISTS result_ssp_bid Float64 AFTER candidate_margin;
ALTER TABLE percenter_state_history ADD COLUMN IF NOT EXISTS result_margin Float64 AFTER result_ssp_bid;

CREATE TABLE IF NOT EXISTS percenter_telemetry
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

-- Logical analytics views collapse any temporary physical replay by event_id.
-- The delivery worker also checks event_id before INSERT, so these FINAL views
-- are a second line of defense and do not depend on background merge timing.
CREATE VIEW IF NOT EXISTS percenter_state_history_logical AS
SELECT *
FROM percenter_state_history FINAL;

CREATE VIEW IF NOT EXISTS percenter_telemetry_logical AS
SELECT *
FROM percenter_telemetry FINAL;
