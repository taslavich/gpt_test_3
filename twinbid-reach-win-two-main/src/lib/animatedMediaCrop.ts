import { FFmpeg } from "@ffmpeg/ffmpeg";
import { fetchFile } from "@ffmpeg/util";
import ffmpegCoreUrl from "@ffmpeg/core?url";
import ffmpegWasmUrl from "@ffmpeg/core/wasm?url";
import { decompressFrames, parseGIF } from "gifuct-js";
import { GIFEncoder, applyPalette, quantize } from "gifenc";
import { buildDerivedCreativeFilename } from "@/lib/creativeApi";

const MAX_GIF_BYTES = 1 * 1024 * 1024;
const MAX_VIDEO_BYTES = 10 * 1024 * 1024;

export interface MediaCropRect {
  sx: number;
  sy: number;
  sw: number;
  sh: number;
  sourceWidth: number;
  sourceHeight: number;
  outW: number;
  outH: number;
}

interface CroppedMedia {
  file: File;
  dataUrl: string;
  dimensions: { w: number; h: number };
}

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

function putPatch(
  context: CanvasRenderingContext2D,
  patch: Uint8ClampedArray,
  width: number,
  height: number,
  left: number,
  top: number,
) {
  const patchCanvas = document.createElement("canvas");
  patchCanvas.width = width;
  patchCanvas.height = height;
  const patchContext = patchCanvas.getContext("2d");
  if (!patchContext) throw new Error("gif-patch-canvas");
  const imageData = new ImageData(patch, width, height);
  patchContext.putImageData(imageData, 0, 0);
  // drawImage alpha-composites transparent GIF pixels over the previous frame.
  context.drawImage(patchCanvas, left, top);
}

/**
 * Crops every decoded GIF frame and re-encodes it, preserving animation,
 * frame delays and the original loop behaviour.
 */
export async function cropAnimatedGif(
  sourceUrl: string,
  crop: MediaCropRect,
  fileNameHint?: string,
): Promise<CroppedMedia> {
  const bytes = await fetch(sourceUrl).then((response) => {
    if (!response.ok) throw new Error("gif-load");
    return response.arrayBuffer();
  });
  const parsed = parseGIF(bytes);
  const frames = decompressFrames(parsed, true);
  if (!frames.length) throw new Error("gif-frames");

  const sourceCanvas = document.createElement("canvas");
  sourceCanvas.width = crop.sourceWidth;
  sourceCanvas.height = crop.sourceHeight;
  const sourceContext = sourceCanvas.getContext("2d", { willReadFrequently: true });
  if (!sourceContext) throw new Error("gif-canvas");

  const outputCanvas = document.createElement("canvas");
  outputCanvas.width = crop.outW;
  outputCanvas.height = crop.outH;
  const outputContext = outputCanvas.getContext("2d", { willReadFrequently: true });
  if (!outputContext) throw new Error("gif-output-canvas");
  outputContext.imageSmoothingEnabled = true;
  outputContext.imageSmoothingQuality = "high";

  const encoder = GIFEncoder();
  let previous:
    | {
        disposalType: number;
        dims: { left: number; top: number; width: number; height: number };
        restore?: ImageData;
      }
    | undefined;

  for (const frame of frames) {
    if (previous?.disposalType === 2) {
      sourceContext.clearRect(
        previous.dims.left,
        previous.dims.top,
        previous.dims.width,
        previous.dims.height,
      );
    } else if (previous?.disposalType === 3 && previous.restore) {
      sourceContext.putImageData(previous.restore, 0, 0);
    }

    const restore = frame.disposalType === 3
      ? sourceContext.getImageData(0, 0, crop.sourceWidth, crop.sourceHeight)
      : undefined;

    putPatch(
      sourceContext,
      frame.patch,
      frame.dims.width,
      frame.dims.height,
      frame.dims.left,
      frame.dims.top,
    );

    outputContext.clearRect(0, 0, crop.outW, crop.outH);
    outputContext.drawImage(
      sourceCanvas,
      crop.sx,
      crop.sy,
      crop.sw,
      crop.sh,
      0,
      0,
      crop.outW,
      crop.outH,
    );

    const rgba = outputContext.getImageData(0, 0, crop.outW, crop.outH).data;
    const palette = quantize(rgba, 256, {
      format: "rgba4444",
      oneBitAlpha: true,
      clearAlpha: true,
    });
    const indexed = applyPalette(rgba, palette, "rgba4444");
    const transparentIndex = palette.findIndex((color) => color.length > 3 && color[3] === 0);

    encoder.writeFrame(indexed, crop.outW, crop.outH, {
      palette,
      delay: Math.max(20, frame.delay || 100),
      repeat: 0,
      transparent: transparentIndex >= 0,
      transparentIndex: transparentIndex >= 0 ? transparentIndex : 0,
      dispose: 1,
    });

    previous = {
      disposalType: frame.disposalType,
      dims: frame.dims,
      restore,
    };
  }

  encoder.finish();
  const blob = new Blob([encoder.bytes()], { type: "image/gif" });
  if (blob.size > MAX_GIF_BYTES) throw new Error("gif-too-large");

  const file = new File(
    [blob],
    buildDerivedCreativeFilename(fileNameHint, "cropped", "gif"),
    { type: "image/gif" },
  );
  return {
    file,
    dataUrl: await readFileAsDataUrl(file),
    dimensions: { w: crop.outW, h: crop.outH },
  };
}

let ffmpegPromise: Promise<FFmpeg> | null = null;

async function getFFmpeg(): Promise<FFmpeg> {
  if (!ffmpegPromise) {
    ffmpegPromise = (async () => {
      const ffmpeg = new FFmpeg();
      await ffmpeg.load({
        coreURL: ffmpegCoreUrl,
        wasmURL: ffmpegWasmUrl,
      });
      return ffmpeg;
    })().catch((error) => {
      ffmpegPromise = null;
      throw error;
    });
  }
  return ffmpegPromise;
}

function evenFloor(value: number): number {
  return Math.max(2, Math.floor(value / 2) * 2);
}

/**
 * Crops the visible frame of an MP4 without changing its time range. The full
 * video is re-encoded to MP4 with the selected crop and target dimensions.
 */
export async function cropMp4Video(
  sourceUrl: string,
  crop: MediaCropRect,
  fileNameHint?: string,
): Promise<CroppedMedia> {
  const ffmpeg = await getFFmpeg();
  const token = `${Date.now()}_${Math.random().toString(36).slice(2)}`;
  const inputName = `input_${token}.mp4`;
  const outputName = `output_${token}.mp4`;

  const sourceW = Math.max(2, Math.floor(crop.sourceWidth));
  const sourceH = Math.max(2, Math.floor(crop.sourceHeight));
  const cropW = Math.min(sourceW, evenFloor(crop.sw));
  const cropH = Math.min(sourceH, evenFloor(crop.sh));
  const cropX = Math.max(0, Math.min(Math.floor(crop.sx), sourceW - cropW));
  const cropY = Math.max(0, Math.min(Math.floor(crop.sy), sourceH - cropH));
  const outW = evenFloor(crop.outW);
  const outH = evenFloor(crop.outH);
  const filter = `crop=${cropW}:${cropH}:${cropX}:${cropY},scale=${outW}:${outH}:flags=lanczos`;

  try {
    await ffmpeg.writeFile(inputName, await fetchFile(sourceUrl));
    const exitCode = await ffmpeg.exec([
      "-i", inputName,
      "-vf", filter,
      "-c:v", "libx264",
      "-preset", "veryfast",
      "-crf", "28",
      "-pix_fmt", "yuv420p",
      "-c:a", "aac",
      "-b:a", "96k",
      "-movflags", "+faststart",
      outputName,
    ]);
    if (exitCode !== 0) throw new Error("video-encode");

    const result = await ffmpeg.readFile(outputName);
    if (typeof result === "string") throw new Error("video-output");
    const blob = new Blob([result], { type: "video/mp4" });
    if (blob.size > MAX_VIDEO_BYTES) throw new Error("video-too-large");

    const file = new File(
      [blob],
      buildDerivedCreativeFilename(fileNameHint, "cropped", "mp4"),
      { type: "video/mp4" },
    );
    return {
      file,
      dataUrl: await readFileAsDataUrl(file),
      dimensions: { w: outW, h: outH },
    };
  } finally {
    await Promise.allSettled([
      ffmpeg.deleteFile(inputName),
      ffmpeg.deleteFile(outputName),
    ]);
  }
}
