import { describe, expect, it } from "vitest";
import {
  detectCropMediaKindFromBytes,
  inferCropMediaKind,
} from "@/lib/cropMediaKind";

describe("animated crop media detection", () => {
  it("does not send GIF and MP4 data URLs through the static image cropper", () => {
    expect(inferCropMediaKind("data:image/gif;base64,R0lGODlh", "creative", "image")).toBe("gif");
    expect(inferCropMediaKind("data:video/mp4;base64,AAAA", "creative", "image")).toBe("video");
  });

  it("recognizes animated media by filename even with URL query parameters", () => {
    expect(inferCropMediaKind("https://cdn.test/file?token=1", "banner.GIF", "image")).toBe("gif");
    expect(inferCropMediaKind("https://cdn.test/video.mp4?token=1", undefined, "image")).toBe("video");
  });

  it("recognizes GIF and MP4 from their binary signatures", () => {
    expect(detectCropMediaKindFromBytes(new Uint8Array([
      0x47, 0x49, 0x46, 0x38, 0x39, 0x61,
    ]))).toBe("gif");
    expect(detectCropMediaKindFromBytes(new Uint8Array([
      0, 0, 0, 24, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6f, 0x6d,
    ]))).toBe("video");
  });
});
