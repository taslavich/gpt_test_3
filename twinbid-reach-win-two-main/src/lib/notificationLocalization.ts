import type { NotificationType } from "@/api/types";

type Translate = (key: string) => string;
type TemplateValues = Record<string, string | number | null | undefined>;

export interface LocalizableNotification {
  title: string;
  description?: string;
  titleKey?: string;
  descriptionKey?: string;
  translationValues?: TemplateValues;
  apiType?: NotificationType;
  apiPayload?: {
    transaction_id?: string | null;
    campaign_id?: string | null;
    deposit_amount?: number | null;
  };
}

export interface LocalizedNotificationContent {
  title: string;
  description?: string;
}

const CAMPAIGN_STATUS_RULES: Array<{ key: string; match: RegExp }> = [
  { key: "status.no_budget", match: /no[ _-]?budget|нет бюджета|без бюджета|sin presupuesto|sans budget/i },
  { key: "status.moderation", match: /moderation|модерац|moderaci[oó]n|mod[eé]ration/i },
  { key: "status.completed", match: /completed|finished|заверш|completad|finalizad|termin[eé]/i },
  { key: "status.paused", match: /paused|pause|приостанов|на паузе|pausad|suspend/i },
  { key: "status.waiting", match: /waiting|ожида|espera|attente/i },
  { key: "status.draft", match: /draft|чернов|borrador|brouillon/i },
  { key: "status.deleted", match: /deleted|удал[её]н|eliminad|supprim/i },
  { key: "status.active", match: /active|актив|activ[aoe]/i },
];

const PAYMENT_STATUS_RULES: Array<{ key: string; match: RegExp }> = [
  { key: "notification.payment.partial", match: /partial|частич|parcial|partiel/i },
  { key: "notification.payment.approved", match: /approved|credited|success(?:ful)?|зачислен|успешн|acreditad|abonad|aprobado|cr[eé]dit[eé]|r[eé]ussi/i },
  { key: "notification.payment.rejected", match: /rejected|failed|error|отклон|ошиб|rechazad|fallid|erreur|refus[eé]|[eé]chou/i },
  { key: "notification.payment.cancelled", match: /cancelled|canceled|отмен|cancelad|annul[eé]/i },
  { key: "notification.payment.pending", match: /pending|waiting|create_unknown|ожида|провер|espera|comprob|attente|v[eé]rifi/i },
];

function interpolate(template: string, values: TemplateValues = {}): string {
  return Object.entries(values).reduce((result, [key, value]) => {
    if (value == null) return result;
    return result
      .split(`\${${key}}`).join(String(value))
      .split(`{${key}}`).join(String(value));
  }, template);
}

function extractAmount(text: string, fallback?: number | null): string | undefined {
  const match = text.match(/(?:\$|USD\s*)(\d(?:[\d\s]|[.,](?=\d))*)/i);
  if (match) return `$${match[1].trim()}`;
  if (fallback != null && Number.isFinite(Number(fallback))) return `$${Number(fallback).toLocaleString("en-US")}`;
  return undefined;
}

function paymentStatusKey(text: string): string {
  return PAYMENT_STATUS_RULES.find(rule => rule.match.test(text))?.key
    ?? "notification.payment.updated";
}

/**
 * Convert persisted backend notification text into the currently selected UI
 * language. The API stores one free-form text value, so known notification
 * types are rendered from stable frontend keys rather than from that saved
 * language. Unknown backend notifications use a localized neutral fallback.
 */
export function localizeNotificationContent(
  notification: LocalizableNotification,
  t: Translate,
): LocalizedNotificationContent {
  const rawText = `${notification.title}\n${notification.description ?? ""}`.trim();

  if (notification.titleKey) {
    return {
      title: interpolate(t(notification.titleKey), notification.translationValues),
      description: notification.descriptionKey
        ? interpolate(t(notification.descriptionKey), notification.translationValues)
        : notification.description,
    };
  }

  if (notification.apiType === "incomplete_topup") {
    const amount = extractAmount(rawText, notification.apiPayload?.deposit_amount);
    return {
      title: t("balance.notif.notCompleted"),
      description: amount
        ? interpolate(t("balance.notif.noHashAmount"), { amount })
        : t("balance.notif.noHash"),
    };
  }

  if (notification.apiType === "low_balance") {
    const amount = extractAmount(rawText);
    return {
      title: t("balance.notif.lowBalance"),
      description: interpolate(t("notification.lowBalance.description"), {
        amount: amount ?? t("notification.amountUnavailable"),
      }),
    };
  }

  if (notification.apiType === "campaign_status") {
    const statusKey = CAMPAIGN_STATUS_RULES.find(rule => rule.match.test(rawText))?.key;
    return {
      title: t("notification.campaignStatus.title"),
      description: statusKey
        ? interpolate(t("notification.campaignStatus.description"), { status: t(statusKey) })
        : t("notification.campaignStatus.updated"),
    };
  }

  if (notification.apiPayload?.transaction_id || /payment|оплат|плат[её]ж|pago|paiement|passimpay|cryptomus/i.test(rawText)) {
    const amount = extractAmount(rawText, notification.apiPayload?.deposit_amount);
    return {
      title: t(paymentStatusKey(rawText)),
      description: amount
        ? interpolate(t("notification.payment.amount"), { amount })
        : t("notification.payment.details"),
    };
  }

  if (notification.apiType === "other") {
    return {
      title: t("notification.other.title"),
      description: t("notification.other.description"),
    };
  }

  return { title: notification.title, description: notification.description };
}
