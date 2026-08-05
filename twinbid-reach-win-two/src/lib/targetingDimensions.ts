import type { Lang } from "@/contexts/LanguageContext";
import { COUNTRIES, LANGUAGES } from "@/lib/dimensions";

type TargetingDimension = "country" | "language";

export type TargetingDimensionOption = {
  value: string;
  label: string;
};

const countryNames = Object.fromEntries(
  COUNTRIES.map((item) => [item.code, { ru: item.ru, en: item.en, es: item.es }]),
) as Record<string, Record<Lang, string>>;

const languageNames = Object.fromEntries(
  LANGUAGES.map((item) => [item.code, { ru: item.ru, en: item.en, es: item.es }]),
) as Record<string, Record<Lang, string>>;

export function formatTargetingDimensionLabel(value: string, lang: Lang): string {
  const name = countryNames[value]?.[lang] ?? languageNames[value]?.[lang];
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
