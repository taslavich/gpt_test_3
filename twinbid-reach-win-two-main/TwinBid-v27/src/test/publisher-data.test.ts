import { describe, expect, it } from "vitest";
import {
  comparePublisherMoney,
  formatPublisherMoney,
  normalizePublisherAmount,
  publisherPeriod,
  publisherTimezone,
  publisherTimezoneLabel,
  publisherDate,
} from "@/lib/publisherData";
describe("Exact Publisher money and timezone boundaries", () => {
  it("displays UTC notation without changing the selected timezone or instant", () => {
    const instant = new Date("2026-10-27T12:00:00Z");
    expect(publisherTimezoneLabel("Etc/GMT-3", instant)).toBe("UTC+3");
    expect(publisherTimezoneLabel("Asia/Kolkata", instant)).toBe("UTC+5:30");
    expect(publisherTimezoneLabel("Etc/GMT+5", instant)).toBe("UTC-5");
    expect(publisherDate(instant.toISOString(), "ru", "Etc/GMT-3")).toContain("15:00 UTC+3");
    expect(publisherDate(instant.toISOString(), "ru")).toContain("12:00 UTC");
  });
  it("preserves precision beyond Number, including small exponential decimals", () => {
    expect(formatPublisherMoney("98765432109876543210.12345678", "en")).toBe(
      "98,765,432,109,876,543,210.12345678",
    );
    expect(
      comparePublisherMoney(
        "9007199254740993.00000001",
        "9007199254740993.00000000",
      ),
    ).toBe(1);
    expect(formatPublisherMoney("1e-8", "en")).toBe("0.00000001");
    expect(normalizePublisherAmount(" 050,12000000 ")).toBe("50.12");
    for (const invalid of ["0", "-1", "1e3", "1.123456789", "NaN"])
      expect(() => normalizePublisherAmount(invalid)).toThrow();
  });
  it("uses an exclusive UTC boundary and respects DST instead of inventing minute buckets", () => {
    expect(publisherTimezone("utc_3")).toBe("Etc/GMT-3");
    expect(publisherTimezone("utc_m5")).toBe("Etc/GMT+5");
    expect(publisherTimezone("utc_5_30")).toBe("Asia/Kolkata");
    expect(publisherPeriod("2026-10-14", "2026-10-14", "Asia/Kolkata")).toEqual(
      { from: "2026-10-13T18:30:00.000Z", to: "2026-10-14T18:30:00.000Z" },
    );
    expect(
      publisherPeriod("2026-03-29", "2026-03-29", "Europe/Berlin"),
    ).toEqual({
      from: "2026-03-28T23:00:00.000Z",
      to: "2026-03-29T22:00:00.000Z",
    });
    expect(() => publisherPeriod("2026-01-01", "2027-01-02", "UTC")).toThrow();
    expect(() => publisherTimezone("invalid")).toThrow();
  });
});
