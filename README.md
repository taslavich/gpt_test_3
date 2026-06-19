# wm-api

Отдельный Go-сервис для SSP/Webmaster статистики.

## Что делает

HTTP ручка `/wm_api/` принимает:

- `feed` — uuid фида из ENV переменных `SSP_*_FEEDS`;
- `group_by` — одно значение из `geo`, `date`, `site`;
- `date_start` — дата начала в формате `YYYY-MM-DD`;
- `date_end` — дата конца в формате `YYYY-MM-DD`.

Сервис конвертирует `feed` uuid в `spp_domain` через ту же map-логику, что и в SSP adapter: строки формата `uuid|domain`, разделенные запятой.

Далее делает запрос в ClickHouse по таблицам:

- `fact_clicks`;
- `fact_impressions`.

И возвращает JSON.

## Запуск

```bash
go mod tidy
go run ./cmd/wm-api
```

По умолчанию сервис читает ENV из файлов:

1. `.env.local`
2. `.env`
3. `wm-api.env`
4. `spp-adapter.env`

Основной файл в этом проекте — `wm-api.env`.

## Пример запроса

```bash
curl "http://localhost:8055/wm_api/?feed=0260c40b-44ad-49bf-a54d-42d6bda5c90f&group_by=geo&date_start=2026-06-01&date_end=2026-06-18"
```

Можно также использовать короткие алиасы:

- `group` вместо `group_by`;
- `start_date` вместо `date_start`;
- `end_date` вместо `date_end`.

## Пример ответа

```json
{
  "feed": "0260c40b-44ad-49bf-a54d-42d6bda5c90f",
  "spp_domain": "mc_moblivion.com",
  "group_by": "geo",
  "date_start": "2026-06-01",
  "date_end": "2026-06-18",
  "rows": [
    {
      "group": "US",
      "impressions": 1000,
      "clicks": 25,
      "cost": 12.34
    }
  ]
}
```

## SQL логика

Группировка:

- `geo` -> поле `geo`;
- `date` -> поле `event_date`;
- `site` -> поле `site_id`.

Стоимость:

- clicks: `POP / 1000 + IPP` по `win_dsp_price`;
- impressions: `BAN/NAT / 1000` по `win_final_price`.

## Healthcheck

```bash
curl -i "http://localhost:8055/health"
```
