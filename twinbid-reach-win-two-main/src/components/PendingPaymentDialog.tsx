import { useState, useEffect } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Copy, ExternalLink, Send } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import { useAuth } from "@/contexts/AuthContext";
import { useNotifications } from "@/contexts/NotificationContext";
import { useProfile } from "@/contexts/ProfileContext";
import { usePendingPayment } from "@/contexts/PendingPaymentContext";

import { api } from "@/api";

const usdtMethods = [
  { id: "usdt_trc20", label: "USDT (TRC-20)", desc: "Tether on Tron", address: "TXkRh4pKz7w9Yb2mN5vQx8Gp3jL6fD0eW" },
  { id: "usdt_erc20", label: "USDT (ERC-20)", desc: "Tether on Ethereum", address: "0x3F7a9c2B1d5E8f4A6C0b9D1e2F3a4B5c6D7e8F9a" },
];

// Track the persistent "payment not completed" notification across the whole app.
let pendingNotifId: string | null = null;

export function PendingPaymentDialog() {
  const { t } = useLanguage();
  const { user } = useAuth();
  const { profile } = useProfile();
  const { notifications, addNotification, removeNotification, attachHandlers } = useNotifications();
  const {
    pendingPayment, setPendingPayment,
    isDialogOpen, openDialog, closeDialog,
    triggerRefresh,
  } = usePendingPayment();

  const [txHash, setTxHash] = useState("");

  useEffect(() => {
    if (isDialogOpen) setTxHash("");
  }, [isDialogOpen]);

  // Re-bind UI handlers to the persisted "incomplete_topup" notification on reload,
  // and rehydrate pendingPayment from the linked transaction so all bonus info
  // (promocode_id, bonus_amount) is restored without re-asking the user.
  useEffect(() => {
    const persisted = notifications.find(n => n.apiType === "incomplete_topup");
    if (!persisted) return;

    pendingNotifId = persisted.id;

    if (!pendingPayment) {
      const txId = persisted.apiPayload?.transaction_id;
      if (txId) {
        // Pull the full transaction (status="created") from backend so we
        // recover promocode_id + bonus_amount + deposit_amount. This is the
        // single source of truth, so we don't need to re-validate the promo
        // string at submit time.
        (async () => {
          try {
            const res = await api.listTransactions();
            const items = Array.isArray(res?.items) ? res.items : [];
            const tx = items.find(x => x.id === txId);
            if (tx) {
              const depositAmt = Number(tx.deposit_amount) || 0;
              // bonus_amount in DB is the PERCENT (e.g. 1.3 means +1.3%), not a $ amount.
              const bonusPct = Number(tx.bonus_amount) || 0;
              const bonusUsd = (depositAmt * bonusPct) / 100;
              // Resolve promo code name from promo_codes table
              let promoName: string | undefined;
              if (tx.promocode_id) {
                try {
                  const { supabase } = await import("@/integrations/supabase/client");
                  const { data } = await supabase.from("promo_codes").select("code").eq("id", tx.promocode_id).maybeSingle();
                  if (data?.code) promoName = data.code;
                } catch (e) { console.error("promo code lookup failed", e); }
              }
              setPendingPayment({
                amount: depositAmt,
                method: tx.payment_method || "usdt_trc20",
                bonus: bonusPct || undefined,
                bonus_amount: bonusUsd,
                promo: promoName,
                promocode_id: tx.promocode_id ?? null,
                transaction_id: tx.id,
              });
            } else if (persisted.apiPayload?.deposit_amount) {
              setPendingPayment({
                amount: Number(persisted.apiPayload.deposit_amount) || 0,
                method: "usdt_trc20",
                transaction_id: txId,
              });
            }
          } catch (e) {
            console.error("[topup] resume: listTransactions failed", e);
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
    attachHandlers(persisted.id, {
      action: { label: t("balance.notif.completePayment"), onClick: () => openDialog() },
      onDismiss: () => {
        // Cancel the underlying transaction (status -> cancelled) on dismiss.
        const txId = pendingPayment?.transaction_id ?? persisted.apiPayload?.transaction_id ?? null;
        if (txId) api.cancelTransaction(txId).catch(e => console.error("cancelTransaction failed", e));
        setPendingPayment(null);
        setTxHash("");
        pendingNotifId = null;
        triggerRefresh();
        toast.info(t("balance.toast.paymentCanceled"));
      },
    });
    // We intentionally depend only on the notifications list snapshot
  }, [notifications]);

  const currentMethod = usdtMethods.find(m => m.id === pendingPayment?.method);

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

  const handleSubmitTx = async () => {
    if (!txHash.trim() || !user || !pendingPayment) return;

    try {
      const hash = txHash.trim();
      const depositAmount = pendingPayment.amount;
      const bonusPercent = pendingPayment.bonus || 0;
      const bonusAmount = Math.floor((depositAmount * bonusPercent) / 100);
      const totalBalanceIncrease = depositAmount + bonusAmount;

      // Prefer PATCH on the existing draft transaction so the promo info
      // (promocode_id, bonus_amount, total_balance_increase) is preserved
      // server-side without re-validating the promo code.
      let patched = false;
      let carriedPromocodeId: string | null = pendingPayment.promocode_id ?? null;
      let carriedPromoCode: string | null = pendingPayment.promo ?? null;

      if (pendingPayment.transaction_id) {
        try {
          const res = await api.listTransactions();
          const items = Array.isArray(res?.items) ? res.items : [];
          const draft = items.find(x => x.id === pendingPayment.transaction_id);
          if (draft?.promocode_id) carriedPromocodeId = draft.promocode_id;
          if (!carriedPromoCode && draft?.promocode_id) {
            try {
              const map = JSON.parse(localStorage.getItem("twinbid_promo_codes") || "{}");
              carriedPromoCode = map[draft.id] || map[draft.promocode_id] || null;
            } catch {}
          }
        } catch (e) {
          console.warn("[topup] could not fetch draft promocode_id", e);
        }

        try {
          await api.patchTransaction(pendingPayment.transaction_id, {
            transaction_id: hash,
            transaction_hash: hash,
            status: "pending",
          });
          patched = true;
        } catch (patchErr: any) {
          const msg = String(patchErr?.message || "").toLowerCase();
          const is404 = msg.includes("404") || msg.includes("not found");
          if (!is404) throw patchErr;
          console.warn("[topup] PATCH not supported, falling back to cancel+recreate");
          try { await api.cancelTransaction(pendingPayment.transaction_id); }
          catch (e) { console.error("cancel during fallback failed", e); }
        }
      }

      if (!patched) {
        const baseBody = {
          user_id: user.id,
          transaction_time: new Date().toISOString(),
          transaction_id: hash,
          payment_method: pendingPayment.method,
          bonus_amount: bonusAmount,
          transaction_hash: hash,
          deposit_amount: depositAmount,
          total_balance_increase: totalBalanceIncrease,
          status: "pending" as const,
          currency: "usdt",
        };

        let created;
        try {
          // Keep the working fallback: use the original promo code when we have it
          // so backend resolves and writes the DB promo id. If that validation fails,
          // retry without promo instead of blocking payment submission.
          created = await api.createTransaction({
            ...baseBody,
            promocode_id: carriedPromoCode || null,
          });
        } catch (createWithPromoErr) {
          console.warn("[topup] recreate with promo failed, retrying without promo", createWithPromoErr);
          created = await api.createTransaction({
            ...baseBody,
            promocode_id: null,
          });
        }

        if (created?.id && carriedPromocodeId) {
          try {
            const map = JSON.parse(localStorage.getItem("twinbid_promo_codes") || "{}");
            if (carriedPromoCode) {
              map[created.id] = carriedPromoCode;
              map[carriedPromocodeId] = carriedPromoCode;
            }
            localStorage.setItem("twinbid_promo_codes", JSON.stringify(map));
          } catch {}
        }
      }
    } catch (e: any) {
      toast.error(`${t("balance.toast.submitError") || "Error submitting payment"}: ${e?.message || e}`);
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
    // Closing without submitting hash → keep pending payment & raise persistent notification
    if (pendingPayment && !txHash.trim()) {
      closeDialog();
      // If a notification already exists (e.g. dialog was reopened from it),
      // keep it as-is — do NOT clear or recreate.
      if (pendingNotifId && notifications.some(n => n.id === pendingNotifId)) {
        return;
      }
      const bonusAmount = pendingPayment.bonus_amount != null
        ? pendingPayment.bonus_amount
        : (pendingPayment.bonus ? Math.floor((pendingPayment.amount * pendingPayment.bonus) / 100) : 0);
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

  return (
    <Dialog open={isDialogOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[500px] bg-card border-border">
        <DialogHeader>
          <DialogTitle>{t("balance.paymentTitle")} {pendingPayment ? `$${((pendingPayment.amount || 0) + (pendingPayment.bonus_amount ?? (pendingPayment.bonus ? Math.floor((pendingPayment.amount * pendingPayment.bonus) / 100) : 0))).toLocaleString()}` : ""}</DialogTitle>
          <DialogDescription>{t("balance.paymentDesc")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 mt-2">
          <div className="p-3 rounded-lg bg-primary/10 border border-primary/20">
            <p className="text-sm font-medium">{t("balance.topUpAmount")} <span className="text-primary">${pendingPayment?.amount.toLocaleString()}</span></p>
            {pendingPayment && (pendingPayment.bonus_amount || pendingPayment.bonus) ? (() => {
              const bonusAmt = pendingPayment.bonus_amount ?? Math.floor((pendingPayment.amount * (pendingPayment.bonus || 0)) / 100);
              const total = (pendingPayment.amount || 0) + bonusAmt;
              const exactPct = pendingPayment.amount > 0 ? (bonusAmt / pendingPayment.amount) * 100 : (pendingPayment.bonus || 0);
              const pctLabel = Number.isInteger(exactPct) ? exactPct.toString() : exactPct.toFixed(1).replace(/\.0$/, "");
              return (
                <>
                  <p className="text-sm text-primary mt-1">
                    + {t("balance.promo.bonusShort")}: +{bonusAmt}$ {pendingPayment.promo ? `(${pendingPayment.promo}, +${pctLabel}%)` : `(+${pctLabel}%)`}
                  </p>
                  <p className="text-sm font-medium mt-1">
                    = <span className="text-primary">${total.toLocaleString()}</span>
                  </p>
                </>
              );
            })() : null}
          </div>

          <div className="space-y-2">
            <Label>{t("balance.walletAddress")} ({currentMethod?.label})</Label>
            <div className="flex gap-2">
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
