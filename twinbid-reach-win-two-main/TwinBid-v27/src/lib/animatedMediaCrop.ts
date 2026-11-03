import { ALL_FORMATS, BlobSource, BufferTarget, Conversion, Input, Mp4OutputFormat, Output, Quality } from "mediabunny";
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
  onProgress?: (percent: number) => void,
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

  for (const [index, frame] of frames.entries()) {
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
    if (index % 8 === 7) {
      onProgress?.(Math.round(((index + 1) / frames.length) * 100));
      await new Promise<void>(resolve => setTimeout(resolve, 0));
    }
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
    dataUrl: URL.createObjectURL(file),
    dimensions: { w: crop.outW, h: crop.outH },
  };
}

/**
 * Decode/encode with the browser's media codecs. Audio is copied when possible;
 * no external FFmpeg core download or software-only 1080p transcoding is needed.
 */
export async function cropMp4Video(
  sourceUrl: string,
  crop: MediaCropRect,
  fileNameHint?: string,
  onProgress?: (percent: number) => void,
): Promise<CroppedMedia> {
  if (typeof VideoEncoder === "undefined" || typeof VideoDecoder === "undefined") {
    throw new Error("video-codec-unsupported");
  }
  const response = await fetch(sourceUrl);
  if (!response.ok) throw new Error("video-load");
  const input = new Input({ formats: ALL_FORMATS, source: new BlobSource(await response.blob()) });
  const videoTrack = await input.getPrimaryVideoTrack();
  if (!videoTrack) throw new Error("video-track-missing");
  const audioTrack = await input.getPrimaryAudioTrack();
  const target = new BufferTarget();
  const output = new Output({ format: new Mp4OutputFormat(), target });
  const conversion = await Conversion.init({
    input, output, tracks: "primary",
    video: {
      crop: { left: crop.sx, top: crop.sy, width: crop.sw, height: crop.sh },
      width: crop.outW, height: crop.outH, fit: "fill", codec: "avc",
      quality: new Quality("medium"), hardwareAcceleration: "no-preference",
      allowTransformationMetadata: false,
    },
  });
  if (!conversion.isValid || !conversion.utilizedTracks.includes(videoTrack)
    || (audioTrack && !conversion.utilizedTracks.includes(audioTrack))) {
    throw new Error("video-codec-unsupported");
  }
  conversion.onProgress = value => onProgress?.(Math.round(value * 100));
  try {
    await conversion.execute();
  } catch (error) {
    if (error instanceof Error && /encoder configuration|codec.*unsupported|not supported in this environment/i.test(error.message)) {
      throw new Error("video-codec-unsupported");
    }
    throw error;
  }
  if (!target.buffer) throw new Error("video-output");
  const blob = new Blob([target.buffer], { type: "video/mp4" });
  if (blob.size > MAX_VIDEO_BYTES) throw new Error("video-too-large");
  const file = new File([blob], buildDerivedCreativeFilename(fileNameHint, "cropped", "mp4"), { type: "video/mp4" });
  return { file, dataUrl: URL.createObjectURL(file), dimensions: { w: crop.outW, h: crop.outH } };
}
