# Применение production-патча диагностики ADV

## 1. Проверка рабочей копии

Патч рассчитан на чистый проект `proj90(5).zip`, до применения предыдущего диагностического patch.

Из корня репозитория:

```bash
git status --short
git apply --check /path/to/adv-auction-diagnostics-production.patch
```

`git apply --check` не должен вывести ошибок.

## 2. Применение

```bash
git apply /path/to/adv-auction-diagnostics-production.patch
```

Проверить изменённые файлы:

```bash
git status --short
git diff --check
git diff --stat
```

## 3. Форматирование, тесты и сборка

Проект требует Go 1.25.11 согласно `go.mod`.

```bash
gofmt -w \
  cmd/adv/main.go \
  internal/services/adv/service/diagnostics.go \
  internal/services/adv/service/diagnostics_test.go \
  internal/services/adv/service/service.go \
  internal/services/adv/service/antiperekrut.go \
  internal/services/adv/service/runtime_store.go \
  internal/services/adv/web/httpRoute.go \
  internal/services/adv/web/server.go \
  internal/services/adv/web/auction_diagnostics_test.go

go test -race ./internal/services/adv/service ./internal/services/adv/web
go test ./...
go build ./cmd/adv
```

Не выкатывать patch, если хотя бы одна из этих команд завершилась ошибкой.

## 4. Начальное состояние production

Рекомендуемая переменная окружения:

```env
AUCTION_DIAGNOSTICS_ENABLED=false
```

При таком состоянии на входе в `Auction()` выполняется atomic load активной diagnostic session. Далее работает тот же единый `auctionCore`, что и при включённой диагностике, но recorder равен `nil`: shard, счётчики и диагностические allocations отсутствуют.

## 5. Запуск

Перезапустить только ADV-процесс штатным способом вашего deployment. После запуска проверить:

```bash
curl -s 'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics/status' | jq .
```

Ожидаемо:

```json
{
  "enabled": false,
  "coverage_percent": 100
}
```

## 6. Включение на одной реплике

Обращаться непосредственно к нужному ADV-instance, а не к общему балансировщику:

```bash
curl -s -X PUT \
  'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics/status?enabled=true' | jq .
```

При включении учитывается 100% запросов, impressions и проверенных кампаний этой реплики. Sampling отсутствует.

Получение данных:

```bash
curl -s 'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics' | jq .
```

Первое окно может быть `partial: true`. Для сопоставимой минутной статистики дождаться `ready: true` и `partial: false`.

## 7. Контроль нагрузки

Во время включённой диагностики сравнивать с состоянием `enabled=false`:

- CPU процесса ADV;
- RSS и скорость allocations;
- частоту и длительность GC;
- p50/p95/p99 аукциона;
- RPS и error rate;
- Redis latency и errors.

Начинать только с одной реплики.

## 8. Отключение

```bash
curl -s -X PUT \
  'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics/status?enabled=false' | jq .
```

Новые аукционы сразу продолжают работу в том же `auctionCore`, но без recorder. Уже начатые аукционы завершают запись, после чего текущий неполный интервал публикуется с `partial: true`.

## 9. Откат patch

До коммита:

```bash
git apply -R /path/to/adv-auction-diagnostics-production.patch
```

После отдельного коммита patch откатывается обычным `git revert <commit>`.

## Важное изменение quality

Патч намеренно удаляет общий ранний выход:

```go
quality.ContainsAny(sspDomain)
```

и оставляет только персональную проверку quality каждой кампании. Поэтому для SSP, отсутствующего во всех quality maps, ADV перебирает кампании до проверки `quality.Contains(...)` даже при выключенной диагностике. Это согласованное функциональное изменение, не связанное с включением или выключением recorder.
