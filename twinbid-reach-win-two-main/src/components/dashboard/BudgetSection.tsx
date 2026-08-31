import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { HelpCircle, AlertTriangle, Info, CalendarIcon, CheckCircle2, Circle } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { format } from "date-fns";
import type { CampaignTypeModel, PricingModel, TrafficQuality } from "@/contexts/CampaignContext";
import { useLanguage } from "@/contexts/LanguageContext";
import {
  resolveDisplayedBidRecommendation,
  type BidRecommendation,
} from "@/lib/bidRecommendation";
import { convertRecommendationToModel, getBidLimits, getMaximumBid } from "@/lib/bidLimits";
import { useEffect } from "react";

interface PaymentModelOption {
  key: "cpm" | "twinbid-cpm" | "cpc";
  pricingModel: PricingModel;
  typeModel: CampaignTypeModel;
  label: string;
}

function getAvailableModels(formatKey: string): PaymentModelOption[] {
  const cpm: PaymentModelOption = { key: "cpm", pricingModel: "cpm", typeModel: 1, label: "CPM" };
  const twinBidCpm: PaymentModelOption = { key: "twinbid-cpm", pricingModel: "cpm", typeModel: 2, label: "TwinBid CPM" };
  const cpc: PaymentModelOption = { key: "cpc", pricingModel: "cpc", typeModel: 1, label: "CPC" };
  if (formatKey === "popunder") return [cpm, twinBidCpm, cpc];
  if (formatKey === "push") return [cpc];
  return [cpm, twinBidCpm];
}

function parseNumericValue(val: string): number {
  return parseFloat(val.replace(",", ".")) || 0;
}

function formatBid(value: number, model: PricingModel): string {
  return Number(value.toFixed(model === "cpc" ? 5 : 4)).toString();
}

interface BudgetSectionProps {
  formatKey: string;
  totalBudget: string;
  setTotalBudget: (v: string) => void;
  priceValue: string;
  setPriceValue: (v: string) => void;
  pricingModel: PricingModel;
  setPricingModel: (v: PricingModel) => void;
  typeModel: CampaignTypeModel;
  setTypeModel: (v: CampaignTypeModel) => void;
  trafficQuality: TrafficQuality;
  setTrafficQuality: (v: TrafficQuality) => void;
  startDate: string;
  setStartDate: (v: string) => void;
  endDate: string;
  setEndDate: (v: string) => void;
  evenSpend: boolean;
  setEvenSpend: (v: boolean) => void;
  bidRecommendation?: BidRecommendation | null;
  errors?: Record<string, string>;
}

export function BudgetSection({
  formatKey, totalBudget, setTotalBudget,
  priceValue, setPriceValue, pricingModel, setPricingModel,
  typeModel, setTypeModel,
  trafficQuality, setTrafficQuality, startDate, setStartDate, endDate, setEndDate,
  evenSpend, setEvenSpend,
  bidRecommendation = null,
  errors = {},
}: BudgetSectionProps) {
  const { t } = useLanguage();
  const availableModels = getAvailableModels(formatKey);
  const enforcedPricingModel = availableModels.length === 1 ? availableModels[0] : null;
  const selectedModelKey = pricingModel === "cpc" ? "cpc" : typeModel === 2 ? "twinbid-cpm" : "cpm";
  const bidLimits = getBidLimits(formatKey, trafficQuality, pricingModel);
  const limits = {
    min: bidLimits.min,
    recommendationMin: bidLimits.recommendationMin,
    rec: bidLimits.recommended,
  };
  const priceNum = parseNumericValue(priceValue);
  const maxPrice = getMaximumBid(formatKey, pricingModel);
  const displayedRecommendation = bidRecommendation
    ? resolveDisplayedBidRecommendation({
        apiMinimumRecommended: convertRecommendationToModel(
          bidRecommendation.minimumRecommended,
          formatKey,
          pricingModel,
        ),
        apiOptimalRecommended: convertRecommendationToModel(
          bidRecommendation.optimalRecommended,
          formatKey,
          pricingModel,
        ),
        hardcodedMinimum: limits.recommendationMin,
        hardcodedRecommended: limits.rec,
      })
    : null;
  const minimumRecommended = displayedRecommendation?.minimumRecommended ?? null;
  const optimalRecommended = displayedRecommendation?.optimalRecommended ?? null;
  const isBelowMin = priceValue !== "" && priceNum < limits.min;
  const activeRecommended = minimumRecommended ?? limits.rec;
  const isBelowRec = priceValue !== "" && priceNum >= limits.min && priceNum < activeRecommended;
  const isAboveMax = priceValue !== "" && priceNum > maxPrice;

  // End date validation
  const endDateInvalid = endDate ? (() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    return new Date(endDate) < today;
  })() : false;

  const trafficInfo: Record<TrafficQuality, { label: string; desc: string }> = {
    common: { label: "Usual", desc: t("budget.trafficCommon") },
    high: { label: "High Quality", desc: t("budget.trafficHigh") },
    ultra: { label: "Ultra High Quality", desc: t("budget.trafficUltra") },
  };

  useEffect(() => {
    if (enforcedPricingModel && (
      pricingModel !== enforcedPricingModel.pricingModel
      || typeModel !== enforcedPricingModel.typeModel
    )) {
      setPricingModel(enforcedPricingModel.pricingModel);
      setTypeModel(enforcedPricingModel.typeModel);
    }
  }, [enforcedPricingModel, pricingModel, setPricingModel, setTypeModel, typeModel]);

  const startDateObj = startDate ? new Date(startDate + "T00:00:00") : undefined;
  const endDateObj = endDate ? new Date(endDate + "T00:00:00") : undefined;
  const today = new Date();
  today.setHours(0, 0, 0, 0);

  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <Label>{t("budget.totalBudget")}</Label>
        <div className="relative w-full max-w-xs">
          <Input value={totalBudget} onChange={(e) => setTotalBudget(e.target.value)}
            placeholder="1000" className={cn("bg-background border-border pr-8", (errors.totalBudget) && "border-destructive")} />
          <span className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">$</span>
        </div>
        {errors.totalBudget && <p className="text-xs text-destructive">{errors.totalBudget}</p>}
        <p className="text-xs text-muted-foreground">{t("budget.totalBudgetHint")}</p>
      </div>

      <div className="space-y-2">
        <Label>{t("budget.trafficType")}</Label>
        <div className="flex flex-wrap gap-2">
          {(["common", "high", "ultra"] as const).map((q) => (
            <div key={q} className="flex items-center gap-1">
              <Button type="button" variant="outline" size="sm"
                onClick={() => setTrafficQuality(q)}
                className={cn(
                  trafficQuality === q
                    ? "bg-primary text-primary-foreground border-primary"
                    : "border-border"
                )}>
                {trafficInfo[q].label}
              </Button>
              <Tooltip>
                <TooltipTrigger asChild>
                  <HelpCircle className="h-4 w-4 text-muted-foreground cursor-help" />
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs">
                  <p className="text-sm">{trafficInfo[q].desc}</p>
                </TooltipContent>
              </Tooltip>
            </div>
          ))}
        </div>
      </div>

      {availableModels.length > 1 && (
        <div className="space-y-2">
          <Label>{t("budget.pricingModel")}</Label>
          <div className="flex flex-wrap gap-2">
            {availableModels.map((model) => (
              <Button key={model.key} type="button" variant="outline" size="sm"
                onClick={() => {
                  setPricingModel(model.pricingModel);
                  setTypeModel(model.typeModel);
                }}
                className={cn(
                  selectedModelKey === model.key
                    ? "bg-primary text-primary-foreground border-primary"
                    : "border-border"
                )}>
                {model.label}
              </Button>
            ))}
          </div>
          {availableModels.some((model) => model.key === "twinbid-cpm") && (
            <Dialog>
              <DialogTrigger asChild>
                <Button type="button" variant="link" size="sm" className="h-auto gap-1.5 px-0 py-1 text-primary">
                  <Info className="h-4 w-4" />
                  {t("budget.twinBidCpmLearnMore")}
                </Button>
              </DialogTrigger>
              <DialogContent className="max-h-[90dvh] overflow-y-auto border-border bg-card sm:max-w-xl">
                <DialogHeader>
                  <DialogTitle>{t("budget.twinBidCpmDialogTitle")}</DialogTitle>
                  <DialogDescription className="text-left leading-relaxed">
                    {t("budget.twinBidCpmDialogIntro")}
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4">
                  <div className="rounded-xl border border-border bg-background/50 p-4">
                    <p className="font-semibold">{t("budget.twinBidCpmVsTitle")}</p>
                    <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                      {t("budget.smartCpmDescription")}
                    </p>
                  </div>
                  <div className="rounded-xl border border-primary/30 bg-primary/5 p-4">
                    <p className="text-sm leading-relaxed">{t("budget.twinBidCpmExample")}</p>
                    <div className="mt-4 grid gap-2 sm:grid-cols-2">
                      <div className="rounded-lg border border-border bg-background p-3">
                        <p className="text-xs text-muted-foreground">Smart CPM</p>
                        <p className="mt-1 font-semibold">{t("budget.smartCpmResult")}</p>
                      </div>
                      <div className="rounded-lg border border-primary/30 bg-primary/10 p-3">
                        <p className="text-xs text-primary">TwinBid CPM</p>
                        <p className="mt-1 font-semibold text-primary">{t("budget.twinBidCpmResult")}</p>
                      </div>
                    </div>
                  </div>
                  <p className="text-sm leading-relaxed text-muted-foreground">
                    {t("budget.twinBidCpmConclusion")}
                  </p>
                </div>
              </DialogContent>
            </Dialog>
          )}
        </div>
      )}

      <div className="space-y-2">
        <Label>{pricingModel === "cpc"
          ? t("budget.cpcLabel")
          : typeModel === 2
            ? t("budget.twinBidCpmMaxBidLabel")
            : t("budget.cpmLabel")} *</Label>
        <div className="relative w-full max-w-xs">
          <Input value={priceValue} onChange={(e) => setPriceValue(e.target.value)}
            placeholder={String(optimalRecommended ?? limits.rec)}
            className={cn("bg-background border-border pr-8", (isBelowMin || isAboveMax || errors.priceValue) && "border-destructive")} />
          <span className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">$</span>
        </div>
        <div className="space-y-1">
          {minimumRecommended !== null && optimalRecommended !== null ? (
            <div className="grid gap-2 rounded-lg border border-border bg-background/40 p-3 sm:grid-cols-3">
              {[
                { label: t("budget.minimumBid"), value: limits.min, reached: priceNum >= limits.min },
                { label: t("budget.minimumRecommended"), value: minimumRecommended, reached: priceNum >= minimumRecommended },
                { label: t("budget.optimalRecommended"), value: optimalRecommended, reached: priceNum >= optimalRecommended },
              ].map((checkpoint) => (
                <div
                  key={checkpoint.label}
                  className={cn(
                    "flex items-start gap-2 rounded-md px-2 py-1.5 transition-colors",
                    checkpoint.reached ? "bg-primary/10 text-primary" : "text-muted-foreground",
                  )}
                >
                  {checkpoint.reached
                    ? <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
                    : <Circle className="mt-0.5 h-4 w-4 shrink-0" />}
                  <div>
                    <p className="text-[11px] leading-tight">{checkpoint.label}</p>
                    <p className="mt-1 text-sm font-semibold">${formatBid(checkpoint.value, pricingModel)}</p>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">
              {t("budget.min")}: ${limits.min} · {t("budget.recommended")}: ${limits.rec}
            </p>
          )}
          {isBelowMin && (
            <div className="flex items-center gap-1 text-destructive">
              <AlertTriangle className="h-3 w-3" />
              <p className="text-xs">{t("budget.belowMin")} (${limits.min})</p>
            </div>
          )}
          {isAboveMax && (
            <div className="flex items-center gap-1 text-destructive">
              <AlertTriangle className="h-3 w-3" />
              <p className="text-xs">{t("budget.aboveMax").replace("{max}", String(maxPrice))}</p>
            </div>
          )}
          {isBelowRec && (
            <div className="flex items-center gap-1 text-yellow-500">
              <Info className="h-3 w-3" />
              <p className="text-xs">{t("budget.belowRec")}</p>
            </div>
          )}
        </div>
        {errors.priceValue && <p className="text-xs text-destructive">{errors.priceValue}</p>}
      </div>

      <div className="grid max-w-sm grid-cols-1 gap-4 min-[420px]:grid-cols-2">
        <div className="space-y-2">
          <Label>{t("budget.startDate")} *</Label>
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="outline" className={cn(
                "w-full justify-start text-left font-normal gap-2",
                !startDate && "text-muted-foreground",
                errors.startDate && "border-destructive"
              )}>
                <CalendarIcon className="h-4 w-4" />
                {startDateObj ? format(startDateObj, "dd.MM.yyyy") : t("budget.selectDate")}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-auto p-0" align="start">
              <Calendar
                mode="single"
                selected={startDateObj}
                onSelect={(d) => d && setStartDate(format(d, "yyyy-MM-dd"))}
                disabled={(date) => date < today}
                className="p-3 pointer-events-auto"
              />
            </PopoverContent>
          </Popover>
          {errors.startDate && <p className="text-xs text-destructive">{errors.startDate}</p>}
        </div>
        <div className="space-y-2">
          <Label>{t("budget.endDate")} *</Label>
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="outline" className={cn(
                "w-full justify-start text-left font-normal gap-2",
                !endDate && "text-muted-foreground",
                (endDateInvalid || errors.endDate) && "border-destructive"
              )}>
                <CalendarIcon className="h-4 w-4" />
                {endDateObj ? format(endDateObj, "dd.MM.yyyy") : t("budget.selectDate")}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-auto p-0" align="start">
              <Calendar
                mode="single"
                selected={endDateObj}
                onSelect={(d) => d && setEndDate(format(d, "yyyy-MM-dd"))}
                disabled={(date) => date < today}
                className="p-3 pointer-events-auto"
              />
            </PopoverContent>
          </Popover>
          {(endDateInvalid || errors.endDate) && <p className="text-xs text-destructive">{errors.endDate || t("budget.endDateError")}</p>}
        </div>
        {errors.dates && <p className="text-xs text-destructive min-[420px]:col-span-2">{errors.dates}</p>}
      </div>

      <div className="flex items-start gap-3">
        <Switch checked={evenSpend} onCheckedChange={setEvenSpend} />
        <Label className="cursor-pointer leading-5" onClick={() => setEvenSpend(!evenSpend)}>{t("budget.evenSpend")}</Label>
        <Tooltip>
          <TooltipTrigger asChild>
            <HelpCircle className="h-4 w-4 text-muted-foreground cursor-help" />
          </TooltipTrigger>
          <TooltipContent side="top" className="max-w-xs">
            <p className="text-sm">{t("budget.evenSpendTooltip")}</p>
          </TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}

export function parseNumeric(val: string): number {
  return parseFloat(val.replace(",", ".")) || 0;
}
