import { describe, expect, it } from "vitest";
import { getBidLimits, getMaximumBid } from "@/lib/bidLimits";

describe("campaign bid limits", () => {
  it("converts Popunder CPM limits to CPC with the existing coefficient", () => {
    expect(getBidLimits("popunder", "common", "cpc")).toEqual({
      min: 0.00051,
      recommended: 0.00306,
    });
  });

  it("keeps Push limits in CPC without a second conversion", () => {
    expect(getBidLimits("push", "common", "cpc")).toEqual({
      min: 0.005,
      recommended: 0.01,
    });
  });

  it("uses the campaign-form maximums in the calculator", () => {
    expect(getMaximumBid("popunder", "cpm")).toBe(50);
    expect(getMaximumBid("banner", "cpm")).toBe(1000);
    expect(getMaximumBid("popunder", "cpc")).toBe(1);
  });
});
