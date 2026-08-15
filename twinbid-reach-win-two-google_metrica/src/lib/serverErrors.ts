// Maps raw backend/server error messages to human-readable, localized text.
// Never surfaces internal server phrases to the user.

type TFn = (key: string) => string;

interface Rule {
  match: RegExp;
  key: string;
}

const RULES: Rule[] = [
  { match: /promocode.*already.*used/i, key: "errors.promoAlreadyUsed" },
  { match: /already.*used.*promo/i, key: "errors.promoAlreadyUsed" },
  { match: /promocode.*not.*found|promo.*code.*not.*found|invalid.*promo/i, key: "errors.promoNotFound" },
  { match: /insufficient.*funds|not enough.*balance/i, key: "errors.insufficientFunds" },
  { match: /unauthorized|401|jwt|not authenticated/i, key: "errors.unauthorized" },
  { match: /forbidden|403/i, key: "errors.forbidden" },
  { match: /not found|404/i, key: "errors.notFound" },
  { match: /network|failed to fetch|timeout|ecconn/i, key: "errors.network" },
  { match: /rate.?limit|too many requests|429/i, key: "errors.rateLimit" },
  { match: /invalid.*hash|transaction.*hash/i, key: "errors.invalidHash" },
];

/**
 * Translate a raw error message into a localized, user-friendly string.
 * Falls back to `errors.generic` when no rule matches — the raw technical
 * message is never shown to the user.
 */
export function translateServerError(raw: string, t: TFn): string {
  if (!raw) return t("errors.generic");
  for (const r of RULES) {
    if (r.match.test(raw)) {
      const translated = t(r.key);
      if (translated && translated !== r.key) return translated;
    }
  }
  const generic = t("errors.generic");
  return generic && generic !== "errors.generic" ? generic : raw;
}
