import { useState } from "react";
import { publisherApi, type PublisherRow } from "@/api/publisher";
import { usePublisherResource } from "@/contexts/PublisherContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { useIsMobile } from "@/hooks/use-mobile";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import { formatPublisherMoney, publisherDate, publisherLocale } from "@/lib/publisherData";
import { publisherAllPages, publisherFormatLabel, publisherRowFormat } from "@/lib/publisherInventory";
import { usePublisherCodeDirectory } from "./usePublisherCodeDirectory";
import { PublisherAsync, PublisherEmpty, PublisherError, PublisherPagination } from "./PublisherApiState";

export function PublisherZoneTable({ siteId, codeId, format = "" }: { siteId?: string; codeId?: string; format?: string }) {
  const { lang } = useLanguage();
  const directory = usePublisherCodeDirectory();
  const [offset, setOffset] = useState(0);
  const isMobile = useIsMobile();
  const params = { limit: 20, offset, site_id: siteId, ad_code_id: codeId };
  const query = usePublisherResource(["zones", params], signal => publisherApi.zones(params, signal), !format);
  const filtered = usePublisherResource(["filtered-zones", siteId, codeId], signal => publisherAllPages<PublisherRow>((page, signal) => publisherApi.zones({...page, site_id:siteId, ad_code_id:codeId}, signal),signal), !!format);
  const codes = directory.data || [];
  const matches = (format ? filtered.data || [] : query.data?.rows || []).filter(row => !format || publisherRowFormat(row, codes) === format);
  const rows = format ? matches.slice(offset,offset+20) : matches;
  const ready = format ? !!filtered.data : !!query.data;
  const labels = [tr(lang,"Показы","Impressions","展示"), tr(lang,"Клики","Clicks","点击"), "CTR", tr(lang,"Доход","Revenue","收益")];
  return <div className="space-y-3">
    {directory.error && <PublisherError error={directory.error} retry={directory.refetch} />}
    <p className="text-xs text-muted-foreground">{tr(lang,"Показы, клики и CTR: последние 6 месяцев. Доход: за всё время.","Impressions, clicks and CTR: last 6 months. Revenue: lifetime.","展示、点击和 CTR：最近六个月。收益：累计收益。")}</p>
    <PublisherAsync loading={(format ? filtered.isPending : query.isPending) || directory.isPending} error={format ? filtered.error : query.error} retry={format ? filtered.refetch : query.refetch}>
      {ready && <>
        {!rows.length ? <PublisherEmpty>{tr(lang,"Зон пока нет. Они появятся после первых рекламных запросов.","No zones yet. They appear after the first ad requests.","暂无广告位，首次广告请求后将自动显示。")}</PublisherEmpty> : isMobile ? <div className="space-y-3">{rows.map(row => <div key={row.zone_id} className="min-w-0 space-y-3 rounded-lg border border-border bg-card p-3 text-sm">
          <div className="min-w-0"><p className="break-all font-medium">{row.hostname || "—"}</p><p className="mt-1 text-xs text-muted-foreground">{publisherFormatLabel(publisherRowFormat(row,codes))}{!codeId && ` · ${codes.find(code=>code.ad_code_id===row.ad_code_id)?.name || tr(lang,"Код","Code","代码")}`}</p><p className="mt-1 truncate text-xs text-muted-foreground" title={row.zone_id}>{tr(lang,"Зона","Zone","广告位")}: {row.zone_id}</p></div>
          <div className="grid grid-cols-2 gap-3 border-t border-border pt-3">{(["impressions","clicks","ctr","revenue_usd"] as const).map((key,index)=><div key={key} className="min-w-0"><p className="text-xs text-muted-foreground">{labels[index]}</p><p className="mt-1 break-words font-medium [overflow-wrap:anywhere]">{key==="revenue_usd"?`$${formatPublisherMoney(row.revenue_usd,lang)}`:`${row[key].toLocaleString(publisherLocale(lang),{maximumFractionDigits:key==="ctr"?2:0})}${key==="ctr"?"%":""}`}</p></div>)}</div>
          <p className="text-xs text-muted-foreground">{tr(lang,"Последний запрос","Last request","最近请求")}: {publisherDate(row.last_request_at,lang)}</p>
        </div>)}</div> : <div className="max-w-full overflow-x-auto"><table className="w-full min-w-[760px] text-sm"><thead><tr className="border-b border-border text-muted-foreground">{[tr(lang,"Зоны","Zones","广告位"),tr(lang,"Сайты","Sites","网站"),...(!codeId?[tr(lang,"Код","Code","代码")]:[]),tr(lang,"Формат","Format","格式"),tr(lang,"Последний запрос","Last request","最近请求"),...labels].map(title=><th key={title} className="whitespace-nowrap px-3 py-2 text-left font-medium">{title}</th>)}</tr></thead><tbody>{rows.map(row=><tr key={row.zone_id} className="border-t border-border/50 hover:bg-muted/30">
          <td className="px-3 py-3"><span className="block max-w-[160px] truncate text-xs" title={row.zone_id}>{row.zone_id}</span></td>
          <td className="px-3 py-3"><span className="block max-w-[220px] truncate" title={row.hostname}>{row.hostname || "—"}</span></td>
          {!codeId && <td className="px-3 py-3"><span className="block max-w-[180px] truncate">{codes.find(code=>code.ad_code_id===row.ad_code_id)?.name || "—"}</span></td>}
          <td className="whitespace-nowrap px-3 py-3">{publisherFormatLabel(publisherRowFormat(row,codes))}</td>
          <td className="whitespace-nowrap px-3 py-3 text-xs">{publisherDate(row.last_request_at,lang)}</td>
          {(["impressions","clicks","ctr"] as const).map(key=><td key={key} className="px-3 py-3 tabular-nums">{row[key].toLocaleString(publisherLocale(lang),{maximumFractionDigits:key==="ctr"?2:0})}{key==="ctr"?"%":""}</td>)}
          <td className="whitespace-nowrap px-3 py-3 tabular-nums">${formatPublisherMoney(row.revenue_usd,lang)}</td>
        </tr>)}</tbody></table></div>}
        <PublisherPagination limit={format ? 20 : query.data?.limit || 20} offset={offset} count={rows.length} hasMore={format ? offset+20 < matches.length : rows.length >= (query.data?.limit || 20)} onChange={setOffset}/>
      </>}
    </PublisherAsync>
  </div>;
}
