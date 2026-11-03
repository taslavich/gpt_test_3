import type { ReactNode } from "react";
import { CircleDollarSign, Clock3, HandCoins, Landmark, Wallet } from "lucide-react";
import { Link } from "react-router-dom";
import { useAuth } from "@/contexts/AuthContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { ApiError } from "@/api/http";
import type {
  PublisherBalance,
  PublisherReport,
  RevenueStatus,
} from "@/api/publisher";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import {
  formatPublisherMoney,
  publisherDate,
  publisherErrorText,
} from "@/lib/publisherData";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

export function PublisherError({
  error,
  retry,
}: {
  error: unknown;
  retry?: () => unknown;
}) {
  const { lang } = useLanguage(),
    { signOut } = useAuth();
  return (
    <div
      role="alert"
      className="space-y-3 rounded-xl border border-destructive/40 bg-destructive/5 p-4"
    >
      <p className="text-sm font-medium">{publisherErrorText(error, lang)}</p>
      {error instanceof ApiError && error.status === 401 ? (
        <Button asChild variant="outline" size="sm">
          <Link to="/publisher" onClick={() => void signOut()}>
            {tr(lang, "Войти снова", "Sign in again", "重新登录")}
          </Link>
        </Button>
      ) : (
        retry && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void retry()}
          >
            {tr(lang, "Обновить данные", "Refresh data", "刷新数据")}
          </Button>
        )
      )}
    </div>
  );
}
export function PublisherAsync({
  loading,
  error,
  retry,
  children,
}: {
  loading: boolean;
  error: unknown;
  retry?: () => unknown;
  children: ReactNode;
}) {
  const { lang } = useLanguage();
  if (error) return <PublisherError error={error} retry={retry} />;
  if (loading)
    return (
      <p
        role="status"
        className="py-8 text-center text-sm text-muted-foreground"
      >
        {tr(lang, "Загрузка…", "Loading…", "加载中…")}
      </p>
    );
  return <>{children}</>;
}
export function PublisherEmpty({ children }: { children: ReactNode }) {
  return (
    <div className="p-8 text-center text-sm text-muted-foreground">
      {children}
    </div>
  );
}
export function PublisherPagination({
  limit,
  offset,
  count,
  hasMore = count >= limit,
  onChange,
}: {
  limit: number;
  offset: number;
  count: number;
  hasMore?: boolean;
  onChange: (offset: number) => void;
}) {
  const { lang } = useLanguage();
  if (offset === 0 && !hasMore) return null;
  return (
    <div className="flex flex-wrap items-center justify-end gap-3 py-4 text-xs text-muted-foreground">
      <span>
        {tr(lang, "Строки", "Rows", "行")}: {count ? offset + 1 : offset}–
        {offset + count}
      </span>
      <Button
        variant="outline"
        size="sm"
        disabled={offset === 0}
        onClick={() => onChange(Math.max(0, offset - limit))}
      >
        {tr(lang, "Назад", "Previous", "上一页")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={!hasMore || offset + limit > 1000000}
        onClick={() => onChange(offset + limit)}
      >
        {tr(lang, "Далее", "Next", "下一页")}
      </Button>
    </div>
  );
}
const revenueLabels: Record<RevenueStatus, [string, string, string]> = {
  agg_stats_pending: [
    "Данные о доходе обновляются.",
    "Revenue data is updating.",
    "收益数据正在更新。",
  ],
  agg_stats_ready: [
    "",
    "",
    "",
  ],
  agg_stats_stale: [
    "Обновление дохода задерживается.",
    "Revenue data is taking longer to update.",
    "收益数据更新延迟。",
  ],
  agg_stats_unavailable: [
    "Не удалось обновить данные о доходе.",
    "Revenue data could not be updated.",
    "无法更新收益数据。",
  ],
};
export function PublisherRevenueStatus({ status }: { status: RevenueStatus }) {
  const { lang } = useLanguage();
  if (status === "agg_stats_ready") return null;
  return (
    <p className="text-xs text-amber-500" role="status">
      {tr(lang, ...revenueLabels[status])}
    </p>
  );
}
export function PublisherBalanceCards({
  balance,
}: {
  balance: PublisherBalance;
}) {
  const { lang } = useLanguage();
  const fields = [
    [
      "cumulative_earned",
      "Заработано за всё время",
      "Cumulative earned",
      "累计收益",
      CircleDollarSign,
    ],
    ["balance", "Баланс", "Balance", "余额", Wallet],
    ["reserved", "Зарезервировано", "Reserved", "已预留", Clock3],
    ["paid_out", "Выплачено", "Paid out", "已支付", Landmark],
    [
      "available_for_payout",
      "Доступно для выплаты",
      "Available for payout",
      "可提现金额",
      HandCoins,
    ],
  ] as const;
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-5">
        {fields.map(([key, ru, en, zh, Icon]) => (
          <Card key={key} className="min-w-0 p-4 last:col-span-2 xl:last:col-span-1">
            <div className="flex items-start justify-between gap-2">
              <p className="min-w-0 break-words text-xs text-muted-foreground [overflow-wrap:anywhere] sm:text-sm">{tr(lang, ru, en, zh)}</p>
              <span className="hidden shrink-0 rounded-lg bg-primary/10 p-2 min-[420px]:flex"><Icon className="h-4 w-4 text-primary" /></span>
            </div>
            <p className="mt-3 break-words text-xl font-bold [overflow-wrap:anywhere]">
              ${formatPublisherMoney(balance[key], lang)}
            </p>
          </Card>
        ))}
      </div>
      <PublisherRevenueStatus status={balance.revenue_status} />
    </div>
  );
}
export function PublisherReportPeriod({
  report,
  timezone = "UTC",
}: {
  report: PublisherReport;
  timezone?: string;
}) {
  const { lang } = useLanguage();
  return (
    <div className="space-y-2 rounded-lg border border-border p-3 text-xs text-muted-foreground">
      <p>
        {tr(lang, "Период", "Period", "期间")}: {publisherDate(report.effective_from, lang, timezone)} — {tr(lang, "до", "until", "至")} {publisherDate(report.effective_to, lang, timezone)}
      </p>
      <PublisherRevenueStatus status={report.revenue_status} />
    </div>
  );
}
