import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe("Deployment API configuration", () => {
  it.each([undefined, "false", "", "FALSE"])(
    "sends campaigns to the real backend when VITE_USE_MOCK=%s",
    async (flag) => {
      vi.resetModules();
      vi.stubEnv("VITE_USE_MOCK", flag);
      vi.stubEnv("VITE_API_BASE_URL", undefined);
      const fetchMock = vi.fn().mockResolvedValue(new Response(
        JSON.stringify({ success: true, data: { items: [], total: 0 } }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ));
      vi.stubGlobal("fetch", fetchMock);
      const { api, USE_MOCK, API_BASE_URL } = await import("@/api");
      expect(USE_MOCK).toBe(false);
      expect(API_BASE_URL).toBe("https://twinbid.io");
      expect(await api.listCampaigns()).toEqual({ items: [], total: 0 });
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(fetchMock.mock.calls[0][0]).toBe("https://twinbid.io/api/campaigns");
    },
  );

  it("preserves an explicitly configured API address", async () => {
    vi.resetModules();
    vi.stubEnv("VITE_API_BASE_URL", "https://backend.example.test");
    expect((await import("@/api/config")).API_BASE_URL).toBe("https://backend.example.test");
  });

  it("uses the working API for an empty URL setting", async () => {
    vi.resetModules();
    vi.stubEnv("VITE_API_BASE_URL", "  ");
    expect((await import("@/api/config")).API_BASE_URL).toBe("https://twinbid.io");
  });

  it("allows local fixtures only when explicitly enabled", async () => {
    vi.resetModules();
    vi.stubEnv("VITE_USE_MOCK", "true");
    expect((await import("@/api/config")).USE_MOCK).toBe(true);
  });
});
