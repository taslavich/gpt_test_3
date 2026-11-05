import { publisherDisplayMetrics } from "@/lib/publisherData";
import { http, ApiError } from "./http";
import { validatePublisherResponse } from "./publisherContract";

export type PublisherTraffic = "adult" | "mainstream";
export type PublisherDevice = "all" | "desktop" | "mobile";
export type RevenueStatus =
  | "agg_stats_pending"
  | "agg_stats_ready"
  | "agg_stats_stale"
  | "agg_stats_unavailable";
export type PayoutMethod = "usdt_trc20" | "usdt_erc20";
export type PublisherCodeFormat = "popunder" | "direct_link";
interface PublisherCodeBase {
  ad_code_id: string;
  user_id: string;
  short_code: string;
  name: string;
  traffic_type: PublisherTraffic;
  device: PublisherDevice;
  is_active: boolean;
  archived: boolean;
  version: number;
  synced_version: number;
  created_at: string;
}
export interface PublisherPopunderCode extends PublisherCodeBase {
  format: "popunder";
  trigger_click: 1 | 2;
  frequency_minutes: number;
  max_per_session: number | null;
}
export interface PublisherDirectLinkCode extends PublisherCodeBase {
  format: "direct_link";
  direct_link_url?: string | null;
}
export type PublisherCodeDTO = PublisherPopunderCode | PublisherDirectLinkCode;
type CreateCodeFields = Pick<PublisherCodeBase, "name" | "traffic_type" | "device">;
export type CreatePopunderCode = CreateCodeFields &
  Pick<PublisherPopunderCode, "format" | "trigger_click" | "frequency_minutes" | "max_per_session">;
export type CreatePublisherCode =
  | CreatePopunderCode
  | (CreateCodeFields & { format: "direct_link" });
export type PatchPublisherCode = Partial<Omit<CreatePopunderCode, "format">> &
  { version: number; is_active?: boolean };
export interface PublisherPage {
  limit: number;
  offset: number;
}
export interface PublisherCodes extends PublisherPage {
  items: PublisherCodeDTO[];
  public_script_url: string;
  supported_formats?: string[];
}
export interface PublisherMetrics {
  requests: number;
  errors: number;
  fills: number;
  no_fills: number;
  impressions: number;
  clicks: number;
  ctr: number;
  revenue_usd: string;
}
export interface PublisherRow extends PublisherMetrics {
  format?: string;
  bucket: string;
  site_id: string;
  zone_id: string;
  ad_code_id: string;
  hostname: string;
  last_request_at: string | null;
  first_request_at?: string;
  metrics_status?: string;
}
export type PublisherGroup =
  | "format"
  | "date"
  | "hour"
  | "country"
  | "device"
  | "os"
  | "browser"
  | "site"
  | "zone"
  | "ad_code";
export interface PublisherStatsRequest extends Partial<PublisherPage> {
  from: string;
  to: string;
  group_by: PublisherGroup;
  timezone?: string;
  site_id?: string;
  zone_id?: string;
  ad_code_id?: string;
  countries?: string[];
  devices?: string[];
  oses?: string[];
  browsers?: string[];
}
export interface PublisherReport extends PublisherPage {
  rows: PublisherRow[];
  totals: PublisherMetrics;
  effective_from: string;
  effective_to: string;
  time_resolution: "1h_utc";
  revenue_status: RevenueStatus;
}
export interface PublisherInventoryReport extends PublisherPage {
  rows: PublisherRow[];
  /** Legacy alias, scoped only to impressions/clicks/CTR. */
  metric_period: "last_6_months";
  metric_period_scope: "impressions_clicks_ctr";
  request_metric_period: "last_7_days";
  request_metric_source: "publisher_zone_hourly";
  advertising_metric_source: "agg_stats";
  revenue_period: "lifetime";
}
export interface PublisherBalance {
  cumulative_earned: string;
  balance: string;
  reserved: string;
  paid_out: string;
  available_for_payout: string;
  revenue_status: RevenueStatus;
}
export interface PublisherPayout {
  id: string;
  amount: string;
  method: PayoutMethod;
  wallet: string;
  status: "pending" | "paid" | "canceled";
  created_at: string;
  transfer_reference: string;
}
export interface CreatePublisherPayout {
  amount: string;
  method: PayoutMethod;
  wallet: string;
  request_id: string;
}
export interface PublisherSettings {
  codeAlerts: boolean;
  payoutMethod: PayoutMethod;
  wallet: string;
}
export interface PublisherZoneDiagnostic {
  ad_code_id: string;
  zone_id: string;
  site_id: string;
  hostname: string;
  status: "active" | "problem";
  requests_200: number;
  requests_204: number;
  requests_4xx: number;
  requests_5xx: number;
  last_request_at: string | null;
}
export interface PublisherZoneDiagnostics extends PublisherPage {
  codes_without_zones: number;
  undiscovered_codes: Array<{ ad_code_id: string; status: "no_zones_detected" }>;
  rows: PublisherZoneDiagnostic[];
  active_zones: number;
  problem_zones: number;
  window_from: string;
  window_to: string;
  refreshed_at: string;
}
export interface PublisherDiagnosticsDTO {
  ad_code: PublisherCodeDTO;
  runtime_ready: boolean;
  zones: PublisherZoneDiagnostics;
}
export interface PublisherSyncHealth {
  backlog: number;
  ready: boolean;
  state: string;
  epoch?: string;
  financial_source_configured: boolean;
  revenue_status: RevenueStatus;
}
export interface PublisherOverview {
  report: PublisherReport;
  balance: PublisherBalance;
  from: string;
  to: string;
}
interface Envelope<T> {
  success: boolean;
  errorMsg: string;
  data: T;
}

// Use the existing authenticated HTTP client, including its shared JWT refresh.
// Publisher never switches to the advertiser mock provider.
async function call<T>(
  path: string,
  method: "GET" | "POST" | "PATCH" | "DELETE" = "GET",
  body?: unknown,
  query?: Record<string, string | number | undefined>,
  signal?: AbortSignal,
): Promise<T> {
  const controller = new AbortController();
  const abort = () => controller.abort();
  signal?.addEventListener("abort", abort, { once: true });
  if (signal?.aborted) abort();
  const timer = setTimeout(abort, 20000);
  try {
    const result = await http<Envelope<T>>("/api/publisher" + path, {
      method,
      body,
      query,
      signal: controller.signal,
    });
    if (!result || result.success !== true || result.data == null) {
      throw new ApiError(
        200,
        result?.errorMsg || "Invalid publisher API envelope",
        "PUBLISHER_CONTRACT",
      );
    }
    const validated = validatePublisherResponse<T>(path, method, result.data);
    // All Publisher views share the same display correction, after contract validation.
    if (path === "/stats/query") {
      const report = validated as PublisherReport;
      return { ...report, rows: report.rows.map(publisherDisplayMetrics),
        totals: publisherDisplayMetrics(report.totals) } as T;
    }
    if (path === "/overview") {
      const overview = validated as PublisherOverview;
      return { ...overview, report: { ...overview.report,
        rows: overview.report.rows.map(publisherDisplayMetrics),
        totals: publisherDisplayMetrics(overview.report.totals) } } as T;
    }
    if (path === "/sites" || path === "/zones") {
      const inventory = validated as PublisherInventoryReport;
      return { ...inventory, rows: inventory.rows.map(publisherDisplayMetrics) } as T;
    }
    return validated;
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
  }
}
const idPath = (id: string) => "/ad-codes/" + encodeURIComponent(id);
export const publisherApi = {
  codes: (page: PublisherPage, signal?: AbortSignal) =>
    call<PublisherCodes>("/ad-codes", "GET", undefined, { ...page }, signal),
  createCode: (body: CreatePublisherCode) =>
    call<PublisherCodeDTO>("/ad-codes", "POST", body),
  code: (id: string, signal?: AbortSignal) =>
    call<PublisherCodeDTO>(idPath(id), "GET", undefined, undefined, signal),
  patchCode: (id: string, body: PatchPublisherCode) =>
    call<PublisherCodeDTO>(idPath(id), "PATCH", body),
  archiveCode: (id: string) =>
    call<{ archived: boolean }>(idPath(id), "DELETE"),
  duplicateCode: (id: string) =>
    call<PublisherCodeDTO>(idPath(id) + "/duplicate", "POST"),
  zoneDiagnostics: (page: PublisherPage, signal?: AbortSignal) =>
    call<PublisherZoneDiagnostics>(
      "/zone-diagnostics",
      "GET",
      undefined,
      { ...page },
      signal,
    ),
  diagnostics: (
    id: string,
    signal?: AbortSignal,
    page: PublisherPage = { limit: 50, offset: 0 },
  ) =>
    call<PublisherDiagnosticsDTO>(
      idPath(id) + "/diagnostics",
      "GET",
      undefined,
      { ...page },
      signal,
    ),
  sites: (page: PublisherPage, signal?: AbortSignal) =>
    call<PublisherInventoryReport>(
      "/sites",
      "GET",
      undefined,
      { ...page },
      signal,
    ),
  zones: (
    params: PublisherPage & { site_id?: string; ad_code_id?: string },
    signal?: AbortSignal,
  ) =>
    call<PublisherInventoryReport>(
      "/zones",
      "GET",
      undefined,
      { ...params },
      signal,
    ),
  overview: (signal?: AbortSignal) =>
    call<PublisherOverview>("/overview", "GET", undefined, undefined, signal),
  stats: (body: PublisherStatsRequest, signal?: AbortSignal) =>
    call<PublisherReport>("/stats/query", "POST", body, undefined, signal),
  balance: (signal?: AbortSignal) =>
    call<PublisherBalance>("/balance", "GET", undefined, undefined, signal),
  payouts: (page: PublisherPage, signal?: AbortSignal) =>
    call<PublisherPage & { items: PublisherPayout[] }>(
      "/payouts",
      "GET",
      undefined,
      { ...page },
      signal,
    ),
  createPayout: (body: CreatePublisherPayout) =>
    call<PublisherPayout>("/payouts", "POST", body),
  settings: (signal?: AbortSignal) =>
    call<PublisherSettings>("/settings", "GET", undefined, undefined, signal),
  patchSettings: (body: PublisherSettings) =>
    call<PublisherSettings>("/settings", "PATCH", body),
  syncHealth: (signal?: AbortSignal) =>
    call<PublisherSyncHealth>(
      "/sync-health",
      "GET",
      undefined,
      undefined,
      signal,
    ),
};
