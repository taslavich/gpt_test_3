import { createElement, useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { randomUUID } from "node:crypto";
import { vi } from "vitest";
import { PublisherProvider } from "@/contexts/PublisherContext";
import type {
  PublisherCodeDTO,
  PublisherPopunderCode,
  PublisherRow,
  PublisherReport,
  PublisherBalance,
  PublisherSettings,
  PublisherPayout,
} from "@/api/publisher";
export const codeFixture: PublisherPopunderCode = {
  ad_code_id: "11111111-1111-4111-8111-111111111111",
  user_id: "22222222-2222-4222-8222-222222222222",
  short_code: "tb_server_only",
  name: "Server Popunder",
  format: "popunder",
  traffic_type: "adult",
  device: "all",
  is_active: true,
  archived: false,
  trigger_click: 2,
  frequency_minutes: 0.1,
  max_per_session: null,
  version: 7,
  synced_version: 6,
  created_at: "2026-10-01T00:00:00Z",
};
export const rowFixture: PublisherRow = {
  bucket: "server-site-729",
  site_id: "server-site-729",
  zone_id: "server-zone-823",
  ad_code_id: codeFixture.ad_code_id,
  hostname: "news.example.test",
  last_request_at: "2026-10-14T12:00:00Z",
  first_request_at: "2026-10-01T00:00:00Z",
  metrics_status: "mixed_periods",
  requests: 98,
  errors: 3,
  fills: 50,
  no_fills: 45,
  impressions: 40,
  clicks: 1,
  ctr: 2.5,
  revenue_usd: "0.12345678",
};
export const reportFixture: PublisherReport = {
  rows: [rowFixture],
  totals: {
    requests: 9876,
    errors: 90,
    fills: 5000,
    no_fills: 4786,
    impressions: 1000,
    clicks: 8,
    ctr: 0.8,
    revenue_usd: "98765.12345678",
  },
  effective_from: "2026-10-07T18:00:00Z",
  effective_to: "2026-10-14T19:00:00Z",
  time_resolution: "1h_utc",
  limit: 50,
  offset: 0,
  revenue_status: "agg_stats_ready",
};
export const balanceFixture: PublisherBalance = {
  cumulative_earned: "9876543210.12345678",
  balance: "200.12345678",
  reserved: "0",
  paid_out: "40",
  available_for_payout: "200.12345678",
  revenue_status: "agg_stats_ready",
};
export const settingsFixture: PublisherSettings = {
  codeAlerts: true,
  payoutMethod: "usdt_erc20",
  wallet: "0x" + "a".repeat(40),
};
export const payoutFixture: PublisherPayout = {
  id: "33333333-3333-4333-8333-333333333333",
  amount: "50.00000000",
  method: "usdt_erc20",
  wallet: settingsFixture.wallet,
  status: "pending",
  created_at: "2026-10-14T12:00:00Z",
  transfer_reference: "",
};
export const userFixture = {
  id: codeFixture.user_id,
  email: "publisher@example.test",
  partner_id: "",
};
export const profileFixture = {
  id: userFixture.id,
  email: userFixture.email,
  fullName: "Publisher",
  telegram: "",
  timezone: "utc_3",
  balance: 0,
  balanceThreshold: 100,
  notifyCampaignStatus: true,
  notifyLowBalance: true,
  managerTelegram: "",
  partnerId: "",
  partner: null,
};
export const languageFixture = {
  lang: "ru",
  t: (key: string) =>
    (
      ({
        "settings.title": "Настройки",
        "settings.subtitle": "Управление аккаунтом",
        "settings.profile": "Профиль",
        "settings.notifications": "Уведомления",
        "settings.security": "Безопасность",
        "settings.appearance": "Цвет интерфейса",
        "settings.name": "Имя",
        "settings.email": "Email",
        "settings.telegram": "Telegram",
        "settings.timezone": "Часовой пояс",
        "settings.save": "Сохранить",
        "settings.currentPassword": "Текущий пароль",
        "settings.newPassword": "Новый пароль",
        "settings.repeatPassword": "Повторите пароль",
        "settings.changePassword": "Сменить пароль",
      }) as Record<string, string>
    )[key] || key,
};
export const reply = (data: unknown, status = 200, errorMsg = "") =>
  new Response(JSON.stringify({ success: status < 400, errorMsg, data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
export interface CapturedPublisherRequest {
  url: URL;
  method: string;
  body?: Record<string, unknown>;
  headers: Headers;
  signal?: AbortSignal;
}
export function publisherServer() {
  const calls: CapturedPublisherRequest[] = [],
    overrides = new Map<
      string,
      (request: CapturedPublisherRequest) => Response | Promise<Response>
    >();
  let balance = { ...balanceFixture },
    payouts: PublisherPayout[] = [],
    codes: PublisherCodeDTO[] = [{ ...codeFixture }],
    settings = { ...settingsFixture };
  const fetch = vi.fn(
    async (input: RequestInfo | URL, init: RequestInit = {}) => {
      const req: CapturedPublisherRequest = {
        url: new URL(String(input)),
        method: init.method || "GET",
        body: init.body ? JSON.parse(String(init.body)) : undefined,
        headers: new Headers(init.headers),
        signal: init.signal || undefined,
      };
      calls.push(req);
      const override = overrides.get(req.method + " " + req.url.pathname);
      if (override) return override(req);
      const path = req.url.pathname.replace("/api/publisher", "");
      if (req.method === "GET" && path === "/ad-codes")
        return reply({
          items: codes,
          limit: Number(req.url.searchParams.get("limit")),
          offset: Number(req.url.searchParams.get("offset")),
          public_script_url: "https://stable.example.test/sdk.js",
        });
      if (req.method === "POST" && path === "/ad-codes") {
        if (!["adult", "mainstream"].includes(String(req.body?.traffic_type)))
          return reply(null, 400, "traffic_type is required");
        const created = {
          ...codeFixture,
          ...req.body,
          ad_code_id: "44444444-4444-4444-8444-444444444444",
          short_code: "tb_created_by_server",
          version: 1,
        };
        codes = [created as PublisherCodeDTO, ...codes];
        return reply(created);
      }
      if (
        req.method === "GET" &&
        path === "/ad-codes/" + codeFixture.ad_code_id
      )
        return reply(codeFixture);
      if (
        req.method === "PATCH" &&
        path === "/ad-codes/" + codeFixture.ad_code_id
      ) {
        if (req.body?.version !== codeFixture.version)
          return reply(null, 409, "version conflict");
        return reply({
          ...codeFixture,
          ...req.body,
          version: codeFixture.version + 1,
        });
      }
      if (req.method === "DELETE" && path.startsWith("/ad-codes/"))
        return reply({ archived: true });
      if (req.method === "POST" && path.endsWith("/duplicate"))
        return reply({
          ...codeFixture,
          ad_code_id: "55555555-5555-4555-8555-555555555555",
          short_code: "tb_duplicate_server",
        });
      if (path === "/zone-diagnostics") return reply(zoneReportFixture);
      if (path.endsWith("/diagnostics"))
        return reply({
          ad_code: codeFixture,
          zones: zoneReportFixture,
          runtime_ready: true,
        });
      if (path === "/sites" || path === "/zones")
        return reply({
          rows: [rowFixture],
          limit: Number(req.url.searchParams.get("limit")),
          offset: Number(req.url.searchParams.get("offset")),
          metric_period: "last_6_months",
        metric_period_scope: "impressions_clicks_ctr",
        request_metric_period: "last_7_days",
        request_metric_source: "publisher_zone_hourly",
        advertising_metric_source: "agg_stats",
          revenue_period: "lifetime",
        });
      if (path === "/stats/query")
        return reply({
          ...reportFixture,
          rows: [{ ...rowFixture, bucket: req.body?.group_by === "date"
            ? String(req.body?.from).slice(0, 10)
            : req.body?.group_by === "hour" ? String(req.body?.from) : rowFixture.bucket }],
          limit: req.body?.limit || 50,
          offset: req.body?.offset || 0,
        });
      if (path === "/overview")
        return reply({
          report: reportFixture,
          balance,
          from: reportFixture.effective_from,
          to: reportFixture.effective_to,
        });
      if (path === "/balance") return reply(balance);
      if (path === "/settings") {
        if (req.method === "PATCH")
          settings = req.body as unknown as PublisherSettings;
        return reply(settings);
      }
      if (path === "/sync-health")
        return reply({
          backlog: 1,
          state: "ready",
          ready: true,
          epoch: "",
          financial_source_configured: true,
          revenue_status: "agg_stats_ready",
        });
      if (path === "/payouts" && req.method === "POST") {
        payouts = [payoutFixture];
        balance = {
          ...balance,
          reserved: "50",
          available_for_payout: "150.12345678",
        };
        return reply(payoutFixture);
      }
      if (path === "/payouts")
        return reply({ items: payouts, limit: 50, offset: 0 });
      return reply(null, 404, "Unexpected test endpoint " + req.url.pathname);
    },
  );
  vi.stubGlobal("fetch", fetch);
  localStorage.setItem("twinbid_access_token", "test-access-token");
  vi.stubGlobal("crypto", { randomUUID });
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  if (!HTMLElement.prototype.scrollIntoView)
    HTMLElement.prototype.scrollIntoView = vi.fn();
  return {
    calls,
    overrides,
    fetch,
    setBalance: (value: PublisherBalance) => {
      balance = value;
    },
  };
}
export function PublisherTestWrapper({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      }),
  );
  return createElement(
    QueryClientProvider,
    { client },
    createElement(
      MemoryRouter,
      {},
      createElement(PublisherProvider, { children }),
    ),
  );
}

export const zoneReportFixture = {
  codes_without_zones: 0, undiscovered_codes: [],
  rows: [
    {
      ad_code_id: codeFixture.ad_code_id,
      zone_id: rowFixture.zone_id,
      site_id: rowFixture.site_id,
      hostname: rowFixture.hostname,
      status: "active",
      requests_200: 1,
      requests_204: 2,
      requests_4xx: 3,
      requests_5xx: 4,
      last_request_at: rowFixture.last_request_at,
    },
  ],
  active_zones: 701,
  problem_zones: 12,
  limit: 50,
  offset: 0,
  window_from: "2026-10-14T11:00:00Z",
  window_to: "2026-10-14T12:00:00Z",
  refreshed_at: "2026-10-14T11:59:00Z",
};
