import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { api } from "@/api";
import type {
  ApiCampaign, ApiCreative, TargetingMap,
  PricingModel as ApiPricing, TrafficType as ApiTraffic,
  CampaignStatus as ApiStatus, FormatType,
} from "@/api/types";
import { useAuth } from "@/contexts/AuthContext";

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

export interface Creative {
  id: string;
  name?: string;
  url: string;
  /** Display URL: presigned read URL from backend, or local data URL preview after a fresh upload. */
  imageUrl?: string;
  imageFileName?: string;
  /** New file picked by the user. Uploaded to the backend after the creative row is created/updated. */
  pendingFile?: File;
  title?: string;
  description?: string;
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
}

// ---- Targeting <-> TargetingMap conversion --------------------------------
function targetingStateToMap(t: TargetingState): TargetingMap {
  if (t.mode === "none" || t.items.length === 0) return {};
  const flag: 0 | 1 = t.mode === "white" ? 1 : 0;
  return Object.fromEntries(t.items.map(v => [v, flag])) as TargetingMap;
}
function targetingMapToState(m: TargetingMap | undefined): TargetingState {
  if (!m || Object.keys(m).length === 0) return { mode: "none", items: [] };
  const entries = Object.entries(m);
  const allWhite = entries.every(([, v]) => v === 1);
  return { mode: allWhite ? "white" : "black", items: entries.map(([k]) => k) };
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
  const out = {} as Pick<ApiCampaign, TargetKey>;
  for (const [uiKey, apiKey] of TARGET_KEY_MAP) out[apiKey] = targetingStateToMap(targeting[uiKey] || { mode: "none", items: [] });
  return out;
}
function readApiTargeting(c: ApiCampaign): Record<string, TargetingState> {
  return {
    ...Object.fromEntries(TARGET_KEY_MAP.map(([uiKey, apiKey]) => [uiKey, targetingMapToState(c[apiKey])])),
    schedule: activeIntervalsToSchedule(c.active_intervals),
  };
}

// ---- Mapping --------------------------------------------------------------
function mapApiCampaignToUi(c: ApiCampaign, creatives: Creative[]): Campaign {
  const priceValue = c.pricing_model === "cpc" ? c.base_price_cpc : c.base_price_cpm;
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
    verticals: (c.vertical && typeof c.vertical === "object" && !Array.isArray(c.vertical)
      ? Object.entries(c.vertical).filter(([, v]) => v === 1).map(([k]) => k)
      : Array.isArray(c.vertical) ? (c.vertical as unknown as string[]) : []) as Vertical[],
    description: undefined,
  };
}

function fileNameFromPath(p?: string): string | undefined {
  if (!p) return undefined;
  // For data URLs we can't derive a file name; show a generic label.
  if (p.startsWith("data:")) return "image";
  try {
    // Strip query string and take last path segment.
    const clean = p.split("?")[0];
    const tail = clean.substring(clean.lastIndexOf("/") + 1);
    return tail || undefined;
  } catch { return undefined; }
}

function mapApiCreativeToUi(cr: ApiCreative): Creative {
  const anyCr = cr as any;
  // Display URL = presigned read URL from the backend (image bytes).
  // File label shown in the cabinet = the `name` field (file name).
  return {
    id: cr.id,
    name: cr.creative_name || undefined,
    url: cr.link,
    imageUrl: anyCr.presigned_s3_url || undefined,
    imageFileName: anyCr.name || undefined,
    title: anyCr.title || undefined,
    description: anyCr.description || undefined,
  };
}

async function downloadCreativeImage(imageUrl: string, filename: string): Promise<File> {
  const resp = await fetch(imageUrl);
  if (!resp.ok) throw new Error(`Failed to download creative image: HTTP ${resp.status}`);
  const blob = await resp.blob();
  return new File([blob], filename, { type: blob.type || "image/jpeg" });
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
  return {
    campaign_name: c.name,
    format_type: (c.formatKey || c.format) as FormatType,
    brand_name: c.brandName ?? null,
    h, w,
    status: c.status,
    traffic_type: c.trafficType,
    vertical: Object.fromEntries((c.verticals || []).map(v => [v, 1])) as Record<string, 0 | 1>,
    pricing_model: c.pricingModel,
    base_price_cpm: c.pricingModel === "cpm" ? c.priceValue : 0,
    base_price_cpc: c.pricingModel === "cpc" ? c.priceValue : 0,
    evenness_by_slot_mode: c.evenSpend,
    goal_total_dollars: c.budget,
    start_ts: startTimestamp(c.startDate),
    end_ts: endTimestamp(c.endDate),
    active_intervals: scheduleToActiveIntervals(c.targeting.schedule),
    quality_type: uiQualityToApi(c.trafficQuality),
    ...buildApiTargeting(c.targeting),
  };
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
  if (updates.verticals !== undefined) p.vertical = Object.fromEntries(updates.verticals.map(v => [v, 1])) as Record<string, 0 | 1>;
  if (updates.pricingModel !== undefined || updates.priceValue !== undefined) {
    // Both fields cooperate; require pricingModel to know which slot.
    const pm = updates.pricingModel;
    const pv = updates.priceValue;
    if (pm !== undefined && pv !== undefined) {
      p.pricing_model = pm;
      p.base_price_cpm = pm === "cpm" ? pv : 0;
      p.base_price_cpc = pm === "cpc" ? pv : 0;
    } else if (pv !== undefined) {
      // Fall back to writing both with whatever the caller sent.
      p.base_price_cpm = pv;
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
    // Banner creatives need w/h on the creative body itself (backend expectation).
    let cw: number | null = null, ch: number | null = null;
    if (c.formatKey === "banner" && c.bannerSize && /^\d+x\d+$/.test(c.bannerSize)) {
      const [ws, hs] = c.bannerSize.split("x");
      cw = Number(ws); ch = Number(hs);
    }
    for (const cr of c.creatives) {
      await api.createCreative(
        created.campaign_id,
        {
          creative_name: cr.name || "",
          link: cr.url,
          trackers_macros: {},
          ...(cw && ch ? { w: cw, h: ch } : {}),
          ...(cr.title ? { title: cr.title } : {}),
          ...(cr.description ? { description: cr.description } : {}),
        } as any,
        cr.pendingFile,
        cr.pendingFile ? (cr.imageFileName || cr.pendingFile.name) : undefined,
      );
    }
    await fetchCampaigns();
    return created.campaign_id;
  }, [user, fetchCampaigns]);

  const updateCampaign = useCallback(async (id: string, updates: Partial<Campaign>) => {
    if (!user) throw new Error("Not authenticated");
    // Build a *partial* patch so toggling a single field (status, budget,
    // ...) does not rewrite unrelated fields.
    const patch = buildApiCampaignPatch(updates);
    if (Object.keys(patch).length > 0) {
      await api.patchCampaign(id, patch);
    }

    if (updates.creatives !== undefined) {
      const existingRaw = await api.readCreatives(id).catch(() => [] as ApiCreative[]);
      const existing: ApiCreative[] = Array.isArray(existingRaw) ? existingRaw : [];
      const existingById = new Map(existing.map(cr => [cr.id, cr]));
      // Resolve current banner size for w/h on creative body.
      const current = campaigns.find(c => c.id === id);
      const formatKey = updates.formatKey ?? current?.formatKey;
      const bannerSize = updates.bannerSize ?? current?.bannerSize;
      let cw: number | null = null, ch: number | null = null;
      if (formatKey === "banner" && bannerSize && /^\d+x\d+$/.test(bannerSize)) {
        const [ws, hs] = bannerSize.split("x");
        cw = Number(ws); ch = Number(hs);
      }
      const preparedCreatives = await Promise.all(updates.creatives.map(async cr => {
        let fileToSend: File | undefined = cr.pendingFile;
        let filenameToSend: string | undefined = cr.pendingFile
          ? (cr.imageFileName || cr.pendingFile.name)
          : undefined;
        const existingCreative = existingById.get(cr.id) as (ApiCreative & { name?: string; presigned_s3_url?: string }) | undefined;
        const sourceImageUrl = existingCreative?.presigned_s3_url || cr.imageUrl;
        if (!fileToSend && sourceImageUrl && /^https?:\/\//i.test(sourceImageUrl)) {
          const fname = cr.imageFileName || existingCreative?.name || fileNameFromPath(sourceImageUrl) || "image.jpg";
          fileToSend = await downloadCreativeImage(sourceImageUrl, fname);
          filenameToSend = fname;
        }
        return { cr, fileToSend, filenameToSend };
      }));

      await Promise.all(existing.map(cr => api.deleteCreative(cr.id)));

      for (const { cr, fileToSend, filenameToSend } of preparedCreatives) {
        await api.createCreative(
          id,
          {
            creative_name: cr.name || "",
            link: cr.url,
            trackers_macros: {},
            ...(cw && ch ? { w: cw, h: ch } : {}),
            ...(cr.title ? { title: cr.title } : {}),
            ...(cr.description ? { description: cr.description } : {}),
          } as any,
          fileToSend,
          filenameToSend,
        );
      }
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
