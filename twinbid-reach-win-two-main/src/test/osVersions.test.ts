import { describe, expect, it } from "vitest";
import { availableOsVersions, formatOsVersionGroup, normalizeSelectedOsVersions, normalizedOsVersionTargeting, selectableOsVersions } from "@/lib/osVersions";
import { buildRecommendBidRequest } from "@/lib/bidRecommendation";

describe("OS version targeting", () => {
  it("offers both whole branches and real minor releases from the uploaded samples", () => {
    const ios = availableOsVersions({ mode: "white", items: ["iOS"] });
    const android = availableOsVersions({ mode: "white", items: ["Android"] });
    expect(ios).toContain("iOS 18");
    expect(ios).toContain("iOS 18.7");
    expect(ios).toContain("iOS 27.2");
    expect(ios).not.toContain("iOS 19.1");
    expect(ios).not.toContain("iOS 10.15");
    expect(ios).not.toContain("iOS 26.7");
    expect(android).toContain("Android 8.1");
    expect(android).toContain("Android 12L");
    expect(android).toContain("Android 17.0");
    expect(android).not.toContain("Android 12.5");
    expect(android).not.toContain("Android 8.11");
    expect(availableOsVersions({ mode: "white", items: ["Windows"] })).toEqual([]);
    expect(availableOsVersions({ mode: "black", items: ["iOS"] })).toEqual([]);
  });

  it("removes stale or cross-OS versions before they reach create/PATCH and recommendations", () => {
    const targeting = {
      os: { mode: "white" as const, items: ["Android"] },
      osVersion: { mode: "black" as const, items: ["iOS 13.5", "Android 8.1", "Android 12.5"] },
    };
    expect(normalizedOsVersionTargeting(targeting)).toEqual({ mode: "black", items: ["Android 8.1"] });
    expect(buildRecommendBidRequest("banner", "mainstream", targeting)).toMatchObject({
      os_version: ["Android 8.1"], os_version_mode: "exclude",
    });
  });

  it("hides minor choices under a selected major and removes redundant minor targeting", () => {
    const os = { mode: "white" as const, items: ["iOS", "Android"] };
    expect(selectableOsVersions(os, ["iOS 18"]))
      .not.toContain("iOS 18.7");
    expect(selectableOsVersions(os, ["iOS 18"]))
      .toContain("iOS 17.7");
    expect(selectableOsVersions(os, ["iOS 18"]))
      .toContain("Android 8.1");
    expect(selectableOsVersions(os, ["Android 12"]))
      .toContain("Android 12L");
    expect(normalizeSelectedOsVersions(["iOS 18.5", "iOS 18.7", "iOS 18"]))
      .toEqual(["iOS 18"]);
    expect(normalizedOsVersionTargeting({ os, osVersion: { mode: "white", items: ["iOS 18", "iOS 18.7"] } }))
      .toEqual({ mode: "white", items: ["iOS 18"] });
    expect(selectableOsVersions(os, []))
      .toContain("iOS 18.7");
  });

  it("groups patch versions by the first dot digit without merging adjacent releases", () => {
    expect(formatOsVersionGroup("iOS|18.7.10")).toBe("iOS 18.7");
    expect(formatOsVersionGroup("iOS|18.3.2")).toBe("iOS 18.3");
    expect(formatOsVersionGroup("Android|8.1.0")).toBe("Android 8.1");
    expect(formatOsVersionGroup("Android|12L")).toBe("Android 12L");
    expect(formatOsVersionGroup("iOS 13.5.1")).toBe("iOS 13.5");
    expect(formatOsVersionGroup("iOS|13")).toBe("iOS 13.0");
    expect(formatOsVersionGroup("iOS|19.1.2")).toBe("iOS");
    expect(formatOsVersionGroup("Android|12.5")).toBe("Android");
    expect(formatOsVersionGroup("Windows|10")).toBe("Windows");
  });
});
