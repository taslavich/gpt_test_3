import { describe, expect, it } from "vitest";
import { formatNumberWithDot, formatStatisticInteger } from "@/lib/numberFormat";

describe("statistics number formatting", () => {
  it("always uses a dot as the decimal separator", () => {
    expect(formatNumberWithDot(1234567.89, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })).toBe("1\u00a0234\u00a0567.89");
  });

  it("keeps billion-scale impression counts readable", () => {
    expect(formatStatisticInteger(9876543210)).toBe("9\u00a0876\u00a0543\u00a0210");
  });
});
