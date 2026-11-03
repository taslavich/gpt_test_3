import { describe, expect, it } from "vitest";
import { localizeNotificationContent } from "@/lib/notificationLocalization";

function translator(values: Record<string, string>) {
  return (key: string) => values[key] ?? key;
}

describe("notification localization", () => {
  it("renders a persisted static-wallet reminder in the current language", () => {
    const result = localizeNotificationContent({
      title: "Оплата не завершена",
      description: "Вы не отправили хэш транзакции на $125",
      apiType: "incomplete_topup",
      apiPayload: { transaction_id: "tx-1", deposit_amount: 100 },
    }, translator({
      "balance.notif.notCompleted": "Payment not completed",
      "balance.notif.noHashAmount": "You did not submit the transaction hash for {amount}",
    }));

    expect(result).toEqual({
      title: "Payment not completed",
      description: "You did not submit the transaction hash for $125",
    });
  });

  it("translates backend campaign statuses instead of displaying saved text", () => {
    const result = localizeNotificationContent({
      title: "Campaign status changed",
      description: "Campaign moved to paused",
      apiType: "campaign_status",
    }, translator({
      "notification.campaignStatus.title": "Le statut de la campagne a changé",
      "notification.campaignStatus.description": "Nouveau statut : {status}",
      "status.paused": "En pause",
    }));

    expect(result).toEqual({
      title: "Le statut de la campagne a changé",
      description: "Nouveau statut : En pause",
    });
  });

  it("translates payment-status notifications and preserves their amount", () => {
    const result = localizeNotificationContent({
      title: "Оплата через PassimPay зачислена",
      description: "Сумма $100",
      apiType: "other",
      apiPayload: { transaction_id: "tx-2", deposit_amount: 100 },
    }, translator({
      "notification.payment.approved": "Pago acreditado",
      "notification.payment.amount": "Importe: {amount}",
    }));

    expect(result).toEqual({
      title: "Pago acreditado",
      description: "Importe: $100",
    });
  });

  it("uses stable keys for local notifications after a language switch", () => {
    const notification = {
      title: "Campaign saved as draft",
      description: "You can continue later",
      titleKey: "create.draftSaved",
      descriptionKey: "create.draftSavedDesc",
    };

    const result = localizeNotificationContent(notification, translator({
      "create.draftSaved": "Campaña guardada como borrador",
      "create.draftSavedDesc": "Puedes continuar editándola más tarde.",
    }));

    expect(result).toEqual({
      title: "Campaña guardada como borrador",
      description: "Puedes continuar editándola más tarde.",
    });
  });

  it("never exposes unknown backend text for generic notifications", () => {
    const result = localizeNotificationContent({
      title: "Internal backend notification",
      description: "provider_status=unexpected_value",
      apiType: "other",
    }, translator({
      "notification.other.title": "Новое уведомление",
      "notification.other.description": "Откройте соответствующий раздел кабинета.",
    }));

    expect(result).toEqual({
      title: "Новое уведомление",
      description: "Откройте соответствующий раздел кабинета.",
    });
  });
});

