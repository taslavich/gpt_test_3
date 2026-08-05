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
import { useNotifications } from "@/contexts/NotificationContext";
import { useProfile } from "@/contexts/ProfileContext";
import { usePendingPayment, type PendingPaymentData } from "@/contexts/PendingPaymentContext";
import { PAYMENT_METHODS } from "@/lib/paymentMethods";
import { getPassimPayFee, getTransactionBonusAmount, getTransactionChannel } from "@/lib/topup";
import type { ApiUserTransaction } from "@/api/types";

import { api } from "@/api";

// Track the persistent "payment not completed" notification across the whole app.
let pendingNotifId: string | null = null;

function pendingFromTransaction(transaction: ApiUserTransaction, promo?: string): PendingPaymentData {
  const amount = Number(transaction.deposit_amount) || 0;
  const bonusAmount = getTransactionBonusAmount(transaction);
  const bonusPercent = amount > 0 ? bonusAmount / amount * 100 : 0;
  const channel = getTransactionChannel(transaction);
  return {
    amount,
    method: transaction.payment_method || (channel === "passimpay_invoice" ? "passimpay" : "usdt_trc20"),
    channel,
    bonus: bonusPercent || undefined,
    bonus_amount: bonusAmount,
    promo,
    promocode_id: transaction.promocode_id ?? null,
    transaction_id: transaction.id,
    total_balance_increase: Number(transaction.total_balance_increase) || amount + bonusAmount,
    status: transaction.status,
    payment_url: transaction.payment_url,
    provider_status: transaction.provider_status,
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
    triggerRefresh,
  } = usePendingPayment();

  const [txHash, setTxHash] = useState("");
  const [checkingPassimPay, setCheckingPassimPay] = useState(false);
  const hydratedTxRef = useRef<string | null>(null);
  const handlersAttachedRef = useRef<string | null>(null);
  const draftCheckedRef = useRef<string | null>(null);
  const approvedHandledRef = useRef<string | null>(null);

  useEffect(() => {
    if (isDialogOpen) setTxHash("");
  }, [isDialogOpen]);

  // If the user has an unfinished static-wallet draft or PassimPay invoice but
  // no "incomplete_topup" notification, create one so the payment can be resumed.
  // Runs once per user session and ONLY
  // after notifications have been hydrated from the backend (otherwise we'd
  // create a duplicate on every reload before the existing one loads).
  useEffect(() => {
    if (!user) { draftCheckedRef.current = null; return; }
    if (!notificationsLoaded) return;
    if (draftCheckedRef.current === user.id) return;
    const hasIncompleteNotif = notifications.some(n => n.apiType === "incomplete_topup");
    if (hasIncompleteNotif) { draftCheckedRef.current = user.id; return; }
    draftCheckedRef.current = user.id;
    (async () => {
      try {
        const res = await api.listTransactions();
        const items = Array.isArray(res?.items) ? res.items : [];
        const unfinished = items.find(
          x => x.user_id === user.id && (
            (x.status === "draft" && x.payment_channel !== "passimpay_invoice")
            || (
              x.status === "pending"
              && x.payment_channel === "passimpay_invoice"
              && x.provider_status !== "error"
            )
          ),
        );
        if (!unfinished) return;
        // Re-check to avoid race with hydration creating the notif elsewhere.
        if (notifications.some(n => n.apiType === "incomplete_topup" || n.apiPayload?.transaction_id === unfinished.id)) return;
        const depositAmt = Number(unfinished.deposit_amount) || 0;
        const bonusUsd = getTransactionBonusAmount(unfinished);
        const total = depositAmt + bonusUsd;
        const isPassimPay = unfinished.payment_channel === "passimpay_invoice";
        await addNotification({
          title: isPassimPay ? t("balance.passimpay.notCompleted") : t("balance.notif.notCompleted"),
          description: isPassimPay
            ? `${t("balance.passimpay.waiting")} · $${total}`
            : `${t("balance.notif.noHash")} $${total}`,
          type: "warning",
          persistent: true,
          apiType: "incomplete_topup",
          apiPayload: {
            deposit_amount: depositAmt,
            transaction_id: unfinished.id,
          },
          dismissWithoutConfirmation: isPassimPay,
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
    const persisted = notifications.find(n => n.apiType === "incomplete_topup");
    if (!persisted) {
      pendingNotifId = null;
      hydratedTxRef.current = null;
      handlersAttachedRef.current = null;
      return;
    }

    pendingNotifId = persisted.id;
    const txId = persisted.apiPayload?.transaction_id ?? null;

    // Hydrate pendingPayment from backend tx — only ONCE per txId to avoid loop.
    if (!pendingPayment && hydratedTxRef.current !== (txId ?? persisted.id)) {
      hydratedTxRef.current = txId ?? persisted.id;
      if (txId) {
        (async () => {
          try {
            const tx = await api.getTransaction(txId);
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
                transaction_id: txId,
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
        dismissWithoutConfirmation: persisted.title.toLowerCase().includes("passimpay"),
        onDismiss: async () => {
          const txId2 = persisted.apiPayload?.transaction_id ?? null;
          if (txId2) {
            try {
              const transaction = await api.getTransaction(txId2);
              if (getTransactionChannel(transaction) === "static_wallet") {
                await api.cancelTransaction(txId2);
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

  const refreshPassimPay = useCallback(async (transactionId: string, showSpinner = false) => {
    if (showSpinner) setCheckingPassimPay(true);
    try {
      const transaction = await api.getTransaction(transactionId);
      setPendingPayment(previous => pendingFromTransaction(transaction, previous?.promo));
      if (
        transaction.status === "approved"
        && transaction.credited_at
        && approvedHandledRef.current !== transaction.id
      ) {
        approvedHandledRef.current = transaction.id;
        await refetchProfile();
        triggerRefresh();
        if (pendingNotifId) {
          removeNotification(pendingNotifId);
          pendingNotifId = null;
        }
        toast.success(t("balance.passimpay.paid"));
      }
      return transaction;
    } catch (error) {
      if (showSpinner) notifyError(t("balance.toast.submitError"), error);
      else console.error("[topup] PassimPay polling failed", error);
      return null;
    } finally {
      if (showSpinner) setCheckingPassimPay(false);
    }
  }, [refetchProfile, removeNotification, setPendingPayment, t, triggerRefresh]);

  useEffect(() => {
    const transactionId = pendingPayment?.transaction_id;
    if (!isDialogOpen || pendingPayment?.channel !== "passimpay_invoice" || !transactionId) return;

    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const startedAt = Date.now();

    const poll = async () => {
      const transaction = await refreshPassimPay(transactionId);
      if (stopped) return;
      if (transaction?.status === "approved" && transaction.credited_at) return;
      if (transaction?.status === "rejected" || transaction?.provider_status === "error") return;
      const delay = Date.now() - startedAt < 2 * 60 * 1000 ? 5_000 : 15_000;
      timer = setTimeout(poll, delay);
    };

    void poll();
    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
    };
  }, [isDialogOpen, pendingPayment?.channel, pendingPayment?.transaction_id, refreshPassimPay]);

  const handleSubmitTx = async () => {
    if (!txHash.trim() || !user || !pendingPayment?.transaction_id) return;

    try {
      // The backend keeps the amount, promo and calculated bonus on the draft.
      // The user-facing PATCH is deliberately limited to the blockchain hash.
      await api.patchTransaction(pendingPayment.transaction_id, {
        transaction_hash: txHash.trim(),
      });
    } catch (e: unknown) {
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
    if (pendingPayment?.channel === "passimpay_invoice") return;
    // Mark the backend transaction as cancelled so it's purged from the
    // user-visible history (the table filters out `cancelled`).
    if (pendingPayment?.transaction_id) {
      try { await api.cancelTransaction(pendingPayment.transaction_id); }
      catch (e) { console.error("cancelTransaction failed", e); }
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
    if (pendingPayment?.channel === "passimpay_invoice") {
      closeDialog();
      const isTerminal = (pendingPayment.status === "approved" && !!pendingPayment.credited_at)
        || pendingPayment.status === "rejected"
        || pendingPayment.provider_status === "error";
      if (isTerminal) {
        setPendingPayment(null);
        clearPendingNotif();
        triggerRefresh();
        return;
      }
      if (pendingNotifId && notifications.some(n => n.id === pendingNotifId)) return;
      const notificationAmount = pendingPayment.total_balance_increase
        || pendingPayment.amount + (pendingPayment.bonus_amount || 0);
      const id = await addNotification({
        title: t("balance.passimpay.notCompleted"),
        description: `${t("balance.passimpay.waiting")} · $${notificationAmount.toLocaleString()}`,
        type: "warning",
        persistent: true,
        apiType: "incomplete_topup",
        apiPayload: {
          deposit_amount: pendingPayment.amount,
          transaction_id: pendingPayment.transaction_id ?? null,
        },
        action: { label: t("balance.notif.completePayment"), onClick: () => openDialog() },
        dismissWithoutConfirmation: true,
        onDismiss: () => {
          setPendingPayment(null);
          pendingNotifId = null;
          triggerRefresh();
        },
      });
      pendingNotifId = id;
      toast(t("balance.toast.notCompleted"), { duration: 5000 });
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
          transaction_id: pendingPayment.transaction_id ?? null,
        },
        action: { label: t("balance.notif.completePayment"), onClick: () => openDialog() },
        onDismiss: async () => {
          if (pendingPayment.transaction_id) {
            try { await api.cancelTransaction(pendingPayment.transaction_id); }
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

  const isPassimPay = pendingPayment?.channel === "passimpay_invoice";
  const passimPayApproved = isPassimPay
    && pendingPayment.status === "approved"
    && !!pendingPayment.credited_at;
  const passimPayFailed = isPassimPay
    && (pendingPayment.status === "rejected" || pendingPayment.provider_status === "error");
  const passimPayCreating = isPassimPay
    && !pendingPayment.payment_url
    && pendingPayment.provider_status === "create_unknown";

  const finishPassimPay = () => {
    closeDialog();
    setPendingPayment(null);
    clearPendingNotif();
    triggerRefresh();
  };

  const visibleBonusAmount = pendingPayment?.bonus_amount != null
    ? pendingPayment.bonus_amount
    : pendingPayment?.bonus
      ? Math.round(pendingPayment.amount * pendingPayment.bonus) / 100
      : 0;
  const visibleBalanceIncrease = pendingPayment?.total_balance_increase
    || (pendingPayment ? pendingPayment.amount + visibleBonusAmount : 0);

  return (
    <Dialog open={isDialogOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[500px] bg-card border-border">
        <DialogHeader>
          <DialogTitle>
            {isPassimPay ? t("balance.passimpay.title") : t("balance.paymentTitle")}
            {!isPassimPay && pendingPayment ? ` $${visibleBalanceIncrease.toLocaleString()}` : ""}
          </DialogTitle>
          <DialogDescription>
            {isPassimPay ? t("balance.passimpay.description") : t("balance.paymentDesc")}
          </DialogDescription>
        </DialogHeader>
        <div className="mt-2 space-y-4">
          {isPassimPay ? (
            <>
              <div className="space-y-2 rounded-xl border border-primary/20 bg-primary/10 p-4 text-sm">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">{t("balance.passimpay.deposit")}</span>
                  <span className="font-semibold">${pendingPayment?.amount.toLocaleString()}</span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">{t("balance.passimpay.fee")}</span>
                  <span className="font-medium text-orange-500">${getPassimPayFee(pendingPayment?.amount || 0).toLocaleString()}</span>
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
                {passimPayApproved ? (
                  <CircleCheckBig className="h-6 w-6 shrink-0 text-green-500" />
                ) : passimPayFailed ? (
                  <CircleX className="h-6 w-6 shrink-0 text-destructive" />
                ) : (
                  <Loader2 className="h-6 w-6 shrink-0 animate-spin text-primary" />
                )}
                <div>
                  <p className="font-medium">
                    {passimPayApproved
                      ? t("balance.passimpay.paid")
                      : passimPayFailed
                        ? t("balance.passimpay.failed")
                        : passimPayCreating || !pendingPayment?.payment_url
                          ? t("balance.passimpay.creatingLink")
                          : t("balance.passimpay.waiting")}
                  </p>
                  {!passimPayApproved && !passimPayFailed && (
                    <p className="mt-0.5 text-xs text-muted-foreground">{t("balance.passimpay.description")}</p>
                  )}
                </div>
              </div>

              {passimPayApproved || passimPayFailed ? (
                <Button type="button" className="w-full" onClick={finishPassimPay}>
                  {t("balance.passimpay.done")}
                </Button>
              ) : (
                <>
                  <Button
                    type="button"
                    className="w-full"
                    disabled={!pendingPayment?.payment_url}
                    onClick={() => {
                      if (pendingPayment?.payment_url) {
                        window.open(pendingPayment.payment_url, "_blank", "noopener,noreferrer");
                      }
                    }}
                  >
                    <ExternalLink className="mr-2 h-4 w-4" />
                    {pendingPayment?.payment_url
                      ? t("balance.passimpay.open")
                      : t("balance.passimpay.creatingLink")}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full border-border"
                    disabled={checkingPassimPay || !pendingPayment?.transaction_id}
                    onClick={() => {
                      if (pendingPayment?.transaction_id) {
                        void refreshPassimPay(pendingPayment.transaction_id, true);
                      }
                    }}
                  >
                    {checkingPassimPay && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    {checkingPassimPay ? t("balance.passimpay.checking") : t("balance.passimpay.check")}
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
