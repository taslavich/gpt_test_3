import { http } from "./http";
import type {
  ApiUser, ApiCampaign, ApiCreative, ApiUserTransaction, ApiPromocode,
  ApiNotification, StatsQueryRequest, StatsQueryResponse, StatsSummary,
  AuthResponse, AuthTokens,
} from "./types";
import type { ApiProvider } from "./mockProvider";

// HTTP implementation. Hits the real backend described in API_CONTRACT.md.
export const httpProvider: ApiProvider = {
  // auth
  signup: (body) => http<AuthResponse>("/api/auth/signup", { method: "POST", body, auth: false }),
  login:  (body) => http<AuthResponse>("/api/auth/login",  { method: "POST", body, auth: false }),
  refresh:(body) => http<AuthTokens>  ("/api/auth/refresh",{ method: "POST", body, auth: false }),
  logout: ()     => http<void>        ("/api/auth/logout", { method: "POST" }),
  getSession:    () => http<{ user_id: string; email: string; full_name: string } | null>("/api/auth/session"),
  changePassword:(body) => http<void>("/api/auth/password", { method: "POST", body }),

  // profile
  getProfile:   ()     => http<ApiUser>("/api/profile"),
  patchProfile: (p)    => http<ApiUser>("/api/profile", { method: "PATCH", body: p }),

  // campaigns
  listCampaigns:   ()      => http<{ items: ApiCampaign[]; total: number }>("/api/campaigns"),
  getCampaign:     (id)    => http<ApiCampaign>(`/api/campaigns/${id}`),
  createCampaign:  (body)  => http<ApiCampaign>("/api/campaigns", { method: "POST", body }),
  patchCampaign:   (id,p)  => http<ApiCampaign>(`/api/campaigns/${id}`, { method: "PATCH", body: p }),
  deleteCampaign:  (id)    => http<void>(`/api/campaigns/${id}`, { method: "DELETE" }),

  // creatives
  listCreatives:   (cid)         => http<ApiCreative[]>(`/api/campaigns/${cid}/creatives`),
  createCreative:  (cid, body)   => http<ApiCreative>(`/api/campaigns/${cid}/creatives`, { method: "POST", body }),
  patchCreative:   (id,  p)      => http<ApiCreative>(`/api/creatives/${id}`, { method: "PATCH", body: p }),
  deleteCreative:  (id)          => http<void>(`/api/creatives/${id}`, { method: "DELETE" }),
  getUploadUrl:    (body)        => http<{ upload_url: string; s3_file_path: string; expires_in: number }>(
    "/api/creatives/upload-url", { method: "POST", body }),

  // topups
  listTopups:   ()        => http<{ items: ApiUserTransaction[]; total: number }>("/api/topups"),
  createTopup:  (body)    => http<ApiUserTransaction>("/api/topups", { method: "POST", body }),
  patchTopup:   (id, p)   => http<ApiUserTransaction>(`/api/topups/${id}`, { method: "PATCH", body: p }),
  cancelTopup:  (id)      => http<ApiUserTransaction>(`/api/topups/${id}/cancel`, { method: "POST" }),

  // promo
  getPromocode: (code)    => http<ApiPromocode>(`/api/promocodes/${encodeURIComponent(code)}`),

  // notifications
  listNotifications:   ()        => http<ApiNotification[]>("/api/notifications", { query: { status: "active" } }),
  createNotification:  (body)    => http<ApiNotification>("/api/notifications", { method: "POST", body }),
  patchNotification:   (id, p)   => http<ApiNotification>(`/api/notifications/${id}`, { method: "PATCH", body: p }),

  // ClickHouse stats
  statsQuery:           (req)    => http<StatsQueryResponse>("/api/stats/query", { method: "POST", body: req }),
  statsCampaignSummary: (id)     => http<StatsSummary>(`/api/stats/campaign/${id}/summary`),
  statsOverview:        ()       => http<StatsSummary>("/api/stats/overview"),
};
