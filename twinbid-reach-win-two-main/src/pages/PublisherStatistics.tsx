import { useEffect, useMemo, useRef, useState } from "react";
import { format, parseISO, subDays } from "date-fns";
import { ru, enUS, zhCN } from "date-fns/locale";
import type { DateRange } from "react-day-picker";
import { ArrowUpDown, CalendarIcon, RefreshCw, Filter, Download, Eye, MousePointer, Percent, DollarSign } from "lucide-react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import {
  publisherApi,
  type PublisherGroup,
  type PublisherMetrics,
  type PublisherRow,
  type PublisherStatsRequest,
} from "@/api/publisher";
import { publisherTimeLabel } from "@/lib/publisherTimeSeries";
import { ApiError } from "@/api/http";
import {
  usePublisher,
  usePublisherResource,
} from "@/contexts/PublisherContext";
import { useProfile } from "@/contexts/ProfileContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { PublisherStatsFilter } from "@/components/publisher/PublisherStatsFilter";
import { Checkbox } from "@/components/ui/checkbox";
import { COUNTRY_CODES } from "@/lib/dimensions";
import { BROWSER_FILTER_MAP, OS_FILTER_MAP } from "@/lib/statFilters";
import { useIsMobile } from "@/hooks/use-mobile";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import {
  publisherCpm,
  publisherLocale,
  publisherDate,
  publisherPeriod,
  publisherTimezone,
  publisherTimezoneLabel,
  formatPublisherMoney,
  comparePublisherMoney,
} from "@/lib/publisherData";
import { formatCountryLabel } from "@/lib/countries";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Calendar } from "@/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  PublisherAsync,
  PublisherError,
  PublisherPagination,
} from "@/components/publisher/PublisherApiState";
const GROUPS = [
  ["date", "Дата", "Date", "日期"],
  ["hour", "Час", "Hour", "小时"],
  ["format", "Формат", "Format", "格式"],
  ["country", "Страна", "Country", "国家"],
  ["device", "Устройство", "Device", "设备"],
  ["os", "ОС", "Operating system", "操作系统"],
  ["browser", "Браузер", "Browser", "浏览器"],
  ["site", "Сайт", "Site", "网站"],
  ["zone", "Зона", "Zone", "广告位"],
  ["ad_code", "Код", "Ad code", "广告代码"],
] as const;
const METRICS = [
  ["impressions", "Показы", "Impressions", "展示"],
  ["clicks", "Клики", "Clicks", "点击"],
  ["ctr", "CTR", "CTR", "CTR"],
  ["revenue_usd", "Доход", "Revenue", "收益"],
  ["cpm", "CPM", "CPM", "CPM"],
] as const;
type MetricKey = keyof PublisherMetrics | "cpm";
function dayInTimezone(date: Date, timezone: string): string {
  const parts = new Intl.DateTimeFormat("en", {
    timeZone: timezone, year: "numeric", month: "2-digit", day: "2-digit",
  }).formatToParts(date);
  const part = (type: string) => parts.find(item => item.type === type)!.value;
  return `${part("year")}-${part("month")}-${part("day")}`;
}
function todayInTimezone(timezone: string): Date {
  return parseISO(dayInTimezone(new Date(), timezone));
}
function values(text: string) {
  const list = text
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  if (list.length > 64 || list.some((s) => s.length > 100))
    throw new ApiError(
      400,
      "At most 64 values of up to 100 characters per filter",
    );
  return list;
}
export default function PublisherStatistics() {
  const isMobile = useIsMobile();
  const [columns, setColumns] = useState(new Set<MetricKey>(["impressions","clicks","ctr","revenue_usd","cpm"]));
  const { lang } = useLanguage(),
    { profile, loading: profileLoading } = useProfile(),
    { adCodes, error: codesError, refetch: refetchCodes } = usePublisher();
  const [range, setRange] = useState<DateRange | undefined>(() => ({
    from: subDays(todayInTimezone("UTC"), 6),
    to: todayInTimezone("UTC"),
  }));
  const [timezone, setTimezone] = useState("UTC");
  const [group, setGroup] = useState<PublisherGroup>("date"),
    [site, setSite] = useState(""),
    [zone, setZone] = useState(""),
    [code, setCode] = useState("");
  const [countries, setCountries] = useState(""),
    [devices, setDevices] = useState(""),
    [oses, setOses] = useState(""),
    [browsers, setBrowsers] = useState("");
  const [request, setRequest] = useState<PublisherStatsRequest | null>(null),
    [inputError, setInputError] = useState<unknown>(null),
    [offset, setOffset] = useState(0);
  const [metric, setMetric] = useState<MetricKey>("revenue_usd"),
    [sort, setSort] = useState<MetricKey | "bucket">("bucket"),
    [desc, setDesc] = useState(false);
  const initialized = useRef(false);
  useEffect(() => {
    if (profileLoading) return;
    try {
      const tz = publisherTimezone(profile?.timezone);
      setTimezone(tz);
      const today = todayInTimezone(tz);
      if (!initialized.current) {
        setRange({ from: subDays(today, 6), to: today });
        initialized.current = true;
      }
      setOffset(0);
      setRequest(previous => {
        if (previous?.timezone === tz) return previous;
        // Keep the applied calendar days and filters when the profile timezone changes.
        const first = previous
          ? dayInTimezone(new Date(previous.from), previous.timezone || "UTC")
          : format(subDays(today, 6), "yyyy-MM-dd");
        const last = previous
          ? dayInTimezone(new Date(new Date(previous.to).getTime() - 1), previous.timezone || "UTC")
          : format(today, "yyyy-MM-dd");
        return {
          ...previous,
          ...publisherPeriod(first, last, tz),
          timezone: tz,
          group_by: previous?.group_by || "date",
          limit: 50,
          offset: 0,
        };
      });
    } catch (error) {
      setInputError(error);
    }
  }, [profileLoading, profile?.timezone]);
  const temporal = request?.group_by === "hour" || request?.group_by === "date";
  const body = request ? { ...request, offset: temporal ? 0 : offset } : null;
  const query = usePublisherResource(
    ["stats", body],
    (signal) => temporal ? publisherApi.statsSeries(body!, signal) : publisherApi.stats(body!, signal),
    !!body,
  );
  const sites = usePublisherResource(
    ["sites", { limit: 50, offset: 0 }],
    (signal) => publisherApi.sites({ limit: 50, offset: 0 }, signal),
  );
  const zones = usePublisherResource(["filter-zones", site, code], (signal) =>
    publisherApi.zones(
      {
        limit: 50,
        offset: 0,
        site_id: site.trim() || undefined,
        ad_code_id: code.trim() || undefined,
      },
      signal,
    ),
  );
  const apply = () => {
    try {
      if (!range?.from) throw new ApiError(400, "Select a period");
      const tz = publisherTimezone(timezone),
        next: PublisherStatsRequest = {
          ...publisherPeriod(
            format(range.from, "yyyy-MM-dd"),
            format(range.to || range.from, "yyyy-MM-dd"),
            tz,
          ),
          timezone: tz,
          group_by: group,
          site_id: site.trim() || undefined,
          zone_id: zone.trim() || undefined,
          ad_code_id: code.trim() || undefined,
          countries: values(countries),
          devices: values(devices),
          oses: values(oses),
          browsers: values(browsers),
          limit: 50,
          offset: 0,
        };
      setOffset(0);
      setInputError(null);
      if (JSON.stringify(body) === JSON.stringify(next)) void query.refetch();
      else setRequest(next);
    } catch (error) {
      setInputError(error);
    }
  };
  const label = (row: PublisherRow) =>
    (request?.group_by === "date" || request?.group_by === "hour")
      ? publisherTimeLabel(row, request.group_by, request.timezone)
      : request?.group_by === "country"
      ? formatCountryLabel(row.bucket, lang)
      : request?.group_by === "site"
        ? row.hostname || row.bucket
        : request?.group_by === "ad_code"
          ? adCodes.find((c) => c.ad_code_id === row.bucket)?.name || row.bucket
          : row.bucket || "—";
  const sortedRows = useMemo(
    () =>
      [...(query.data?.rows || [])].sort((a, b) => {
        const compare =
          sort === "bucket"
            ? a.bucket.localeCompare(b.bucket)
            : sort === "revenue_usd"
              ? comparePublisherMoney(a.revenue_usd, b.revenue_usd)
              : sort === "cpm"
                ? comparePublisherMoney(publisherCpm(a), publisherCpm(b))
                : a[sort] - b[sort];
        return desc ? -compare : compare;
      }),
    [query.data?.rows, sort, desc],
  );
  const rows = temporal ? sortedRows.slice(offset, offset + 50) : sortedRows;
  const display = (data: PublisherMetrics, key: MetricKey) =>
    key === "cpm" ? "$" + formatPublisherMoney(publisherCpm(data), lang) : key === "revenue_usd"
      ? "$" + formatPublisherMoney(data[key], lang)
      : data[key].toLocaleString(publisherLocale(lang), {
          maximumFractionDigits: key === "ctr" ? 2 : 0,
        }) + (key === "ctr" ? "%" : "");
  // Approximation is only a visual coordinate; financial totals/tooltips retain decimal strings.
  const chart = [...(query.data?.rows || [])]
    .sort((a, b) => a.bucket.localeCompare(b.bucket))
    .map((row) => ({ ...row, chartLabel: label(row), plot: Number(metric === "cpm" ? publisherCpm(row) : row[metric]) }));
  const selectedMetrics = METRICS.filter(([key]) => columns.has(key));
  const groupTitle = GROUPS.find(([value]) => value === request?.group_by);
  const exportCsv = () => {
    const cell = (v: string) => '"' + (/^[=+@-]/.test(v) ? "'" + v : v).replace(/"/g, '""') + '"';
    const records = [[tr(lang, "Группа", "Group", "分组"), ...selectedMetrics.map(([, ruText, en, zh]) => tr(lang, ruText, en, zh))], ...sortedRows.map(row => [label(row), ...selectedMetrics.map(([key]) => display(row, key))])];
    const url = URL.createObjectURL(new Blob(["\uFEFF" + records.map(row => row.map(cell).join(",")).join("\r\n")], { type: "text/csv;charset=utf-8" }));
    const link = document.createElement("a"); link.href = url; link.download = "publisher-statistics.csv"; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return (
    <div className="space-y-6">
      <div><h1 className="text-2xl font-bold">{tr(lang, "Статистика", "Statistics", "统计数据")}</h1><p className="text-sm text-muted-foreground">{tr(lang, "Подробная аналитика ваших сайтов и рекламных кодов", "Detailed analytics for your sites and ad codes", "网站和广告代码的详细分析")}</p></div>
      <div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-end sm:gap-6">
        <PublisherStatsFilter id="publisher-site-filter" label={tr(lang,"Сайт (ID)","Site (ID)","网站 (ID)")} options={(sites.data?.rows || []).map(r => ({value:r.site_id,label:r.hostname || r.site_id}))} value={site} onChange={value => {setSite(value);setZone("");}} />
        <PublisherStatsFilter id="publisher-code-filter" label={tr(lang,"Рекламный код (ID)","Ad code (ID)","广告代码 (ID)")} options={adCodes.map(c => ({value:c.ad_code_id,label:c.name || c.short_code}))} value={code} onChange={value => {setCode(value);setZone("");}} />
        <PublisherStatsFilter id="publisher-zone-filter" label={tr(lang,"Зона (ID)","Zone (ID)","广告位 (ID)")} options={(zones.data?.rows || []).map(r=>({value:r.zone_id,label:`${r.hostname || r.site_id} · ${r.zone_id}`}))} value={zone} onChange={setZone} />
        <div className="flex min-w-0 flex-col gap-2"><Label className="text-sm font-medium text-muted-foreground">{tr(lang,"Период","Period","期间")} · {publisherTimezoneLabel(timezone)}</Label><div className="flex min-w-0 flex-wrap items-center gap-2">
          <Popover><PopoverTrigger asChild><Button variant="outline" className="w-full min-w-0 justify-start border-border bg-background text-left font-normal min-[420px]:w-[220px]"><CalendarIcon className="mr-2 h-4 w-4" />{range?.from ? format(range.from,"dd.MM.yy") : "—"} — {range?.to ? format(range.to,"dd.MM.yy") : "—"}</Button></PopoverTrigger><PopoverContent className="w-auto max-w-[calc(100vw-1rem)] p-0" align="start"><Calendar mode="range" selected={range} onSelect={setRange} numberOfMonths={isMobile ? 1 : 2} locale={lang === "ru" ? ru : lang === "zh-CN" ? zhCN : enUS} className="p-3 pointer-events-auto" /></PopoverContent></Popover>
          {[[0,0,tr(lang,"Сегодня","Today","今天")],[1,1,tr(lang,"Вчера","Yesterday","昨天")],[6,0,tr(lang,"Неделя","Week","一周")],[29,0,tr(lang,"Месяц","Month","一个月")]].map(([from,to,title])=><Button key={String(title)} variant="outline" size="sm" className="border-border text-xs" onClick={()=>setRange({from:subDays(todayInTimezone(timezone),Number(from)),to:subDays(todayInTimezone(timezone),Number(to))})}>{title}</Button>)}
        </div></div>
      </div>
      <div className="flex flex-wrap items-center gap-3"><Button onClick={apply} disabled={profileLoading || (!!body && query.isFetching)} className="gap-2 bg-primary text-primary-foreground hover:bg-primary/90"><RefreshCw className={`h-4 w-4 ${query.isFetching ? "animate-spin" : ""}`} />{tr(lang,"Обновить","Refresh","刷新")}</Button><Button onClick={exportCsv} variant="outline" className="gap-2 border-border" disabled={!rows.length}><Download className="h-4 w-4" />{tr(lang,"Скачать CSV","Download CSV","下载 CSV")}</Button></div>
      <Card className="border-border bg-card"><CardContent className="p-4"><div className="mb-3 flex items-center gap-2"><Filter className="h-4 w-4 text-muted-foreground" /><span className="text-sm font-medium">{tr(lang,"Фильтры","Filters","筛选")}</span>{[countries,browsers,devices,oses].some(Boolean) && <Button variant="ghost" size="sm" className="h-6 px-2 text-xs" onClick={()=>{setCountries("");setBrowsers("");setDevices("");setOses("");}}>{tr(lang,"Сбросить","Clear filters","清除筛选")}</Button>}</div><div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:gap-4">
        <PublisherStatsFilter compact multiple id="publisher-countries" label={tr(lang,"Страна","Country","国家")} options={COUNTRY_CODES.map(value=>({value,label:formatCountryLabel(value,lang)}))} value={countries} onChange={setCountries} />
        <PublisherStatsFilter compact multiple id="publisher-browsers" label={tr(lang,"Браузер","Browser","浏览器")} options={Array.from(new Set(Object.values(BROWSER_FILTER_MAP).flat())).map(value=>({value,label:value}))} value={browsers} onChange={setBrowsers} />
        <PublisherStatsFilter compact multiple id="publisher-devices" label={tr(lang,"Устройство","Device","设备")} options={["mobile","tablet","desktop"].map(value=>({value,label:value}))} value={devices} onChange={setDevices} />
        <PublisherStatsFilter compact multiple id="publisher-oses" label={tr(lang,"ОС","Operating system","操作系统")} options={Array.from(new Set(Object.values(OS_FILTER_MAP).flat())).map(value=>({value,label:value}))} value={oses} onChange={setOses} />
      </div></CardContent></Card>
      {codesError && <PublisherError error={codesError} retry={refetchCodes} />}{sites.error && <PublisherError error={sites.error} retry={sites.refetch} />}{zones.error && <PublisherError error={zones.error} retry={zones.refetch} />}{inputError && <PublisherError error={inputError} />}
      <PublisherAsync loading={profileLoading || (!!body && query.isPending)} error={query.error} retry={query.refetch}>
      {query.data && <>
        <p className="text-xs text-muted-foreground">{tr(lang,"Период:","Period:","期间：")} {publisherDate(query.data.effective_from,lang,request?.timezone)} — {publisherDate(query.data.effective_to,lang,request?.timezone)}</p>
        <div className="grid grid-cols-1 gap-4 min-[480px]:grid-cols-2 lg:grid-cols-4">{METRICS.filter(([key]) => key !== "cpm").map(([key,ruText,en,zh],index)=>{const Icon=[Eye,MousePointer,Percent,DollarSign][index];return <Card key={key} className="border-border bg-card"><CardContent className="p-4 sm:p-6"><div className="flex min-w-0 items-center justify-between gap-3"><div className="min-w-0"><p className="text-sm text-muted-foreground">{tr(lang,ruText,en,zh)}</p><p className="mt-1 break-words text-2xl font-bold [overflow-wrap:anywhere]">{display(query.data.totals,key)}</p></div><div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-muted text-primary"><Icon className="h-6 w-6" /></div></div></CardContent></Card>;})}</div>
        {(request?.group_by === "date" || request?.group_by === "hour") && chart.length > 0 && <Card className="border-border bg-card"><CardHeader><div className="flex flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between"><CardTitle className="text-lg">{request.group_by === "hour" ? tr(lang,"Динамика по часам","Hourly performance","每小时表现") : tr(lang,"Динамика по дням","Daily performance","每日表现")}</CardTitle><div className="flex max-w-full flex-wrap gap-1">{METRICS.map(([key,ruText,en,zh])=><Button key={key} size="sm" variant={metric===key?"default":"outline"} className="border-border text-xs" onClick={()=>setMetric(key)}>{tr(lang,ruText,en,zh)}</Button>)}</div></div></CardHeader><CardContent><div className="h-[280px]"><ResponsiveContainer width="100%" height="100%"><AreaChart data={chart}><defs><linearGradient id="pubFill" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="hsl(var(--primary))" stopOpacity={0.3}/><stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0}/></linearGradient></defs><CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))"/><XAxis dataKey="chartLabel" minTickGap={24} interval="preserveStartEnd" stroke="hsl(var(--muted-foreground))" fontSize={12}/><YAxis stroke="hsl(var(--muted-foreground))" fontSize={12}/><Tooltip content={({active,payload})=>active && payload?.[0]?.payload ? <div className="rounded-lg border bg-card p-3 text-sm"><p>{payload[0].payload.chartLabel}</p><p>{display(payload[0].payload as PublisherRow,metric)}</p></div>:null}/><Area type="monotone" dataKey="plot" stroke="hsl(var(--primary))" fill="url(#pubFill)" strokeWidth={2} isAnimationActive={false}/></AreaChart></ResponsiveContainer></div></CardContent></Card>}
        <Card className="border-border bg-card"><CardHeader><div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between"><div role="group" aria-label={tr(lang,"Группировка","Group by","分组")} className="grid min-w-0 grid-cols-2 gap-2 sm:flex sm:flex-wrap sm:items-center">{GROUPS.map(([value,ruText,en,zh])=><Button key={value} size="sm" variant={group===value?"default":"outline"} className="h-auto min-h-9 min-w-0 whitespace-normal border-border py-2 sm:min-w-[100px]" onClick={()=>{setGroup(value);setOffset(0);setRequest(prev=>prev?{...prev,group_by:value,offset:0}:prev);}}>{tr(lang,ruText,en,zh)}</Button>)}</div><Popover><PopoverTrigger asChild><Button variant="outline" size="sm" className="gap-2 border-border"><Filter className="h-3.5 w-3.5"/>{tr(lang,"Колонки","Columns","列")}</Button></PopoverTrigger><PopoverContent className="w-64 p-3" align="end"><div className="space-y-1">{METRICS.map(([key,ruText,en,zh])=><label key={key} className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted/50"><Checkbox checked={columns.has(key)} disabled={columns.size===1 && columns.has(key)} onCheckedChange={()=>setColumns(previous=>{const next=new Set(previous);if(next.has(key))next.delete(key);else next.add(key);return next;})}/>{tr(lang,ruText,en,zh)}</label>)}</div></PopoverContent></Popover></div></CardHeader><CardContent className="p-0"><div className="max-w-full overflow-x-auto overscroll-x-contain"><table className="w-full min-w-[640px] border-collapse text-sm tabular-nums"><thead><tr className="border-b border-border text-[11px] uppercase tracking-wide text-muted-foreground/70"><th className="px-2 py-1.5 text-left"/><th colSpan={selectedMetrics.length} className="px-2 py-1.5 text-left">{tr(lang,"Результаты","Performance","表现")}</th></tr><tr className="border-b border-border">{[["bucket",groupTitle?tr(lang,groupTitle[1],groupTitle[2],groupTitle[3]):tr(lang,"Группа","Group","分组")],...selectedMetrics.map(([key,ruText,en,zh])=>[key,tr(lang,ruText,en,zh)])].map(([key,title])=><th key={key} className={`whitespace-nowrap px-2 py-2 text-left font-medium text-muted-foreground ${key === "bucket" ? "sticky left-0 z-10 bg-card" : ""}`}><button className="inline-flex items-center gap-1" onClick={()=>{if(sort===key)setDesc(!desc);else{setSort(key as typeof sort);setDesc(true);}}}>{title}<ArrowUpDown className="h-3 w-3"/></button></th>)}</tr></thead><tbody>{!rows.length?<tr><td colSpan={selectedMetrics.length+1} className="py-16 text-center text-muted-foreground">{tr(lang,"Нет данных за выбранный период на этой странице.","No data for this period on this page.","本页所选期间暂无数据。")}</td></tr>:rows.map(row=><tr key={row.bucket} className="border-b border-border/50 transition-colors hover:bg-muted/50"><td className="sticky left-0 z-10 min-w-[180px] break-words bg-card px-2 py-2 font-medium [overflow-wrap:anywhere]">{label(row)}</td>{selectedMetrics.map(([key])=><td key={key} className="whitespace-nowrap px-2 py-2">{display(row,key)}</td>)}</tr>)}</tbody><tfoot className="bg-muted/30 font-semibold"><tr><td className="sticky left-0 z-10 bg-card px-2 py-2">{tr(lang,"Итого за весь период","Total for the full period","整个期间的合计")}</td>{selectedMetrics.map(([key])=><td key={key} className="whitespace-nowrap px-2 py-2">{display(query.data.totals,key)}</td>)}</tr></tfoot></table></div></CardContent></Card>
        <PublisherPagination {...query.data} limit={temporal ? 50 : query.data.limit} offset={offset} count={rows.length} hasMore={temporal ? offset + 50 < sortedRows.length : undefined} onChange={setOffset}/>
      </>}
      </PublisherAsync>
    </div>
  );
}
