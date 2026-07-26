// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import type { ApiCreative, ApiCreativeImage, ApiCreativeWrite } from "@/api/types";
import {
  CreativeImageUploadError,
  buildDerivedCreativeFilename,
  buildUrlWithMacros,
  buildCreativeWriteBody,
  buildIframeAdm,
  createCampaignCreatives,
  creativeRequiresImage,
  extractMacrosFromUrl,
  hasInsecureHttpReference,
  isInsecureHttpUrl,
  isCreativeReadyForCreate,
  isValidCreativeUrl,
  MAX_CREATIVE_IMAGE_BYTES,
  MAX_CREATIVE_VIDEO_BYTES,
  normalizeCreativeUploadFile,
  sanitizeCreativeFilename,
  syncCampaignCreatives,
  validateCreativeFile,
  type CreativeApiClient,
  type CreativeDraft,
} from "@/lib/creativeApi";
import { mapApiCreativeToUi } from "@/contexts/CampaignContext";

function imageFile(name = "banner.jpg", type = "image/jpeg") {
  return new File(["bytes"], name, { type });
}

function baseCreative(overrides: Partial<CreativeDraft> = {}): CreativeDraft {
  return {
    id: "local-1",
    name: "Creative",
    url: "https://target.example?site_id={site_id}",
    creativeType: "image",
    pendingFile: imageFile(),
    imageFileName: "banner.jpg",
    imageMimeType: "image/jpeg",
    mediaType: "image",
    ...overrides,
  };
}

function existingCreative(overrides: Partial<ApiCreative> = {}): ApiCreative {
  return {
    id: "creative-1",
    campaign_id: "campaign-1",
    creative_name: "Creative",
    adm: '<a href="https://target.example" target="_blank" rel="noopener noreferrer"><img src="https://cdn.example/image.jpg" width="300" height="250" alt="" style="display:block;border:0"></a>',
    banner_type: "img",
    image_id: "image-1",
    image_url: "https://cdn.example/image.jpg",
    image_name: "banner.jpg",
    mime_type: "image/jpg",
    trackers_macros: {},
    w: 300,
    h: 250,
    title: null,
    description: null,
    ...overrides,
  };
}

class FakeCreativeApi implements CreativeApiClient {
  uploads: Array<{ campaignId: string; file: File; filename?: string }> = [];
  creates: Array<{ campaignId: string; body: ApiCreativeWrite }> = [];
  patches: Array<{ id: string; body: Partial<ApiCreativeWrite> }> = [];
  deletes: string[] = [];
  failUpload = false;

  async uploadCreativeImage(campaignId: string, file: File, filename?: string): Promise<ApiCreativeImage> {
    this.uploads.push({ campaignId, file, filename });
    if (this.failUpload) throw new Error("Backend rejected file");
    return {
      image_id: `uploaded-image-${this.uploads.length}`,
      campaign_id: campaignId,
      creative_id: null,
      image_url: `https://cdn.example/${filename || file.name}`,
      filename: filename || file.name,
      mime_type: file.type,
      file_format: file.type,
      size_bytes: file.size,
      created_at: "2026-07-25T00:00:00Z",
      updated_at: "2026-07-25T00:00:00Z",
    };
  }

  async createCreative(campaignId: string, body: ApiCreativeWrite): Promise<ApiCreative> {
    this.creates.push({ campaignId, body });
    return { ...body, id: `creative-${this.creates.length}`, campaign_id: campaignId };
  }

  async patchCreative(id: string, body: Partial<ApiCreativeWrite>): Promise<ApiCreative> {
    this.patches.push({ id, body });
    return { ...existingCreative({ id }), ...body };
  }

  async deleteCreative(id: string): Promise<void> {
    this.deletes.push(id);
  }
}

const dimensions = { w: 300, h: 250 };

describe("creative API migration", () => {
  it("creates banner image only after upload and uses permanent image_url in ADM", async () => {
    const client = new FakeCreativeApi();
    await createCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative()],
    });

    expect(client.uploads).toHaveLength(1);
    expect(client.uploads[0].file.type).toBe("image/jpg");
    expect(client.creates).toHaveLength(1);
    expect(client.creates[0].body).toMatchObject({
      banner_type: "img",
      image_id: "uploaded-image-1",
      trackers_macros: { site_id: true },
      w: 300,
      h: 250,
    });
    expect(client.creates[0].body.adm).toContain('href="https://target.example"');
    expect(client.creates[0].body.adm).not.toContain("{site_id}");
    expect(client.creates[0].body.adm).toContain('src="https://cdn.example/banner.jpg"');
  });

  it("replaces unsafe filename separators before creative upload", async () => {
    expect(sanitizeCreativeFilename("image (13)-cropped.png")).toBe("image_13_cropped.png");
    expect(buildDerivedCreativeFilename("image (13).png", "cropped", "png"))
      .toBe("image_13_cropped.png");
    expect(buildDerivedCreativeFilename("banner-name.jpg", "autocrop", "jpg"))
      .toBe("banner_name_autocrop.jpg");

    const client = new FakeCreativeApi();
    await createCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative({
        pendingFile: imageFile("image (13)-cropped.png", "image/png"),
        imageFileName: "image (13)-cropped.png",
        imageMimeType: "image/png",
      })],
    });

    expect(client.uploads[0].file.name).toBe("image_13_cropped.png");
    expect(client.uploads[0].filename).toBe("image_13_cropped.png");
  });

  it("uses a boolean macro map and restores only explicitly enabled macros", () => {
    expect(extractMacrosFromUrl(
      "https://target.example?site_id={site_id}&country_code={country_code}",
    )).toEqual({
      site_id: true,
      country_code: true,
    });
    expect(buildUrlWithMacros("https://target.example", {
      site_id: true,
      country_code: false,
    })).toBe("https://target.example?site_id={site_id}");
  });

  it("requires an image for a banner image creative", () => {
    const creative = baseCreative({ pendingFile: undefined });
    expect(creativeRequiresImage("banner", creative)).toBe(true);
    expect(isCreativeReadyForCreate("banner", creative)).toBe(false);
  });

  it("does not require an image for a complete banner HTML creative", () => {
    const creative = baseCreative({
      creativeType: "html",
      htmlCode: "<canvas id='ad'></canvas>",
      pendingFile: undefined,
    });
    expect(creativeRequiresImage("banner", creative)).toBe(false);
    expect(isCreativeReadyForCreate("banner", creative)).toBe(true);
  });

  it("does not require an image for a complete banner iframe creative", () => {
    const creative = baseCreative({
      creativeType: "iframe",
      iframeMode: "url",
      iframeUrl: "http://creative.example/frame",
      pendingFile: undefined,
    });
    expect(creativeRequiresImage("banner", creative)).toBe(false);
    expect(isCreativeReadyForCreate("banner", creative)).toBe(true);
  });

  it.each(["native", "push"] as const)("requires an image for %s", (format) => {
    const creative = baseCreative({ pendingFile: undefined });
    expect(creativeRequiresImage(format, creative)).toBe(true);
    expect(isCreativeReadyForCreate(format, creative)).toBe(false);
  });

  it("does not require an image for popunder", () => {
    const creative = baseCreative({ pendingFile: undefined });
    expect(creativeRequiresImage("popunder", creative)).toBe(false);
    expect(isCreativeReadyForCreate("popunder", creative)).toBe(true);
  });

  it("builds iframe code from a URL with campaign banner dimensions and does not upload", async () => {
    const client = new FakeCreativeApi();
    await createCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative({
        creativeType: "iframe",
        iframeMode: "url",
        iframeUrl: "https://creative.example/frame",
        pendingFile: undefined,
      })],
    });

    expect(client.uploads).toHaveLength(0);
    expect(client.creates[0].body.banner_type).toBe("iframe");
    expect(client.creates[0].body.adm).toBe(
      buildIframeAdm("https://creative.example/frame", 300, 250, "Creative"),
    );
  });

  it("sends banner HTML as ADM with backend iframe type", async () => {
    const html = "\n  <canvas id='ad'></canvas>\n";
    const body = buildCreativeWriteBody({
      format: "banner",
      dimensions,
      creative: baseCreative({
        creativeType: "html",
        htmlCode: html,
        pendingFile: undefined,
      }),
    });
    expect(body).toMatchObject({ banner_type: "iframe", adm: html, trackers_macros: {} });
    expect(body.image_id).toBeUndefined();
  });

  it.each([
    "<div>HTML banner</div>",
    "<canvas id='ad'></canvas>",
    "<svg><rect width='10' height='10'/></svg>",
    "<video src='/banner.mp4'></video>",
  ])("accepts non-empty HTML without requiring an img tag: %s", (html) => {
    expect(isCreativeReadyForCreate("banner", baseCreative({
      creativeType: "html",
      htmlCode: html,
      pendingFile: undefined,
    }))).toBe(true);
  });

  it("rejects only empty HTML", () => {
    expect(isCreativeReadyForCreate("banner", baseCreative({
      creativeType: "html",
      htmlCode: "   ",
      pendingFile: undefined,
    }))).toBe(false);
  });

  it.each([
    ["http://creative.example/frame", true],
    ["HTTP://creative.example/frame", true],
    ["https://creative.example/frame", false],
    ["/relative/frame", false],
  ] as const)("detects whether URL %s needs an HTTP warning", (url, expected) => {
    expect(isInsecureHttpUrl(url)).toBe(expected);
  });

  it.each([
    ["<canvas></canvas>", false],
    ["<img src='https://cdn.example/banner.png'>", false],
    ["<img src='http://cdn.example/banner.png'>", true],
    ["<a href='HTTP://target.example'>Open</a>", true],
  ] as const)("detects HTTP references in HTML", (html, expected) => {
    expect(hasInsecureHttpReference(html)).toBe(expected);
  });

  it.each([
    ["http://creative.example/frame", true],
    ["https://creative.example/frame", true],
    ["/relative/frame", false],
    ["data:text/html,test", false],
    ["javascript:alert(1)", false],
  ] as const)("validates creative URL %s", (url, expected) => {
    expect(isValidCreativeUrl(url)).toBe(expected);
  });

  it("skips an incomplete HTML creative during draft autosave", async () => {
    const client = new FakeCreativeApi();
    await createCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative({
        creativeType: "html",
        htmlCode: "   ",
        pendingFile: undefined,
      })],
      skipIncomplete: true,
    });
    expect(client.uploads).toHaveLength(0);
    expect(client.creates).toHaveLength(0);
  });

  it.each(["native", "push"] as const)("uploads image before %s creative create", async (format) => {
    const client = new FakeCreativeApi();
    await createCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format,
      dimensions: { w: null, h: null },
      creatives: [baseCreative({ title: "Title", description: "Description" })],
    });
    expect(client.uploads).toHaveLength(1);
    expect(client.creates[0].body.image_id).toBe("uploaded-image-1");
    expect(client.creates[0].body.banner_type).toBeUndefined();
  });

  it("creates popunder without image upload or image fields", async () => {
    const client = new FakeCreativeApi();
    await createCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "popunder",
      dimensions: { w: null, h: null },
      creatives: [baseCreative({ pendingFile: undefined })],
    });
    expect(client.uploads).toHaveLength(0);
    expect(client.creates[0].body.adm).toBe("https://target.example");
    expect(client.creates[0].body.image_id).toBeUndefined();
    expect(client.creates[0].body.banner_type).toBeUndefined();
  });

  it("uploads a replacement image and PATCHes the same creative ID", async () => {
    const client = new FakeCreativeApi();
    await syncCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative({ id: "creative-1" })],
      existing: [existingCreative()],
    });
    expect(client.uploads).toHaveLength(1);
    expect(client.patches).toHaveLength(1);
    expect(client.patches[0]).toMatchObject({ id: "creative-1", body: { image_id: "uploaded-image-1" } });
    expect(client.deletes).toHaveLength(0);
  });

  it("does not upload or PATCH an unchanged existing image creative", async () => {
    const client = new FakeCreativeApi();
    const existing = existingCreative();
    await syncCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative({
        id: "creative-1",
        url: "https://target.example",
        pendingFile: undefined,
        imageId: "image-1",
        imageUrl: "https://cdn.example/image.jpg",
        imageFileName: "banner.jpg",
        imageMimeType: "image/jpg",
      })],
      existing: [existing],
    });
    expect(client.uploads).toHaveLength(0);
    expect(client.patches).toHaveLength(0);
    expect(client.deletes).toHaveLength(0);
  });

  it("clears image_id when banner switches from img to iframe", async () => {
    const client = new FakeCreativeApi();
    await syncCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative({
        id: "creative-1",
        creativeType: "iframe",
        iframeMode: "code",
        iframeCode: '<iframe src="https://creative.example"></iframe>',
        pendingFile: undefined,
      })],
      existing: [existingCreative()],
    });
    expect(client.uploads).toHaveLength(0);
    expect(client.patches[0].body).toMatchObject({
      banner_type: "iframe",
      image_id: null,
    });
  });

  it("clears image_id when banner switches from img to HTML", async () => {
    const client = new FakeCreativeApi();
    const html = "<canvas id='ad'></canvas>";
    await syncCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative({
        id: "creative-1",
        creativeType: "html",
        htmlCode: html,
        pendingFile: undefined,
      })],
      existing: [existingCreative()],
    });
    expect(client.uploads).toHaveLength(0);
    expect(client.patches[0].body).toMatchObject({
      banner_type: "iframe",
      adm: html,
      image_id: null,
      trackers_macros: {},
    });
  });

  it("deletes only creatives removed by the user", async () => {
    const client = new FakeCreativeApi();
    await syncCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [],
      existing: [existingCreative()],
    });
    expect(client.deletes).toEqual(["creative-1"]);
  });

  it("can add, update and delete different creatives without full recreation", async () => {
    const client = new FakeCreativeApi();
    const changed = existingCreative({ id: "creative-change", creative_name: "Old" });
    const removed = existingCreative({ id: "creative-remove" });
    await syncCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "popunder",
      dimensions: { w: null, h: null },
      creatives: [
        baseCreative({ id: "creative-change", name: "New", pendingFile: undefined }),
        baseCreative({ id: "local-new", name: "Added", pendingFile: undefined }),
      ],
      existing: [changed, removed],
    });
    expect(client.creates).toHaveLength(1);
    expect(client.patches.map(item => item.id)).toEqual(["creative-change"]);
    expect(client.deletes).toEqual(["creative-remove"]);
  });

  it("maps permanent image_url back into the editor independently of campaign status", () => {
    const mapped = mapApiCreativeToUi(existingCreative({
      image_url: "https://cdn.example/permanent.jpg",
      image_name: "permanent.jpg",
    }));
    expect(mapped.imageUrl).toBe("https://cdn.example/permanent.jpg");
    expect(mapped.imageFileName).toBe("permanent.jpg");
    expect(mapped.pendingFile).toBeUndefined();
    expect(mapped.creativeType).toBe("image");
  });

  it("sends MP4 with video/mp4 MIME and builds video ADM", async () => {
    const original = imageFile("banner.mp4", "application/octet-stream");
    expect(normalizeCreativeUploadFile(original).type).toBe("video/mp4");
    const client = new FakeCreativeApi();
    await createCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative({
        pendingFile: original,
        imageFileName: "banner.mp4",
        imageMimeType: "video/mp4",
        mediaType: "video",
      })],
    });
    expect(client.uploads[0].file.type).toBe("video/mp4");
    expect(client.creates[0].body.adm).toContain("<video ");
  });

  it("allows MP4 only for banners and enforces the 10 MB limit", () => {
    const exactLimit = new File(
      [new Uint8Array(MAX_CREATIVE_VIDEO_BYTES)],
      "banner.mp4",
      { type: "video/mp4" },
    );
    const aboveLimit = new File(
      [new Uint8Array(MAX_CREATIVE_VIDEO_BYTES + 1)],
      "banner.mp4",
      { type: "video/mp4" },
    );

    expect(validateCreativeFile(exactLimit, true)).toEqual({
      valid: true,
      mediaType: "video",
    });
    expect(validateCreativeFile(aboveLimit, true)).toEqual({
      valid: false,
      reason: "video-size",
    });
    expect(validateCreativeFile(exactLimit, false)).toEqual({
      valid: false,
      reason: "format",
    });
  });

  it.each([
    ["banner.png", "image/png", "image/png"],
    ["banner.jpg", "image/jpeg", "image/jpg"],
    ["banner.gif", "image/gif", "image/gif"],
  ] as const)("enforces the 1 MB limit and upload MIME for %s", (name, browserMime, uploadMime) => {
    const exactLimit = new File(
      [new Uint8Array(MAX_CREATIVE_IMAGE_BYTES)],
      name,
      { type: browserMime },
    );
    const aboveLimit = new File(
      [new Uint8Array(MAX_CREATIVE_IMAGE_BYTES + 1)],
      name,
      { type: browserMime },
    );

    expect(validateCreativeFile(exactLimit, true)).toEqual({
      valid: true,
      mediaType: "image",
    });
    expect(normalizeCreativeUploadFile(exactLimit).type).toBe(uploadMime);
    expect(validateCreativeFile(aboveLimit, true)).toEqual({
      valid: false,
      reason: "image-size",
    });
  });

  it("stops before creative POST when image upload fails", async () => {
    const client = new FakeCreativeApi();
    client.failUpload = true;
    await expect(createCampaignCreatives({
      client,
      campaignId: "campaign-1",
      format: "banner",
      dimensions,
      creatives: [baseCreative()],
    })).rejects.toBeInstanceOf(CreativeImageUploadError);
    expect(client.creates).toHaveLength(0);
  });
});
