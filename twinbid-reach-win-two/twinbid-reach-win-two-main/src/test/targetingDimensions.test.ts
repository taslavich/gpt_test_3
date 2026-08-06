import { describe, expect, it } from "vitest";
import { COUNTRIES, LANGUAGES } from "@/lib/dimensions";
import {
  formatTargetingDimensionLabel,
  getTargetingDimensionOptions,
} from "@/lib/targetingDimensions";

describe("shared campaign and calculator targeting dimensions", () => {
  it("includes every country with a localized name and code", () => {
    const options = getTargetingDimensionOptions("country", "ru");

    expect(options).toHaveLength(COUNTRIES.length);
    expect(options).toContainEqual({
      value: "GB",
      label: "Великобритания (GB)",
    });
  });

  it("includes every language with a localized name and code", () => {
    const options = getTargetingDimensionOptions("language", "ru");

    expect(options).toHaveLength(LANGUAGES.length);
    expect(options).toContainEqual({
      value: "en",
      label: "Английский (en)",
    });
  });

  it("uses the same formatter for campaign and calculator labels", () => {
    expect(formatTargetingDimensionLabel("GB", "en")).toBe("United Kingdom (GB)");
    expect(formatTargetingDimensionLabel("en", "es")).toBe("Inglés (en)");
  });
});
