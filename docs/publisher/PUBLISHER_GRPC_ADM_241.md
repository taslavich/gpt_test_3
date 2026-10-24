# Publisher POP: gRPC-флаг и ADM n (proj241)

Патч применяется прямо к исходному proj241.zip. Старые патчи не нужны.

## Изменения

- SSP Adapter использует результат существующего `trustedPublisherUUID()`:
  `publisherID != ""`. Отказ проверки подписи, signed UUID и его регистрация
  не меняются. Дополнительного Redis-запроса нет.
- `is_publisher` передаётся SSP → Orchestrator → Router → Orchestrator → BidEngine.
  Номера полей: 10 у OrchestratorRequest, RouterRequest/Response; 13 у
  BidEngineRequest. Reserved и остальные номера сохранены. Go protobuf получен
  генерацией (protoc 3.21.12, protoc-gen-go 1.36.11, protoc-gen-go-grpc 1.6.2)
  по параметрам `proto/protoc.txt`; сгенерированные файлы включены в патч.
- Только при `is_publisher && format == POP` BidEngine дописывает ADM `p=1`.
  У DSP исходный NURL берётся из уже сформированного BURL (`Query().Get("url")`)
  и кодируется через `net/url` как ADM `n`. ADV и DSP без NURL получают только p.
  ADM destination, BURL/NURL, остальные форматы и обычные SSP не меняются.
- `pub_dsp_nurl` больше не записывается и не читается в Redis. Повторное
  определение Publisher через ORTB/marker в BidEngine удалено.
- ADM использует прежний `UUIDKeyExistsInRedis()`; если DSP UUID отсутствует,
  остаётся `ReadWinner()` ADV. Необязательный `n` декодируется HTTP parser один
  раз и сразу попадает в существующий `Record.BURL`. Пустой n не мешает клику,
  редиректу и внутреннему BURL; внешний DSP HTTP при этом пропускается.
- processBurl, очередь, DSP executor, Outbox, финансовые guards и billing
  не изменены. Существующий JSON очереди сохраняет BURL при retry/restart.
  Publisher `pub` production-код, loaders, signed UUID/page_url, финансовые
  расчёты, routing, GEO/domain changer и percenters не изменены. В `pub`
  скорректирован только старый интеграционный тест под новый контракт.

## Применение и сборка

Из корня RTB с исходниками proj241, Go 1.25.11:

```bash
cd ~/gpt_test_3
git apply --check ~/Downloads/proj241_publisher_grpc_flag_adm_nurl_FINAL.patch
git apply ~/Downloads/proj241_publisher_grpc_flag_adm_nurl_FINAL.patch
mkdir -p bin
for service in spp-adapter orchestrator router bid-engine adm-adapter; do
  go build -o "bin/$service" "./cmd/$service" || exit 1
done
```

Миграций БД и новых ENV нет. Для перехода остановить/дренировать выдачу
Publisher POP и дождаться истечения уже выданных callback по фактическому TTL.
Согласованно обновить все пять сервисов цепочки, затем возобновить выдачу.
Старый gRPC hop теряет новое boolean-поле; старый ADM Adapter не использует n.
Новый ADM больше не читает NURL из Redis у уже выданных старых ссылок: их
внутренний BURL сохраняется, но внешнее DSP-уведомление без n не восстановится.
Redis/Outbox не очищать. Ранее сохранённые задачи содержат BURL и восстанавливаются
по прежним правилам. Старое Redis-поле можно оставить до естественного TTL.

## Реально выполненные проверки

- Генерация трёх protobuf-контрактов, gofmt и `git diff --check`.
- `go test -race -count=1` десяти пакетов с локальным Redis 7.0.15:

```bash
# Только отдельный тестовый Redis; DB 7–13 используются тестами.
export PUB_TEST_REDIS_ADDR=127.0.0.1:16390
go test -p 2 -race -count=1 \
  ./internal/services/orchestrator/web \
  ./internal/services/dspRouter/web \
  ./internal/grpc/utils_grpc \
  ./internal/services/bidEngine/service \
  ./internal/services/bidEngine/web \
  ./internal/services/sspAdapter/web \
  ./internal/services/sspAdapter/billing \
  ./internal/services/sspAdapter/outbox \
  ./internal/services/pub ./internal/pubuuid
```

Все десять пакетов прошли. Проверены: настоящая HTTP SSP admission и три
сервиса через gRPC bufconn/protobuf, обычный feed, неверная подпись (403),
все успешные Router-ветки, protobuf round-trip, выбор возвращённого Router-флага,
DSP/ADV/пустой NURL, пять форматов, вложенный URL с `? & % = #`, отсутствие
Redis-атрибуции в Publisher helper, прежние UUID lookup и legacy HTTP callbacks,
асинхронный BURL, повторы без двойного списания/показа, ошибки внешнего DSP,
перезапуск durable очереди с сохранённым URL из настоящего HTTP query parser.

Собраны spp-adapter, orchestrator, router, bid-engine, adm-adapter.
Патч проверен `git apply --check`, затем реально применён на новой распаковке
proj241 с побайтным сравнением полученных файлов.

Четыре opt-in теста пропущены: ClickHouse revenue integration, ADM latency
benchmark, реальный filter process и live official lists. Полный `go test ./...`,
живой Kafka/ClickHouse и production-стенд не запускались. Сетевая gRPC-цепочка
проверялась через настоящий gRPC transport в памяти, без отдельных процессов.
