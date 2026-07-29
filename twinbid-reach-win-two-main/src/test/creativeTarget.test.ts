import { describe, expect, it } from "vitest";
import { getTargetDims, isMediaSizeMismatch } from "@/lib/creativeTarget";

describe("creative media dimensions", () => {
  it("checks MP4 dimensions against the selected banner size", () => {
    const target = getTargetDims("banner", "300x250");

    expect(isMediaSizeMismatch(target, 300, 250)).toBe(false);
    expect(isMediaSizeMismatch(target, 1920, 1080)).toBe(true);
  });

  it("keeps the square minimum-size rule for native visuals", () => {
    const target = getTargetDims("native");

    expect(isMediaSizeMismatch(target, 400, 400)).toBe(false);
    expect(isMediaSizeMismatch(target, 199, 199)).toBe(true);
    expect(isMediaSizeMismatch(target, 400, 300)).toBe(true);
  });
});
