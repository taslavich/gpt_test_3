import { z } from "zod";
import { ApiError } from "./http";
const money = z
  .string()
  .max(100)
  .regex(/^-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?$/)
  .refine(
    (s) => !/[eE]/.test(s) || Math.abs(Number(s.split(/[eE]/)[1])) <= 100,
  );
const count = z.number().finite().nonnegative();
const status = z.enum([
  "agg_stats_pending",
  "agg_stats_ready",
  "agg_stats_stale",
  "agg_stats_unavailable",
]);
const method = z.enum(["usdt_trc20", "usdt_erc20"]);
const page = {
  limit: z.number().int().min(1).max(500),
  offset: z.number().int().min(0).max(1000000),
};
const metrics = z
  .object({
    requests: count,
    errors: count,
    fills: count,
    no_fills: count,
    impressions: count,
    clicks: count,
    ctr: count,
    revenue_usd: money,
  })
  .passthrough();
const row = metrics
  .extend({
    bucket: z.string(),
    site_id: z.string(),
    zone_id: z.string(),
    ad_code_id: z.string(),
    hostname: z.string(),
    format: z.string().optional(),
    last_request_at: z.string().nullable(),
  })
  .passthrough();
const codeBase = {
  ad_code_id: z.string().min(1),
  user_id: z.string().min(1),
  short_code: z.string().min(1),
  name: z.string(),
  traffic_type: z.enum(["adult", "mainstream"]),
  device: z.enum(["all", "desktop", "mobile"]),
  is_active: z.boolean(),
  archived: z.boolean(),
  version: z.number().int().positive(),
  synced_version: z.number().int().nonnegative(),
  created_at: z.string(),
};
const code = z.discriminatedUnion("format", [
  z.object({
    ...codeBase,
    format: z.literal("popunder"),
    trigger_click: z.union([z.literal(1), z.literal(2)]),
    frequency_minutes: z.number().min(0.1).max(10080),
    max_per_session: z.number().int().min(1).max(100000).nullable(),
  }).passthrough(),
  z.object({
    ...codeBase,
    format: z.literal("direct_link"),
    direct_link_url: z.string().nullish(),
  }).passthrough(),
]);
const balance = z
  .object({
    cumulative_earned: money,
    balance: money,
    reserved: money,
    paid_out: money,
    available_for_payout: money,
    revenue_status: status,
  })
  .passthrough();
const payout = z
  .object({
    id: z.string().min(1),
    amount: money,
    method,
    wallet: z.string(),
    status: z.enum(["pending", "paid", "canceled"]),
    created_at: z.string(),
    transfer_reference: z.string(),
  })
  .passthrough();
const report = z
  .object({
    ...page,
    rows: z.array(row),
    totals: metrics,
    effective_from: z.string(),
    effective_to: z.string(),
    time_resolution: z.literal("1h_utc"),
    revenue_status: status,
  })
  .passthrough();
const inventory = z
  .object({
    ...page,
    rows: z.array(row),
    metric_period: z.literal("last_6_months"),
    metric_period_scope: z.literal("impressions_clicks_ctr"),
    request_metric_period: z.literal("last_7_days"),
    request_metric_source: z.literal("publisher_zone_hourly"),
    advertising_metric_source: z.literal("agg_stats"),
    revenue_period: z.literal("lifetime"),
  })
  .passthrough();
const settings = z.object({
  codeAlerts: z.boolean(),
  payoutMethod: method,
  wallet: z.string(),
});
const zoneDiagnostics = z.object({
 codes_without_zones: count,
 undiscovered_codes: z.array(z.object({ad_code_id:z.string(),status:z.literal("no_zones_detected")})),
  ...page,
  rows: z.array(
    z.object({
      ad_code_id: z.string(),
      zone_id: z.string(),
      site_id: z.string(),
      hostname: z.string(),
      status: z.enum(["active", "problem"]),
      requests_200: count,
      requests_204: count,
      requests_4xx: count,
      requests_5xx: count,
      last_request_at: z.string().nullable(),
    }),
  ),
  active_zones: count,
  problem_zones: count,
  window_from: z.string(),
  window_to: z.string(),
  refreshed_at: z.string(),
});
const diagnostics = z.object({
  ad_code: code,
  runtime_ready: z.boolean(),
  zones: zoneDiagnostics,
});
const sync = z.object({
  backlog: count,
  state: z.string(),
  ready: z.boolean(),
  financial_source_configured: z.boolean(),
  revenue_status: status,
});
export function validatePublisherResponse<T>(
  path: string,
  verb: string,
  value: T,
): T {
  const schema =
    path === "/ad-codes" && verb === "GET"
      ? z.object({
          ...page,
          items: z.array(code),
          public_script_url: z.string().min(1),
          supported_formats: z.array(z.string()).optional(),
        })
      : path.startsWith("/ad-codes") && verb === "DELETE"
        ? z.object({ archived: z.literal(true) })
        : path === "/zone-diagnostics"
          ? zoneDiagnostics
          : path.endsWith("/diagnostics")
            ? diagnostics
            : path.startsWith("/ad-codes")
              ? code
              : path === "/sites" || path === "/zones"
                ? inventory
                : path === "/overview"
                  ? z.object({
                      report,
                      balance,
                      from: z.string(),
                      to: z.string(),
                    })
                  : path === "/stats/query"
                    ? report
                    : path === "/balance"
                      ? balance
                      : path === "/payouts" && verb === "GET"
                        ? z.object({ ...page, items: z.array(payout) })
                        : path === "/payouts"
                          ? payout
                          : path === "/settings"
                            ? settings
                            : path === "/sync-health"
                              ? sync
                              : null;
  if (schema) {
    const result = schema.safeParse(value);
    if (!result.success)
      throw new ApiError(
        200,
        "Invalid publisher API response: " +
          result.error.issues.map((issue) => issue.path.join(".")).join(", "),
        "PUBLISHER_CONTRACT",
      );
  }
  return value;
}
