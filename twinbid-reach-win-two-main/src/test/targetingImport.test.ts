import { describe, expect, it } from "vitest";
import { getImportableTargetingKeys, importTargetingGroups } from "@/lib/targetingImport";

describe("targeting import", () => {
  it("copies selected values and their white/black mode without duplicates", () => {
    const current = {
      sites: { mode: "white" as const, items: ["old"] },
      country: { mode: "white" as const, items: ["US"] },
    };
    const source = {
      sites: { mode: "black" as const, items: ["10", "20", "10"] },
      country: { mode: "black" as const, items: ["DE"] },
    };

    const result = importTargetingGroups(current, source, ["sites"]);

    expect(result.sites).toEqual({ mode: "black", items: ["10", "20"] });
    expect(result.country).toEqual(current.country);
    expect(result.sites).not.toBe(source.sites);
  });

  it("offers only non-empty targeting groups", () => {
    expect(getImportableTargetingKeys({
      sites: { mode: "black", items: ["42"] },
      browser: { mode: "none", items: [] },
      schedule: { mode: "white", items: ["monday:0"] },
    })).toEqual(["schedule", "sites"]);
  });

  it("copies a large SiteID blacklist in one operation", () => {
    const siteIds = Array.from({ length: 800 }, (_, index) => String(index + 1));
    const result = importTargetingGroups(
      { sites: { mode: "none", items: [] } },
      { sites: { mode: "black", items: siteIds } },
      ["sites"],
    );

    expect(result.sites.mode).toBe("black");
    expect(result.sites.items).toHaveLength(800);
    expect(result.sites.items).toEqual(siteIds);
  });
});
