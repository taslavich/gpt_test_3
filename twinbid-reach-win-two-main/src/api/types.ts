// Shared API types. Mirror the contract in API_CONTRACT.md.
// These are the *transport* shapes (snake_case) the backend will return.
// UI contexts may map them into their own camelCase view models.

export type CampaignStatus = "active" | "paused" | "draft" | "completed" | "moderation" | "no_budget" | "waiting" | "deleted";
export type PricingModel = "cpm" | "cpc";
export type CampaignTypeModel = 1 | 2;
export type TrafficType = "mainstream" | "adult" | "mixed";
export type FormatType = "banner" | "popunder" | "native" | "push";
export type TopupStatus = "draft" | "pending" | "approved" | "rejected" | "cancelled";
export type PaymentChannel = "static_wallet" | "passimpay_invoice" | "cryptomus_invoice";
export type PaymentProvider = "passimpay" | "cryptomus";
export type NotificationType = "incomplete_topup" | "low_balance" | "campaign_status" | "other";
export type NotificationStatus = "active" | "inactive";

/** HASHMAP targeting: { value: 1 (white) | 0 (black) }. Empty = no targeting. */
export type TargetingMap = Record<string, 0 | 1>;

/** Schedule interval like ["mon,1", "thu,2"]. */
export type ScheduleInterval = [string, string];

export interface ApiUser {
  login: string;
  mail: string;
  name: string;
  telegram: string | null;
  manager_telegram: string;
  balance: number;
  timezone: string;
  email_notifications: boolean;
  campaign_status_notifications: boolean;
  low_balance_notifications: boolean;
  balance_treshold: number;
  utm_source?: string | null;
  /** User's own permanent public partner ID, created during signup. */
  partner_id: string;
  /** Partner ID of the user who referred this user. */
  partner?: string | null;
}

export interface SignupRequest {
  email: string;
  password: string;
  full_name?: string;
  telegram: string;
  manager_telegram: string;
  utm_source?: string;
  /** User's own permanent public partner ID. */
  partner_id: string;
  /** Partner ID captured from `?partner=...`, when present. */
  partner?: string | null;
}

/** Aggregated TwinBid Partners dashboard data for the authenticated user. */
export interface PartnerStatsResponse {
  /** Public affiliate code used in `?partner=...` links. */
  partner: string;
  advertisers: number;
  /** Total amount spent by all advertisers assigned to this partner. */
  turnover: number;
  withdrawn: number;
}

export interface ApiCampaign {
  campaign_id: string;
  user_id: string;
  campaign_name: string;
  format_type: FormatType;
  brand_name?: string | null;
  h?: number | null;
  w?: number | null;
  status: CampaignStatus;
  traffic_type: TrafficType;
  vertical: Record<string, 0 | 1>;
  pricing_model: PricingModel;
  /** 1 = standard CPM/CPC bidding, 2 = TwinBid CPM optimization. */
  type_model?: CampaignTypeModel;
  base_price: number;
  evenness_by_slot_mode: boolean;
  goal_total_dollars: number;
  /** filled by backend only */
  cum_done_dollars: number;
  start_ts: string | null;
  end_ts: string | null;
  active_intervals: ScheduleInterval[];
  country: TargetingMap;
  language: TargetingMap;
  device_type: TargetingMap;
  os: TargetingMap;
  browser: TargetingMap;
  site_id: TargetingMap;
  ip: TargetingMap;
  /** When true, VPN-classified traffic is blocked for the campaign. */
  block_vpn?: boolean;
  quality_type: "usual" | "high" | "ultra";
  /** Fixed reward per conversion (USD). Used in statistics when the postback does not deliver a payout value. */
  payout?: number | null;
}

export type BannerType = "img" | "iframe";

export interface ApiCreativeImage {
  image_id: string;
  campaign_id: string;
  creative_id: string | null;
  image_url: string;
  filename: string;
  mime_type: string;
  file_format: string;
  size_bytes: number;
  created_at: string;
  updated_at: string;
}

export interface ApiCreative {
  id: string;
  campaign_id: string;
  creative_name: string;
  adm: string;
  banner_type?: BannerType | null;
  image_id?: string | null;
  image_url?: string | null;
  image_name?: string | null;
  /** MIME type returned for the attached permanent media asset. */
  mime_type?: string | null;
  /** Macro token -> query parameter name. Boolean values are accepted only for legacy responses. */
  trackers_macros?: Record<string, string | boolean>;
  w?: number | null;
  h?: number | null;
  title?: string | null;
  description?: string | null;
}

/** JSON body accepted by creative POST/PATCH. File bytes are never included. */
export interface ApiCreativeWrite {
  creative_name: string;
  adm: string;
  banner_type?: BannerType | null;
  image_id?: string | null;
  /** Macro token -> query parameter name, e.g. { click_id: "subid" }. */
  trackers_macros?: Record<string, string>;
  w?: number | null;
  h?: number | null;
  title?: string | null;
  description?: string | null;
}

export interface ApiUserTransaction {
  id: string;
  user_id: string;
  transaction_time?: string;
  transaction_id: string;
  payment_channel?: PaymentChannel;
  payment_method?: string | null;
  bonus_amount?: number | null;
  promocode_id: string | null;
  transaction_hash?: string | null;
  deposit_amount: number;
  total_balance_increase?: number | null;
  status: TopupStatus;
  currency: string;
  payment_url?: string | null;
  provider_status?: string | null;
  provider_payment_id?: string | null;
  provider_transaction_id?: string | null;
  amount_paid?: number | null;
  amount_credited?: number | null;
  fee_service?: number | null;
  fee_network?: number | null;
  credited_at?: string | null;
  created_at?: string;
  updated_at?: string;
}

interface ApiCreateTransactionCommon {
  deposit_amount: number;
  currency: string;
  promocode_id?: string | null;
}

/**
 * Invoice transactions are selected by `provider`; the backend derives their
 * response `payment_channel`. Static-wallet payments explicitly select their
 * channel and network and never send a provider.
 */
export type ApiCreateTransactionRequest = ApiCreateTransactionCommon & (
  | {
      payment_channel: "static_wallet";
      payment_method: string;
      provider?: never;
    }
  | {
      provider: "passimpay";
      payment_channel?: never;
      payment_method?: never;
    }
  | {
      provider: "cryptomus";
      payment_channel?: never;
      payment_method?: never;
    }
);

/** The user-facing PATCH is intentionally limited to a static-wallet txhash. */
export interface ApiPatchTransactionRequest {
  transaction_hash: string;
}

export interface ApiPromocode {
  id: string;
  promocode_text: string;
  bonus_percent: number;
  usage_count: number;
  usage_limit: number | null;
  valid_from: string | null;
  valid_to: string | null;
}

export interface ApiNotification {
  id: string;
  user_id: string;
  transaction_id: string | null;
  campaign_id: string | null;
  deposit_amount: number | null;
  status: NotificationStatus;
  text: string;
  type: NotificationType;
}

// ---- ClickHouse statistics ----
/** Allowed values for `group_by` (single value, not array). */
export type StatsGroupBy =
  | "date" | "hour" | "country" | "os" | "browser" | "device_type" | "site_id" | "campaign";

/** Allowed keys inside `filters` — narrower than `group_by`. */
export type StatsFilterBy = "country" | "os" | "browser" | "device_type";

export interface StatsQueryRequest {
  from: string; // YYYY-MM-DD (UTC). For a single day send from === to.
  to: string;
  /** Optional UUID list of campaigns. Empty/omitted = all user campaigns. Multi-select supported. */
  campaign_ids?: string[];
  /** Optional UUID list of creatives. */
  creative_ids?: string[];
  /** Single grouping. The frontend re-issues the request whenever the user picks another group. */
  group_by: StatsGroupBy;
  filters?: Partial<Record<StatsFilterBy, string[]>>;
}

export interface StatsSummary {
  impressions: number;
  clicks: number;
  spent: number;
  ctr: number;
}

/**
 * Map keyed by the bucket value (e.g. country code "DE", campaign UUID, "YYYY-MM-DD"…).
 * Each value is the metrics object for that bucket. `totals` aggregates across all rows
 * with the same WHERE filters but no GROUP BY.
 */
export interface StatsQueryResponse {
  rows: Record<string, StatsSummary>;
  totals: StatsSummary;
}

// ---- Historical traffic calculator --------------------------------------
// The backend uses the latest fully closed day from the ad-request statistics
// table. Bid and pricing model are intentionally absent: this query describes
// traffic that matched the targeting, not traffic won by a particular bid.
export interface TrafficSegmentRequest {
  format_type?: FormatType;
  traffic_type?: TrafficType;
  country?: string[];
  country_mode?: "include" | "exclude";
  language?: string[];
  language_mode?: "include" | "exclude";
  device_type?: string[];
  device_type_mode?: "include" | "exclude";
  os?: string[];
  os_mode?: "include" | "exclude";
  browser?: string[];
  browser_mode?: "include" | "exclude";
  site_id?: string[];
  site_id_mode?: "include" | "exclude";
}

export type CalculatorRequest = TrafficSegmentRequest;

export interface CalculatorResponse {
  /** Historical number of available impressions matching the request. */
  potential_impressions: number;
}

// ---- Historical bid recommendation -------------------------------------
// Uses the same segment filters as the calculator. The backend returns the
// average non-zero winning bid for the latest fully closed day.
export type RecommendBidRequest = TrafficSegmentRequest;

export interface RecommendBidResponse {
  /** Average non-zero bid for the requested segment. */
  average_bid: number;
}

// ---- Auth ----
export interface AuthTokens {
  access_token: string;
  refresh_token: string;
}
export interface AuthResponse extends AuthTokens {
  user: ApiUser;
}

// ---- Standard response envelope ----
// Every backend handler returns this shape. The api layer unwraps it: on
// `success: true` callers receive `data`; on `success: false` an `ApiError`
// is thrown carrying `errorMsg`.
export interface ApiEnvelope<T> {
  success: boolean;
  errorMsg: string;
  data?: T;
}
