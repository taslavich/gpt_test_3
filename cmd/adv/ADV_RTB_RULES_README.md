# ADV RTB-only BidRequest rules

This is a **second, independent filter layer** for RTB campaigns, not a replacement
for existing ADV campaign filters, quality maps or antipererekrut. It does not
apply to ordinary campaigns. BannerPartner's existing user-scoped BAN handling
is unchanged.

## Files and endpoints

- `ADV_RTB_RULES_MAP_FILE_PATH=./adv_rtb_rules_map.json` in the existing ADV env.
- `adv_rtb_rules_map.json`: **production startup sample** with all three existing
  54Ads campaigns migrated exactly from hardcoded `rtb.go` normalization. Deploy
  this file with the ADV executable, or set the env variable to its absolute path.
- `adv_rtb_rules_example.json`: independent **syntax example** for analysts,
  including VID and IPP examples, campaign grouping, filters, add, replace,
  delete and rename. The dummy IDs
  are *not* enabled in the production map.
- `GET /filter/rtb_rules_map`: persisted, grouped configuration read from disk.
- `GET /filter/debug_rtb_rules_map`: active, expanded configuration in RAM.
- `PUT /filter/rtb_rules_map`: validates the entire JSON, atomically saves and
  swaps the RAM snapshot (204 on success; 400 on validation errors).

Manual edits to disk take effect on the **next ADV restart only**. A missing or
invalid configuration is a fatal startup error. Runtime traffic does not read
files. Invalid PUT does not replace the previous snapshot or persisted file.

## JSON syntax

Top-level keys are one or more *campaign UUIDs* separated by commas, or `ALL`
for every RTB campaign. The same UUID may appear in other keys. All keys are
expanded to one in-memory entry per campaign; overlapping module+field rules
are overwritten with no guaranteed precedence (Go map iteration). `ALL` is
merged when looking up each campaign, including UUIDs not mentioned in the file.
The persisted JSON retains the original grouping.

Each group may have five arrays: `filters`, `add`, `replace`, `delete`, `rename`.
Omitted arrays mean no action.

**Filter:** `{ "field":"imp.video.protocols",
"isWhiteList":true, "values":[3] }`. For array
fields, whitelist membership means **contains any specified value**. For
scalar fields, it means equality to any specified value. Blacklist passes only
when no configured value matches. A missing field cannot satisfy whitelist.
Alternative existence filter: `{ "field":"device.make",
"exists":true }`, or `false` to require absence. Exactly one of `values`, `exists`, and `compare` must be specified.
Use separate filter objects for independent conditions.

**Native JSON-string filters (RTB second layer only):** `imp.native.request`
is a protobuf **string** whose contents can be JSON. Add `json_path` to a
filter to read fields *inside* that string, for example:

```json
{"field":"imp.native.request", "json_path":"assets[id=102].img.wmin", "compare":">=", "value":200}
{"field":"imp.native.request", "json_path":"assets[0].title.len", "values":[80], "isWhiteList":true}
```

`json_path` uses dot-separated JSON object keys, numeric array indices
(`[0]`), or an asset-ID selector (`[id=102]`) that does not depend on array
position. These paths are **not** additional protobuf fields. The embedded
string is decoded once per impression for the whole incoming BidRequest and
reused across filters and RTB campaigns; no native JSON is parsed when no
`json_path` is configured. The original string is never modified. The
optional numeric `compare` supports `>`, `>=`, `<`, `<=` and requires a numeric
`value`. Values and operators are checked at map load/HTTP PUT time.
Malformed native JSON or a JSON-path type mismatch rejects only that
RTB campaign's impression (no HTTP request sent for that impression) and
records **-6** under its campaign UUID in `ADV_RTB_RESPONSES`. Missing
paths and valid but nonmatching values are normal filter rejects and **do
not** create a new error code. Earlier ADV filtering stages remain unchanged.
Missing or null `native.request` is treated as an absent field, not invalid
JSON. The cache is per BidRequest and is discarded after its preflight. Timestamps,
HTTP status codes, and other existing RTB statistics are not changed.

**Add:** `{ "field":"device.geo.country", "value":"PHL" }` fills a
known protobuf field only if missing/null, before serialization; present
values are not overwritten. To create a new JSON-only extension, specify an
exact known protobuf object parent and a nested `postfix_field`, for example
`{ "field":"imp.ext", "postfix_field":"type", "value":"pop" }`.
This creates `imp.ext.type` *after* protobuf-to-JSON serialization. Parents
may be shared across rules. A scalar parent or protobuf field collision is
invalid. Neither form creates extra impressions. `add` has no mode.

**Replace:** `{ "field":"tmax", "mode":"set", "value":500 }` changes an
existing field only. `set_or_add` restores the combined behavior for a known
protobuf field: it fills the missing field using ordinary protobuf `add`
semantics, then replaces its value (including an existing zero). For example,
`{ "field":"tmax", "mode":"set_or_add", "value":300 }` ensures `tmax=300`
without a separate `add` rule. `set` and ordinary `add` are unchanged;
`set_or_add` does not support JSON-only `postfix_field` paths.
`countries` maps specified two-letter countries to three-letter countries,
e.g. `{"field":"device.geo.country","countries":
{"PH":"PHL"},"copy_to":"user.geo.country"}`. Do not specify `mode` with `countries`; these are mutually exclusive.
`iso` converts any recognized ISO 3166-1 alpha-2 country to alpha-3, e.g.
`{"field":"device.geo.country","mode":"iso"}`.
`copy_to` copies the final source value to any supported compatible target,
overwriting an existing value or creating a missing field. If the source field
is absent, nothing is copied. It works with any replace mode. Input and target
types are checked when the map is loaded or updated via PUT. For impression
fields both endpoints must be impression-scoped, so different impressions are
never mixed.
Normalizers: `hostname` only for `site.domain` and `full_page` only for
`site.page`. If either is missing but the other is present, its value is
reconstructed from the other. If both are missing, no field is created and
no extra filter rejects the request.

**Delete:** `{ "field":"imp.banner" }`. Clears only protobuf pointer fields (`nil`) on the campaign-local copy, before encoding. Absent fields are ignored. Paths not backed by a pointer are rejected on map validation. Rules that overlap a deleted path are rejected. With `imp.*` rules, actions apply
to all impressions in the **already campaign-filtered outgoing copy**.

For `add`, `replace` and `delete`, multiple fields may be given with commas.
They remain grouped in the JSON file, but are independent validated entries
in memory. Every field must support the operation: `countries` cannot mix
`device.geo.country` with `tmax`; `hostname` only targets `site.domain`;
`full_page` only targets `site.page`. `filters`, `rename` and `copy_to` do not
support combined source fields.

All supported protobuf paths are explicitly defined in `internal/services/adv/rtbrules/schema.go`. Unknown paths, types, and operations are rejected. `formats` is **not supported**: a campaign has one format.
**Rename:** `{ "field":"device.deviceType", "to_field":"devicetype" }`. `to_field` names only the terminal key; the parent path is inherited from `field`. Executes last, preserving the value and removing the old JSON key. All other modules use original protobuf field names; additional JSON-only keys are introduced only by `add.postfix_field`.
Logical order: existing ADV checks → RTB filters → **add → replace → delete → rename** → HTTP request to the RTB advertiser. Known-field additions and pointer-only deletion operate on the request copy before protobuf-to-JSON serialization; JSON-only postfix fields, replace and rename operate on the outgoing JSON. Overlapping delete/add/replace paths are rejected.

### Preserving behavior of 54Ads

The three UUIDs in `adv_rtb_rules_map.json` reproduce the previous:
`CN→CHN`, `PH→PHL`, `MX→MEX`, `PE→PER`; user country receives the final device country via `copy_to` (overwrites an
existing user country); `tmax=500`; `deviceType→devicetype`; site hostname
and full-page URL; `imp.ext.type=pop`; `imp.banner` deleted for the existing POP campaigns. Campaigns have a fixed format, so there is no per-rule `formats` condition. This behavior can now be changed via the map rather than code.

Protobuf additions are *additive only*: `device.make/model/osv`, `user.geo`,
`site.publisher.domain` and `imp.video.sequence`. No existing field was removed
or renumbered. These fields remain available for the new rules when the source
SSP actually supplies them.

**Important:** The video and IPP example rules demonstrate filtering on actual
source capabilities. They are not 54Ads production settings. Do not put them
in the active map until the campaign and placement requirements are agreed.
