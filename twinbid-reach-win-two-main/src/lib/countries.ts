// Country labels backed by the full ISO list (see src/lib/dimensions.ts).
import { COUNTRIES } from "./dimensions";
import type { Lang } from "@/contexts/LanguageContext";

export const COUNTRY_NAMES: Record<string, { en: string; ru: string }> =
  Object.fromEntries(COUNTRIES.map(c => [c.code, { en: c.en, ru: c.ru }]));

export function formatCountryLabel(code: string, lang: Lang): string {
  const entry = COUNTRY_NAMES[code];
  if (!entry) return code;
  // Spanish falls back to English (country codes only).
  const name = lang === "ru" ? entry.ru : entry.en;
  return `${name} (${code})`;
}

