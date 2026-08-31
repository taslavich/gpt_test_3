// @vitest-environment jsdom
import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { BudgetSection } from "@/components/dashboard/BudgetSection";
import { LanguageProvider } from "@/contexts/LanguageContext";
import type { CampaignTypeModel, PricingModel } from "@/contexts/CampaignContext";
import { TooltipProvider } from "@/components/ui/tooltip";

function Harness() {
  const [pricingModel, setPricingModel] = useState<PricingModel>("cpm");
  const [typeModel, setTypeModel] = useState<CampaignTypeModel>(1);
  const [priceValue, setPriceValue] = useState("1");

  return (
    <LanguageProvider>
      <TooltipProvider>
        <BudgetSection
          formatKey="banner"
          totalBudget="100"
          setTotalBudget={() => undefined}
          priceValue={priceValue}
          setPriceValue={setPriceValue}
          pricingModel={pricingModel}
          setPricingModel={setPricingModel}
          typeModel={typeModel}
          setTypeModel={setTypeModel}
          trafficQuality="common"
          setTrafficQuality={() => undefined}
          startDate=""
          setStartDate={() => undefined}
          endDate=""
          setEndDate={() => undefined}
          evenSpend={false}
          setEvenSpend={() => undefined}
        />
      </TooltipProvider>
    </LanguageProvider>
  );
}

describe("TwinBid CPM payment model", () => {
  beforeEach(() => {
    window.localStorage.setItem("twinbid_lang", "en");
  });

  it("keeps TwinBid CPM visible but unavailable", () => {
    render(<Harness />);

    const twinBidCpm = screen.getByRole("button", { name: "TwinBid CPM" });
    expect(twinBidCpm).toBeDisabled();
    fireEvent.click(twinBidCpm);
    expect(screen.getByText("CPM (cost per 1000 impressions) *")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Learn more about TwinBid CPM" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("How TwinBid CPM works")).toBeInTheDocument();
    expect(screen.getByText("$0.20 per 1,000 impressions")).toBeInTheDocument();
  });
});
