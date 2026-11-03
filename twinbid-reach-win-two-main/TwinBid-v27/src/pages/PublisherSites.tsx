import { useState } from "react";
import { ChevronDown, Globe, Search } from "lucide-react";
import { publisherApi, type PublisherRow } from "@/api/publisher";
import { usePublisherResource } from "@/contexts/PublisherContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import {
  formatPublisherMoney,
  publisherDate,
  publisherLocale,
} from "@/lib/publisherData";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { publisherAllPages, publisherRowFormat } from "@/lib/publisherInventory";
import { usePublisherCodeDirectory } from "@/components/publisher/usePublisherCodeDirectory";
import { PublisherStatsFilter } from "@/components/publisher/PublisherStatsFilter";
import { PublisherZoneTable } from "@/components/publisher/PublisherInventoryTable";
import {
  PublisherAsync,
  PublisherEmpty,
  PublisherPagination,
} from "@/components/publisher/PublisherApiState";
export default function PublisherSites() {
  const { lang } = useLanguage();
  const [offset, setOffset] = useState(0),
    [search, setSearch] = useState(""),
    [format, setFormat] = useState(""),
    [code, setCode] = useState(""),
    [expanded, setExpanded] = useState<string | null>(null);
  const directory = usePublisherCodeDirectory();
  const codes = directory.data || [];
  const filtered = !!(search.trim() || format || code);
  const page = { limit: 50, offset };
  const query = usePublisherResource(["sites", page], signal => publisherApi.sites(page, signal), !filtered);
  const allSites = usePublisherResource(["all-sites"], signal => publisherAllPages<PublisherRow>(publisherApi.sites, signal), filtered);
  const matchingZones = usePublisherResource(["site-filter-zones", code], signal =>
    publisherAllPages<PublisherRow>((page, signal) => publisherApi.zones({ ...page, ad_code_id: code || undefined }, signal), signal), !!(format || code));
  const matchedSites = new Set((matchingZones.data || [])
    .filter(row => (!code || row.ad_code_id === code) && (!format || publisherRowFormat(row, codes) === format))
    .map(row => row.site_id));
  const matches = filtered ? (allSites.data || []).filter(row =>
    `${row.hostname} ${row.site_id}`.toLowerCase().includes(search.trim().toLowerCase()) &&
    (!(format || code) || matchedSites.has(row.site_id))) : [];
  const rows = filtered ? matches.slice(offset, offset + 50) : query.data?.rows || [];
  const ready = filtered ? !!allSites.data && (!(format || code) || !!matchingZones.data) && !!directory.data : !!query.data;
  const error = (filtered ? allSites.error : query.error) || ((format || code) && matchingZones.error) || directory.error;
  const loading = (filtered ? allSites.isPending : query.isPending) || (!!(format || code) && matchingZones.isPending) || directory.isPending;
  const retry = async () => {
    await directory.refetch();
    if (filtered) await allSites.refetch(); else await query.refetch();
    if (format || code) await matchingZones.refetch();
  };
  const reset = () => { setOffset(0); setExpanded(null); };
  return (
    <div className="mx-auto max-w-[1440px] space-y-5">
      <div>
        <h1 className="text-2xl font-bold">
          {tr(lang, "Сайты", "Sites", "网站")}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {tr(
            lang,
            "Обнаруженные домены и автоматически созданные зоны.",
            "Detected domains and automatically created zones.",
            "自动识别的域名和广告位。",
          )}
        </p>
      </div>
      <Card className="border-primary/25 bg-gradient-to-r from-primary/10 via-card to-card p-5">
        <div className="flex gap-3">
          <Globe className="h-5 w-5 shrink-0 text-primary" />
          <p className="text-sm">
            {tr(
              lang,
              "Сайты и зоны появляются после реальных рекламных запросов. Активность показана за последние 6 месяцев, доход — за всю историю.",
              "Sites and zones appear after real ad requests. Activity covers the last 6 months; revenue covers the lifetime.",
              "真实广告请求后会自动显示网站和广告位。活动数据为最近六个月，收益为累计值。",
            )}
          </p>
        </div>
      </Card>
      <Card className="flex flex-col gap-4 p-4 lg:flex-row lg:flex-wrap lg:items-end">
        <div className="relative min-w-0 flex-1">
          <Search className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
          <Input
            className="pl-9"
            value={search}
            onChange={(e) => { setSearch(e.target.value); reset(); }}
            placeholder={tr(
              lang,
              "Домен или ID сайта",
              "Domain or site ID",
              "域名或网站 ID",
            )}
          />
        </div>
        <PublisherStatsFilter id="publisher-site-format" allLabel={tr(lang,"Все форматы","All formats","全部格式")} label={tr(lang,"Формат","Format","格式")} options={[{value:"popunder",label:"Popunder"}]} value={format} onChange={value=>{setFormat(value);reset();}} />
        <PublisherStatsFilter id="publisher-site-code" allLabel={tr(lang,"Все коды","All codes","全部代码")} label={tr(lang,"Рекламный код","Ad code","广告代码")} options={codes.map(item=>({value:item.ad_code_id,label:item.name || tr(lang,"Без названия","Untitled","未命名")}))} value={code} onChange={value=>{setCode(value);reset();}} />
      </Card>
      <PublisherAsync
        loading={loading}
        error={error}
        retry={retry}
      >
        {ready && (
          <>
            {!rows?.length ? (
              <PublisherEmpty>
                {filtered
                  ? tr(
                      lang,
                      "Нет сайтов по выбранным фильтрам.",
                      "No sites match the selected filters.",
                      "没有符合筛选条件的网站。",
                    )
                  : tr(
                      lang,
                      "Сайтов на этой странице нет. Установите код и дождитесь первого запроса.",
                      "No sites on this page. Install a code and wait for the first request.",
                      "本页暂无网站，请安装代码并等待首次请求。",
                    )}
              </PublisherEmpty>
            ) : (
              <div className="space-y-3">
                {rows.map((row) => (
                  <Card className="overflow-hidden" key={row.site_id}>
                    <button
                      className="grid w-full min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-x-3 gap-y-4 p-4 text-left xl:flex xl:items-center"
                      aria-expanded={expanded === row.site_id}
                      onClick={() =>
                        setExpanded(
                          expanded === row.site_id ? null : row.site_id,
                        )
                      }
                    >
                      <Globe className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
                      <span className="min-w-0 flex-1">
                        <span className="block break-all font-medium">
                          {row.hostname || row.site_id}
                        </span>
                        <span className="block text-xs text-muted-foreground">
                          {tr(
                            lang,
                            "Последний запрос",
                            "Last request",
                            "最近请求",
                          )}
                          : {publisherDate(row.last_request_at, lang)}
                        </span>
                      </span>
                      <ChevronDown className={`h-4 w-4 shrink-0 xl:order-last ${expanded === row.site_id ? "rotate-180" : ""}`} />
                      <span className="col-span-3 grid grid-cols-2 gap-3 border-t border-border/70 pt-3 text-sm xl:contents">
                        <span>{tr(lang,"Показы","Impressions","展示")}: {row.impressions.toLocaleString(publisherLocale(lang))}</span>
                        <span className="font-semibold text-primary">{tr(lang,"Доход","Revenue","收益")}: ${formatPublisherMoney(row.revenue_usd, lang)}</span>
                      </span>
                    </button>
                    {expanded === row.site_id && (
                      <div className="space-y-4 border-t border-border bg-muted/20 p-4">
                        <dl className="flex min-w-0 flex-col gap-1 text-sm sm:flex-row sm:gap-3">
                          <dt className="shrink-0 text-muted-foreground">site_id:</dt>
                          <dd className="min-w-0 select-all break-all font-mono text-xs leading-5">{row.site_id}</dd>
                        </dl>
                        <PublisherZoneTable key={`${row.site_id}:${code}:${format}`} siteId={row.site_id} codeId={code || undefined} format={format} />
                      </div>
                    )}
                  </Card>
                ))}
              </div>
            )}
            <PublisherPagination
              limit={filtered ? 50 : query.data?.limit || 50}
              offset={offset}
              count={rows.length}
              hasMore={filtered ? offset + 50 < matches.length : rows.length >= (query.data?.limit || 50)}
              onChange={value => {setOffset(value);setExpanded(null);}}
            />
          </>
        )}
      </PublisherAsync>
    </div>
  );
}
