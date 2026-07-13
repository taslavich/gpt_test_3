# ADV runtime deployment notes

The ADV service, `kafkaredis`, and ADM Adapter must use the same Redis server:
DB 5 for cumulative expenses/pacing/outbox markers and DB 6 for winner hashes.

`docker-compose.redis-aof.yml` starts Redis with the checked-in
`redis-aof.conf` and a persistent named volume. Before starting the Go
services, set their `REDIS_UUID_ADDR` and `REDIS_PASSWORD` to this Redis
endpoint, provide the real `POSTGRES_DSN`, and replace every entry in
`ADV_SERVICE_CONTROL_URLS` with the actual HTTP control URLs of all ADV
instances. Production may use an external
Redis, but that instance must have the same AOF settings and persistent `/data`
volume; all three executables fail fast when `INFO persistence` or Redis config
does not confirm `appendonly yes` and `appendfsync everysec`.

The checked-in quality map is initialized from every SSP feed domain currently
present in `cmd/spp-adapter/spp-adapter.env`. Operators can atomically replace
segments through `PUT /filter/quality_map`.
