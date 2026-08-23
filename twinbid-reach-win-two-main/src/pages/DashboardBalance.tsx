import { useState, useEffect, useRef, useCallback } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { CreditCard, ExternalLink, Wallet, Plus, Receipt, Tag } from "lucide-react";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { getLocalizedErrorMessage, notifyError } from "@/lib/apiStatus";
import { useLanguage } from "@/contexts/LanguageContext";
import { useProfile } from "@/contexts/ProfileContext";
import { useAuth } from "@/contexts/AuthContext";
import { usePendingPayment } from "@/contexts/PendingPaymentContext";
import { api, ApiError } from "@/api";
import { supabase } from "@/integrations/supabase/client";
import type { ApiUserTransaction, PaymentChannel } from "@/api/types";
import { PAYMENT_METHODS } from "@/lib/paymentMethods";
import { formatCurrencyAmount } from "@/lib/numberFormat";
import { PassimPayBrand } from "@/components/dashboard/PassimPayBrand";
import { CryptomusBrand } from "@/components/dashboard/CryptomusBrand";
import { rememberTopupForGoal, trackBalanceTopupSuccess } from "@/lib/yandexMetrikaTopup";
import {
  buildCryptomusTopup,
  buildPassimPayTopup,
  buildStaticWalletTopup,
  CRYPTOMUS_FEE_PERCENT,
  getInvoiceChargeAmount,
  getPassimPayFee,
  getTransactionHistoryRefreshInterval,
  getTransactionBonusAmount,
  isInvoicePaymentChannel,
  isInvoicePartial,
  isTransactionCredited,
  isUnfinishedStaticWalletTransaction,
  parseTopupAmount,
  PASSIMPAY_FEE_PERCENT,
  sanitizeTopupAmountInput,
  validatePromocodeForTopup,
} from "@/lib/topup";

const fmtMoney = (n: number | string | null | undefined) =>
  formatCurrencyAmount(Number(n || 0));

const amounts = [100, 250, 500, 1000, 5000];

type TopupRequest = ApiUserTransaction;

export default function DashboardBalance() {
  const { t } = useLanguage();
  const { user } = useAuth();
  const { profile, loading: profileLoading } = useProfile();
  const {
    pendingPayment, openPayment, registerRefreshHandler,
  } = usePendingPayment();

  const [selectedAmount, setSelectedAmount] = useState<number | null>(250);
  const [customAmount, setCustomAmount] = useState("");
  const [selectedChannel, setSelectedChannel] = useState<PaymentChannel | null>(null);
  const [selectedMethod, setSelectedMethod] = useState("usdt_trc20");
  const [promoCode, setPromoCode] = useState("");
  const [appliedPromo, setAppliedPromo] = useState<{ code: string; bonus: number; id: string } | null>(null);
  const [topupRequests, setTopupRequests] = useState<TopupRequest[]>([]);
  const [loadingRequests, setLoadingRequests] = useState(true);
  const [submittingTopup, setSubmittingTopup] = useState(false);
  const [topupError, setTopupError] = useState<string | null>(null);
  const topupSubmitLockRef = useRef(false);

  const [promoNames, setPromoNames] = useState<Record<string, string>>({});

  const balance = profile?.balance ?? 0;

  const fetchTopupRequests = useCallback(async (showLoading = true) => {
    if (!user) return;
    if (showLoading) setLoadingRequests(true);
    try {
      const res = await api.listTransactions();
      const items = Array.isArray(res?.items) ? res.items : [];
      items.forEach(trackBalanceTopupSuccess);
      setTopupRequests(items);
    } catch (e) {
      console.error("Topups fetch error:", e);
    } finally {
      if (showLoading) setLoadingRequests(false);
    }
  }, [user]);

  useEffect(() => { void fetchTopupRequests(); }, [fetchTopupRequests]);

  // Resolve promo code names (id → code text) for transactions that reference one.
  useEffect(() => {
    const ids = Array.from(new Set(
      topupRequests
        .map(r => r.promocode_id)
        .filter((v): v is string => !!v && /^[0-9a-f-]{36}$/i.test(v) && !promoNames[v])
    ));
    if (ids.length === 0) return;
    (async () => {
      try {
        const { data } = await supabase.from("promo_codes").select("id, code").in("id", ids);
        if (data) {
          setPromoNames(prev => {
            const next = { ...prev };
            data.forEach((p: { id: string; code: string }) => { next[p.id] = p.code; });
            return next;
          });
        }
      } catch (e) { console.warn("promo names fetch failed", e); }
    })();
  }, [topupRequests]);

  const historyRefreshInterval = getTransactionHistoryRefreshInterval(topupRequests);

  // Keep active invoice statuses fresh; otherwise retain the normal 5-minute refresh.
  useEffect(() => {
    if (!user) return;
    const interval = setInterval(() => void fetchTopupRequests(false), historyRefreshInterval);
    const onFocus = () => void fetchTopupRequests(false);
    window.addEventListener("focus", onFocus);
    return () => { clearInterval(interval); window.removeEventListener("focus", onFocus); };
  }, [user, historyRefreshInterval, fetchTopupRequests]);

  // Allow the global dialog to refresh our history after submission
  useEffect(() => {
    registerRefreshHandler(() => void fetchTopupRequests(false));
  }, [fetchTopupRequests, registerRefreshHandler]);

  const finalAmount = customAmount ? parseTopupAmount(customAmount) : selectedAmount;
  const promoPreviewBonus = finalAmount && appliedPromo
    ? Math.round((finalAmount * appliedPromo.bonus + Number.EPSILON)) / 100
    : 0;
  const passimPayFee = finalAmount ? getPassimPayFee(finalAmount) : 0;
  const selectedInvoiceChargeAmount = finalAmount
    ? getInvoiceChargeAmount(selectedChannel ?? undefined, finalAmount)
    : 0;

  const handleApplyPromo = async () => {
    const code = promoCode.trim().toUpperCase();
    if (!code) return;
    try {
      const promo = await api.getPromocode(code);
      const validationFailure = validatePromocodeForTopup({
        promo,
        transactions: topupRequests,
        userId: user?.id,
      });
      if (validationFailure === "expired" || validationFailure === "limit") {
        toast.error(t("balance.promo.invalid"));
        return;
      }
      if (validationFailure === "already_used") {
        toast.error(t("balance.promo.alreadyUsed"));
        return;
      }
      setAppliedPromo({ code, bonus: Number(promo.bonus_percent), id: promo.id });
      toast.success(t("balance.promo.applied").replace("{percent}", `${promo.bonus_percent}`));
    } catch {
      setAppliedPromo(null);
      toast.error(t("balance.promo.invalid"));
    }
  };

  const handleRemovePromo = () => {
    setAppliedPromo(null);
    setPromoCode("");
  };

  const hasUnfinishedStaticWallet = topupRequests.some(
    tx => isUnfinishedStaticWalletTransaction(tx)
      && (!user || tx.user_id === user.id),
  );
  const hasPendingStaticWalletDialog = !!pendingPayment
    && pendingPayment.channel === "static_wallet";
  const isTopUpBlocked = selectedChannel === "static_wallet"
    && (hasPendingStaticWalletDialog || hasUnfinishedStaticWallet);

  const handleTopUp = async () => {
    if (!finalAmount || finalAmount < 100 || !user || !selectedChannel || submittingTopup || topupSubmitLockRef.current) return;
    if (isTopUpBlocked) {
      toast.error(t("balance.disabledReason"));
      return;
    }
    topupSubmitLockRef.current = true;
    setSubmittingTopup(true);
    setTopupError(null);
    let paymentWindow: Window | null = null;
    if (isInvoicePaymentChannel(selectedChannel)) {
      try {
        paymentWindow = window.open("", "_blank");
        if (paymentWindow) paymentWindow.opener = null;
      } catch {
        // The dialog keeps the backend payment URL as a regular fallback link.
      }
    }
    const promoCodeText = appliedPromo?.code ?? null;
    try {
      const body = selectedChannel === "passimpay_invoice"
        ? buildPassimPayTopup({ depositAmount: finalAmount, promoCode: promoCodeText })
        : selectedChannel === "cryptomus_invoice"
          ? buildCryptomusTopup({ depositAmount: finalAmount, promoCode: promoCodeText })
          : buildStaticWalletTopup({
            paymentMethod: selectedMethod,
            depositAmount: finalAmount,
            promoCode: promoCodeText,
          });
      const created = await api.createTransaction(body);
      const transactionRowId = created.id;
      const transactionChannel = created.payment_channel || selectedChannel;
      const bonusAmount = getTransactionBonusAmount(created);

      rememberTopupForGoal(transactionRowId);
      trackBalanceTopupSuccess(created);

      if (transactionRowId && appliedPromo) {
        try {
          const map = JSON.parse(localStorage.getItem("twinbid_promo_codes") || "{}");
          map[transactionRowId] = appliedPromo.code;
          map[appliedPromo.id] = appliedPromo.code;
          localStorage.setItem("twinbid_promo_codes", JSON.stringify(map));
        } catch { /* local display cache is best-effort */ }
      }

      openPayment({
        amount: Number(created.deposit_amount) || finalAmount,
        method: created.payment_method
          || (transactionChannel === "passimpay_invoice"
            ? "passimpay"
            : transactionChannel === "cryptomus_invoice"
              ? "cryptomus"
              : selectedMethod),
        channel: transactionChannel,
        promo: appliedPromo?.code,
        bonus: appliedPromo?.bonus,
        bonus_amount: bonusAmount,
        promocode_id: created.promocode_id ?? appliedPromo?.id ?? null,
        transactionRowId,
        total_balance_increase: Number(created.total_balance_increase) || finalAmount + bonusAmount,
        status: created.status,
        payment_url: created.payment_url,
        provider_status: created.provider_status,
        amount_paid: created.amount_paid,
        amount_credited: created.amount_credited,
        credited_at: created.credited_at,
      });
      setAppliedPromo(null);
      setPromoCode("");
      if (isInvoicePaymentChannel(transactionChannel) && created.payment_url) {
        if (paymentWindow && !paymentWindow.closed) {
          try {
            paymentWindow.location.href = created.payment_url;
          } catch {
            paymentWindow.close();
          }
        }
      } else if (paymentWindow && !paymentWindow.closed) {
        paymentWindow.close();
      }
      void fetchTopupRequests();
    } catch (e: unknown) {
      if (paymentWindow && !paymentWindow.closed) paymentWindow.close();
      const message = isInvoicePaymentChannel(selectedChannel) && e instanceof ApiError && e.status === 503
        ? t(selectedChannel === "cryptomus_invoice" ? "balance.cryptomus.unavailable" : "balance.passimpay.unavailable")
        : getLocalizedErrorMessage(e, t);
      setTopupError(message);
      notifyError(t("balance.toast.submitError"), e);
    } finally {
      topupSubmitLockRef.current = false;
      setSubmittingTopup(false);
    }
  };

  const formatDate = (dateStr: string) => {
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return "—";
    return `${String(d.getUTCDate()).padStart(2, "0")}.${String(d.getUTCMonth() + 1).padStart(2, "0")}.${d.getUTCFullYear()}`;
  };

  const statusMap: Record<string, { label: string; className: string }> = {
    draft: { label: t("balance.created"), className: "text-muted-foreground border-border" },
    pending: { label: t("balance.pending"), className: "text-yellow-500 border-yellow-500/20" },
    approved: { label: t("balance.completed"), className: "text-green-500 border-green-500/20" },
    rejected: { label: t("balance.rejected") || "Rejected", className: "text-destructive border-destructive/20" },
    cancelled: { label: t("balance.cancelled"), className: "text-muted-foreground border-border" },
  };

  const openInvoiceFromHistory = (transaction: ApiUserTransaction) => {
    const amount = Number(transaction.deposit_amount) || 0;
    const bonusAmount = getTransactionBonusAmount(transaction);
    let promoName = transaction.promocode_id ? promoNames[transaction.promocode_id] : undefined;
    if (!promoName && transaction.promocode_id) {
      try {
        const map = JSON.parse(localStorage.getItem("twinbid_promo_codes") || "{}");
        promoName = map[transaction.id] || map[transaction.promocode_id] || undefined;
      } catch { /* local display cache is best-effort */ }
    }

    openPayment({
      amount,
      method: transaction.payment_method || (transaction.payment_channel === "cryptomus_invoice" ? "cryptomus" : "passimpay"),
      channel: transaction.payment_channel === "cryptomus_invoice" ? "cryptomus_invoice" : "passimpay_invoice",
      promo: promoName,
      bonus: amount > 0 && bonusAmount > 0 ? bonusAmount / amount * 100 : undefined,
      bonus_amount: bonusAmount,
      promocode_id: transaction.promocode_id ?? null,
      transactionRowId: transaction.id,
      total_balance_increase: Number(transaction.total_balance_increase) || amount + bonusAmount,
      status: transaction.status,
      payment_url: transaction.payment_url,
      provider_status: transaction.provider_status,
      amount_paid: transaction.amount_paid,
      amount_credited: transaction.amount_credited,
      credited_at: transaction.credited_at,
    });
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">{t("balance.title")}</h2>
        <p className="text-muted-foreground text-sm">{t("balance.subtitle")}</p>
      </div>

      <div className="grid lg:grid-cols-3 gap-6">
        <Card className="bg-card border-border">
          <CardContent className="p-4 sm:p-6">
            <div className="flex items-center gap-3 mb-4">
              <div className="h-12 w-12 rounded-xl bg-primary/20 flex items-center justify-center">
                <Wallet className="h-6 w-6 text-primary" />
              </div>
              <div>
                <p className="text-sm text-muted-foreground">{t("balance.current")}</p>
                <p className="break-all text-2xl font-bold bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent sm:text-3xl">
                  {profileLoading ? "..." : `$${fmtMoney(balance)}`}
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
                  inputMode="decimal"
                  onChange={(e) => {
                    const nextValue = sanitizeTopupAmountInput(e.target.value);
                    if (nextValue === null) return;
                    setCustomAmount(nextValue);
                    setSelectedAmount(null);
                  }}
                  className="bg-background border-border pr-8" />
                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">$</span>
              </div>
            </div>

            <div className="space-y-3">
              <Label>{t("balance.paymentMethod")}</Label>
              <div className="grid gap-3 sm:grid-cols-3">
                <button
                  type="button"
                  onClick={() => {
                    setSelectedChannel("static_wallet");
                    setTopupError(null);
                  }}
                  className={cn(
                    "relative flex min-h-24 items-center gap-3 rounded-xl border p-4 text-left transition-colors",
                    selectedChannel === "static_wallet"
                      ? "border-primary bg-primary/10"
                      : "border-border bg-background hover:border-primary/50",
                  )}
                >
                  <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary/15 text-primary">
                    <Wallet className="h-5 w-5" />
                  </span>
                  <span className="min-w-0">
                    <span className="block font-semibold">TwinBid Crypto</span>
                    <span className="mt-1 block text-xs text-muted-foreground">{t("balance.crypto.choose")}</span>
                  </span>
                  <Badge className="absolute right-3 top-3 border-primary/30 bg-primary/15 px-2.5 py-1 text-sm font-bold text-primary hover:bg-primary/15">
                    0%
                  </Badge>
                </button>

                <button
                  type="button"
                  onClick={() => {
                    setSelectedChannel("passimpay_invoice");
                    setTopupError(null);
                  }}
                  className={cn(
                    "relative flex min-h-24 items-center rounded-xl border p-4 pr-16 text-left transition-colors",
                    selectedChannel === "passimpay_invoice"
                      ? "border-primary bg-primary/10"
                      : "border-border bg-background hover:border-primary/50",
                  )}
                >
                  <PassimPayBrand />
                  <Badge className="absolute right-3 top-3 border-orange-500/30 bg-orange-500/10 px-2.5 py-1 text-sm font-bold text-orange-600 hover:bg-orange-500/10 dark:text-orange-400">
                    {PASSIMPAY_FEE_PERCENT}%
                  </Badge>
                </button>

                <button
                  type="button"
                  onClick={() => {
                    setSelectedChannel("cryptomus_invoice");
                    setTopupError(null);
                  }}
                  className={cn(
                    "relative flex min-h-24 items-center rounded-xl border p-4 pr-16 text-left transition-colors",
                    selectedChannel === "cryptomus_invoice"
                      ? "border-primary bg-primary/10"
                      : "border-border bg-background hover:border-primary/50",
                  )}
                >
                  <CryptomusBrand />
                  <Badge className="absolute right-3 top-3 border-primary/30 bg-primary/15 px-2.5 py-1 text-sm font-bold text-primary hover:bg-primary/15">
                    {CRYPTOMUS_FEE_PERCENT}%
                  </Badge>
                </button>
              </div>

              {selectedChannel === "static_wallet" ? (
                <div className="grid gap-3 rounded-xl border border-border bg-muted/20 p-3 sm:grid-cols-2">
                  {PAYMENT_METHODS.map((m) => (
                    <button key={m.id} type="button" onClick={() => setSelectedMethod(m.id)}
                      className={cn("flex flex-col items-start gap-1 rounded-lg border p-4 text-left transition-colors",
                        selectedMethod === m.id ? "border-primary bg-primary/10" : "border-border bg-background hover:border-primary/50"
                      )}>
                      <span className={cn("text-sm font-medium", selectedMethod === m.id ? "text-foreground" : "text-muted-foreground")}>{m.label}</span>
                      <span className="text-xs text-muted-foreground">{m.desc}</span>
                    </button>
                  ))}
                </div>
              ) : selectedChannel === "passimpay_invoice" ? (
                <div className="flex items-start gap-3 rounded-xl border border-orange-500/20 bg-orange-500/5 p-3 text-sm">
                  <CreditCard className="mt-0.5 h-4 w-4 shrink-0 text-orange-500" />
                  <p className="text-muted-foreground">
                    {t("balance.passimpay.feeHint")
                      .replace("{amount}", finalAmount ? `$${fmtMoney(finalAmount)}` : "—")
                      .replace("{fee}", finalAmount ? `$${fmtMoney(passimPayFee)}` : "—")}
                  </p>
                </div>
              ) : selectedChannel === "cryptomus_invoice" ? (
                <div className="flex items-start gap-3 rounded-xl border border-primary/20 bg-primary/5 p-3 text-sm">
                  <CreditCard className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                  <p className="text-muted-foreground">{t("balance.cryptomus.feeHint")}</p>
                </div>
              ) : null}
            </div>

            <div className="space-y-2">
              <Label className="flex items-center gap-2">
                <Tag className="h-4 w-4" />
                {t("balance.promo.label")}
              </Label>
              <div className="flex max-w-sm flex-col gap-2 min-[420px]:flex-row">
                <Input
                  placeholder={t("balance.promo.placeholder")}
                  value={promoCode}
                  onChange={(e) => setPromoCode(e.target.value)}
                  className="bg-background border-border uppercase"
                  disabled={!!appliedPromo}
                />
                {appliedPromo ? (
                  <Button variant="outline" onClick={handleRemovePromo} className="shrink-0 border-border">
                    {t("balance.promo.remove")}
                  </Button>
                ) : (
                  <Button variant="outline" onClick={handleApplyPromo} className="shrink-0 border-border" disabled={!promoCode.trim()}>
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

            <div className="flex flex-col sm:flex-row sm:items-center gap-3">
              <Button onClick={handleTopUp} className="h-auto min-h-10 w-full whitespace-normal bg-accent py-2 hover:bg-accent/90 text-accent-foreground sm:w-auto"
                disabled={!finalAmount || finalAmount < 100 || !selectedChannel || isTopUpBlocked || submittingTopup}>
                {submittingTopup
                  ? t("balance.payment.creating")
                  : selectedChannel === "passimpay_invoice"
                    ? t("balance.passimpay.create")
                    : selectedChannel === "cryptomus_invoice"
                      ? t("balance.cryptomus.create")
                    : t("balance.topUpBtn")} {finalAmount
                      ? `$${fmtMoney(isInvoicePaymentChannel(selectedChannel) ? selectedInvoiceChargeAmount : finalAmount)}`
                      : ""}
                {appliedPromo && finalAmount ? ` (+${fmtMoney(promoPreviewBonus)}$ ${t("balance.promo.bonusShort")})` : ""}
              </Button>
              {isTopUpBlocked && (
                <p className="text-xs text-yellow-500">
                  {t("balance.disabledReason")}
                </p>
              )}
            </div>
            {topupError && (
              <p className="text-sm text-destructive" role="alert">{topupError}</p>
            )}
            <div className="space-y-1 text-xs text-muted-foreground">
              <p>{t("balance.minAmount")}</p>
              <a
                href="https://t.me/twinbid/712"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 font-medium text-primary underline decoration-primary/40 underline-offset-4 transition-colors hover:decoration-primary"
              >
                {t("balance.promo.telegramPost")}
                <ExternalLink className="h-3 w-3" />
              </a>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Transaction History */}
      <Card className="bg-card border-border">
        <CardHeader>
          <CardTitle className="text-lg flex items-center gap-2">
            <Receipt className="h-5 w-5" /> {t("balance.history")}
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="max-w-full overflow-x-auto overscroll-x-contain">
            {(() => {
              const visible = topupRequests;
              if (loadingRequests) {
                return <div className="py-12 text-center text-muted-foreground">Loading...</div>;
              }
              if (visible.length === 0) {
                return <div className="py-12 text-center text-muted-foreground">{t("balance.noTransactions")}</div>;
              }
              return (
              <table className="w-full min-w-[680px]">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("balance.date")}</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("balance.description")}</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("balance.amountCol")}</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">{t("balance.statusCol")}</th>
                  </tr>
                </thead>
                <tbody>
                  {visible.map((req) => {
                    const isInvoice = isInvoicePaymentChannel(req.payment_channel);
                    const methodLabel = req.payment_channel === "passimpay_invoice"
                      ? "PassimPay"
                      : req.payment_channel === "cryptomus_invoice"
                        ? "Cryptomus"
                        : "TwinBid Crypto";
                    const credited = isTransactionCredited(req);
                    const partial = isInvoice
                      && req.status === "pending"
                      && req.provider_status !== "error"
                      && isInvoicePartial(req);
                    const historyStatus = credited
                      ? "approved"
                      : req.status === "approved"
                        ? "pending"
                        : req.status;
                    const baseStatus = statusMap[historyStatus] || statusMap.pending;
                    const providerStatus = isInvoice
                      && req.status === "pending"
                      && !credited
                      ? partial
                        ? { label: t("balance.passimpay.partial"), className: "text-yellow-500 border-yellow-500/20" }
                        : req.provider_status === "create_unknown"
                          ? { label: t("balance.passimpay.checkingPayment"), className: "text-yellow-500 border-yellow-500/20" }
                          : req.provider_status === "waiting" && req.status === "pending"
                            ? { label: t("balance.passimpay.waiting"), className: "text-yellow-500 border-yellow-500/20" }
                            : req.provider_status === "error"
                              ? { label: t("balance.passimpay.error"), className: "text-destructive border-destructive/20" }
                              : null
                      : null;
                    const st = providerStatus || baseStatus;
                    let promoLabel: string | null = null;
                    if (req.promocode_id) {
                      // Prefer the resolved DB name; fall back to the local cache used by submission flow.
                      promoLabel = promoNames[req.promocode_id] || null;
                      if (!promoLabel) {
                        try {
                          const map = JSON.parse(localStorage.getItem("twinbid_promo_codes") || "{}");
                          promoLabel = map[req.id] || map[req.promocode_id] || null;
                        } catch { /* local display cache is best-effort */ }
                      }
                    }
                    const bonusAmt = Math.max(0, Number(req.total_balance_increase || 0) - Number(req.deposit_amount || 0));
                    return (
                      <tr key={req.id} className="border-b border-border/50 hover:bg-muted/50 transition-colors">
                        <td className="py-3 px-4 text-sm">{formatDate(req.created_at || req.transaction_time || "")}</td>
                        <td className="py-3 px-4 text-sm">
                          {t("balance.topUpVia")} · {methodLabel}
                          {req.promocode_id && (
                            <span className="text-primary ml-1">
                              ({promoLabel || t("balance.promo.label")}{bonusAmt > 0 ? `, +$${fmtMoney(bonusAmt)}` : ""})
                            </span>
                          )}
                        </td>
                        <td className={cn(
                          "py-3 px-4 text-sm text-left font-medium",
                          credited ? "text-green-500" : "text-foreground",
                        )}>
                          {credited ? "+" : ""}${fmtMoney(req.total_balance_increase || req.deposit_amount)}
                        </td>
                        <td className="py-3 px-4 text-left">
                          <div className="flex items-center gap-2">
                            <Badge variant="outline" className={cn("font-normal", st.className)}>
                              {st.label}
                            </Badge>
                            {isInvoice
                              && !credited
                              && req.status !== "rejected"
                              && req.status !== "cancelled"
                              && req.provider_status !== "error" && (
                                <Button
                                  type="button"
                                  variant="outline"
                                  size="sm"
                                  className="h-8 whitespace-nowrap border-border"
                                  onClick={() => openInvoiceFromHistory(req)}
                                >
                                  <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                                  {t(req.payment_channel === "cryptomus_invoice" ? "balance.cryptomus.view" : "balance.passimpay.view")}
                                </Button>
                              )}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
              );
            })()}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
