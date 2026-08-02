import { describe, expect, it } from "vitest";
import { buildTrafficCleanerTargeting, selectUnconvertedSiteIds } from "@/lib/trafficCleaner";

describe("Traffic Cleaner", () => {
  it("selects unique SiteIDs without conversions above the spend threshold", () => {
    expect(selectUnconvertedSiteIds([
      { label: "site-a", spent: 3, conversions: 0 },
      { label: "site-b", spent: 12, conversions: 1 },
      { label: "site-c", spent: 10, conversions: 0 },
      { label: "site-c", spent: 11, conversions: 0 },
      { label: "site-d", spent: 9.99, conversions: 0 },
      { label: "site-e", spent: 15, conversions: 0 },
      { label: "site-f", spent: 10, conversions: 0 },
    ], 10, ["site-e"])).toEqual(["site-c"]);
  });

  it("merges selected sites into a list with the same mode", () => {
    const result = buildTrafficCleanerTargeting(
      { mode: "black", items: ["site-a", "site-b"] },
      ["site-b", "site-c"],
      "black",
    );

    expect(result).toEqual({
      next: { mode: "black", items: ["site-a", "site-b", "site-c"] },
      replacesExisting: false,
      addedCount: 1,
    });
  });

  it("does not add a SiteID that is already in the selected list", () => {
    const result = buildTrafficCleanerTargeting(
      { mode: "black", items: ["site-a"] },
      ["site-a", "site-a"],
      "black",
    );

    expect(result).toEqual({
      next: { mode: "black", items: ["site-a"] },
      replacesExisting: false,
      addedCount: 0,
    });
  });

  it("replaces an existing list when its mode changes", () => {
    const result = buildTrafficCleanerTargeting(
      { mode: "white", items: ["trusted-site"] },
      ["bad-site"],
      "black",
    );

    expect(result).toEqual({
      next: { mode: "black", items: ["bad-site"] },
      replacesExisting: true,
      addedCount: 1,
    });
  });
});
