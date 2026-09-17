// @vitest-environment jsdom
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mobileMock = vi.hoisted(() => ({ value: false }));

vi.mock("@/contexts/CampaignContext", () => ({
  VERTICALS: ["Gaming"],
  useCampaigns: () => ({
    campaigns: [],
    addCampaign: vi.fn(),
    updateCampaign: vi.fn(),
    refetch: vi.fn(),
  }),
}));

vi.mock("@/contexts/NotificationContext", () => ({
  useNotifications: () => ({ addNotification: vi.fn() }),
}));

const labels: Record<string, string> = {
  "create.title": "Create campaign",
  "create.step": "Step",
  "create.of": "of",
  "create.step1": "Basic info and creative",
  "create.step1Rtb": "Basic info and RTB endpoint",
  "create.launchTypeTitle": "How do you want to launch the campaign?",
  "create.launchTypeDescription": "Choose a launch method",
  "create.launchCabinet": "Launch in the cabinet",
  "create.launchCabinetDescription": "Manage creatives in the cabinet",
  "create.launchRtb": "RTB integration",
  "create.launchRtbDescription": "Connect your RTB endpoint",
  "create.selectLaunchType": "Select",
  "create.changeLaunchType": "Change launch method",
  "create.launchTypeCurrent": "Selected launch method",
  "create.rtbEndpoint": "RTB endpoint",
  "create.rtbEndpointPlaceholder": "https://bidder.example.com/openrtb2",
  "create.rtbEndpointHint": "Full HTTP endpoint",
  "create.adFormat": "Ad format *",
  "create.selectFormat": "Select format",
  "create.formatVideo": "Video",
  "create.videoCampaignUnsupported": "Video campaigns can no longer be created or edited.",
  "create.videoUnavailable": "unavailable for campaigns",
};

vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => ({ t: (key: string) => labels[key] || key }),
}));

vi.mock("@/hooks/use-mobile", () => ({ useIsMobileImmediate: () => mobileMock.value }));
vi.mock("@/api", () => ({ api: { recommendBid: vi.fn() } }));
vi.mock("@/components/dashboard/AutoCropConfirmDialog", () => ({ AutoCropConfirmDialog: () => null }));
vi.mock("@/components/dashboard/TargetingSection", () => ({
  targetingConfigs: [{ key: "schedule" }],
  TargetingSection: () => null,
}));
vi.mock("@/components/dashboard/TargetingImportDialog", () => ({ TargetingImportDialog: () => null }));
vi.mock("@/components/dashboard/BudgetSection", () => ({ BudgetSection: () => null }));
vi.mock("@/components/dashboard/PostbackSection", () => ({ PostbackSection: () => null }));
vi.mock("@/components/dashboard/CreativesEditor", async () => {
  const React = await import("react");
  return {
    CreativesEditor: React.forwardRef(() => <div>Creative editor</div>),
  };
});

import CreateCampaign from "@/pages/CreateCampaign";
import { CreateCampaignDialog } from "@/components/dashboard/CreateCampaignDialog";

describe("campaign launch type step", () => {
  beforeEach(() => {
    mobileMock.value = false;
  });

  it("requires a large launch-method dialog before showing the form", () => {
    render(<MemoryRouter><CreateCampaign /></MemoryRouter>);

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveClass("min-h-[50dvh]");
    expect(screen.getByRole("button", { name: /Launch in the cabinet/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /RTB integration/ })).toBeInTheDocument();
  });

  it("replaces creatives with a required RTB endpoint after selecting RTB", () => {
    render(<MemoryRouter><CreateCampaign /></MemoryRouter>);

    fireEvent.click(screen.getByRole("button", { name: /RTB integration/ }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByLabelText("RTB endpoint *")).toBeRequired();
    expect(screen.queryByText("Creative editor")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Change launch method" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("disables video and blocks programmatic selection for cabinet campaigns", () => {
    mobileMock.value = true;
    render(<MemoryRouter><CreateCampaign /></MemoryRouter>);

    fireEvent.click(screen.getByRole("button", { name: /Launch in the cabinet/ }));
    const formatSelect = screen.getByLabelText("Ad format *") as HTMLSelectElement;
    const videoOption = Array.from(formatSelect.options).find(option => option.value === "video");

    expect(videoOption).toBeDefined();
    expect(videoOption).toBeDisabled();
    fireEvent.change(formatSelect, { target: { value: "video" } });
    expect(formatSelect.value).toBe("");
    expect(screen.getByText("Video campaigns can no longer be created or edited.")).toBeInTheDocument();
    expect(screen.queryByText("Creative editor")).not.toBeInTheDocument();
  });

  it("disables video and blocks programmatic format changes after selecting RTB", () => {
    mobileMock.value = true;
    render(<MemoryRouter><CreateCampaign /></MemoryRouter>);

    fireEvent.click(screen.getByRole("button", { name: /RTB integration/ }));
    const formatSelect = screen.getByLabelText("Ad format *") as HTMLSelectElement;
    const videoOption = Array.from(formatSelect.options).find(option => option.value === "video");

    expect(videoOption).toBeDisabled();
    fireEvent.change(formatSelect, { target: { value: "video" } });
    expect(formatSelect.value).toBe("");
    expect(screen.getByText("Video campaigns can no longer be created or edited.")).toBeInTheDocument();
  });
});

describe("legacy campaign creation dialog", () => {
  it("keeps its video option disabled", () => {
    render(<CreateCampaignDialog open onOpenChange={vi.fn()} />);

    fireEvent.keyDown(screen.getAllByRole("combobox")[0], { key: "Enter" });
    const videoOption = screen.getByRole("option", { name: /Video — unavailable for campaigns/ });

    expect(videoOption).toHaveAttribute("aria-disabled", "true");
  });
});
