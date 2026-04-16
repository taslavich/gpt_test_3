import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Wallet, Plus, Receipt, Copy, ExternalLink, Tag } from "lucide-react";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { useNotifications } from "@/contexts/NotificationContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { useProfile } from "@/contexts/ProfileContext";
import { useAuth } from "@/contexts/AuthContext";
import { supabase } from "@/integrations/supabase/client";

const amounts = [100, 250, 500, 1000, 5000];

const usdtMethods = [
  { id: "usdt_trc20", label: "USDT (TRC-20)", desc: "Tether on Tron", address: "TXkRh4pKz7w9Yb2mN5vQx8Gp3jL6fD0eW" },
  { id: "usdt_erc20", label: "USDT (ERC-20)", desc: "Tether on Ethereum", address: "0x3F7a9c2B1d5E8f4A6C0b9D1e2F3a4B5c6D7e8F9a" },
];

interface TopupRequest {
  id: string;
  amount: number;
  created_at: string;
  payment_method: string;
  status: "pending" | "approved" | "rejected";
  promo_code: string | null;
  bonus_percent: number | null;
}

export default function DashboardBalance() {
  const { t } = useLanguage();
  const { user } = useAuth();
  const { profile, loading: profileLoading } = useProfile();
  const { addNotification, removeNotification } = useNotifications();

  const [selectedAmount, setSelectedAmount] = useState<number | null>(250);
  const [customAmount, setCustomAmount] = useState("");
  const [selectedMethod, setSelectedMethod] = useState("usdt_trc20");
  const [showTxDialog, setShowTxDialog] = useState(false);
  const [txHash, setTxHash] = useState("");
  const [promoCode, setPromoCode] = useState("");
  const [appliedPromo, setAppliedPromo] = useState<{ code: string; bonus: number } | null>(null);
  const [pendingPayment, setPendingPayment] = useState<{ amount: number; method: string; promo?: string; bonus?: number } | null>(null);
  const [pendingNotificationId, setPendingNotificationId] = useState<string | null>(null);
  const [topupRequests, setTopupRequests] = useState<TopupRequest[]>([]);
  const [loadingRequests, setLoadingRequests] = useState(true);

  const balance = profile?.balance ?? 0;

  const fetchTopupRequests = async () => {
    if (!user) return;
    setLoadingRequests(true);
    const { data, error } = await supabase
      .from("topup_requests")
      .select("*")
      .eq("user_id", user.id)
      .order("created_at", { ascending: false });
    if (!error && data) setTopupRequests(data as TopupRequest[]);
    setLoadingRequests(false);
  };

  useEffect(() => { fetchTopupRequests(); }, [user]);

  // Check if there's a pending topup request
  const hasPendingRequest = topupRequests.some(r => r.status === "pending");

  const finalAmount = customAmount ? parseInt(customAmount) : selectedAmount;

  const handleApplyPromo = async () => {
    const code = promoCode.trim().toUpperCase();
    if (!code) return;
    const { data, error } = await supabase
      .from("promo_codes")
      .select("*")
      .eq("code", code)
      .eq("is_active", true)
      .single();
    if (error || !data) {
      setAppliedPromo(null);
      toast.error(t("balance.promo.invalid"));
      return;
    }
    // Check expiry
    if (data.expires_at && new Date(data.expires_at) < new Date()) {
      toast.error(t("balance.promo.invalid"));
      return;
    }
    // Check max uses
    if (data.max_uses && data.current_uses >= data.max_uses) {
      toast.error(t("balance.promo.invalid"));
      return;
    }
    setAppliedPromo({ code, bonus: Number(data.bonus_percent) });
    toast.success(t("balance.promo.applied").replace("{percent}", `${data.bonus_percent}`));
  };

  const handleRemovePromo = () => {
    setAppliedPromo(null);
    setPromoCode("");
  };

  const handleTopUp = () => {
    if (!finalAmount || finalAmount < 100) return;
    if (hasPendingRequest || pendingPayment) {
      toast.error(t("balance.toast.pendingExists"));
      return;
    }
    setPendingPayment({ amount: finalAmount, method: selectedMethod, promo: appliedPromo?.code, bonus: appliedPromo?.bonus });
    setTxHash("");
    setShowTxDialog(true);
    if (pendingNotificationId) {
      removeNotification(pendingNotificationId);
      setPendingNotificationId(null);
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

    // Record promo usage if applicable
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

    setShowTxDialog(false);
    setPendingPayment(null);
    setTxHash("");
    setAppliedPromo(null);
    setPromoCode("");
    if (pendingNotificationId) {
      removeNotification(pendingNotificationId);
      setPendingNotificationId(null);
    }
    fetchTopupRequests();
  };

  const handleCancelPayment = () => {
    setShowTxDialog(false);
    setPendingPayment(null);
    setTxHash("");
    if (pendingNotificationId) {
      removeNotification(pendingNotificationId);
      setPendingNotificationId(null);
    }
    toast.info(t("balance.toast.paymentCanceled"));
  };

  const handleCloseTxDialog = (open: boolean) => {
    if (!open && pendingPayment && !txHash.trim()) {
      setShowTxDialog(false);
      const nId = addNotification({
        title: t("balance.notif.notCompleted"),
        description: `${t("balance.notif.noHash")} $${pendingPayment.amount}`,
        type: "warning",
        persistent: true,
        action: { label: t("balance.notif.completePayment"), onClick: () => setShowTxDialog(true) },
        onDismiss: () => {
          setPendingPayment(null);
          setTxHash("");
          setPendingNotificationId(null);
          toast.info(t("balance.toast.paymentCanceled"));
        },
      });
      setPendingNotificationId(nId);
      toast(t("balance.toast.notCompleted"), { duration: 5000 });
    } else {
      setShowTxDialog(open);
    }
  };

  const copyAddress = (address: string) => {
    navigator.clipboard.writeText(address);
    toast.success(t("balance.toast.addressCopied"));
  };

  const currentMethod = usdtMethods.find(m => m.id === selectedMethod);

  const formatDate = (dateStr: string) => {
    const d = new Date(dateStr);
    return `${String(d.getDate()).padStart(2, "0")}.${String(d.getMonth() + 1).padStart(2, "0")}.${d.getFullYear()}`;
  };

  const statusMap: Record<string, { label: string; className: string }> = {
    pending: { label: t("balance.pending"), className: "text-yellow-500 border-yellow-500/20" },
    approved: { label: t("balance.completed"), className: "text-green-500 border-green-500/20" },
    rejected: { label: t("balance.rejected") || "Rejected", className: "text-destructive border-destructive/20" },
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">{t("balance.title")}</h2>
        <p className="text-muted-foreground text-sm">{t("balance.subtitle")}</p>
      </div>

      <div className="grid lg:grid-cols-3 gap-6">
        <Card className="bg-card border-border">
          <CardContent className="p-6">
            <div className="flex items-center gap-3 mb-4">
              <div className="h-12 w-12 rounded-xl bg-primary/20 flex items-center justify-center">
                <Wallet className="h-6 w-6 text-primary" />
              </div>
              <div>
                <p className="text-sm text-muted-foreground">{t("balance.current")}</p>
                <p className="text-3xl font-bold bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
                  {profileLoading ? "..." : `$${balance.toLocaleString()}`}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="lg:col-span-2 bg-card border-border">
          <CardHeader>
            <CardTitle className="text-lg flex items-center gap-2">
              <Plus className="h-5 w-5" />
              {t("balance.topUp")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-5">
            <div className="space-y-2">
              <Label>{t("balance.amount")}</Label>
              <div className="flex flex-wrap gap-2">
                {amounts.map((a) => (
                  <button key={a} onClick={() => { setSelectedAmount(a); setCustomAmount(""); }}
                    className={cn("py-2 px-4 rounded-lg border text-sm font-medium transition-colors",
                      selectedAmount === a && !customAmount ? "border-primary bg-primary/10 text-primary" : "border-border bg-background hover:border-primary/50"
                    )}>
                    ${a.toLocaleString()}
                  </button>
                ))}
              </div>
              <div className="relative max-w-xs">
                <Input placeholder={t("balance.otherAmount")} value={customAmount}
                  onChange={(e) => { setCustomAmount(e.target.value); setSelectedAmount(null); }}
                  className="bg-background border-border pr-8" />
                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">$</span>
              </div>
            </div>

            <div className="space-y-2">
              <Label>{t("balance.network")}</Label>
              <div className="grid sm:grid-cols-2 gap-3">
                {usdtMethods.map((m) => (
                  <button key={m.id} onClick={() => setSelectedMethod(m.id)}
                    className={cn("flex flex-col items-start gap-1 p-4 rounded-lg border transition-colors text-left",
                      selectedMethod === m.id ? "border-primary bg-primary/10" : "border-border bg-background hover:border-primary/50"
                    )}>
                    <span className={cn("text-sm font-medium", selectedMethod === m.id ? "text-foreground" : "text-muted-foreground")}>{m.label}</span>
                    <span className="text-xs text-muted-foreground">{m.desc}</span>
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-2">
              <Label className="flex items-center gap-2">
                <Tag className="h-4 w-4" />
                {t("balance.promo.label")}
              </Label>
              <div className="flex gap-2 max-w-sm">
                <Input
                  placeholder={t("balance.promo.placeholder")}
                  value={promoCode}
                  onChange={(e) => setPromoCode(e.target.value)}
                  className="bg-background border-border uppercase"
                  disabled={!!appliedPromo}
                />
                {appliedPromo ? (
                  <Button variant="outline" onClick={handleRemovePromo} className="border-border shrink-0">
                    {t("balance.promo.remove")}
                  </Button>
                ) : (
                  <Button variant="outline" onClick={handleApplyPromo} className="border-border shrink-0" disabled={!promoCode.trim()}>
                    {t("balance.promo.apply")}
                  </Button>
                )}
              </div>
              {appliedPromo && (
                <p className="text-sm text-primary font-medium">
                  ✓ {t("balance.promo.active").replace("{code}", appliedPromo.code).replace("{percent}", `${appliedPromo.bonus}`)}
                </p>
              )}
            </div>

            <Button onClick={handleTopUp} className="bg-accent hover:bg-accent/90 text-accent-foreground"
              disabled={!finalAmount || finalAmount < 100 || hasPendingRequest}>
              {t("balance.topUpBtn")} {finalAmount ? `$${finalAmount.toLocaleString()}` : ""}
              {appliedPromo && finalAmount ? ` (+${Math.floor(finalAmount * appliedPromo.bonus / 100)}$ ${t("balance.promo.bonusShort")})` : ""}
            </Button>
            <p className="text-xs text-muted-foreground">{t("balance.minAmount")}</p>
          </CardContent>
        </Card>
      </div>

      {/* Transaction Hash Dialog */}
      <Dialog open={showTxDialog} onOpenChange={handleCloseTxDialog}>
        <DialogContent className="sm:max-w-[500px] bg-card border-border">
          <DialogHeader>
            <DialogTitle>{t("balance.paymentTitle")} {pendingPayment ? `$${pendingPayment.amount.toLocaleString()}` : ""}</DialogTitle>
            <DialogDescription>{t("balance.paymentDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 mt-2">
            <div className="p-3 rounded-lg bg-primary/10 border border-primary/20">
              <p className="text-sm font-medium">{t("balance.topUpAmount")} <span className="text-primary">${pendingPayment?.amount.toLocaleString()}</span></p>
              {pendingPayment?.bonus && (
                <p className="text-sm text-primary mt-1">
                  + {t("balance.promo.bonusShort")}: +{Math.floor((pendingPayment.amount * pendingPayment.bonus) / 100)}$ ({pendingPayment.promo}, +{pendingPayment.bonus}%)
                </p>
              )}
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

      {/* Transaction History */}
      <Card className="bg-card border-border">
        <CardHeader>
          <CardTitle className="text-lg flex items-center gap-2">
            <Receipt className="h-5 w-5" /> {t("balance.history")}
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            {loadingRequests ? (
              <div className="py-12 text-center text-muted-foreground">Loading...</div>
            ) : topupRequests.length === 0 ? (
              <div className="py-12 text-center text-muted-foreground">{t("balance.noTransactions")}</div>
            ) : (
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("balance.date")}</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("balance.description")}</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("balance.amountCol")}</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("balance.statusCol")}</th>
                  </tr>
                </thead>
                <tbody>
                  {topupRequests.map((req) => {
                    const methodLabel = usdtMethods.find(m => m.id === req.payment_method)?.label || req.payment_method;
                    const st = statusMap[req.status] || statusMap.pending;
                    return (
                      <tr key={req.id} className="border-b border-border/50 hover:bg-muted/50 transition-colors">
                        <td className="py-3 px-4 text-sm">{formatDate(req.created_at)}</td>
                        <td className="py-3 px-4 text-sm">
                          {t("balance.topUpVia")} · {methodLabel}
                          {req.promo_code && <span className="text-primary ml-1">({req.promo_code})</span>}
                        </td>
                        <td className="py-3 px-4 text-sm text-left font-medium text-green-500">
                          +${req.amount.toLocaleString()}
                        </td>
                        <td className="py-3 px-4 text-left">
                          <Badge variant="outline" className={cn("font-normal", st.className)}>
                            {st.label}
                          </Badge>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
