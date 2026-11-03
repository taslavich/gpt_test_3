import { useState } from "react";
import { publisherApi } from "@/api/publisher";
import { usePublisherResource } from "@/contexts/PublisherContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import { publisherDate } from "@/lib/publisherData";
import { useIsMobile } from "@/hooks/use-mobile";
import { Card } from "@/components/ui/card";
import {
  PublisherPagination,
  PublisherAsync,
  PublisherEmpty,
} from "./PublisherApiState";
export function PublisherZoneActivityCards() {
  const { lang } = useLanguage();
  const query = usePublisherResource(["zone-diagnostics"], (signal) =>
    publisherApi.zoneDiagnostics({ limit: 1, offset: 0 }, signal),
  );
  return (
    <PublisherAsync
      loading={query.isPending}
      error={query.error}
      retry={query.refetch}
    >
      {query.data && (
        <div className="grid min-w-0 grid-cols-1 gap-3 min-[420px]:grid-cols-2 xl:grid-cols-3">
          {(
            [
              ["active_zones", "Активные зоны", "Active zones", "活跃区域"],
              ["codes_without_zones", "Коды без обнаруженных зон", "Codes without detected zones", "未发现区域的代码"],
              [
                "problem_zones",
                "Зоны с проблемами",
                "Zones with issues",
                "问题区域",
              ],
            ] as const
          ).map(([key, ru, en, zh]) => (
            <Card key={key} className="min-w-0 p-4">
              <p className="text-sm text-muted-foreground">
                {tr(lang, ru, en, zh)}
              </p>
              <p className="mt-2 break-all text-2xl font-bold">
                {query.data[key]}
              </p>
              {(key !== "codes_without_zones" || query.data[key] > 0) && <p className="mt-1 text-xs text-muted-foreground">
                {tr(
                  lang,
                  key === "codes_without_zones" ? "Зоны ещё не обнаружены; проверьте установку и наличие трафика." : "За последний час",
                  key === "codes_without_zones" ? "No zones detected yet; check installation and site traffic." : "Over the last hour",
                  key === "codes_without_zones" ? "尚未记录到请求，请检查安装；网站也可能暂无流量。" : "最近一小时",
                )}
              </p>}
            </Card>
          ))}
        </div>
      )}
    </PublisherAsync>
  );
}
export function PublisherDiagnostics({ codeId }: { codeId: string }) {
  const { lang } = useLanguage();
  const isMobile = useIsMobile();
  const [offset, setOffset] = useState(0);
  const limit = 50;
  const query = usePublisherResource(
    ["diagnostics", codeId, offset],
    (signal) => publisherApi.diagnostics(codeId, signal, { limit, offset }),
  );
  return (
    <PublisherAsync
      loading={query.isPending}
      error={query.error}
      retry={query.refetch}
    >
      {query.data && (
        <div className="min-w-0 max-w-full space-y-3">
          <p className="text-sm text-muted-foreground">{tr(lang, "Данные за последний час, обновляются каждые 5 минут. Найденная реклама не означает показ.", "Data for the last hour, updated every 5 minutes. An ad found does not mean it was shown.", "最近一小时的数据，每 5 分钟更新。找到广告并不代表已经展示。")}</p>
          {!query.data.runtime_ready && <p role="alert" className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm">{tr(lang, "Рекламный сервис временно недоступен. Повторите проверку позже.", "The ad service is temporarily unavailable. Check again later.", "广告服务暂时不可用，请稍后重试。")}</p>}
          {query.data.zones.codes_without_zones > 0 && (
            <p role="status" className="rounded-lg border border-border p-3 text-sm">
              {tr(lang,
                "Зоны пока не обнаружены. Проверьте установку кода: возможно, на сайте ещё нет посещений.",
                "No zones detected yet. Check the code installation; the site may not have visitors yet.",
                "尚未发现广告位。请检查代码安装，网站也可能暂无访问量。")}
            </p>
          )}
          {!query.data.zones.rows.length ? (
            <PublisherEmpty>
              {tr(
                lang,
                "На этой странице нет обнаруженных зон.",
                "There are no detected zones on this page.",
                "此页没有已发现的区域。",
              )}
            </PublisherEmpty>
          ) : isMobile ? (
            <div className="space-y-3">
              {query.data.zones.rows.map((z) => (
                <div key={[z.ad_code_id, z.site_id, z.zone_id].join(":")} className="min-w-0 space-y-3 rounded-lg border border-border p-3 text-sm">
                  <div className="flex min-w-0 items-start justify-between gap-3">
                    <div className="min-w-0"><p className="break-all font-medium">{z.hostname || z.site_id}</p><p className="mt-1 break-all text-xs text-muted-foreground">{tr(lang, "Зона", "Zone", "广告位")}: {z.zone_id}</p></div>
                    <span className={z.status === "active" ? "shrink-0 text-emerald-500" : "shrink-0 text-amber-500"}>{z.status === "active" ? tr(lang, "Активна", "Active", "活跃") : tr(lang, "Требует проверки", "Needs attention", "需要检查")}</span>
                  </div>
                  <div className="grid grid-cols-2 gap-3 border-t border-border pt-3">
                    {([
                      ["requests_200", "Реклама найдена", "Ad found", "已找到广告"],
                      ["requests_204", "Нет рекламы", "No ad", "无广告"],
                      ["requests_4xx", "Ошибки запроса", "Request errors", "请求错误"],
                      ["requests_5xx", "Ошибки сервиса", "Service errors", "服务错误"],
                    ] as const).map(([key, ru, en, zh]) => <div key={key}><p className="text-xs text-muted-foreground">{tr(lang, ru, en, zh)}</p><p className="mt-1 font-medium">{z[key]}</p></div>)}
                  </div>
                  <p className="text-xs text-muted-foreground">{tr(lang, "Последний запрос", "Last request", "最近请求")}: {publisherDate(z.last_request_at, lang)}</p>
                </div>
              ))}
            </div>
          ) : (
            <div className="max-w-full overflow-x-auto">
              <table className="w-full min-w-[1050px] text-sm">
                <thead>
                  <tr>
                    {[
                      tr(lang, "Зона", "Zone", "区域"),
                      tr(lang, "Сайт", "Site", "网站"),
                      tr(lang, "Статус", "Status", "状态"),
                      tr(lang, "Реклама найдена", "Ad found", "已找到广告"),
                      tr(lang, "Нет рекламы", "No ad", "无广告"),
                      tr(lang, "Ошибки запроса", "Request errors", "请求错误"),
                      tr(lang, "Ошибки сервиса", "Service errors", "服务错误"),
                      tr(
                        lang,
                        "Последнее обращение (UTC)",
                        "Last request (UTC)",
                        "最后请求 (UTC)",
                      ),
                    ].map((v) => (
                      <th key={v} className="whitespace-nowrap p-2 text-left">
                        {v}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {query.data.zones.rows.map((z) => (
                    <tr
                      key={[z.ad_code_id, z.site_id, z.zone_id].join(":")}
                      className="border-t border-border"
                    >
                      <td className="max-w-48 break-all p-2 font-mono text-xs">
                        {z.zone_id}
                      </td>
                      <td className="max-w-48 break-all p-2">
                        {z.hostname || z.site_id}
                        <span className="block font-mono text-xs text-muted-foreground">
                          {z.site_id}
                        </span>
                      </td>
                      <td
                        className={
                          "p-2 " +
                          (z.status === "active"
                            ? "text-emerald-500"
                            : "text-amber-500")
                        }
                      >
                        {z.status === "active"
                          ? tr(lang, "Активна", "Active", "活跃")
                          : tr(
                              lang,
                              "Требует проверки",
                              "Needs attention",
                              "需要检查",
                            )}
                      </td>
                      <td className="p-2">{z.requests_200}</td>
                      <td className="p-2">{z.requests_204}</td>
                      <td className="p-2">{z.requests_4xx}</td>
                      <td className="p-2">{z.requests_5xx}</td>
                      <td className="whitespace-nowrap p-2">
                        {publisherDate(z.last_request_at, lang)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <PublisherPagination
            limit={limit}
            offset={offset}
            count={query.data.zones.rows.length}
            onChange={setOffset}
          />
        </div>
      )}
    </PublisherAsync>
  );
}
