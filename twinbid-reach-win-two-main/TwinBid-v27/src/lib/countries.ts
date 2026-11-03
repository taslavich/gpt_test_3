// Country labels backed by the full ISO list (see src/lib/dimensions.ts).
import { COUNTRIES } from "./dimensions";
import type { Lang } from "@/contexts/LanguageContext";

export const COUNTRY_NAMES: Record<string, { en: string; ru: string; es: string }> =
  Object.fromEntries(COUNTRIES.map(c => [c.code, { en: c.en, ru: c.ru, es: c.es }]));

export function formatCountryLabel(code: string, lang: Lang): string {
  const entry = COUNTRY_NAMES[code];
  if (!entry) return code;
  let name = lang === "ru" ? entry.ru : lang === "es" ? entry.es : entry.en;
  if (lang === "fr") {
    try {
      name = new Intl.DisplayNames(["fr"], { type: "region" }).of(code) || entry.en;
    } catch {
      name = entry.en;
    }
  }
  return `${name} (${code})`;
}
