import { useMemo, useState } from "react";
import { Check, ChevronsUpDown, CopyCheck } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import type { Campaign, TargetingState } from "@/contexts/CampaignContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { targetingConfigs } from "@/components/dashboard/TargetingSection";
import { cn } from "@/lib/utils";
import {
  getImportableTargetingKeys,
  importTargetingGroups,
  type TargetingImportKey,
} from "@/lib/targetingImport";

interface TargetingImportDialogProps {
  campaigns: Campaign[];
  currentCampaignId?: string;
  currentTargeting: Record<string, TargetingState>;
  onImport: (targeting: Record<string, TargetingState>) => void;
}

export function TargetingImportDialog({
  campaigns,
  currentCampaignId,
  currentTargeting,
  onImport,
}: TargetingImportDialogProps) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const [campaignSelectorOpen, setCampaignSelectorOpen] = useState(false);
  const [sourceCampaignId, setSourceCampaignId] = useState("");
  const [selectedKeys, setSelectedKeys] = useState<Set<TargetingImportKey>>(new Set());

  const sourceCampaigns = useMemo(
    () => campaigns.filter(campaign => campaign.id !== currentCampaignId),
    [campaigns, currentCampaignId],
  );
  const sourceCampaign = sourceCampaigns.find(campaign => campaign.id === sourceCampaignId);
  const availableKeys = getImportableTargetingKeys(sourceCampaign?.targeting);

  const selectSource = (campaignId: string) => {
    setSourceCampaignId(campaignId);
    setCampaignSelectorOpen(false);
    const source = sourceCampaigns.find(campaign => campaign.id === campaignId);
    setSelectedKeys(new Set(getImportableTargetingKeys(source?.targeting)));
  };

  const toggleKey = (key: TargetingImportKey) => {
    setSelectedKeys(previous => {
      const next = new Set(previous);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const handleImport = () => {
    if (!sourceCampaign || selectedKeys.size === 0) return;
    const imported = importTargetingGroups(currentTargeting, sourceCampaign.targeting, selectedKeys);
    const valuesCount = Array.from(selectedKeys).reduce(
      (total, key) => total + (sourceCampaign.targeting[key]?.items.length ?? 0),
      0,
    );
    onImport(imported);
    toast.success(
      t("targetingImport.success")
        .replace("{groups}", String(selectedKeys.size))
        .replace("{values}", String(valuesCount)),
    );
    setOpen(false);
  };

  const resetDialog = () => {
    setCampaignSelectorOpen(false);
    setSourceCampaignId("");
    setSelectedKeys(new Set());
  };

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => {
      setOpen(nextOpen);
      if (!nextOpen) resetDialog();
    }}>
      <DialogTrigger asChild>
        <Button type="button" variant="outline" className="w-full gap-2 border-primary/40 text-primary hover:bg-primary/10 hover:text-primary sm:w-auto">
          <CopyCheck className="h-4 w-4" />
          {t("targetingImport.open")}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto bg-card sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle>{t("targetingImport.title")}</DialogTitle>
          <DialogDescription>{t("targetingImport.description")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <p className="text-sm font-medium">{t("targetingImport.source")}</p>
            <Popover open={campaignSelectorOpen} onOpenChange={setCampaignSelectorOpen}>
              <PopoverTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  role="combobox"
                  aria-expanded={campaignSelectorOpen}
                  className="w-full justify-between bg-background px-3 font-normal"
                >
                  <span className={cn("truncate", !sourceCampaign && "text-muted-foreground")}>
                    {sourceCampaign
                      ? `${sourceCampaign.name} · ${sourceCampaign.format}`
                      : t("targetingImport.selectCampaign")}
                  </span>
                  <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                </Button>
              </PopoverTrigger>
              <PopoverContent
                align="start"
                className="w-[var(--radix-popover-trigger-width)] bg-card p-1"
              >
                <div
                  className="max-h-72 overflow-y-auto overscroll-contain"
                  onWheelCapture={event => event.stopPropagation()}
                >
                  {sourceCampaigns.map(campaign => (
                    <button
                      key={campaign.id}
                      type="button"
                      onClick={() => selectSource(campaign.id)}
                      className={cn(
                        "flex w-full items-center rounded-sm px-2 py-2 text-left text-sm outline-none hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:text-accent-foreground",
                        campaign.id === sourceCampaignId && "bg-accent text-accent-foreground",
                      )}
                    >
                      <Check
                        className={cn(
                          "mr-2 h-4 w-4 shrink-0",
                          campaign.id === sourceCampaignId ? "opacity-100" : "opacity-0",
                        )}
                      />
                      <span className="min-w-0 truncate">{campaign.name} · {campaign.format}</span>
                    </button>
                  ))}
                </div>
              </PopoverContent>
            </Popover>
          </div>

          {sourceCampaign && availableKeys.length === 0 && (
            <div className="rounded-lg border border-border bg-muted/30 p-4 text-sm text-muted-foreground">
              {t("targetingImport.empty")}
            </div>
          )}

          {sourceCampaign && availableKeys.length > 0 && (
            <div className="space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-sm font-medium">{t("targetingImport.groups")}</p>
                <div className="flex gap-2">
                  <Button type="button" variant="ghost" size="sm" onClick={() => setSelectedKeys(new Set(availableKeys))}>
                    {t("targetingImport.selectAll")}
                  </Button>
                  <Button type="button" variant="ghost" size="sm" onClick={() => setSelectedKeys(new Set())}>
                    {t("targetingImport.clear")}
                  </Button>
                </div>
              </div>

              <div className="space-y-2">
                {availableKeys.map(key => {
                  const config = targetingConfigs.find(item => item.key === key);
                  const sourceList = sourceCampaign.targeting[key];
                  return (
                    <label key={key} className="flex cursor-pointer items-center gap-3 rounded-lg border border-border p-3 hover:bg-muted/40">
                      <Checkbox checked={selectedKeys.has(key)} onCheckedChange={() => toggleKey(key)} />
                      <span className="min-w-0 flex-1 text-sm font-medium">{t(config?.labelKey || `targeting.${key}`)}</span>
                      {key !== "schedule" && (
                        <Badge variant="outline" className={sourceList.mode === "black" ? "border-red-500/30 text-red-500" : "border-green-500/30 text-green-500"}>
                          {sourceList.mode === "black" ? "Black" : "White"}
                        </Badge>
                      )}
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {t("targetingImport.values").replace("{count}", String(sourceList.items.length))}
                      </span>
                    </label>
                  );
                })}
              </div>

              <p className="rounded-lg border border-yellow-500/20 bg-yellow-500/10 p-3 text-xs text-yellow-600 dark:text-yellow-400">
                {t("targetingImport.replaceWarning")}
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>{t("campaigns.cancel")}</Button>
          <Button type="button" onClick={handleImport} disabled={!sourceCampaign || selectedKeys.size === 0}>
            {t("targetingImport.import")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
