# Percenter production deployment contract

This is the final Stage 01-06 deployment/hardening checklist for the Simple/Complex percenter stack. It does not change routing, pricing, optimizer state machines, thresholds, step sequences or RTB semantics.

## Shared policy and required configuration

ADV and the percenter daemon must receive the same policy values. Both processes parse the same `PercenterPolicyConfig`, validate canonical values before normalization and log the same deterministic policy fingerprint.

```env
# Existing Redis instance only; no new shard/instance.
REDIS_ADV_ADDR=<existing-redis-host:port>
REDIS_PASSWORD=<secret-if-required-by-that-instance>
REDIS_DB_ADV_PERCENTER=7

SIMPLE_OPTIMIZE_INTERVAL=5m
SIMPLE_REBENCHMARK_INTERVAL=6h
SIMPLE_STATE_TTL=168h
SIMPLE_MIN_IMPRESSIONS=5
SIMPLE_WIN_RATE_RETENTION=0.5
SIMPLE_MARGIN_SEARCH_STEPS=5,2,1
SIMPLE_MAX_MARGIN=0.9

COMPLEX_OPTIMIZE_INTERVAL=5m
COMPLEX_REBENCHMARK_INTERVAL=6h
COMPLEX_STATE_TTL=168h
COMPLEX_MIN_IMPRESSIONS=5
COMPLEX_BUYOUT_RETENTION=0.8
COMPLEX_EFFICIENCY_RETENTION=0.8
COMPLEX_SSP_SEARCH_STEPS=10,5,2,1
COMPLEX_MARGIN_SEARCH_STEPS=10,5,2,1
COMPLEX_MAX_MARGIN=0.9

# Independent recovery retention, not optimizer StateTTL.
HISTORY_PENDING_TTL=720h
```

`SIMPLE_STATE_TTL` and `COMPLEX_STATE_TTL` are positive storage TTLs and do not participate in optimizer-point compatibility. `HISTORY_PENDING_TTL` is intentionally independent from both state TTLs.

The default `HISTORY_PENDING_TTL=720h` is an explicit 30-day operational retention for a committed history transition that is still mirrored in Redis DB7 before it has been made durable in the local bbolt outbox/recovery path. There is no separate project-approved business minimum, so startup validation enforces only `>0` rather than inventing a new threshold. Operations must keep the configured value large enough for the expected Redis/ClickHouse/Kafka incident window, restart/deployment time and recovery backlog. Once a record is in the local bbolt outbox, downstream Redis/Kafka/ClickHouse retries are retained by that durable outbox until acknowledgement; shortening Simple/Complex StateTTL does not shorten this recovery chain.

## ADV-specific configuration

ADV still requires its existing PostgreSQL, snapshot, percent/quality map and VPN configuration. Stage 01-06 additionally relies on:

```env
POSTGRES_DSN=<secret-dsn>
ADV_PERCENT_MAP_FILE_PATH=<percent-map-json-path>
ADV_PERCENTER_TELEMETRY_OUTBOX_PATH=./data/adv-percenter-telemetry-outbox.db
ADV_PERCENTER_TELEMETRY_FLUSH=1m  # Redis relay cadence, not minute cut-over
```

The percent-map file must always contain both `ALL` (ordinary fallback) and `ALL_RTB` (RTB fallback). Their numeric values are runtime configuration, not hard-coded business defaults, and may be changed within the accepted 0..90% range. A campaign-specific entry always has priority over `ALL`/`ALL_RTB`, even when it is lower. Missing fallback keys are a fail-closed startup/update error. `REDIS_DB_ADV_RUNTIME=5`, `REDIS_DB_ADV_WINNER=6` and `REDIS_DB_ADV_PERCENTER=7` are canonical ADV DB assignments and invalid values fail startup with the ENV name, actual value and required value.

Promo semantics are uniform across ordinary and RTB traffic and across `type_model=1/2/3`: while `promo_spend_remaining > 0`, the effective minimum is `max(resolved_map_percent, 30%)`. `resolved_map_percent` is campaign-specific when present, otherwise `ALL`/`ALL_RTB`. When promo is exhausted, the 30% promo floor disappears and the current resolved map value is used. Promo spend continues to decrement on billed traffic even when the resolved map value is already above 30%. RTB `type_model=2` still bypasses the Complex optimizer; promo only floors its exact map/fallback percentage.


## Percenter-daemon configuration

The daemon must use the same production ClickHouse host/database and ORTB/impression/click tables that `clickhouse-loader` writes, plus the existing Kafka cluster. Credentials stay in deployment secrets; do not copy a second/stale credential set into source-controlled env files:

```env
CLICKHOUSE_HOST=<host>
CLICKHOUSE_PORT=9440
CLICKHOUSE_USERNAME=<user>
CLICKHOUSE_PASSWORD=<secret>
CLICKHOUSE_DB=<database-containing-ortb-impressions-clicks>
CLICKHOUSE_TABLE_ORTB=ortb
CLICKHOUSE_TABLE_IMPRESSIONS=impressions_in
CLICKHOUSE_TABLE_CLICKS=clicks_in

PERCENTER_OUTBOX_PATH=./data/percenter-observability-outbox.db
PERCENTER_RELAY_INTERVAL=1m
PERCENTER_HISTORY_TABLE=percenter_state_history
PERCENTER_TELEMETRY_TABLE=percenter_telemetry
KAFKA_BROKERS=<existing-kafka-brokers>
KAFKA_TOPIC_PERCENTER=percenter_observability
PERCENTER_DIGEST_INTERVAL=30m
```

`KAFKA_BROKERS` and a non-empty `KAFKA_TOPIC_PERCENTER` are required. A configured but temporarily unreachable broker is a degraded dependency rather than a startup blocker because Redis/bbolt retain pending delivery.

`BOT_BASE_URL` and `BOT_INTERNAL_SECRET` are optional for percenter/telemetry alerts. Both must be present to enable Telegram; otherwise Telegram is disabled and processing continues. Existing antiperekrut startup-control requirements are unchanged and may still require bot configuration when that separate feature is enabled.

## bbolt files and filesystem permissions

Two local durable files are used by this stack:

- ADV telemetry: `ADV_PERCENTER_TELEMETRY_OUTBOX_PATH` (default `./data/adv-percenter-telemetry-outbox.db`);
- percenter history/observability: `PERCENTER_OUTBOX_PATH` (default `./data/percenter-observability-outbox.db`).

Create/mount their parent directories on persistent storage. The service account must be able to create, read, write, lock and fsync the files/directories. Do not place them on an ephemeral filesystem if restart durability is required. A corrupt/unopenable bbolt file fails startup and is not silently truncated or recreated.

## ADV telemetry durability semantics and observability

`ADVTelemetry.Record()` remains RAM-only and performs no synchronous disk/network I/O in the auction hot path. Logical buckets are one UTC calendar minute wide (`ADVTelemetryMinuteBucketPeriod=1m`). A background worker wakes for the nominal minute boundary and durably closes older buckets into bbolt.

The one-minute value is **not** a strict wall-clock hard-crash-loss upper bound. Userspace execution of the boundary callback may be delayed by process freeze, stop-the-world pauses, CPU starvation, VM pause or OS scheduling. The exact semantics are:

- after a minute boundary has actually been processed successfully, the previous closed minute is durable;
- RAM-only exposure is the current open bucket plus any scheduler lateness before the worker processes a boundary;
- graceful shutdown calls `FlushAll()` and durably closes the open bucket;
- SIGKILL/power loss can lose only buckets/counters the worker has not yet made durable.

Each processed boundary logs `PERCENTER_TELEMETRY_FLUSH_STATUS` with the scheduled boundary, actual processing time/boundary lateness, last successfully durable closed minute, oldest RAM-only closed-bucket age, pending closed bucket/counter counts, durable outbox count, and last flush error/time. Lateness beyond the warning threshold and durable/relay failures use transition-based `ERROR -> silence -> RECOVERED` reporting, so retry loops do not send Telegram on every retry.

## Kafka consumer contract

Physical Kafka delivery is at-least-once. The final consumer is outside this repository. Consumer storage MUST atomically combine the `event_id` dedupe marker and the logical mutation in one transaction/equivalent atomic primitive. A split `exists(event_id) -> apply` sequence and a permanent pre-claim before a non-transactional mutation are invalid. See `docs/percenter-kafka-consumer-contract.md` for the integration API contract.

## Migration and deployment checklist

1. Back up/verify the current percent-map JSON and PostgreSQL/ClickHouse migration targets.
2. Apply `migrations/001_percenter_stage01.sql` to PostgreSQL. It adds/validates `campaigns.type_model`, adds `users.promo_spend_remaining`, and creates the idempotent `adv_promo_spend_events` ledger used by promo-spend logic.
3. Apply `migrations/004_percenter_observability.sql` to the configured ClickHouse database. It adds `ortb.exact_segment_hash`, creates `percenter_state_history` and `percenter_telemetry`, and creates their `_logical` views.
4. Ensure the existing Redis instance is reachable as `REDIS_ADV_ADDR` and logical DB7 is available for Simple/Complex state and recovery indexes. Do not add a new Redis shard/instance.
5. Ensure Kafka topic `KAFKA_TOPIC_PERCENTER` (default `percenter_observability`) exists on the configured existing Kafka cluster.
6. Create/mount the two bbolt parent directories and verify service-user read/write/lock/fsync permissions.
7. Deploy the Stage 01-06-compatible stats/ClickHouse ingestion components after the ClickHouse migration so the new attribution columns are accepted.
8. Deploy/restart the percenter daemon and ADV replicas with the same policy ENV. Starting the daemon before ADV is operationally preferable because it can drain immediately, but it is not a correctness requirement: ADV has the local bbolt/Redis recovery path for temporary downstream unavailability.
9. Compare the ADV and percenter startup logs: `fingerprint`, Simple/Complex StateTTL, `HISTORY_PENDING_TTL`, Redis DB7, delivery-component enabled/disabled status and initial degraded dependencies must agree with the deployment. Logs must not contain Redis/PostgreSQL/ClickHouse/Kafka credentials, Telegram token/secret or credential-bearing DSNs.
10. Verify that both `ALL` and `ALL_RTB` exist in the deployed percent map with the intended current values; confirm `percenter:observability:ready`, pending-history indexes and both local bbolt backlogs stop growing after dependencies recover.
11. For analytics, read `percenter_state_history_logical` / `percenter_telemetry_logical` or otherwise deduplicate by stable `event_id`.

Temporary Redis DB7, Kafka, ClickHouse or Telegram outages are degraded states and do not introduce synchronous telemetry I/O into the auction path. Missing mandatory configuration, an invalid canonical policy value or an unusable local durable outbox remains a fail-closed startup error.
