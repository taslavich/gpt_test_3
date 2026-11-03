import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DashboardPartners from "@/pages/DashboardPartners";

const apiMock = vi.hoisted(() => ({
  getPartnerStats: vi.fn(),
}));

vi.mock("@/api", () => ({
  api: apiMock,
}));

vi.mock("@/contexts/ProfileContext", () => ({
  useProfile: () => ({
    profile: {
      partnerId: "TBPROFILE123",
      partner: "TBREFERRER456",
    },
  }),
}));

vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => ({ t: (key: string) => key }),
}));

describe("DashboardPartners partner link", () => {
  beforeEach(() => {
    apiMock.getPartnerStats.mockReset().mockResolvedValue({
      partner: "TBSTATSCODE999",
      advertisers: 1,
      turnover: 100,
      withdrawn: 0,
    });
  });

  it("builds the link only from profile.partner_id", async () => {
    render(<DashboardPartners />);

    expect(await screen.findByText("http://localhost:3000/?partner=TBPROFILE123")).toBeInTheDocument();
    expect(screen.getByText("$10.00")).toBeInTheDocument();
    expect(screen.queryByText(/TBSTATSCODE999/)).not.toBeInTheDocument();
    expect(screen.queryByText(/TBREFERRER456/)).not.toBeInTheDocument();
  });
});
