# Percenter production deployment contract

This document is the Stage 05 deployment checklist for the Simple/Complex percenter stack. It does not change the auction algorithm.

## Required configuration

ADV and the percenter daemon must receive the same policy values. Both processes now parse the same `PercenterPolicyConfig` and reject drift from the agreed production thresholds.

```env
# Existing Redis instance only; no new shard/instance.
REDIS_ADV_ADDR=<existing-redis-host:port>
REDIS_PASSWORD=<secret>
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
```

ADV additionally requires its existing PostgreSQL/snapshot configuration and:

```env
ADV_PERCENT_MAP_FILE_PATH=<writable-json-path>
ADV_PERCENTER_TELEMETRY_OUTBOX_PATH=./data/adv-percenter-telemetry-outbox.db
ADV_PERCENTER_TELEMETRY_FLUSH=1m
```

The percenter daemon requires the ClickHouse database that receives the ORTB rows containing `exact_segment_hash`, `segment_hash`, and `percenter_point_version`:

```env
CLICKHOUSE_HOST=<host>
CLICKHOUSE_PORT=9440
CLICKHOUSE_USERNAME=<user>
CLICKHOUSE_PASSWORD=<secret>
CLICKHOUSE_DB=<same-database-containing-ortb/impressions/clicks>
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

`BOT_BASE_URL` and `BOT_INTERNAL_SECRET` are optional for percenter operational telemetry. If either is absent, Telegram delivery is disabled and auction/optimizer processing continues. Existing antiperekrut startup-control rules are unchanged and may still require their bot configuration when that unrelated feature is enabled.

## Filesystem permissions

The service account must be able to create and fsync the parent directories of both bbolt files. A corrupt bbolt file is not silently truncated/recreated: startup fails with the original file preserved, because silently replacing it would violate durable-delivery guarantees.

## Migrations and topic order

1. Apply `migrations/001_percenter_stage01.sql` to PostgreSQL before deploying snapshot readers that expect `type_model` and `promo_spend_remaining`.
2. Apply `migrations/004_percenter_observability.sql` to the same ClickHouse database configured for the percenter daemon.
3. Ensure Kafka topic `percenter_observability` exists on the existing Kafka cluster. The repository Kafka topic bootstrap includes this topic.
4. Ensure the bbolt parent directories are writable by the ADV/percenter service users.
5. Deploy/restart the stats loaders, then ADV replicas, then the single active percenter daemon.

Temporary Redis DB7, Kafka, ClickHouse, or Telegram outages are degraded states. They must not block the auction hot path. Missing mandatory configuration or an unusable/corrupt local durable outbox is a startup configuration/storage error and is intentionally fail-closed.

## Verification after deployment

Check logs for the shared policy values and Redis DB7, verify `ALL=0.20` and `ALL_RTB=0.30` are physically present in the percent-map JSON, and confirm no growing `percenter:observability:ready` or local bbolt backlog after dependencies recover. Analytics that require logical deduplication should query `percenter_state_history_logical` / `percenter_telemetry_logical` or otherwise deduplicate by `event_id`.
