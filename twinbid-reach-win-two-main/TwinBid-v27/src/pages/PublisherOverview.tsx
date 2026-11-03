import { useState } from "react";
import { ArrowRight, Plus, ShieldCheck } from "lucide-react";
import { Link } from "react-router-dom";
import { publisherApi } from "@/api/publisher";
import {
  usePublisherResource,
  type PublisherAdCode,
} from "@/contexts/PublisherContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { useIsMobile } from "@/hooks/use-mobile";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import { formatPublisherMoney, publisherLocale } from "@/lib/publisherData";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CreateZoneDialog } from "@/components/publisher/CreateZoneDialog";
import { ZoneCodeDialog } from "@/components/publisher/ZoneCodeDialog";
import {
  PublisherZoneActivityCards,
} from "@/components/publisher/PublisherDiagnostics";
import {
  PublisherAsync,
  PublisherBalanceCards,
  PublisherEmpty,
  PublisherReportPeriod,
} from "@/components/publisher/PublisherApiState";
export default function PublisherOverview() {
  const isMobile = useIsMobile();
  const { lang } = useLanguage(),
    [create, setCreate] = useState(false),
    [code, setCode] = useState<PublisherAdCode | null>(null);
  const overview = usePublisherResource(["overview"], publisherApi.overview),
    balance = usePublisherResource(["balance"], publisherApi.balance);
  const metrics = [
    ["impressions", "Показы", "Impressions", "展示"],
    ["clicks", "Клики", "Clicks", "点击"],
    ["ctr", "CTR", "CTR", "CTR"],
    ["revenue_usd", "Доход", "Revenue", "收益"],
  ] as const;
  return (
    <div className="mx-auto max-w-[1440px] space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">
            {tr(lang, "Обзор дохода", "Revenue overview", "收入概览")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {tr(
              lang,
              "Доход и эффективность сайтов и зон.",
              "Revenue and performance across your sites and zones.",
              "网站和广告位的收入与表现。",
            )}
          </p>
        </div>
      </div>
      <Card className="border-primary/25 bg-gradient-to-r from-primary/15 via-card to-card p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3"><ShieldCheck className="h-6 w-6 shrink-0 text-primary" /><p className="font-semibold">{tr(lang, "До 0% комиссии", "As low as 0% commission", "佣金低至 0%")}</p></div>
          <Button className="w-full sm:w-auto" onClick={() => setCreate(true)}><Plus className="mr-2 h-4 w-4" />{tr(lang, "Создать рекламный код", "Create ad code", "创建广告代码")}</Button>
        </div>
      </Card>
      <PublisherAsync
        loading={balance.isPending}
        error={balance.error}
        retry={balance.refetch}
      >
        {balance.data && <PublisherBalanceCards balance={balance.data} />}
      </PublisherAsync>
      <PublisherZoneActivityCards />
      <PublisherAsync
        loading={overview.isPending}
        error={overview.error}
        retry={overview.refetch}
      >
        {overview.data && (
          <>
            <PublisherReportPeriod report={overview.data.report} />
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              {metrics.map(([key, ru, en, zh]) => (
                <Card key={key} className="min-w-0 p-4">
                  <p className="text-xs text-muted-foreground">
                    {tr(lang, ru, en, zh)}
                  </p>
                  <p className="mt-2 break-all text-xl font-bold">
                    {key === "revenue_usd"
                      ? "$" +
                        formatPublisherMoney(
                          overview.data.report.totals[key],
                          lang,
                        )
                      : overview.data.report.totals[key].toLocaleString(
                          publisherLocale(lang),
                          { maximumFractionDigits: key === "ctr" ? 2 : 0 },
                        ) + (key === "ctr" ? "%" : "")}
                  </p>
                </Card>
              ))}
            </div>
            <Card className="overflow-hidden">
              <div className="flex items-center justify-between gap-3 border-b border-border p-4">
                <h2 className="font-semibold">{tr(lang, "Сайты", "Sites", "网站")}</h2>
                <Button asChild size="sm" variant="ghost"><Link to="/publisher/sites">{tr(lang, "Все сайты", "All sites", "所有网站")}<ArrowRight className="ml-2 h-4 w-4" /></Link></Button>
              </div>
              {!overview.data.report.rows.length ? (
                <PublisherEmpty>
                  {tr(
                    lang,
                    "Запросов за период пока нет",
                    "No requests for this period yet",
                    "此期间暂无请求",
                  )}
                </PublisherEmpty>
              ) : isMobile ? (
                <div className="divide-y divide-border">
                  {overview.data.report.rows.slice(0, 5).map((row) => (
                    <div key={row.bucket} className="flex min-w-0 items-start justify-between gap-4 p-4 text-sm">
                      <div className="min-w-0"><p className="break-all font-medium">{row.hostname || row.site_id || row.bucket}</p><p className="mt-1 text-xs text-muted-foreground">{row.impressions.toLocaleString(publisherLocale(lang))} {tr(lang, "показов", "impressions", "次展示")}</p></div>
                      <p className="shrink-0 font-semibold text-primary">${formatPublisherMoney(row.revenue_usd, lang)}</p>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="max-w-full overflow-x-auto overscroll-x-contain">
                  <table className="w-full min-w-[560px] text-sm">
                    <thead>
                      <tr>
                        {[
                          tr(lang, "Сайт", "Site", "网站"),
                          tr(lang, "Показы", "Impressions", "展示"),
                          tr(lang, "Клики", "Clicks", "点击"),
                          "CTR",
                          tr(lang, "Доход", "Revenue", "收益"),
                        ].map((label) => (
                          <th className="px-4 py-3 text-left" key={label}>
                            {label}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {overview.data.report.rows.slice(0, 5).map((row) => (
                        <tr className="border-t border-border" key={row.bucket}>
                          <td className="min-w-[180px] break-all px-4 py-3">
                            {row.hostname || row.site_id || row.bucket}
                          </td>
                          <td className="whitespace-nowrap px-4 py-3">
                            {row.impressions.toLocaleString(
                              publisherLocale(lang),
                            )}
                          </td>
                          <td className="whitespace-nowrap px-4 py-3">
                            {row.clicks.toLocaleString(publisherLocale(lang))}
                          </td>
                          <td className="whitespace-nowrap px-4 py-3">
                            {row.ctr.toLocaleString(publisherLocale(lang), {
                              maximumFractionDigits: 2,
                            })}
                            %
                          </td>
                          <td className="whitespace-nowrap px-4 py-3">
                            ${formatPublisherMoney(row.revenue_usd, lang)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </Card>
          </>
        )}
      </PublisherAsync>
      <CreateZoneDialog
        open={create}
        onOpenChange={setCreate}
        onCreated={setCode}
      />
      <ZoneCodeDialog
        code={code}
        onOpenChange={(open) => {
          if (!open) setCode(null);
        }}
      />
    </div>
  );
}
