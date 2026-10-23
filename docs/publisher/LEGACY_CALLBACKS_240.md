# proj240: legacy callbacks + Publisher POP

Основа патча — только `proj240.zip`. Для обычных callback эталон —
`proj212(20261016-140205).zip`; остальные исходники сохранены из proj240.

Изменения:

- `utils_grpc/urls.go`, `bidEngine/service/v_2_5.go`, `cmd/bid-engine/main.go`
  возвращены к точной реализации proj212. ADM несёт id/url/f, NURL — id/url/s/f
  (ADV без url), BURL — id/f с необязательным url. Коды POP/BAN/NAT/IPP/VID:
  0/1/2/3/4. Encoding — прежний net/url, порядок query-параметров несущественен.
- В `bidEngine/web/v_2_5.go` возвращены WriteUUIDKeyToRedis, исключены Collect/Save
  и удаление победителей при ошибке callback-контекста. Дополнительного ADV Redis
  клиента Bid Engine больше нет. Удалён глобальный пакет internal/callback,
  producer/consumer callback_context и переключатель UUID-only.
- `sspAdapter/web/route.go`, `common.go`, `billing/store.go` восстановлены для
  входных id/url/f/s и прежней UUID-проверки DSP/чтения финансового winner ADV.
  getNurl и публичный getBurl совпадают с proj212. Удалены глобальные Destination/NURL/BURL и DNS/IP
  проверки, HTTP client обычный с прежним timeout 3s. ReadWinner игнорирует
  оставшиеся старые поля контекста. Click ID, iframe, stats и финансовый Outbox
  сохранены. Сам UUID аукциона не меняется.
- `pub/server.go` напрямую сохраняет ADM от собственного BE в Delivery.Destination,
  без проверки/переписывания. Удалены deliveryADM/destination и мёртвая ветка
  unsupported_adm. PUB_CALLBACK_ORIGINS принимается для совместимости env-файлов,
  но больше не обязателен и не фильтрует ADM. Проверка входящего page_url сохранена.
- Новый `bidEngine/web/publisher_pop.go` после выбора победителя добавляет p=1
  только в ADM POP с pub_user_id, ad_code_id, исходным feed и существующим
  подписанным/claimed Publisher marker в ORTB. Feed token не сравнивается с
  отображаемым SSP domain: это разные значения. user_id рекламодателя не используется.
- Для Publisher POP с DSP-победителем сохраняется только pub_dsp_nurl в уже
  существующем UUID HASH ADM Redis (REDIS_UUID_ADDR / REDIS_DB_ADM). Запись условна
  на существующий uuid, не создаёт ключ и не меняет его TTL. Дополнительной записи
  для ADV/обычного RTB нет. ORTB используется только для прежней серверной проверки
  происхождения Publisher, никогда для хранения DSP NURL.
- По f=0,p=1 ADM выполняет один HGETALL UUID в ADM Redis: uuid подтверждает DSP,
  pub_dsp_nurl передаётся внутреннему BURL. Повторного EXISTS/HGET/HMGET нет. При
  отсутствии DSP UUID читается существующий финансовый ReadWinner ADV один раз.
  Для обычного RTB без p=1 остаётся прежний EXISTS без дополнительных чтений.
- Отсутствующий/несохранённый DSP NURL фиксируется существующим мониторингом:
  редирект, внутренний показ/BURL и биллинг продолжаются; внешнего HTTP по пустому
  адресу нет. Ошибка опциональной записи NURL не исключает выигравшую ставку.
  При полном отказе ADM Redis используются прежние ошибки и ADM Outbox.
- Четыре дублированных Redis-поля, WritePublisherBURL/ReadPublisherBURL и файл
  utils_grpc/publisher_burl.go удалены. В publisher_burl_input.go остались только
  существующие типы задачи. Старое JSON-поле publisher_id сохранено для совместимости
  durable jobs; у новых задач оно является внутренним признаком p=1, а не источником
  owner/billing identity. Ни новый Redis-контекст, ни новые структуры хранения не
  создаются. Очередь при восстановлении использует уже сохранённые Record/Winner.
- processBurl и publisher_burl_queue.go сохранены из предыдущего патча без изменений.
  Worker limits, retry, lease, stable billing ID, Outbox и DSP executor сохранены.
  Автоматический BURL запускается через существующую очередь после редиректа,
  без HTTP-запроса к собственному /burl. Публичные NURL/BURL используют старые DB
  и параметры proj212, без чтения удалённых четырёх полей.

Проверено локально: Go 1.25.11; Redis 7.0.15; gofmt; race-тесты восьми пакетов
utils_grpc, bidEngine/service, bidEngine/web, sspAdapter/web, sspAdapter/billing,
sspAdapter/outbox, pub, pubuuid. Дополнительные проверки: ошибка записи Publisher
DSP NURL не отбрасывает ставку; обычный ADV BURL работает при недоступном DSP Redis;
p=1 не подменяет отсутствующего победителя. Реальный сквозной тест
для DSP и ADV выполняет Publisher request → signed UUID claim → auction BE → ADM
с p=1 → сохранение Delivery → ADM HTTP route → очередь BURL. Исходный ORTB HASH
перед ADM удаляется. Повторные Publisher ADM сохраняют один внутренний BURL effect/ADV charge;
сами реальные click events сохраняют прежнюю семантику proj240.

Прошли сборки bid-engine, adm-adapter, pub, spp-adapter; git apply --check и реальное
применение патча на чистом proj240 с побайтным сравнением всех результирующих файлов.
Проверены точное число Redis-команд, абсолютное время истечения UUID до/после
записи NURL, отсутствующий NURL и его мониторинг, отказ записи NURL при сохранённом
UUID, повтор задачи после удаления NURL из Redis. Также все пять форматов, local/private
callback URLs, прежние обязательные
параметры, отсутствие p в NURL/BURL/не-POP, retry/restart/перегрузка очереди,
100/1000 concurrent ADM, iframe и неизменность финансовых Apply/DDL/Loader.

Не запускались: полный go test ./..., живой Kafka/gRPC межпроцессный стенд,
ClickHouse revenue integration, opt-in HTTP latency benchmark и live anti-AdBlock
lists/filter process. Четыре соответствующие opt-in проверки были явно skipped;
прочие проверки затронутых пакетов прошли. Production не использовался.

Применение из корня RTB:

```bash
cd ~/gpt_test_3
git apply --check ~/Downloads/proj240_legacy_callbacks_publisher_FINAL_v2.patch
git apply ~/Downloads/proj240_legacy_callbacks_publisher_FINAL_v2.patch
mkdir -p bin
go build -o bin/bid-engine ./cmd/bid-engine
go build -o bin/adm-adapter ./cmd/adm-adapter
go build -o bin/pub ./cmd/pub
go build -o bin/spp-adapter ./cmd/spp-adapter
```

Никакая миграция ClickHouse/PostgreSQL не нужна. Старый финансовый DDL не запускать.
Loaders, auction/price/percenters, GEO/domain changer, Router/ADV/Orchestrator,
Publisher signed SSP admission/ORTB/диагностика, финансовые формулы не изменены.

Ранее выданная id-only ссылка не содержит url/f/s и новым старым контрактом не
обслуживается. Для перехода остановить выдачу и выждать фактический срок старых
callback/late events, затем согласованно заменить producer/adapter/pub. Redis и
финансовый/Publisher Outbox не очищать. Старые поля контекста можно оставить до
естественного TTL: новый код их не читает. При отсутствии DSP NURL внутренняя обработка не пропускается. При полном отказе
ADM Redis победитель не выдумывается: редирект и прежний ADM Outbox сохраняются.
Существующие ограничения RAM fast path/переполнения очереди остаются прежними.
