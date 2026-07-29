import type { CropperTarget } from "@/components/dashboard/ImageCropperDialog";

export const BANNER_SIZES = ["300x100", "300x250", "300x600", "728x90"] as const;

export function isMediaSizeMismatch(
  target: CropperTarget | null,
  mediaWidth: number,
  mediaHeight: number,
): boolean {
  if (!target) return false;
  if (target.mode === "fixed") {
    return mediaWidth !== target.w || mediaHeight !== target.h;
  }
  return mediaWidth !== mediaHeight || mediaWidth < (target.minSide ?? 200);
}

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
