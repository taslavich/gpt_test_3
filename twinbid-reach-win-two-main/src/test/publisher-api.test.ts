import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { publisherApi, type CreatePublisherCode } from "@/api/publisher";
import {
  codeFixture,
  publisherServer,
  reply,
  balanceFixture,
} from "./publisher-fixtures";
beforeEach(() => localStorage.clear());
afterEach(() => vi.unstubAllGlobals());
const {
  name,
  format,
  traffic_type,
  device,
  trigger_click,
  frequency_minutes,
  max_per_session,
} = codeFixture;
const create = {
  name,
  format,
  traffic_type,
  device,
  trigger_click,
  frequency_minutes,
  max_per_session,
};
describe("Publisher API uses the existing authenticated HTTP transport", () => {
  it("covers the real endpoint matrix and exact backend fields", async () => {
    const s = publisherServer(),
      id = codeFixture.ad_code_id,
      page = { limit: 50, offset: 0 };
    expect((await publisherApi.codes(page)).public_script_url).toBe(
      "https://stable.example.test/sdk.js",
    );
    await publisherApi.createCode(create);
    await publisherApi.code(id);
    await publisherApi.patchCode(id, { name: "Updated", version: 7 });
    await publisherApi.archiveCode(id);
    await publisherApi.duplicateCode(id);
    await publisherApi.diagnostics(id);
    await publisherApi.sites(page);
    await publisherApi.zones({
      ...page,
      site_id: "server-site",
      ad_code_id: id,
    });
    await publisherApi.overview();
    await publisherApi.stats({
      from: "2026-10-01T00:00:00Z",
      to: "2026-10-02T00:00:00Z",
      group_by: "hour",
      ...page,
    });
    await publisherApi.balance();
    await publisherApi.payouts(page);
    await publisherApi.settings();
    await publisherApi.syncHealth();
    const prefs = {
      codeAlerts: false,
      payoutMethod: "usdt_trc20" as const,
      wallet: "test-wallet",
    };
    await publisherApi.patchSettings(prefs);
    const pay = {
      amount: "50",
      wallet: "test-wallet",
      method: "usdt_trc20" as const,
      request_id: crypto.randomUUID(),
    };
    await publisherApi.createPayout(pay);
    expect(
      s.calls.every(
        (c) => c.headers.get("Authorization") === "Bearer test-access-token",
      ),
    ).toBe(true);
    expect(
      s.calls.filter((c) => c.body).every((c) => !("user_id" in c.body!)),
    ).toBe(true);
    expect(
      s.calls.find(
        (c) => c.method === "PATCH" && c.url.pathname.endsWith("/settings"),
      )!.body,
    ).toEqual(prefs);
    expect(
      s.calls.find(
        (c) => c.method === "POST" && c.url.pathname.endsWith("/payouts"),
      )!.body,
    ).toEqual(pay);
  });
  it("POST omission remains HTTP 400 and PATCH omission preserves the existing traffic type", async () => {
    const s = publisherServer();
    const invalid = { ...create } as Partial<CreatePublisherCode>;
    delete invalid.traffic_type;
    await expect(
      publisherApi.createCode(invalid as CreatePublisherCode),
    ).rejects.toMatchObject({ status: 400 });
    const patched = await publisherApi.patchCode(codeFixture.ad_code_id, {
      name: "New name",
      version: 7,
    });
    expect(patched.traffic_type).toBe("adult");
    expect(s.calls.at(-1)?.body).toEqual({ name: "New name", version: 7 });
  });
  it.each([400, 403, 404, 409, 503])(
    "surfaces HTTP %i instead of silently falling back to demonstration data",
    async (status) => {
      const s = publisherServer();
      s.overrides.set("GET /api/publisher/balance", () =>
        reply(null, status, "backend failure"),
      );
      await expect(publisherApi.balance()).rejects.toMatchObject({
        status,
        message: "backend failure",
      });
    },
  );
  it("uses the existing JWT refresh once after 401", async () => {
    const s = publisherServer();
    localStorage.setItem("twinbid_refresh_token", "refresh-before");
    let tries = 0;
    s.overrides.set("GET /api/publisher/balance", () =>
      ++tries === 1 ? reply(null, 401) : reply(balanceFixture),
    );
    s.overrides.set("POST /api/auth/refresh", () =>
      reply({ access_token: "access-after", refresh_token: "refresh-after" }),
    );
    await publisherApi.balance();
    expect(
      s.calls.filter((c) => c.url.pathname === "/api/auth/refresh"),
    ).toHaveLength(1);
    expect(s.calls.at(-1)?.headers.get("Authorization")).toBe(
      "Bearer access-after",
    );
  });
  it("rejects envelope failures and malformed successful business responses", async () => {
    const s = publisherServer();
    s.overrides.set(
      "GET /api/publisher/balance",
      () =>
        new Response(
          JSON.stringify({
            success: false,
            errorMsg: "failure",
            data: balanceFixture,
          }),
          { status: 200 },
        ),
    );
    await expect(publisherApi.balance()).rejects.toThrow("failure");
    s.overrides.set("GET /api/publisher/balance", () => reply({}));
    await expect(publisherApi.balance()).rejects.toMatchObject({
      code: "PUBLISHER_CONTRACT",
    });
    s.overrides.set(
      "GET /api/publisher/ad-codes/" + codeFixture.ad_code_id,
      () => reply({ ...codeFixture, traffic_type: "mixed" }),
    );
    await expect(
      publisherApi.code(codeFixture.ad_code_id),
    ).rejects.toMatchObject({ code: "PUBLISHER_CONTRACT" });
  });
});

it("describes distinct technical, advertising and lifetime periods",async()=>{
 publisherServer();const v=await publisherApi.sites({limit:50,offset:0});
 expect(v).toMatchObject({metric_period:"last_6_months",metric_period_scope:"impressions_clicks_ctr",request_metric_period:"last_7_days",request_metric_source:"publisher_zone_hourly",advertising_metric_source:"agg_stats",revenue_period:"lifetime"});
});
