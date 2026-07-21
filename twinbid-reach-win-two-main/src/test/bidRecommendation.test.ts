import { describe, expect, it } from "vitest";
import { buildRecommendBidRequest, resolveDisplayedBidRecommendation } from "@/lib/bidRecommendation";

describe("resolveDisplayedBidRecommendation", () => {
  it("uses the API recommendations when the returned bid reaches the active minimum", () => {
    expect(resolveDisplayedBidRecommendation({
      apiMinimumRecommended: 0.8,
      apiOptimalRecommended: 1.76,
      hardcodedMinimum: 0.3,
      hardcodedRecommended: 1.8,
    })).toEqual({
      minimumRecommended: 0.8,
      optimalRecommended: 1.76,
    });
  });

  it("falls back to the old CPM recommendation and puts the minimum recommendation halfway", () => {
    expect(resolveDisplayedBidRecommendation({
      apiMinimumRecommended: 0.2,
      apiOptimalRecommended: 0.44,
      hardcodedMinimum: 0.3,
      hardcodedRecommended: 1.8,
    })).toEqual({
      minimumRecommended: 1.05,
      optimalRecommended: 1.8,
    });
  });

  it("applies the same fallback to values already converted to CPC", () => {
    expect(resolveDisplayedBidRecommendation({
      apiMinimumRecommended: 0.00017,
      apiOptimalRecommended: 0.000374,
      hardcodedMinimum: 0.00051,
      hardcodedRecommended: 0.00306,
    })).toEqual({
      minimumRecommended: 0.001785,
      optimalRecommended: 0.00306,
    });
  });
});

describe("buildRecommendBidRequest", () => {
  it("passes site IDs and their include/exclude mode to recommend_bid", () => {
    const request = buildRecommendBidRequest("popunder", "adult", {
      sites: { mode: "black", items: ["12345", "abdjhx"] },
    });

    expect(request.site_id).toEqual(["12345", "abdjhx"]);
    expect(request.site_id_mode).toBe("exclude");
  });
});
