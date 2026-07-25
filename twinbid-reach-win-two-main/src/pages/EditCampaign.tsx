import { useState, useEffect, useMemo, useRef } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AutoCropConfirmDialog } from "@/components/dashboard/AutoCropConfirmDialog";
import { getTargetDims } from "@/lib/creativeTarget";
import { ArrowLeft, Save, AlertCircle } from "lucide-react";
import { toast } from "sonner";
import { useCampaigns, type TargetingState, type PricingModel, type TrafficQuality, type TrafficType, type Creative, type Vertical, VERTICALS } from "@/contexts/CampaignContext";
import { TargetingSection } from "@/components/dashboard/TargetingSection";
import { BudgetSection } from "@/components/dashboard/BudgetSection";
import { CreativesEditor, type CreativesEditorHandle } from "@/components/dashboard/CreativesEditor";
import { PostbackSection } from "@/components/dashboard/PostbackSection";
import { useLanguage } from "@/contexts/LanguageContext";
import { api } from "@/api";
import { buildRecommendBidRequest, makeBidRecommendation, type BidRecommendation } from "@/lib/bidRecommendation";
import { getBidLimits, getMaximumBid } from "@/lib/bidLimits";
import {
  creativeRequiresImage,
  extractIframeSrc,
  isCreativeImageUploadError,
  isValidCreativeUrl,
} from "@/lib/creativeApi";

const bannerSizes = ["300x100", "300x250", "300x600", "728x90"];

export default function EditCampaign() {
  const navigate = useNavigate();
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const defaultTab = searchParams.get("tab") || "general";
  const { getCampaign, updateCampaign } = useCampaigns();
  const { t } = useLanguage();
  const campaign = getCampaign(id || "");

  const [name, setName] = useState("");
  const [bannerSize, setBannerSize] = useState("");
  const [brandName, setBrandName] = useState("");
  const [creatives, setCreatives] = useState<Creative[]>([]);
  const [initialCreatives, setInitialCreatives] = useState<Creative[]>([]);
  const [lists, setLists] = useState<Record<string, TargetingState>>({});
  const [totalBudget, setTotalBudget] = useState("");
  const [priceValue, setPriceValue] = useState("");
  const [pricingModel, setPricingModel] = useState<PricingModel>("cpm");
  const [trafficQuality, setTrafficQuality] = useState<TrafficQuality>("common");
  const [startDate, setStartDate] = useState("");
  const [endDate, setEndDate] = useState("");
  const [evenSpend, setEvenSpend] = useState(false);
  const [trafficType, setTrafficType] = useState<TrafficType>("mainstream");
  const [initialTrafficType, setInitialTrafficType] = useState<TrafficType>("mainstream");
  const [verticals, setVerticals] = useState<Vertical[]>([]);
  
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [activeTab, setActiveTab] = useState(defaultTab);
  const [confirmMismatchOpen, setConfirmMismatchOpen] = useState(false);
  const creativesEditorRef = useRef<CreativesEditorHandle>(null);
  const [bidRecommendation, setBidRecommendation] = useState<BidRecommendation | null>(null);

  useEffect(() => {
    if (campaign) {
      setName(campaign.name);
      setBannerSize(campaign.bannerSize || "");
      setBrandName(campaign.brandName || "");
      const crvs = campaign.creatives?.length ? campaign.creatives : [{ id: "migrated", url: "" }];
      setCreatives(crvs);
      setInitialCreatives(JSON.parse(JSON.stringify(crvs)));
      const targeting = campaign.targeting || {};
      // Default schedule to all days/hours if not set
      if (!targeting.schedule || targeting.schedule.mode === "none" || !targeting.schedule.items?.length) {
        const allItems: string[] = [];
        const days = ["monday","tuesday","wednesday","thursday","friday","saturday","sunday"];
        for (const d of days) for (let h = 0; h < 24; h++) allItems.push(`${d}:${h}`);
        targeting.schedule = { mode: "white", items: allItems };
      }
      setLists(targeting);
      setTotalBudget(String(campaign.budget));
      setPriceValue(String(campaign.priceValue));
      setPricingModel(campaign.pricingModel);
      setTrafficQuality(campaign.trafficQuality);
      setStartDate(campaign.startDate);
      setEndDate(campaign.endDate);
      setEvenSpend(campaign.evenSpend ?? false);
      setTrafficType(campaign.trafficType || "mainstream");
      setInitialTrafficType(campaign.trafficType || "mainstream");
      setVerticals(campaign.verticals || []);
      
      setInitialBannerSize(campaign.bannerSize || "");
    }
  }, [campaign]);

  const hasCreativeChanged = useMemo(() => {
    return JSON.stringify(creatives) !== JSON.stringify(initialCreatives);
  }, [creatives, initialCreatives]);

  const [initialBannerSize, setInitialBannerSize] = useState("");
  const isRestart = campaign?.status === "completed";
  const showBannerSize = campaign?.formatKey === "banner";
  const showBrandName = campaign?.formatKey === "native" || campaign?.formatKey === "push";
  const hasBannerSizeChanged = showBannerSize && bannerSize !== initialBannerSize;
  const hasTrafficTypeChanged = trafficType !== initialTrafficType;
  const needsModeration = hasCreativeChanged || hasTrafficTypeChanged || hasBannerSizeChanged;

  const clearError = (...keys: string[]) => setErrors(prev => {
    const next = { ...prev };
    keys.forEach(k => delete next[k]);
    return next;
  });

  // Reactively clear budget/date/price errors
  useEffect(() => { if (totalBudget && parseFloat(totalBudget.replace(",",".")) >= 1) clearError("totalBudget"); }, [totalBudget]);
  useEffect(() => { if (startDate) clearError("startDate"); }, [startDate]);
  useEffect(() => { if (endDate) { const today = new Date(); today.setHours(0,0,0,0); if (new Date(endDate) >= today) clearError("endDate"); } }, [endDate]);
  useEffect(() => { if (name.trim()) clearError("name"); }, [name]);
  useEffect(() => {
    if (priceValue && campaign) {
      const pv = parseFloat(priceValue.replace(",", ".")) || 0;
      const { min } = getBidLimits(campaign.formatKey, trafficQuality, pricingModel);
      const max = getMaximumBid(campaign.formatKey, pricingModel);
      if (pv >= min && pv <= max) clearError("priceValue");
    }
  }, [priceValue, pricingModel, trafficQuality, campaign]);

  const updateList = (key: string, updates: Partial<TargetingState>) => {
    setLists(prev => ({ ...prev, [key]: { ...prev[key], ...updates } }));
  };

  const loadBidRecommendation = async () => {
    if (!campaign) return;
    setBidRecommendation(null);
    try {
      const response = await api.recommendBid(buildRecommendBidRequest(campaign.formatKey, trafficType, lists));
      setBidRecommendation(makeBidRecommendation(Number(response?.average_bid)));
    } catch (error) {
      // Recommendation is optional. Editing must keep working exactly as
      // before when the endpoint is unavailable or returns no usable value.
      console.warn("recommend_bid is unavailable", error);
      setBidRecommendation(null);
    }
  };

  const changeTab = (nextTab: string) => {
    if (activeTab === "targeting" && nextTab === "budget") {
      void loadBidRecommendation();
    }
    setActiveTab(nextTab);
  };

  if (!campaign) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">{t("edit.notFound")}</p>
        <Button variant="outline" onClick={() => navigate("/dashboard/campaigns")} className="mt-4">{t("create.back")}</Button>
      </div>
    );
  }

  const parseNum = (v: string) => parseFloat(v.replace(",", ".")) || 0;

  const handleSave = async (skipMismatchCheck = false, overrideCreatives?: Creative[]) => {
    const crvs = overrideCreatives ?? creatives;
    const e: Record<string, string> = {};
    const tb = parseNum(totalBudget);
    if (!totalBudget || isNaN(tb) || tb < 1) e.totalBudget = t("edit.errorBudgetMin");
    if (campaign.status === "no_budget" && tb <= campaign.budget) {
      e.totalBudget = t("edit.errorBudgetMustIncrease");
    }

    const { min } = getBidLimits(campaign.formatKey, trafficQuality, pricingModel);
    const pv = parseNum(priceValue);
    const max = getMaximumBid(campaign.formatKey, pricingModel);
    if (!priceValue || isNaN(pv) || pv < min) e.priceValue = `${t("budget.belowMin")} ($${min})`;
    else if (pv > max) e.priceValue = t("budget.aboveMaxError").replace("{max}", String(max));

    if (!startDate) e.startDate = t("create.required");
    if (!endDate) e.endDate = t("create.required");
    if (endDate) {
      const today = new Date(); today.setHours(0, 0, 0, 0);
      if (new Date(endDate) < today) e.endDate = t("create.endDateError");
    }
    if (!name.trim()) e.name = t("create.required");

    const sched = lists.schedule;
    if (!sched || !sched.items || sched.items.length === 0) {
      toast.error(t("targeting.scheduleRequired"));
      return;
    }

    crvs.forEach(c => {
      if (!c.name?.trim()) e[`creative_${c.id}_name`] = t("create.required");
      const type = campaign.formatKey === "banner" ? (c.creativeType || "image") : "image";
      if (campaign.formatKey === "banner" && type === "html") {
        if (!c.htmlCode?.trim()) e[`creative_${c.id}_html`] = t("create.required");
      } else if (campaign.formatKey === "banner" && type === "iframe") {
        const mode = c.iframeMode || "url";
        let u = "";
        if (mode === "code") {
          const snippet = (c.iframeCode || "").trim();
          if (!snippet) { e[`creative_${c.id}_iframe`] = t("create.required"); }
          else {
            u = extractIframeSrc(snippet);
            if (!u) e[`creative_${c.id}_iframe`] = t("create.iframeCodeNoSrc");
          }
        } else {
          u = (c.iframeUrl || "").trim();
          if (!u) e[`creative_${c.id}_iframe`] = t("create.required");
        }
        if (u && !e[`creative_${c.id}_iframe`]) {
          if (!isValidCreativeUrl(u)) {
            e[`creative_${c.id}_iframe`] = t("create.iframeUrlInvalid");
          }
        }
      } else {
        if (!c.url.trim()) e[`creative_${c.id}_url`] = t("create.required");
        if (
          creativeRequiresImage(campaign.formatKey, c)
          && !c.imageUrl
          && !c.pendingFile
        ) {
          e[`creative_${c.id}_image`] = t("create.required");
        }
      }
      if ((campaign.formatKey === "native" || campaign.formatKey === "push") && !c.title?.trim()) e[`creative_${c.id}_title`] = t("create.required");
      if ((campaign.formatKey === "native" || campaign.formatKey === "push") && !c.description?.trim()) e[`creative_${c.id}_description`] = t("create.required");
    });

    if (campaign.formatKey === "banner" && !bannerSize) e.bannerSize = t("create.required");

    // Block save when a banner html/iframe creative has a size mismatch (no auto-crop for those).
    if (campaign.formatKey === "banner") {
      const bad = crvs.find(c => {
        const type = c.creativeType || "image";
        return (type === "html" || type === "iframe") && c.sizeMismatch;
      });
      if (bad) {
        const key = bad.creativeType === "iframe" ? "iframe" : "html";
        e[`creative_${bad.id}_${key}`] = t("create.required");
        toast.error(
          (bad.creativeType === "iframe"
            ? t("create.iframeSizeMismatch")
            : t("create.htmlSizeMismatch"))
            .replace("{actualW}", "?").replace("{actualH}", "?")
            .replace("{w}", String(bannerSize.split("x")[0] || "?"))
            .replace("{h}", String(bannerSize.split("x")[1] || "?"))
        );
      }
    }

    setErrors(e);
    if (Object.keys(e).length > 0) {
      if (e.totalBudget) setActiveTab("budget");
      return;
    }

    // Only image creatives fall into the auto-crop confirm path.
    if (!skipMismatchCheck && crvs.some(c => (c.creativeType || "image") === "image" && c.mediaType !== "video" && c.sizeMismatch)) {
      setConfirmMismatchOpen(true);
      return;
    }




    let newStatus = campaign.status;
    if (campaign.status === "draft") {
      newStatus = "moderation";
    } else if (isRestart) {
      newStatus = needsModeration ? "moderation" : "active";
    } else if (campaign.status === "no_budget") {
      newStatus = needsModeration ? "moderation" : "active";
    } else if (needsModeration) {
      newStatus = "moderation";
    }

    try {
      await updateCampaign(campaign.id, {
        name: name.trim(), creatives: crvs, trafficType, verticals,
        targeting: Object.fromEntries(Object.entries(lists).map(([k, v]) => [k, { mode: v.mode, items: v.items }])),
        budget: tb, dailyBudget: null,
        priceValue: pv, pricingModel, trafficQuality, startDate, endDate, evenSpend, status: newStatus,
        bannerSize: showBannerSize ? bannerSize : undefined,
        brandName: showBrandName ? brandName : undefined,
        
      });
    } catch (err: any) {
      toast.error(
        isCreativeImageUploadError(err)
          ? err.message
          : `${t("edit.saveFailed") || "Failed to save campaign"}: ${err?.message || err}`,
      );
      return;
    }

    if (campaign.status === "draft") {
      toast.success(t("edit.savedModeration"));
    } else if (isRestart) {
      toast.success(needsModeration ? t("edit.savedModeration") : t("edit.restartedActive"));
    } else if (campaign.status === "no_budget") {
      toast.success(needsModeration ? t("edit.savedModeration") : t("campaigns.started"));
    } else {
      toast.success(needsModeration ? t("edit.savedModeration") : t("edit.saved"));
    }
    navigate("/dashboard/campaigns");
  };

  return (
    <div className="max-w-3xl min-w-0 space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate("/dashboard/campaigns")}><ArrowLeft className="h-5 w-5" /></Button>
        <div>
          <h2 className="text-2xl font-bold">{t("edit.title")}</h2>
          <p className="text-muted-foreground text-sm">ID: {id}</p>
        </div>
      </div>

      {needsModeration && (
        <div className="flex items-center gap-2 p-3 rounded-lg bg-yellow-500/10 border border-yellow-500/20">
          <AlertCircle className="h-4 w-4 text-yellow-500 shrink-0" />
          <p className="text-sm text-yellow-500">{t("edit.moderationWarning")}</p>
        </div>
      )}

      <Tabs value={activeTab} onValueChange={changeTab}>
        <TabsList className="w-full justify-start overflow-x-auto border border-border bg-card sm:w-auto">
          <TabsTrigger value="general">{t("edit.general")}</TabsTrigger>
          <TabsTrigger value="targeting">{t("edit.targeting")}</TabsTrigger>
          <TabsTrigger value="budget">{t("edit.budget")}</TabsTrigger>
          <TabsTrigger value="conversion">{t("edit.conversion")}</TabsTrigger>
        </TabsList>

        <TabsContent value="general">
          <Card className="bg-card border-border">
            <CardContent className="space-y-5 pt-6">
              <div className="space-y-2">
                <Label>{t("create.trafficType")}</Label>
                <Select value={trafficType} onValueChange={(v) => { setTrafficType(v as TrafficType); if (v === "mainstream") setVerticals(prev => prev.filter(x => x !== "Adult")); }}>
                  <SelectTrigger className="bg-background border-border">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="bg-card border-border">
                    <SelectItem value="mainstream">{t("create.mainstream")}</SelectItem>
                    <SelectItem value="adult">{t("create.adult")}</SelectItem>
                    <SelectItem value="mixed">{t("create.mixed")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("create.vertical")}</Label>
                <div className="flex flex-wrap gap-2">
                  {VERTICALS.filter(v => trafficType === "mainstream" ? v !== "Adult" : true).map(v => {
                    const isChecked = verticals.includes(v);
                    return (
                      <button
                        key={v}
                        type="button"
                        onClick={() => setVerticals(prev => isChecked ? prev.filter(x => x !== v) : [...prev, v])}
                        className={`px-3 py-1 text-xs rounded-full border transition-colors ${
                          isChecked
                            ? "bg-primary/15 border-primary/40 text-primary"
                            : "bg-background border-border text-muted-foreground hover:border-primary/30"
                        }`}
                      >
                        {v}
                      </button>
                    );
                  })}
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t("edit.name")} *</Label>
                <Input value={name} onChange={(e) => { setName(e.target.value); if (e.target.value.trim()) clearError("name"); }}
                  className={`bg-background border-border ${errors.name ? "border-destructive" : ""}`} />
                {errors.name && <p className="text-xs text-destructive">{errors.name}</p>}
              </div>
              <div className="space-y-2">
                <Label>{t("edit.formatLabel")}</Label>
                <Input value={campaign.format} disabled className="bg-muted border-border text-muted-foreground cursor-not-allowed" />
                <p className="text-xs text-muted-foreground">{t("edit.formatLocked")}</p>
              </div>

              {showBannerSize && (
                <div className="space-y-2">
                  <Label>{t("create.bannerSize")} *</Label>
                  <Select value={bannerSize} onValueChange={(v) => { setBannerSize(v); clearError("bannerSize"); }}>
                    <SelectTrigger className={`bg-background border-border ${errors.bannerSize ? "border-destructive" : ""}`}>
                      <SelectValue placeholder={t("create.selectBannerSize")} />
                    </SelectTrigger>
                    <SelectContent className="bg-card border-border">
                      {bannerSizes.map(s => <SelectItem key={s} value={s}>{s}</SelectItem>)}
                    </SelectContent>
                  </Select>
                  {errors.bannerSize && <p className="text-xs text-destructive">{errors.bannerSize}</p>}
                </div>
              )}

              {showBrandName && (
                <div className="space-y-2">
                  <Label>{t("create.brandName")}</Label>
                  <Input value={brandName} onChange={(e) => setBrandName(e.target.value)}
                    placeholder={t("create.brandNamePlaceholder")} className="bg-background border-border" />
                </div>
              )}

              <div className="pt-2">
                <p className="text-sm font-medium text-muted-foreground mb-3">{t("create.creatives")}</p>
                <CreativesEditor ref={creativesEditorRef} formatKey={campaign.formatKey} bannerSize={bannerSize} creatives={creatives} onChange={setCreatives} errors={errors} onClearError={clearError} />
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="targeting">
          <Card className="bg-card border-border">
            <CardHeader><CardTitle className="text-lg">{t("edit.targeting")}</CardTitle></CardHeader>
            <CardContent><TargetingSection lists={lists} onUpdate={updateList} /></CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="budget">
          <Card className="bg-card border-border">
            <CardContent className="pt-6">
              <BudgetSection
                formatKey={campaign.formatKey}
                totalBudget={totalBudget} setTotalBudget={setTotalBudget}
                priceValue={priceValue} setPriceValue={setPriceValue}
                pricingModel={pricingModel} setPricingModel={setPricingModel}
                trafficQuality={trafficQuality} setTrafficQuality={setTrafficQuality}
                startDate={startDate} setStartDate={setStartDate}
                endDate={endDate} setEndDate={setEndDate}
                evenSpend={evenSpend} setEvenSpend={setEvenSpend}
                bidRecommendation={bidRecommendation}
                errors={errors}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="conversion">
          <Card className="bg-card border-border">
            <CardHeader><CardTitle className="text-lg">{t("edit.conversion")}</CardTitle></CardHeader>
            <CardContent><PostbackSection /></CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {(() => {
        const tabs = ["general", "targeting", "budget", "conversion"];
        const idx = tabs.indexOf(activeTab);
        const isLast = activeTab === "conversion";

        const validateGeneral = () => {
          const e: Record<string, string> = {};
          if (!name.trim()) e.name = t("create.required");
          if (showBannerSize && !bannerSize) e.bannerSize = t("create.required");
          creatives.forEach(c => {
            if (!c.name?.trim()) e[`creative_${c.id}_name`] = t("create.required");
            const type = campaign.formatKey === "banner" ? (c.creativeType || "image") : "image";
            if (campaign.formatKey === "banner" && type === "html") {
              if (!c.htmlCode?.trim()) e[`creative_${c.id}_html`] = t("create.required");
            } else if (campaign.formatKey === "banner" && type === "iframe") {
              const mode = c.iframeMode || "url";
              let iframeUrl = "";
              if (mode === "code") {
                if (!c.iframeCode?.trim()) {
                  e[`creative_${c.id}_iframe`] = t("create.required");
                } else {
                  iframeUrl = extractIframeSrc(c.iframeCode);
                  if (!iframeUrl) {
                    e[`creative_${c.id}_iframe`] = t("create.iframeCodeNoSrc");
                  }
                }
              } else if (!c.iframeUrl?.trim()) {
                e[`creative_${c.id}_iframe`] = t("create.required");
              } else {
                iframeUrl = c.iframeUrl;
              }
              if (iframeUrl && !e[`creative_${c.id}_iframe`] && !isValidCreativeUrl(iframeUrl)) {
                e[`creative_${c.id}_iframe`] = t("create.iframeUrlInvalid");
              }
            } else {
              if (!c.url.trim()) e[`creative_${c.id}_url`] = t("create.required");
              if (
                creativeRequiresImage(campaign.formatKey, c)
                && !c.imageUrl
                && !c.pendingFile
              ) {
                e[`creative_${c.id}_image`] = t("create.required");
              }
            }
            if ((campaign.formatKey === "native" || campaign.formatKey === "push") && !c.title?.trim()) e[`creative_${c.id}_title`] = t("create.required");
            if ((campaign.formatKey === "native" || campaign.formatKey === "push") && !c.description?.trim()) e[`creative_${c.id}_description`] = t("create.required");
          });
          setErrors(prev => ({ ...prev, ...e }));
          return Object.keys(e).length === 0;
        };

        const handleNextTab = () => {
          if (activeTab === "general" && !validateGeneral()) return;
          const nextTab = tabs[idx + 1];
          changeTab(nextTab);
        };

        return (
          <div className="flex items-center justify-between gap-3">
            {idx > 0 ? (
              <Button variant="outline" onClick={() => changeTab(tabs[idx - 1])} className="border-border">
                {t("create.back")}
              </Button>
            ) : <div />}
            {isLast ? (
              <Button onClick={() => handleSave()} className="bg-primary hover:bg-primary/90 text-primary-foreground">
                <Save className="h-4 w-4 mr-2" /> {t("edit.save")}
              </Button>
            ) : (
              <Button onClick={handleNextTab} className="bg-primary hover:bg-primary/90 text-primary-foreground">
                {t("create.next")}
              </Button>
            )}
          </div>
        );
      })()}

      <AutoCropConfirmDialog
        open={confirmMismatchOpen}
        creatives={creatives}
        target={getTargetDims(campaign.formatKey, bannerSize)}
        onCancel={() => {
          setConfirmMismatchOpen(false);
          setActiveTab("general");
          setTimeout(() => { void creativesEditorRef.current?.openCropperFor(); }, 50);
        }}
        onConfirm={async (next) => {
          setCreatives(next);
          setConfirmMismatchOpen(false);
          await handleSave(true, next);
        }}
      />

    </div>
  );
}
