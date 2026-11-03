// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LanguageProvider } from "@/contexts/LanguageContext";

const { cropMp4Video, cropAnimatedGif } = vi.hoisted(() => ({
  cropMp4Video: vi.fn(), cropAnimatedGif: vi.fn(),
}));
vi.mock("@/lib/animatedMediaCrop", () => ({ cropMp4Video, cropAnimatedGif }));

import { ImageCropperDialog } from "@/components/dashboard/ImageCropperDialog";

describe("animated crop editor", () => {
  beforeEach(() => {
    window.localStorage.setItem("twinbid_lang", "en");
    cropMp4Video.mockReset(); cropAnimatedGif.mockReset();
    vi.stubGlobal("ResizeObserver", class { observe() {} unobserve() {} disconnect() {} });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: vi.fn() });
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({ drawImage: vi.fn() } as unknown as CanvasRenderingContext2D);
  });

  it("plays the source in the crop frame, displays a live region, and lets the user play the encoded MP4 before applying it", async () => {
    const file = new File(["cropped video"], "cropped.mp4", { type: "video/mp4" });
    cropMp4Video.mockResolvedValue({ file, dataUrl: "blob:cropped", dimensions: { w: 1920, h: 1080 } });
    const onSave = vi.fn();
    render(<LanguageProvider><ImageCropperDialog open
      source={{ dataUrl: "blob:source", naturalWidth: 1280, naturalHeight: 720, mediaKind: "video" }}
      target={{ w: 1920, h: 1080, mode: "fixed" }} fileNameHint="original.mp4"
      onSave={onSave} onClose={vi.fn()} /></LanguageProvider>);

    expect(screen.getByText("Live preview of the selected area")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
    expect(document.querySelector('[role=dialog] video[src="blob:source"]')).not.toBeNull();
    expect(document.querySelector("[role=dialog] canvas")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Crop and preview result" }));
    await waitFor(() => expect(document.querySelector('[role=dialog] video[src="blob:cropped"][controls]')).not.toBeNull());
    expect(onSave).not.toHaveBeenCalled();
    expect(cropMp4Video).toHaveBeenCalledWith("blob:source", expect.objectContaining({
      outW: 1920, outH: 1080, sourceWidth: 1280, sourceHeight: 720,
    }), "original.mp4", expect.any(Function));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(file, "blob:cropped", { w: 1920, h: 1080 }));
  });

  it("allows GIF crop to be reviewed and changed before accepting", async () => {
    const file = new File(["cropped gif"], "cropped.gif", { type: "image/gif" });
    cropAnimatedGif.mockResolvedValue({ file, dataUrl: "blob:cropped-gif", dimensions: { w: 300, h: 250 } });
    const onSave = vi.fn();
    render(<LanguageProvider><ImageCropperDialog open
      source={{ dataUrl: "blob:source-gif", naturalWidth: 400, naturalHeight: 300, mediaKind: "gif" }}
      target={{ w: 300, h: 250, mode: "fixed" }} fileNameHint="original.gif"
      onSave={onSave} onClose={vi.fn()} /></LanguageProvider>);

    fireEvent.click(screen.getByRole("button", { name: "Crop and preview result" }));
    await screen.findByAltText("Cropped result");
    expect(onSave).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Change area" }));
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:cropped-gif");
    expect(screen.getByText("Live preview of the selected area")).toBeInTheDocument();
  });
});
