import { describe, expect, it } from "vitest";
import { createBannerSizeVariants } from "@/lib/bannerCreativeVariants";

describe("banner creative variants", () => {
  it("creates every supported size with sequential names and independent IDs", () => {
    let id = 0;
    const variants = createBannerSizeVariants(
      {
        id: "original",
        name: "Original",
        bannerSize: undefined,
        imageUrl: "data:image/png;base64,preview",
      },
      {
        startIndex: 2,
        creativeLabel: "Creative",
        sourceWidth: 300,
        sourceHeight: 250,
        createId: () => `generated-${++id}`,
      },
    );

    expect(variants.map(variant => variant.bannerSize)).toEqual([
      "300x100",
      "300x250",
      "300x600",
      "728x90",
    ]);
    expect(variants.map(variant => variant.name)).toEqual([
      "Creative 3",
      "Creative 4",
      "Creative 5",
      "Creative 6",
    ]);
    expect(variants.map(variant => variant.id)).toEqual([
      "original",
      "generated-1",
      "generated-2",
      "generated-3",
    ]);
    expect(variants.map(variant => variant.sizeMismatch)).toEqual([
      true,
      false,
      true,
      true,
    ]);
    expect(variants.every(variant => variant.imageUrl === "data:image/png;base64,preview")).toBe(true);
    expect(variants.every(variant => variant.allBannerSizesGenerated)).toBe(true);
  });
});
