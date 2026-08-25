import { beforeEach, describe, expect, it } from "vitest";
import {
  capturePartnerCodeFromUrl,
  createPartnerLinkFromCode,
  generatePartnerId,
  getStoredPartnerCode,
  normalizePartnerCode,
} from "@/lib/partners";
import { captureUtmSourceFromUrl, getStoredUtmSource } from "@/lib/utmSource";

describe("TwinBid Partners attribution", () => {
  beforeEach(() => {
    localStorage.clear();
    window.history.replaceState({}, "", "/");
  });

  it("creates short random partner ids with the TB prefix", () => {
    const first = generatePartnerId();
    const second = generatePartnerId();

    expect(first).toMatch(/^TB[A-Z0-9]{10}$/);
    expect(second).toMatch(/^TB[A-Z0-9]{10}$/);
    expect(first).not.toBe(second);
  });

  it("builds a partner URL for the supplied origin", () => {
    const partnerId = "TBABC123XYZ";
    expect(createPartnerLinkFromCode(partnerId, "https://twinbid.io/"))
      .toBe(`https://twinbid.io/?partner=${partnerId}`);
  });

  it("captures partner independently from the marketing source", () => {
    window.history.replaceState({}, "", "/?partner=TBABC123XYZ&utm_source=telegram_campaign");
    capturePartnerCodeFromUrl();
    captureUtmSourceFromUrl();

    expect(getStoredPartnerCode()).toBe("TBABC123XYZ");
    expect(getStoredUtmSource()).toBe("telegram_campaign");
  });

  it("rejects malformed partner codes", () => {
    expect(normalizePartnerCode("bad code<script>")).toBeNull();
  });
});
