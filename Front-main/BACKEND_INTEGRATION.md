# Backend integration guide

By default frontend uses local stubs (`VITE_USE_API_STUBS=true`).

## 1) Main API file
- `src/lib/api.ts`

This is the single place where auth/profile/campaign/billing calls are implemented now.
To connect a real backend:
1. Set `VITE_USE_API_STUBS=false` in `.env`.
2. Replace stub methods with real `fetch`/axios calls.
3. Keep response contracts compatible with current return types (`ApiUser`, `ProfileDto`, `CampaignDto`, `TopupRequestDto`).

## 2) Frontend places already wired to API layer
- `src/contexts/AuthContext.tsx` — login/register/logout/session.
- `src/contexts/ProfileContext.tsx` — profile read/update.
- `src/contexts/CampaignContext.tsx` — campaigns CRUD + creatives payload.
- `src/pages/DashboardBalance.tsx` — top-up requests + promo validation.
- `src/pages/DashboardSettings.tsx` — password change.

## 3) Suggested backend endpoints
- `POST /auth/register`
- `POST /auth/login`
- `POST /auth/logout`
- `GET /auth/me`
- `POST /auth/change-password`
- `GET /profiles/me`
- `PATCH /profiles/me`
- `GET /campaigns`
- `POST /campaigns`
- `PATCH /campaigns/:id`
- `DELETE /campaigns/:id`
- `GET /billing/topups`
- `POST /billing/topups`
- `POST /billing/promo/validate`

## 4) Statistics
`src/pages/DashboardStatistics.tsx` currently builds demo data in-browser.
If you move to ClickHouse through backend, replace that page data source with your API endpoint (for example `GET /stats`).
