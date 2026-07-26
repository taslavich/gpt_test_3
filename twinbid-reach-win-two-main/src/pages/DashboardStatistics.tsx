import { useMemo, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Eye, MousePointer, Target, TrendingUp, ArrowUpDown, CalendarIcon, RefreshCw, Filter, Download, Zap, Percent, DollarSign } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";
import { format, subDays } from "date-fns";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

import { cn } from "@/lib/utils";
import type { DateRange } from "react-day-picker";
import { toast } from "sonner";
import { useCampaigns } from "@/contexts/CampaignContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { useStatistics } from "@/contexts/StatisticsContext";
import { formatCountryLabel } from "@/lib/countries";
import { COUNTRY_CODES } from "@/lib/dimensions";
import {
  BROWSER_FILTER_KEYS, OS_FILTER_KEYS, DEVICE_FILTER_KEYS,
  BROWSER_REVERSE, OS_REVERSE, DEVICE_REVERSE,
  expandFilter, mapRawToGroup, OTHER_KEY,
  BROWSER_FILTER_MAP, OS_FILTER_MAP, DEVICE_FILTER_MAP,
} from "@/lib/statFilters";
import { api } from "@/api";
import type { StatsGroupBy, StatsFilterBy } from "@/api/types";
import { formatNumberWithDot, formatStatisticInteger } from "@/lib/numberFormat";
import { useIsMobile } from "@/hooks/use-mobile";

type GroupBy = "dates" | "hours" | "browsers" | "siteid" | "devices" | "os" | "country";
type SortKey = "label" | "impressions" | "clicks" | "spent" | "cpm" | "cpc" | "conversions" | "income";
type SortDir = "asc" | "desc";

interface UiRow { label: string; impressions: number; clicks: number; spent: number; conversions: number; income: number; confirmedConversions: number; confirmedIncome: number; }

// UI groupBy → ClickHouse group_by + bucket key in the response row.
const GROUP_MAP: Record<GroupBy, { api: StatsGroupBy }> = {
  dates:    { api: "date" },
  hours:    { api: "hour" },
  browsers: { api: "browser" },
  siteid:   { api: "site_id" },
  devices:  { api: "device_type" },
  os:       { api: "os" },
  country:  { api: "country" },
};

function formatDateLabel(iso: string): string {
  // YYYY-MM-DD → dd.MM.yyyy
  const [y, m, d] = iso.split("-");
  return `${d}.${m}.${y}`;
}
function formatHourLabel(raw: string): string {
  // "YYYY-MM-DD HH:00" → "dd.MM.yyyy HH:00"
  const [day, hour] = raw.split(" ");
  return `${formatDateLabel(day)} ${hour}`;
}
// "Today" in UTC 0. Returns a local Date whose Y/M/D fields equal the current
// UTC calendar day, so fmtUtcDay() (which reads getFullYear/Month/Date) emits
// the correct UTC date string regardless of the user's timezone.
function utcToday(): Date {
  const n = new Date();
  return new Date(n.getUTCFullYear(), n.getUTCMonth(), n.getUTCDate());
}

// Dictionaries used purely for filter UI options.
const DIMENSION_MAP: Record<string, string[]> = {
  country: COUNTRY_CODES,
  browsers: BROWSER_FILTER_KEYS,
  devices: DEVICE_FILTER_KEYS,
  os: OS_FILTER_KEYS,
};

// Multi-select filter component (supports plain string options or {value,label} pairs)
type FilterOption = string | { value: string; label: string };
function MultiSelectFilter({ label, options, selected, onChange }: {
  label: string; options: FilterOption[]; selected: Set<string>;
  onChange: React.Dispatch<React.SetStateAction<Set<string>>>;
}) {
  const { t } = useLanguage();
  const normalized = options
    .map(o => typeof o === "string" ? { value: o, label: o } : o)
    .slice()
    .sort((a, b) => {
      if (a.value === OTHER_KEY) return 1;
      if (b.value === OTHER_KEY) return -1;
      return a.label.localeCompare(b.label);
    });
  const toggle = (val: string) => {
    onChange(prev => {
      const next = new Set(prev);
      if (next.has(val)) next.delete(val); else next.add(val);
      return next;
    });
  };
  const displayText = selected.size === 0 ? t("stats.allValues") : `${selected.size} ${t("stats.selected")}`;

  return (
    <div className="flex min-w-0 flex-col gap-1">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <Popover>
        <PopoverTrigger asChild>
          <Button variant="outline" className="h-8 w-full min-w-0 justify-start truncate bg-background border-border text-left text-sm font-normal sm:w-[220px]">
            {displayText}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[min(260px,calc(100vw-1rem))] p-2" align="start">
          <div className="space-y-1 max-h-56 overflow-y-auto">
            <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm font-medium border-b border-border pb-2 mb-1">
              <Checkbox checked={selected.size === 0} onCheckedChange={(checked) => { if (checked) onChange(new Set()); }} />
              {t("stats.allValues")}
            </label>
            {normalized.map(opt => (
              <label key={opt.value} className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                <Checkbox checked={selected.has(opt.value)} onCheckedChange={() => toggle(opt.value)} />
                <span className="truncate">{opt.label}</span>
              </label>
            ))}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}

export default function DashboardStatistics() {
  const isMobile = useIsMobile();
  const { campaigns, loadCampaignCreatives } = useCampaigns();
  const { t, lang } = useLanguage();
  
  const {
    selectedCampaignIds, setSelectedCampaignIds,
    selectedCreativeIds, setSelectedCreativeIds,
    dateRange, setDateRange,
    clickCount, setClickCount,
    filterCountry, setFilterCountry,
    filterBrowser, setFilterBrowser,
    filterDevice, setFilterDevice,
    filterOS, setFilterOS,
    groupBy, setGroupBy,
    chartMetric, setChartMetric,
    sortKey, setSortKey,
    sortDir, setSortDir,
    appliedCampaignIds, setAppliedCampaignIds,
    appliedCreativeIds, setAppliedCreativeIds,
    appliedDateRange, setAppliedDateRange,
    appliedFilterCountry, setAppliedFilterCountry,
    appliedFilterBrowser, setAppliedFilterBrowser,
    appliedFilterDevice, setAppliedFilterDevice,
    appliedFilterOS, setAppliedFilterOS,
    showConversions, setShowConversions,
    showImpressions, setShowImpressions,
    showClicks, setShowClicks,
    showCtr, setShowCtr,
    showSpent, setShowSpent,
    showCpm, setShowCpm,
    showCpc, setShowCpc,
    showConversionsCol, setShowConversionsCol,
    showConfirmedConversions, setShowConfirmedConversions,
    showCr, setShowCr,
    showIncome, setShowIncome,
    showConfirmedIncome, setShowConfirmedIncome,
    showRoi, setShowRoi,
  } = useStatistics();

  const appliedGroupBy = groupBy;

  const hasActiveFilters = appliedFilterCountry.size > 0 || appliedFilterBrowser.size > 0 || appliedFilterDevice.size > 0 || appliedFilterOS.size > 0;

  const groupLabels: Record<GroupBy, string> = {
    dates: t("stats.byDates"), hours: t("stats.byHours"), browsers: t("stats.byBrowsers"),
    siteid: t("stats.bySiteId"), devices: t("stats.byDevices"), os: t("stats.byOS"), country: t("stats.byCountry"),
  };

  const activeCampaigns = useMemo(() =>
    campaigns.filter(c => c.status === "active" || c.status === "completed" || c.status === "paused"),
    [campaigns]
  );

  const selectedCampaignId = useMemo(() => {
    if (selectedCampaignIds.size === 1) return Array.from(selectedCampaignIds)[0];
    return "";
  }, [selectedCampaignIds]);

  const availableCreatives = useMemo(() => {
    const result: { id: string; label: string }[] = [];
    if (!selectedCampaignId) return result;
    const campaign = campaigns.find(c => c.id === selectedCampaignId);
    if (!campaign) return result;
    (campaign.creatives || []).forEach((cr, idx) => {
      const label = cr.name || cr.title || cr.url || `Creative #${idx + 1}`;
      result.push({ id: cr.id, label });
    });
    return result;
  }, [selectedCampaignId, campaigns]);

  // Country filter options with localized name + ISO code
  const countryOptions = useMemo(
    () => DIMENSION_MAP.country.map(code => ({ value: code, label: formatCountryLabel(code, lang) })),
    [lang]
  );

  // On mount: if nothing applied yet, auto-apply "all active campaigns" + last 7 days.
  useEffect(() => {
    if (appliedCampaignIds.size === 0 && activeCampaigns.length > 0) {
      const defaultRange: DateRange = { from: subDays(utcToday(), 6), to: utcToday() };
      setAppliedCampaignIds(new Set(activeCampaigns.map(c => c.id)));
      // Reflect defaults in the UI controls so the user sees what's applied
      if (!dateRange?.from) setDateRange(defaultRange);
      setAppliedDateRange(appliedDateRange ?? defaultRange);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeCampaigns]);

  const hasSelection = appliedCampaignIds.size > 0 && appliedDateRange?.from;

  const [data, setData] = useState<UiRow[]>([]);
  const [slowLoading, setSlowLoading] = useState(false);
  type PageSize = 50 | 100 | "all";
  const [pageSize, setPageSize] = useState<PageSize>(50);
  useEffect(() => { setPageSize(50); }, [appliedGroupBy]);

  // Preserve scroll position when the user switches grouping or page size.
  // Without this, clearing rows synchronously shrinks the page and the browser
  // jumps up to the widgets. We snapshot scrollY on the intent (button click)
  // and restore it in a layout effect once new rows have laid out.
  const pendingScrollRef = useRef<number | null>(null);
  useLayoutEffect(() => {
    if (pendingScrollRef.current != null) {
      window.scrollTo({ top: pendingScrollRef.current });
      pendingScrollRef.current = null;
    }
  }, [data, appliedGroupBy, pageSize]);

  useEffect(() => {
    if (!hasSelection) { setData([]); return; }
    let cancelled = false;
    const apiGroup = GROUP_MAP[appliedGroupBy].api;
    // Do NOT clear rows synchronously — keeping the previous table visible
    // until new data arrives prevents the page from collapsing and the
    // browser from scrolling up. `slowLoading` still shows a spinner overlay
    // for long queries.
    const fmtUtcDay = (d: Date) => {
      const y = d.getFullYear();
      const m = String(d.getMonth() + 1).padStart(2, "0");
      const day = String(d.getDate()).padStart(2, "0");
      return `${y}-${m}-${day}`;
    };
    const from = appliedDateRange?.from ? fmtUtcDay(appliedDateRange.from) : "";
    const to = appliedDateRange?.to ? fmtUtcDay(appliedDateRange.to) : from;

    // Build the base filter set. For dimensions where "other" is selected we
    // can't just enumerate values, so we resolve them via a preliminary query
    // (see below) and inject the resulting raw list here.
    const filters: Partial<Record<StatsFilterBy, string[]>> = {};
    if (appliedFilterCountry.size) filters.country = Array.from(appliedFilterCountry);
    const browserRaw = expandFilter(appliedFilterBrowser, BROWSER_FILTER_MAP);
    if (browserRaw && browserRaw.length) filters.browser = browserRaw;
    const deviceRaw = expandFilter(appliedFilterDevice, DEVICE_FILTER_MAP);
    if (deviceRaw && deviceRaw.length) filters.device_type = deviceRaw;
    const osRaw = expandFilter(appliedFilterOS, OS_FILTER_MAP);
    if (osRaw && osRaw.length) filters.os = osRaw;

    // Dimensions where the user picked "other" — we need to discover the raw
    // values actually present and keep the subset whose group ∈ filterSet.
    type OtherDim = {
      field: StatsFilterBy;
      group: StatsGroupBy;
      filterSet: Set<string>;
      reverse: Map<string, string>;
    };
    const otherDims: OtherDim[] = [];
    if (appliedFilterBrowser.has(OTHER_KEY))
      otherDims.push({ field: "browser", group: "browser", filterSet: appliedFilterBrowser, reverse: BROWSER_REVERSE });
    if (appliedFilterOS.has(OTHER_KEY))
      otherDims.push({ field: "os", group: "os", filterSet: appliedFilterOS, reverse: OS_REVERSE });
    if (appliedFilterDevice.has(OTHER_KEY))
      otherDims.push({ field: "device_type", group: "device_type", filterSet: appliedFilterDevice, reverse: DEVICE_REVERSE });

    const slowTimer = window.setTimeout(() => { if (!cancelled) setSlowLoading(true); }, 1000);

    const baseReq = {
      from, to,
      campaign_ids: Array.from(appliedCampaignIds),
      creative_ids: appliedCreativeIds.size ? Array.from(appliedCreativeIds) : undefined,
    };

    const resolveOthers = async () => {
      if (otherDims.length === 0) return;
      // For each "other" dimension, query grouped by that dimension using the
      // other (already resolved) filters, to enumerate the raw values present.
      const results = await Promise.all(otherDims.map(dim => {
        const preFilters: Partial<Record<StatsFilterBy, string[]>> = { ...filters };
        delete preFilters[dim.field];
        return api.statsQuery({
          ...baseReq,
          group_by: dim.group,
          filters: preFilters,
        }).then(res => ({ dim, rawKeys: Object.keys(res.rows) }));
      }));
      for (const { dim, rawKeys } of results) {
        const kept = rawKeys.filter(raw => dim.filterSet.has(mapRawToGroup(raw, dim.reverse)));
        // If the user selected only "other" and there are no unknown values,
        // send an impossible filter so the main query returns empty.
        filters[dim.field] = kept.length ? kept : ["__none__"];
      }
    };

    resolveOthers()
      .then(() => {
        if (cancelled) return null;
        return api.statsQuery({ ...baseReq, group_by: apiGroup, filters });
      })
      .then(res => {
      if (cancelled || !res) return;
      const byKey = new Map<string, { impressions: number; clicks: number; spent: number; conversions: number; income: number; confirmedConversions: number; confirmedIncome: number }>();
      for (const [key, m] of Object.entries(res.rows)) {
        const extra = m as unknown as { conversions?: number; income?: number; conversions_approved?: number; income_approved?: number };
        byKey.set(key, {
          impressions: Number(m.impressions) || 0,
          clicks: Number(m.clicks) || 0,
          spent: Number(m.spent) || 0,
          conversions: Number(extra.conversions) || 0,
          income: Number(extra.income) || 0,
          confirmedConversions: Number(extra.conversions_approved) || 0,
          confirmedIncome: Number(extra.income_approved) || 0,
        });
      }
      const empty = { impressions: 0, clicks: 0, spent: 0, conversions: 0, income: 0, confirmedConversions: 0, confirmedIncome: 0 };
      let rows: UiRow[];
      if (apiGroup === "hour") {
        const keys: string[] = [];
        const start = new Date(`${from}T00:00:00Z`);
        const end = new Date(`${to}T00:00:00Z`);
        for (let d = new Date(start); d <= end; d.setUTCDate(d.getUTCDate() + 1)) {
          const y = d.getUTCFullYear();
          const mo = String(d.getUTCMonth() + 1).padStart(2, "0");
          const da = String(d.getUTCDate()).padStart(2, "0");
          for (let h = 0; h < 24; h++) {
            keys.push(`${y}-${mo}-${da} ${String(h).padStart(2, "0")}:00`);
          }
        }
        rows = keys.map(k => {
          const m = byKey.get(k) ?? empty;
          return { label: formatHourLabel(k), ...m };
        });
      } else if (apiGroup === "date") {
        const keys: string[] = [];
        const start = new Date(`${from}T00:00:00Z`);
        const end = new Date(`${to}T00:00:00Z`);
        for (let d = new Date(start); d <= end; d.setUTCDate(d.getUTCDate() + 1)) {
          const y = d.getUTCFullYear();
          const mo = String(d.getUTCMonth() + 1).padStart(2, "0");
          const da = String(d.getUTCDate()).padStart(2, "0");
          keys.push(`${y}-${mo}-${da}`);
        }
        rows = keys.map(k => {
          const m = byKey.get(k) ?? empty;
          return { label: formatDateLabel(k), ...m };
        });
      } else {
        const reverse =
          apiGroup === "browser" ? BROWSER_REVERSE :
          apiGroup === "os"      ? OS_REVERSE :
          apiGroup === "device_type" ? DEVICE_REVERSE : null;
        const filterSet =
          apiGroup === "browser" ? appliedFilterBrowser :
          apiGroup === "os"      ? appliedFilterOS :
          apiGroup === "device_type" ? appliedFilterDevice : null;

        if (reverse) {
          const grouped = new Map<string, typeof empty>();
          for (const [rawKey, m] of byKey.entries()) {
            const groupKey = mapRawToGroup(rawKey, reverse);
            const acc = grouped.get(groupKey) ?? { ...empty };
            acc.impressions += m.impressions;
            acc.clicks += m.clicks;
            acc.spent += m.spent;
            acc.conversions += m.conversions;
            acc.income += m.income;
            acc.confirmedConversions += m.confirmedConversions;
            acc.confirmedIncome += m.confirmedIncome;
            grouped.set(groupKey, acc);
          }
          rows = Array.from(grouped.entries())
            .filter(([key]) => !filterSet || filterSet.size === 0 || filterSet.has(key))
            .map(([key, m]) => ({ label: key, ...m }));
        } else {
          rows = Array.from(byKey.entries()).map(([key, m]) => ({ label: key, ...m }));
        }
      }
      setData(rows);
    }).catch(e => { if (!cancelled) console.error("Stats query error:", e); })
      .finally(() => {
        window.clearTimeout(slowTimer);
        if (!cancelled) setSlowLoading(false);
      });
    return () => {
      cancelled = true;
      window.clearTimeout(slowTimer);
      setSlowLoading(false);
    };
  }, [appliedCampaignIds, appliedCreativeIds, appliedGroupBy, appliedDateRange, appliedFilterCountry, appliedFilterBrowser, appliedFilterDevice, appliedFilterOS, hasSelection]);

  

  const metricCards = useMemo(() => {
    const totalImpressions = data.reduce((s, r) => s + r.impressions, 0);
    const totalClicks = data.reduce((s, r) => s + r.clicks, 0);
    const totalSpent = data.reduce((s, r) => s + r.spent, 0);
    const totalConversions = data.reduce((s, r) => s + r.conversions, 0);
    const totalIncome = data.reduce((s, r) => s + r.income, 0);
    const ctr = totalImpressions > 0 ? ((totalClicks / totalImpressions) * 100).toFixed(2) : "0.00";
    const cr = totalClicks > 0 ? ((totalConversions / totalClicks) * 100).toFixed(2) : "0.00";
    const totalConfirmedIncome = data.reduce((s, r) => s + r.confirmedIncome, 0);
    const roi = totalSpent > 0 ? (((totalConfirmedIncome - totalSpent) / totalSpent) * 100).toFixed(2) : "0.00";
    const base = [
      { label: t("stats.impressions"), value: formatStatisticInteger(totalImpressions), icon: Eye },
      { label: t("stats.clicks"), value: formatStatisticInteger(totalClicks), icon: MousePointer },
      { label: t("stats.ctr"), value: `${ctr}%`, icon: Target },
      { label: t("stats.spent"), value: `$${formatNumberWithDot(totalSpent)}`, icon: TrendingUp },
    ];
    if (!showConversions) return base;
    return [
      ...base,
      { label: t("stats.conversions"), value: formatStatisticInteger(totalConversions), icon: Zap },
      { label: t("stats.cr"), value: `${cr}%`, icon: Percent },
      { label: t("stats.income"), value: `$${formatNumberWithDot(totalIncome)}`, icon: DollarSign },
      { label: t("stats.roi"), value: `${roi}%`, icon: TrendingUp },
    ];
  }, [data, t, showConversions]);

  useEffect(() => {
    if (appliedGroupBy === "dates") { setSortKey("label"); setSortDir("desc"); }
    else if (appliedGroupBy === "hours") { setSortKey("label"); setSortDir("asc"); }
    else { setSortKey("impressions"); setSortDir("desc"); }
  }, [appliedGroupBy]);


   const handleRefresh = useCallback(() => {
    // For "all" campaigns, add all active campaign ids
    if (selectedCampaignIds.size === 0) {
      setAppliedCampaignIds(new Set(activeCampaigns.map(c => c.id)));
    } else {
      setAppliedCampaignIds(new Set(selectedCampaignIds));
    }
    setAppliedCreativeIds(new Set(selectedCreativeIds));
    setAppliedDateRange(dateRange);
    setAppliedFilterCountry(filterCountry);
    setAppliedFilterBrowser(filterBrowser);
    setAppliedFilterDevice(filterDevice);
    setAppliedFilterOS(filterOS);
    toast.success(t("stats.refreshed"));
  }, [selectedCampaignIds, selectedCreativeIds, dateRange, filterCountry, filterBrowser, filterDevice, filterOS, t, activeCampaigns]);

  const toggleCampaign = (id: string) => {
    setSelectedCampaignIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
    setSelectedCreativeIds(new Set());
  };

  const handleDayClick = (day: Date) => {
    const from = dateRange?.from;
    const to = dateRange?.to;
    // Start new range if nothing selected, or a complete range already exists
    if (!from || (from && to)) {
      setDateRange({ from: day, to: undefined });
      return;
    }
    // Only "from" is selected — second click sets "to"
    if (day.getTime() < from.getTime()) {
      // Earlier date becomes the new start
      setDateRange({ from: day, to: undefined });
    } else {
      setDateRange({ from, to: day });
    }
  };

  const chartData = useMemo(() => {
    if (appliedGroupBy !== "dates" && appliedGroupBy !== "hours") return [];
    // `data` is already inserted in chronological order by the fill loop above.
    return data;
  }, [data, appliedGroupBy]);

  const sortedData = useMemo(() => {
    if (sortKey === "label") {
      return [...data].sort((a, b) => sortDir === "asc" ? a.label.localeCompare(b.label) : b.label.localeCompare(a.label));
    }
    const valueOf = (r: UiRow): number => {
      if (sortKey === "cpm") return r.impressions > 0 ? r.spent / r.impressions * 1000 : 0;
      if (sortKey === "cpc") return r.clicks > 0 ? r.spent / r.clicks : 0;
      return r[sortKey];
    };
    return [...data].sort((a, b) => {
      const av = valueOf(a);
      const bv = valueOf(b);
      return sortDir === "desc" ? bv - av : av - bv;
    });
  }, [data, sortKey, sortDir]);

  const visibleRows = useMemo(
    () => pageSize === "all" ? sortedData : sortedData.slice(0, pageSize),
    [sortedData, pageSize],
  );

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) setSortDir(d => d === "desc" ? "asc" : "desc");
    else { setSortKey(key); setSortDir("desc"); }
  };

  const SortIcon = ({ col }: { col: SortKey }) => (
    <ArrowUpDown className={cn("h-3 w-3 ml-1 inline shrink-0", sortKey === col ? "text-primary" : "text-muted-foreground")} />
  );

  const totals = useMemo(() => ({
    impressions: sortedData.reduce((s, r) => s + r.impressions, 0),
    clicks: sortedData.reduce((s, r) => s + r.clicks, 0),
    spent: sortedData.reduce((s, r) => s + r.spent, 0),
    conversions: sortedData.reduce((s, r) => s + r.conversions, 0),
    income: sortedData.reduce((s, r) => s + r.income, 0),
    confirmedConversions: sortedData.reduce((s, r) => s + r.confirmedConversions, 0),
    confirmedIncome: sortedData.reduce((s, r) => s + r.confirmedIncome, 0),
  }), [sortedData]);

  const labelHeader = appliedGroupBy === "dates" ? t("stats.date") : appliedGroupBy === "hours" ? t("stats.dateAndHour") : appliedGroupBy === "browsers" ? t("stats.browser") : appliedGroupBy === "siteid" ? "SiteID" : appliedGroupBy === "os" ? t("stats.os") : appliedGroupBy === "country" ? t("stats.country") : t("stats.device");
  const canSortByLabel = appliedGroupBy === "dates" || appliedGroupBy === "hours";

  const handleDownloadCsv = useCallback(() => {
    if (!sortedData.length) return;
    const baseHeaders = [labelHeader, t("stats.impressions"), t("stats.clicks"), t("stats.ctr"), t("stats.spent")];
    if (showCpm) baseHeaders.push(t("stats.cpm"));
    if (showCpc) baseHeaders.push(t("stats.cpc"));
    const convHeaders: string[] = [t("stats.conversions")];
    if (showConfirmedConversions) convHeaders.push(t("stats.confirmedConversions"));
    convHeaders.push(t("stats.cr"), t("stats.income"));
    if (showConfirmedIncome) convHeaders.push(t("stats.confirmedIncome"));
    convHeaders.push(t("stats.roi"));
    const headers = showConversions ? [...baseHeaders, ...convHeaders] : baseHeaders;
    const escape = (v: string | number) => {
      const s = String(v);
      return /[",\n;]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
    };
    const cpmOf = (r: { spent: number; impressions: number }) => {
      if (r.impressions === 0) return "0.00";
      const v = r.spent / r.impressions * 1000;
      return (Math.floor(v * 100) / 100).toFixed(2);
    };
    const cpcOf = (r: { spent: number; clicks: number }) => {
      if (r.clicks === 0) return "0.00000";
      const v = r.spent / r.clicks;
      return (Math.floor(v * 100000) / 100000).toFixed(5);
    };
    const rows = sortedData.map(r => {
      const label = appliedGroupBy === "country" ? formatCountryLabel(r.label, lang) : r.label;
      const ctr = r.impressions > 0 ? ((r.clicks / r.impressions) * 100).toFixed(2) + "%" : "0.00%";
      const base: (string | number)[] = [label, r.impressions, r.clicks, ctr, r.spent.toFixed(2)];
      if (showCpm) base.push(cpmOf(r));
      if (showCpc) base.push(cpcOf(r));
      if (!showConversions) return base.map(escape).join(",");
      const cr = r.clicks > 0 ? ((r.conversions / r.clicks) * 100).toFixed(2) + "%" : "0.00%";
      const roi = r.spent > 0 ? (((r.confirmedIncome - r.spent) / r.spent) * 100).toFixed(2) + "%" : "0.00%";
      const conv: (string | number)[] = [r.conversions];
      if (showConfirmedConversions) conv.push(r.confirmedConversions);
      conv.push(cr, r.income.toFixed(2));
      if (showConfirmedIncome) conv.push(r.confirmedIncome.toFixed(2));
      conv.push(roi);
      return [...base, ...conv].map(escape).join(",");
    });
    const ctrTotal = totals.impressions > 0 ? ((totals.clicks / totals.impressions) * 100).toFixed(2) + "%" : "0.00%";
    const baseTotal: (string | number)[] = [t("stats.total"), totals.impressions, totals.clicks, ctrTotal, totals.spent.toFixed(2)];
    if (showCpm) baseTotal.push(cpmOf(totals));
    if (showCpc) baseTotal.push(cpcOf(totals));
    const crTotal = totals.clicks > 0 ? ((totals.conversions / totals.clicks) * 100).toFixed(2) + "%" : "0.00%";
    const roiTotal = totals.spent > 0 ? (((totals.confirmedIncome - totals.spent) / totals.spent) * 100).toFixed(2) + "%" : "0.00%";
    const convTotal: (string | number)[] = [totals.conversions];
    if (showConfirmedConversions) convTotal.push(totals.confirmedConversions);
    convTotal.push(crTotal, totals.income.toFixed(2));
    if (showConfirmedIncome) convTotal.push(totals.confirmedIncome.toFixed(2));
    convTotal.push(roiTotal);
    const totalsRow = (showConversions
      ? [...baseTotal, ...convTotal]
      : baseTotal
    ).map(escape).join(",");
    const csv = "\uFEFF" + [headers.map(escape).join(","), ...rows, totalsRow].join("\n");
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    const ts = new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-");
    a.href = url;
    a.download = `twinbid-stats-${appliedGroupBy}-${ts}.csv`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, [sortedData, totals, labelHeader, appliedGroupBy, lang, t, showConversions, showCpm, showCpc, showConfirmedConversions, showConfirmedIncome]);

  // Custom tooltip for hours chart
  const HoursTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ value?: number }>; label?: string }) => {
    if (!active || !payload?.length) return null;
    const metricLabel = chartMetric === "impressions" ? t("stats.impressions") : chartMetric === "clicks" ? t("stats.clicks") : t("stats.spent");
    const value = payload[0]?.value;
    return (
      <div className="rounded-lg border bg-card px-3 py-2 text-sm shadow-md" style={{ borderColor: "hsl(var(--border))" }}>
        <p className="font-medium text-foreground">{label}</p>
        <p className="text-muted-foreground">{metricLabel}: <span className="font-semibold text-foreground">{chartMetric === "spent" ? `$${formatNumberWithDot(Number(value) || 0)}` : formatNumberWithDot(Number(value) || 0)}</span></p>
      </div>
    );
  };

  return (
    <div className="space-y-6">
      {slowLoading && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/60 backdrop-blur-sm pointer-events-none">
          <div className="pointer-events-auto rounded-lg border border-border bg-card px-6 py-4 shadow-lg flex items-center gap-3">
            <RefreshCw className="h-5 w-5 animate-spin text-primary" />
            <span className="text-sm text-foreground">{t("stats.loading")}</span>
          </div>
        </div>
      )}
      <div>
        <h2 className="text-2xl font-bold">{t("stats.title")}</h2>
        <p className="text-muted-foreground text-sm">{t("stats.subtitle")}</p>
      </div>

      <div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-end sm:gap-6">
        <div className="flex min-w-0 flex-col gap-2">
          <Label className="text-sm text-muted-foreground font-medium">{t("stats.campaigns")}</Label>
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="outline" className="w-full min-w-0 justify-start truncate bg-background border-border text-left font-normal sm:w-[280px]">
                {selectedCampaignIds.size === 0
                  ? t("stats.allCampaigns")
                  : selectedCampaignIds.size === 1
                    ? (activeCampaigns.find(c => c.id === Array.from(selectedCampaignIds)[0])?.name ?? `${t("stats.selected")} 1`)
                    : `${t("stats.selected")} ${selectedCampaignIds.size}`}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-[min(320px,calc(100vw-1rem))] p-2" align="start">
              <div className="space-y-1 max-h-64 overflow-y-auto">
                <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm font-medium border-b border-border pb-2 mb-1">
                  <Checkbox checked={selectedCampaignIds.size === 0} onCheckedChange={(checked) => {
                    if (checked) { setSelectedCampaignIds(new Set()); setSelectedCreativeIds(new Set()); }
                  }} />
                  {t("stats.allCampaigns")}
                </label>
                {activeCampaigns.map(c => (
                  <label key={c.id} className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                    <Checkbox checked={selectedCampaignIds.has(c.id)} onCheckedChange={() => toggleCampaign(c.id)} />
                    <span className="text-muted-foreground mr-1">{c.id}</span>
                    <span className="truncate">— {c.name}</span>
                  </label>
                ))}
              </div>
            </PopoverContent>
          </Popover>
        </div>

        <div className="flex min-w-0 flex-col gap-2">
          <Label className="text-sm text-muted-foreground font-medium">{t("stats.creatives")}</Label>
          <Popover onOpenChange={(open) => {
            if (open && selectedCampaignId) {
              void loadCampaignCreatives(selectedCampaignId).catch((error: unknown) => {
                toast.error(error instanceof Error ? error.message : String(error));
              });
            }
          }}>
            <PopoverTrigger asChild>
              <Button variant="outline" className="w-full min-w-0 justify-start bg-background border-border text-left font-normal sm:w-[260px]" disabled={!selectedCampaignId}>
                {!selectedCampaignId
                  ? t("stats.selectCreative")
                  : selectedCreativeIds.size === 0
                    ? t("stats.allCreatives")
                    : `${t("stats.selected")} ${selectedCreativeIds.size}`}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-[min(320px,calc(100vw-1rem))] p-2" align="start">
              <div className="space-y-1 max-h-64 overflow-y-auto">
                <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm font-medium border-b border-border pb-2 mb-1">
                  <Checkbox checked={selectedCreativeIds.size === 0} onCheckedChange={(checked) => { if (checked) setSelectedCreativeIds(new Set()); }} />
                  {t("stats.allCreatives")}
                </label>
                {availableCreatives.map(cr => (
                  <label key={cr.id} className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                    <Checkbox checked={selectedCreativeIds.has(cr.id)} onCheckedChange={() => {
                      setSelectedCreativeIds(prev => {
                        const next = new Set(prev);
                        if (next.has(cr.id)) next.delete(cr.id); else next.add(cr.id);
                        return next;
                      });
                    }} />
                    <span className="truncate">{cr.label}</span>
                  </label>
                ))}
              </div>
            </PopoverContent>
          </Popover>
        </div>

        <div className="flex min-w-0 flex-col gap-2">
          <Label className="text-sm text-muted-foreground font-medium">{t("stats.period")}</Label>
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Popover>
              <PopoverTrigger asChild>
                <Button variant="outline" className="w-full min-w-0 justify-start bg-background border-border text-left font-normal min-[420px]:w-[220px]">
                  <CalendarIcon className="mr-2 h-4 w-4" />
                  {dateRange?.from ? (
                    dateRange.to && dateRange.from.getTime() !== dateRange.to.getTime() ? (
                      <>{format(dateRange.from, "dd.MM.yy")} — {format(dateRange.to, "dd.MM.yy")}</>
                    ) : format(dateRange.from, "dd.MM.yy")
                  ) : t("stats.selectPeriod")}
                </Button>
              </PopoverTrigger>
              <PopoverContent className="w-auto p-0" align="start">
                <Calendar
                  mode="single"
                  onDayClick={handleDayClick}
                  selected={dateRange?.from}
                  modifiers={{
                    selected: (() => {
                      const from = dateRange?.from;
                      const to = dateRange?.to;
                      if (!from) return [];
                      if (!to) return [from];
                      const days: Date[] = [];
                      const start = new Date(from);
                      start.setHours(0, 0, 0, 0);
                      const end = new Date(to);
                      end.setHours(0, 0, 0, 0);
                      for (let d = new Date(start); d.getTime() <= end.getTime(); d.setDate(d.getDate() + 1)) {
                        days.push(new Date(d));
                      }
                      return days;
                    })(),
                  }}
                  numberOfMonths={isMobile ? 1 : 2}
                  className="p-3 pointer-events-auto"
                  classNames={{
                    cell: "h-9 w-9 text-center text-sm p-0 relative focus-within:relative focus-within:z-20",
                  }}
                />

              </PopoverContent>
            </Popover>
            {[
              { label: t("stats.today"), getRange: () => { const d = utcToday(); return { from: d, to: d }; } },
              { label: t("stats.yesterday"), getRange: () => { const d = subDays(utcToday(), 1); return { from: d, to: d }; } },
              { label: t("stats.week"), getRange: () => ({ from: subDays(utcToday(), 6), to: utcToday() }) },
              { label: t("stats.month"), getRange: () => ({ from: subDays(utcToday(), 29), to: utcToday() }) },
            ].map((preset) => (
              <Button key={preset.label} variant="outline" size="sm" className="border-border text-xs"
                onClick={() => setDateRange(preset.getRange())}>
                {preset.label}
              </Button>
            ))}
          </div>
        </div>
      </div>

      {/* Actions row — fixed placement so Show conversions never shifts on toggle */}
      <div className="flex flex-wrap items-center gap-3">
        <label
          className={cn(
            "inline-flex min-h-10 max-w-full items-center gap-2 rounded-md border px-3 cursor-pointer transition-colors select-none",
            showConversions
              ? "border-primary/60 bg-primary/10 text-primary"
              : "border-border bg-card text-muted-foreground hover:text-foreground"
          )}
        >
          <Zap className="h-4 w-4" />
          <span className="min-w-0 text-sm font-medium">{t("stats.showConversions")}</span>
          <Switch checked={showConversions} onCheckedChange={setShowConversions} />
        </label>

        <Button onClick={handleRefresh} className="bg-primary hover:bg-primary/90 text-primary-foreground gap-2">
          <RefreshCw className="h-4 w-4" /> {t("stats.refresh")}
        </Button>
        <Button onClick={handleDownloadCsv} variant="outline" className="border-border gap-2" disabled={!hasSelection || sortedData.length === 0}>
          <Download className="h-4 w-4" /> {t("stats.downloadCsv")}
        </Button>
      </div>

      <p className={cn("text-sm text-muted-foreground -mt-2", !showConversions && "invisible")}>
        {t("stats.postbackHint")}
      </p>

      {/* Filters */}
      <Card className="bg-card border-border">
        <CardContent className="p-4">
          <div className="flex items-center gap-2 mb-3">
            <Filter className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm font-medium">{t("stats.filters")}</span>
            {hasActiveFilters && (
              <Button variant="ghost" size="sm" className="text-xs h-6 px-2" onClick={() => { setFilterCountry(new Set()); setFilterBrowser(new Set()); setFilterDevice(new Set()); setFilterOS(new Set()); }}>
                {t("stats.clearFilters")}
              </Button>
            )}
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:gap-4">
            <MultiSelectFilter label={t("stats.filterCountry")} options={countryOptions} selected={filterCountry} onChange={setFilterCountry} />
            <MultiSelectFilter label={t("stats.filterBrowser")} options={DIMENSION_MAP.browsers} selected={filterBrowser} onChange={setFilterBrowser} />
            <MultiSelectFilter label={t("stats.filterDevice")} options={DIMENSION_MAP.devices} selected={filterDevice} onChange={setFilterDevice} />
            <MultiSelectFilter label={t("stats.filterOS")} options={DIMENSION_MAP.os} selected={filterOS} onChange={setFilterOS} />
          </div>
        </CardContent>
      </Card>

      {!hasSelection ? (
        <Card className="bg-card border-border">
          <CardContent className="py-16 text-center">
            <p className="text-muted-foreground">{t("stats.selectCampaignAndPeriod")}</p>
          </CardContent>
        </Card>
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 min-[480px]:grid-cols-2 lg:grid-cols-4">
            {metricCards.map((m) => (
              <Card key={m.label} className="bg-card border-border">
                <CardContent className="p-4 sm:p-6">
                  <div className="flex min-w-0 items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-sm text-muted-foreground">{m.label}</p>
                      <p className="mt-1 break-words text-2xl font-bold">{m.value}</p>
                    </div>
                    <div className="h-12 w-12 rounded-lg bg-muted flex items-center justify-center text-primary">
                      <m.icon className="h-6 w-6" />
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          {/* Chart */}
          {(appliedGroupBy === "dates" || appliedGroupBy === "hours") && chartData.length > 0 && (
            <Card className="bg-card border-border">
              <CardHeader>
                <div className="flex flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <CardTitle className="text-lg">
                    {appliedGroupBy === "hours" ? t("stats.chartTitleHours") : t("stats.chartTitle")}
                  </CardTitle>
                  <div className="flex max-w-full flex-wrap gap-1">
                    {(["impressions", "clicks", "spent"] as const).map(m => (
                      <Button key={m} variant={chartMetric === m ? "default" : "outline"} size="sm"
                        onClick={() => setChartMetric(m)}
                        className={cn("text-xs", chartMetric === m ? "bg-primary text-primary-foreground" : "border-border")}>
                        {m === "impressions" ? t("stats.impressions") : m === "clicks" ? t("stats.clicks") : t("stats.spent")}
                      </Button>
                    ))}
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <div className="h-[280px] animate-[reveal-chart_1.2s_ease-out_forwards]" style={{ clipPath: 'inset(0 0 0 0)' }}>
                  <style>{`
                    @keyframes reveal-chart {
                      from { clip-path: inset(0 100% 0 0); }
                      to { clip-path: inset(0 0 0 0); }
                    }
                  `}</style>
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart data={chartData}>
                      <defs>
                        <linearGradient id="grad-metric" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor="hsl(var(--primary))" stopOpacity={0.3} />
                          <stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                        </linearGradient>
                      </defs>
                      <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                      <XAxis
                        dataKey="label"
                        stroke="hsl(var(--muted-foreground))"
                        fontSize={appliedGroupBy === "hours" ? 10 : 12}
                        tickFormatter={appliedGroupBy === "hours" ? (val: string) => val.split(" ")[1] || val : undefined}
                        interval={appliedGroupBy === "hours" ? "preserveStartEnd" : undefined}
                      />
                      <YAxis stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      {appliedGroupBy === "hours" ? (
                        <Tooltip content={<HoursTooltip />} />
                      ) : (
                        <Tooltip contentStyle={{ backgroundColor: "hsl(var(--card))", border: "1px solid hsl(var(--border))", borderRadius: "8px", color: "hsl(var(--foreground))" }} />
                      )}
                      <Area type="monotone" dataKey={chartMetric} stroke="hsl(var(--primary))" fill="url(#grad-metric)" strokeWidth={2} isAnimationActive={false} />
                    </AreaChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>
          )}

          <Card className="bg-card border-border">
            <CardHeader>
                <div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
                <div className="grid min-w-0 grid-cols-2 gap-2 sm:flex sm:flex-wrap sm:items-center">
                  {(Object.keys(groupLabels) as GroupBy[]).map((g) => (
                    <Button key={g} variant={groupBy === g ? "default" : "outline"} size="sm"
                      onClick={() => {
                        if (groupBy === g) return;
                        pendingScrollRef.current = window.scrollY;
                        setGroupBy(g);
                      }}
                      className={cn("h-auto min-h-9 min-w-0 whitespace-normal py-2 sm:min-w-[100px]", groupBy === g ? "bg-primary text-primary-foreground" : "border-border")}>
                      {groupLabels[g]}
                    </Button>
                  ))}
                </div>
                <div className="flex min-w-0 flex-wrap items-center gap-3">
                  <Popover>
                    <PopoverTrigger asChild>
                      <Button variant="outline" size="sm" className="border-border gap-2">
                        <Filter className="h-3.5 w-3.5" /> {t("stats.columns")}
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-64 p-3 max-h-[70vh] overflow-y-auto" align="end">
                      <div className="space-y-3">
                        <div>
                          <div className="text-[11px] uppercase tracking-wide text-muted-foreground/80 px-1 mb-1">{t("stats.groupTraffic")}</div>
                          <div className="space-y-0.5">
                            <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                              <Checkbox checked={showImpressions} onCheckedChange={(c) => setShowImpressions(!!c)} />
                              {t("stats.impressions")}
                            </label>
                            <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                              <Checkbox checked={showClicks} onCheckedChange={(c) => setShowClicks(!!c)} />
                              {t("stats.clicks")}
                            </label>
                            <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                              <Checkbox checked={showCtr} onCheckedChange={(c) => setShowCtr(!!c)} />
                              {t("stats.ctr")}
                            </label>
                          </div>
                        </div>
                        <div>
                          <div className="text-[11px] uppercase tracking-wide text-muted-foreground/80 px-1 mb-1">{t("stats.groupCost")}</div>
                          <div className="space-y-0.5">
                            <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                              <Checkbox checked={showSpent} onCheckedChange={(c) => setShowSpent(!!c)} />
                              {t("stats.spent")}
                            </label>
                            <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                              <Checkbox checked={showCpm} onCheckedChange={(c) => setShowCpm(!!c)} />
                              {t("stats.cpm")}
                            </label>
                            <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-muted/50 cursor-pointer text-sm">
                              <Checkbox checked={showCpc} onCheckedChange={(c) => setShowCpc(!!c)} />
                              {t("stats.cpc")}
                            </label>
                          </div>
                        </div>
                        <div>
                          <div className={cn("text-[11px] uppercase tracking-wide px-1 mb-1", showConversions ? "text-muted-foreground/80" : "text-muted-foreground/40")}>{t("stats.groupConversions")}</div>
                          <div className={cn("space-y-0.5", !showConversions && "opacity-50")}>
                            <label className={cn("flex items-center gap-2 px-2 py-1.5 rounded text-sm", showConversions ? "hover:bg-muted/50 cursor-pointer" : "")}>
                              <Checkbox checked={showConversionsCol} disabled={!showConversions} onCheckedChange={(c) => setShowConversionsCol(!!c)} />
                              {t("stats.conversions")}
                            </label>
                            <label className={cn("flex items-center gap-2 px-2 py-1.5 rounded text-sm", showConversions ? "hover:bg-muted/50 cursor-pointer" : "")}>
                              <Checkbox checked={showConfirmedConversions} disabled={!showConversions} onCheckedChange={(c) => setShowConfirmedConversions(!!c)} />
                              {t("stats.confirmedConversions")}
                            </label>
                            <label className={cn("flex items-center gap-2 px-2 py-1.5 rounded text-sm", showConversions ? "hover:bg-muted/50 cursor-pointer" : "")}>
                              <Checkbox checked={showCr} disabled={!showConversions} onCheckedChange={(c) => setShowCr(!!c)} />
                              {t("stats.cr")}
                            </label>
                            <label className={cn("flex items-center gap-2 px-2 py-1.5 rounded text-sm", showConversions ? "hover:bg-muted/50 cursor-pointer" : "")}>
                              <Checkbox checked={showIncome} disabled={!showConversions} onCheckedChange={(c) => setShowIncome(!!c)} />
                              {t("stats.income")}
                            </label>
                            <label className={cn("flex items-center gap-2 px-2 py-1.5 rounded text-sm", showConversions ? "hover:bg-muted/50 cursor-pointer" : "")}>
                              <Checkbox checked={showConfirmedIncome} disabled={!showConversions} onCheckedChange={(c) => setShowConfirmedIncome(!!c)} />
                              {t("stats.confirmedIncome")}
                            </label>
                            <label className={cn("flex items-center gap-2 px-2 py-1.5 rounded text-sm", showConversions ? "hover:bg-muted/50 cursor-pointer" : "")}>
                              <Checkbox checked={showRoi} disabled={!showConversions} onCheckedChange={(c) => setShowRoi(!!c)} />
                              {t("stats.roi")}
                            </label>
                          </div>
                        </div>
                      </div>
                    </PopoverContent>
                  </Popover>
                  <div className="flex min-w-0 flex-wrap items-center gap-1">
                    <span className="text-xs text-muted-foreground mr-1">{t("stats.rows")}</span>
                    {([50, 100, "all"] as PageSize[]).map(sz => (
                      <Button key={String(sz)} size="sm"
                        variant={pageSize === sz ? "default" : "outline"}
                        onClick={() => {
                          if (pageSize === sz) return;
                          pendingScrollRef.current = window.scrollY;
                          setPageSize(sz);
                        }}
                        className={cn("min-w-[52px]", pageSize === sz ? "bg-primary text-primary-foreground" : "border-border")}>
                        {sz === "all" ? t("stats.rowsAll") : sz}
                      </Button>
                    ))}
                  </div>
                </div>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              {data.length === 0 ? (
                <div className="py-16 text-center text-muted-foreground"><p>{t("stats.noData")}</p></div>
              ) : (
                <div className="overflow-x-auto overflow-y-hidden">
                  {(() => {
                    const trafficCols = (showImpressions ? 1 : 0) + (showClicks ? 1 : 0) + (showCtr ? 1 : 0);
                    const costCols = (showSpent ? 1 : 0) + (showCpm ? 1 : 0) + (showCpc ? 1 : 0);
                    const convCols = showConversions
                      ? (showConversionsCol ? 1 : 0) + (showConfirmedConversions ? 1 : 0) + (showCr ? 1 : 0) + (showIncome ? 1 : 0) + (showConfirmedIncome ? 1 : 0) + (showRoi ? 1 : 0)
                      : 0;
                    const stickyCell = "sticky left-0 z-10";
                    const stickyHead = `${stickyCell} bg-card`;
                    const stickyBody = `${stickyCell} bg-card`;
                    const stickyAlt  = `${stickyCell} bg-[hsl(var(--muted)/0.3)]`;
                    // subtle vertical separator between column groups
                    const sep = "border-l border-border/60";
                    // First-visible-in-group helpers so the vertical separator lands on the right cell.
                    const firstCost: "spent" | "cpm" | "cpc" | null = showSpent ? "spent" : showCpm ? "cpm" : showCpc ? "cpc" : null;
                    const firstConv: "conversions" | "confirmedConv" | "cr" | "income" | "confirmedIncome" | "roi" | null =
                      showConversions
                        ? (showConversionsCol ? "conversions"
                          : showConfirmedConversions ? "confirmedConv"
                          : showCr ? "cr"
                          : showIncome ? "income"
                          : showConfirmedIncome ? "confirmedIncome"
                          : showRoi ? "roi"
                          : null)
                        : null;
                    const fmtMoney = (n: number) => `$${formatNumberWithDot(n, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
                    const fmtMoney2 = (n: number) => `$${formatNumberWithDot(Math.floor(n * 100) / 100, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
                    const fmtMoney5 = (n: number) => `$${formatNumberWithDot(Math.floor(n * 100000) / 100000, { minimumFractionDigits: 5, maximumFractionDigits: 5 })}`;
                    const cpmOf = (r: { spent: number; impressions: number }) => r.impressions > 0 ? r.spent / r.impressions * 1000 : 0;
                    const cpcOf = (r: { spent: number; clicks: number }) => r.clicks > 0 ? r.spent / r.clicks : 0;
                    return (
                  <table className="w-full min-w-[640px] border-collapse">
                    <thead>
                      {/* Group header row */}
                      <tr className="border-b border-border/60 text-[11px] uppercase tracking-wide text-muted-foreground/70">
                        <th className={cn("py-1.5 px-2 text-left", stickyHead)}></th>
                        {trafficCols > 0 && (
                          <th colSpan={trafficCols} className="py-1.5 px-2 text-left">{t("stats.groupTraffic")}</th>
                        )}
                        {costCols > 0 && (
                          <th colSpan={costCols} className={cn("py-1.5 px-2 text-left", trafficCols > 0 && sep)}>{t("stats.groupCost")}</th>
                        )}
                        {showConversions && convCols > 0 && (
                          <th colSpan={convCols} className={cn("py-1.5 px-2 text-left", (trafficCols + costCols) > 0 && sep)}>{t("stats.groupConversions")}</th>
                        )}
                      </tr>
                      <tr className="border-b border-border">
                        <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground whitespace-nowrap", stickyHead, canSortByLabel && "cursor-pointer select-none")}
                          onClick={() => canSortByLabel && toggleSort("label")}>
                          {labelHeader} {canSortByLabel && <SortIcon col="label" />}
                        </th>
                        {showImpressions && (
                          <th className="text-left py-2 px-2 text-sm font-medium text-muted-foreground cursor-pointer select-none whitespace-nowrap" onClick={() => toggleSort("impressions")}>
                            {t("stats.impressions")} <SortIcon col="impressions" />
                          </th>
                        )}
                        {showClicks && (
                          <th className="text-left py-2 px-2 text-sm font-medium text-muted-foreground cursor-pointer select-none whitespace-nowrap" onClick={() => toggleSort("clicks")}>
                            {t("stats.clicks")} <SortIcon col="clicks" />
                          </th>
                        )}
                        {showCtr && (
                          <th className="text-left py-2 px-2 text-sm font-medium text-muted-foreground whitespace-nowrap">{t("stats.ctr")}</th>
                        )}
                        {showSpent && (
                          <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground cursor-pointer select-none whitespace-nowrap", firstCost === "spent" && trafficCols > 0 && sep)} onClick={() => toggleSort("spent")}>
                            {t("stats.spent")} <SortIcon col="spent" />
                          </th>
                        )}
                        {showCpm && (
                          <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground cursor-pointer select-none whitespace-nowrap", firstCost === "cpm" && trafficCols > 0 && sep)} onClick={() => toggleSort("cpm")}>
                            {t("stats.cpm")} <SortIcon col="cpm" />
                          </th>
                        )}
                        {showCpc && (
                          <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground cursor-pointer select-none whitespace-nowrap", firstCost === "cpc" && trafficCols > 0 && sep)} onClick={() => toggleSort("cpc")}>
                            {t("stats.cpc")} <SortIcon col="cpc" />
                          </th>
                        )}
                        {showConversions && (
                          <>
                            {showConversionsCol && (
                              <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground cursor-pointer select-none whitespace-nowrap", firstConv === "conversions" && (trafficCols + costCols) > 0 && sep)} onClick={() => toggleSort("conversions")}>
                                {t("stats.conversions")} <SortIcon col="conversions" />
                              </th>
                            )}
                            {showConfirmedConversions && (
                              <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground whitespace-nowrap", firstConv === "confirmedConv" && (trafficCols + costCols) > 0 && sep)}>{t("stats.confirmed")}</th>
                            )}
                            {showCr && (
                              <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground whitespace-nowrap", firstConv === "cr" && (trafficCols + costCols) > 0 && sep)}>{t("stats.cr")}</th>
                            )}
                            {showIncome && (
                              <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground cursor-pointer select-none whitespace-nowrap", firstConv === "income" && (trafficCols + costCols) > 0 && sep)} onClick={() => toggleSort("income")}>
                                {t("stats.income")} <SortIcon col="income" />
                              </th>
                            )}
                            {showConfirmedIncome && (
                              <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground whitespace-nowrap", firstConv === "confirmedIncome" && (trafficCols + costCols) > 0 && sep)}>{t("stats.confirmed")}</th>
                            )}
                            {showRoi && (
                              <th className={cn("text-left py-2 px-2 text-sm font-medium text-muted-foreground whitespace-nowrap", firstConv === "roi" && (trafficCols + costCols) > 0 && sep)}>{t("stats.roi")}</th>
                            )}
                          </>
                        )}
                      </tr>
                    </thead>
                    <tbody>
                      {visibleRows.map((row) => {
                        const cr = row.clicks > 0 ? ((row.conversions / row.clicks) * 100).toFixed(2) : "0.00";
                        const roiNum = row.spent > 0 ? ((row.confirmedIncome - row.spent) / row.spent) * 100 : 0;
                        const roi = row.spent > 0 ? roiNum.toFixed(2) : "0.00";
                        return (
                        <tr key={row.label} className="group border-b border-border/50 hover:bg-muted/50 transition-colors">
                          <td className={cn("py-2 px-2 font-medium whitespace-nowrap", stickyBody, "group-hover:bg-muted/50")}>
                            {appliedGroupBy === "country" ? formatCountryLabel(row.label, lang) : row.label}
                          </td>
                          {showImpressions && <td className="py-2 px-2 whitespace-nowrap">{formatStatisticInteger(row.impressions)}</td>}
                          {showClicks && <td className="py-2 px-2 whitespace-nowrap">{formatStatisticInteger(row.clicks)}</td>}
                          {showCtr && <td className="py-2 px-2 whitespace-nowrap">{row.impressions > 0 ? ((row.clicks / row.impressions) * 100).toFixed(2) : "0.00"}%</td>}
                          {showSpent && <td className={cn("py-2 px-2 whitespace-nowrap", firstCost === "spent" && trafficCols > 0 && sep)}>{fmtMoney(row.spent)}</td>}
                          {showCpm && <td className={cn("py-2 px-2 whitespace-nowrap", firstCost === "cpm" && trafficCols > 0 && sep)}>{fmtMoney2(cpmOf(row))}</td>}
                          {showCpc && <td className={cn("py-2 px-2 whitespace-nowrap", firstCost === "cpc" && trafficCols > 0 && sep)}>{fmtMoney5(cpcOf(row))}</td>}
                          {showConversions && (
                            <>
                              {showConversionsCol && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "conversions" && (trafficCols + costCols) > 0 && sep)}>{formatStatisticInteger(row.conversions)}</td>}
                              {showConfirmedConversions && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "confirmedConv" && (trafficCols + costCols) > 0 && sep)}>{formatStatisticInteger(row.confirmedConversions)}</td>}
                              {showCr && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "cr" && (trafficCols + costCols) > 0 && sep)}>{cr}%</td>}
                              {showIncome && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "income" && (trafficCols + costCols) > 0 && sep)}>{fmtMoney(row.income)}</td>}
                              {showConfirmedIncome && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "confirmedIncome" && (trafficCols + costCols) > 0 && sep)}>{fmtMoney(row.confirmedIncome)}</td>}
                              {showRoi && <td className={cn("py-2 px-2 whitespace-nowrap font-medium", firstConv === "roi" && (trafficCols + costCols) > 0 && sep, roiNum > 0 ? "text-emerald-500" : roiNum < 0 ? "text-red-500" : "")}>{roi}%</td>}
                            </>
                          )}
                        </tr>
                        );
                      })}
                      <tr className="bg-muted/30 font-semibold">
                        <td className={cn("py-2 px-2 whitespace-nowrap", stickyAlt)}>{t("stats.total")}</td>
                        {showImpressions && <td className="py-2 px-2 whitespace-nowrap">{formatStatisticInteger(totals.impressions)}</td>}
                        {showClicks && <td className="py-2 px-2 whitespace-nowrap">{formatStatisticInteger(totals.clicks)}</td>}
                        {showCtr && <td className="py-2 px-2 whitespace-nowrap">{totals.impressions > 0 ? ((totals.clicks / totals.impressions) * 100).toFixed(2) : "0.00"}%</td>}
                        {showSpent && <td className={cn("py-2 px-2 whitespace-nowrap", firstCost === "spent" && trafficCols > 0 && sep)}>{fmtMoney(totals.spent)}</td>}
                        {showCpm && <td className={cn("py-2 px-2 whitespace-nowrap", firstCost === "cpm" && trafficCols > 0 && sep)}>{fmtMoney2(cpmOf(totals))}</td>}
                        {showCpc && <td className={cn("py-2 px-2 whitespace-nowrap", firstCost === "cpc" && trafficCols > 0 && sep)}>{fmtMoney5(cpcOf(totals))}</td>}
                        {showConversions && (() => {
                          const cr = totals.clicks > 0 ? ((totals.conversions / totals.clicks) * 100).toFixed(2) : "0.00";
                          const roiNum = totals.spent > 0 ? ((totals.confirmedIncome - totals.spent) / totals.spent) * 100 : 0;
                          const roi = totals.spent > 0 ? roiNum.toFixed(2) : "0.00";
                          return (
                            <>
                              {showConversionsCol && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "conversions" && (trafficCols + costCols) > 0 && sep)}>{formatStatisticInteger(totals.conversions)}</td>}
                              {showConfirmedConversions && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "confirmedConv" && (trafficCols + costCols) > 0 && sep)}>{formatStatisticInteger(totals.confirmedConversions)}</td>}
                              {showCr && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "cr" && (trafficCols + costCols) > 0 && sep)}>{cr}%</td>}
                              {showIncome && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "income" && (trafficCols + costCols) > 0 && sep)}>{fmtMoney(totals.income)}</td>}
                              {showConfirmedIncome && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "confirmedIncome" && (trafficCols + costCols) > 0 && sep)}>{fmtMoney(totals.confirmedIncome)}</td>}
                              {showRoi && <td className={cn("py-2 px-2 whitespace-nowrap", firstConv === "roi" && (trafficCols + costCols) > 0 && sep, roiNum > 0 ? "text-emerald-500" : roiNum < 0 ? "text-red-500" : "")}>{roi}%</td>}
                            </>
                          );
                        })()}
                      </tr>
                    </tbody>
                  </table>
                    );
                  })()}
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
