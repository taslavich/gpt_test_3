import type { PublisherReport, PublisherRow, PublisherStatsRequest } from "@/api/publisher";
import { ApiError } from "@/api/http";
import { addPublisherMoney, publisherDisplayMetrics, publisherTimezone } from "./publisherData";

const HOUR = 3600000;
const COUNTERS = ["requests", "errors", "fills", "no_fills", "impressions", "clicks"] as const;
function invalid(): never {
  throw new ApiError(200, "Invalid publisher time-series data", "PUBLISHER_CONTRACT");
}
const formatters = new Map<string, Intl.DateTimeFormat>();
function parts(instant: number, timezone: string) {
  let formatter = formatters.get(timezone);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat("en-GB", {
      timeZone: timezone, year: "numeric", month: "2-digit", day: "2-digit",
      hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23",
    });
    if (formatters.size >= 16) formatters.clear();
    formatters.set(timezone, formatter);
  }
  return Object.fromEntries(formatter.formatToParts(instant).map(p => [p.type, p.value]));
}
function localDay(instant: number, timezone: string) {
  const p = parts(instant, timezone);
  return p.year + "-" + p.month + "-" + p.day;
}
function hourStart(instant: number, timezone: string) {
  const p = parts(instant, timezone);
  return instant - Number(p.minute) * 60000 - Number(p.second) * 1000 - instant % 1000;
}
function bucketInstant(value: string) {
  // Unqualified timestamps are UTC in the Publisher statistics contract.
  if (!/^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?$/i.test(value)) return invalid();
  const iso = value.replace(" ", "T");
  const result = Date.parse(/[zZ]|[+-]\d{2}:?\d{2}$/.test(iso) ? iso : iso + "Z");
  if (!Number.isFinite(result)) return invalid();
  return result;
}
function emptyRow(bucket: string): PublisherRow {
  return { bucket, site_id: "", zone_id: "", ad_code_id: "", hostname: "", last_request_at: null,
    requests: 0, errors: 0, fills: 0, no_fills: 0, impressions: 0, clicks: 0, ctr: 0, revenue_usd: "0" };
}

/** Aggregate every raw row first, then fill the entire applied period; never add totals from pages. */
export function publisherTimeSeries(rows: PublisherRow[], request: PublisherStatsRequest): PublisherRow[] {
  if (request.group_by !== "hour" && request.group_by !== "date") return rows;
  const from = Date.parse(request.from), to = Date.parse(request.to);
  if (!Number.isFinite(from) || !Number.isFinite(to) || to <= from || to - from > 367 * 86400000) return invalid();
  const timezone = publisherTimezone(request.timezone);
  const buckets = new Map<string, PublisherRow>();
  if (request.group_by === "hour") {
    for (let time = hourStart(from, timezone); time < to; time += HOUR) {
      const key = new Date(time).toISOString();
      buckets.set(key, emptyRow(key));
    }
  } else {
    const last = localDay(to - 1, timezone);
    for (let day = localDay(from, timezone); day <= last;) {
      buckets.set(day, emptyRow(day));
      day = new Date(Date.parse(day + "T00:00:00Z") + 86400000).toISOString().slice(0, 10);
    }
  }
  for (const row of rows) {
    let key: string;
    if (request.group_by === "date" && /^\d{4}-\d{2}-\d{2}$/.test(row.bucket)) {
      key = row.bucket;
    } else {
      const instant = bucketInstant(row.bucket);
      if (instant < from || instant >= to) continue;
      key = request.group_by === "hour"
        ? new Date(hourStart(instant, timezone)).toISOString()
        : localDay(instant, timezone);
    }
    const current = buckets.get(key);
    if (!current) continue;
    for (const counter of COUNTERS) current[counter] += row[counter];
    current.revenue_usd = addPublisherMoney(current.revenue_usd, row.revenue_usd);
  }
  return [...buckets.values()].map(row => publisherDisplayMetrics({
    ...row, ctr: row.impressions > 0 ? row.clicks / row.impressions * 100 : 0,
  }));
}

/** Same dd.MM.yyyy HH:00 notation as the advertiser cabinet, in the profile timezone. */
export function publisherTimeLabel(row: PublisherRow, group: "hour" | "date", timezone?: string) {
  const p = group === "hour" ? parts(bucketInstant(row.bucket), publisherTimezone(timezone)) : null;
  const day = p ? p.year + "-" + p.month + "-" + p.day : row.bucket;
  return day.slice(8, 10) + "." + day.slice(5, 7) + "." + day.slice(0, 4) + (p ? " " + p.hour + ":00" : "");
}

export async function publisherTimeReport(
  request: PublisherStatsRequest,
  load: (body: PublisherStatsRequest) => Promise<PublisherReport>,
  signal?: AbortSignal,
): Promise<PublisherReport> {
  const rows: PublisherRow[] = [];
  let first: PublisherReport | undefined, offset = 0;
  while (offset <= 1000000) {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    const page = await load({ ...request, limit: 500, offset });
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    if (page.offset !== offset || page.limit < 1) return invalid();
    if (!first) first = page;
    else if (page.effective_from !== first.effective_from || page.effective_to !== first.effective_to) return invalid();
    rows.push(...page.rows);
    if (page.rows.length < page.limit) {
      return { ...first, rows: publisherTimeSeries(rows, request), totals: publisherDisplayMetrics(first.totals), offset: 0 };
    }
    offset += page.limit;
  }
  return invalid();
}
