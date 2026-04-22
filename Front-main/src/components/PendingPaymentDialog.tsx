import { useState, useEffect } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Copy, ExternalLink } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import { useAuth } from "@/contexts/AuthContext";
import { useNotifications } from "@/contexts/NotificationContext";
import { usePendingPayment } from "@/contexts/PendingPaymentContext";
import { supabase } from "@/integrations/supabase/client";

const usdtMethods = [
  { id: "usdt_trc20", label: "USDT (TRC-20)", desc: "Tether on Tron", address: "TXkRh4pKz7w9Yb2mN5vQx8Gp3jL6fD0eW" },
  { id: "usdt_erc20", label: "USDT (ERC-20)", desc: "Tether on Ethereum", address: "0x3F7a9c2B1d5E8f4A6C0b9D1e2F3a4B5c6D7e8F9a" },
];

// Track the persistent "payment not completed" notification across the whole app.
let pendingNotifId: string | null = null;

export function PendingPaymentDialog() {
  const { t } = useLanguage();
  const { user } = useAuth();
  const { addNotification, removeNotification } = useNotifications();
  const {
    pendingPayment, setPendingPayment,
    isDialogOpen, openDialog, closeDialog,
    triggerRefresh,
  } = usePendingPayment();

  const [txHash, setTxHash] = useState("");

  useEffect(() => {
    if (isDialogOpen) setTxHash("");
  }, [isDialogOpen]);

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

    const { error } = await supabase.from("topup_requests").insert({
      user_id: user.id,
      amount: pendingPayment.amount,
      payment_method: pendingPayment.method,
      tx_hash: txHash.trim(),
      promo_code: pendingPayment.promo || null,
      bonus_percent: pendingPayment.bonus || 0,
    });

    if (error) {
      toast.error("Error submitting payment");
      console.error(error);
      return;
    }

    if (pendingPayment.promo) {
      const { data: promoData } = await supabase
        .from("promo_codes")
        .select("id")
        .eq("code", pendingPayment.promo)
        .single();
      if (promoData) {
        await supabase.from("promo_usage").insert({
          user_id: user.id,
          promo_code_id: promoData.id,
        });
      }
    }

    toast.success(t("balance.toast.paymentSent"), {
      duration: 8000,
      description: t("balance.toast.paymentSupport"),
      action: {
        label: "@GregTwinbid",
        onClick: () => window.open("https://t.me/GregTwinbid", "_blank"),
      },
    });

    addNotification({
      title: t("balance.notif.paymentSuccess"),
      description: t("balance.notif.paymentSuccessDesc").replace("${amount}", `$${pendingPayment.amount.toLocaleString()}`),
      type: "info",
      persistent: false,
      action: {
        label: "@GregTwinbid",
        onClick: () => window.open("https://t.me/GregTwinbid", "_blank"),
      },
    });

    closeDialog();
    setPendingPayment(null);
    setTxHash("");
    clearPendingNotif();
    triggerRefresh();
  };

  const handleCancelPayment = () => {
    closeDialog();
    setPendingPayment(null);
    setTxHash("");
    clearPendingNotif();
    toast.info(t("balance.toast.paymentCanceled"));
  };

  const handleOpenChange = (open: boolean) => {
    if (open) {
      openDialog();
      return;
    }
    // Closing without submitting hash → keep pending payment & raise persistent notification
    if (pendingPayment && !txHash.trim()) {
      closeDialog();
      // Avoid duplicating the notification
      clearPendingNotif();
      const id = addNotification({
        title: t("balance.notif.notCompleted"),
        description: `${t("balance.notif.noHash")} $${pendingPayment.amount}`,
        type: "warning",
        persistent: true,
        action: { label: t("balance.notif.completePayment"), onClick: () => openDialog() },
        onDismiss: () => {
          setPendingPayment(null);
          setTxHash("");
          pendingNotifId = null;
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
          <DialogTitle>{t("balance.paymentTitle")} {pendingPayment ? `$${pendingPayment.amount.toLocaleString()}` : ""}</DialogTitle>
          <DialogDescription>{t("balance.paymentDesc")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 mt-2">
          <div className="p-3 rounded-lg bg-primary/10 border border-primary/20">
            <p className="text-sm font-medium">{t("balance.topUpAmount")} <span className="text-primary">${pendingPayment?.amount.toLocaleString()}</span></p>
            {pendingPayment?.bonus ? (
              <p className="text-sm text-primary mt-1">
                + {t("balance.promo.bonusShort")}: +{Math.floor((pendingPayment.amount * pendingPayment.bonus) / 100)}$ ({pendingPayment.promo}, +{pendingPayment.bonus}%)
              </p>
            ) : null}
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

          <div className="flex items-center gap-2 p-3 rounded-lg bg-muted/50 border border-border">
            <ExternalLink className="h-4 w-4 text-muted-foreground shrink-0" />
            <p className="text-xs text-muted-foreground">
              {t("balance.supportText")}{" "}
              <a href="https://t.me/GregTwinbid" target="_blank" rel="noopener" className="text-primary hover:underline">@GregTwinbid</a>{" "}
              {t("balance.inTelegram")}
            </p>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
