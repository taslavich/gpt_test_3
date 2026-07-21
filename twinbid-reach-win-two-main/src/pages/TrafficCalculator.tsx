import { useState } from "react";
import {
  BarChart3,
  Check,
  ChevronDown,
  CircleDollarSign,
  Gauge,
  Loader2,
  Percent,
  Plus,
  RefreshCw,
  Save,
  Sparkles,
  Target,
  X,
} from "lucide-react";
import { api, type CalculatorResponse } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useCampaigns, type Campaign, type PricingModel } from "@/contexts/CampaignContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { getBidLimits, getMaximumBid } from "@/lib/bidLimits";
import { COUNTRIES, LANGUAGES } from "@/lib/dimensions";
import { formatNumberWithDot, formatStatisticInteger } from "@/lib/numberFormat";
import { BROWSER_FILTER_KEYS, DEVICE_FILTER_KEYS, OS_FILTER_KEYS, OTHER_KEY } from "@/lib/statFilters";
import { cn } from "@/lib/utils";
import { toast } from "sonner";

const copy = {
  ru: {
    title: "Калькулятор трафика",
    subtitle: "Узнайте, сколько показов было доступно по выбранным таргетингам за последние полностью закрытые сутки.",
    selectCampaign: "Выберите кампанию",
    selectCampaignDesc: "Таргетинги кампании подставятся автоматически. Без кампании можно проверить любой набор параметров.",
    free: "Свободный расчёт",
    freeDesc: "Настроить таргетинги вручную",
    otherCampaign: "Найти другую кампанию",
    parameters: "Параметры расчёта",
    campaignParameters: "Параметры загружены из выбранной кампании — при необходимости их можно изменить для расчёта.",
    allTraffic: "Пустой выбор означает весь доступный трафик.",
    format: "Формат",
    trafficType: "Тип трафика",
    countries: "Страны",
    languages: "Языки",
    devices: "Устройства",
    browsers: "Браузеры",
    sites: "ID сайтов",
    sitesHint: "Введите один или несколько ID через запятую.",
    sitesPlaceholder: "12345,abdjhx",
    sitesFormatError: "ID сайтов вводятся без пробелов, через запятую",
    getData: "Получить данные",
    placeholderTitle: "Здесь появится доступный объём",
    placeholderDesc: "Выберите таргетинги и получите доступный объём показов за последние полностью закрытые сутки.",
    unavailable: "Данные недоступны",
    unavailableDesc: "Не удалось получить данные. Проверьте подключение ручки POST /api/calculator.",
    potentialImpressions: "Доступные показы",
    targetingHint: "по выбранным таргетингам",
    actualImpressions: "Фактические показы",
    sameDay: "за те же полные сутки",
    statsHint: "по данным статистики",
    noData: "за эту дату данных нет",
    share: "Полученная доля",
    shareHint: "от доступных показов",
    currentBid: "Текущая ставка",
    moreTraffic: "Хотите получать больше трафика?",
    bidDesc: "Увеличьте ставку выбранной кампании. Здесь меняется только её размер — модель оплаты остаётся прежней.",
    newBid: "Новая ставка, USD",
    fixedType: "тип зафиксирован",
    save: "Сохранить",
    bidPositive: "Укажите ставку больше нуля",
    bidBelowMin: "Ставка не может быть ниже {value}",
    bidAboveMax: "Ставка не может быть выше {value}",
    bidSaved: "Ставка кампании обновлена",
    bidError: "Не удалось изменить ставку",
    all: "Все",
    include: "Включить",
    exclude: "Исключить",
    excluded: "Исключено",
    clear: "Сбросить выбор",
  },
  en: {
    title: "Traffic calculator",
    subtitle: "See how many impressions were available for the selected targeting during the latest complete day.",
    selectCampaign: "Select a campaign",
    selectCampaignDesc: "The campaign targeting will be filled automatically. Without a campaign, you can check any set of parameters.",
    free: "Free calculation",
    freeDesc: "Set targeting manually",
    otherCampaign: "Find another campaign",
    parameters: "Calculation parameters",
    campaignParameters: "Parameters are loaded from the selected campaign and can be adjusted for the calculation.",
    allTraffic: "An empty selection means all available traffic.",
    format: "Format",
    trafficType: "Traffic type",
    countries: "Countries",
    languages: "Languages",
    devices: "Devices",
    browsers: "Browsers",
    sites: "Site IDs",
    sitesHint: "Enter one or more IDs separated by commas.",
    sitesPlaceholder: "12345,abdjhx",
    sitesFormatError: "Enter site IDs without spaces, separated by commas",
    getData: "Get data",
    placeholderTitle: "Available volume will appear here",
    placeholderDesc: "Choose targeting to get the available impression volume for the latest complete day.",
    unavailable: "Data unavailable",
    unavailableDesc: "Could not load the data. Check the POST /api/calculator integration.",
    potentialImpressions: "Available impressions",
    targetingHint: "for selected targeting",
    actualImpressions: "Actual impressions",
    sameDay: "for the same complete day",
    statsHint: "from campaign statistics",
    noData: "no data for this date",
    share: "Share received",
    shareHint: "of available impressions",
    currentBid: "Current bid",
    moreTraffic: "Want to receive more traffic?",
    bidDesc: "Increase the selected campaign bid. Only its amount can be changed here; the pricing model stays the same.",
    newBid: "New bid, USD",
    fixedType: "type is fixed",
    save: "Save",
    bidPositive: "Enter a bid above zero",
    bidBelowMin: "The bid cannot be lower than {value}",
    bidAboveMax: "The bid cannot be higher than {value}",
    bidSaved: "Campaign bid updated",
    bidError: "Could not update bid",
    all: "All",
    include: "Include",
    exclude: "Exclude",
    excluded: "excluded",
    clear: "Clear selection",
  },
  es: {
    title: "Calculadora de tráfico",
    subtitle: "Consulta cuántas impresiones estuvieron disponibles para la segmentación elegida durante el último día completo.",
    selectCampaign: "Selecciona una campaña",
    selectCampaignDesc: "La segmentación se cargará automáticamente. Sin campaña puedes comprobar cualquier conjunto de parámetros.",
    free: "Cálculo libre",
    freeDesc: "Configurar la segmentación manualmente",
    otherCampaign: "Buscar otra campaña",
    parameters: "Parámetros de cálculo",
    campaignParameters: "Los parámetros se cargaron desde la campaña y se pueden ajustar para el cálculo.",
    allTraffic: "Una selección vacía incluye todo el tráfico disponible.",
    format: "Formato",
    trafficType: "Tipo de tráfico",
    countries: "Países",
    languages: "Idiomas",
    devices: "Dispositivos",
    browsers: "Navegadores",
    sites: "ID de sitios",
    sitesHint: "Introduce uno o varios ID separados por comas.",
    sitesPlaceholder: "12345,abdjhx",
    sitesFormatError: "Introduce los ID sin espacios y separados por comas",
    getData: "Obtener datos",
    placeholderTitle: "Aquí aparecerá el volumen disponible",
    placeholderDesc: "Elige la segmentación para obtener el volumen de impresiones disponible del último día completo.",
    unavailable: "Datos no disponibles",
    unavailableDesc: "No se pudieron obtener los datos. Comprueba la integración POST /api/calculator.",
    potentialImpressions: "Impresiones disponibles",
    targetingHint: "según la segmentación elegida",
    actualImpressions: "Impresiones reales",
    sameDay: "durante el mismo día completo",
    statsHint: "según las estadísticas",
    noData: "sin datos para esta fecha",
    share: "Cuota obtenida",
    shareHint: "de las impresiones disponibles",
    currentBid: "Puja actual",
    moreTraffic: "¿Quieres recibir más tráfico?",
    bidDesc: "Aumenta la puja de la campaña. Aquí solo cambia el importe; el modelo de pago permanece igual.",
    newBid: "Nueva puja, USD",
    fixedType: "tipo fijo",
    save: "Guardar",
    bidPositive: "Introduce una puja superior a cero",
    bidBelowMin: "La puja no puede ser inferior a {value}",
    bidAboveMax: "La puja no puede ser superior a {value}",
    bidSaved: "Puja actualizada",
    bidError: "No se pudo actualizar la puja",
    all: "Todos",
    include: "Incluir",
    exclude: "Excluir",
    excluded: "excluidos",
    clear: "Limpiar selección",
  },
};

const formats = [
  { value: "banner", label: "Banner" },
  { value: "native", label: "Native" },
  { value: "push", label: "In-page Push" },
  { value: "popunder", label: "Popunder" },
] as const;

type FilterState = {
  format: string;
  trafficType: "mainstream" | "adult" | "mixed";
  country: string[];
  countryMode: "include" | "exclude";
  language: string[];
  languageMode: "include" | "exclude";
  deviceType: string[];
  deviceTypeMode: "include" | "exclude";
  os: string[];
  osMode: "include" | "exclude";
  browser: string[];
  browserMode: "include" | "exclude";
  sites: string[];
  sitesMode: "include" | "exclude";
};

const defaults: FilterState = {
  format: "banner",
  trafficType: "mainstream",
  country: [],
  countryMode: "include",
  language: [],
  languageMode: "include",
  deviceType: [],
  deviceTypeMode: "include",
  os: [],
  osMode: "include",
  browser: [],
  browserMode: "include",
  sites: [],
  sitesMode: "include",
};

function fromCampaign(campaign: Campaign): FilterState {
  const items = (key: string) => campaign.targeting[key]?.items ?? [];
  const mode = (key: string): "include" | "exclude" => campaign.targeting[key]?.mode === "black" ? "exclude" : "include";
  return {
    format: campaign.formatKey || "banner",
    trafficType: campaign.trafficType,
    country: items("country"),
    countryMode: mode("country"),
    language: items("language"),
    languageMode: mode("language"),
    deviceType: items("deviceType"),
    deviceTypeMode: mode("deviceType"),
    os: items("os"),
    osMode: mode("os"),
    browser: items("browser"),
    browserMode: mode("browser"),
    sites: items("sites"),
    sitesMode: mode("sites"),
  };
}

type CampaignDayStats = { impressions: number; hasData: boolean };

const previousCompleteUtcDate = () => {
  const date = new Date();
  date.setUTCDate(date.getUTCDate() - 1);
  return date.toISOString().slice(0, 10);
};
const money = (value: number, model: PricingModel) => `$${value.toLocaleString("en-US", {
  minimumFractionDigits: model === "cpc" ? 5 : 2,
  maximumFractionDigits: model === "cpc" ? 5 : 2,
}).replace(/,/g, "\u00a0")}`;

export default function TrafficCalculator() {
  const { lang } = useLanguage();
  const text = copy[lang] ?? copy.en;
  const { campaigns, loading: campaignsLoading, updateCampaign } = useCampaigns();
  const [selectedId, setSelectedId] = useState("");
  const [filters, setFilters] = useState<FilterState>(defaults);
  const [result, setResult] = useState<CalculatorResponse | null>(null);
  const [actual, setActual] = useState<CampaignDayStats | null>(null);
  const [bidDraft, setBidDraft] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const selected = campaigns.find((campaign) => campaign.id === selectedId);

  const resetResult = () => {
    setResult(null);
    setActual(null);
    setError("");
  };

  const updateFilters = (patch: Partial<FilterState>) => {
    setFilters((current) => ({ ...current, ...patch }));
    resetResult();
  };

  const selectCampaign = (campaign: Campaign) => {
    setSelectedId(campaign.id);
    setFilters(fromCampaign(campaign));
    setBidDraft(String(campaign.priceValue));
    resetResult();
  };

  const selectFreeCalculation = () => {
    setSelectedId("");
    setFilters(defaults);
    setBidDraft("");
    resetResult();
  };

  const calculate = async () => {
    setLoading(true);
    setError("");
    setActual(null);
    try {
      const calculation = await api.calculator({
        format_type: filters.format as "banner" | "native" | "push" | "popunder",
        traffic_type: filters.trafficType,
        country: filters.country,
        country_mode: filters.countryMode,
        language: filters.language,
        language_mode: filters.languageMode,
        device_type: filters.deviceType,
        device_type_mode: filters.deviceTypeMode,
        os: filters.os,
        os_mode: filters.osMode,
        browser: filters.browser,
        browser_mode: filters.browserMode,
        site_id: filters.sites,
        site_id_mode: filters.sitesMode,
      });
      setResult(calculation);

      if (selected) {
        const statsDate = previousCompleteUtcDate();
        const stats = await api.statsQuery({
          from: statsDate,
          to: statsDate,
          campaign_ids: [selected.id],
          group_by: "campaign",
        });
        setActual({
          impressions: Number(stats.totals.impressions) || 0,
          hasData: Boolean(stats.rows[selected.id]),
        });
      }
    } catch (cause) {
      console.error(cause);
      setResult(null);
      setActual(null);
      setError(text.unavailableDesc);
    } finally {
      setLoading(false);
    }
  };

  const saveBid = async () => {
    if (!selected) return;
    const bid = Number(bidDraft.replace(",", "."));
    if (bidValidationError) {
      toast.error(bidValidationError);
      return;
    }
    setSaving(true);
    try {
      await updateCampaign(selected.id, { priceValue: bid });
      toast.success(text.bidSaved);
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : text.bidError);
    } finally {
      setSaving(false);
    }
  };

  const share = result && actual?.hasData && result.potential_impressions > 0
    ? (actual.impressions / result.potential_impressions) * 100
    : null;

  const parsedBid = Number(bidDraft.replace(",", "."));
  const selectedBidLimits = selected
    ? getBidLimits(selected.formatKey, selected.trafficQuality, selected.pricingModel)
    : null;
  const selectedMaxBid = selected ? getMaximumBid(selected.formatKey, selected.pricingModel) : null;
  const bidValidationError = !Number.isFinite(parsedBid) || parsedBid <= 0
    ? text.bidPositive
    : selectedBidLimits && parsedBid < selectedBidLimits.min
      ? text.bidBelowMin.replace("{value}", money(selectedBidLimits.min, selected!.pricingModel))
      : selectedMaxBid !== null && parsedBid > selectedMaxBid
        ? text.bidAboveMax.replace("{value}", money(selectedMaxBid, selected!.pricingModel))
        : "";

  return (
    <div className="mx-auto max-w-[1440px] space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{text.title}</h1>
        <p className="mt-1 max-w-3xl text-sm text-muted-foreground">{text.subtitle}</p>
      </div>

      <Card className="overflow-hidden">
        <div className="border-b border-border p-5">
          <div className="flex items-start gap-3">
            <span className="rounded-xl bg-primary/10 p-2.5"><Target className="h-5 w-5 text-primary" /></span>
            <div><h2 className="font-semibold">{text.selectCampaign}</h2><p className="mt-0.5 text-sm text-muted-foreground">{text.selectCampaignDesc}</p></div>
          </div>
        </div>
        <div className="p-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <button type="button" onClick={selectFreeCalculation} className={cn("rounded-xl border p-4 text-left transition-colors", !selectedId ? "border-primary bg-primary/10" : "border-border hover:border-primary/40")}>
              <div className="flex items-center justify-between"><span className="rounded-lg bg-muted p-2"><Sparkles className="h-4 w-4" /></span>{!selectedId && <Check className="h-4 w-4 text-primary" />}</div>
              <p className="mt-3 text-sm font-semibold">{text.free}</p><p className="mt-1 text-xs text-muted-foreground">{text.freeDesc}</p>
            </button>
            {campaignsLoading ? (
              <div className="flex min-h-28 items-center justify-center"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
            ) : campaigns.slice(0, 7).map((campaign) => (
              <button key={campaign.id} type="button" onClick={() => selectCampaign(campaign)} className={cn("rounded-xl border p-4 text-left transition-colors", selectedId === campaign.id ? "border-primary bg-primary/10" : "border-border hover:border-primary/40")}>
                <div className="flex items-center justify-between gap-2"><FormatMark format={campaign.formatKey} /><Badge variant={campaign.status === "active" ? "default" : "secondary"} className="max-w-24 truncate text-[10px]">{campaign.status}</Badge></div>
                <p className="mt-3 truncate text-sm font-semibold">{campaign.name}</p><p className="mt-1 text-xs text-muted-foreground">{campaign.formatKey} · {campaign.pricingModel.toUpperCase()} {money(campaign.priceValue, campaign.pricingModel)}</p>
              </button>
            ))}
          </div>
          {campaigns.length > 7 && (
            <div className="mt-3"><Select value={selectedId} onValueChange={(id) => { const campaign = campaigns.find((item) => item.id === id); if (campaign) selectCampaign(campaign); }}><SelectTrigger className="max-w-md"><SelectValue placeholder={text.otherCampaign} /></SelectTrigger><SelectContent>{campaigns.map((campaign) => <SelectItem key={campaign.id} value={campaign.id}>{campaign.name} · {campaign.formatKey}</SelectItem>)}</SelectContent></Select></div>
          )}
        </div>
      </Card>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.35fr)]">
        <Card className="p-5">
          <div className="mb-5"><h2 className="font-semibold">{text.parameters}</h2><p className="mt-1 text-xs text-muted-foreground">{selected ? text.campaignParameters : text.allTraffic}</p></div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={text.format}><Select value={filters.format} onValueChange={(format) => updateFilters({ format })}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{formats.map((format) => <SelectItem key={format.value} value={format.value}>{format.label}</SelectItem>)}</SelectContent></Select></Field>
            <Field label={text.trafficType}><Select value={filters.trafficType} onValueChange={(trafficType: FilterState["trafficType"]) => updateFilters({ trafficType })}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="mainstream">Mainstream</SelectItem><SelectItem value="adult">Adult</SelectItem><SelectItem value="mixed">Mixed</SelectItem></SelectContent></Select></Field>
          </div>
          <div className="mt-5 grid gap-3 sm:grid-cols-2">
            <MultiChoice label={text.countries} text={text} mode={filters.countryMode} values={filters.country} options={COUNTRIES.map((item) => ({ value: item.code, label: lang === "ru" ? item.ru : lang === "es" ? item.es : item.en }))} onModeChange={(countryMode) => updateFilters({ countryMode })} onChange={(country) => updateFilters({ country })} />
            <MultiChoice label={text.languages} text={text} mode={filters.languageMode} values={filters.language} options={LANGUAGES.map((item) => ({ value: item.code, label: lang === "ru" ? item.ru : lang === "es" ? item.es : item.en }))} onModeChange={(languageMode) => updateFilters({ languageMode })} onChange={(language) => updateFilters({ language })} />
            <MultiChoice label={text.devices} text={text} mode={filters.deviceTypeMode} values={filters.deviceType} options={DEVICE_FILTER_KEYS.filter((item) => item !== OTHER_KEY).map(simpleOption)} onModeChange={(deviceTypeMode) => updateFilters({ deviceTypeMode })} onChange={(deviceType) => updateFilters({ deviceType })} />
            <MultiChoice label="OS" text={text} mode={filters.osMode} values={filters.os} options={OS_FILTER_KEYS.filter((item) => item !== OTHER_KEY).map(simpleOption)} onModeChange={(osMode) => updateFilters({ osMode })} onChange={(os) => updateFilters({ os })} />
            <MultiChoice label={text.browsers} text={text} mode={filters.browserMode} values={filters.browser} options={BROWSER_FILTER_KEYS.filter((item) => item !== OTHER_KEY).map(simpleOption)} onModeChange={(browserMode) => updateFilters({ browserMode })} onChange={(browser) => updateFilters({ browser })} />
            <SiteChoice
              label={text.sites}
              text={text}
              mode={filters.sitesMode}
              values={filters.sites}
              onModeChange={(sitesMode) => updateFilters({ sitesMode })}
              onChange={(sites) => updateFilters({ sites })}
            />
          </div>
          <Button className="mt-5 w-full" onClick={calculate} disabled={loading}>{loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}{text.getData}</Button>
        </Card>

        <div className="space-y-6">
          {!result && !error && (
            <Card className="flex min-h-[420px] flex-col items-center justify-center border-dashed p-8 text-center"><span className="rounded-2xl bg-primary/10 p-4"><Gauge className="h-8 w-8 text-primary" /></span><h3 className="mt-4 font-semibold">{text.placeholderTitle}</h3><p className="mt-2 max-w-md text-sm text-muted-foreground">{text.placeholderDesc}</p></Card>
          )}
          {error && <Card className="border-destructive/30 p-6"><p className="font-medium text-destructive">{text.unavailable}</p><p className="mt-2 text-sm text-muted-foreground">{error}</p></Card>}
          {result && (
            <>
              <div className={cn("grid gap-3", selected ? "md:grid-cols-2 xl:grid-cols-5" : "grid-cols-1")}>
                <Metric featured={Boolean(selected)} icon={Target} label={text.potentialImpressions} value={formatStatisticInteger(result.potential_impressions)} hint={text.targetingHint} />
                {selected && (
                  <>
                    <Metric icon={BarChart3} label={text.actualImpressions} value={actual?.hasData ? formatStatisticInteger(actual.impressions) : "—"} hint={actual?.hasData ? text.statsHint : text.noData} />
                    <Metric icon={Percent} label={text.share} value={share === null ? "—" : `${formatNumberWithDot(share, { maximumFractionDigits: 1 })}%`} hint={text.shareHint} />
                    <Metric icon={CircleDollarSign} label={text.currentBid} value={money(selected.priceValue, selected.pricingModel)} hint={selected.pricingModel.toUpperCase()} />
                  </>
                )}
              </div>
              {selected && (
                <Card className="border-primary/20 p-5"><div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between"><div className="max-w-xl"><span className="inline-flex rounded-xl bg-primary/10 p-2.5"><CircleDollarSign className="h-5 w-5 text-primary" /></span><h3 className="mt-4 font-semibold">{text.moreTraffic}</h3><p className="mt-1 text-sm text-muted-foreground">{text.bidDesc}</p></div><div className="w-full lg:max-w-sm"><div className="mb-2 flex items-center justify-between gap-3 text-xs text-muted-foreground"><span>{text.newBid}</span><span>{selected.pricingModel.toUpperCase()} · {text.fixedType}</span></div><div className="flex gap-2"><Input inputMode="decimal" aria-invalid={Boolean(bidValidationError)} className={cn(bidValidationError && "border-destructive")} value={bidDraft} onChange={(event) => setBidDraft(event.target.value)} /><Button disabled={saving || Boolean(bidValidationError) || parsedBid === selected.priceValue} onClick={saveBid}>{saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}{text.save}</Button></div>{bidValidationError && <p className="mt-2 text-xs text-destructive">{bidValidationError}</p>}</div></div></Card>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function simpleOption(value: string) { return { value, label: value }; }
function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div>; }

type TextCopy = typeof copy.en;

function MultiChoice({ label, text, mode, values, options, onModeChange, onChange }: {
  label: string;
  text: TextCopy;
  mode: "include" | "exclude";
  values: string[];
  options: { value: string; label: string }[];
  onModeChange: (mode: "include" | "exclude") => void;
  onChange: (values: string[]) => void;
}) {
  const summary = values.length
    ? (mode === "exclude" ? `${text.excluded} ${values.length}` : values.length <= 2 ? options.filter((item) => values.includes(item.value)).map((item) => item.label).join(", ") : `${values.length}`)
    : text.all;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild><Button variant="outline" className="h-auto min-h-10 justify-between px-3 font-normal"><span className="min-w-0 truncate text-left"><span className="text-muted-foreground">{label}:</span> {summary}</span><ChevronDown className="ml-2 h-4 w-4 shrink-0 text-muted-foreground" /></Button></DropdownMenuTrigger>
      <DropdownMenuContent className="max-h-72 w-64 overflow-y-auto"><DropdownMenuLabel>{label}</DropdownMenuLabel><div className="flex gap-1 px-2 pb-2"><button type="button" onClick={(event) => { event.preventDefault(); onModeChange("include"); }} className={cn("flex-1 rounded-md px-2 py-1.5 text-xs", mode === "include" ? "bg-primary/15 text-primary" : "bg-muted text-muted-foreground")}>{text.include}</button><button type="button" onClick={(event) => { event.preventDefault(); onModeChange("exclude"); }} className={cn("flex-1 rounded-md px-2 py-1.5 text-xs", mode === "exclude" ? "bg-primary/15 text-primary" : "bg-muted text-muted-foreground")}>{text.exclude}</button></div>{values.length > 0 && <><DropdownMenuCheckboxItem checked={false} onCheckedChange={() => onChange([])}>{text.clear}</DropdownMenuCheckboxItem><DropdownMenuSeparator /></>}{options.map((option) => <DropdownMenuCheckboxItem key={option.value} checked={values.includes(option.value)} onCheckedChange={(checked) => onChange(checked ? [...values, option.value] : values.filter((value) => value !== option.value))}>{option.label}</DropdownMenuCheckboxItem>)}</DropdownMenuContent>
    </DropdownMenu>
  );
}

function SiteChoice({ label, text, mode, values, onModeChange, onChange }: {
  label: string;
  text: TextCopy;
  mode: "include" | "exclude";
  values: string[];
  onModeChange: (mode: "include" | "exclude") => void;
  onChange: (values: string[]) => void;
}) {
  const [value, setValue] = useState("");
  const addSites = () => {
    const raw = value.trim();
    if (!raw) return;
    if (/\s/.test(raw)) {
      toast.error(text.sitesFormatError);
      return;
    }
    const additions = raw.split(",").map((item) => item.trim()).filter(Boolean);
    onChange([...values, ...additions.filter((item) => !values.includes(item))]);
    setValue("");
  };

  return (
    <div className="space-y-3 rounded-lg border border-border/60 p-3 sm:col-span-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div><Label>{label}</Label><p className="mt-1 text-xs text-muted-foreground">{text.sitesHint}</p></div>
        <div className="flex gap-1">
          <Button type="button" size="sm" variant="outline" onClick={() => onModeChange("include")} className={cn(mode === "include" && "border-primary bg-primary/15 text-primary")}>{text.include}</Button>
          <Button type="button" size="sm" variant="outline" onClick={() => onModeChange("exclude")} className={cn(mode === "exclude" && "border-primary bg-primary/15 text-primary")}>{text.exclude}</Button>
        </div>
      </div>
      <div className="flex gap-2">
        <Input value={value} onChange={(event) => setValue(event.target.value)} placeholder={text.sitesPlaceholder} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); addSites(); } }} />
        <Button type="button" size="icon" variant="outline" onClick={addSites} className="shrink-0"><Plus className="h-4 w-4" /></Button>
      </div>
      {values.length > 0 && <div className="flex flex-wrap gap-1.5">{values.map((site) => <Badge key={site} variant="outline" className={cn("gap-1", mode === "include" ? "border-green-500/30 text-green-400" : "border-red-500/30 text-red-400")}>{site}<button type="button" aria-label={`Remove ${site}`} onClick={() => onChange(values.filter((item) => item !== site))}><X className="h-3 w-3" /></button></Badge>)}</div>}
    </div>
  );
}

function Metric({ icon: Icon, label, value, hint, featured = false }: { icon: typeof Target; label: string; value: string; hint: string; featured?: boolean }) {
  return <Card className={cn("min-w-0 p-4", featured && "md:col-span-2")}><span className="inline-flex rounded-lg bg-primary/10 p-2"><Icon className="h-4 w-4 text-primary" /></span><p className="mt-4 text-xs text-muted-foreground">{label}</p><p className={cn("mt-1 min-w-0 font-bold tabular-nums tracking-tight", featured ? "whitespace-nowrap text-[clamp(1.75rem,3.4vw,3rem)]" : "text-2xl")}>{value}</p><p className="mt-1 text-[11px] text-muted-foreground">{hint}</p></Card>;
}

function FormatMark({ format }: { format: string }) {
  const shapes: Record<string, string> = { banner: "aspect-[3/1]", native: "aspect-square", push: "aspect-[5/3]", popunder: "aspect-[4/3]" };
  return <span className={cn("relative block h-8 overflow-hidden rounded-md border border-primary/30 bg-primary/10", format === "native" ? "w-8" : "w-12")}><span className={cn("absolute inset-1 rounded-sm border border-primary/60", shapes[format] || shapes.banner)} /></span>;
}
