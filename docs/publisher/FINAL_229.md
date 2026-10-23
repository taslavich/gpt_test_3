> Для точечного отката callback в proj240 используйте [LEGACY_CALLBACKS_240.md](LEGACY_CALLBACKS_240.md). Финансовые инструкции ниже относятся к исторической миграции 229 и не нужны для этого отката.

# TwinBid 229 v4 — replacement, deployment and checks

Базы: только `proj229(6).zip` и `FProj229(3).zip`. Патчи v4 полные: предыдущие патчи
перед ними не применять. AGENTS.md в исходных архивах/родительских каталогах нет.
Frontend не предоставлен: скрытие четырёх UI-показателей и UI-группировка Format
не реализованы и не объявляются завершёнными. Requests/fills/no_fills/errors в API,
ClickHouse и сборе данных сохранены. SDK, /confirm и frequency capping не меняются.

## Применение исходников

```bash
cd ~/RTB
git apply --check -p1 ~/Downloads/proj229_publisher_final_v3.patch
git apply -p1 ~/Downloads/proj229_publisher_final_v3.patch

cd ~/frontBack
git apply --check -p1 ~/Downloads/FProj229_publisher_final_v3.patch
git apply -p1 ~/Downloads/FProj229_publisher_final_v3.patch
```

В frontBack сохраняется вложенный `twinbid-backend/`. На checkout с v1/v2 сначала
подготовить чистый checkout исходного ZIP, затем применить v3. Не делать reverse
v1 на рабочем сервере и не удалять БД. Смена исходников и миграция данных — разные
действия. Архивные базы проверяются отдельно через git apply --check.

## POP-доход: изменившиеся сегменты вместо всей истории

Источник — реальные Publisher POP /adm → существующий click pipeline → fact_clicks.
Сумма — win_final_price / 1000 один раз на clicks_uuid; SSP owner берётся только из
серверного ORTB-контекста. BURL/POP recovery продолжают advertiser accounting, но
не создают POP SSP revenue. Поле назначения прежнее: agg_stats.ssp_revenue_views.

* Для новых Publisher POP /adm click ID создаётся как UUIDv7. Остальные клики
  сохраняют прежний UUIDv4. UUID остаётся случайным, уникальным, серверным ID.
* Сегмент определяется **самим UUID**, а не event_hour входной строки: час timestamp
  UUIDv7 + постоянный offset 1000000. Старые UUIDv4 попадают в 256 фиксированных
  hash-сегментов 1..256. Повтор того же ID с другой датой остаётся в одном сегменте.
* publisher_pop_revenue_events_v2 — AggregatingMergeTree, ключ (bucket,clicks_uuid),
  day-size partition по bucket, argMin-state атрибуции/цены + sum-state `seen`.
  Нет TTL. Дубликаты из разных blocks/повторных Kafka deliveries объединяются
  **до** суммы денег. Результат не зависит от того, завершились ли merges.
* Проекция `generations` хранит сумму seen по сегменту в тех же parts. На каждом
  проходе читаются эти маленькие summaries и checkpoints, а не все UUID истории.
* `publisher_pop_revenue_dirty` выбирает сегменты с изменившимся поколением.
  `publisher_pop_revenue_delta` читает канонические UUID только этих сегментов,
  сравнивает entitlement с уже записанными деньгами того же bucket в agg_stats.
  Проекция `publisher_pop_by_bucket` позволяет отбирать их независимо от event_date.
* Список dirty buckets фиксируется одним non-correlated scalar Array на запрос
  и одинаково используется source/target ветками. Повторять обычную CTE/табличный
  subquery в двух ветках нельзя: конкурентный checkpoint может убрать уже оплаченный
  bucket между чтениями. Это отдельно проверено стресс-тестом с меняющимся checkpoint;
  настройка scalar optimization зафиксирована в SQL view.
* Единственная native refresh task `mv_publisher_pop_revenue_to_agg_stats` раз в
  минуту дописывает только остаток/коррекцию. Остальные метрики имеют нулевые defaults.
* Зависимая task `mv_publisher_pop_revenue_checkpoint` не доверяет факту запуска
  INSERT. Она заново проверяет совпадение **всех** групп сегмента с entitlement и
  только тогда сохраняет его поколение. Поколение и entitlement получаются одним
  registry scan. При конкурентном late insert он либо входит в snapshot, либо
  увеличивает generation и остаётся dirty для следующего прохода.
* Partial/unknown INSERT не помечается завершённым: следующий проход видит уже
  записанный кусок и дописывает остаток. Lost checkpoint ACK вызывает проверку,
  а не повторное безусловное начисление. Старый дубликат делает сегмент dirty,
  но его финансовая delta равна нулю.
* Новый `agg_stats.pop_revenue_bucket` — часть sorting key. Это обязательно:
  обычная numeric non-key колонка SummingMergeTree могла бы суммироваться/теряться.
  Старые агрегаты/прочие writers имеют bucket=0; остальные показатели не меняются.

Ресурсы: registry на диске O(уникальных UUID), без TTL — иначе нельзя распознать
произвольно старый повтор. Dirty discovery O(сегментов × немёрженных parts),
канонизация O(строк **изменившихся** сегментов), память зависит от этих сегментов,
а не всей истории. В активный час остаётся повторный пересчёт текущего часа.
Массовая доставка старых событий закономерно перечитывает затронутые старые сегменты.
Старые v4-сегменты не растут от нового Publisher трафика после выкладки v7 producer.
Это не O(1) при любом объёме и не отсутствие стоимости хранения истории.

Нельзя запускать второй независимый POP money writer или ручной INSERT той же
SELECT одновременно с native task. Одна согласованная ClickHouse DB/authority;
независимые Atomic databases на нескольких серверах не образуют общий lock.
`optimize_use_projections=1` должен оставаться включённым. Проверено на CH 25.8.18.1;
эта схема использует UUIDv7ToDateTime, projections с rebuild и native REFRESH/DEPENDS.
Не подменять её ReplacingMergeTree → additive money MV или block-local DISTINCT.
Арифметика внутри reconciliation Decimal128, прежнее поле Float64/округление
кабинета до 8 знаков сохраняются; бесконечная точность Float64 не обещается.

### Миграция и исключительный repair

CreateDB останавливает fact-click producer **до** отключения registry consumer и
воссоздаёт producer последним. Это закрывает окно пропуска кликов при рестарте.
Старый v1 registry сохраняется для импорта/аудита. Однократный cutover с marker=2:
обнулить только POP SSP revenue старого bucket=0, материализовать проекцию, потоково
импортировать сохранённые v1 states и retained fact_clicks, затем сохранить marker.
Advertiser amounts не обнуляются. Backend sync должен быть остановлен до сверки.
Повторное CreateDB не повторяет полный backfill и не сбрасывает checkpoint.
Не удалять migration marker и не очищать registry/checkpoints после запуска.

ClickHouse source insert и его downstream MV не являются общей транзакцией при
всех ошибках. При зафиксированном сбое source→registry insertion требуется repair
с сохранённых facts/v1 registry; режим включён в готовый код:

```bash
# Штатное окружение ClickHouse Loader, writers и финансовый sync остановлены:
./clickhouse-loader --schema-only --publisher-pop-repair
```

Он потоково импортирует retained events, не делает прямого начисления и безопасен
при повторе. Это исключительная maintenance-операция, а не минутный full refresh.
После неё дождаться money/checkpoint refresh и сверить суммы до возврата sync.
Нужны мониторинг ошибок ingestion/refresh и доступные исходные events. Если событие
не дошло ни до registry, ни до сохранившихся facts/Kafka, восстановить его нельзя.
Существующие TTL clicks_in/ORTB и факт-таблиц не расширены. Exactly-once транспорта
Kafka/ClickHouse insertion не заявляется.

## BURL v4: Redis-first, bounded execution, Bolt fallback

После существующей регистрации ADM-клика Publisher /adm отправляет и flush-ит
прежнюю HTML-страницу перехода. Только затем неблокирующий admission передаёт
доверенный контекст в общий processBurl. Нет self-HTTP /burl, request context или
неограниченной goroutine на каждый ADM. Обычный ADM не получает flush/queue шагов.

Общий semaphore ограничивает одновременно исполняемые fast/retry задачи числом
PUB_BURL_WORKERS (8 по умолчанию). При свободном worker BURL выполняется сразу
асинхронно. Успех обязательных внутренних Redis/ADV операций означает успех BURL;
успешная fast-задача **не создаёт Publisher task в Bolt**. Ошибка внутренней
операции сохраняется самим worker в прежние buckets того же ADV_OUTBOX_PATH.

Если worker занят, задачу принимает bounded staging buffer (128 × workers).
Один фиксированный persister сохраняет такие задачи в существующий due-index.
Сам HTTP handler никогда не ждёт BURL или Publisher fsync. Диспетчер и persister
фиксированы, активных рабочих goroutine не более workers; RAM O(workers),
дисковый backlog O(pending), индекс O(log pending). Новых ENV нет.

Конечный буфер не может принять бесконечный поток при остановившемся диске.
При полном staging задача явно теряется, CRITICAL + failure signal/остановка
upstream; redirect продолжается. Это предел best-effort admission, не обещание
сохранения всей перегрузки. Ошибка SaveTask (например ENOSPC) обрабатывается так
же: без бесконечного RAM retry. Логи показывают pending/active/capacity/staging,
fallback_attempts/lost; первый lost и степени двух логируются против log flood.
High-water durable backlog также использует прежний stop-streams сигнал.

После успешных внутренних операций и атомарного завершения Redis guard
Publisher POP передаёт **именно Record.BURL** в отдельный best-effort executor.
Его фиксированные 8 workers и очередь на 1024 уведомления не связаны с
PUB_BURL_WORKERS. Admission неблокирующий, без новых Redis lookup, Bolt или goroutine
на задачу. Основной BURL-worker освобождается без ожидания HTTP/DNS/DSP.
Уже действующий DSP NURL delegate остаётся Record.BURL; целевой URL не подменяется.
Обычный RTB BURL (не Publisher POP) сохраняет прежний синхронный HTTP-путь.

DSP 500/timeout/network/response-body error только диагностируются. Полная или
остановленная DSP-очередь отбрасывает уведомление; dropped логируется при первой
потере и степенях двух. Никаких retry внутреннего BURL/финансовых операций либо
Bolt-записей из-за DSP. Ошибка внутреннего Redis/guard completion либо обязательного
ADV intent остаётся ошибкой и использует существующий Bolt fallback. Если guard
больше не принадлежит worker, он не сообщает успех и не передаёт уведомление DSP.
HTTP использует прежний защищённый client: timeout 3s, ограничение body, DNS/IP и
redirect-проверки; executor дополнительно задаёт deadline 3s от начала отправки.
При shutdown отменяются текущие HTTP, workers завершаются, ожидающие необязательные
уведомления отбрасываются. Потеря после Redis done и до/во время отправки допустима:
DSP-доставка best-effort. Новых ENV, миграций и durable outbox нет.
ADV **по-прежнему сначала сохраняет financial write-ahead intent** через
ApplyDurableCallbackIntent, потом выполняет spend с прежним стабильным event ID.
Это отдельный обязательный финансовый outbox, его оптимизация не проводилась.

Внешний/внутренний Publisher DSP BURL используют один ADM DB1 guard. До истечения
guard повтор не начисляет advertiser spend/impression; busy означает retry, не ACK.
Impression UUID стабилен. Winner TTL не увеличен. Принят редкий повтор после
истечения guard; новые долгосрочные locks не добавлены. Внешний DSP может увидеть
повтор после crash с неизвестным результатом: exactly-once у DSP не обещается.

Штатный shutdown прекращает admission, отменяет и join-ит активные задачи,
сохраняет неуспешные fast-задачи и дренирует staging перед закрытием outbox.
Долговечные leases/ожидающие задачи восстанавливаются из того же volume. SIGKILL
до Redis completion/Bolt fallback может потерять RAM-задачу — согласованный риск.
Блокировка файловой системы может задержать shutdown/persister, но не admission
и уже отправленный redirect. Не делить Bolt-файл между работающими процессами.

Прежний retry horizon = min(24h, ADV_APPLIED_MARKER_TTL/2): старые durable задачи
сохраняются для оператора, автоматически не выполняются. ADV financial intent
остаётся отдельным. Не очищать/replay без проверки ledger. Полная потеря volume
не покрывается локальным outbox. ORTB TTL около часа не расширен: поздний BURL
или восстановленный ADM может не сформировать fact_impressions/fact_clicks.
Фиктивные показы, компенсации и отдельное хранилище аукционов не добавлены.
Publisher доход остаётся только по подтверждённым fact_clicks, независимо от BURL.

## Какие форматы оплачиваются

В обеих базах `internal/pubcontract/contract.go: Code.Validate` принимает только
`format=popunder`. В кабинете `internal/db/publisher.go` содержит CHECK(format='popunder').
Pub обслуживает существующий POP-путь; Publisher BAN/NAT/IPP/VID пока не реализованы.
Исходная impression MV не платила эти форматы. V1 ошибочно расширял для них SSP revenue.

Поэтому group_by=format сохраняет все пять buckets и их реальные counters, но
неподдерживаемые форматы имеют revenue=0 в статистике/lifetime reports и не попадают
в баланс. PublisherCumulativeRevenue остаётся POP-only по явному основанию в коде.
Чужие SSP не становятся publisher income: требуется pub_user_id и owner filter.
Создание других форматов, новые тарифы/правила выплат этим патчем не включаются.
PG ledger, revenue checkpoints, начисление delta раз в 30s и payouts не меняются.

## Redis lifecycle и ENV

В proj240 после точечного отката callback снова передают id/url/f/s по контракту
proj212. Полный callback-контекст для обычных ставок не собирается и не читается;
ошибка его сохранения больше не исключает победителя. Publisher POP добавляет p=1
только в ADM. Для DSP в существующем ADM UUID HASH сохраняется только pub_dsp_nurl, без
изменения TTL. ADM читает uuid/NURL одним HGETALL; отсутствие NURL не отменяет
внутренний BURL. Для ADV используется прежний ReadWinner. Очередь, финансовый
Outbox и guards сохранены. См. актуальную
инструкцию LEGACY_CALLBACKS_240.md.

| ENV | Default | Назначение |
|---|---|---|
| PUB_BURL_WORKERS | 8 | 1..64, adm-adapter, активные Publisher BURL |
| PUB_BURL_TASK_TIMEOUT | 20s | положительный, не более 1m; DSP HTTP timeout остаётся 3s |
| PUB_BURL_BACKLOG_HIGH_WATER | 100000 | сигнал существующей остановки streams; не drop лимит |

Существующие настройки не подменяются: ADV_OUTBOX_PATH (default
./data/adv-billing-outbox.db), ADV_APPLIED_MARKER_TTL (720h), REDIS_UUID_KEY_TTL
(45m DSP), ADV_WINNER_TTL (45m ADV), REDIS_DB_ADM=1, REDIS_DB_BURL=0,
REDIS_DB_ADV_WINNER=6, ADV runtime DB5. ADM Adapter REDIS_ADV_ADDR обязан
указывать на настоящий ADV winner Redis (его собственный REDIS_UUID_ADDR),
а не на DSP Redis другого инстанса/порта. PUB_ANTIBLOCK_ENABLED=false сохраняется.

На существующих Redis primary нужны AOF и достаточная retention/noeviction.
Для минимального риска потери подтверждённого write — appendonly yes,
appendfsync always, no-appendfsync-on-rewrite no, maxmemory-policy noeviction.
appendfsync everysec допускает потерю примерно последней секунды при crash;
replication асинхронна и сама по себе не гарантирует durable failover. При
использовании реплик полезны min-replicas-to-write/min-replicas-max-lag, но это
tradeoff доступности, не гарантия записи каждой команды на диск всех реплик.
Настройки применяются вашей штатной Redis-инфраструктурой, не новым cluster.
Retention настраивать ДО выдачи новых callbacks по фактическим late-event срокам.
Если все серверные копии уничтожены/evicted/истекли — восстановление без источника
невозможно; URL-параметры не возвращаются даже при этом отказе.

## Безопасная выкладка

Для обновления v3 → v4 собрать полный replacement на чистом original checkout.
V4 поверх V3 через git apply не накладывать. Финансовые DDL/checkpoints, Redis DB/TTL,
ADV outbox и кабинет не меняются; **не запускать reset/backfill/replay**.
На каждом Adapter прекратить admission, дренировать HTTP и вызвать штатный Close,
сохранить Bolt volume, заменить бинарник, проверить canary и продолжить rollout.
Close сначала завершает внутреннюю BURL-очередь, затем отменяет отдельные DSP-workers.
Откат v4 → v3 совместим с существующими Redis/Bolt данными, возвращает ожидание DSP
в основном worker. Ни доход, ни финансовый outbox при таком откате не обнулять.
Кабинет V4 побайтово равен V3, его повторный запуск для этой доработки не требуется.
Ниже сохранён порядок первой выкладки/старых версий; он не нужен повторно при v3 → v4.

Если production уже на v2: финансовая DDL, UUID lookup и кабинет в v3 не меняются.
Не выполнять повторный reset/backfill. Собрать v3 на чистом original checkout,
остановить admission на экземпляре, штатно дренировать HTTP и Close BURL queue,
сохранить прежний Bolt volume, выкатить новый adm-adapter; сначала canary с
контролем pending/lost/fallback_attempts и Redis errors. Затем остальные экземпляры.
Кабинет можно не перезапускать, если его бинарник уже соответствует полному v2.
Откат только v3→v2 Adapter совместим с тем же Bolt task schema/UUID context; он
вернёт обязательный Publisher task fsync. ADV financial outbox не удалять/не replay.
При SIGKILL до сохранения fallback RAM-задачи v3 восстановить гарантированно нельзя.
Далее порядок для чистых исходных архивов/первой финансовой миграции:

1. На чистых checkout применить v3, собрать оба проекта, прогнать disposable tests.
   Сверить production CH version/profile/projection support; Redis DB/address mapping,
   AOF/retention, persistent outbox volume и место на диске. Не тестировать разрушительно
   на production. Сохранить backup schema/data, aggregate advertiser totals, PG
   ledger/checkpoints/accounts и Kafka offsets. Финансовые checkpoints не обнулять.
2. Исторический этап перехода на UUID-only отменён точечным откатом proj240.
   Порядок установки callback — только в LEGACY_CALLBACKS_240.md; финансовые
   этапы ниже при этом откате не выполнять.
3. На maintenance остановить Publisher выдачу и **backend revenue sync/переходы
   выплат**, все ClickHouse Loaders/ручные fact writers, дождаться async inserts.
   Kafka сохранить. Не затягивать окно за TTL input buffers/ORTB. Сверить balances
   и advertiser SUM до миграции. Обычный внешний трафик приостанавливается штатным
   maintenance/control-порядком, а не потерей очереди.
4. Одним экземпляром нового бинарника с обычным env Loader выполнить:
   `./clickhouse-loader --schema-only`. Не запускать параллельно CreateDB.
   CLI не поднимает Kafka consumers. Исходный IPP repair сохраняется без изменения
   своей двухчасовой границы/формул. Новые MV definitions/registry/key устанавливаются
   штатным DDL, one-time POP import выполняется до запуска money task.
5. На фактической DB (ниже пример ads) принудительно завершить оба native refresh:

```sql
SYSTEM REFRESH VIEW ads.mv_publisher_pop_revenue_to_agg_stats;
SYSTEM WAIT VIEW ads.mv_publisher_pop_revenue_to_agg_stats;
SYSTEM REFRESH VIEW ads.mv_publisher_pop_revenue_checkpoint;
SYSTEM WAIT VIEW ads.mv_publisher_pop_revenue_checkpoint;
SELECT * FROM ads.publisher_pop_revenue_dirty;
SELECT version FROM ads.publisher_pop_revenue_migrations;
SELECT sorting_key FROM system.tables WHERE database='ads' AND name='agg_stats';
SELECT pub_user_id,sum(ssp_revenue_views) FROM ads.agg_stats
 WHERE format='POP' AND pub_user_id!='' GROUP BY pub_user_id;
SELECT win_user_id,win_cid,sum(spend_views_table),sum(spend_clicks_table)
 FROM ads.agg_stats GROUP BY win_user_id,win_cid ORDER BY win_user_id,win_cid;
SELECT database,view,status,exception FROM system.view_refreshes WHERE database='ads';
```

   Сверить три POP writer/checkpoint views, обе projections, registry totals и delta;
   advertiser SUM до/после должны совпасть при остановленных producers. Учесть
   исходный штатный IPP repair window, не выполнять повторную агрегацию advertiser
   facts. Не возобновлять sync при ошибке/неполном cutover.
6. Выкатить Adapter с bounded outbox worker и кабинет (полный v2 patch без изменений, имя v3). Убедиться в сохранности
   pending billing/Publisher tasks. Запустить Loader consumers, дождаться Kafka
   lag/refresh. Повторный CreateDB пропускает завершённый full import.
7. Для исторической финансовой миграции проверить ADV/DSP ссылки всех
   форматов и Publisher signed delivery → тот же ADM. Возобновить sync, сверить
   account delta с ledger/checkpoint, затем включить Publisher/обычные streams.
   Проверить no-fill, popup blocking, /confirm, quotas и 30s sync на canary.

## Откат без повторного начисления

Остановить Publisher, sync и переходы выплат, дренировать/сохранить outbox, остановить
Loaders. Снять диагностический backup. Оставить v2 registry, checkpoints, agg_stats,
sorting key/projections, PG ledger и Kafka offsets **вместе**. Не очищать migration
marker, не переводить BURL обратно в источник Publisher дохода и не replay advertiser
facts. Не запускать старый CreateDB: он вернёт старые финансовые writers/ключ.

Безопасный временный rollback — отключённый Publisher/финансовый sync при сохранённой
v2 денежной схеме и исправление/roll-forward бинарника. Обычные RTB сервисы можно
возвращать только с совместимым UUID resolver и без старого DDL initializer.
Старый handler, требующий url/f/s, не обслужит уже выданные UUID-only callbacks.
Уже выданные ссылки без url/f/s не содержат данных старого механизма; см. порядок перехода в LEGACY_CALLBACKS_240.md.

Полный rollback данных возможен только согласованным snapshot всех финансовых
состояний на одну границу и audited replay с неизменными UUID/offsets; одним
`git apply -R` это не делается. Уже выплаченные деньги/PG checkpoints не откатывать
автоматически. После любого восстановления agg_stats и registry/checkpoints должны
быть согласованы; отдельный restore только одного из них запрещён.

## Проверки и границы уверенности

Локально Go 1.25.11, Redis 7.0.15 с AOF, ClickHouse 25.8.18.1. Только disposable
экземпляры. Полный отчёт приложен отдельно как TwinBid_229_v3_Deploy_and_Checks.md.
Есть реальные Redis/ClickHouse integration, source DDL migration/repeat migration,
BURL queue SIGKILL/lease recovery, concurrent ADM, ADV stable billing, SSRF guards,
UUID-only ADV/DSP × 5 formats, race detector, source/production JS VM. Benchmark
100k/1M/3M history events выполнен ранее для v2; финансовый SQL побайтово сохранён,
в v3 этот большой benchmark не повторялся. В v3 измерены три пары HTTP latency
прогонов v2/v3. npm client build --check падает и на исходном ZIP: runtime.prod.js
is stale; клиентские исходники/production JS не менялись. VM не выдаётся за браузер.

Не заявляется production-ready/live E2E: отсутствуют production-конфигурация/версия
CH, реальный Kafka broker/DSP, disposable PostgreSQL для live account/ledger теста,
браузеры/расширения и актуальный frontend. Репликационный Redis failover не измерен.
Существующие общие тесты исходных архивов имеют отдельные падения вне этой задачи;
их не маскируем исправлениями рекламодателей. Перед production обязательны canary,
живой финансовый цикл и проверка фактических durability настроек.
