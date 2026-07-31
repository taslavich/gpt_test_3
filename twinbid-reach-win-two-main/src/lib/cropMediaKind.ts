export type CropMediaKind = "image" | "gif" | "video";

function sourceExtension(value: string | undefined): string {
  if (!value) return "";
  const clean = value.split(/[?#]/, 1)[0].toLowerCase();
  const dot = clean.lastIndexOf(".");
  return dot >= 0 ? clean.slice(dot) : "";
}

function dataUrlMime(value: string): string {
  const match = value.match(/^data:([^;,]+)/i);
  return match?.[1]?.toLowerCase() || "";
}

function isDefinitelyStaticImage(sourceUrl: string, fileName: string | undefined): boolean {
  const mime = dataUrlMime(sourceUrl);
  const extension = sourceExtension(fileName) || sourceExtension(sourceUrl);
  return ["image/png", "image/jpeg", "image/jpg"].includes(mime)
    || [".png", ".jpg", ".jpeg"].includes(extension);
}

/**
 * Resolve the media kind from metadata that is available synchronously.
 * The source MIME/extension wins over a stale declared `image` flag.
 */
export function inferCropMediaKind(
  sourceUrl: string,
  fileName: string | undefined,
  declared: CropMediaKind | undefined,
): CropMediaKind {
  const mime = dataUrlMime(sourceUrl);
  const extension = sourceExtension(fileName) || sourceExtension(sourceUrl);

  if (mime === "image/gif" || extension === ".gif") return "gif";
  if (mime === "video/mp4" || extension === ".mp4") return "video";
  if (declared === "gif" || declared === "video") return declared;
  return "image";
}

export function detectCropMediaKindFromBytes(
  bytes: Uint8Array,
  contentType = "",
): Exclude<CropMediaKind, "image"> | null {
  const mime = contentType.toLowerCase().split(";", 1)[0].trim();
  if (mime === "image/gif") return "gif";
  if (mime === "video/mp4") return "video";

  const gif = bytes.length >= 6
    && bytes[0] === 0x47
    && bytes[1] === 0x49
    && bytes[2] === 0x46
    && bytes[3] === 0x38
    && (bytes[4] === 0x37 || bytes[4] === 0x39)
    && bytes[5] === 0x61;
  if (gif) return "gif";

  const mp4 = bytes.length >= 12
    && bytes[4] === 0x66
    && bytes[5] === 0x74
    && bytes[6] === 0x79
    && bytes[7] === 0x70;
  return mp4 ? "video" : null;
}

/**
 * Before using the static canvas path, inspect the actual source bytes. This
 * protects animated assets whose filename or MIME metadata was lost by an API.
 */
export async function resolveCropMediaKind(
  sourceUrl: string,
  fileName: string | undefined,
  declared: CropMediaKind | undefined,
): Promise<CropMediaKind> {
  const inferred = inferCropMediaKind(sourceUrl, fileName, declared);
  if (inferred !== "image") return inferred;
  if (isDefinitelyStaticImage(sourceUrl, fileName)) return inferred;

  try {
    const response = await fetch(sourceUrl);
    if (!response.ok) return inferred;
    const contentType = response.headers.get("content-type") || "";
    const bytes = new Uint8Array(await response.arrayBuffer());
    return detectCropMediaKindFromBytes(bytes.subarray(0, 16), contentType) || inferred;
  } catch {
    return inferred;
  }
}
