import type { Creative } from "@/contexts/CampaignContext";
import type { CropperTarget } from "@/components/dashboard/ImageCropperDialog";

const MAX_BYTES = 1 * 1024 * 1024;

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error("image load"));
    img.src = src;
  });
}

/** Compute source crop rect (center-cover) for the target. */
function computeCoverCrop(sw: number, sh: number, target: CropperTarget) {
  if (target.mode === "fixed") {
    const targetAspect = target.w / target.h;
    const srcAspect = sw / sh;
    let cw: number, ch: number;
    if (srcAspect > targetAspect) {
      ch = sh;
      cw = ch * targetAspect;
    } else {
      cw = sw;
      ch = cw / targetAspect;
    }
    const cx = (sw - cw) / 2;
    const cy = (sh - ch) / 2;
    return { sx: cx, sy: cy, sw: cw, sh: ch, outW: target.w, outH: target.h };
  }
  // square-resizable: centered square
  const side = Math.min(sw, sh);
  const outSide = Math.max(target.minSide ?? 200, Math.round(side));
  const cx = (sw - side) / 2;
  const cy = (sh - side) / 2;
  return { sx: cx, sy: cy, sw: side, sh: side, outW: outSide, outH: outSide };
}

export function isGifDataUrl(src: string): boolean {
  return src.startsWith("data:image/gif");
}

export function isGifFileName(name?: string): boolean {
  return !!name && /\.gif$/i.test(name);
}

export async function autoCropImage(
  dataUrl: string,
  target: CropperTarget,
  fileNameHint?: string,
): Promise<{ dataUrl: string; file: File }> {
  const img = await loadImage(dataUrl);
  const crop = computeCoverCrop(img.naturalWidth, img.naturalHeight, target);
  const canvas = document.createElement("canvas");
  canvas.width = crop.outW;
  canvas.height = crop.outH;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("canvas");
  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = "high";
  ctx.drawImage(img, crop.sx, crop.sy, crop.sw, crop.sh, 0, 0, crop.outW, crop.outH);

  const toBlob = (type: string, q?: number) =>
    new Promise<Blob | null>(res => canvas.toBlob(res, type, q));

  let blob = await toBlob("image/png");
  let ext = "png", mime = "image/png";
  if (!blob || blob.size > MAX_BYTES) {
    blob = await toBlob("image/jpeg", 0.9);
    ext = "jpg"; mime = "image/jpeg";
  }
  if (!blob) throw new Error("blob");
  if (blob.size > MAX_BYTES) {
    // reduce quality further
    blob = await toBlob("image/jpeg", 0.75);
    if (!blob || blob.size > MAX_BYTES) throw new Error("too large");
  }
  const base = (fileNameHint || "image").replace(/\.[^.]+$/, "");
  const file = new File([blob], `${base}-autocrop.${ext}`, { type: mime });
  const outUrl: string = await new Promise((res, rej) => {
    const r = new FileReader();
    r.onload = () => res(String(r.result));
    r.onerror = () => rej(r.error);
    r.readAsDataURL(file);
  });
  return { dataUrl: outUrl, file };
}

/**
 * Auto-crop all mismatched non-GIF creatives. GIFs are skipped (return as-is).
 * Returns new creatives array + count of successfully cropped.
 */
export async function autoCropMismatched(
  creatives: Creative[],
  target: CropperTarget | null,
): Promise<{ creatives: Creative[]; cropped: number; skipped: number }> {
  if (!target) return { creatives, cropped: 0, skipped: 0 };
  let cropped = 0, skipped = 0;
  const out = await Promise.all(creatives.map(async c => {
    if (!c.sizeMismatch || !c.imageUrl) return c;
    if (isGifDataUrl(c.imageUrl) || isGifFileName(c.imageFileName)) {
      skipped++;
      return c;
    }
    try {
      const { dataUrl, file } = await autoCropImage(c.imageUrl, target, c.imageFileName);
      cropped++;
      return { ...c, imageUrl: dataUrl, pendingFile: file, imageFileName: file.name, sizeMismatch: false };
    } catch {
      skipped++;
      return c;
    }
  }));
  return { creatives: out, cropped, skipped };
}
