import { expect, it, vi } from "vitest";
import { publisherTimeLabel, publisherTimeReport, publisherTimeSeries } from "@/lib/publisherTimeSeries";
import { publisherCpm } from "@/lib/publisherData";
import { reportFixture, rowFixture } from "./publisher-fixtures";
import type { PublisherStatsRequest } from "@/api/publisher";
const request: PublisherStatsRequest = { from: "2026-10-01T00:00:00Z", to: "2026-10-02T00:00:00Z", group_by: "hour", timezone: "UTC" };
it("merges minute and second timestamps into one hour, preserves exact money and fills 24 hours", () => {
  const rows = publisherTimeSeries([
    { ...rowFixture, bucket: "2026-10-01T09:12:34Z", impressions: 0, clicks: 10, revenue_usd: "0.10000001" },
    { ...rowFixture, bucket: "2026-10-01 09:47:59", impressions: 20, clicks: 0, revenue_usd: "0.20000002" },
  ], request);
  expect(rows).toHaveLength(24);
  expect(rows[9]).toMatchObject({ bucket: "2026-10-01T09:00:00.000Z", impressions: 20, clicks: 10, ctr: 50, revenue_usd: "0.30000003" });
  expect(publisherCpm(rows[9])).toBe("15.00000150");
  expect(rows[0]).toMatchObject({ impressions: 0, clicks: 0, ctr: 0, revenue_usd: "0" });
  expect(publisherTimeLabel(rows[9], "hour", "UTC")).toBe("01.10.2026 09:00");
});
it("fills every day, rolls timestamp rows into local days and excludes the right boundary", () => {
  const rows = publisherTimeSeries([
    { ...rowFixture, bucket: "2026-10-01T22:31:00Z", impressions: 5, clicks: 0 },
    { ...rowFixture, bucket: "2026-10-02", impressions: 5, clicks: 0 },
    { ...rowFixture, bucket: "2026-10-03T21:00:00Z", impressions: 999 },
  ], { ...request, group_by: "date", timezone: "utc_3", from: "2026-09-30T21:00:00Z", to: "2026-10-03T21:00:00Z" });
  expect(rows.map(row => [row.bucket, row.impressions])).toEqual([["2026-10-01", 0], ["2026-10-02", 10], ["2026-10-03", 0]]);
  expect(publisherTimeLabel(rows[1], "date")).toBe("02.10.2026");
});
it("uses the profile timezone hour boundaries, including half-hour offsets", () => {
  const rows = publisherTimeSeries([{ ...rowFixture, bucket: "2026-10-01T04:41:29+00:00" }],
    { ...request, timezone: "Asia/Kolkata", from: "2026-09-30T18:30:00Z", to: "2026-10-01T18:30:00Z" });
  expect(rows).toHaveLength(24);
  expect(publisherTimeLabel(rows[10], "hour", "Asia/Kolkata")).toBe("01.10.2026 10:00");
  expect(rows[10].impressions).toBe(rowFixture.impressions);
});
it.each([
  ["2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z", 23],
  ["2026-11-01T04:00:00Z", "2026-11-02T05:00:00Z", 25],
])("handles daylight-saving dates %s", (from, to, hours) => {
  const rows = publisherTimeSeries([], { ...request, from, to, timezone: "America/New_York" });
  expect(rows).toHaveLength(hours);
  expect(new Set(rows.map(row => row.bucket)).size).toBe(hours);
});
it("loads every server page before combining buckets and keeps server totals once", async () => {
  const first = { ...reportFixture, limit: 1, offset: 0, rows: [{ ...rowFixture, bucket: "2026-10-01T09:12:00Z" }] };
  const load = vi.fn(async (body: PublisherStatsRequest) => ({ ...first, offset: body.offset!, rows: body.offset! < 2 ? [{ ...first.rows[0], revenue_usd: "0.1" }] : [] }));
  const report = await publisherTimeReport(request, load);
  expect(load.mock.calls.map(([body]) => body.offset)).toEqual([0, 1, 2]);
  expect(report.rows[9].revenue_usd).toBe("0.2");
  expect(report.totals).toEqual(reportFixture.totals);
});
it("rejects invalid time data and cancelled loads instead of displaying misleading zeros", async () => {
  expect(() => publisherTimeSeries([{ ...rowFixture, bucket: "invalid-time" }], request)).toThrow();
  const controller = new AbortController(); controller.abort();
  const load = vi.fn();
  await expect(publisherTimeReport(request, load, controller.signal)).rejects.toMatchObject({ name: "AbortError" });
  expect(load).not.toHaveBeenCalled();
});
