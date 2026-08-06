import { describe, expect, it } from "vitest";
import { formatCurrencyAmount, formatNumberWithDot, formatStatisticInteger, formatStatisticRate, formatStatisticSpend } from "@/lib/numberFormat";

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

  it("truncates statistic rates to three decimal places", () => {
    expect(formatStatisticRate(12.9879)).toBe("12.987");
    expect(formatStatisticRate(0)).toBe("0.000");
  });

  it("truncates campaign statistics spend to two decimal places instead of rounding it", () => {
    expect(formatStatisticSpend(12.9879)).toBe("12.98");
    expect(formatStatisticSpend(0)).toBe("0.00");
  });

  it("formats a fractional balance with a decimal point and two digits", () => {
    expect(formatCurrencyAmount(39.769)).toBe("39.77");
  });
});
