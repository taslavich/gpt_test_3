import type { CropperTarget } from "@/components/dashboard/ImageCropperDialog";

export function getTargetDims(formatKey: string, bannerSize?: string): CropperTarget | null {
  if (formatKey === "banner") {
    if (!bannerSize || !/^\d+x\d+$/.test(bannerSize)) return null;
    const [w, h] = bannerSize.split("x").map(Number);
    return { w, h, mode: "fixed" };
  }
  if (formatKey === "push") return { w: 192, h: 192, mode: "fixed" };
  if (formatKey === "native") return { w: 200, h: 200, mode: "square-resizable", minSide: 200 };
  return null;
}
