import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { supabase } from "@/integrations/supabase/client";
import { useAuth } from "@/contexts/AuthContext";

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

function mapCampaignFromDb(row: any, creatives: Creative[]): Campaign {
  return {
    id: row.id,
    name: row.name,
    status: row.status,
    format: row.format,
    formatKey: row.format_key,
    budget: Number(row.budget) || 0,
    dailyBudget: row.daily_budget != null ? Number(row.daily_budget) : null,
    spent: Number(row.spent) || 0,
    impressions: Number(row.impressions) || 0,
    clicks: Number(row.clicks) || 0,
    ctr: Number(row.ctr) || 0,
    pricingModel: row.pricing_model,
    priceValue: Number(row.price_value) || 0,
    trafficQuality: row.traffic_quality,
    startDate: row.start_date || "",
    endDate: row.end_date || "",
    creatives,
    targeting: (row.targeting && typeof row.targeting === "object") ? row.targeting : {},
    evenSpend: row.even_spend ?? false,
    bannerSize: row.banner_size || undefined,
    brandName: row.brand_name || undefined,
    trafficType: row.traffic_type || "mainstream",
    verticals: Array.isArray(row.verticals) ? row.verticals : [],
    description: row.description || undefined,
  };
}

function mapCreativeFromDb(row: any): Creative {
  return {
    id: row.id,
    name: row.name || undefined,
    url: row.url,
    imageUrl: row.image_url || undefined,
    imageFileName: row.image_file_name || undefined,
    storagePath: row.storage_path || undefined,
    title: row.title || undefined,
    description: row.description || undefined,
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
    const { data: rows, error } = await supabase
      .from("campaigns")
      .select("*")
      .eq("user_id", user.id)
      .order("created_at", { ascending: false });

    if (error) { console.error("Campaigns fetch error:", error); setLoading(false); return; }

    const ids = (rows || []).map(r => r.id);
    let creativesMap = new Map<string, Creative[]>();

    if (ids.length > 0) {
      const { data: crRows } = await supabase
        .from("creatives")
        .select("*")
        .in("campaign_id", ids);
      (crRows || []).forEach(cr => {
        const list = creativesMap.get(cr.campaign_id) || [];
        list.push(mapCreativeFromDb(cr));
        creativesMap.set(cr.campaign_id, list);
      });
    }

    setCampaigns((rows || []).map(r => mapCampaignFromDb(r, creativesMap.get(r.id) || [])));
    setLoading(false);
  }, [user]);

  useEffect(() => { fetchCampaigns(); }, [fetchCampaigns]);

  const addCampaign = useCallback(async (c: Omit<Campaign, "id">): Promise<string | undefined> => {
    if (!user) return undefined;
    const { data, error } = await supabase
      .from("campaigns")
      .insert({
        user_id: user.id,
        name: c.name,
        status: c.status,
        format: c.format,
        format_key: c.formatKey,
        budget: c.budget,
        daily_budget: c.dailyBudget,
        spent: c.spent,
        impressions: c.impressions,
        clicks: c.clicks,
        ctr: c.ctr,
        pricing_model: c.pricingModel,
        price_value: c.priceValue,
        traffic_quality: c.trafficQuality,
        traffic_type: c.trafficType,
        start_date: c.startDate || null,
        end_date: c.endDate || null,
        targeting: c.targeting as any,
        even_spend: c.evenSpend,
        banner_size: c.bannerSize || null,
        brand_name: c.brandName || null,
        verticals: c.verticals,
        description: c.description || null,
      })
      .select("id")
      .single();

    if (error) { console.error("Add campaign error:", error); return undefined; }

    const campaignId = data.id;

    // Insert creatives
    if (c.creatives.length > 0) {
      const { error: crError } = await supabase
        .from("creatives")
        .insert(c.creatives.map(cr => ({
          campaign_id: campaignId,
          url: cr.url,
          name: cr.name || null,
          image_url: cr.imageUrl || null,
          image_file_name: cr.imageFileName || null,
          storage_path: cr.storagePath || null,
          title: cr.title || null,
          description: cr.description || null,
        })));
      if (crError) console.error("Add creatives error:", crError);
    }

    await fetchCampaigns();
    return campaignId;
  }, [user, fetchCampaigns]);

  const updateCampaign = useCallback(async (id: string, updates: Partial<Campaign>) => {
    if (!user) return;
    const dbUpdates: any = {};
    if (updates.name !== undefined) dbUpdates.name = updates.name;
    if (updates.status !== undefined) dbUpdates.status = updates.status;
    if (updates.format !== undefined) dbUpdates.format = updates.format;
    if (updates.formatKey !== undefined) dbUpdates.format_key = updates.formatKey;
    if (updates.budget !== undefined) dbUpdates.budget = updates.budget;
    if (updates.dailyBudget !== undefined) dbUpdates.daily_budget = updates.dailyBudget;
    if (updates.spent !== undefined) dbUpdates.spent = updates.spent;
    if (updates.impressions !== undefined) dbUpdates.impressions = updates.impressions;
    if (updates.clicks !== undefined) dbUpdates.clicks = updates.clicks;
    if (updates.ctr !== undefined) dbUpdates.ctr = updates.ctr;
    if (updates.pricingModel !== undefined) dbUpdates.pricing_model = updates.pricingModel;
    if (updates.priceValue !== undefined) dbUpdates.price_value = updates.priceValue;
    if (updates.trafficQuality !== undefined) dbUpdates.traffic_quality = updates.trafficQuality;
    if (updates.trafficType !== undefined) dbUpdates.traffic_type = updates.trafficType;
    if (updates.startDate !== undefined) dbUpdates.start_date = updates.startDate || null;
    if (updates.endDate !== undefined) dbUpdates.end_date = updates.endDate || null;
    if (updates.targeting !== undefined) dbUpdates.targeting = updates.targeting;
    if (updates.evenSpend !== undefined) dbUpdates.even_spend = updates.evenSpend;
    if (updates.bannerSize !== undefined) dbUpdates.banner_size = updates.bannerSize;
    if (updates.brandName !== undefined) dbUpdates.brand_name = updates.brandName;
    if (updates.verticals !== undefined) dbUpdates.verticals = updates.verticals;
    if (updates.description !== undefined) dbUpdates.description = updates.description;

    if (Object.keys(dbUpdates).length > 0) {
      const { error } = await supabase.from("campaigns").update(dbUpdates).eq("id", id);
      if (error) { console.error("Update campaign error:", error); return; }
    }

    // Update creatives if provided
    if (updates.creatives !== undefined) {
      // Delete old creatives and insert new ones
      await supabase.from("creatives").delete().eq("campaign_id", id);
      if (updates.creatives.length > 0) {
        await supabase.from("creatives").insert(
          updates.creatives.map(cr => ({
            campaign_id: id,
            url: cr.url,
            name: cr.name || null,
            image_url: cr.imageUrl || null,
            image_file_name: cr.imageFileName || null,
            storage_path: cr.storagePath || null,
            title: cr.title || null,
            description: cr.description || null,
          }))
        );
      }
    }

    await fetchCampaigns();
  }, [user, fetchCampaigns]);

  const deleteCampaign = useCallback(async (id: string) => {
    if (!user) return;
    // Creatives will be cascade-deleted via FK
    const { error } = await supabase.from("campaigns").delete().eq("id", id);
    if (error) { console.error("Delete campaign error:", error); return; }
    await fetchCampaigns();
  }, [user, fetchCampaigns]);

  const getCampaign = useCallback((id: string) => {
    return campaigns.find(c => c.id === id);
  }, [campaigns]);

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
