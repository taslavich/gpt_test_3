import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { api } from "@/api";
import type {
  ApiCampaign, ApiCreative, TargetingMap,
  PricingModel as ApiPricing, TrafficType as ApiTraffic,
  CampaignStatus as ApiStatus, FormatType,
} from "@/api/types";
import { useAuth } from "@/contexts/AuthContext";
import {
  BROWSER_FILTER_MAP, OS_FILTER_MAP, DEVICE_FILTER_MAP,
  BROWSER_REVERSE, OS_REVERSE, DEVICE_REVERSE,
} from "@/lib/statFilters";
import {
  buildUrlWithMacros,
  createCampaignCreatives,
  extractBannerTargetUrl,
  extractIframeSrc,
  isVideoAsset,
  syncCampaignCreatives,
} from "@/lib/creativeApi";

// Browser/OS/device_type items in the UI are group keys (e.g. "Chrome",
// "iOS", "mobile") but the backend targeting expects raw values (e.g.
// "Chromium", "JSChromeBrowser"). Expand on send, collapse on read.
// Unknown raw values on read are dropped silently — the UI has no "other"
// bucket for targeting because targeting is a server-side rule and cannot
// express "everything not in the known groups".
const EXPAND_BY_UI_KEY: Record<string, Record<string, string[]>> = {
  browser: BROWSER_FILTER_MAP,
  os: OS_FILTER_MAP,
  deviceType: DEVICE_FILTER_MAP,
};
const COLLAPSE_BY_UI_KEY: Record<string, Map<string, string>> = {
  browser: BROWSER_REVERSE,
  os: OS_REVERSE,
  deviceType: DEVICE_REVERSE,
};
function expandTargetingItems(uiKey: string, items: string[]): string[] {
  const map = EXPAND_BY_UI_KEY[uiKey];
  if (!map) return items;
  return Array.from(new Set(items.flatMap(k => map[k] ?? [k])));
}
function collapseTargetingItems(uiKey: string, items: string[]): string[] {
  const rev = COLLAPSE_BY_UI_KEY[uiKey];
  if (!rev) return items;
  const out = new Set<string>();
  for (const raw of items) {
    const group = rev.get(raw);
    if (group) out.add(group);
  }
  return Array.from(out);
}

export type CampaignStatus = ApiStatus;
export type PricingModel = ApiPricing;
export type TrafficQuality = "common" | "high" | "ultra";
export type ListMode = "none" | "white" | "black";
export type TrafficType = ApiTraffic;

type ApiQuality = ApiCampaign["quality_type"];
const uiQualityToApi = (q: TrafficQuality): ApiQuality =>
  q === "common" ? "usual" : q === "high" ? "high_quality" : "ultra_high_quality";
const apiQualityToUi = (q: ApiQuality | string | undefined): TrafficQuality => {
  if (q === "usual" || q === "common") return "common";
  if (q === "high_quality" || q === "high") return "high";
  if (q === "ultra_high_quality" || q === "ultra") return "ultra";
  return "common";
};

export interface TargetingState {
  mode: ListMode;
  items: string[];
}

export type CreativeType = "image" | "html" | "iframe";

export interface Creative {
  id: string;
  name?: string;
  url: string;
  /** Permanent backend image_url, or a local data URL preview before upload. */
  imageUrl?: string;
  /** Permanent backend image identifier. Kept only for display/state; unchanged images are omitted from PATCH. */
  imageId?: string;
  imageFileName?: string;
  imageMimeType?: string;
  mediaType?: "image" | "video";
  /** New file picked by the user. Uploaded before the creative JSON POST/PATCH. */
  pendingFile?: File;
  title?: string;
  description?: string;
  /** UI-only flag: the uploaded image dimensions don't match the required size. Not sent to API. */
  sizeMismatch?: boolean;
  /** UI representation mapped to backend banner_type (`image` -> `img`, HTML/iframe -> `iframe`). */
  creativeType?: CreativeType;
  /** Raw HTML markup stored in backend `adm` with banner_type `iframe`. */
  htmlCode?: string;
  /** iframe URL. The frontend turns it into complete iframe markup for backend `adm`. */
  iframeUrl?: string;
  /** Raw <iframe ...> snippet stored in backend `adm`. */
  iframeCode?: string;
  /** UI-only: sub-mode inside iframe creative type. Defaults to "url". Not sent to API. */
  iframeMode?: "url" | "code";
  /** UI-only: user confirmed the cross-origin iframe matches the banner size. */
  iframeSizeConfirmed?: boolean;
}

export const VERTICALS = [
  "Dating", "Nutra", "Betting / iGaming", "Gaming", "Crypto",
  "Finance", "Software", "E-commerce", "Beauty", "Adult", "Other",
] as const;
export type Vertical = typeof VERTICALS[number];

export interface Campaign {
  id: string;
  name: string;
  status: CampaignStatus;
  format: string;
  formatKey: string;
  budget: number;
  /** Removed from UI but kept on the type for backwards compatibility. Always null. */
  dailyBudget: number | null;
  spent: number;
  impressions: number;
  clicks: number;
  ctr: number;
  pricingModel: PricingModel;
  priceValue: number;
  trafficQuality: TrafficQuality;
  startDate: string;
  endDate: string;
  creatives: Creative[];
  targeting: Record<string, TargetingState>;
  evenSpend: boolean;
  bannerSize?: string;
  brandName?: string;
  trafficType: TrafficType;
  verticals: Vertical[];
  description?: string;
  /** Fixed reward per conversion (USD). Sent as `payout` on the API body. */
  conversionPayout?: number | null;
}

// ---- Targeting <-> API list conversion ------------------------------------
// New API shape: { isWhiteList: boolean, objects: string[] }.
// Old shape (still accepted on read for backwards compatibility): { "<id>": 0 | 1 }.
interface TargetingListPayload { isWhiteList: boolean; objects: string[] }
function targetingStateToPayload(t: TargetingState): TargetingListPayload {
  if (t.mode === "none" || t.items.length === 0) return { isWhiteList: true, objects: [] };
  return { isWhiteList: t.mode === "white", objects: t.items };
}
function targetingPayloadToState(m: TargetingListPayload | TargetingMap | undefined): TargetingState {
  if (!m) return { mode: "none", items: [] };
  // New shape
  if (typeof (m as any).isWhiteList === "boolean" && Array.isArray((m as any).objects)) {
    const p = m as TargetingListPayload;
    if (!p.objects.length) return { mode: "none", items: [] };
    return { mode: p.isWhiteList ? "white" : "black", items: p.objects.map(String) };
  }
  // Old shape
  const entries = Object.entries(m as Record<string, unknown>);
  if (!entries.length) return { mode: "none", items: [] };
  const allWhite = entries.every(([, v]) => v === 1 || v === true);
  return { mode: allWhite ? "white" : "black", items: entries.map(([k]) => k) };
}

function verticalsToApiArray(verticals: readonly string[] | undefined): Record<string, 1> {
  return Object.fromEntries((verticals || []).map(v => [v, 1])) as Record<string, 1>;
}

const TARGET_KEY_MAP = [
  ["country", "country"], ["language", "language"], ["deviceType", "device_type"],
  ["os", "os"], ["browser", "browser"], ["sites", "site_id"], ["ip", "ip"],
] as const;
type TargetKey = typeof TARGET_KEY_MAP[number][1];

const DAY_ORDER = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"];
const DAY_TO_API: Record<string, string> = { monday: "mon", tuesday: "tue", wednesday: "wed", thursday: "thu", friday: "fri", saturday: "sat", sunday: "sun" };
const API_TO_DAY: Record<string, string> = { mon: "monday", tue: "tuesday", wed: "wednesday", wen: "wednesday", thu: "thursday", fri: "friday", sat: "saturday", sun: "sunday" };

function scheduleItemIndex(item: string): number | null {
  const [day, hourRaw] = item.split(":");
  const dayIndex = DAY_ORDER.indexOf(day);
  const hour = Number(hourRaw);
  return dayIndex >= 0 && Number.isInteger(hour) && hour >= 0 && hour <= 23 ? dayIndex * 24 + hour : null;
}

function indexToApiPoint(index: number): string {
  const day = DAY_ORDER[Math.floor(index / 24)] || "monday";
  return `${DAY_TO_API[day]},${index % 24}`;
}

function apiPointToIndex(point: string): number | null {
  const [dayRaw, hourRaw] = point.split(",");
  const day = API_TO_DAY[dayRaw?.trim().toLowerCase()];
  return scheduleItemIndex(`${day}:${hourRaw}`);
}

function scheduleToActiveIntervals(schedule?: TargetingState): ApiCampaign["active_intervals"] {
  if (!schedule || schedule.mode === "none" || schedule.items.length === 0) return [["mon,1", "sun,23"]];
  const indexes = Array.from(new Set(schedule.items.map(scheduleItemIndex).filter((v): v is number => v !== null))).sort((a, b) => a - b);
  if (!indexes.length) return [["mon,1", "sun,23"]];
  const intervals: ApiCampaign["active_intervals"] = [];
  let start = indexes[0], prev = indexes[0];
  for (let i = 1; i < indexes.length; i += 1) {
    if (indexes[i] === prev + 1) { prev = indexes[i]; continue; }
    intervals.push([indexToApiPoint(start), indexToApiPoint(prev)]);
    start = prev = indexes[i];
  }
  intervals.push([indexToApiPoint(start), indexToApiPoint(prev)]);
  return intervals;
}

function activeIntervalsToSchedule(intervals: ApiCampaign["active_intervals"] | undefined): TargetingState {
  if (!Array.isArray(intervals) || intervals.length === 0) return { mode: "none", items: [] };
  const indexes = new Set<number>();
  for (const [from, to] of intervals) {
    const start = apiPointToIndex(from), end = apiPointToIndex(to);
    if (start === null || end === null) continue;
    for (let i = Math.min(start, end); i <= Math.max(start, end); i += 1) indexes.add(i);
  }
  const items = Array.from(indexes).sort((a, b) => a - b).map(i => `${DAY_ORDER[Math.floor(i / 24)]}:${i % 24}`);
  return items.length ? { mode: "white", items } : { mode: "none", items: [] };
}

function buildApiTargeting(targeting: Record<string, TargetingState>): Pick<ApiCampaign, TargetKey> {
  const out: any = {};
  for (const [uiKey, apiKey] of TARGET_KEY_MAP) {
    const state = targeting[uiKey] || { mode: "none", items: [] };
    const expanded: TargetingState = { ...state, items: expandTargetingItems(uiKey, state.items) };
    out[apiKey] = targetingStateToPayload(expanded);
  }
  return out as Pick<ApiCampaign, TargetKey>;
}
function readApiTargeting(c: ApiCampaign): Record<string, TargetingState> {
  return {
    ...Object.fromEntries(TARGET_KEY_MAP.map(([uiKey, apiKey]) => {
      const state = targetingPayloadToState(c[apiKey] as any);
      return [uiKey, { ...state, items: collapseTargetingItems(uiKey, state.items) }];
    })),
    schedule: activeIntervalsToSchedule(c.active_intervals),
  };
}


// ---- Mapping --------------------------------------------------------------
function mapApiCampaignToUi(c: ApiCampaign, creatives: Creative[]): Campaign {
  let priceValue = Number(c.base_price) || 0;
  // Popunder CPC is stored as CPM-equivalent (value * 1000). Convert back for display.
  if (c.format_type === "popunder" && c.pricing_model === "cpc") {
    priceValue = priceValue / 1000;
  }
  return {
    id: c.campaign_id,
    name: c.campaign_name,
    status: c.status,
    format: c.format_type, // human label = key for now
    formatKey: c.format_type,
    budget: Number(c.goal_total_dollars) || 0,
    dailyBudget: null,
    spent: Number(c.cum_done_dollars) || 0,
    impressions: 0,
    clicks: 0,
    ctr: 0,
    pricingModel: c.pricing_model,
    priceValue: Number(priceValue) || 0,
    trafficQuality: apiQualityToUi(c.quality_type),
    startDate: c.start_ts ? c.start_ts.slice(0, 10) : "",
    endDate: c.end_ts ? c.end_ts.slice(0, 10) : "",
    creatives,
    targeting: readApiTargeting(c),
    evenSpend: !!c.evenness_by_slot_mode,
    bannerSize: c.w && c.h ? `${c.w}x${c.h}` : undefined,
    brandName: c.brand_name || undefined,
    trafficType: c.traffic_type,
    verticals: (Array.isArray(c.vertical)
      ? (c.vertical as string[])
      : c.vertical && typeof c.vertical === "object"
        ? Object.entries(c.vertical as Record<string, 0 | 1>).filter(([, v]) => v === 1).map(([k]) => k)
        : []) as Vertical[],
    description: undefined,
    conversionPayout: c.payout ?? null,
  };
}

export function mapApiCreativeToUi(cr: ApiCreative): Creative {
  const adm = cr.adm || "";
  const isBannerImage = cr.banner_type === "img";
  const isBackendIframe = cr.banner_type === "iframe";
  const isStandaloneIframe = isBackendIframe && /^\s*<iframe\b/i.test(adm);
  const iframeSrc = isStandaloneIframe ? extractIframeSrc(adm) : "";
  const creativeType: CreativeType | undefined = isBannerImage
    ? "image"
    : isBackendIframe
      ? (isStandaloneIframe ? "iframe" : "html")
      : undefined;
  const cleanTargetUrl = isBannerImage ? extractBannerTargetUrl(adm) : adm;
  const mimeType = cr.mime_type || undefined;
  const imageName = cr.image_name || undefined;

  const mapped: Creative = {
    id: cr.id,
    name: cr.creative_name || undefined,
    url: buildUrlWithMacros(cleanTargetUrl, cr.trackers_macros),
    imageId: cr.image_id || undefined,
    imageUrl: cr.image_url || undefined,
    imageFileName: imageName,
    imageMimeType: mimeType,
    mediaType: isVideoAsset(mimeType, imageName) ? "video" : "image",
    title: cr.title || undefined,
    description: cr.description || undefined,
    creativeType,
  };

  if (creativeType === "html") {
    mapped.htmlCode = adm;
  } else if (creativeType === "iframe") {
    // The backend intentionally stores HTML and iframe URL mode identically:
    // banner_type=iframe + adm string. A complete iframe is reopened in code
    // mode because the original frontend sub-mode is not part of the API.
    mapped.iframeMode = "code";
    mapped.iframeCode = adm;
    mapped.iframeUrl = iframeSrc;
    mapped.iframeSizeConfirmed = true;
  }
  return mapped;
}

/**
 * Convert a `YYYY-MM-DD` form value into the timestamps the backend expects.
 * Returns `null` for empty values — the backend's Go time parser rejects "" with
 * `parsing time "" as "2006-01-02T15:04:05Z07:00"`, so drafts with no dates
 * MUST send null instead of an empty string.
 */
function startTimestamp(date: string): string | null {
  if (!date) return null;
  return `${date}T00:00:00Z`;
}
function endTimestamp(date: string): string | null {
  if (!date) return null;
  // Inclusive end-of-day for the chosen end date.
  return `${date}T23:59:59Z`;
}

function buildApiCampaignBody(c: Omit<Campaign, "id">): Omit<ApiCampaign, "campaign_id" | "user_id" | "cum_done_dollars"> {
  let w: number | null = null, h: number | null = null;
  if (c.bannerSize && /^\d+x\d+$/.test(c.bannerSize)) {
    const [ws, hs] = c.bannerSize.split("x");
    w = Number(ws); h = Number(hs);
  }
  const body: any = {
    campaign_name: c.name,
    format_type: (c.formatKey || c.format) as FormatType,
    h, w,
    status: c.status,
    traffic_type: c.trafficType,
    vertical: verticalsToApiArray(c.verticals),
    pricing_model: c.pricingModel,
    base_price: c.priceValue,
    evenness_by_slot_mode: c.evenSpend,
    goal_total_dollars: c.budget,
    start_ts: startTimestamp(c.startDate),
    end_ts: endTimestamp(c.endDate),
    active_intervals: scheduleToActiveIntervals(c.targeting.schedule),
    quality_type: uiQualityToApi(c.trafficQuality),
    ...buildApiTargeting(c.targeting),
  };
  // brand_name is optional. Only include when the user provided a value
  // so the backend can apply its own default / nullability handling.
  if (c.brandName) body.brand_name = c.brandName;
  // For popunder, the backend only stores CPM. If the user selected CPC,
  // send the value as its CPM-equivalent (×1000), but keep pricing_model = "cpc"
  // so the user's choice is preserved and displayed back correctly.
  if (c.formatKey === "popunder" && c.pricingModel === "cpc") {
    body.base_price = c.priceValue * 1000;
  }
  return body;
}

/**
 * Map a *partial* UI update to a partial API patch. Only fields that are
 * actually present in `updates` are forwarded — this prevents bugs where
 * toggling one switch (e.g. status) rewrites unrelated fields like
 * notification preferences or budget.
 */
function buildApiCampaignPatch(updates: Partial<Campaign>): Partial<ApiCampaign> {
  const p: Partial<ApiCampaign> = {};
  if (updates.name !== undefined) p.campaign_name = updates.name;
  if (updates.formatKey !== undefined || updates.format !== undefined) {
    p.format_type = ((updates.formatKey ?? updates.format) || "") as FormatType;
  }
  if (updates.brandName !== undefined) p.brand_name = updates.brandName ?? null;
  if (updates.bannerSize !== undefined) {
    if (updates.bannerSize && /^\d+x\d+$/.test(updates.bannerSize)) {
      const [ws, hs] = updates.bannerSize.split("x");
      p.w = Number(ws); p.h = Number(hs);
    } else {
      p.w = null; p.h = null;
    }
  }
  if (updates.status !== undefined) p.status = updates.status;
  if (updates.trafficType !== undefined) p.traffic_type = updates.trafficType;
  if (updates.verticals !== undefined) p.vertical = verticalsToApiArray(updates.verticals);
  if (updates.pricingModel !== undefined || updates.priceValue !== undefined) {
    const pm = updates.pricingModel;
    const pv = updates.priceValue;
    if (pm !== undefined && pv !== undefined) {
      p.pricing_model = pm;
      p.base_price = pv;
    } else if (pv !== undefined) {
      p.base_price = pv;
    } else if (pm !== undefined) {
      p.pricing_model = pm;
    }
  }
  if (updates.evenSpend !== undefined) p.evenness_by_slot_mode = updates.evenSpend;
  if (updates.trafficQuality !== undefined) p.quality_type = uiQualityToApi(updates.trafficQuality);
  if (updates.budget !== undefined) p.goal_total_dollars = updates.budget;
  if (updates.startDate !== undefined) p.start_ts = startTimestamp(updates.startDate);
  if (updates.endDate !== undefined) p.end_ts = endTimestamp(updates.endDate);
  if (updates.targeting !== undefined) {
    Object.assign(p, buildApiTargeting(updates.targeting));
    p.active_intervals = scheduleToActiveIntervals(updates.targeting.schedule);
  }
  return p;
}

interface CampaignContextType {
  campaigns: Campaign[];
  loading: boolean;
  addCampaign: (c: Omit<Campaign, "id">) => Promise<string | undefined>;
  updateCampaign: (id: string, updates: Partial<Campaign>) => Promise<void>;
  deleteCampaign: (id: string) => Promise<void>;
  getCampaign: (id: string) => Campaign | undefined;
  refetch: () => Promise<void>;
}

const CampaignContext = createContext<CampaignContextType | null>(null);

export function CampaignProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchCampaigns = useCallback(async () => {
    if (!user) { setCampaigns([]); setLoading(false); return; }
    setLoading(true);
    try {
      const res = await api.listCampaigns();
      // Backend may return `items: null` when the user has no campaigns yet.
      // Treat null/undefined as an empty list instead of crashing on .map.
      const items: ApiCampaign[] = Array.isArray(res?.items) ? res.items : [];
      // Isolate creative loading per-campaign: a single failure must not
      // break the whole list. Failed reads degrade to an empty creatives
      // array so the rest of the campaign still shows up.
      const withCreatives = await Promise.all(items.map(async c => {
        let crs: ApiCreative[] = [];
        try {
          const r = await api.readCreatives(c.campaign_id);
          crs = Array.isArray(r) ? r : [];
        } catch (e) {
          console.error(`readCreatives failed for ${c.campaign_id}:`, e);
        }
        return mapApiCampaignToUi(c, crs.map(mapApiCreativeToUi));
      }));
      setCampaigns(withCreatives);
    } catch (e) {
      console.error("Campaigns fetch error:", e);
      setCampaigns([]);
    } finally {
      setLoading(false);
    }
  }, [user]);

  useEffect(() => { fetchCampaigns(); }, [fetchCampaigns]);

  const addCampaign = useCallback(async (c: Omit<Campaign, "id">): Promise<string | undefined> => {
    if (!user) throw new Error("Not authenticated");
    // Errors here propagate to the caller so the UI can show the real
    // backend message instead of a fake success toast.
    const created = await api.createCampaign(buildApiCampaignBody(c));
    let cw: number | null = null, ch: number | null = null;
    if (c.formatKey === "banner" && c.bannerSize && /^\d+x\d+$/.test(c.bannerSize)) {
      const [ws, hs] = c.bannerSize.split("x");
      cw = Number(ws); ch = Number(hs);
    }
    await createCampaignCreatives({
      client: api,
      campaignId: created.campaign_id,
      format: c.formatKey,
      dimensions: { w: cw, h: ch },
      creatives: c.creatives,
      // Incomplete auto-saved drafts may legitimately have no finished
      // creative yet. Formal campaign creation is validated by the page.
      skipIncomplete: c.status === "draft",
    });
    await fetchCampaigns();
    return created.campaign_id;
  }, [user, fetchCampaigns]);

  const updateCampaign = useCallback(async (id: string, updates: Partial<Campaign>) => {
    if (!user) throw new Error("Not authenticated");
    // For popunder CPC, keep pricing_model = "cpc" but store the value as CPM-equivalent (×1000).
    const current = campaigns.find(c => c.id === id);
    const effectiveUpdates: Partial<Campaign> = { ...updates };
    const fmt = effectiveUpdates.formatKey ?? current?.formatKey;
    const pm = effectiveUpdates.pricingModel ?? current?.pricingModel;
    if (fmt === "popunder" && pm === "cpc" && effectiveUpdates.priceValue !== undefined) {
      effectiveUpdates.priceValue = (effectiveUpdates.priceValue as number) * 1000;
    }
    // Sync creatives BEFORE patching the campaign. Some status transitions
    // (draft → moderation) are rejected by the backend when the campaign has
    // no creatives, so we need the creatives to exist before the PATCH runs.
    if (updates.creatives !== undefined) {
      // Never degrade a failed read to an empty list here: doing so would
      // misclassify every existing creative as new and create duplicates.
      const existingRaw = await api.readCreatives(id);
      const existing: ApiCreative[] = Array.isArray(existingRaw) ? existingRaw : [];
      // Resolve current banner size for w/h on creative body.
      const currentC = campaigns.find(c => c.id === id);
      const formatKey = updates.formatKey ?? currentC?.formatKey;
      const bannerSize = updates.bannerSize ?? currentC?.bannerSize;
      let cw: number | null = null, ch: number | null = null;
      if (formatKey === "banner" && bannerSize && /^\d+x\d+$/.test(bannerSize)) {
        const [ws, hs] = bannerSize.split("x");
        cw = Number(ws); ch = Number(hs);
      }
      await syncCampaignCreatives({
        client: api,
        campaignId: id,
        format: formatKey || "",
        dimensions: { w: cw, h: ch },
        creatives: updates.creatives,
        existing,
      });
    }

    // Build a *partial* patch so toggling a single field (status, budget,
    // ...) does not rewrite unrelated fields.
    const patch = buildApiCampaignPatch(effectiveUpdates);
    if (Object.keys(patch).length > 0) {
      await api.patchCampaign(id, patch);
    }

    await fetchCampaigns();
  }, [user, fetchCampaigns, campaigns]);

  const deleteCampaign = useCallback(async (id: string) => {
    if (!user) throw new Error("Not authenticated");
    await api.deleteCampaign(id);
    await fetchCampaigns();
  }, [user, fetchCampaigns]);

  const getCampaign = useCallback((id: string) => campaigns.find(c => c.id === id), [campaigns]);

  return (
    <CampaignContext.Provider value={{ campaigns, loading, addCampaign, updateCampaign, deleteCampaign, getCampaign, refetch: fetchCampaigns }}>
      {children}
    </CampaignContext.Provider>
  );
}

export function useCampaigns() {
  const ctx = useContext(CampaignContext);
  if (!ctx) throw new Error("useCampaigns must be used within CampaignProvider");
  return ctx;
}
