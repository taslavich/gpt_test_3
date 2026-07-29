import { ApiError, authenticatedFetch, http } from "./http";
import type {
  ApiUser, ApiCampaign, ApiCreative, ApiCreativeImage, ApiUserTransaction, ApiPromocode,
  ApiNotification, StatsQueryRequest, StatsQueryResponse,
  CalculatorResponse, RecommendBidResponse,
  AuthResponse, AuthTokens, ApiEnvelope,
} from "./types";
import type { RawApiProvider } from "./mockProvider";
import { API_BASE_URL } from "./config";
import { normalizeCreativeUploadFile, sanitizeCreativeFilename } from "@/lib/creativeApi";

/** Only creative image upload uses multipart/form-data. */
function buildCreativeImageForm(file: File, filename?: string): FormData {
  const fd = new FormData();
  const normalizedFile = normalizeCreativeUploadFile(file);
  const safeFilename = sanitizeCreativeFilename(filename || normalizedFile.name);
  fd.append("file", normalizedFile, safeFilename);
  fd.append("filename", safeFilename);
  return fd;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? value as Record<string, unknown>
    : null;
}

async function multipart<T>(url: string, fd: FormData): Promise<ApiEnvelope<T>> {
  const r = await authenticatedFetch(`${API_BASE_URL}${url}`, { method: "POST", body: fd });
  let data: unknown = null;
  const text = await r.text();
  if (text) { try { data = JSON.parse(text); } catch { data = text; } }
  if (!r.ok) {
    const payload = asRecord(data);
    const error = asRecord(payload?.error);
    const message =
      (typeof payload?.errorMsg === "string" && payload.errorMsg)
      || (typeof error?.message === "string" && error.message)
      || (typeof data === "string" && data)
      || `HTTP ${r.status}`;
    const code = typeof error?.code === "string" ? error.code : undefined;
    const fields = asRecord(error?.fields);
    const normalizedFields = fields
      ? Object.fromEntries(
          Object.entries(fields).filter((entry): entry is [string, string] => typeof entry[1] === "string"),
        )
      : undefined;
    throw new ApiError(r.status, message, code, normalizedFields);
  }
  // Backend may return an envelope already, or a bare payload.
  if (data && typeof data === "object" && "success" in data) {
    return { errorMsg: "", ...(data as ApiEnvelope<T>) };
  }
  return { success: true, errorMsg: "", data: data as T };
}

// HTTP implementation. Every method returns `ApiEnvelope<T>`; `api/index.ts`
// unwraps it (throws on success:false, returns `data` on success:true) so
// callers see the same shape that mocks used to return directly.
export const httpProvider: RawApiProvider = {
  // auth
  signup: (body) => http<ApiEnvelope<AuthResponse>>("/api/auth/signup", { method: "POST", body, auth: false }),
  login:  (body) => http<ApiEnvelope<AuthResponse>>("/api/auth/login",  { method: "POST", body, auth: false }),
  refresh:(body) => http<ApiEnvelope<AuthTokens>>  ("/api/auth/refresh",{ method: "POST", body, auth: false }),
  logout: ()     => http<ApiEnvelope<void>>        ("/api/auth/logout", { method: "POST" }),
  getSession:    () => http<ApiEnvelope<{ user_id: string; email: string; full_name: string } | null>>("/api/auth/session"),
  changePassword:(body) => http<ApiEnvelope<void>>("/api/auth/password", { method: "POST", body }),
  verifyEmail:   (body) => http<ApiEnvelope<void>>("/api/auth/verify",   { method: "PATCH", body, auth: false }),

  // profile
  getProfile:   ()     => http<ApiEnvelope<ApiUser>>("/api/profile"),
  patchProfile: (p)    => http<ApiEnvelope<ApiUser>>("/api/profile", { method: "PATCH", body: p }),

  // campaigns
  listCampaigns:   ()      => http<ApiEnvelope<{ items: ApiCampaign[]; total: number }>>("/api/campaigns"),
  getCampaign:     (id)    => http<ApiEnvelope<ApiCampaign>>(`/api/campaigns/${id}`),
  createCampaign:  (body)  => http<ApiEnvelope<ApiCampaign>>("/api/campaigns", { method: "POST", body }),
  patchCampaign:   (id,p)  => http<ApiEnvelope<ApiCampaign>>(`/api/campaigns/${id}`, { method: "PATCH", body: p }),

  // creatives
  readCreatives:   (cid)         => http<ApiEnvelope<ApiCreative[]>>(`/api/campaigns/${cid}/creatives`),
  uploadCreativeImage: (cid, file, filename) =>
    multipart<ApiCreativeImage>(
      `/api/campaigns/${cid}/creative-images`,
      buildCreativeImageForm(file, filename),
    ),
  createCreative:  (cid, body) =>
    http<ApiEnvelope<ApiCreative>>(`/api/campaigns/${cid}/creatives`, { method: "POST", body }),
  patchCreative:   (id, p) =>
    http<ApiEnvelope<ApiCreative>>(`/api/creatives/${id}`, { method: "PATCH", body: p }),
  deleteCreative:  (id)          => http<ApiEnvelope<void>>(`/api/creatives/${id}`, { method: "DELETE" }),

  // transactions
  listTransactions:   ()        => http<ApiEnvelope<{ items: ApiUserTransaction[]; total: number }>>("/api/transactions"),
  createTransaction:  (body)    => http<ApiEnvelope<ApiUserTransaction>>("/api/transactions", { method: "POST", body }),
  patchTransaction:   (id, p)   => http<ApiEnvelope<ApiUserTransaction>>(`/api/transactions/${id}`, { method: "PATCH", body: p }),
  cancelTransaction:  (id)      => http<ApiEnvelope<ApiUserTransaction>>(`/api/transactions/${id}/cancel`, { method: "POST" }),

  // promo
  getPromocode: (code)    => http<ApiEnvelope<ApiPromocode>>(`/api/promocodes/${encodeURIComponent(code)}`),

  // notifications
  listNotifications:   ()        => http<ApiEnvelope<ApiNotification[]>>("/api/notifications", { query: { status: "active" } }),
  createNotification:  (body)    => http<ApiEnvelope<ApiNotification>>("/api/notifications", { method: "POST", body }),
  patchNotification:   (id, p)   => http<ApiEnvelope<ApiNotification>>(`/api/notifications/${id}`, { method: "PATCH", body: p }),

  // ClickHouse stats — single universal endpoint for Overview / Campaigns / Statistics.
  statsQuery: (req) => http<ApiEnvelope<StatsQueryResponse>>("/api/stats/query", { method: "POST", body: req }),

  // Historical potential traffic for the latest fully closed day.
  calculator: (req) => http<ApiEnvelope<CalculatorResponse>>("/api/calculator", { method: "POST", body: req }),

  // Average historical bid for the selected segment and latest fully closed day.
  recommendBid: (req) => http<ApiEnvelope<RecommendBidResponse>>("/api/recommend_bid", { method: "POST", body: req }),
};
