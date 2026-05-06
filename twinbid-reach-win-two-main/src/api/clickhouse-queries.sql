-- =============================================================================
-- ClickHouse queries for TwinBid dashboard statistics
-- Source table: ads.agg_stats (SummingMergeTree, partitioned by toYYYYMMDD(event_date))
--
-- These queries back the three dashboard sections:
--   1. Overview     — top KPI cards + per-campaign rows
--   2. Campaigns    — per-campaign aggregated metrics for the campaigns table
--   3. Statistics   — flexible group_by (date/hour/country/browser/device/os/site_id),
--                     plus filters and totals
--
-- All times are stored and returned in UTC 0.
-- Spend is computed as: spend_clicks_table + spend_views_table.
-- CTR (%) is computed as: clicks / impressions * 100, with safe-divide.
--
-- Bind parameters use ClickHouse's {name:Type} syntax. The backend should
-- substitute campaign_ids/user_id/date range/filters before execution.
-- =============================================================================


-- -----------------------------------------------------------------------------
-- 0. Common building blocks
-- -----------------------------------------------------------------------------

-- Filter mandatory: user_id (always isolate per user)
-- Filter optional: campaign_ids (array of UUID), date range [from, to],
--                  geo, browser, device_type, os, site_id (arrays of strings).
--
-- Recommended WHERE skeleton used in every query below:
--
--   WHERE user_id = {user_id:UUID}
--     AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
--     AND (length({creative_ids:Array(UUID)}) = 0 OR creative_id IN {creative_ids:Array(UUID)})
--     AND event_date >= {date_from:Date}
--     AND event_date <= {date_to:Date}
--     AND (length({f_geo:Array(String)})         = 0 OR geo         IN {f_geo:Array(String)})
--     AND (length({f_browser:Array(String)})     = 0 OR browser     IN {f_browser:Array(String)})
--     AND (length({f_device_type:Array(String)}) = 0 OR device_type IN {f_device_type:Array(String)})
--     AND (length({f_os:Array(String)})          = 0 OR os          IN {f_os:Array(String)})
--     AND (length({f_site_id:Array(String)})     = 0 OR site_id     IN {f_site_id:Array(String)})
--
-- Notes:
--  - For a single specific date, the frontend sends from = to (one day window).
--  - {creative_ids} narrows the result to chosen creatives; pass [] to disable.


-- =============================================================================
-- 1. OVERVIEW PAGE  (src/pages/DashboardOverview.tsx)
-- =============================================================================

-- 1a. Top KPI totals across all the user's non-draft / non-completed campaigns.
--     Returns a single row: impressions, clicks, spent, ctr.
SELECT
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0
       OR campaign_id IN {campaign_ids:Array(UUID)});

-- 1b. Per-campaign rows for the overview table (same query as 2a).
--     The frontend already calls api.statsQuery(group_by: ["campaign"])
--     and reads `campaign`, `impressions`, `clicks`, `spent`, `ctr`.


-- =============================================================================
-- 2. CAMPAIGNS PAGE  (src/pages/DashboardCampaigns.tsx)
-- =============================================================================

-- 2a. Per-campaign aggregated metrics. One row per campaign_id.
--     Maps to api.statsQuery({ group_by: ["campaign"] }).
SELECT
    campaign_id                                                            AS campaign,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0
       OR campaign_id IN {campaign_ids:Array(UUID)})
GROUP BY campaign_id;


-- =============================================================================
-- 3. STATISTICS PAGE  (src/pages/DashboardStatistics.tsx)
--    The UI sends one of the following `group_by` keys:
--      date | hour | campaign | country | creative |
--      os | browser | device_type | site_id
--    Each query returns columns: <bucket>, impressions, clicks, spent, ctr.
-- =============================================================================

-- 3a. Group by date
SELECT
    toString(event_date)                                                   AS date,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
GROUP BY event_date
ORDER BY event_date;

-- 3b. Group by hour (event_hour is DateTime('UTC'))
--     Returns "YYYY-MM-DD HH:00" string the UI parses with formatHourLabel().
SELECT
    formatDateTime(toStartOfHour(event_hour), '%F %H:00', 'UTC')           AS hour,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
GROUP BY toStartOfHour(event_hour)
ORDER BY toStartOfHour(event_hour);

-- 3c. Group by campaign
SELECT
    toString(campaign_id)                                                  AS campaign,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
GROUP BY campaign_id;

-- 3d. Group by country (geo column)
SELECT
    geo                                                                    AS country,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
  AND (length({f_geo:Array(String)}) = 0 OR geo IN {f_geo:Array(String)})
GROUP BY geo;

-- 3e. Group by creative
SELECT
    toString(creative_id)                                                  AS creative,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
GROUP BY creative_id;

-- 3f. Group by os
SELECT
    os                                                                     AS os,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
  AND (length({f_os:Array(String)}) = 0 OR os IN {f_os:Array(String)})
GROUP BY os;

-- 3g. Group by browser
SELECT
    browser                                                                AS browser,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
  AND (length({f_browser:Array(String)}) = 0 OR browser IN {f_browser:Array(String)})
GROUP BY browser;

-- 3h. Group by device_type
SELECT
    device_type                                                            AS device_type,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
  AND (length({f_device_type:Array(String)}) = 0 OR device_type IN {f_device_type:Array(String)})
GROUP BY device_type;

-- 3i. Group by site_id
SELECT
    site_id                                                                AS site_id,
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date}
  AND (length({f_site_id:Array(String)}) = 0 OR site_id IN {f_site_id:Array(String)})
GROUP BY site_id;


-- =============================================================================
-- 4. TOTALS (returned alongside `rows` in StatsQueryResponse.totals)
--    Same WHERE clause as the matching 3.* query, no GROUP BY.
-- =============================================================================
SELECT
    sum(impressions)                                                       AS impressions,
    sum(clicks)                                                            AS clicks,
    round(sum(spend_clicks_table + spend_views_table), 2)                  AS spent,
    round(if(sum(impressions) = 0, 0,
             sum(clicks) * 100.0 / sum(impressions)), 2)                   AS ctr
FROM ads.agg_stats
WHERE user_id = {user_id:UUID}
  AND (length({campaign_ids:Array(UUID)}) = 0 OR campaign_id IN {campaign_ids:Array(UUID)})
  AND event_date BETWEEN {date_from:Date} AND {date_to:Date};


-- =============================================================================
-- 5. Notes for the backend implementing /api/stats/query
-- =============================================================================
-- - Map the JSON field `campaign_ids` 1:1 to {campaign_ids:Array(UUID)}.
-- - Map `from`/`to` (YYYY-MM-DD) to {date_from:Date}/{date_to:Date}.
--   When both are empty strings, default to a wide range (e.g. last 90 days)
--   or omit the date filter entirely.
-- - Map `filters.country|browser|device_type|os|site_id` to the matching
--   {f_*:Array(String)} parameters; pass [] when absent.
-- - The frontend always sends a single value in `group_by`; pick the matching
--   query from section 3.
-- - Always run the totals query (section 4) with the SAME filters and return
--   it as `totals` in the response envelope.
-- - All timestamps are UTC 0; do not shift on the server.
