# ADV runtime deployment notes

The ADV service, `kafkaredis`, and ADM Adapter must use the same Redis server:

- DB 5 for `spent:*`, `pacing:*`, and `outbox:applied:*`;
- DB 6 for ADV winner hashes.

Redis must run with AOF enabled (`appendonly yes`, `appendfsync everysec`) and a
persistent data volume. The included compose file is an example for this setup.

`ADV_SERVICE_CONTROL_URLS` must contain the actual HTTP control URLs of all ADV
instances so the ADM Adapter can disable them after an accounting write error.

## Feed quality maps

ADV receives the original SSP `feed` UUID in parallel with the existing
`feed -> ssp_domain` conversion. The conversion and `ssp_domain` behavior are
unchanged. Quality is selected only by membership of the original feed UUID in
one of three maps stored in `ADV_QUALITY_MAP_FILE_PATH`:

- `usual`;
- `high`;
- `ultra`.

A feed UUID may exist in one, two, or all three maps. ADV does not assign one
exclusive segment to a feed. For each campaign it reads `quality_type` and
checks the incoming feed UUID only in the corresponding map. The checked-in
file places all feeds currently configured in `cmd/spp-adapter/spp-adapter.env`
into `usual`, while `high` and `ultra` start empty.

Read or atomically replace all three persisted maps:

```text
GET /filter/quality_map
PUT /filter/quality_map
```

The body has the same `usual`/`high`/`ultra` structure as
`adv_quality_map.json`. The update atomically replaces all three maps and may
contain the same feed UUID in several maps.

Read or replace one persisted map:

```text
GET /filter/quality_map?quality=usual
PUT /filter/quality_map?quality=usual
```

The PUT body is the complete replacement map for that segment:

```json
{
  "a15c30da-6dea-4945-a5cf-40bb34b1047b": true
}
```

The same endpoints accept `quality=high` and `quality=ultra`. The replacement is
fully validated, persisted atomically, and only then published as a new
in-memory snapshot. Membership in other maps is preserved and overlapping feed
UUIDs across segments are allowed.

Debug the current in-memory maps with:

```text
GET /filter/debug_quality_map
GET /filter/debug_quality_map?quality=usual
```
