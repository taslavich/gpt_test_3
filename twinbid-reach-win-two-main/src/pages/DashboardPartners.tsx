import { useEffect, useState } from "react";
import { ArrowDown, Check, CircleDollarSign, Copy, Link2, Network, TrendingUp, Users, WalletCards } from "lucide-react";
import { toast } from "sonner";
import { api, type PartnerStatsResponse } from "@/api";
import { useProfile } from "@/contexts/ProfileContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { createPartnerLinkFromCode } from "@/lib/partners";

const EMPTY_STATS: Omit<PartnerStatsResponse, "partner"> = {
  advertisers: 0,
  turnover: 0,
  withdrawn: 0,
};

const formatMoney = (value: number) => `$${value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

export default function DashboardPartners() {
  const { profile } = useProfile();
  const { t } = useLanguage();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [partnerStats, setPartnerStats] = useState<PartnerStatsResponse | null>(null);
  const [statsLoading, setStatsLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setStatsLoading(true);
    api.getPartnerStats()
      .then((stats) => { if (active) setPartnerStats(stats); })
      .catch(() => { if (active) setPartnerStats(null); })
      .finally(() => { if (active) setStatsLoading(false); });
    return () => { active = false; };
  }, []);

  const stats = partnerStats ?? { partner: "", ...EMPTY_STATS };
  const income = stats.turnover * 0.1;
  const partnerLink = profile?.partnerId
    ? createPartnerLinkFromCode(profile.partnerId)
    : "";
  const metrics = [
    { key: "advertisers", label: t("partners.stats.total"), value: String(stats.advertisers), icon: Users },
    { key: "turnover", label: t("partners.stats.turnover"), value: formatMoney(stats.turnover), icon: TrendingUp },
    { key: "income", label: t("partners.stats.income"), value: formatMoney(income), icon: CircleDollarSign, featured: true },
    { key: "withdrawn", label: t("partners.stats.withdrawn"), value: formatMoney(stats.withdrawn), icon: WalletCards },
  ];

  const copyLink = async () => {
    if (!partnerLink) return;
    try {
      await navigator.clipboard.writeText(partnerLink);
      toast.success(t("partners.link.copied"));
    } catch {
      toast.error(t("partners.link.copyFailed"));
    }
  };

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <header className="max-w-4xl">
        <h1 className="text-3xl font-bold tracking-tight text-foreground sm:text-4xl">TwinBid Partners</h1>
        <p className="mt-3 max-w-3xl text-sm leading-relaxed text-muted-foreground sm:text-base">{t("partners.hero.description")}</p>
      </header>

      <Card className="overflow-hidden border-border bg-card">
        <CardContent className="grid gap-6 p-5 sm:p-6 lg:grid-cols-[minmax(220px,.55fr)_minmax(0,1.45fr)_auto] lg:items-center">
          <div className="border-b border-border pb-5 lg:border-b-0 lg:border-r lg:pb-0 lg:pr-7">
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">{t("partners.public.modelLabel")}</p>
            <strong className="mt-3 block text-5xl font-bold tracking-[-0.06em] text-primary sm:text-6xl">50 / 50</strong>
          </div>
          <p className="max-w-2xl text-sm font-medium leading-relaxed text-foreground sm:text-base">{t("partners.hero.recurring")}</p>
          <Button type="button" variant="outline" className="w-full lg:w-auto" onClick={() => setDetailsOpen(true)}>{t("partners.hero.details")}</Button>
        </CardContent>
      </Card>

      <section className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4" aria-label={t("partners.brand")}>
        {metrics.map(({ key, label, value, icon: Icon, featured }) => (
          <Card key={key} className={featured ? "border-primary/40 bg-primary/[0.055]" : "border-border bg-card"}>
            <CardContent className="p-5 sm:p-6">
              <div className="flex items-start justify-between gap-4">
                <p className="max-w-[14rem] text-sm leading-snug text-muted-foreground">{label}</p>
                <span className="rounded-lg bg-primary/10 p-2 text-primary"><Icon className="h-4 w-4" aria-hidden="true" /></span>
              </div>
              {statsLoading ? <div className="mt-5 h-9 w-28 animate-pulse rounded-md bg-muted" /> : <p className={`mt-5 font-bold tracking-tight ${featured ? "text-3xl text-primary sm:text-4xl" : "text-3xl text-foreground"}`}>{value}</p>}
            </CardContent>
          </Card>
        ))}
      </section>

      <Card className="border-border bg-card">
        <CardHeader className="pb-4">
          <CardTitle className="flex items-center gap-2.5 text-xl"><Link2 className="h-5 w-5 text-primary" aria-hidden="true" />{t("partners.link.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex min-w-0 flex-col gap-3 sm:flex-row">
            <div className="min-w-0 flex-1 rounded-lg border border-border bg-background px-4 py-3 font-mono text-sm text-foreground"><span className="block truncate">{partnerLink}</span></div>
            <Button type="button" onClick={copyLink} disabled={!partnerLink} className="shrink-0 gap-2"><Copy className="h-4 w-4" aria-hidden="true" />{t("partners.link.copy")}</Button>
          </div>
          <p className="mt-3 max-w-4xl text-sm leading-relaxed text-muted-foreground">{t("partners.link.description")}</p>
        </CardContent>
      </Card>

      <PartnerDetailsDialog open={detailsOpen} onOpenChange={setDetailsOpen} />
    </div>
  );
}

function PartnerDetailsDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLanguage();
  const steps = Array.from({ length: 6 }, (_, index) => t(`partners.details.step${index + 1}`));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-1rem)] overflow-y-auto border-border bg-card sm:max-w-3xl">
        <DialogHeader className="pr-7 text-left">
          <p className="text-sm font-semibold text-primary">TwinBid Partners</p>
          <DialogTitle className="text-2xl sm:text-3xl">{t("partners.details.title")}</DialogTitle>
          <DialogDescription className="text-sm leading-relaxed sm:text-base">{t("partners.details.subtitle")}</DialogDescription>
        </DialogHeader>
        <Separator />

        <section>
          <h3 className="text-lg font-semibold">{t("partners.details.howTitle")}</h3>
          <ol className="mt-4 grid gap-x-6 border-y border-border sm:grid-cols-2">
            {steps.map((step, index) => (
              <li key={step} className="grid grid-cols-[32px_1fr] gap-3 border-b border-border py-3.5 text-sm leading-relaxed last:border-b-0 sm:[&:nth-last-child(-n+2)]:border-b-0">
                <span className="font-mono text-xs font-semibold text-primary">0{index + 1}</span><span>{step}</span>
              </li>
            ))}
          </ol>
        </section>

        <section className="rounded-xl border border-primary/30 bg-primary/[0.055] p-4 sm:p-5">
          <p className="text-sm font-semibold text-primary">{t("partners.example.title")}</p>
          <div className="mt-5 grid items-center gap-3 text-center sm:grid-cols-[1fr_auto_1fr_auto_1fr]">
            <DialogCalculation label={t("partners.example.turnover")} value="$30,000" />
            <ArrowDown className="mx-auto h-4 w-4 text-muted-foreground sm:-rotate-90" aria-hidden="true" />
            <DialogCalculation label={t("partners.example.profit")} value="$6,000" />
            <ArrowDown className="mx-auto h-4 w-4 text-muted-foreground sm:-rotate-90" aria-hidden="true" />
            <DialogCalculation label={t("partners.example.share")} value="$3,000" featured />
          </div>
          <p className="mt-4 text-xs leading-relaxed text-muted-foreground">{t("partners.example.note")}</p>
        </section>

        <section className="grid overflow-hidden rounded-xl border border-border sm:grid-cols-2">
          <ResponsibilityList icon={Network} title={t("partners.details.youTitle")} role={t("partners.details.youRole")} items={[1, 2, 3].map((index) => t(`partners.details.you${index}`))} />
          <ResponsibilityList title="TwinBid" role={t("partners.details.twinbidRole")} items={[1, 2, 3, 4, 5, 6, 7].map((index) => t(`partners.details.twinbid${index}`))} />
        </section>
        <p className="text-lg font-semibold leading-snug">{t("partners.details.finalLine")}</p>
        <p className="border-t border-border pt-4 text-xs leading-relaxed text-muted-foreground">{t("partners.details.terms")}</p>
      </DialogContent>
    </Dialog>
  );
}

function DialogCalculation({ label, value, featured = false }: { label: string; value: string; featured?: boolean }) {
  return <div><p className="text-xs text-muted-foreground">{label}</p><strong className={`mt-1 block text-xl ${featured ? "text-2xl text-primary" : "text-foreground"}`}>{value}</strong></div>;
}

function ResponsibilityList({ icon: Icon, title, role, items }: { icon?: typeof Network; title: string; role: string; items: string[] }) {
  return (
    <div className="p-4 sm:p-5 sm:[&+&]:border-l sm:[&+&]:border-border max-sm:[&+&]:border-t max-sm:[&+&]:border-border">
      <div className="flex items-center gap-3">{Icon ? <Icon className="h-5 w-5 text-primary" aria-hidden="true" /> : <CircleDollarSign className="h-5 w-5 text-primary" aria-hidden="true" />}<div><h3 className="font-semibold">{title}</h3><p className="text-xs text-primary">{role}</p></div></div>
      <ul className="mt-4 space-y-2.5">{items.map((item) => <li key={item} className="flex gap-2 text-sm leading-relaxed text-muted-foreground"><Check className="mt-0.5 h-4 w-4 shrink-0 text-primary" aria-hidden="true" /><span>{item}</span></li>)}</ul>
    </div>
  );
}
