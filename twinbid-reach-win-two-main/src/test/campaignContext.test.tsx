import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ApiCampaign, ApiCreative } from "@/api/types";

const apiMock = vi.hoisted(() => ({
  listCampaigns: vi.fn(),
  readCreatives: vi.fn(),
  createCampaign: vi.fn(),
  createCreative: vi.fn(),
  uploadCreativeImage: vi.fn(),
  patchCampaign: vi.fn(),
  patchCreative: vi.fn(),
  deleteCreative: vi.fn(),
  deleteCampaign: vi.fn(),
}));
const authMock = vi.hoisted(() => ({ user: { id: "user-1" } }));

vi.mock("@/api", () => ({ api: apiMock }));
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => authMock,
}));

import { CampaignProvider, useCampaigns, type Campaign } from "@/contexts/CampaignContext";

const apiCampaign: ApiCampaign = {
  campaign_id: "campaign-1",
  user_id: "user-1",
  campaign_name: "Campaign",
  format_type: "popunder",
  status: "draft",
  traffic_type: "mainstream",
  vertical: {},
  pricing_model: "cpm",
  base_price: 1,
  evenness_by_slot_mode: false,
  goal_total_dollars: 10,
  cum_done_dollars: 0,
  start_ts: "2026-07-26T00:00:00Z",
  end_ts: "2026-07-27T23:59:59Z",
  active_intervals: [["mon,1", "sun,23"]],
  country: {},
  language: {},
  device_type: {},
  os: {},
  browser: {},
  site_id: {},
  ip: {},
  quality_type: "usual",
};

const apiCreative: ApiCreative = {
  id: "creative-1",
  campaign_id: "campaign-1",
  creative_name: "Landing",
  adm: "http://example.com",
  trackers_macros: {},
  w: null,
  h: null,
};

const campaignDraft: Omit<Campaign, "id"> = {
  name: "Campaign",
  status: "draft",
  format: "popunder",
  formatKey: "popunder",
  budget: 10,
  dailyBudget: null,
  spent: 0,
  impressions: 0,
  clicks: 0,
  ctr: 0,
  pricingModel: "cpm",
  priceValue: 1,
  trafficQuality: "common",
  startDate: "2026-07-26",
  endDate: "2026-07-27",
  creatives: [{ id: "local-1", name: "Landing", url: "http://example.com" }],
  targeting: {},
  evenSpend: false,
  trafficType: "mainstream",
  verticals: [],
};

function wrapper({ children }: { children: ReactNode }) {
  return <CampaignProvider>{children}</CampaignProvider>;
}

describe("CampaignProvider mutation requests", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.listCampaigns.mockResolvedValue({ items: [], total: 0 });
    apiMock.createCampaign.mockResolvedValue(apiCampaign);
    apiMock.createCreative.mockResolvedValue(apiCreative);
    apiMock.patchCampaign.mockImplementation(async (_id, patch) => ({
      ...apiCampaign,
      ...patch,
    }));
  });

  it("does not reload creatives for every campaign after create and status update", async () => {
    const { result } = renderHook(() => useCampaigns(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    let id: string | undefined;
    await act(async () => {
      id = await result.current.addCampaign(campaignDraft);
    });
    await act(async () => {
      await result.current.updateCampaign(id!, { status: "moderation" });
    });

    expect(apiMock.createCreative).toHaveBeenCalledTimes(1);
    expect(apiMock.patchCampaign).toHaveBeenCalledTimes(1);
    expect(apiMock.listCampaigns).toHaveBeenCalledTimes(1);
    expect(apiMock.readCreatives).not.toHaveBeenCalled();
    expect(result.current.campaigns).toHaveLength(1);
    expect(result.current.campaigns[0].creatives[0].id).toBe("creative-1");
    expect(result.current.campaigns[0].status).toBe("moderation");
  });

  it("sends the technical 999x999 size for a banner campaign", async () => {
    const { result } = renderHook(() => useCampaigns(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.addCampaign({
        ...campaignDraft,
        format: "banner",
        formatKey: "banner",
        creatives: [],
      });
    });

    expect(apiMock.createCampaign).toHaveBeenCalledWith(
      expect.objectContaining({ w: 999, h: 999 }),
    );
  });

  it("keeps the technical banner size on status updates", async () => {
    apiMock.listCampaigns.mockResolvedValue({
      items: [{ ...apiCampaign, format_type: "banner", w: null, h: null }],
      total: 1,
    });
    const { result } = renderHook(() => useCampaigns(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.updateCampaign("campaign-1", { status: "moderation" });
    });

    expect(apiMock.patchCampaign).toHaveBeenCalledWith(
      "campaign-1",
      expect.objectContaining({ status: "moderation", w: 999, h: 999 }),
    );
  });

  it("loads the campaign list without requesting creatives for every campaign", async () => {
    apiMock.listCampaigns.mockResolvedValue({
      items: [
        apiCampaign,
        { ...apiCampaign, campaign_id: "campaign-2", campaign_name: "Campaign 2" },
        { ...apiCampaign, campaign_id: "campaign-3", campaign_name: "Campaign 3" },
      ],
      total: 3,
    });

    const { result } = renderHook(() => useCampaigns(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.campaigns).toHaveLength(3);
    expect(apiMock.listCampaigns).toHaveBeenCalledTimes(1);
    expect(apiMock.readCreatives).not.toHaveBeenCalled();
    expect(result.current.campaigns.every(campaign => campaign.creatives.length === 0)).toBe(true);
  });

  it("loads creatives only on demand and deduplicates concurrent reads", async () => {
    apiMock.listCampaigns.mockResolvedValue({ items: [apiCampaign], total: 1 });
    apiMock.readCreatives.mockResolvedValue([apiCreative]);

    const { result } = renderHook(() => useCampaigns(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await Promise.all([
        result.current.loadCampaignCreatives("campaign-1"),
        result.current.loadCampaignCreatives("campaign-1"),
      ]);
    });

    expect(apiMock.readCreatives).toHaveBeenCalledTimes(1);
    expect(result.current.campaigns[0].creativesLoaded).toBe(true);
    expect(result.current.campaigns[0].creatives[0].id).toBe("creative-1");

    await act(async () => {
      await result.current.loadCampaignCreatives("campaign-1");
    });
    expect(apiMock.readCreatives).toHaveBeenCalledTimes(1);
  });
});
