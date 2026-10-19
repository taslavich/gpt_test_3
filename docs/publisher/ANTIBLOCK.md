# Popunder anti-block: incremental extension

Bases: proj225(3).zip / FProj226(1).zip, with FINAL and the optional PATCH traffic_type
fix already applied. AGENTS.md is absent in both. No older archive/patch was used.

## Behavior and boundaries

Default PUB_ANTIBLOCK_ENABLED=false. Existing /static/adManager.js and all old URLs
remain. The SDK also works with the flag off: it loads the old runtime using
PUB_PUBLIC_URL, without infrastructure selection or a new auction protocol.

When enabled, a stable /sdk.js loads /infra.json from the pinned stable origin,
then a production build of the SAME Popunder runtime from an available pool origin.
The child runtime uses anonymous CORS and a pinned SHA-384 SRI hash. Transport
fetches reject redirects; a session token cannot follow a 307 to an unknown origin.
Infrastructure caches are scoped by stable origin/fingerprint, not shared across
different infrastructure configurations.
It passes data-ad-code-id through the actual dynamically created script tag and
keeps routing context in a WeakMap. Duplicate SDK/legacy/late script executions
share __twinbidPopV1, one page session, click listener and quota per Code. Nonce is
passed to the child script. SPA/history/BFCache behavior remains the existing behavior.

A separate infrastructure worker (inside pub) checks five official subscriptions
and public health endpoints. Node 22+ and the locked @adguard/tsurlfilter 6.0.3
helper parse network rules: domain boundaries, URL paths, script/XHR/document/popup,
third-party/source domain, exceptions, important/badfilter and request methods (including HEAD and CORS OPTIONS).
The helper is an external GPL-3.0-only dependency, installed unmodified by npm;
it is not linked into Go or sent to browsers. List text/rules are never executed.
Cosmetic/scriptlet rules and real browser/extension heuristics are not simulated.
Configured sample publisher pages matter for domain-scoped rules; this finite scan
cannot certify every publisher, every query-dependent rule or all blocker settings.

No list processing on /request. The worker defaults to one attempt per 24 hours
(including failed attempts); a 15-second poll distributes the validated snapshot.
The snapshot, attempt marker and fenced refresh lease use the separate Redis namespace
`twinbid:pubinfra:v1:{domains}:*`, not ORTB, ready sets, zone state or money objects.
A shared lease prevents simultaneous writers; an expired owner cannot publish.
All pub instances must use identical settings. A different fingerprint, corrupt or
lost snapshot fails closed for infrastructure; a Redis transport outage retains the
last valid in-memory state. A restart reloads the persistent shared snapshot.
A fresh installation seeds only the explicitly trusted pool, marked unverified,
until the first full check. Use persistent/noeviction Redis as already required by pub.

Five complete downloads + matching + probes must succeed before publishing a new
snapshot. Errors, oversized/HTML lists, redirects and helper failures retain the last
valid configuration. Domain unavailability requires two failed 3-second public
health probes. Probes run with at most four concurrent workers; the job is bounded
by 4 minutes, lease by 5 minutes. TLS/DNS probes reject private/reserved IPs, DNS
rebinding and redirects, and do not inherit an HTTP proxy. There is no arbitrary
list URL or outbound target supplied by an advertising request. List sources:

- https://easylist.to/easylist/easylist.txt
- https://easylist.to/easylist/easyprivacy.txt
- https://pgl.yoyo.org/adservers/serverlist.php?hostformat=plaintext&showintro=0&mimetype=plaintext
- https://filters.adtidy.org/extension/ublock/filters/2.txt
- https://filters.adtidy.org/extension/ublock/filters/3.txt

If a valid check finds every pool domain unsuitable, /infra.json returns 503;
no unknown fallback domain is invented. With AUTO_ROTATE=false, each component
stays on its explicitly preferred origin (or the first configured origin); checks
and metrics still run and report blocking. Manual mode is an operator decision,
not a declaration that a blocked domain is clean. The stable origin is monitored
separately; its loss cannot be repaired by a script that never downloaded.

Config and POST /request use fetch, JSON, credentials:'omit', existing signed session
and request_id. Only network failure triggers the existing sequential retry loop:
maximum three attempts, same ID/body, no fan-out. HTTP business refusal/no-fill never
starts another auction. The optional X-TwinBid-Infrastructure header selects a
SERVER-configured delivery prefix; clients cannot submit an endpoint/open target.
All instances share existing HMAC secrets and Redis reservations/cache.

Runtime/API/delivery origins can differ. open_url must be a pinned HTTPS origin,
port 443, the exact delivery prefix and UUID.HMAC token, without userinfo, query,
fragment or extra path. A HEAD probe may switch only the bridge origin, preserving
the signed token and reservation; it never confirms or starts an auction. At most
three sequential HEAD probes are made. A document-only browser block may still
occur after a successful HEAD: this cannot be ruled out by network probing.
The bridge makes ONE /confirm POST, then follows the exact existing ADM. /confirm
still only confirms bridge opening; it does not create billing/tracking/money events.
ADM/BURL/DSP creative URLs are not rewritten or proxied.

Diagnostics: bounded component/reason counters at /infra/event and /metrics;
no visitor/session/token/page URL or financial payload is submitted. Events are
observations, not proof of an extension. A fully blocked SDK cannot report itself;
a blocked delivery document cannot execute its own reporter. DOM/network errors,
CORS and popup restrictions may have other causes. Optional publisher UX uses a
plain span and the `twinbid:availability` CustomEvent; no modal or forced navigation.
Example opt-in text: data-antiblock-message="Реклама сейчас недоступна."
Do not present a network error as proven AdBlock use.

## ENV

Existing pub, SSP, Redis, callback, financial ENV remains unchanged.

| ENV | Default / meaning |
|---|---|
| PUB_ANTIBLOCK_ENABLED | false |
| PUB_ANTIBLOCK_STABLE_ORIGIN | empty; required HTTPS origin when enabled |
| PUB_ANTIBLOCK_DOMAINS | empty; required comma-separated 1..32 trusted HTTPS origins, usually 5..15 |
| PUB_ANTIBLOCK_ROUTES_JSON | defaults below; partial object overrides known keys only |
| PUB_ANTIBLOCK_RUNTIME_ORIGIN | empty: first available pool origin |
| PUB_ANTIBLOCK_API_ORIGIN | empty: first available pool origin |
| PUB_ANTIBLOCK_DELIVERY_ORIGIN | empty: first available pool origin |
| PUB_ANTIBLOCK_AUTO_ROTATE | true; false pins each component |
| PUB_ANTIBLOCK_CHECK_INTERVAL | 24h; accepted 1m..168h |
| PUB_ANTIBLOCK_POLL_INTERVAL | 15s; accepted 1s..60s |
| PUB_ANTIBLOCK_HTTP_TIMEOUT | 10s per download; accepted 1s..30s |
| PUB_ANTIBLOCK_MAX_LIST_BYTES | 16777216 per list; accepted 1024..33554432 |
| PUB_ANTIBLOCK_FILTER_SCRIPT | scripts/pub-client/filter.cjs; use an absolute installed path in systemd |
| PUB_ANTIBLOCK_NODE | node; use an absolute Node 22+ executable if necessary |
| PUB_ANTIBLOCK_SAMPLE_PAGES | https://publisher.invalid/; replace with representative real publisher HTTPS pages (max 64); no request is sent to them |
| PUBLISHER_PUBLIC_SCRIPT_URL (cabinet) | empty: old public_script_url; set to the stable HTTPS SDK URL to issue new snippets |

Default path map:

```json
{"sdk":"/sdk.js","infrastructure":"/infra.json","runtime":"/assets/runtime.js","config":"/config","request":"/request","delivery":"/delivery/","confirm":"/confirm/","diagnostics":"/infra/event","health":"/infra/health"}
```

Paths are canonical local paths, not URLs. Delivery/confirm prefixes end in '/'.
Unknown, colliding, overlapping, encoded or traversal paths are refused at startup.
Legacy paths remain registered even with aliases. Do not rotate route paths per
request. Keep aliases across a disable/restart to complete in-flight reservations.
The fingerprint also includes the runtime build hash. Keep pool/routes/settings and
binaries consistent across pub instances; changing the pool or
paths requires a coordinated fleet update and invalidation of cached sdk.js.
Daily changes of domain availability within the configured pool do not change pins.

## Build and deploy

Apply both incremental patches to their supplied archive versions (see README_APPLY).
No database migration, finance change or new RTB Redis object is needed.
From the RTB repository root:

```bash
npm ci --prefix scripts/pub-client --ignore-scripts
npm run build --prefix scripts/pub-client
npm run check --prefix scripts/pub-client
go build -o /usr/local/bin/twinbid-pub ./cmd/pub
```

Generated sdk.prod.js/runtime.prod.js are included and embedded, so the existing
flag-off binary can be built without Node. Enabling monitoring requires Node 22+
and installed runtime dependencies. Keep scripts/pub-client/{filter.cjs,package*.json,
node_modules} on the server and set PUB_ANTIBLOCK_FILTER_SCRIPT to its absolute path.
Production minification uses pinned esbuild 0.25.12 and identifier mangling, without
property mangling, eval, string packers or obfuscated Go/business logic. Readable
sdk.js/routing.js/adManager.js stay in the repository; --check detects stale output.

1. Register domains manually, install DNS and certificates. Do not use the test
   domains from tests in production. Use no wildcard/untrusted origins in the pool.
2. Configure ALL pool and stable hosts to proxy to the same pub fleet. Share
   existing Redis/ORTB shard order, delivery/internal secrets and config epochs.
3. Set identical pool/routes/preferences/sample pages on every pub instance.
   Run Node filter tests, pub Redis tests and real browser/profile checks on a test
   Ad Code before enabling. Check /infra.json and /metrics after the first scan.
4. Set PUB_ANTIBLOCK_ENABLED=true in pub.env, restart ONLY pub. Set the optional
   cabinet PUBLISHER_PUBLIC_SCRIPT_URL to https://STABLE_DOMAIN/sdk.js, restart
   cabinet, and issue new snippets with the SAME data-ad-code-id attribute:

```html
<script async src="https://STABLE_DOMAIN/sdk.js" data-ad-code-id="tb_EXISTING_CODE"></script>
```

Do not change the Ad Code ID, visitor/session storage, quotas or financial interval.
Old published /static/adManager.js snippets continue to use the old same-origin
request/delivery path. Cabinet's empty override continues returning that old URL.
The frontend must use the returned public_script_url rather than construct a URL;
frontend sources are not present and have not been altered.

## DNS/TLS/Nginx

Replace STABLE_DOMAIN, WORK_A_DOMAIN, WORK_B_DOMAIN and CERT paths with domains
and certificate files you actually control. A/AAAA point to your public edge.
Each exact hostname needs a valid certificate/SAN and HTTPS port 443. No DNS
registration or certificate purchase is automated. Existing RTB/SSP/ADM/BURL
server blocks, locations and proxy_pass are NOT edited.

Example NEW virtual host for pub only (reuse your actual pub upstream address):

```nginx
server {
    listen 443 ssl;
    server_name STABLE_DOMAIN WORK_A_DOMAIN WORK_B_DOMAIN;
    ssl_certificate     /CERT/fullchain.pem;
    ssl_certificate_key /CERT/privkey.pem;
    location / {
        proxy_pass http://127.0.0.1:8087;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_next_upstream off;
        proxy_connect_timeout 3s;
        proxy_read_timeout 15s;
        proxy_send_timeout 15s;
    }
}
```

Add a separate default/unrecognized-host block that returns 444 at the edge.
Only an actually trusted reverse proxy belongs in PUB_TRUSTED_PROXIES. Preserve
client IP/UA; do not fake these values in the public snippets. For multiple pub
instances use your pub upstream pool in this NEW block, not an advertiser upstream.
Preserve URIs, custom header and Content-Type; do not strip query/session values or
replay POST upstream. OPTIONS must reach pub. pub owns wildcard CORS for this
cookie-free public API; do not add Access-Control-Allow-Credentials or contradictory
CORS headers at Nginx. SDK caching is 300s, runtime 60s, infrastructure is no-store.
Restrictive publisher CSP must allow the stable and pool script/connect origins
(or propagated nonce/strict-dynamic as appropriate). Do not replace bridge CSP or
its existing ADM callbacks. A TLS/CORS/DNS misconfiguration is not an AdBlock result.

## Disable and rollback

Set PUB_ANTIBLOCK_ENABLED=false on the whole pub fleet and restart pub. Keep the
configured domain hosts/routes/secrets in place for in-flight tokens. A cached SDK
can accept a controlled disable only to its PINNED legacy runtime origin; an active
runtime refresh returns to old endpoints without a new page session. In-flight
header requests fall back only to the server's old delivery origin. This cannot
redirect to a client-supplied endpoint. Default flag-off CORS remains the old form;
a configured pool retains the extra header during disable for in-flight clients.

Clear PUBLISHER_PUBLIC_SCRIPT_URL and restart cabinet to issue old snippets again.
No RTB/CH/PG schema rollback or financial reconciliation is introduced by this feature.
For code rollback, first disable, allow >=5 minutes for old SDK cache plus in-flight
requests/tokens, and return issuer URL to the old value. Keep a compatibility SDK
at the stable published URL if new snippets are already installed: do not remove
that endpoint while publishers still use it. Code can be reversed only after this
operational compatibility is handled. No destructive Redis/ledger/CH cleanup.

For a deliberate coordinated pool/routes fingerprint replacement, stop the
infrastructure worker on ALL pub instances, change identical settings and then
remove ONLY the three infrastructure namespace keys using an authorized operator
workflow. Never flush Redis or modify publisher ready/frequency/ORTB/finance keys.
A forced immediate recheck clears ONLY the infrastructure attempt marker while
no refresh lease is held; it does not invalidate the snapshot or cached auctions.

## Original requirements coverage

| Original file item | Result |
|---|---|
| Network/domain layer | Implemented: trusted pool, full URL/type/context matching, fenced daily checks and sequential failover |
| Cosmetic class/ID layer | Not applicable to the Popunder creative: no banner container; no artificial/random markup |
| JS behavior layer | Existing click/popup behavior preserved; minimal routing/diagnostic additions; actual extension behavior requires external testing |
| A: 5–15 domains + daily rotation | Technical implementation complete, configurable 1..32; registration/DNS/TLS are external actions |
| B: neutral names/subdomains | Operator choice; only explicitly listed origins accepted, no random DNS creation or promise that a name remains unblocked |
| C: minification/obfuscation | Reproducible minification + moderate identifier mangling; readable sources retained; random CSS irrelevant |
| D: own-domain creatives/CDN | pub-owned SDK/runtime/bridge can use the pool; DSP-owned ADM/creative/tracker is untouched and not proxied |
| E: modest JS + fetch/JSONP | Existing JSON POST/fetch/credentials:omit retained; JSONP not needed; no new cookies/redirect chains |
| F: optional UX | CustomEvent and opt-in plain status message; complete bootstrap/document blocking cannot self-report |
| G: Acceptable Ads | External organizational application/criteria; Popunder eligibility not established, no compatibility claim |
| Stable two-stage bootstrap | Implemented; loss of stable hostname still requires publisher/edge/operator action |
| Separate request/delivery origins | Implemented with pinned HTTPS origins/paths/tokens and same reservation/ADM |
| JSONP vs fetch/CORS | Existing protected POST retained, OPTIONS supported; no token-bearing JSONP execution |
| “80–90%”/“85–90%” claims | Not adopted: no measured bypass percentage or guarantee |

Official context: https://easylist.to/ , https://pgl.yoyo.org/as/ ,
https://adguardteam.github.io/AdguardFilters/ ,
https://github.com/AdguardTeam/tsurlfilter/tree/master/packages/tsurlfilter ,
https://adguard.com/kb/general/ad-filtering/create-own-filters/ ,
https://adblockplus.org/acceptable-ads .
