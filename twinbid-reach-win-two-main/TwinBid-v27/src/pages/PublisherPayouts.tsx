import { useState } from "react";
import { Wallet, Send } from "lucide-react";
import { toast } from "sonner";
import { publisherApi, type PayoutMethod } from "@/api/publisher";
import { ApiError } from "@/api/http";
import {
  usePublisher,
  usePublisherResource,
} from "@/contexts/PublisherContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { useIsMobile } from "@/hooks/use-mobile";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import {
  comparePublisherMoney,
  formatPublisherMoney,
  normalizePublisherAmount,
  publisherDate,
} from "@/lib/publisherData";
import { PAYOUT_METHODS } from "@/lib/publisherPreferences";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  PublisherAsync,
  PublisherEmpty,
  PublisherError,
  PublisherBalanceCards,
  PublisherPagination,
} from "@/components/publisher/PublisherApiState";
export default function PublisherPayouts() {
  const isMobile = useIsMobile();
  const { lang } = useLanguage(),
    { createPayout, payoutIntent, payoutBusy } = usePublisher();
  const [offset, setOffset] = useState(0),
    [open, setOpen] = useState(false),
    [amount, setAmount] = useState(""),
    [method, setMethod] = useState<PayoutMethod>("usdt_trc20"),
    [wallet, setWallet] = useState(""),
    [source, setSource] = useState<"manual" | "saved">("manual"),
    [error, setError] = useState<unknown>(null);
  const page = { limit: 50, offset },
    balance = usePublisherResource(["balance"], publisherApi.balance),
    history = usePublisherResource(["payouts", page], (signal) =>
      publisherApi.payouts(page, signal),
    ),
    settings = usePublisherResource(["settings"], publisherApi.settings);
  const begin = () => {
    if (payoutIntent) {
      setAmount(payoutIntent.amount);
      setMethod(payoutIntent.method);
      setWallet(payoutIntent.wallet);
      setSource("manual");
    } else {
      setAmount("");
      setSource(settings.data?.wallet ? "saved" : "manual");
      setMethod(settings.data?.payoutMethod || "usdt_trc20");
      setWallet("");
      setError(null);
    }
    setOpen(true);
  };
  const submit = async () => {
    if (payoutBusy) return;
    try {
      const payload = payoutIntent || {
        amount: normalizePublisherAmount(amount),
        method: source === "saved" ? settings.data!.payoutMethod : method,
        wallet: (source === "saved" ? settings.data!.wallet : wallet).trim(),
      };
      // Existing backend policy: internal/publisher/finance.go. Never invent a UI fee.
      if (
        !payoutIntent &&
        (comparePublisherMoney(payload.amount, "50") < 0 ||
          !balance.data ||
          comparePublisherMoney(
            payload.amount,
            balance.data.available_for_payout,
          ) > 0)
      )
        throw new ApiError(
          400,
          tr(
            lang,
            "Минимум $50 в пределах доступного баланса",
            "Minimum $50 within the available balance",
            "最低 50 美元，且不得超过可用余额",
          ),
        );
      if (
        !payload.wallet ||
        !PAYOUT_METHODS.some((item) => item.value === payload.method)
      )
        throw new ApiError(
          400,
          tr(
            lang,
            "Выберите сеть и укажите адрес кошелька",
            "Select a network and enter a wallet address",
            "请选择网络并输入钱包地址",
          ),
        );
      setError(null);
      await createPayout(payload);
      setOpen(false);
      setAmount("");
      setOffset(0);
      toast.success(
        tr(lang, "Заявка создана", "Payout requested", "提现申请已创建"),
      );
    } catch (err) {
      setError(err);
    }
  };
  return (
    <div className="mx-auto max-w-[1440px] space-y-6">
      <div>
        <h1 className="text-2xl font-bold">
          {tr(lang, "Выплаты", "Payouts", "提现")}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {tr(
            lang,
            "Баланс и история выплат.",
            "Your balance and payout history.",
            "您的余额和提现记录。",
          )}
        </p>
      </div>
      <PublisherAsync
        loading={balance.isPending}
        error={balance.error}
        retry={balance.refetch}
      >
        {balance.data && <PublisherBalanceCards balance={balance.data} />}
      </PublisherAsync>
      <Card className="flex flex-wrap items-center justify-between gap-4 p-6">
        <div className="flex items-center gap-3">
          <Wallet className="h-6 w-6 text-primary" />
          <div>
            <p className="font-medium">USDT TRC20 / ERC20</p>
            <p className="mt-1 text-xs text-muted-foreground">
              {tr(
                lang,
                "Минимум $50. Отменить заявку может только оператор.",
                "Minimum $50. Only an operator can cancel a request.",
                "最低 50 美元，只有操作员可以取消申请。",
              )}
            </p>
          </div>
        </div>
        <Button
          disabled={
            payoutBusy ||
            (!payoutIntent &&
              (!balance.data ||
                !!balance.error ||
                comparePublisherMoney(balance.data.available_for_payout, "50") <
                  0))
          }
          onClick={begin}
        >
          <Send className="mr-2 h-4 w-4" />
          {payoutIntent
            ? tr(
                lang,
                "Повторить прежнюю заявку",
                "Retry the same request",
                "重试同一申请",
              )
            : tr(lang, "Запросить выплату", "Request payout", "申请提现")}
        </Button>
      </Card>
      {settings.error && (
        <PublisherError error={settings.error} retry={settings.refetch} />
      )}
      <PublisherAsync
        loading={history.isPending}
        error={history.error}
        retry={history.refetch}
      >
        {history.data && (
          <>
            <Card className="overflow-hidden">
              <div className="border-b border-border p-4">
                <h2 className="font-semibold">
                  {tr(lang, "История выплат", "Payout history", "提现记录")}
                </h2>
              </div>
              {!history.data.items.length ? (
                <PublisherEmpty>
                  {tr(
                    lang,
                    "Заявок на этой странице пока нет",
                    "No requests on this page yet",
                    "本页暂无申请",
                  )}
                </PublisherEmpty>
              ) : isMobile ? (
                <div className="divide-y divide-border">
                  {history.data.items.map((payout) => (
                    <div key={payout.id} className="min-w-0 space-y-3 p-4 text-sm">
                      <div className="flex items-start justify-between gap-3"><div><p className="text-lg font-semibold">${formatPublisherMoney(payout.amount, lang)}</p><p className="mt-1 text-xs text-muted-foreground">{publisherDate(payout.created_at, lang)}</p></div><Badge variant={payout.status === "paid" ? "default" : payout.status === "canceled" ? "destructive" : "secondary"}>{payout.status === "paid" ? tr(lang, "Выплачена", "Paid", "已支付") : payout.status === "pending" ? tr(lang, "Ожидает выплаты", "Pending payout", "等待付款") : tr(lang, "Отменена", "Canceled", "已取消")}</Badge></div>
                      <div><p>{PAYOUT_METHODS.find((item) => item.value === payout.method)?.label || payout.method}</p><p className="mt-1 break-all font-mono text-xs text-muted-foreground">{payout.wallet}</p></div>
                      <p className="break-all text-xs text-muted-foreground">ID: {payout.id}</p>
                      {payout.transfer_reference && <p className="break-all text-xs text-muted-foreground">{tr(lang, "Подтверждение перевода", "Transfer reference", "转账凭证")}: {payout.transfer_reference}</p>}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="max-w-full overflow-x-auto overscroll-x-contain">
                  <table className="w-full min-w-[850px] text-sm">
                    <thead className="bg-muted/40 text-xs text-muted-foreground">
                      <tr>
                        {[
                          "ID / UTC",
                          tr(lang, "Сумма, USD", "Amount, USD", "金额（美元）"),
                          tr(
                            lang,
                            "Сеть / кошелёк",
                            "Network / wallet",
                            "网络 / 钱包",
                          ),
                          tr(lang, "Статус", "Status", "状态"),
                          tr(
                            lang,
                            "Подтверждение перевода",
                            "Transfer reference",
                            "转账凭证",
                          ),
                        ].map((label) => (
                          <th className="px-4 py-3 text-left" key={label}>
                            {label}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {history.data.items.map((payout) => (
                        <tr key={payout.id} className="border-t border-border">
                          <td className="px-4 py-3">
                            <p className="font-mono text-xs">{payout.id}</p>
                            <p className="text-xs text-muted-foreground">
                              {publisherDate(payout.created_at, lang)}
                            </p>
                          </td>
                          <td className="px-4 py-3">
                            ${formatPublisherMoney(payout.amount, lang)}
                          </td>
                          <td className="px-4 py-3">
                            <p>
                              {PAYOUT_METHODS.find(
                                (item) => item.value === payout.method,
                              )?.label || payout.method}
                            </p>
                            <p className="max-w-[260px] break-all font-mono text-xs text-muted-foreground">
                              {payout.wallet}
                            </p>
                          </td>
                          <td className="px-4 py-3">
                            <Badge
                              variant={
                                payout.status === "paid"
                                  ? "default"
                                  : payout.status === "canceled"
                                    ? "destructive"
                                    : "secondary"
                              }
                            >
                              {payout.status === "paid"
                                ? tr(lang, "Выплачена", "Paid", "已支付")
                                : payout.status === "pending"
                                  ? tr(
                                      lang,
                                      "Ожидает выплаты",
                                      "Pending payout",
                                      "等待付款",
                                    )
                                  : tr(lang, "Отменена", "Canceled", "已取消")}
                            </Badge>
                          </td>
                          <td className="break-all px-4 py-3 font-mono text-xs">
                            {payout.transfer_reference || "—"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </Card>
            <PublisherPagination
              {...history.data}
              count={history.data.items.length}
              onChange={setOffset}
            />
          </>
        )}
      </PublisherAsync>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!payoutBusy) setOpen(value);
        }}
      >
        <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-[480px]">
          <DialogHeader>
            <DialogTitle>
              {tr(lang, "Заявка на выплату", "Payout request", "提现申请")}
            </DialogTitle>
            <DialogDescription>
              {tr(
                lang,
                "Укажите сумму в USD и кошелёк. Реквизиты сохранятся в заявке.",
                "Enter an amount in USD and a wallet. These details are saved with the request.",
                "请输入美元金额和钱包地址，收款信息将保存在申请中。",
              )}
            </DialogDescription>
          </DialogHeader>
          {payoutIntent && !payoutBusy && (
            <p className="text-sm text-amber-500">
              {tr(
                lang,
                "Предыдущая заявка ещё не подтверждена. При повторе будут использованы те же реквизиты.",
                "The previous request is not yet confirmed. Retrying uses the same payout details.",
                "上次申请尚未确认。重试时将使用相同的收款信息。",
              )}
            </p>
          )}
          <fieldset
            disabled={payoutBusy || !!payoutIntent}
            className="space-y-4"
          >
            <div className="space-y-2">
              <Label htmlFor="payout-amount">
                {tr(lang, "Сумма, USD", "Amount, USD", "金额（美元）")}
              </Label>
              <Input
                id="payout-amount"
                inputMode="decimal"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
              />
            </div>
            {settings.data?.wallet && (
              <label className="flex gap-2 rounded-lg border border-border p-3 text-sm">
                <input
                  type="radio"
                  name="payout-source"
                  checked={source === "saved"}
                  onChange={() => setSource("saved")}
                />
                <span>
                  {tr(
                    lang,
                    "Использовать сохранённые реквизиты",
                    "Use saved details",
                    "使用已保存的收款信息",
                  )}
                  <span className="mt-1 block break-all text-xs text-muted-foreground">
                    {settings.data.payoutMethod} · {settings.data.wallet}
                  </span>
                </span>
              </label>
            )}
            <label className="flex gap-2 text-sm">
              <input
                type="radio"
                name="payout-source"
                checked={source === "manual"}
                onChange={() => setSource("manual")}
              />
              {tr(
                lang,
                "Указать для этой заявки",
                "Enter for this request",
                "为本次申请填写",
              )}
            </label>
            {source === "manual" && (
              <>
                <div className="space-y-2">
                  <Label htmlFor="payout-network">
                    {tr(lang, "Сеть", "Network", "网络")}
                  </Label>
                  <Select
                    value={method}
                    onValueChange={(value) => setMethod(value as PayoutMethod)}
                  >
                    <SelectTrigger id="payout-network">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {PAYOUT_METHODS.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="payout-wallet">
                    {tr(lang, "Адрес кошелька", "Wallet address", "钱包地址")}
                  </Label>
                  <Input
                    id="payout-wallet"
                    value={wallet}
                    onChange={(e) => setWallet(e.target.value)}
                  />
                </div>
              </>
            )}
          </fieldset>
          {error && <PublisherError error={error} />}
          <DialogFooter>
            <Button
              variant="ghost"
              disabled={payoutBusy}
              onClick={() => setOpen(false)}
            >
              {tr(lang, "Назад", "Back", "返回")}
            </Button>
            <Button disabled={payoutBusy} onClick={() => void submit()}>
              {payoutIntent
                ? tr(
                    lang,
                    "Повторить ту же заявку",
                    "Retry the same request",
                    "重试同一申请",
                  )
                : tr(lang, "Создать заявку", "Create request", "创建申请")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
