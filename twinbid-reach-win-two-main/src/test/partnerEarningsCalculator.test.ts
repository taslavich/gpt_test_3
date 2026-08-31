import { describe, expect, it } from "vitest";
import { calculatePartnerEarnings } from "@/lib/partnerEarningsCalculator";

describe("partner earnings calculator", () => {
  it("matches the economics from the TwinBid Partners brief", () => {
    expect(calculatePartnerEarnings({
      monthlyAffiliatePayout: 50_000,
      roiPercent: 25,
      mediaBuyers: 5,
    })).toEqual({
      trafficSpend: 200_000,
      twinbidProfit: 40_000,
      partnerIncome: 20_000,
      annualPartnerIncome: 240_000,
    });
  });

  it("keeps empty and invalid values safe", () => {
    expect(calculatePartnerEarnings({
      monthlyAffiliatePayout: Number.NaN,
      roiPercent: -20,
      mediaBuyers: 0,
    })).toEqual({
      trafficSpend: 0,
      twinbidProfit: 0,
      partnerIncome: 0,
      annualPartnerIncome: 0,
    });
  });
});
