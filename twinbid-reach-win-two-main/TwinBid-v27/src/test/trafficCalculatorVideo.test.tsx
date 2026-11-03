// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Campaign } from "@/contexts/CampaignContext";

const mocks = vi.hoisted(() => ({ campaigns: [] as Campaign[], calculator: vi.fn() }));
vi.mock("@/contexts/CampaignContext", () => ({ useCampaigns: () => ({
  campaigns: mocks.campaigns, loading: false, updateCampaign: vi.fn(),
}) }));
vi.mock("@/contexts/LanguageContext", () => ({ useLanguage: () => ({ lang: "en", t: (key: string) => key }) }));
vi.mock("@/api", () => ({ api: { calculator: mocks.calculator, statsQuery: vi.fn() } }));
vi.mock("@/hooks/use-touch-scroll-selection-guard", () => ({ useTouchScrollSelectionGuard: () => {} }));

import TrafficCalculator from "@/pages/TrafficCalculator";

beforeEach(() => {
  mocks.campaigns = [];
  mocks.calculator.mockReset().mockRejectedValue(new Error("test response"));
});

describe("video traffic calculation", () => {
  it("offers Video in a free calculation and sends video format_type", async () => {
    render(<TrafficCalculator />);
    fireEvent.keyDown(screen.getAllByRole("combobox")[0], { key: "Enter" });
    fireEvent.click(screen.getByRole("option", { name: "Video" }));
    fireEvent.click(screen.getByRole("button", { name: "Get data" }));
    await waitFor(() => expect(mocks.calculator).toHaveBeenCalledWith(expect.objectContaining({ format_type: "video" })));
  });

  it("loads the Video format when an existing video campaign is selected", () => {
    mocks.campaigns = [{
      id: "video-1", name: "Existing video campaign", status: "paused", formatKey: "video", format: "video",
      pricingModel: "cpm", priceValue: 1, trafficQuality: "common", trafficType: "mainstream", targeting: {},
    } as Campaign];
    render(<TrafficCalculator />);
    fireEvent.click(screen.getByRole("button", { name: /Existing video campaign/ }));
    expect(screen.getAllByRole("combobox")[0]).toHaveTextContent("Video");
  });

  it("hides iOS minor releases in the calculator after choosing a major release", () => {
    render(<TrafficCalculator />);
    fireEvent.keyDown(screen.getByRole("button", { name: /^OS:/ }), { key: "Enter" });
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "iOS" }));
    fireEvent.keyDown(screen.getByRole("button", { name: /^targeting.osVersion:/ }), { key: "Enter" });
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "iOS 18" }));
    expect(screen.queryByRole("menuitemcheckbox", { name: "iOS 18.7" })).not.toBeInTheDocument();
  });
});
