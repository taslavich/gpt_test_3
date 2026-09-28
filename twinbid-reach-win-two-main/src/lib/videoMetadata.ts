import type { VideoCreativeMetadata } from "@/api/types";

export type VideoDimensionsAndDuration = Pick<VideoCreativeMetadata, "width" | "height" | "duration">;

export function isValidVideoMetadata(value: VideoCreativeMetadata | null | undefined): value is VideoCreativeMetadata {
  return !!value
    && Number.isFinite(value.duration) && value.duration > 0
    && Number.isFinite(value.width) && value.width > 0
    && Number.isFinite(value.height) && value.height > 0
    && Number.isFinite(value.file_size) && value.file_size > 0
    && Array.isArray(value.mimes) && value.mimes.includes("video/mp4");
}

/** Read actual MP4 metadata; local object URLs and listeners are released on every path. */
export function readVideoMetadata(source: File | string): Promise<VideoDimensionsAndDuration> {
  const localFile = source instanceof File;
  const url = localFile ? URL.createObjectURL(source) : source;
  return new Promise((resolve, reject) => {
    const video = document.createElement("video");
    let settled = false;
    const finish = (result?: VideoDimensionsAndDuration, error?: Error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      video.onloadedmetadata = null;
      video.onerror = null;
      video.removeAttribute("src");
      video.load();
      if (localFile) URL.revokeObjectURL(url);
      if (error) reject(error);
      else if (result) resolve(result);
    };
    video.preload = "metadata";
    video.onloadedmetadata = () => {
      const { videoWidth: width, videoHeight: height, duration } = video;
      if (!Number.isFinite(width) || width <= 0 || !Number.isFinite(height) || height <= 0
        || !Number.isFinite(duration) || duration <= 0) {
        finish(undefined, new Error("Cannot read video duration or dimensions. Upload a valid MP4 file."));
      } else finish({ width, height, duration: Math.max(1, Math.round(duration)) });
    };
    video.onerror = () => finish(undefined, new Error("Cannot read video metadata. Upload a valid MP4 file."));
    const timeout = setTimeout(() => finish(undefined, new Error("Video metadata loading timed out. Upload the MP4 again.")), 20000);
    video.src = url;
  });
}

export function videoMetadataFromFile(file: File, metadata: VideoDimensionsAndDuration): VideoCreativeMetadata {
  return {
    mimes: ["video/mp4"], duration: metadata.duration, protocols: [2, 3, 7],
    api: [], battr: [], bitrate: 0, linearity: 1,
    width: metadata.width, height: metadata.height, codec: "", file_size: file.size,
  };
}

/** Old creatives may have only a permanent MP4 URL. Recover real file size as well. */
export async function recoverVideoMetadata(url: string): Promise<VideoCreativeMetadata> {
  const dimensions = await readVideoMetadata(url);
  const response = await fetch(url);
  if (!response.ok) throw new Error("Cannot download existing MP4. Upload the file again to edit this creative.");
  const file = new File([await response.blob()], "existing.mp4", { type: "video/mp4" });
  const metadata = videoMetadataFromFile(file, dimensions);
  if (!isValidVideoMetadata(metadata)) throw new Error("Cannot read existing MP4 metadata. Upload the file again.");
  return metadata;
}
