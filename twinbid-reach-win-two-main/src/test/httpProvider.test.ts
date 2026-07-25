// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { ACCESS_TOKEN_KEY, REFRESH_TOKEN_KEY } from "@/api/config";
import { ApiError } from "@/api/http";
import { httpProvider } from "@/api/httpProvider";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function uploadSuccess() {
  return {
    success: true,
    errorMsg: "",
    data: {
      image_id: "image-1",
      campaign_id: "campaign-1",
      creative_id: null,
      image_url: "http://cdn.example/banner.png",
      filename: "banner.png",
      mime_type: "image/png",
      file_format: "image/png",
      size_bytes: 5,
      created_at: "2026-07-25T00:00:00Z",
      updated_at: "2026-07-25T00:00:00Z",
    },
  };
}

function imageFile() {
  return new File(["image"], "banner.png", { type: "image/png" });
}

afterEach(() => {
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe("creative multipart authentication", () => {
  it("uploads directly with a valid access token", async () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, "access-old");
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer access-old");
      return jsonResponse(uploadSuccess());
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await httpProvider.uploadCreativeImage("campaign-1", imageFile());
    expect(result.success).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("refreshes once after 401 and retries multipart with the new token", async () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, "access-old");
    localStorage.setItem(REFRESH_TOKEN_KEY, "refresh-old");
    const authorization: Array<string | null> = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/auth/refresh")) {
        return jsonResponse({ access_token: "access-new", refresh_token: "refresh-new" });
      }
      authorization.push(new Headers(init?.headers).get("Authorization"));
      return authorization.length === 1
        ? jsonResponse({ error: { message: "Unauthorized" } }, 401)
        : jsonResponse(uploadSuccess());
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await httpProvider.uploadCreativeImage("campaign-1", imageFile());
    expect(result.success).toBe(true);
    expect(authorization).toEqual(["Bearer access-old", "Bearer access-new"]);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBe("refresh-new");
  });

  it("shares one refresh across simultaneous multipart 401 responses", async () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, "access-old");
    localStorage.setItem(REFRESH_TOKEN_KEY, "refresh-old");
    let refreshCalls = 0;
    let oldTokenCalls = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/auth/refresh")) {
        refreshCalls += 1;
        return jsonResponse({ access_token: "access-new", refresh_token: "refresh-new" });
      }
      const token = new Headers(init?.headers).get("Authorization");
      if (token === "Bearer access-old") {
        oldTokenCalls += 1;
        return jsonResponse({ error: { message: "Unauthorized" } }, 401);
      }
      expect(token).toBe("Bearer access-new");
      return jsonResponse(uploadSuccess());
    });
    vi.stubGlobal("fetch", fetchMock);

    const [first, second] = await Promise.all([
      httpProvider.uploadCreativeImage("campaign-1", imageFile()),
      httpProvider.uploadCreativeImage("campaign-1", imageFile()),
    ]);
    expect(first.success).toBe(true);
    expect(second.success).toBe(true);
    expect(oldTokenCalls).toBe(2);
    expect(refreshCalls).toBe(1);
  });

  it("does not loop when token refresh fails", async () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, "access-old");
    localStorage.setItem(REFRESH_TOKEN_KEY, "refresh-old");
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      return String(input).endsWith("/api/auth/refresh")
        ? jsonResponse({ error: { message: "Refresh rejected" } }, 401)
        : jsonResponse({ error: { message: "Unauthorized" } }, 401);
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      httpProvider.uploadCreativeImage("campaign-1", imageFile()),
    ).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("preserves the real multipart HTTP error status", async () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, "access");
    vi.stubGlobal("fetch", vi.fn(async () =>
      jsonResponse({ error: { message: "File is too large", code: "FILE_TOO_LARGE" } }, 413),
    ));

    const request = httpProvider.uploadCreativeImage("campaign-1", imageFile());
    await expect(request).rejects.toBeInstanceOf(ApiError);
    await expect(request).rejects.toMatchObject({
      status: 413,
      code: "FILE_TOO_LARGE",
      message: "File is too large",
    });
  });

  it("does not set multipart Content-Type manually", async () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, "access");
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      expect(headers.has("Content-Type")).toBe(false);
      expect(init?.body).toBeInstanceOf(FormData);
      return jsonResponse(uploadSuccess());
    });
    vi.stubGlobal("fetch", fetchMock);

    await httpProvider.uploadCreativeImage("campaign-1", imageFile());
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("reports multipart network failures consistently as ApiError status 0", async () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, "access");
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new TypeError("Network down");
    }));

    await expect(
      httpProvider.uploadCreativeImage("campaign-1", imageFile()),
    ).rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
  });
});
