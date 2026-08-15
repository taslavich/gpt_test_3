import { useState, useEffect, useRef, useCallback } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CircleCheckBig, CircleX, Copy, ExternalLink, Loader2, Send } from "lucide-react";
import { toast } from "sonner";
import { notifyError } from "@/lib/apiStatus";
import { useLanguage } from "@/contexts/LanguageContext";
import { useAuth } from "@/contexts/AuthContext";
import { useNotifications, type Notification } from "@/contexts/NotificationContext";
import { useProfile } from "@/contexts/ProfileContext";
import { usePendingPayment, type PendingPaymentData } from "@/contexts/PendingPaymentContext";
import { PAYMENT_METHODS } from "@/lib/paymentMethods";
import {
  getInvoiceChargeAmount,
  getInvoiceFee,
  getTransactionBonusAmount,
  getTransactionChannel,
  isInvoicePaymentChannel,
  openFreshInvoicePayment,
} from "@/lib/topup";
import { trackBalanceTopupSuccess } from "@/lib/yandexMetrikaTopup";
import type { ApiUserTransaction } from "@/api/types";

import { api, ApiError } from "@/api";

// Track the persistent "payment not completed" notification across the whole app.
let pendingNotifId: string | null = null;

const STATIC_WALLET_REMINDER_TITLES = new Set([
  "Оплата не завершена",
  "Payment not completed",
  "Pago no completado",
]);

const LEGACY_PASSIMPAY_REMINDER_TITLES = new Set([
  "Оплата через PassimPay не завершена",
  "PassimPay payment is not complete",
  "El pago con PassimPay no está completo",
]);

function isStaticWalletReminder(notification: Pick<Notification, "apiType" | "title">): boolean {
  return notification.apiType === "incomplete_topup"
    && STATIC_WALLET_REMINDER_TITLES.has(notification.title.trim());
}

function isLegacyPassimPayReminder(notification: Pick<Notification, "apiType" | "title">): boolean {
  return notification.apiType === "incomplete_topup"
    && LEGACY_PASSIMPAY_REMINDER_TITLES.has(notification.title.trim());
}

function pendingFromTransaction(transaction: ApiUserTransaction, promo?: string): PendingPaymentData {
  const amount = Number(transaction.deposit_amount) || 0;
  const bonusAmount = getTransactionBonusAmount(transaction);
  const bonusPercent = amount > 0 ? bonusAmount / amount * 100 : 0;
  const channel = getTransactionChannel(transaction);
  return {
    amount,
    method: transaction.payment_method
      || (channel === "passimpay_invoice" ? "passimpay" : channel === "cryptomus_invoice" ? "cryptomus" : "usdt_trc20"),
    channel,
    bonus: bonusPercent || undefined,
    bonus_amount: bonusAmount,
    promo,
    promocode_id: transaction.promocode_id ?? null,
    transactionRowId: transaction.id,
    total_balance_increase: Number(transaction.total_balance_increase) || amount + bonusAmount,
    status: transaction.status,
    payment_url: transaction.payment_url,
    provider_status: transaction.provider_status,
    amount_paid: transaction.amount_paid,
    amount_credited: transaction.amount_credited,
    credited_at: transaction.credited_at,
  };
}

export function PendingPaymentDialog() {
  const { t } = useLanguage();
  const { user } = useAuth();
  const { profile, refetch: refetchProfile } = useProfile();
  const { notifications, notificationsLoaded, addNotification, removeNotification, attachHandlers } = useNotifications();
  const {
    pendingPayment, setPendingPayment,
    isDialogOpen, openDialog, closeDialog,
    restorePaymentAfterInvoice, triggerRefresh,
  } = usePendingPayment();

  const [txHash, setTxHash] = useState("");
  const [checkingInvoice, setCheckingInvoice] = useState(false);
  const [pollingError, setPollingError] = useState<string | null>(null);
  const hydratedTxRef = useRef<string | null>(null);
  const handlersAttachedRef = useRef<string | null>(null);
  const draftCheckedRef = useRef<string | null>(null);
  const approvedHandledRef = useRef<string | null>(null);

  useEffect(() => {
    if (isDialogOpen) {
      setTxHash("");
      setPollingError(null);
    }
  }, [isDialogOpen]);

  // If the user has an unfinished static-wallet draft but no
  // "incomplete_topup" notification, create one so the payment can be resumed.
  // Invoice status notifications are created exclusively by the backend.
  // Runs once per user session and ONLY
  // after notifications have been hydrated from the backend (otherwise we'd
  // create a duplicate on every reload before the existing one loads).
  useEffect(() => {
    if (!user) { draftCheckedRef.current = null; return; }
    if (!notificationsLoaded) return;
    if (draftCheckedRef.current === user.id) return;
    const hasIncompleteNotif = notifications.some(isStaticWalletReminder);
    if (hasIncompleteNotif) { draftCheckedRef.current = user.id; return; }
    draftCheckedRef.current = user.id;
    (async () => {
      try {
        const res = await api.listTransactions();
        const items = Array.isArray(res?.items) ? res.items : [];
        const unfinished = items.find(
          x => x.user_id === user.id
            && x.status === "draft"
            && x.payment_channel === "static_wallet",
        );
        if (!unfinished) return;
        // Re-check to avoid race with hydration creating the notif elsewhere.
        if (notifications.some(
          n => isStaticWalletReminder(n) || n.apiPayload?.transaction_id === unfinished.id,
        )) return;
        const depositAmt = Number(unfinished.deposit_amount) || 0;
        const bonusUsd = getTransactionBonusAmount(unfinished);
        const total = depositAmt + bonusUsd;
        await addNotification({
          title: t("balance.notif.notCompleted"),
          description: `${t("balance.notif.noHash")} $${total}`,
          type: "warning",
          persistent: true,
          apiType: "incomplete_topup",
          apiPayload: {
            deposit_amount: depositAmt,
            transaction_id: unfinished.id,
          },
        });
      } catch (e) {
        console.error("[topup] draft auto-notify failed", e);
      }
    })();
  }, [user, notificationsLoaded, notifications, addNotification, t]);

  // Re-bind UI handlers to the persisted "incomplete_topup" notification on reload,
  // and rehydrate pendingPayment from the linked transaction so all bonus info
  // (promocode_id, bonus_amount) is restored without re-asking the user.
  useEffect(() => {
    const obsoletePassimPayReminder = notifications.find(isLegacyPassimPayReminder);
    if (obsoletePassimPayReminder) {
      removeNotification(obsoletePassimPayReminder.id);
      return;
    }

    const persisted = notifications.find(isStaticWalletReminder);
    if (!persisted) {
      pendingNotifId = null;
      hydratedTxRef.current = null;
      handlersAttachedRef.current = null;
      return;
    }

    pendingNotifId = persisted.id;
    const transactionRowId = persisted.apiPayload?.transaction_id ?? null;

    // Hydrate pendingPayment from backend tx — only ONCE per txId to avoid loop.
    if (!pendingPayment && hydratedTxRef.current !== (transactionRowId ?? persisted.id)) {
      hydratedTxRef.current = transactionRowId ?? persisted.id;
      if (transactionRowId) {
        (async () => {
          try {
            const tx = await api.getTransaction(transactionRowId);
            let promoName: string | undefined;
            if (tx.promocode_id) {
              try {
                const { supabase } = await import("@/integrations/supabase/client");
                const { data } = await supabase.from("promo_codes").select("code").eq("id", tx.promocode_id).maybeSingle();
                if (data?.code) promoName = data.code;
              } catch (e) { console.error("promo code lookup failed", e); }
            }
            setPendingPayment(pendingFromTransaction(tx, promoName));
          } catch (e) {
            console.error("[topup] resume: getTransaction failed", e);
            if (persisted.apiPayload?.deposit_amount) {
              setPendingPayment({
                amount: Number(persisted.apiPayload.deposit_amount) || 0,
                method: "usdt_trc20",
                channel: "static_wallet",
                transactionRowId,
              });
            }
          }
        })();
      } else if (persisted.apiPayload?.deposit_amount) {
        setPendingPayment({
          amount: Number(persisted.apiPayload.deposit_amount) || 0,
          method: "usdt_trc20",
        });
      }
    }

    // Attach handlers only once per notification to avoid setNotifications loop.
    if (handlersAttachedRef.current !== persisted.id) {
      handlersAttachedRef.current = persisted.id;
      attachHandlers(persisted.id, {
        action: { label: t("balance.notif.completePayment"), onClick: () => openDialog() },
        onDismiss: async () => {
          const transactionRowIdToDismiss = persisted.apiPayload?.transaction_id ?? null;
          if (transactionRowIdToDismiss) {
            try {
              const transaction = await api.getTransaction(transactionRowIdToDismiss);
              if (getTransactionChannel(transaction) === "static_wallet") {
                await api.cancelTransaction(transactionRowIdToDismiss);
              }
            } catch (e) {
              console.error("dismiss incomplete topup failed", e);
            }
          }
          setPendingPayment(null);
          setTxHash("");
          pendingNotifId = null;
          hydratedTxRef.current = null;
          handlersAttachedRef.current = null;
          triggerRefresh();
        },
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [notifications]);

  const currentMethod = PAYMENT_METHODS.find(m => m.id === pendingPayment?.method);

  const copyAddress = (address: string) => {
    navigator.clipboard.writeText(address);
    toast.success(t("balance.toast.addressCopied"));
  };

  const clearPendingNotif = () => {
    if (pendingNotifId) {
      removeNotification(pendingNotifId);
      pendingNotifId = null;
    }
  };

  const handleInvoiceCredited = useCallback(async (transaction: ApiUserTransaction) => {
    if (
      transaction.status === "approved"
      && transaction.credited_at
      && approvedHandledRef.current !== transaction.id
    ) {
      approvedHandledRef.current = transaction.id;
      trackBalanceTopupSuccess(transaction);
      await refetchProfile();
      triggerRefresh();
      toast.success(t("balance.passimpay.paid"));
    }
  }, [refetchProfile, t, triggerRefresh]);

  const refreshInvoice = useCallback(async (transactionRowId: string, showSpinner = false) => {
    if (showSpinner) setCheckingInvoice(true);
    try {
      const transaction = await api.getTransaction(transactionRowId);
      setPollingError(null);
      setPendingPayment(previous => pendingFromTransaction(transaction, previous?.promo));
      await handleInvoiceCredited(transaction);
      return { transaction, stopPolling: false };
    } catch (error) {
      const stopPolling = error instanceof ApiError && (error.status === 404 || error.status === 503);
      const message = error instanceof ApiError && error.status === 503
        ? t("balance.passimpay.unavailable")
        : error instanceof ApiError && error.status === 404
          ? t("balance.passimpay.notFound")
          : error instanceof Error
            ? error.message
            : t("balance.toast.submitError");
      setPollingError(message);
      if (showSpinner) notifyError(t("balance.toast.submitError"), error);
      else console.error("[topup] invoice polling failed", error);
      return { transaction: null, stopPolling };
    } finally {
      if (showSpinner) setCheckingInvoice(false);
    }
  }, [handleInvoiceCredited, setPendingPayment, t]);

  const handleOpenInvoicePayment = useCallback(async () => {
    const transactionRowId = pendingPayment?.transactionRowId;
    if (!transactionRowId) return;

    setCheckingInvoice(true);
    setPollingError(null);
    try {
      const result = await openFreshInvoicePayment({
        transactionRowId,
        getTransaction: id => api.getTransaction(id),
        openWindow: () => window.open("", "_blank"),
        onFreshTransaction: transaction => {
          setPendingPayment(previous => pendingFromTransaction(transaction, previous?.promo));
          triggerRefresh();
        },
      });
      await handleInvoiceCredited(result.transaction);
    } catch (error) {
      const message = error instanceof ApiError && error.status === 503
        ? t("balance.passimpay.unavailable")
        : error instanceof ApiError && error.status === 404
          ? t("balance.passimpay.notFound")
          : error instanceof Error
            ? error.message
            : t("balance.toast.submitError");
      setPollingError(message);
      notifyError(t("balance.toast.submitError"), error);
    } finally {
      setCheckingInvoice(false);
    }
  }, [handleInvoiceCredited, pendingPayment?.transactionRowId, setPendingPayment, t, triggerRefresh]);

  useEffect(() => {
    const transactionRowId = pendingPayment?.transactionRowId;
    if (!isDialogOpen || !isInvoicePaymentChannel(pendingPayment?.channel) || !transactionRowId) return;

    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const startedAt = Date.now();

    const poll = async () => {
      const result = await refreshInvoice(transactionRowId);
      if (stopped) return;
      if (result.stopPolling) return;
      const transaction = result.transaction;
      if (transaction?.status === "approved" && transaction.credited_at) return;
      if (
        transaction?.status === "rejected"
        || transaction?.status === "cancelled"
        || transaction?.provider_status === "error"
      ) return;
      const delay = Date.now() - startedAt < 2 * 60 * 1000 ? 5_000 : 15_000;
      timer = setTimeout(poll, delay);
    };

    void poll();
    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
    };
  }, [isDialogOpen, pendingPayment?.channel, pendingPayment?.transactionRowId, refreshInvoice]);

  const handleSubmitTx = async () => {
    if (!txHash.trim() || !user || !pendingPayment?.transactionRowId) return;

    try {
      // The backend keeps the amount, promo and calculated bonus on the draft.
      // The user-facing PATCH is deliberately limited to the blockchain hash.
      await api.patchTransaction(pendingPayment.transactionRowId, {
        transaction_hash: txHash.trim(),
      });
    } catch (e: unknown) {
      if (e instanceof ApiError && e.status === 409 && pendingPayment.transactionRowId) {
        try {
          const transaction = await api.getTransaction(pendingPayment.transactionRowId);
          setPendingPayment(previous => pendingFromTransaction(transaction, previous?.promo));
        } catch (refreshError) {
          console.error("refresh transaction after conflict failed", refreshError);
        }
      }
      notifyError(t("balance.toast.submitError"), e);
      console.error(e);
      return;
    }

    toast.success(t("balance.toast.paymentSent"), {
      duration: 8000,
      description: t("balance.toast.paymentSupport"),
    });

    await addNotification({
      title: t("balance.notif.paymentSuccess"),
      description: t("balance.notif.paymentSuccessDesc").replace("${amount}", `$${pendingPayment.amount.toLocaleString()}`),
      type: "info",
      persistent: false,
    });

    closeDialog();
    setPendingPayment(null);
    setTxHash("");
    clearPendingNotif();
    triggerRefresh();
  };

  const handleCancelPayment = async () => {
    if (isInvoicePaymentChannel(pendingPayment?.channel)) return;
    // Static-wallet drafts may be cancelled before they are sent for review.
    if (pendingPayment?.transactionRowId) {
      try {
        await api.cancelTransaction(pendingPayment.transactionRowId);
      } catch (e) {
        if (e instanceof ApiError && e.status === 409) {
          try {
            const transaction = await api.getTransaction(pendingPayment.transactionRowId);
            setPendingPayment(previous => pendingFromTransaction(transaction, previous?.promo));
          } catch (refreshError) {
            console.error("refresh transaction after cancel conflict failed", refreshError);
          }
        }
        notifyError(t("balance.toast.submitError"), e);
        return;
      }
    }
    closeDialog();
    setPendingPayment(null);
    setTxHash("");
    clearPendingNotif();
    triggerRefresh();
    toast.info(t("balance.toast.paymentCanceled"));
  };

  const handleOpenChange = async (open: boolean) => {
    if (open) {
      openDialog();
      return;
    }
    if (isInvoicePaymentChannel(pendingPayment?.channel)) {
      closeDialog();
      restorePaymentAfterInvoice();
      triggerRefresh();
      return;
    }

    // Closing a static-wallet dialog without submitting the hash keeps the
    // draft and raises the existing persistent reminder.
    if (pendingPayment && !txHash.trim()) {
      closeDialog();
      // If a notification already exists (e.g. dialog was reopened from it),
      // keep it as-is — do NOT clear or recreate.
      if (pendingNotifId && notifications.some(n => n.id === pendingNotifId)) {
        return;
      }
      const bonusAmount = pendingPayment.bonus_amount != null
        ? pendingPayment.bonus_amount
        : (pendingPayment.bonus ? Math.round(pendingPayment.amount * pendingPayment.bonus) / 100 : 0);
      const notificationAmount = pendingPayment.amount + bonusAmount;
      const id = await addNotification({
        title: t("balance.notif.notCompleted"),
        description: `${t("balance.notif.noHash")} $${notificationAmount}`,
        type: "warning",
        persistent: true,
        apiType: "incomplete_topup",
        apiPayload: {
          deposit_amount: pendingPayment.amount,
          // Persist the tx id so a reload can rehydrate full bonus info.
          transaction_id: pendingPayment.transactionRowId ?? null,
        },
        action: { label: t("balance.notif.completePayment"), onClick: () => openDialog() },
        onDismiss: async () => {
          if (pendingPayment.transactionRowId) {
            try { await api.cancelTransaction(pendingPayment.transactionRowId); }
            catch (e) { console.error("cancelTransaction failed", e); }
          }
          setPendingPayment(null);
          setTxHash("");
          pendingNotifId = null;
          triggerRefresh();
          toast.info(t("balance.toast.paymentCanceled"));
        },
      });
      pendingNotifId = id;
      toast(t("balance.toast.notCompleted"), { duration: 5000 });
    } else {
      closeDialog();
    }
  };

  const isInvoice = isInvoicePaymentChannel(pendingPayment?.channel);
  const isCryptomus = pendingPayment?.channel === "cryptomus_invoice";
  const invoiceApproved = isInvoice
    && pendingPayment.status === "approved"
    && !!pendingPayment.credited_at;
  const invoiceFailed = isInvoice
    && (pendingPayment.status === "rejected" || pendingPayment.provider_status === "error");
  const invoiceCancelled = isInvoice && pendingPayment.status === "cancelled";
  const invoicePartial = isInvoice
    && pendingPayment.status === "pending"
    && pendingPayment.provider_status !== "error"
    && Number(pendingPayment.amount_paid) > 0
    && !invoiceApproved;
  const invoiceCreating = isInvoice
    && !pendingPayment.payment_url
    && pendingPayment.provider_status === "create_unknown";

  const finishInvoice = () => {
    closeDialog();
    restorePaymentAfterInvoice();
    triggerRefresh();
  };

  const visibleBonusAmount = pendingPayment?.bonus_amount != null
    ? pendingPayment.bonus_amount
    : pendingPayment?.bonus
      ? Math.round(pendingPayment.amount * pendingPayment.bonus) / 100
      : 0;
  const visibleBalanceIncrease = pendingPayment?.total_balance_increase
    || (pendingPayment ? pendingPayment.amount + visibleBonusAmount : 0);
  const visibleInvoiceCharge = getInvoiceChargeAmount(pendingPayment?.channel, pendingPayment?.amount || 0);

  return (
    <Dialog open={isDialogOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[500px] bg-card border-border">
        <DialogHeader>
          <DialogTitle>
            {isInvoice
              ? t(isCryptomus ? "balance.cryptomus.title" : "balance.passimpay.title")
              : t("balance.paymentTitle")}
            {!isInvoice && pendingPayment ? ` $${visibleBalanceIncrease.toLocaleString()}` : ""}
          </DialogTitle>
          <DialogDescription>
            {isInvoice
              ? t(isCryptomus ? "balance.cryptomus.description" : "balance.passimpay.description")
              : t("balance.paymentDesc")}
          </DialogDescription>
        </DialogHeader>
        <div className="mt-2 space-y-4">
          {isInvoice ? (
            <>
              <div className="space-y-2 rounded-xl border border-primary/20 bg-primary/10 p-4 text-sm">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">{t("balance.passimpay.deposit")}</span>
                  <span className="font-semibold">${pendingPayment?.amount.toLocaleString()}</span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">
                    {t(isCryptomus ? "balance.cryptomus.fee" : "balance.passimpay.fee")}
                  </span>
                  <span className={isCryptomus ? "font-medium text-primary" : "font-medium text-orange-500"}>
                    ${getInvoiceFee(pendingPayment?.channel, pendingPayment?.amount || 0).toLocaleString()}
                  </span>
                </div>
                <div className="border-t border-primary/20 pt-2">
                  <div className="flex items-center justify-between gap-4">
                    <span className="font-medium">{t("balance.passimpay.credit")}</span>
                    <span className="font-bold text-primary">${visibleBalanceIncrease.toLocaleString()}</span>
                  </div>
                  {visibleBonusAmount > 0 && (
                    <p className="mt-1 text-right text-xs text-primary">
                      +${visibleBonusAmount.toLocaleString()} {t("balance.promo.bonusShort")}
                      {pendingPayment?.promo ? ` · ${pendingPayment.promo}` : ""}
                    </p>
                  )}
                </div>
              </div>

              <div className="flex items-center gap-3 rounded-xl border border-border bg-muted/40 p-4">
                {invoiceApproved ? (
                  <CircleCheckBig className="h-6 w-6 shrink-0 text-green-500" />
                ) : invoiceCancelled ? (
                  <CircleX className="h-6 w-6 shrink-0 text-muted-foreground" />
                ) : invoiceFailed && !invoicePartial ? (
                  <CircleX className="h-6 w-6 shrink-0 text-destructive" />
                ) : (
                  <Loader2 className="h-6 w-6 shrink-0 animate-spin text-primary" />
                )}
                <div>
                  <p className="font-medium">
                    {invoiceApproved
                      ? t("balance.passimpay.paid")
                      : invoiceCancelled
                        ? t("balance.passimpay.cancelled")
                        : invoicePartial
                          ? t("balance.passimpay.partial")
                          : invoiceFailed
                            ? t("balance.passimpay.failed")
                            : invoiceCreating || !pendingPayment?.payment_url
                              ? t("balance.passimpay.creatingLink")
                              : t("balance.passimpay.waiting")}
                  </p>
                  {invoicePartial && (
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {t("balance.passimpay.received")
                        .replace("{paid}", `$${Number(pendingPayment?.amount_paid || 0).toLocaleString()}`)
                        .replace("{total}", `$${visibleInvoiceCharge.toLocaleString()}`)}
                    </p>
                  )}
                  {!invoiceApproved && !invoiceFailed && !invoiceCancelled && !invoicePartial && (
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {t(isCryptomus ? "balance.cryptomus.description" : "balance.passimpay.description")}
                    </p>
                  )}
                </div>
              </div>

              {pollingError && (
                <p className="text-sm text-destructive" role="alert">{pollingError}</p>
              )}

              {invoiceApproved || invoiceFailed || invoiceCancelled ? (
                <Button type="button" className="w-full" onClick={finishInvoice}>
                  {t("balance.passimpay.done")}
                </Button>
              ) : (
                <>
                  <Button
                    type="button"
                    className="w-full"
                    disabled={!pendingPayment?.payment_url || checkingInvoice}
                    onClick={() => void handleOpenInvoicePayment()}
                  >
                    <ExternalLink className="mr-2 h-4 w-4" />
                    {pendingPayment?.payment_url
                      ? `${t("balance.passimpay.open")} · $${visibleInvoiceCharge.toLocaleString()}`
                      : t("balance.passimpay.creatingLink")}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full border-border"
                    disabled={checkingInvoice || !pendingPayment?.transactionRowId}
                    onClick={() => {
                      if (pendingPayment?.transactionRowId) {
                        void refreshInvoice(pendingPayment.transactionRowId, true);
                      }
                    }}
                  >
                    {checkingInvoice && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    {checkingInvoice ? t("balance.passimpay.checking") : t("balance.passimpay.check")}
                  </Button>
                </>
              )}
            </>
          ) : (
            <>
              <div className="rounded-lg border border-primary/20 bg-primary/10 p-3">
                <p className="text-sm font-medium">{t("balance.topUpAmount")} <span className="text-primary">${pendingPayment?.amount.toLocaleString()}</span></p>
                {visibleBonusAmount > 0 && pendingPayment ? (() => {
                  const exactPct = pendingPayment.amount > 0 ? (visibleBonusAmount / pendingPayment.amount) * 100 : (pendingPayment.bonus || 0);
                  const pctLabel = Number.isInteger(exactPct) ? exactPct.toString() : exactPct.toFixed(1).replace(/\.0$/, "");
                  return (
                    <>
                      <p className="mt-1 text-sm text-primary">
                        + {t("balance.promo.bonusShort")}: +{visibleBonusAmount}$ {pendingPayment.promo ? `(${pendingPayment.promo}, +${pctLabel}%)` : `(+${pctLabel}%)`}
                      </p>
                      <p className="mt-1 text-sm font-medium">
                        = <span className="text-primary">${visibleBalanceIncrease.toLocaleString()}</span>
                      </p>
                    </>
                  );
                })() : null}
              </div>

              <div className="space-y-2">
                <Label>{t("balance.walletAddress")} ({currentMethod?.label})</Label>
                <div className="flex flex-col gap-2 min-[420px]:flex-row">
                  <Input value={currentMethod?.address || ""} readOnly className="bg-background border-border font-mono text-xs" />
                  <Button variant="outline" size="icon" onClick={() => copyAddress(currentMethod?.address || "")} className="border-border shrink-0">
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">{t("balance.transferNote")}</p>
              </div>

              <div className="space-y-2">
                <Label>{t("balance.txHash")}</Label>
                <Input value={txHash} onChange={(e) => setTxHash(e.target.value)}
                  placeholder="0x..." className="bg-background border-border font-mono text-sm" />
              </div>

              <Button onClick={handleSubmitTx} className="w-full bg-primary hover:bg-primary/90 text-primary-foreground"
                disabled={!txHash.trim()}>
                {t("balance.submit")}
              </Button>

              <Button variant="outline" className="w-full border-border" onClick={handleCancelPayment}>
                {t("balance.cancelPayment")}
              </Button>
            </>
          )}

          <div className="flex items-start gap-2 p-3 rounded-lg bg-muted/50 border border-border">
            <ExternalLink className="h-4 w-4 text-muted-foreground shrink-0 mt-0.5" />
            <div className="text-xs text-muted-foreground space-y-1">
              <p>{t("balance.supportText")}</p>
              {profile?.managerTelegram && (
                <a
                  href={`https://t.me/${profile.managerTelegram}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1.5 text-primary hover:text-primary/80 transition-colors font-medium"
                >
                  <Send className="h-3.5 w-3.5" />
                  @{profile.managerTelegram}
                </a>
              )}
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
