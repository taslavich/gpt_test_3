// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { readVideoMetadata, videoMetadataFromFile } from "@/lib/videoMetadata";

afterEach(() => {
  vi.restoreAllMocks();
  delete (URL as typeof URL & { createObjectURL?: unknown }).createObjectURL;
  delete (URL as typeof URL & { revokeObjectURL?: unknown }).revokeObjectURL;
});

function fakeBrowserMetadata(width: number, height: number, duration: number) {
  const revoke = vi.fn();
  Object.defineProperty(URL, "createObjectURL", { configurable: true, value: vi.fn(() => "blob:video-test") });
  Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: revoke });
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
  const create = document.createElement.bind(document);
  vi.spyOn(document, "createElement").mockImplementation((tag: string) => {
    const element = create(tag);
    if (tag === "video") {
      Object.defineProperties(element, {
        videoWidth: { configurable: true, value: width },
        videoHeight: { configurable: true, value: height },
        duration: { configurable: true, value: duration },
        src: { configurable: true, set() { queueMicrotask(() => (element as HTMLVideoElement).onloadedmetadata?.(new Event("loadedmetadata"))); } },
      });
    }
    return element;
  });
  return revoke;
}

describe("real video metadata extraction", () => {
  it("reads dimensions and rounded positive duration and revokes the local URL", async () => {
    const revoke = fakeBrowserMetadata(1920, 1080, 14.6);
    const file = new File(["real bytes"], "creative.mp4", { type: "video/mp4" });
    const result = await readVideoMetadata(file);
    expect(result).toEqual({ width: 1920, height: 1080, duration: 15 });
    expect(videoMetadataFromFile(file, result)).toMatchObject({ duration: 15, width: 1920, height: 1080, file_size: file.size, codec: "", bitrate: 0 });
    expect(revoke).toHaveBeenCalledWith("blob:video-test");
  });

  it.each([0, Infinity, NaN])("rejects invalid duration %s and still revokes the URL", async duration => {
    const revoke = fakeBrowserMetadata(1920, 1080, duration);
    await expect(readVideoMetadata(new File(["bytes"], "bad.mp4", { type: "video/mp4" })))
      .rejects.toThrow("Cannot read video duration");
    expect(revoke).toHaveBeenCalledWith("blob:video-test");
  });

  it("rejects missing frame dimensions and revokes the URL", async () => {
    const revoke = fakeBrowserMetadata(0, 1080, 12);
    await expect(readVideoMetadata(new File(["bytes"], "bad.mp4", { type: "video/mp4" })))
      .rejects.toThrow("Cannot read video duration");
    expect(revoke).toHaveBeenCalledWith("blob:video-test");
  });
});
