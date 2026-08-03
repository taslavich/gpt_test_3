# Диагностика аукциона ADV: production-версия

## Назначение

Механизм показывает, на каком фактическом этапе конкретная кампания перестала участвовать в аукционе и почему ADV не всегда возвращает bid. Статистика хранится **локально в каждой реплике ADV** и не агрегируется между репликами.

Когда диагностика включена, учитывается **100% поступившего на эту реплику трафика**. Sampling отсутствует.

## Основная единица статистики

Одна единица кампанийной статистики:

```text
одна валидная кампания × один валидный impression
```

Для каждой такой пары записывается ровно один терминальный код. Поэтому у каждой кампании:

```text
total = сумма count всех её кодов
percent = count / total × 100
```

Проценты округляются до шести знаков после запятой, поэтому их сумма может отличаться от 100 только на величину округления.

Кампания не получает результат по impression, когда до её перебора невозможно дойти: например, impression невалиден, отсутствует winner UUID, нет snapshot или не загружен AntiPerekrut state. Такие случаи находятся в `global`.

## Код 200

`200 bid_won` — внутренний диагностический код, а не HTTP status. Он выставляется только после трёх успешных этапов:

1. Кампания выбрана победителем.
2. Bid успешно собран.
3. Winner record успешно записан в winner Redis.

Если Redis-запись winner не удалась, кампания получает `513 winner_redis_write_failed`, а ADV может перейти к следующему кандидату.

## Порядок определения причины кампании

Для кампании фиксируется первое условие, остановившее её в реальном порядке выполнения:

1. Формат кампании.
2. Тип трафика.
3. Статус, даты и active intervals.
4. AntiPerekrut campaign eligibility.
5. AntiPerekrut traffic-percent hash gate.
6. Quality map конкретной кампании.
7. Расчёт charge price.
8. Чтение расхода кампании из runtime Redis.
9. Остаток бюджета кампании.
10. Pacing текущего слота.
11. Подбор креативов.
12. Фильтры: country → language → device type → OS → browser → site ID → IP.
13. Effective price после deduction.
14. Bid floor.
15. Candidate pool, выбор победителя, сборка bid и запись winner.

Ранний общий выход:

```go
quality.ContainsAny(sspDomain)
```

удалён. Остаётся только проверка quality конкретной кампании:

```go
quality.Contains(campaign.QualitySegment, sspDomain)
```

Поэтому неизвестный SSP-домен отображается кодом `309 quality_mismatch` у каждой кампании, для которой quality стала первым препятствием.

## Правило последнего креатива

Если подошёл хотя бы один креатив, кампания продолжает участие и не получает creative-код.

Если не подошёл ни один креатив, кампании записывается причина **последнего проверенного креатива**. Причины предыдущих креативов в итоговый счётчик кампании не попадают.

Пример:

```text
creative-1 → 406 banner_size_mismatch
creative-2 → 407 banner_mime_mismatch
creative-3 → 402 creative_adm_url_empty

итог кампании → 402 creative_adm_url_empty
```

## AntiPerekrut

AntiPerekrut учтён отдельными причинами, а не одним общим кодом:

```text
325 state missing
307 durable user block
308 pending local user block
324 balance guard
326 unclassified blocked state
327 traffic-percent hash gate
```

Если AntiPerekrut manager или весь state недоступен до перебора кампаний, используются глобальные коды `913` и `914`.

Обычный быстрый путь при выключенной диагностике продолжает использовать прежний `CampaignAllowed(...) bool`. Подробная классификация вызывается только в диагностическом пути.

## Включение и отключение

Начальное состояние задаётся переменной окружения:

```env
AUCTION_DIAGNOSTICS_ENABLED=false
```

Значение по умолчанию — `false`.

Статус:

```http
GET /internal/auction-diagnostics/status
```

Включение 100% учёта:

```http
PUT /internal/auction-diagnostics/status?enabled=true
```

Отключение:

```http
PUT /internal/auction-diagnostics/status?enabled=false
```

Примеры:

```bash
curl -s -X PUT   'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics/status?enabled=true' | jq .

curl -s -X PUT   'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics/status?enabled=false' | jq .
```

Ручка действует только на реплику, которая обработала HTTP-запрос. При наличии балансировщика нужно обращаться к конкретному instance напрямую.

Эти internal endpoints не добавляют собственную аутентификацию. Их следует оставлять только во внутренней сети или закрывать правилами proxy/firewall.

## Получение статистики

```http
GET /internal/auction-diagnostics
```

```bash
curl -s 'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics' | jq .
```

После включения и до публикации первого окна:

```json
{
  "enabled": true,
  "coverage_percent": 100,
  "ready": false,
  "partial": true
}
```

Если диагностика включена не на минутной границе, первое окно неполное и получает `partial: true`. Последующие минутные окна имеют `partial: false`. При отключении текущий интервал немедленно публикуется как `partial: true`, а `enabled` становится `false`.

`coverage_percent: 100` означает отсутствие sampling во время включённой диагностики. Это не означает, что диагностика работала всю минуту — для этого нужно смотреть `partial`, `window_start` и `window_end`.

## Формат JSON

```json
{
  "replica": "adv-1",
  "enabled": true,
  "coverage_percent": 100,
  "ready": true,
  "partial": false,
  "window_start": "2026-08-03T16:35:00Z",
  "window_end": "2026-08-03T16:36:00Z",
  "generated_at": "2026-08-03T16:36:00.010Z",
  "global": {
    "requests": {
      "total": 100000,
      "codes": {}
    },
    "impressions": {
      "total": 100000,
      "codes": {
        "933": {
          "name": "winner_uuid_missing",
          "count": 20,
          "percent": 0.02
        }
      }
    }
  },
  "campaigns": {
    "campaign-a": {
      "user_id": "user-a",
      "total": 100000,
      "codes": {
        "200": {"name": "bid_won", "count": 5000, "percent": 5},
        "309": {"name": "quality_mismatch", "count": 60000, "percent": 60},
        "335": {"name": "site_id_filter_rejected", "count": 35000, "percent": 35}
      }
    }
  },
  "codebook": {}
}
```

Поле `codebook` содержит машинно-читаемую расшифровку всех кодов: `name`, `description`, `scope`.

## Global

`global.requests.total` — количество запросов, вошедших в диагностику на этой реплике. Сюда также относится `899 service_disabled`, который фиксируется в gRPC server до входа в `Auction()`.

`global.impressions.total` — количество элементов `imp` во всех учтённых запросах.

`global.requests.codes` содержит причины уровня всего запроса или инфраструктуры, когда нормальный перебор кампаний не начался либо аукцион завершился общей ошибкой.

`global.impressions.codes` содержит проблемы конкретного impression или повреждённого snapshot применительно к impression.

Проценты `global` не обязаны складываться в 100%:

- успешные запросы и impressions не получают отдельного global success-кода;
- на одном impression одновременно могут быть отмечены признаки повреждённого snapshot, например `935` и `936`.

Quality map не входит в global. Это всегда кампанийный код `309`.

## Двойной буфер и конкурентность

Логически существуют две структуры:

- `current` — sharded atomic counters текущего интервала;
- `published` — неизменяемый JSON snapshot последнего закрытого интервала.

На минутной границе:

1. Создаётся новый пустой `current`.
2. Указатель `current` атомарно переключается.
3. ADV дожидается завершения writers, уже вошедших в старый буфер.
4. Счётчики старого буфера агрегируются.
5. Указатель `published` атомарно заменяется.

GET читает только `published`, поэтому JSON-сериализация не блокирует аукционный hot path.

## Производительность

### Единая логика аукциона

В сервисе существует только один `auctionCore`. Диагностика передаётся в него как необязательный пассивный recorder.

Recorder может только увеличить диагностический счётчик после уже принятого решения. Он не используется в:

- campaign eligibility;
- AntiPerekrut и hash gate;
- quality и targeting filters;
- расчёте charge/effective price;
- candidate pool;
- random draw;
- выборе креатива;
- построении bid;
- записи winner.

Поэтому включение диагностики не переключает ADV на другую реализацию аукциона и не может изменить победителя или ответ.

### Диагностика выключена

На входе в `Auction()` выполняется atomic load активной diagnostic session. В едином `auctionCore` recorder равен `nil`, поэтому:

- не выбирается shard;
- не увеличиваются счётчики;
- не создаются диагностические states;
- не выполняются диагностические map/array lookups;
- нет диагностических allocations и дополнительной нагрузки на GC.

В местах терминального результата остаются дешёвые проверки `recorder != nil`. Это цена единой бизнес-логики без двух расходящихся копий аукциона.

Общий ранний `quality.ContainsAny(sspDomain)` удалён по согласованному требованию. Остаётся только персональная `quality.Contains(...)` каждой кампании.

### Диагностика включена

Учитывается 100% трафика. На один request выполняется выбор одного из 32 shard. На каждую пару `campaign × imp` выполняется один terminal counter increment.

Оптимизации:

- в hot path отсутствует `sync.Map` и поиск по строковому UUID;
- каждая кампания получает плотный `diagnosticIndex` при публикации snapshot;
- отдельного `total.Add(1)` нет — `total` вычисляется суммой терминальных кодов при закрытии окна;
- 32 shard уменьшают cache-line contention между CPU;
- `campaignOutcomes` и `attemptResults` размером со весь snapshot отсутствуют;
- отклонённая кампания записывается сразу;
- временные slices создаются только для реально eligible candidates;
- recorder создаётся значением, а не отдельной heap-структурой;
- pointer на блоки счётчиков кэшируется в recorder; повторная atomic load нужна только при редкой публикации snapshot с ростом capacity;
- подробная pacing-причина не вызывает повторных Redis-read только ради диагностического лога.

Стабильные индексы сохраняются, пока diagnostics включена, чтобы in-flight auctions со старым snapshot не смешивались с новыми кампаниями. После отключения registry уплотняется до текущего snapshot, поэтому удалённые campaign IDs не удерживают counter capacity до перезапуска процесса.

Текущий layout использует 32 shard и 77 campaign-reason slots, то есть приблизительно:

```text
32 × 77 × 8 = 19 712 байт счётчиков на один campaign index в active buffer
```

Counter blocks выделяются по 64 campaign indexes. Опубликованный snapshot хранится отдельно как обычная неизменяемая JSON-структура.

Полный 100% учёт физически не является бесплатным. Для оценки реального влияния нужно запускать `diagnostics=true` сначала на одной реплике и сравнивать CPU, allocations, GC, p95/p99 и RPS с той же репликой при `diagnostics=false`.

## Коды кампаний: основные проверки и фильтры

| Код | Имя | Точное значение |
|---:|---|---|
| 200 | `bid_won` | Кампания победила, bid собран, winner записан в Redis. |
| 300 | `campaign_format_mismatch` | Формат кампании не совпал с форматом запроса. |
| 301 | `traffic_type_mismatch` | Тип трафика кампании не совпал с запросом. |
| 302 | `campaign_status_not_active` | Статус кампании не `active`. |
| 303 | `campaign_time_window_invalid` | Не заданы/невалидны `StartTS` и `EndTS`. |
| 304 | `campaign_not_started` | Текущее время раньше старта. |
| 305 | `campaign_ended` | Кампания уже завершилась. |
| 306 | `outside_active_intervals` | Время не попало ни в один активный интервал. |
| 307 | `antiperekrut_durable_user_block` | В snapshot кампаний у пользователя установлен постоянный флаг `antiperekrut_blocked` и AntiPerekrut запретил кампанию. |
| 308 | `antiperekrut_pending_user_block` | Пользователь заблокирован локально через `PendingUserBlocks`, пока постоянный флаг ещё не подтверждён новым snapshot. |
| 309 | `quality_mismatch` | SSP-домен отсутствует в quality-сегменте кампании. |
| 310 | `invalid_charge_price` | Charge price неположительный, NaN или Inf. |
| 311 | `campaign_spent_read_failed` | Не удалось прочитать расход кампании из runtime Redis. |
| 312 | `campaign_balance_insufficient` | Остатка бюджета кампании меньше, чем charge price. |
| 313 | `pacing_check_failed` | Неклассифицированная ошибка pacing. Защитный fallback. |
| 314 | `pacing_current_slot_missing` | Нет ключа текущего pacing-слота. |
| 315 | `pacing_target_non_positive` | Цель текущего pacing-слота не положительна. |
| 316 | `pacing_slot_limit_reached` | Расход текущего слота достиг его цели. |
| 317 | `no_creatives_configured` | У кампании отсутствуют креативы. |
| 318 | `effective_price_non_positive` | Effective price после вычета комиссии неположительный, NaN или Inf. |
| 319 | `effective_price_below_bidfloor` | Effective price ниже `imp.bidfloor`. |
| 320 | `pacing_current_slot_read_failed` | Не удалось прочитать ключ текущего pacing-слота из Redis. |
| 321 | `pacing_current_slot_key_invalid` | Текущий pacing-ключ указывает на ключ неправильного формата. |
| 322 | `pacing_slot_spent_read_failed` | Не удалось прочитать или распарсить расход текущего слота. |
| 323 | `pacing_slot_target_failed` | Ошибка расчёта цели текущего pacing-слота. |
| 324 | `antiperekrut_balance_guard_rejected` | Сработал предаукционный балансный guard: `user_spend_last_minute × 2 > user_remaining_balance`. |
| 325 | `antiperekrut_campaign_state_missing` | В опубликованном AntiPerekrut state отсутствует `CampaignAuctionAllowed[campaign_id]`; применяется fail-closed. |
| 326 | `antiperekrut_campaign_blocked_unknown` | `CampaignAuctionAllowed[campaign_id] == false`, но state не позволяет отнести запрет к durable, pending или balance guard. |
| 327 | `antiperekrut_hash_gate_rejected` | Конкретный request не прошёл hash gate текущего traffic percent кампании. |
| 330 | `country_filter_rejected` | Не пройден country filter. |
| 331 | `language_filter_rejected` | Не пройден language filter. |
| 332 | `device_type_filter_rejected` | Не пройден device type filter. |
| 333 | `os_filter_rejected` | Не пройден OS filter. |
| 334 | `browser_filter_rejected` | Не пройден browser filter. |
| 335 | `site_id_filter_rejected` | Не пройден `site_id` filter. |
| 336 | `ip_filter_rejected` | Не пройден IP filter. |

## Коды кампаний: креативы

| Код | Имя | Точное значение |
|---:|---|---|
| 400 | `creative_nil` | Последний проверенный креатив равен `nil`. |
| 401 | `creative_id_empty` | У последнего креатива пустой ID. |
| 402 | `creative_adm_url_empty` | У последнего креатива пустой ADM URL. |
| 403 | `banner_object_missing` | Для BAN impression отсутствует объект banner. |
| 404 | `banner_sizes_missing` | В BAN-запросе нет валидного размера. |
| 405 | `banner_creative_dimensions_invalid` | У последнего banner-креатива невалидные размеры. |
| 406 | `banner_size_mismatch` | Размер последнего banner-креатива не запрошен. |
| 407 | `banner_mime_mismatch` | MIME последнего banner-креатива не принят запросом. |
| 410 | `native_object_missing` | Для NAT/IPP отсутствует объект native. |
| 411 | `native_request_empty` | Пустой payload native request. |
| 412 | `native_request_json_invalid` | Невалидный внешний JSON native request. |
| 413 | `native_payload_json_invalid` | Невалидный внутренний native payload. |
| 414 | `native_assets_empty` | В native request нет assets. |
| 415 | `native_asset_id_invalid` | Некорректный ID native asset. |
| 416 | `native_campaign_format_invalid` | При native-проверке формат кампании не NAT/IPP. |
| 417 | `native_required_title_missing` | Нет обязательного title. |
| 418 | `native_required_brand_name_missing` | Нет обязательного brand name. |
| 419 | `native_required_description_missing` | Нет обязательного description. |
| 420 | `native_required_data_type_unsupported` | Обязательный тип data asset не поддерживается. |
| 421 | `native_required_image_url_missing` | Нет обязательного image URL. |
| 422 | `native_required_image_dimensions_invalid` | Невалидные размеры native image. |
| 423 | `native_required_image_width_below_wmin` | Ширина изображения меньше `wmin`. |
| 424 | `native_required_image_height_below_hmin` | Высота изображения меньше `hmin`. |
| 425 | `native_required_image_format_unsupported` | Формат native image не поддерживается. |
| 426 | `native_required_image_not_eligible` | Неклассифицированная причина непригодности обязательного изображения. |
| 427 | `native_required_asset_type_unsupported` | Обязательный тип native asset не поддерживается. |
| 428 | `native_adm_build_failed_unknown` | Неклассифицированная ошибка сборки native ADM. |
| 429 | `creative_not_matched_unknown` | Неклассифицированная форматная причина несовпадения креатива. |

## Коды кампаний: candidate pool и победитель

| Код | Имя | Точное значение |
|---:|---|---|
| 500 | `below_weighted_top_threshold` | Кампания не попала в weighted-top pool из-за порога цены. |
| 501 | `lower_effective_price_than_winner` | В max-bid effective price ниже цены победителя. |
| 502 | `equal_top_price_not_selected_after_shuffle` | Цена равна максимальной, но кампания не стала первой после shuffle ничьей. |
| 503 | `not_selected_by_weighted_draw` | Кампания была в pool, но weighted draw выбрал другую раньше. |
| 504 | `winner_selected_before_attempt` | Другая кампания победила до попытки этой кампании. |
| 505 | `no_winner_selected` | Кампания осталась неиспытанной, а победитель не был сформирован. |
| 510 | `eligible_candidate_has_no_creatives` | Защитная проверка: у eligible candidate неожиданно нет креативов. |
| 511 | `random_creative_nil` | Случайно выбранный креатив оказался `nil`. |
| 512 | `bid_build_failed` | Не удалось собрать bid из выбранного креатива. |
| 513 | `winner_redis_write_failed` | Не удалось записать winner record в Redis. |
| 514 | `weighted_index_unavailable` | Weighted selection не смог вернуть индекс кандидата. |
| 515 | `excluded_unknown_auction_mode` | Кампания исключена из-за неизвестного auction mode. |

## Коды `global.requests`

| Код | Имя | Точное значение |
|---:|---|---|
| 899 | `service_disabled` | `work_status` отключил ADV до входа в Auction. |
| 900 | `bid_request_nil` | `BidRequest == nil`. |
| 901 | `request_id_empty` | Пустой OpenRTB request ID. |
| 902 | `impressions_empty` | В запросе нет impressions. |
| 903 | `imp_uuid_map_empty` | Пустая map `imp_id → winner UUID`. |
| 904 | `no_valid_impressions` | Нет ни одного ненулевого impression с пригодным ID. |
| 905 | `invalid_format` | Пустой или неподдерживаемый формат запроса. |
| 906 | `invalid_traffic_type` | Пустой или неподдерживаемый traffic type. |
| 907 | `ssp_domain_empty` | После нормализации SSP domain пуст. |
| 908 | `runtime_store_unavailable` | Runtime store ADV не инициализирован. |
| 909 | `winner_store_unavailable` | Winner store ADV не инициализирован. |
| 910 | `percent_store_unavailable` | Percent store ADV не инициализирован. |
| 911 | `quality_store_unavailable` | Quality store ADV не инициализирован. |
| 912 | `campaign_snapshot_unavailable` | Snapshot кампаний отсутствует. |
| 913 | `antiperekrut_manager_unavailable` | AntiPerekrut включён, но manager отсутствует. |
| 914 | `antiperekrut_state_unavailable` | AntiPerekrut state отсутствует или ещё не загружен. |
| 915 | `auction_infrastructure_error` | После проверок кампаний аукцион завершился общей инфраструктурной ошибкой и не вернул bid. |

## Коды `global.impressions`

| Код | Имя | Точное значение |
|---:|---|---|
| 930 | `impression_nil` | Элемент impression равен `nil`. |
| 931 | `impression_id_empty` | Пустой impression ID. |
| 932 | `impression_id_duplicate` | Impression ID повторяется в запросе. |
| 933 | `winner_uuid_missing` | Для impression отсутствует winner UUID. |
| 934 | `winner_uuid_duplicate` | Один winner UUID используется несколькими impressions. |
| 935 | `campaign_entry_nil` | Snapshot содержит хотя бы одну `nil`-кампанию; считается не более одного раза на impression. |
| 936 | `campaign_id_empty` | Snapshot содержит кампанию с пустым ID; считается не более одного раза на impression. |
| 937 | `campaign_snapshot_empty` | В snapshot нет ни одной кампании для проверки impression. |

## Проверка после установки

Сборка и тесты должны выполняться toolchain проекта — Go 1.25.11:

```bash
gofmt -w   cmd/adv/main.go   internal/services/adv/service/diagnostics.go   internal/services/adv/service/diagnostics_test.go   internal/services/adv/service/service.go   internal/services/adv/service/antiperekrut.go   internal/services/adv/service/runtime_store.go   internal/services/adv/web/httpRoute.go   internal/services/adv/web/server.go   internal/services/adv/web/auction_diagnostics_test.go

go test -race ./internal/services/adv/service ./internal/services/adv/web
go test ./...
go build ./cmd/adv
```

Проверка выключенного состояния:

```bash
curl -s 'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics/status' | jq .
```

Включение на одной реплике:

```bash
curl -s -X PUT   'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics/status?enabled=true' | jq .
```

Проверка согласованности totals:

```bash
curl -s 'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics'   | jq '.campaigns | to_entries[] | {campaign_id: .key, total: .value.total, sum: ([.value.codes[].count] | add)}'
```

Для каждой кампании `total` должен совпадать с `sum`.

Отключение после диагностики:

```bash
curl -s -X PUT   'http://127.0.0.1:<HTTP_PORT>/internal/auction-diagnostics/status?enabled=false' | jq .
```

## Рекомендуемый rollout

1. Применить patch и выполнить полный `go test -race`, `go test ./...`, `go build` на Go 1.25.11.
2. Запустить ADV с `AUCTION_DIAGNOSTICS_ENABLED=false`.
3. Убедиться, что latency/RPS соответствует исходной версии.
4. Включить diagnostics только на одной реплике через прямой internal HTTP address.
5. Собирать статистику несколько полных минут.
6. Одновременно сравнить CPU, GC, allocations, p95/p99 и error rate.
7. Отключить diagnostics через PUT после завершения расследования.
