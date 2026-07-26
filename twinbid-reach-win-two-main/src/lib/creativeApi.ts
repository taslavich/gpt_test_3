import type { ApiCreative, ApiCreativeImage, ApiCreativeWrite, FormatType } from "@/api/types";

export type UiCreativeType = "image" | "html" | "iframe";

export interface CreativeDraft {
  id: string;
  name?: string;
  url: string;
  imageId?: string;
  imageUrl?: string;
  imageFileName?: string;
  imageMimeType?: string;
  mediaType?: "image" | "video";
  pendingFile?: File;
  title?: string;
  description?: string;
  creativeType?: UiCreativeType;
  htmlCode?: string;
  iframeUrl?: string;
  iframeCode?: string;
  iframeMode?: "url" | "code";
}

export interface CreativeApiClient {
  uploadCreativeImage: (campaignId: string, file: File, filename?: string) => Promise<ApiCreativeImage>;
  createCreative: (campaignId: string, body: ApiCreativeWrite) => Promise<ApiCreative>;
  patchCreative: (creativeId: string, body: Partial<ApiCreativeWrite>) => Promise<ApiCreative>;
  deleteCreative: (creativeId: string) => Promise<void>;
}

export interface CreativeDimensions {
  w: number | null;
  h: number | null;
}

export class CreativeImageUploadError extends Error {
  constructor(message: string, options?: { cause?: unknown }) {
    super(message, options);
    this.name = "CreativeImageUploadError";
  }
}

export const MAX_CREATIVE_IMAGE_BYTES = 1 * 1024 * 1024;
export const MAX_CREATIVE_VIDEO_BYTES = 10 * 1024 * 1024;

export type CreativeFileValidation =
  | { valid: true; mediaType: "image" | "video" }
  | { valid: false; reason: "format" | "image-size" | "video-size" };

export function validateCreativeFile(file: File, allowVideo: boolean): CreativeFileValidation {
  const name = file.name.toLowerCase();
  const extension = name.includes(".") ? name.slice(name.lastIndexOf(".")) : "";
  const video = allowVideo && (file.type === "video/mp4" || extension === ".mp4");
  const image =
    ["image/png", "image/jpeg", "image/jpg", "image/gif"].includes(file.type)
    || [".png", ".jpg", ".jpeg", ".gif"].includes(extension);
  if (!image && !video) return { valid: false, reason: "format" };
  if (video && file.size > MAX_CREATIVE_VIDEO_BYTES) {
    return { valid: false, reason: "video-size" };
  }
  if (image && file.size > MAX_CREATIVE_IMAGE_BYTES) {
    return { valid: false, reason: "image-size" };
  }
  return { valid: true, mediaType: video ? "video" : "image" };
}

export function isCreativeImageUploadError(error: unknown): error is CreativeImageUploadError {
  return error instanceof Error && error.name === "CreativeImageUploadError";
}

const TRACKER_MACRO_KEYS = [
  "device", "browser", "site_id", "device_os", "ip_address",
  "campaign_id", "creative_id", "country_code",
] as const;

const URL_MACRO_TOKENS = [
  "click_id", "site_id", "country_code", "creative_id",
  "campaign_id", "browser", "device", "device_os", "ip_address",
] as const;

function escapeHtmlAttribute(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/"/g, "&quot;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function decodeHtmlAttribute(value: string): string {
  return value
    .replace(/&quot;/g, "\"")
    .replace(/&#39;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&amp;/g, "&");
}

export function stripMacrosFromUrl(url: string | undefined): string {
  if (!url) return "";
  let result = url;
  for (const macro of URL_MACRO_TOKENS) {
    result = result.replace(new RegExp(`[?&]${macro}=\\{${macro}\\}`, "g"), "");
  }
  if (!result.includes("?")) {
    const ampIndex = result.indexOf("&");
    if (ampIndex !== -1) {
      result = `${result.slice(0, ampIndex)}?${result.slice(ampIndex + 1)}`;
    }
  }
  return result.replace(/[?&]+$/, "");
}

export function extractMacrosFromUrl(url: string | undefined): Record<string, boolean> {
  return Object.fromEntries(
    TRACKER_MACRO_KEYS
      .filter((macro) => !!url?.includes(`{${macro}}`))
      .map((macro) => [macro, true]),
  );
}

export function buildUrlWithMacros(
  cleanUrl: string | undefined,
  macros: Record<string, boolean> | undefined,
): string {
  let url = cleanUrl || "";
  for (const macro of TRACKER_MACRO_KEYS) {
    if (macros?.[macro] !== true || url.includes(`{${macro}}`) || !url.trim()) continue;
    url += `${url.includes("?") ? "&" : "?"}${macro}={${macro}}`;
  }
  return url;
}

export function buildIframeAdm(url: string, w: number, h: number, title = "Advertisement"): string {
  return `<iframe src="${escapeHtmlAttribute(url.trim())}" width="${w}" height="${h}" frameborder="0" scrolling="no" style="border:0;overflow:hidden" title="${escapeHtmlAttribute(title)}"></iframe>`;
}

export function extractIframeSrc(adm: string | undefined): string {
  const match = adm?.match(/<iframe[^>]*\ssrc\s*=\s*["']([^"']+)["']/i);
  return match ? decodeHtmlAttribute(match[1]) : "";
}

export function isValidCreativeUrl(value: string | undefined): boolean {
  if (!value?.trim()) return false;
  try {
    const parsed = new URL(value.trim());
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

export function isInsecureHttpUrl(value: string | undefined): boolean {
  if (!value?.trim()) return false;
  try {
    return new URL(value.trim()).protocol === "http:";
  } catch {
    return false;
  }
}

export function hasInsecureHttpReference(html: string | undefined): boolean {
  return /\bhttp:\/\//i.test(html || "");
}

export function extractBannerTargetUrl(adm: string | undefined): string {
  const match = adm?.match(/<a[^>]*\shref\s*=\s*["']([^"']+)["']/i);
  return match ? decodeHtmlAttribute(match[1]) : "";
}

export function isVideoAsset(mimeType?: string | null, filename?: string | null): boolean {
  return mimeType?.toLowerCase() === "video/mp4" || /\.mp4$/i.test(filename || "");
}

export function normalizedUploadMimeType(file: File): "image/jpg" | "image/gif" | "image/png" | "video/mp4" {
  const lowerName = file.name.toLowerCase();
  if (file.type === "video/mp4" || lowerName.endsWith(".mp4")) return "video/mp4";
  if (file.type === "image/gif" || lowerName.endsWith(".gif")) return "image/gif";
  if (file.type === "image/png" || lowerName.endsWith(".png")) return "image/png";
  return "image/jpg";
}

export function sanitizeCreativeFilename(filename: string | undefined): string {
  const leaf = (filename || "image")
    .replace(/\\/g, "/")
    .split("/")
    .pop()
    ?.trim() || "image";
  const dotIndex = leaf.lastIndexOf(".");
  const rawBase = dotIndex > 0 ? leaf.slice(0, dotIndex) : leaf;
  const rawExtension = dotIndex > 0 ? leaf.slice(dotIndex + 1) : "";
  const base = rawBase
    .normalize("NFKD")
    .replace(/[\s\-()[\]{}]+/g, "_")
    .replace(/[^a-zA-Z0-9_]/g, "_")
    .replace(/_+/g, "_")
    .replace(/^_+|_+$/g, "") || "image";
  const extension = rawExtension.toLowerCase().replace(/[^a-z0-9]/g, "");
  return extension ? `${base}.${extension}` : base;
}

export function buildDerivedCreativeFilename(
  filename: string | undefined,
  suffix: "cropped" | "autocrop",
  extension: string,
): string {
  const safeSource = sanitizeCreativeFilename(filename);
  const base = safeSource.replace(/\.[^.]+$/, "");
  return sanitizeCreativeFilename(`${base}_${suffix}.${extension}`);
}

/**
 * The multipart part Content-Type is the format consumed by the backend.
 * Re-wrap browser JPEG files because browsers normally expose `image/jpeg`
 * while the TwinBid upload contract expects `image/jpg`.
 */
export function normalizeCreativeUploadFile(file: File): File {
  const mimeType = normalizedUploadMimeType(file);
  const filename = sanitizeCreativeFilename(file.name);
  if (file.type === mimeType && file.name === filename) return file;
  return new File([file], filename, {
    type: mimeType,
    lastModified: file.lastModified,
  });
}

export function buildBannerMediaAdm(
  targetUrl: string,
  mediaUrl: string,
  w: number,
  h: number,
  video: boolean,
): string {
  const href = escapeHtmlAttribute(targetUrl);
  const src = escapeHtmlAttribute(mediaUrl);
  const media = video
    ? `<video src="${src}" width="${w}" height="${h}" autoplay muted loop playsinline style="display:block;border:0"></video>`
    : `<img src="${src}" width="${w}" height="${h}" alt="" style="display:block;border:0">`;
  return `<a href="${href}" target="_blank" rel="noopener noreferrer">${media}</a>`;
}

export function creativeRequiresImage(format: string, creative: CreativeDraft): boolean {
  return format === "native"
    || format === "push"
    || (format === "banner" && (creative.creativeType || "image") === "image");
}

export function isCreativeReadyForCreate(format: string, creative: CreativeDraft): boolean {
  if (!creative.name?.trim()) return false;
  if (format === "banner") {
    const type = creative.creativeType || "image";
    if (type === "html") return !!creative.htmlCode?.trim();
    if (type === "iframe") {
      const iframeUrl = creative.iframeMode === "code"
        ? extractIframeSrc(creative.iframeCode)
        : creative.iframeUrl;
      return isValidCreativeUrl(iframeUrl);
    }
    return !!creative.pendingFile && !!creative.url.trim();
  }
  if (format === "popunder") return !!creative.url.trim();
  return !!creative.pendingFile && !!creative.url.trim();
}

function withImageId(body: ApiCreativeWrite, imageId: string | null | undefined): ApiCreativeWrite {
  if (imageId !== undefined) body.image_id = imageId;
  return body;
}

export function buildCreativeWriteBody({
  format,
  creative,
  dimensions,
  imageId,
  imageUrl,
  imageMimeType,
}: {
  format: FormatType | string;
  creative: CreativeDraft;
  dimensions: CreativeDimensions;
  imageId?: string | null;
  imageUrl?: string;
  imageMimeType?: string;
}): ApiCreativeWrite {
  const base: ApiCreativeWrite = {
    creative_name: creative.name?.trim() || "",
    adm: "",
    trackers_macros: {},
  };

  if (format === "banner") {
    const w = dimensions.w;
    const h = dimensions.h;
    if (!w || !h) throw new Error("Banner size is required");
    base.w = w;
    base.h = h;

    const type = creative.creativeType || "image";
    if (type === "html") {
      base.adm = creative.htmlCode || "";
      base.banner_type = "iframe";
      return withImageId(base, imageId);
    }
    if (type === "iframe") {
      base.adm = creative.iframeMode === "code"
        ? creative.iframeCode?.trim() || ""
        : buildIframeAdm(creative.iframeUrl || "", w, h, creative.name || "Advertisement");
      base.banner_type = "iframe";
      return withImageId(base, imageId);
    }

    if (!imageUrl) throw new Error("Creative image is missing");
    base.adm = buildBannerMediaAdm(
      stripMacrosFromUrl(creative.url),
      imageUrl,
      w,
      h,
      isVideoAsset(imageMimeType, creative.imageFileName),
    );
    base.banner_type = "img";
    base.trackers_macros = extractMacrosFromUrl(creative.url);
    return withImageId(base, imageId);
  }

  base.adm = stripMacrosFromUrl(creative.url);
  base.trackers_macros = extractMacrosFromUrl(creative.url);
  if (format === "native" || format === "push") {
    base.title = creative.title?.trim() || "";
    base.description = creative.description?.trim() || "";
    return withImageId(base, imageId);
  }
  return base;
}

function normalizeComparable(value: unknown): unknown {
  if (value === undefined) return null;
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, child]) => [key, normalizeComparable(child)]),
    );
  }
  return value;
}

export function creativePatchChanged(existing: ApiCreative, patch: Partial<ApiCreativeWrite>): boolean {
  return Object.entries(patch).some(([key, nextValue]) => {
    const currentValue = existing[key as keyof ApiCreative];
    return JSON.stringify(normalizeComparable(currentValue)) !== JSON.stringify(normalizeComparable(nextValue));
  });
}

async function uploadImage(
  client: CreativeApiClient,
  campaignId: string,
  creative: CreativeDraft,
): Promise<ApiCreativeImage> {
  if (!creative.pendingFile) throw new Error("Creative image file is required");
  try {
    const file = normalizeCreativeUploadFile(creative.pendingFile);
    return await client.uploadCreativeImage(
      campaignId,
      file,
      sanitizeCreativeFilename(creative.imageFileName || file.name),
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : "Failed to upload creative image";
    throw new CreativeImageUploadError(message, { cause: error });
  }
}

export async function createCampaignCreatives({
  client,
  campaignId,
  format,
  dimensions,
  creatives,
  skipIncomplete = false,
}: {
  client: CreativeApiClient;
  campaignId: string;
  format: FormatType | string;
  dimensions: CreativeDimensions;
  creatives: CreativeDraft[];
  skipIncomplete?: boolean;
}): Promise<ApiCreative[]> {
  const created: ApiCreative[] = [];
  for (const creative of creatives) {
    if (skipIncomplete && !isCreativeReadyForCreate(format, creative)) continue;
    let uploaded: ApiCreativeImage | undefined;
    if (creativeRequiresImage(format, creative)) {
      uploaded = await uploadImage(client, campaignId, creative);
    }
    const body = buildCreativeWriteBody({
      format,
      creative,
      dimensions,
      imageId: uploaded?.image_id,
      imageUrl: uploaded?.image_url,
      imageMimeType: uploaded?.mime_type || uploaded?.file_format,
    });
    created.push(await client.createCreative(campaignId, body));
  }
  return created;
}

export async function syncCampaignCreatives({
  client,
  campaignId,
  format,
  dimensions,
  creatives,
  existing,
}: {
  client: CreativeApiClient;
  campaignId: string;
  format: FormatType | string;
  dimensions: CreativeDimensions;
  creatives: CreativeDraft[];
  existing: ApiCreative[];
}): Promise<void> {
  const existingById = new Map(existing.map((creative) => [creative.id, creative]));
  const retainedIds = new Set<string>();

  // Create and update first, so a campaign never temporarily loses all creatives.
  for (const creative of creatives) {
    const current = existingById.get(creative.id);
    if (!current) {
      await createCampaignCreatives({
        client,
        campaignId,
        format,
        dimensions,
        creatives: [creative],
      });
      continue;
    }
    retainedIds.add(current.id);

    let uploaded: ApiCreativeImage | undefined;
    if (creative.pendingFile && creativeRequiresImage(format, creative)) {
      uploaded = await uploadImage(client, campaignId, creative);
    }

    const type = format === "banner" ? (creative.creativeType || "image") : "image";
    const switchingAwayFromImage =
      format === "banner"
      && type !== "image"
      && current.banner_type === "img";
    const imageId = uploaded?.image_id ?? (switchingAwayFromImage ? null : undefined);
    const imageUrl = uploaded?.image_url || current.image_url || creative.imageUrl;
    const imageMimeType =
      uploaded?.mime_type
      || uploaded?.file_format
      || current.mime_type
      || creative.imageMimeType;

    const patch = buildCreativeWriteBody({
      format,
      creative,
      dimensions,
      imageId,
      imageUrl: imageUrl || undefined,
      imageMimeType: imageMimeType || undefined,
    });
    if (creativePatchChanged(current, patch)) {
      await client.patchCreative(current.id, patch);
    }
  }

  for (const current of existing) {
    if (!retainedIds.has(current.id)) {
      await client.deleteCreative(current.id);
    }
  }
}
