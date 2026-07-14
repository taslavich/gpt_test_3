# ADV runtime deployment notes

The ADV service, `kafkaredis`, and ADM Adapter must use the same Redis server:

- DB 5 for `spent:*`, `pacing:*`, and `outbox:applied:*`;
- DB 6 for ADV winner hashes.

Redis must run with AOF enabled (`appendonly yes`, `appendfsync everysec`) and a
persistent data volume. The included compose file is an example for this setup.

`ADV_SERVICE_CONTROL_URLS` must contain the actual HTTP control URLs of all ADV
instances so the ADM Adapter can disable them after an accounting write error.

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
