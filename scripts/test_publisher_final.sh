#!/usr/bin/env bash
# Disposable integration only. Actual RTB DDL -> existing cabinet statistics API.
set -euo pipefail
cabinet_root=${1:?Usage: scripts/test_publisher_final.sh /path/to/frontBack/twinbid-backend}
: "${PUBLISHER_TEST_CLICKHOUSE_ADDR:?set disposable native ClickHouse host:port}"
: "${PUB_TEST_REDIS_ADDR:?set disposable Redis host:port (DB 0..15)}"
rtb_root=$(cd "$(dirname "$0")/.." && pwd)
client=${CLICKHOUSE_CLIENT:-clickhouse-client}
test_db="publisher_final_$(python3 -c 'import uuid;print(uuid.uuid4().hex)')"
ddl=$(mktemp)
cleanup() {
 "$client" --host "${PUBLISHER_TEST_CLICKHOUSE_ADDR%:*}" --port "${PUBLISHER_TEST_CLICKHOUSE_ADDR##*:}" --query "DROP DATABASE IF EXISTS $test_db SYNC" || true
 rm -f "$ddl"
}
trap cleanup EXIT
python3 - "$rtb_root/internal/services/clickhouse-loader/createDb.go" "$test_db" >"$ddl" <<'PY'
import pathlib,sys,datetime
text=pathlib.Path(sys.argv[1]).read_text()
sql=text.split('const ddlTemplate = `',1)[1].split('\n`',1)[0]
cutover=(datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(hours=2)).replace(minute=0,second=0,microsecond=0).strftime('%Y-%m-%d %H:%M:%S')
print(sql.replace('{db}',sys.argv[2]).replace('{ipp_cutover_from}',cutover))
PY
"$client" --host "${PUBLISHER_TEST_CLICKHOUSE_ADDR%:*}" --port "${PUBLISHER_TEST_CLICKHOUSE_ADDR##*:}" --multiquery <"$ddl"
(cd "$rtb_root"; go test -p 1 -count=1 ./internal/callback ./internal/services/bidEngine/... ./internal/services/sspAdapter/... ./internal/services/pub/... ./internal/services/clickhouse-loader/...)
(cd "$cabinet_root"; PUBLISHER_FORMAT_TEST_DATABASE="$test_db" go test -count=1 ./internal/stats -run '^TestPublisherFormatGroupingRealSchema$' -v)
