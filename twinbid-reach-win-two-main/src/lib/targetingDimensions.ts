import type { Lang } from "@/contexts/LanguageContext";
import { COUNTRIES, LANGUAGES } from "@/lib/dimensions";

type TargetingDimension = "country" | "language";

export type TargetingDimensionOption = {
  value: string;
  label: string;
};

const countryNames = Object.fromEntries(
  COUNTRIES.map((item) => [item.code, { ru: item.ru, en: item.en, es: item.es }]),
) as Record<string, Partial<Record<Lang, string>>>;

const languageNames = Object.fromEntries(
  LANGUAGES.map((item) => [item.code, { ru: item.ru, en: item.en, es: item.es }]),
) as Record<string, Partial<Record<Lang, string>>>;

const createFrenchDisplayNames = (type: "region" | "language") => {
  try {
    return typeof Intl.DisplayNames === "function"
      ? new Intl.DisplayNames(["fr"], { type })
      : null;
  } catch {
    return null;
  }
};

const frenchRegions = createFrenchDisplayNames("region");
const frenchLanguages = createFrenchDisplayNames("language");

export function formatTargetingDimensionLabel(value: string, lang: Lang): string {
  const isCountry = countryNames[value] !== undefined;
  const fallback = countryNames[value]?.en ?? languageNames[value]?.en;
  const name = lang === "fr"
    ? (isCountry ? frenchRegions?.of(value) : frenchLanguages?.of(value)) ?? fallback
    : countryNames[value]?.[lang] ?? languageNames[value]?.[lang] ?? fallback;
  return name ? `${name} (${value})` : value;
}

export function getTargetingDimensionOptions(
  dimension: TargetingDimension,
  lang: Lang,
): TargetingDimensionOption[] {
  const entries = dimension === "country" ? COUNTRIES : LANGUAGES;

  return entries
    .map((item) => ({
      value: item.code,
      label: formatTargetingDimensionLabel(item.code, lang),
    }))
    .sort((left, right) => left.label.localeCompare(right.label, lang));
}
