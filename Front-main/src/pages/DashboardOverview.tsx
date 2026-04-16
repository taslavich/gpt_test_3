import { StatsCards } from "@/components/dashboard/StatsCards";
import { BalanceCard } from "@/components/dashboard/BalanceCard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useLanguage } from "@/contexts/LanguageContext";
import { useCampaigns } from "@/contexts/CampaignContext";

export default function DashboardOverview() {
  const { t } = useLanguage();
  const { campaigns } = useCampaigns();

  const statusConfig: Record<string, { label: string; className: string }> = {
    active: { label: t("status.active"), className: "bg-green-500/10 text-green-500 border-green-500/20" },
    paused: { label: t("status.paused"), className: "bg-yellow-500/10 text-yellow-500 border-yellow-500/20" },
    moderation: { label: t("status.moderation"), className: "bg-purple-500/10 text-purple-500 border-purple-500/20" },
    draft: { label: t("status.draft"), className: "bg-muted text-muted-foreground border-border" },
    completed: { label: t("status.completed"), className: "bg-blue-500/10 text-blue-500 border-blue-500/20" },
  };

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        <div className="lg:col-span-3"><StatsCards /></div>
        <div className="lg:col-span-1"><BalanceCard /></div>
      </div>
      <Card className="bg-card border-border">
        <CardHeader><CardTitle>{t("overview.recentCampaigns")}</CardTitle></CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("overview.id")}</th>
                  <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("overview.name")}</th>
                  <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("overview.status")}</th>
                  <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("overview.impressions")}</th>
                  <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("overview.spent")}</th>
                </tr>
              </thead>
              <tbody>
                {campaigns.map((c) => (
                  <tr key={c.id} className="border-b border-border/50">
                    <td className="py-3 px-4 text-muted-foreground font-mono text-sm">{c.id}</td>
                    <td className="py-3 px-4 font-medium">{c.name}</td>
                    <td className="py-3 px-4">
                      <Badge variant="outline" className={cn("font-normal", statusConfig[c.status]?.className)}>
                        {statusConfig[c.status]?.label}
                      </Badge>
                    </td>
                    <td className="py-3 px-4">{c.impressions.toLocaleString()}</td>
                    <td className="py-3 px-4">${c.spent.toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
