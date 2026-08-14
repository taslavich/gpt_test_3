import { describe, expect, it } from "vitest";
import { getBidLimits, getMaximumBid } from "@/lib/bidLimits";

describe("campaign bid limits", () => {
  it("uses the same 0.05 CPM minimum for every Popunder traffic quality", () => {
    expect(getBidLimits("popunder", "common", "cpm")).toEqual({
      min: 0.05,
      recommendationMin: 0.3,
      recommended: 1.8,
    });
    expect(getBidLimits("popunder", "high", "cpm")).toEqual({
      min: 0.05,
      recommendationMin: 0.7,
      recommended: 3,
    });
    expect(getBidLimits("popunder", "ultra", "cpm")).toEqual({
      min: 0.05,
      recommendationMin: 0.9,
      recommended: 4.7,
    });
  });

  it("converts the new Popunder minimum to CPC but keeps the old recommendation inputs", () => {
    expect(getBidLimits("popunder", "common", "cpc")).toEqual({
      min: 0.00009,
      recommendationMin: 0.00051,
      recommended: 0.00306,
    });
  });

  it("keeps Push limits in CPC without a second conversion", () => {
    expect(getBidLimits("push", "common", "cpc")).toEqual({
      min: 0.005,
      recommendationMin: 0.005,
      recommended: 0.01,
    });
  });

  it("uses the campaign-form maximums in the calculator", () => {
    expect(getMaximumBid("popunder", "cpm")).toBe(50);
    expect(getMaximumBid("banner", "cpm")).toBe(1000);
    expect(getMaximumBid("popunder", "cpc")).toBe(1);
  });
});
