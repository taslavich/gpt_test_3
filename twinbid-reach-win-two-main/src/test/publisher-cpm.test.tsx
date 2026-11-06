import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { publisherApi } from "@/api/publisher";
import { publisherCpm, publisherDisplayMetrics } from "@/lib/publisherData";
import PublisherStatistics from "@/pages/PublisherStatistics";
import {
  PublisherTestWrapper, publisherServer, userFixture, languageFixture,
  profileFixture, reportFixture, rowFixture, reply, balanceFixture,
} from "./publisher-fixtures";
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ user: userFixture, signOut: vi.fn() }) }));
vi.mock("@/contexts/LanguageContext", () => ({ useLanguage: () => languageFixture }));
vi.mock("@/contexts/ProfileContext", () => ({ useProfile: () => ({ profile: profileFixture, loading: false }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); localStorage.clear(); });

it.each([
  [1000, 10, "1.25", "1.25000000"],
  [10, 20, "1", "50.00000000"],
  [0, 10, "0.1", "10.00000000"],
  [0, 0, "5", "0"],
  [3, 1, "0.00000001", "0.00000333"],
  [10, 1, "123456789012345678.12345678", "12345678901234567812.34567800"],
  [1000, 1, "1e-8", "0.00000001"],
])("calculates exact CPM for impressions=%s clicks=%s revenue=%s", (impressions, clicks, revenue_usd, expected) => {
  expect(publisherCpm({ ...rowFixture, impressions, clicks, revenue_usd })).toBe(expected);
});

it("corrects only inconsistent impressions and CTR, without mutating source or revenue", () => {
  const source = Object.freeze({ ...rowFixture, impressions: 10, clicks: 20, ctr: 200 });
  expect(publisherDisplayMetrics(source)).toMatchObject({ impressions: 20, clicks: 20, ctr: 100, revenue_usd: source.revenue_usd });
  expect(source.impressions).toBe(10);
  expect(publisherDisplayMetrics(rowFixture)).toBe(rowFixture);
});

it("applies the display rule to stats, overview, site and zone rows and full-report totals", async () => {
  const server = publisherServer();
  const wrong = { ...rowFixture, impressions: 0, clicks: 7, ctr: 700 };
  const report = { ...reportFixture, rows: [wrong], totals: { ...wrong, revenue_usd: "10.00000001" } };
  server.overrides.set("POST /api/publisher/stats/query", () => reply(report));
  server.overrides.set("GET /api/publisher/overview", () => reply({ report, balance: balanceFixture, from: report.effective_from, to: report.effective_to }));
  for (const path of ["sites", "zones"])
    server.overrides.set("GET /api/publisher/" + path, () => reply({
      rows: [wrong], limit: 50, offset: 0,
      metric_period: "last_6_months", metric_period_scope: "impressions_clicks_ctr",
      request_metric_period: "last_7_days", request_metric_source: "publisher_zone_hourly",
      advertising_metric_source: "agg_stats", revenue_period: "lifetime",
    }));
  const stats = await publisherApi.stats({ from: report.effective_from, to: report.effective_to, group_by: "date" });
  const overview = await publisherApi.overview();
  const sites = await publisherApi.sites({ limit: 50, offset: 0 });
  const zones = await publisherApi.zones({ limit: 50, offset: 0 });
  for (const metrics of [stats.rows[0], stats.totals, overview.report.rows[0], overview.report.totals, sites.rows[0], zones.rows[0]])
    expect(metrics).toMatchObject({ impressions: 7, clicks: 7, ctr: 100 });
  expect(stats.totals.revenue_usd).toBe("10.00000001");
  expect(report.totals.impressions).toBe(0);
});

it("shows CPM by default, calculates it from corrected impressions and sorts it numerically", async () => {
  const server = publisherServer();
  server.overrides.set("POST /api/publisher/stats/query", req => reply({
    ...reportFixture,
    rows: req.body?.group_by === "date" ? [] : [
      { ...rowFixture, bucket: "a", impressions: 10, clicks: 20, ctr: 200, revenue_usd: "1" },
      { ...rowFixture, bucket: "b", impressions: 1000, clicks: 10, revenue_usd: "2" },
    ],
    totals: { ...reportFixture.totals, impressions: 100, clicks: 200, ctr: 200, revenue_usd: "12" },
  }));
  render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
  await screen.findByText("Итого за весь период");
  fireEvent.click(screen.getByRole("button", { name: "Формат" }));
  await screen.findByRole("cell", { name: "a" });
  const totalLabel = screen.getByText("Итого за весь период");
  const total = totalLabel.closest("tr")!;
  expect(within(total).getAllByRole("cell").map(cell => cell.textContent)).toEqual([
    "Итого за весь период", "200", "200", "100%", "$12,00", "$60,00",
  ]);
  const row = screen.getByRole("cell", { name: "a" }).closest("tr")!;
  expect(within(row).getAllByRole("cell").map(cell => cell.textContent)).toEqual(["a", "20", "20", "100%", "$1,00", "$50,00"]);
  const sort = within(screen.getByRole("columnheader", { name: "CPM" })).getByRole("button");
  fireEvent.click(sort);
  fireEvent.click(sort);
  expect(within(screen.getByRole("table")).getAllByRole("row")[2]).toHaveTextContent("b");
});
