// @vitest-environment jsdom
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

const loadCampaignCreatives = vi.fn();
const legacyVideoCampaign = {
  id: "legacy-video-1",
  name: "Old video campaign",
  launchType: "cabinet" as const,
  status: "paused" as const,
  format: "video",
  formatKey: "video",
  budget: 100,
  dailyBudget: null,
  spent: 10,
  impressions: 0,
  clicks: 0,
  ctr: 0,
  pricingModel: "cpm" as const,
  typeModel: 1 as const,
  priceValue: 1,
  trafficQuality: "common" as const,
  startDate: "2026-01-01",
  endDate: "2026-01-31",
  creatives: [],
  creativesLoaded: false,
  targeting: {},
  blockVpnTraffic: false,
  evenSpend: false,
  trafficType: "mainstream" as const,
  verticals: [],
};

vi.mock("@/contexts/CampaignContext", () => ({
  VERTICALS: [],
  useCampaigns: () => ({
    campaigns: [legacyVideoCampaign],
    getCampaign: (id: string) => id === legacyVideoCampaign.id ? legacyVideoCampaign : undefined,
    updateCampaign: vi.fn(),
    loadCampaignCreatives,
    loading: false,
  }),
}));

const labels: Record<string, string> = {
  "edit.title": "Edit campaign",
  "edit.name": "Name",
  "edit.formatLabel": "Ad format",
  "edit.legacyVideoReadOnlyTitle": "Legacy video campaign",
  "edit.legacyVideoReadOnlyDescription": "This campaign is available in read-only mode.",
  "create.back": "Back",
};

vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => ({ t: (key: string) => labels[key] || key }),
}));

vi.mock("@/api", () => ({ api: { recommendBid: vi.fn() } }));
vi.mock("@/components/dashboard/AutoCropConfirmDialog", () => ({ AutoCropConfirmDialog: () => null }));
vi.mock("@/components/dashboard/TargetingSection", () => ({ TargetingSection: () => null }));
vi.mock("@/components/dashboard/TargetingImportDialog", () => ({ TargetingImportDialog: () => null }));
vi.mock("@/components/dashboard/BudgetSection", () => ({ BudgetSection: () => null }));
vi.mock("@/components/dashboard/PostbackSection", () => ({ PostbackSection: () => null }));
vi.mock("@/components/dashboard/CreativesEditor", async () => {
  const React = await import("react");
  return { CreativesEditor: React.forwardRef(() => <div>Creative editor</div>) };
});

import EditCampaign from "@/pages/EditCampaign";

describe("legacy video campaign editing", () => {
  it("renders the campaign read-only without loading an editor", () => {
    render(
      <MemoryRouter initialEntries={["/dashboard/campaigns/legacy-video-1/edit"]}>
        <Routes>
          <Route path="/dashboard/campaigns/:id/edit" element={<EditCampaign />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText("Legacy video campaign")).toBeInTheDocument();
    expect(screen.getByText("This campaign is available in read-only mode.")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Old video campaign")).toBeDisabled();
    expect(screen.getByDisplayValue("video")).toBeDisabled();
    expect(screen.queryByText("Creative editor")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "edit.save" })).not.toBeInTheDocument();
    expect(loadCampaignCreatives).not.toHaveBeenCalled();
  });
});
