# TwinBid API Contract

Контракт между фронтом кабинета и бэкендом. Фронт хранит JWT в `localStorage` и
шлёт его в заголовке `Authorization: Bearer <token>`. Все JSON-ответы — UTF-8.

> Источник данных:
> - **Postgres** — всё, кроме статистики откруток.
> - **ClickHouse** — любая статистика (impressions, clicks, spent, CTR, разрезы).
> - **S3** — файлы креативов (presigned URL).

Base URL фронта берётся из `VITE_API_BASE_URL`. Все ручки начинаются с `/api`.

---

## 0. Общие правила

- Ошибки: `{ "error": { "code": "string", "message": "string", "fields"?: { [k]: string } } }` со статусами 4xx/5xx.
- Пагинация: query `?limit=50&offset=0`, ответ `{ items: [...], total: number }`.
- Время: ISO 8601 в UTC (`2025-04-22T10:00:00Z`), даты — `YYYY-MM-DD`.
- HASHMAP-таргетинги (country/language/device_type/os/browser/site_id/ip):
  объект `{ "<value>": 1 | 0 }`. `1` = whitelist, `0` = blacklist.
  Пустой объект `{}` = таргетинг не применён.

---

## 1. Auth (Postgres `users`)

### POST `/api/auth/signup`
Body: `{ email, password, full_name?, manager_telegram }` (manager_telegram приходит с фронта как константа, по умолчанию `"GregTwinbid"`).
Resp 201: `{ access_token, refresh_token, user: User }`

### POST `/api/auth/login`
Body: `{ email, password }`.
Resp 200: `{ access_token, refresh_token, user: User }`

### POST `/api/auth/refresh`
Body: `{ refresh_token }`. Resp 200: `{ access_token, refresh_token }`

### POST `/api/auth/logout`
Auth required. Resp 204.

### `User` schema
```json
{
  "login": "user@example.com",
  "mail": "user@example.com",
  "name": "Ivan",
  "telegram": "@ivan",
  "manager_telegram": "GregTwinbid",
  "balance": 0,
  "timezone": "utc_3",
  "email_notifications": true,
  "campaign_status_notifications": true,
  "low_balance_notifications": true,
  "campaign_balanse_notifications": true,
  "balance_treshold": 100
}
```

---

## 2. Profile (Postgres `users`)

### GET `/api/profile` → `User`
### PATCH `/api/profile`
Body: any subset of `{ name, telegram, timezone, email_notifications, campaign_status_notifications, low_balance_notifications, campaign_balanse_notifications, balance_treshold }`.
Resp: `User`.

> Поле `manager_telegram` не редактируется через фронт — только админом со стороны бэка.

---

## 3. Campaigns (Postgres `campaigns`)

### `Campaign` schema (соответствует таблице)
```json
{
  "campaign_id": "uuid",
  "user_id": "uuid",
  "campaign_name": "string",
  "format_type": "banner | popunder | native | push",
  "brand_name": "string?",
  "h": 250, "w": 300,
  "status": "active | paused | draft | completed | moderation",
  "traffic_type": "mainstream | adult | mixed",
  "vertical": ["Dating", "Nutra"],
  "pricing_model": "cpm | cpc",
  "base_price_cpm": 0.05,
  "base_price_cpc": 0.0001,
  "evenness_by_slot_mode": false,
  "goal_total_dollars": 1000,
  "cum_done_dollars": 0,
  "start_ts": "2025-04-22T00:00:00Z",
  "end_ts": "2025-05-22T23:59:59Z",
  "active_intervals": [["mon,1","thu,2"], ["wed,3","fri,5"]],
  "country": { "US": 1, "RU": 1 },
  "language": {},
  "device_type": { "mobile": 1 },
  "os": {},
  "browser": {},
  "site_id": {},
  "ip": {}
}
```

> Если расписание выключено — `active_intervals = [["mon,0","sun,23"]]`.
> `cum_done_dollars` приходит ТОЛЬКО с бэка (не пишется фронтом).

### GET `/api/campaigns?status=&limit=&offset=` → `{ items: Campaign[], total }`
### GET `/api/campaigns/:id` → `Campaign`
### POST `/api/campaigns` (без `cum_done_dollars`, без `campaign_id`) → `Campaign`
### PATCH `/api/campaigns/:id` → `Campaign`
### DELETE `/api/campaigns/:id` → 204
### POST `/api/campaigns/:id/status` body `{ status }` → `Campaign`

---

## 4. Creatives (Postgres `pop_creatives` / `ban_creatives` / `ipp_creatives` / `nat_creatives`)

Тип креатива выбирается по `format_type` кампании:
- `popunder` → `pop_creatives`
- `banner` → `ban_creatives`
- `push` → `ipp_creatives` (in-page push)
- `native` → `nat_creatives`

### Общая `Creative` форма (ответ бэка)
```json
{
  "id": "uuid",
  "campaign_id": "uuid",
  "creative_name": "string",
  "link": "https://target.example",
  "trackers_macros": { "{CLICK_ID}": 1, "{ZONE_ID}": 0 },

  // banner only:
  "w": 300, "h": 250,
  // banner / push / native (заполняются БЭКОМ после загрузки файла):
  "name": "banner_300x250.png",
  "presigned_s3_url": "https://s3.amazonaws.com/...&X-Amz-Signature=...",
  // push / native:
  "title": "string",
  "description": "string"
}
```

> Поле `name` — это имя файла загруженной картинки. Его выставляет бэк при загрузке; фронт отображает его в кабинете как подпись к картинке.
> `presigned_s3_url` — временный signed URL для чтения, который бэк добавляет в ответы GET. Фронт использует его в `<img src>`.
> Фронт никогда не пишет ни `name`, ни `presigned_s3_url`. Никаких `s3_file_path` / `file_format` в контракте больше нет.

### GET `/api/campaigns/:id/creatives` → `Creative[]` (с `presigned_s3_url`, `name`)
Внутренний клиентский метод фронта, который вызывает эту ручку, называется **`readCreatives`**.

### POST `/api/campaigns/:id/creatives`
**Multipart form-data** (фронт всегда шлёт multipart, даже если файла нет):
- JSON-поля: `creative_name`, `link`, `trackers_macros`, `w?`, `h?`, `title?`, `description?`;
- `file` *(опционально)* — бинарь картинки;
- `filename` *(опционально, обязательно если есть `file`)* — имя файла.

Бэк сам кладёт файл в S3 **и записывает имя файла в поле `name` строки креатива.** Resp: `Creative` (с `presigned_s3_url` и `name`, если файл был).

### PATCH `/api/creatives/:id`
То же самое: multipart form-data с любым подмножеством JSON-полей и опциональными `file` + `filename`. Если `file` пришёл — бэк перезаписывает картинку и обновляет `name`.

### DELETE `/api/creatives/:id` → 204

---

## 5. Transactions & promo (Postgres `user_transactions`, `promocodes`)

### `UserTransaction`
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "transaction_time": "iso",
  "transaction_id": "string",
  "payment_method": "usdt_trc20 | usdt_erc20 | ...",
  "bonus_amount": 25,
  "promocode_id": "uuid?",
  "transaction_hash": "string?",
  "deposit_amount": 100,
  "total_balance_increase": 125,
  "status": "draft | pending | approved | rejected | cancelled",
  "currency": "usdt",
  "created_at": "iso",
  "updated_at": "iso"
}
```

### Поток пополнения
1. `POST /api/transactions` — создаёт транзакцию. Фронт присылает **все** поля `UserTransaction`, которые может посчитать сам (`user_id`, `transaction_time`, `transaction_id`, `payment_method`, `bonus_amount`, `promocode_id`, `transaction_hash`, `deposit_amount`, `total_balance_increase`, `status`, `currency`). Бэк присваивает только PK `id` и проставляет `created_at` / `updated_at`:
   ```json
   {
     "user_id": "uuid",
     "transaction_time": "iso",
     "transaction_id": "string",
     "payment_method": "usdt_trc20",
     "bonus_amount": 25,
     "promocode_id": "uuid?",
     "transaction_hash": "string?",
     "deposit_amount": 100,
     "total_balance_increase": 125,
     "status": "draft | pending",
     "currency": "usdt"
   }
   ```
   Resp: `UserTransaction`.
2. `PATCH /api/transactions/:id` — частичное обновление (например `{ transaction_hash, status: "pending" }`). Resp: `UserTransaction`.
3. `POST /api/transactions/:id/cancel` → `cancelled`. Resp: `UserTransaction`.
4. `GET /api/transactions?status=&limit=&offset=` → история.

> Внутренние клиентские методы фронта: `listTransactions`, `createTransaction`, `patchTransaction`, `cancelTransaction`.

### Promo
- GET `/api/promocodes/:code` → `{ id, promocode_text, bonus_percent, usage_count, usage_limit, valid_from, valid_to }`.
  Бэк проверяет: код активен, не достигнут `usage_limit`, не использован этим юзером ранее (через `user_transactions.promocode_id`).
  Если нельзя — 400 `{ error: { code: "PROMO_LIMIT" | "PROMO_USED" | "PROMO_EXPIRED", message } }`.

---

## 6. Notifications (Postgres `notifications`)

### Schema
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "transaction_id": "uuid?",
  "campaign_id": "uuid?",
  "deposit_amount": 100,
  "status": "active | inactive",
  "text": "string",
  "type": "incomplete_topup | low_balance | campaign_status | other"
}
```

### GET `/api/notifications?status=active` → `Notification[]`
### POST `/api/notifications` body `{ type, text, transaction_id?, campaign_id?, deposit_amount? }` → `Notification` (создание с фронта, например для незавершённой транзакции).
### PATCH `/api/notifications/:id` body `{ status }` → `Notification` (отметка как `inactive`).

> Уведомление о незавершённой транзакции переводится в `inactive` ТОЛЬКО при отмене транзакции (`POST /api/transactions/:id/cancel`) или при её успешном завершении (статус `pending`/`approved`).

---

## 7. Statistics — ClickHouse

> Все ручки ниже — read-only выборки из таблицы `ads.agg_stats` (ClickHouse).
> **Весь SQL живёт на бэкенде.** Фронт никогда не формирует и не отправляет SQL —
> он только передаёт параметры (даты, фильтры, group_by) в JSON.
> Референсные SQL-запросы для бэка лежат в `src/api/clickhouse-queries.sql`
> (этот файл — временный, существует только как образец, его удалит бэкендер
> после интеграции).
>
> Авторизация: `Authorization: Bearer <jwt>`. Бэк извлекает `user_id` из JWT и
> ВСЕГДА подставляет его в `WHERE user_id = ...`. Фронт `user_id` НЕ шлёт.
>
> Все даты — `YYYY-MM-DD` в UTC 0. Бэк маппит их в `{date_from:Date}` /
> `{date_to:Date}`. Если `from`/`to` пустые строки — бэк не накладывает
> фильтр по датам (или применяет дефолт «последние 90 дней»).

---

### 7.1 POST `/api/stats/query` — единственная ручка статистики

Используется страницами **Overview**, **Campaigns** и **Statistics**. Бэк по
значению `group_by` выбирает соответствующий запрос из секции 3
`clickhouse-queries.sql` и одновременно выполняет totals-запрос (секция 4) с
теми же фильтрами. Никаких отдельных эндпоинтов вроде `/api/stats/overview`
или `/api/stats/campaign/:id/summary` больше нет — это были обёртки над этой
же ручкой; теперь Overview/Campaigns шлют тот же `POST /api/stats/query`
с `group_by: "campaign"` и нужными `campaign_ids`.

**Request body:**
```json
{
  "from": "2025-04-22",
  "to":   "2025-04-22",
  "campaign_ids":  ["uuid", "..."],
  "creative_ids":  ["uuid", "..."],
  "group_by": "date",
  "filters": {
    "country":     ["US", "RU"],
    "browser":     ["chrome"],
    "os":          ["android"],
    "device_type": ["mobile"]
  }
}
```

| Поле | Тип | Обяз. | Описание / маппинг на ClickHouse-параметры |
|---|---|---|---|
| `from` | `string YYYY-MM-DD` | да* | `{date_from:Date}`. Пустая строка = без нижней границы. Для одной конкретной даты фронт шлёт `from = to`. |
| `to`   | `string YYYY-MM-DD` | да* | `{date_to:Date}`. Пустая строка = без верхней границы. |
| `campaign_ids` | `string[]` (UUID) | нет | `{campaign_ids:Array(UUID)}`. `[]` или отсутствует = все кампании пользователя. Поддерживается мульти-выбор: пользователь может смотреть стату по нескольким кампаниям одновременно. |
| `creative_ids` | `string[]` (UUID) | нет | `{creative_ids:Array(UUID)}`. `[]` = все креативы. |
| `group_by` | `StatsGroupBy` | да | **Скаляр**, не массив. При смене группировки фронт шлёт новый запрос. Допустимые значения: `date`, `hour`, `country`, `os`, `browser`, `device_type`, `site_id`, `campaign`. |
| `filters.country` | `string[]` | нет | `{f_geo:Array(String)}` (колонка `geo`). |
| `filters.browser` | `string[]` | нет | `{f_browser:Array(String)}`. |
| `filters.os` | `string[]` | нет | `{f_os:Array(String)}`. |
| `filters.device_type` | `string[]` | нет | `{f_device_type:Array(String)}`. |

\* Поля присутствуют всегда; пустая строка означает «не фильтровать».

> Тип `StatsFilterBy` (ключи `filters`) уже́е, чем `StatsGroupBy`: туда входят
> только `country | os | browser | device_type`. По остальным группировкам
> фильтрации с фронта нет.


**Response:**
```json
{
  "rows": {
    "DE": { "impressions": 1000, "clicks": 50, "spent": 12.3, "ctr": 0.05 },
    "FR": { "impressions": 800,  "clicks": 40, "spent": 10.1, "ctr": 0.05 }
  },
  "totals": { "impressions": 1800, "clicks": 90, "spent": 22.4, "ctr": 0.05 }
}
```

`rows` — это **map**, где ключ = значение группировки, а значение =
`StatsSummary` (метрики этой группы). Формат ключа зависит от `group_by`:

| `group_by` | Тип ключа в `rows` |
|---|---|
| `date` | `"YYYY-MM-DD"` |
| `hour` | `"YYYY-MM-DD HH:00"` (UTC) |
| `campaign` | UUID кампании (string) |
| `country` | ISO-код (`"US"`) |
| `os` | строка |
| `browser` | строка |
| `device_type` | строка |
| `site_id` | строка |

Поля метрик во всех записях одинаковые: `impressions:int`, `clicks:int`,
`spent:number` (USD, 2 знака), `ctr:number` (проценты, 2 знака).

`totals` — `StatsSummary` со сводкой по тем же `WHERE`-условиям без `GROUP BY`.

---

### 7.2 Где это лежит на фронте

- Типы запроса/ответа: `src/api/types.ts`
  (`StatsQueryRequest`, `StatsQueryResponse`, `StatsSummary`, `StatsGroupBy`, `StatsFilterBy`).
- Клиентский метод: `statsQuery(req: StatsQueryRequest): Promise<StatsQueryResponse>` → `POST /api/stats/query`.
- HTTP-реализация: `src/api/httpProvider.ts` (только сериализация JSON, никакой бизнес-логики).
- Потребители:
  - `src/components/dashboard/StatsCards.tsx` + `src/hooks/use-campaign-stats.ts` — Overview KPI (через `group_by: "campaign"`).
  - `src/pages/DashboardCampaigns.tsx` — per-row summary (через тот же хук).
  - `src/pages/DashboardStatistics.tsx` (через `src/contexts/StatisticsContext.tsx`) — flexible group_by + мульти-выбор кампаний.

---

### 7.3 POST `/api/calculator` — доступный объём по таргетингам

Используется страницей `/dashboard/traffic-calculator`. Ручка не делает
прогнозов и не использует модель оплаты или ставку. Она берёт последнюю
полностью закрытую дату из таблицы статистики запросов на показ рекламы,
применяет переданные таргетинги и возвращает исторический объём доступных
показов за эту дату.

SQL целиком остаётся на бэкенде. Фронт передаёт только параметры фильтрации:

```json
{
  "format_type": "banner",
  "traffic_type": "mainstream",
  "country": ["DE", "FR"],
  "country_mode": "include",
  "language": ["de"],
  "language_mode": "include",
  "device_type": ["desktop"],
  "device_type_mode": "include",
  "os": ["Windows"],
  "os_mode": "include",
  "browser": ["Chrome"],
  "browser_mode": "include"
}
```

Поле `*_mode` равно `include` для белого списка и `exclude` для чёрного.
Пустой массив означает «все значения» независимо от режима. Поля
`verticals`, `verticals_mode`, `pricing_model`, `bid` и `campaign_id` в эту
ручку не отправляются.

Бэк сам определяет последнюю полностью закрытую дату. Текущие неполные сутки
не используются.

**Response:**

```json
{
  "potential_impressions": 128400
}
```

Если выбрана кампания, фронт после этого ответа вызывает существующий
`POST /api/stats/query` за предыдущую полную UTC-дату с `campaign_ids`
выбранной кампании и `group_by: "campaign"`. Он выводит `totals.clicks`,
`totals.impressions` и считает полученную долю показов как
`totals.impressions / potential_impressions * 100`.

Размер ставки меняется существующим `PATCH /api/campaigns/:id` с body
`{ "base_price": 1.25 }`. `pricing_model` не отправляется и остаётся прежним.

---

### 7.4 POST `/api/recommend_bid` — средняя ставка по сегменту

Ручка вызывается при переходе с таргетингов на шаг бюджета и ставки:

- при создании кампании — переход с шага 2 на шаг 3;
- при редактировании кампании — переход со вкладки «Таргетинг» на вкладку
  «Бюджет» кнопкой «Далее».

Ручка использует последнюю полностью закрытую дату и возвращает среднюю
**ненулевую** выигравшую ставку по выбранному сегменту. Прогнозная модель не
используется.

Фронт отправляет тот же набор параметров сегмента, что и в `/api/calculator`:

```json
{
  "format_type": "popunder",
  "traffic_type": "mixed",
  "country": ["DE", "FR"],
  "country_mode": "include",
  "language": ["de"],
  "language_mode": "include",
  "device_type": ["desktop"],
  "device_type_mode": "include",
  "os": ["Windows"],
  "os_mode": "include",
  "browser": ["Chrome"],
  "browser_mode": "include"
}
```

Допустимые значения:

- `format_type`: `banner | popunder | native | push`;
- `traffic_type`: `mainstream | adult | mixed`;
- `*_mode`: `include | exclude`;
- пустой массив означает отсутствие ограничения по измерению.

В запрос не передаются `verticals`, `verticals_mode`, `pricing_model`, текущая
ставка, качество трафика и `campaign_id`.

**Response:**

```json
{
  "average_bid": 1.24
}
```

- `average_bid` — средняя ненулевая выигравшая ставка сегмента.

Единица `average_bid` определяется форматом: для `push` это CPC, для `banner`,
`native` и `popunder` — CPM. Если у `popunder` пользователь переключает модель
на CPC, фронт переводит рекомендацию тем же коэффициентом, который уже
используется для пересчёта минимальной CPM-ставки в CPC.

Фронт показывает:

1. существующую обязательную минимальную ставку;
2. `average_bid` как минимально рекомендованную;
3. `average_bid × random(1.9, 2.3)` как оптимальную рекомендованную.

Случайный коэффициент генерируется на фронте один раз для каждого успешного
ответа. Ниже существующей минимальной ставки сохранить кампанию нельзя.
Максимум для `popunder` в модели CPM — `$50`; остальные текущие ограничения
остаются без изменений.

`recommend_bid` является необязательным улучшением интерфейса. При сетевой
ошибке, неуспешном ответе, отсутствующем или неположительном `average_bid`
фронт не показывает динамические чекпоинты и продолжает использовать прежние
статические минимальные и рекомендованные значения. Ошибка ручки не блокирует
создание или редактирование кампании.

---

## 8. Маппинг фронт-экранов на ручки

| Экран | Источник |
|---|---|
| `/dashboard` overview cards | `POST /api/stats/query` с `group_by: "campaign"` (totals идут в `totals`) + `GET /api/profile` |
| Список кампаний на overview/campaigns | `GET /api/campaigns` (Postgres) + `POST /api/stats/query` с `group_by: "campaign"` (одним запросом на все строки) |
| `/dashboard/statistics` | `POST /api/stats/query` (ClickHouse) |
| `/dashboard/traffic-calculator` | `POST /api/calculator` + `POST /api/stats/query` для выбранной кампании + `PATCH /api/campaigns/:id` для изменения только размера ставки |
| `/dashboard/balance` баланс/история | `GET /api/profile`, `GET /api/transactions`, `POST /api/transactions`, `PATCH /api/transactions/:id`, `POST /api/transactions/:id/cancel` |
| Создание/редактирование кампании | `POST /api/recommend_bid` при переходе от таргетингов к ставке + `POST/PATCH /api/campaigns`, `POST /api/creatives/upload-url`, CRUD `/api/creatives` |
| Уведомления (колокольчик) | `GET/POST/PATCH /api/notifications` |
| Настройки | `GET/PATCH /api/profile` |
| Auth | `/api/auth/*` |

---

## 9. Фронт-флаги

- `VITE_API_BASE_URL` — base URL бэка (например `https://api.twinbid.com`).
- `VITE_USE_MOCK` — `"true"` использует mock-провайдер с фикстурами в `src/api/mocks/*`. По умолчанию `true` пока бэк не готов.
