// Country labels backed by the full ISO list (see src/lib/dimensions.ts).
import { COUNTRIES } from "./dimensions";

export const COUNTRY_NAMES: Record<string, { en: string; ru: string }> =
  Object.fromEntries(COUNTRIES.map(c => [c.code, { en: c.en, ru: c.ru }]));

export function formatCountryLabel(code: string, lang: "ru" | "en"): string {
  const entry = COUNTRY_NAMES[code];
  if (!entry) return code;
  const name = lang === "ru" ? entry.ru : entry.en;
  return `${name} (${code})`;
}
