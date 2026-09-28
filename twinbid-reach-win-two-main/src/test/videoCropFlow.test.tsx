// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LanguageProvider } from "@/contexts/LanguageContext";
import type { Creative } from "@/contexts/CampaignContext";

const { cropMp4Video, cropAnimatedGif, readVideoMetadata } = vi.hoisted(() => ({
  cropMp4Video: vi.fn(), cropAnimatedGif: vi.fn(), readVideoMetadata: vi.fn(),
}));

vi.mock("@/lib/animatedMediaCrop", () => ({ cropMp4Video, cropAnimatedGif }));
vi.mock("@/lib/videoMetadata", async original => ({
  ...await original<typeof import("@/lib/videoMetadata")>(), readVideoMetadata,
}));

import { AutoCropConfirmDialog } from "@/components/dashboard/AutoCropConfirmDialog";
import { CreativePreviewDialog } from "@/components/dashboard/CreativePreviewDialog";

const makeCreative = (type: "video" | "gif"): Creative => ({
  id: type, name: type.toUpperCase(), url: "https://example.com", imageUrl: `blob:${type}`,
  imageFileName: `original.${type === "gif" ? "gif" : "mp4"}`,
  imageMimeType: type === "gif" ? "image/gif" : "video/mp4",
  mediaType: type === "video" ? "video" : "image",
  imageWidth: 1280, imageHeight: type === "video" ? 800 : 720, sizeMismatch: true,
  bannerSize: "300x250",
  videoFormat: "outstream",
});

describe("animated media cropping and placement preview", () => {
  afterEach(() => vi.restoreAllMocks());
  beforeEach(() => {
    window.localStorage.setItem("twinbid_lang", "en");
    cropMp4Video.mockReset();
    cropAnimatedGif.mockReset();
    readVideoMetadata.mockReset().mockResolvedValue({ duration: 2, width: 1920, height: 1080 });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: vi.fn() });
  });

  it("shows the centered MP4 area, transcodes only after confirmation, and passes fresh metadata", async () => {
    const output = new File(["mp4 data"], "cropped.mp4", { type: "video/mp4" });
    cropMp4Video.mockResolvedValue({ file: output, dataUrl: "blob:cropped", dimensions: { w: 1920, h: 1080 } });
    const onConfirm = vi.fn();
    render(<LanguageProvider><AutoCropConfirmDialog open creatives={[makeCreative("video")]}
      formatKey="video" onCancel={vi.fn()} onConfirm={onConfirm} /></LanguageProvider>);

    expect(await screen.findByText(/frame shows the area/i)).toBeInTheDocument();
    expect(cropMp4Video).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Auto-crop and save" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(cropMp4Video).toHaveBeenCalledWith("blob:video", expect.objectContaining({
      sx: 0, sy: 40, sw: 1280, sh: 720, outW: 1920, outH: 1080,
    }), "original.mp4", expect.any(Function));
    expect(onConfirm.mock.calls[0][0][0]).toMatchObject({ pendingFile: output, videoMetadata: {
      duration: 2, width: 1920, height: 1080, file_size: output.size,
    }, sizeMismatch: false });
  });

  it("offers the same centered crop for GIF rather than blocking save", async () => {
    const output = new File(["gif data"], "cropped.gif", { type: "image/gif" });
    cropAnimatedGif.mockResolvedValue({ file: output, dataUrl: "blob:gif-cropped", dimensions: { w: 300, h: 250 } });
    const onConfirm = vi.fn();
    render(<LanguageProvider><AutoCropConfirmDialog open creatives={[makeCreative("gif")]}
      formatKey="banner" onCancel={vi.fn()} onConfirm={onConfirm} /></LanguageProvider>);
    await screen.findByText(/frame shows the area/i);
    fireEvent.click(screen.getByRole("button", { name: "Auto-crop and save" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(cropAnimatedGif).toHaveBeenCalledWith("blob:gif", expect.objectContaining({
      sx: 208, sy: 0, sw: 864, sh: 720, outW: 300, outH: 250,
    }), "original.gif", expect.any(Function));
    expect(onConfirm.mock.calls[0][0][0]).toMatchObject({ pendingFile: output, sizeMismatch: false });
  });

  it("does not submit the campaign if animated cropping fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    cropMp4Video.mockRejectedValue(new Error("video-codec-unsupported"));
    const onConfirm = vi.fn();
    render(<LanguageProvider><AutoCropConfirmDialog open creatives={[makeCreative("video")]}
      formatKey="video" onCancel={vi.fn()} onConfirm={onConfirm} /></LanguageProvider>);
    await screen.findByText(/frame shows the area/i);
    fireEvent.click(screen.getByRole("button", { name: "Auto-crop and save" }));
    await waitFor(() => expect(cropMp4Video).toHaveBeenCalled());
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("shows Video placement on a page without mounting or downloading the MP4", () => {
    render(<LanguageProvider><CreativePreviewDialog open onClose={vi.fn()}
      formatKey="video" creative={makeCreative("video")} /></LanguageProvider>);
    expect(screen.getByText("Out-stream")).toBeInTheDocument();
    expect(screen.getByText("1920×1080")).toBeInTheDocument();
    expect(document.querySelector("[role=dialog] video")).toBeNull();
  });
});
