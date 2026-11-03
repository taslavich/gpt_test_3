import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { selectPublisherFilter } from "./publisher-filter-ui";
import PublisherSites from "@/pages/PublisherSites";
import PublisherZones from "@/pages/PublisherZones";
import {
  PublisherTestWrapper,
  publisherServer,
  codeFixture,
  rowFixture,
  userFixture,
  languageFixture,
  reply,
} from "./publisher-fixtures";
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ user: userFixture, signOut: vi.fn() }),
}));
vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => languageFixture,
}));
beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
describe("Server-discovered inventory and real Code actions", () => {
  it("loads server sites and nested zones without generating or posting identifiers", async () => {
    const s = publisherServer();
    render(<PublisherSites />, { wrapper: PublisherTestWrapper });
    const site = await screen.findByRole("button", { name: /news.example.test/ });
    expect(screen.queryByText(rowFixture.site_id)).not.toBeInTheDocument();
    fireEvent.click(site);
    await screen.findByText(rowFixture.zone_id);
    expect(screen.getByText("site_id:")).toBeInTheDocument();
    expect(screen.getByText(rowFixture.site_id)).toBeInTheDocument();
    fireEvent.click(site);
    expect(screen.queryByText(rowFixture.site_id)).not.toBeInTheDocument();
    expect(
      s.calls.some(
        (c) =>
          c.url.pathname.endsWith("/zones") &&
          c.url.searchParams.get("site_id") === rowFixture.site_id,
      ),
    ).toBe(true);
    expect(s.calls.every((c) => c.method === "GET")).toBe(true);
    expect(Object.keys(localStorage).some((key) => /publisher/.test(key))).toBe(
      false,
    );
  });
  it("distinguishes successful empty discovery from a 503 failure", async () => {
    const s = publisherServer();
    s.overrides.set("GET /api/publisher/sites", () =>
      reply({
        rows: [],
        limit: 50,
        offset: 0,
        metric_period: "last_6_months",
        metric_period_scope: "impressions_clicks_ctr",
        request_metric_period: "last_7_days",
        request_metric_source: "publisher_zone_hourly",
        advertising_metric_source: "agg_stats",
        revenue_period: "lifetime",
      }),
    );
    render(<PublisherSites />, { wrapper: PublisherTestWrapper });
    await screen.findByText(/Сайтов на этой странице нет/);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    cleanup();
    s.overrides.set("GET /api/publisher/sites", () =>
      reply(null, 503, "inventory unavailable"),
    );
    render(<PublisherSites />, { wrapper: PublisherTestWrapper });
    await screen.findByRole("alert");
    expect(
      screen.queryByText(/Сайтов на этой странице нет/),
    ).not.toBeInTheDocument();
  });
  it("uses server pagination rather than inventing a total", async () => {
    const s = publisherServer();
    s.overrides.set("GET /api/publisher/sites", (req) =>
      reply({
        rows: Array.from({ length: 50 }, (_, i) => ({
          ...rowFixture,
          site_id: "server-" + i,
          hostname: "site" + i + ".test",
        })),
        limit: 50,
        offset: Number(req.url.searchParams.get("offset")),
        metric_period: "last_6_months",
        metric_period_scope: "impressions_clicks_ctr",
        request_metric_period: "last_7_days",
        request_metric_source: "publisher_zone_hourly",
        advertising_metric_source: "agg_stats",
        revenue_period: "lifetime",
      }),
    );
    render(<PublisherSites />, { wrapper: PublisherTestWrapper });
    await screen.findByText("site0.test");
    fireEvent.click(screen.getByRole("button", { name: "Далее" }));
    await waitFor(() =>
      expect(
        s.calls.some(
          (c) =>
            c.url.pathname.endsWith("/sites") &&
            c.url.searchParams.get("offset") === "50",
        ),
      ).toBe(true),
    );
  });
  it("keeps service identifiers out of code cards and uses clear zone/site/revenue columns", async () => {
    publisherServer();
    render(<PublisherZones />, { wrapper: PublisherTestWrapper });
    const title = await screen.findByRole("button", { name: new RegExp(codeFixture.name) });
    expect(screen.queryByText(new RegExp(codeFixture.short_code))).not.toBeInTheDocument();
    await selectPublisherFilter("Формат", ["Popunder"]);
    expect(screen.getByLabelText("Формат")).toHaveTextContent("Popunder");
    fireEvent.click(title);
    const table = await screen.findByRole("table");
    expect(within(table).getByRole("columnheader", { name: "Зоны" })).toBeInTheDocument();
    expect(within(table).getByRole("columnheader", { name: "Сайты" })).toBeInTheDocument();
    expect(within(table).getByRole("columnheader", { name: "Доход" })).toBeInTheDocument();
    expect(within(table).getByRole("cell", { name: "Popunder" })).toBeInTheDocument();
    expect(within(table).queryByText(codeFixture.ad_code_id)).not.toBeInTheDocument();
    expect(within(table).queryByText("USD")).not.toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "Активность кода" })).toBeInTheDocument();
    fireEvent.click(title);
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "Активность кода" })).toBeInTheDocument();
  });
  it("filters sites by zone/code relationships across all pages, combining code and format on the same zone", async () => {
    const s = publisherServer();
    const code2 = { ...codeFixture, ad_code_id: "code-two", name: "Second code" };
    s.overrides.set("GET /api/publisher/ad-codes", req => reply({ items: Number(req.url.searchParams.get("offset")) === 0 ? [codeFixture] : Number(req.url.searchParams.get("offset")) === 1 ? [code2] : [], limit:1,offset:Number(req.url.searchParams.get("offset")),public_script_url:"https://stable.example.test/sdk.js" }));
    const sites = [
      {...rowFixture,site_id:"first",hostname:"first.example.test"},
      {...rowFixture,site_id:"second",hostname:"second.example.test"},
    ];
    const inventory = { metric_period:"last_6_months",metric_period_scope:"impressions_clicks_ctr",request_metric_period:"last_7_days",request_metric_source:"publisher_zone_hourly",advertising_metric_source:"agg_stats",revenue_period:"lifetime" };
    s.overrides.set("GET /api/publisher/sites", req => { const offset = Number(req.url.searchParams.get("offset")); return reply({...inventory,limit:1,offset,rows:sites.slice(offset,offset+1)}); });
    const zones = [
      {...sites[0],ad_code_id:codeFixture.ad_code_id,zone_id:"first-zone",format:"popunder"},
      {...sites[1],ad_code_id:code2.ad_code_id,zone_id:"second-zone",format:"popunder"},
      {...sites[0],ad_code_id:code2.ad_code_id,zone_id:"historical-zone",format:"banner"},
    ];
    s.overrides.set("GET /api/publisher/zones", req => {
      const offset = Number(req.url.searchParams.get("offset"));
      const filtered = zones.filter(row => !req.url.searchParams.get("ad_code_id") || row.ad_code_id === req.url.searchParams.get("ad_code_id"));
      return reply({...inventory,limit:1,offset,rows:filtered.slice(offset,offset+1)});
    });
    render(<PublisherSites />, { wrapper: PublisherTestWrapper });
    await screen.findByRole("button", { name:/first.example.test/ });
    await selectPublisherFilter("Рекламный код", ["Second code"]);
    await screen.findByRole("button", { name:/second.example.test/ });
    expect(screen.getByRole("button", { name:/first.example.test/ })).toBeInTheDocument();
    await selectPublisherFilter("Формат", ["Popunder"]);
    await waitFor(() => expect(screen.queryByRole("button", { name:/first.example.test/ })).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name:/second.example.test/ })).toBeInTheDocument();
    expect(s.calls.some(req => req.url.pathname.endsWith("/ad-codes") && req.url.searchParams.get("offset") === "1")).toBe(true);
    expect(s.calls.some(req => req.url.pathname.endsWith("/sites") && req.url.searchParams.get("offset") === "1")).toBe(true);
    expect(s.calls.some(req => req.url.pathname.endsWith("/zones") && req.url.searchParams.get("ad_code_id") === code2.ad_code_id && req.url.searchParams.get("offset") === "1")).toBe(true);
    await selectPublisherFilter("Рекламный код", ["Server Popunder"]);
    await waitFor(() => expect(screen.queryByRole("button", { name:/second.example.test/ })).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name:/first.example.test/ })).toBeInTheDocument();
    expect(s.calls.every(req=>req.method==="GET")).toBe(true);
  });
  it("uses stable URL/short_code, diagnostics and real duplicate/toggle/archive endpoints", async () => {
    const s = publisherServer();
    render(<PublisherZones />, { wrapper: PublisherTestWrapper });
    await screen.findByText(codeFixture.name);
    fireEvent.click(screen.getByRole("switch", { name: "Активность кода" }));
    await waitFor(() =>
      expect(
        s.calls.some(
          (c) =>
            c.method === "PATCH" &&
            c.body?.version === 7 &&
            c.body?.is_active === false,
        ),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Дублировать" })).toBeEnabled(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Код" }));
    await screen.findByText(
      '<script async src="https://stable.example.test/sdk.js" food="tb_server_only"></script>',
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Диагностика установки" }),
    );
    await screen.findByText(rowFixture.site_id);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: "Дублировать" }));
    await screen.findByText(
      '<script async src="https://stable.example.test/sdk.js" food="tb_duplicate_server"></script>',
    );
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: "Архивировать код" }));
    fireEvent.click(screen.getByRole("button", { name: "Архивировать" }));
    await waitFor(() =>
      expect(s.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
    expect(
      s.calls.some((c) => /\/(confirm|request|cancel)/.test(c.url.pathname)),
    ).toBe(false);
  });
});
