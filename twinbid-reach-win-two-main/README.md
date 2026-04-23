Вот переработанная документация API и всех типов данных, основанная на ваших исходных файлах. В ней:

- Все эндпоинты с указанием метода, пути, тела запроса (какие поля, их типы, обязательность) и тела ответа.
- Все типы данных вынесены в отдельные таблицы и перечисления.
- Убраны дублирующиеся определения и избыточный код.
- Сохранены важные детали (например, `campaing_id` с опечаткой, формат `multipart/form-data` для креативов, авторизация).

---

# API TwinBid — полная спецификация (по фронтовому контракту)

## Общие правила

- **Формат данных**: все поля в запросах и ответах — `snake_case`.
- **Авторизация**: для всех запросов, кроме `signup`, `login`, `refresh`, требуется заголовок:  
  `Authorization: Bearer <access_token>`
- **Кодировка**:
  - Обычные запросы — `application/json`.
  - Создание и обновление креативов (`createCreative`, `patchCreative`) — `multipart/form-data`.  
    При этом поля, не являющиеся строками (объекты, массивы), фронт сериализует в JSON-строки.
- **Даты** передаются в ISO-формате (`YYYY-MM-DD` или `YYYY-MM-DDTHH:mm:ssZ`).
- **Ошибки**: в текущей спецификации нет детального описания кодов ошибок. Фронт ожидает HTTP-статусы 4xx/5xx и тело с полем `{ error: { message, code?, fields? } }`.

---

## Типы данных

### Перечисления (enum)

| Тип | Возможные значения |
| --- | ----------------- |
| `CampaignStatus` | `active`, `paused`, `draft`, `completed`, `moderation` |
| `PricingModel` | `cpm`, `cpc` |
| `TrafficType` | `mainstream`, `adult`, `mixed` |
| `FormatType` | `banner`, `popunder`, `native`, `push` |
| `TopupStatus` | `draft`, `pending`, `approved`, `rejected`, `cancelled` |
| `NotificationType` | `incomplete_topup`, `low_balance`, `campaign_status`, `other` |
| `NotificationStatus` | `active`, `inactive` |
| `StatsGroupBy` | `date`, `hour`, `campaign`, `country`, `format`, `creative`, `os`, `browser`, `device_type`, `language`, `site_id` |

### Вспомогательные типы

| Тип | Описание |
| --- | -------- |
| `TargetingMap` | `Record<string, 0 \| 1>` – ключ — значение таргетинга (например, `"US"`), `1` = белый список, `0` = чёрный. Пустой объект = нет таргетинга. |
| `ScheduleInterval` | `[string, string]` – кортеж из двух строк, интервал времени (например, `["mon,1", "thu,2"]`). |

---

### Основные объекты

#### `ApiUser` – профиль пользователя

| Поле | Тип | Обязательное | Описание |
| ---- | --- | ------------ | -------- |
| `login` | string | да | Логин |
| `mail` | string | да | Email |
| `name` | string | да | Отображаемое имя |
| `telegram` | string \| null | да | Telegram пользователя |
| `manager_telegram` | string | да | Telegram менеджера |
| `balance` | number | да | Текущий баланс |
| `timezone` | string | да | Часовой пояс (например, `utc_3`) |
| `email_notifications` | boolean | да | Вкл/выкл email-уведомления |
| `campaign_status_notifications` | boolean | да | Уведомления о статусе кампании |
| `low_balance_notifications` | boolean | да | Уведомления о низком балансе |
| `campaign_balanse_notifications` | boolean | да | Уведомления о балансе кампании (опечатка в оригинале) |
| `balance_treshold` | number | да | Порог баланса для уведомлений |

#### `ApiCampaign` – кампания

Обратите внимание: поле ID кампании называется `campaing_id` (с опечаткой), НЕ `campaign_id`.

| Поле | Тип | Обязательное | Примечание |
| ---- | --- | ------------ | ---------- |
| `campaing_id` | string | да | Уникальный ID кампании |
| `user_id` | string | да | ID владельца |
| `campaign_name` | string | да | Название кампании |
| `format_type` | FormatType | да | Формат рекламы |
| `brand_name` | string \| null | нет | Бренд |
| `h` | number \| null | нет | Высота (для баннеров) |
| `w` | number \| null | нет | Ширина (для баннеров) |
| `status` | CampaignStatus | да | Статус |
| `traffic_type` | TrafficType | да | Тип трафика |
| `vertical` | string[] | да | Массив вертикалей (тем) |
| `pricing_model` | PricingModel | да | Модель ценообразования |
| `base_price_cpm` | number | да | Цена за 1000 показов |
| `base_price_cpc` | number | да | Цена за клик |
| `evenness_by_slot_mode` | boolean | да | Равномерный показ по слотам |
| `goal_total_dollars` | number | да | Бюджет кампании в долларах |
| `cum_done_dollars` | number | да | Сумма, потраченная на данный момент (заполняется бэком) |
| `start_ts` | string | да | Дата начала (ISO) |
| `end_ts` | string | да | Дата окончания (ISO) |
| `active_intervals` | ScheduleInterval[] | да | Расписание внутри дня |
| `country` | TargetingMap | да | Таргетинг по странам |
| `language` | TargetingMap | да | Таргетинг по языкам |
| `device_type` | TargetingMap | да | Таргетинг по типу устройства |
| `os` | TargetingMap | да | Таргетинг по ОС |
| `browser` | TargetingMap | да | Таргетинг по браузерам |
| `site_id` | TargetingMap | да | Таргетинг по сайтам/площадкам |
| `ip` | TargetingMap | да | Таргетинг по IP |

#### `ApiCreative` – креатив (рекламное объявление)

`ApiCreative` — это объединение четырёх вариантов. У всех есть **общие поля**:

| Поле | Тип | Описание |
| ---- | --- | -------- |
| `id` | string | ID креатива |
| `campaign_id` | string | ID родительской кампании |
| `creative_name` | string | Название креатива |
| `link` | string | URL перехода при клике |
| `trackers_macros` | `Record<string, 0 \| 1>` | Какие макросы трекеров включены |

**Дополнительные поля по типу креатива:**

| Тип (`format_type` кампании) | Дополнительные поля | Примечание |
| ---------------------------- | ------------------- | ---------- |
| `popunder` | нет | — |
| `banner` | `w: number`<br>`h: number`<br>`name?: string`<br>`presigned_s3_url?: string` | `w`/`h` – размеры; `name` – имя файла; `presigned_s3_url` – временная ссылка на изображение (только в ответе) |
| `native` | `title: string`<br>`description: string`<br>`name?: string`<br>`presigned_s3_url?: string` | заголовок, описание |
| `push` | `title: string`<br>`description: string`<br>`name?: string`<br>`presigned_s3_url?: string` | заголовок, описание |

> **Важно**: в ответе бэк возвращает `presigned_s3_url` (GET-ссылку на файл в S3). При создании/обновлении клиент передаёт файл через `multipart/form-data`, а также может передать `name` (имя файла).

#### `ApiUserTransaction` – пополнение баланса

| Поле | Тип | Описание |
| ---- | --- | -------- |
| `id` | string | ID транзакции в системе |
| `user_id` | string | ID пользователя |
| `transaction_time` | string | Дата/время создания (ISO) |
| `transaction_id` | string | Внешний ID транзакции (например, от платёжной системы) |
| `payment_method` | string | Способ оплаты |
| `bonus_amount` | number | Сумма бонуса в процентах |
| `promocode_id` | string \| null | ID применённого промокода |
| `transaction_hash` | string \| null | Хэш транзакции |
| `deposit_amount` | number | Сумма депозита |
| `total_balance_increase` | number | Общее увеличение баланса (депозит + бонус) |
| `status` | TopupStatus | Статус пополнения |
| `currency` | string | Валюта |
| `created_at` | string | Дата создания записи |
| `updated_at` | string | Дата последнего обновления |

#### `ApiPromocode` – промокод

| Поле | Тип | Описание |
| ---- | --- | -------- |
| `id` | string | ID промокода |
| `promocode_text` | string | Текст кода |
| `bonus_percent` | number | Процент бонуса |
| `usage_count` | number | Сколько раз использован |
| `usage_limit` | number \| null | Лимит использований |
| `valid_from` | string \| null | Дата начала действия |
| `valid_to` | string \| null | Дата окончания действия |

#### `ApiNotification` – уведомление

| Поле | Тип | Описание |
| ---- | --- | -------- |
| `id` | string | ID уведомления |
| `user_id` | string | ID пользователя |
| `transaction_id` | string \| null | ID связанной транзакции |
| `campaign_id` | string \| null | ID связанной кампании |
| `deposit_amount` | number \| null | Сумма депозита (если применимо) |
| `status` | NotificationStatus | Статус (`active` / `inactive`) |
| `text` | string | Текст уведомления |
| `type` | NotificationType | Тип уведомления |

---

### Статистические типы

#### `StatsQueryRequest` – запрос статистики

| Поле | Тип | Описание |
| ---- | --- | -------- |
| `from` | string | Начальная дата `YYYY-MM-DD` |
| `to` | string | Конечная дата `YYYY-MM-DD` |
| `campaign_ids` | string[] | (опционально) Массив ID кампаний |
| `group_by` | StatsGroupBy[] | Список полей для группировки |
| `filters` | `Partial<Record<StatsGroupBy, string[]>>` | Фильтры: ключ – поле группировки, значение – массив допустимых значений |

#### `StatsRow` – строка статистики

Динамический объект: содержит поля, указанные в `group_by`, плюс следующие метрики:

| Поле | Тип |
| ---- | --- |
| `impressions` | number |
| `clicks` | number |
| `spent` | number |
| `ctr` | number |

#### `StatsQueryResponse` – ответ статистики

```ts
{
  rows: StatsRow[];
  totals: { impressions: number; clicks: number; spent: number; ctr: number };
}
```

#### `StatsSummary` – краткая сводка

```ts
{
  impressions: number;
  clicks: number;
  spent: number;
  ctr: number;
}
```

---

### Auth-типы

#### `AuthTokens`

```ts
{ access_token: string; refresh_token: string }
```

#### `AuthResponse`

```ts
{ access_token: string; refresh_token: string; user: ApiUser }
```

---

## Эндпоинты API

### 🔐 Auth

| Метод | Путь | Описание | Тело запроса | Ответ | Авторизация |
| ----- | ---- | -------- | ------------ | ----- | ----------- |
| POST | `/api/auth/signup` | Регистрация | `{ email: string, password: string, full_name?: string, manager_telegram: string }` | `AuthResponse` | ❌ |
| POST | `/api/auth/login` | Вход | `{ email: string, password: string }` | `AuthResponse` | ❌ |
| POST | `/api/auth/refresh` | Обновить токены | `{ refresh_token: string }` | `AuthTokens` | ❌ |
| POST | `/api/auth/logout` | Выход | – | – | ✅ |
| GET | `/api/auth/session` | Получить сессию | – | `{ user_id: string, email: string, full_name: string }` или `null` | ✅ |
| POST | `/api/auth/password` | Сменить пароль | `{ new_password: string }` | – | ✅ |

### 👤 Profile

| Метод | Путь | Описание | Тело запроса | Ответ | Авторизация |
| ----- | ---- | -------- | ------------ | ----- | ----------- |
| GET | `/api/profile` | Получить профиль | – | `ApiUser` | ✅ |
| PATCH | `/api/profile` | Обновить профиль | `Partial<ApiUser>` (любые поля) | `ApiUser` | ✅ |

### 📢 Campaigns

| Метод | Путь | Описание | Тело запроса | Ответ | Авторизация |
| ----- | ---- | -------- | ------------ | ----- | ----------- |
| GET | `/api/campaigns` | Список кампаний | – | `{ items: ApiCampaign[], total: number }` | ✅ |
| GET | `/api/campaigns/{id}` | Получить одну кампанию | – | `ApiCampaign` | ✅ |
| POST | `/api/campaigns` | Создать кампанию | `ApiCampaign` без полей `campaing_id`, `user_id`, `cum_done_dollars` | `ApiCampaign` | ✅ |
| PATCH | `/api/campaigns/{id}` | Обновить кампанию | `Partial<ApiCampaign>` | `ApiCampaign` | ✅ |
| DELETE | `/api/campaigns/{id}` | Удалить кампанию | – | – | ✅ |

### 🖼️ Creatives

**Особенности**:
- `POST` и `PATCH` используют `multipart/form-data`.
- Поля, кроме `file`/`filename`, передаются как строки; объекты (`trackers_macros`) нужно JSON-сериализовать.
- Тип креатива определяется наличием специфичных полей:  
  `w`/`h` → баннер; `title`/`description` → native/push; никаких → popunder.

| Метод | Путь | Описание | Тело запроса | Ответ | Авторизация |
| ----- | ---- | -------- | ------------ | ----- | ----------- |
| GET | `/api/campaigns/{campaignId}/creatives` | Список креативов кампании | – | `ApiCreative[]` | ✅ |
| POST | `/api/campaigns/{campaignId}/creatives` | Создать креатив | `multipart/form-data`:<br> - `creative_name`<br> - `link`<br> - `trackers_macros` (JSON-строка)<br> - для баннера: `w`, `h`<br> - для native/push: `title`, `description`<br> - опционально: `file` (бинарный), `filename` | `ApiCreative` | ✅ |
| PATCH | `/api/creatives/{id}` | Обновить креатив | `multipart/form-data`:<br> любые поля из `ApiCreative` (как строки, кроме `trackers_macros` – JSON) + опционально `file`, `filename` | `ApiCreative` | ✅ |
| DELETE | `/api/creatives/{id}` | Удалить креатив | – | – | ✅ |

### 💰 Topups (пополнения)

| Метод | Путь | Описание | Тело запроса | Ответ | Авторизация |
| ----- | ---- | -------- | ------------ | ----- | ----------- |
| GET | `/api/topups` | Список пополнений | – | `{ items: ApiUserTransaction[], total: number }` | ✅ |
| POST | `/api/topups` | Создать пополнение | `{ payment_method: string, deposit_amount: number, currency: string, promocode_id?: string \| null, bonus_amount?: number, transaction_hash?: string \| null, status?: TopupStatus }` | `ApiUserTransaction` | ✅ |
| PATCH | `/api/topups/{id}` | Обновить пополнение | `Partial<ApiUserTransaction>` | `ApiUserTransaction` | ✅ |
| POST | `/api/topups/{id}/cancel` | Отменить пополнение | – | `ApiUserTransaction` (статус `cancelled`) | ✅ |

### 🎟️ Promocode

| Метод | Путь | Описание | Тело запроса | Ответ | Авторизация |
| ----- | ---- | -------- | ------------ | ----- | ----------- |
| GET | `/api/promocodes/{code}` | Проверить промокод | – | `ApiPromocode` | ✅ |

### 🔔 Notifications

| Метод | Путь | Описание | Тело запроса / параметры | Ответ | Авторизация |
| ----- | ---- | -------- | ------------------------ | ----- | ----------- |
| GET | `/api/notifications` | Список активных уведомлений | Query: `status=active` | `ApiNotification[]` | ✅ |
| POST | `/api/notifications` | Создать уведомление | `{ transaction_id?: string \| null, campaign_id?: string \| null, deposit_amount?: number \| null, text: string, type: NotificationType }` | `ApiNotification` | ✅ |
| PATCH | `/api/notifications/{id}` | Обновить уведомление | `Partial<ApiNotification>` | `ApiNotification` | ✅ |

### 📊 Stats (ClickHouse)

| Метод | Путь | Описание | Тело запроса | Ответ | Авторизация |
| ----- | ---- | -------- | ------------ | ----- | ----------- |
| POST | `/api/stats/query` | Детальная статистика с группировками | `StatsQueryRequest` | `StatsQueryResponse` | ✅ |
| GET | `/api/stats/campaign/{id}/summary` | Сводка по одной кампании | – | `StatsSummary` | ✅ |
| GET | `/api/stats/overview` | Общая сводка по всем | – | `StatsSummary` | ✅ |

---

## Примечания

1. **Идентификатор кампании**: во всём API используется поле `campaing_id` (с «a» после «mp»). Это опечатка, но она зафиксирована в контракте.
2. **Тип креатива**: дискриминатора нет – тип определяется по наличию полей (`w`,`h` или `title`,`description`). Бэкенд должен корректно обрабатывать объединение.
3. **Загрузка файлов**: для креативов используется схема:
   - Фронт сначала получает `presigned_s3_url` (но в текущем контракте этого эндпоинта нет – вместо этого файл передаётся сразу в `createCreative` через `multipart/form-data`, и бэк сам сохраняет в S3 и возвращает `presigned_s3_url` в ответе).
4. **Пагинация**: в текущем контракте пагинация присутствует только в `listCampaigns` и `listTopups` (через `total` в ответе). Параметры `offset/limit` не описаны.
5. **Статусы ошибок**: не специфицированы. Фронт ожидает HTTP 4xx/5xx с JSON-объектом `{ error: { message, code?, fields? } }`.

---

Данная документация полностью отражает контракт, используемый в `mockProvider` и ожидаемый `httpProvider`. При реализации бэкенда следует строго придерживаться описанных полей и форматов.
