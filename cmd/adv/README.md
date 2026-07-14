# ADV runtime deployment notes

ADV, `kafkaredis`, and ADM Adapter connect to an already deployed Redis through environment variables. This repository does not deploy or configure Redis.

All three services must point to the same Redis server and use:

- DB 5 for `spent:*`, `pacing:*`, and `outbox:applied:*`;
- DB 6 for ADV winner hashes.

Required Redis connection variables are `REDIS_UUID_ADDR`, `REDIS_PASSWORD`, `REDIS_POOL_SIZE`, and `REDIS_MIN_IDLE_CONNS`. Keep `REDIS_DB_ADV_RUNTIME=5` and `REDIS_DB_ADV_WINNER=6`.

`ADV_SERVICE_CONTROL_URLS` must contain the actual HTTP control URLs of all ADV instances so the ADM Adapter can disable them after an accounting write error.

## SSP-domain quality maps

ADV uses the existing normalized `ssp_domain` passed through SSP Adapter,
Orchestrator, and Router. Quality membership is stored in three maps in
`ADV_QUALITY_MAP_FILE_PATH`:

- `usual`;
- `high`;
- `ultra`.

The same SSP domain may exist in one, two, or all three maps. For each campaign
ADV reads `quality_type` and checks the incoming `ssp_domain` only in the
corresponding map.

Read or atomically replace all three persisted maps:

```text
GET /filter/quality_map
PUT /filter/quality_map
```

The body has the same `usual`/`high`/`ultra` structure as
`adv_quality_map.json`. The update atomically replaces all three maps and may
contain the same SSP domain in several maps.

Read or replace one persisted map:

```text
GET /filter/quality_map?quality=usual
PUT /filter/quality_map?quality=usual
```

The PUT body is the complete replacement map for that segment:

```json
{
  "mc_moblivion.com": true
}
```

The same endpoints accept `quality=high` and `quality=ultra`. The replacement is
fully validated, persisted atomically, and only then published as a new
in-memory snapshot. Membership in other maps is preserved and overlapping SSP
domains across segments are allowed.

Debug the current in-memory maps with:

```text
GET /filter/debug_quality_map
GET /filter/debug_quality_map?quality=usual
```
