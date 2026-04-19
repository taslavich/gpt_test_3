import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { useAuth } from "@/contexts/AuthContext";
import { createCampaign as apiCreateCampaign, deleteCampaign as apiDeleteCampaign, listCampaigns as apiListCampaigns, updateCampaign as apiUpdateCampaign, type CampaignDto } from "@/lib/api";

export type CampaignStatus = "active" | "paused" | "draft" | "completed" | "moderation";
export type PricingModel = "cpm" | "cpc";
export type TrafficQuality = "common" | "high" | "ultra";
export type ListMode = "none" | "white" | "black";
export type TrafficType = "mainstream" | "adult" | "mixed";

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

export const VERTICALS = ["Dating", "Nutra", "Betting / iGaming", "Gaming", "Crypto", "Finance", "Software", "E-commerce", "Beauty", "Adult", "Other"] as const;
export type Vertical = typeof VERTICALS[number];

export interface Campaign {
  id: string;
  name: string;
  status: CampaignStatus;
  format: string;
  formatKey: string;
  budget: number;
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

const fromDto = (c: CampaignDto): Campaign => ({
  id: c.id,
  name: c.name,
  status: c.status,
  format: c.format,
  formatKey: c.formatKey,
  budget: c.budget,
  dailyBudget: c.dailyBudget,
  spent: c.spent,
  impressions: c.impressions,
  clicks: c.clicks,
  ctr: c.ctr,
  pricingModel: c.pricingModel,
  priceValue: c.priceValue,
  trafficQuality: c.trafficQuality,
  startDate: c.startDate,
  endDate: c.endDate,
  creatives: c.creatives,
  targeting: c.targeting,
  evenSpend: c.evenSpend,
  bannerSize: c.bannerSize,
  brandName: c.brandName,
  trafficType: c.trafficType,
  verticals: c.verticals as Vertical[],
  description: c.description,
});

export function CampaignProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchCampaigns = useCallback(async () => {
    if (!user) {
      setCampaigns([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    const rows = await apiListCampaigns(user.id);
    setCampaigns(rows.map(fromDto));
    setLoading(false);
  }, [user]);

  useEffect(() => {
    fetchCampaigns();
  }, [fetchCampaigns]);

  const addCampaign = useCallback(async (c: Omit<Campaign, "id">): Promise<string | undefined> => {
    if (!user) return undefined;
    const campaignId = await apiCreateCampaign(user.id, c);
    await fetchCampaigns();
    return campaignId;
  }, [user, fetchCampaigns]);

  const updateCampaign = useCallback(async (id: string, updates: Partial<Campaign>) => {
    if (!user) return;
    await apiUpdateCampaign(id, updates);
    await fetchCampaigns();
  }, [user, fetchCampaigns]);

  const deleteCampaign = useCallback(async (id: string) => {
    if (!user) return;
    await apiDeleteCampaign(id);
    await fetchCampaigns();
  }, [user, fetchCampaigns]);

  const getCampaign = useCallback((id: string) => campaigns.find(c => c.id === id), [campaigns]);

  return <CampaignContext.Provider value={{ campaigns, loading, addCampaign, updateCampaign, deleteCampaign, getCampaign, refetch: fetchCampaigns }}>{children}</CampaignContext.Provider>;
}

export function useCampaigns() {
  const ctx = useContext(CampaignContext);
  if (!ctx) throw new Error("useCampaigns must be used within CampaignProvider");
  return ctx;
}
