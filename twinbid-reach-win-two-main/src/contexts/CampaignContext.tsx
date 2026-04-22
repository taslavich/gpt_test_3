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

export interface TargetingState {
  mode: ListMode;
  items: string[];
}

export interface Creative {
  id: string;
  name?: string;
  url: string;
  imageUrl?: string;
  imageFileName?: string;
  storagePath?: string;
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

const TARGET_KEYS = ["country", "language", "device_type", "os", "browser", "site_id", "ip"] as const;
type TargetKey = typeof TARGET_KEYS[number];

function buildApiTargeting(targeting: Record<string, TargetingState>): Pick<ApiCampaign, TargetKey> {
  const out = {} as Pick<ApiCampaign, TargetKey>;
  for (const k of TARGET_KEYS) out[k] = targetingStateToMap(targeting[k] || { mode: "none", items: [] });
  return out;
}
function readApiTargeting(c: ApiCampaign): Record<string, TargetingState> {
  return Object.fromEntries(TARGET_KEYS.map(k => [k, targetingMapToState(c[k])]));
}

// ---- Mapping --------------------------------------------------------------
function mapApiCampaignToUi(c: ApiCampaign, creatives: Creative[]): Campaign {
  const priceValue = c.pricing_model === "cpc" ? c.base_price_cpc : c.base_price_cpm;
  return {
    id: c.campaing_id,
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
    trafficQuality: "common",
    startDate: c.start_ts ? c.start_ts.slice(0, 10) : "",
    endDate: c.end_ts ? c.end_ts.slice(0, 10) : "",
    creatives,
    targeting: readApiTargeting(c),
    evenSpend: !!c.evenness_by_slot_mode,
    bannerSize: c.w && c.h ? `${c.w}x${c.h}` : undefined,
    brandName: c.brand_name || undefined,
    trafficType: c.traffic_type,
    verticals: (c.vertical || []) as Vertical[],
    description: undefined,
  };
}

function mapApiCreativeToUi(cr: ApiCreative): Creative {
  const anyCr = cr as any;
  return {
    id: cr.id,
    name: cr.creative_name || undefined,
    url: cr.link,
    imageUrl: anyCr.s3_file_path || undefined,
    imageFileName: undefined,
    storagePath: anyCr.s3_file_path || undefined,
    title: anyCr.title || undefined,
    description: anyCr.description || undefined,
  };
}

function buildApiCampaignBody(c: Omit<Campaign, "id">): Omit<ApiCampaign, "campaing_id" | "user_id" | "cum_done_dollars"> {
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
    vertical: c.verticals,
    pricing_model: c.pricingModel,
    base_price_cpm: c.pricingModel === "cpm" ? c.priceValue : 0,
    base_price_cpc: c.pricingModel === "cpc" ? c.priceValue : 0,
    evenness_by_slot_mode: c.evenSpend,
    goal_total_dollars: c.budget,
    start_ts: c.startDate || "",
    end_ts: c.endDate || "",
    active_intervals: [],
    ...buildApiTargeting(c.targeting),
  };
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
      const { items } = await api.listCampaigns();
      const withCreatives = await Promise.all(items.map(async c => {
        const crs = await api.listCreatives(c.campaing_id);
        return mapApiCampaignToUi(c, crs.map(mapApiCreativeToUi));
      }));
      setCampaigns(withCreatives);
    } catch (e) {
      console.error("Campaigns fetch error:", e);
    } finally {
      setLoading(false);
    }
  }, [user]);

  useEffect(() => { fetchCampaigns(); }, [fetchCampaigns]);

  const addCampaign = useCallback(async (c: Omit<Campaign, "id">): Promise<string | undefined> => {
    if (!user) return undefined;
    try {
      const created = await api.createCampaign(buildApiCampaignBody(c));
      // Insert creatives. The API contract distinguishes shapes per format; here
      // we send the lowest common denominator that the mock accepts.
      for (const cr of c.creatives) {
        await api.createCreative(created.campaing_id, {
          creative_name: cr.name || "",
          link: cr.url,
          trackers_macros: {},
          ...(cr.imageUrl ? { s3_file_path: cr.imageUrl, file_format: "image/png" } : {}),
          ...(cr.title ? { title: cr.title } : {}),
          ...(cr.description ? { description: cr.description } : {}),
        } as any);
      }
      await fetchCampaigns();
      return created.campaing_id;
    } catch (e) {
      console.error("Add campaign error:", e);
      return undefined;
    }
  }, [user, fetchCampaigns]);

  const updateCampaign = useCallback(async (id: string, updates: Partial<Campaign>) => {
    if (!user) return;
    try {
      const current = campaigns.find(c => c.id === id);
      if (!current) return;
      const merged: Campaign = { ...current, ...updates };
      // Patch the campaign itself (full re-build keeps it simple).
      const body = buildApiCampaignBody(merged);
      await api.patchCampaign(id, body as Partial<ApiCampaign>);

      // Replace creatives wholesale if provided.
      if (updates.creatives !== undefined) {
        const existing = await api.listCreatives(id);
        await Promise.all(existing.map(cr => api.deleteCreative(cr.id)));
        for (const cr of updates.creatives) {
          await api.createCreative(id, {
            creative_name: cr.name || "",
            link: cr.url,
            trackers_macros: {},
            ...(cr.imageUrl ? { s3_file_path: cr.imageUrl, file_format: "image/png" } : {}),
            ...(cr.title ? { title: cr.title } : {}),
            ...(cr.description ? { description: cr.description } : {}),
          } as any);
        }
      }
      await fetchCampaigns();
    } catch (e) {
      console.error("Update campaign error:", e);
    }
  }, [user, fetchCampaigns, campaigns]);

  const deleteCampaign = useCallback(async (id: string) => {
    if (!user) return;
    try {
      await api.deleteCampaign(id);
      await fetchCampaigns();
    } catch (e) {
      console.error("Delete campaign error:", e);
    }
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
