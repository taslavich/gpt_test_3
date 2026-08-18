import type { ApiCreateTransactionRequest, ApiPromocode, ApiUserTransaction, PaymentChannel } from "@/api/types";

export const PASSIMPAY_FEE_PERCENT = 1;
export const CRYPTOMUS_FEE_PERCENT = 2.5;
export const PENDING_INVOICE_HISTORY_REFRESH_MS = 5_000;
export const DEFAULT_HISTORY_REFRESH_MS = 5 * 60 * 1_000;
export type PromocodeValidationFailure = "expired" | "limit" | "already_used";

const roundMoney = (value: number) => Math.round((value + Number.EPSILON) * 100) / 100;

export function parseTopupAmount(value: string): number | null {
  const normalized = value.trim().replace(",", ".");
  if (!/^\d+(?:\.\d{1,2})?$/.test(normalized)) return null;
  const amount = Number(normalized);
  return Number.isFinite(amount) && amount > 0 ? amount : null;
}

export function sanitizeTopupAmountInput(value: string): string | null {
  const normalized = value.replace(",", ".");
  return /^\d*(?:\.\d{0,2})?$/.test(normalized) ? normalized : null;
}

export function getPassimPayFee(depositAmount: number): number {
  return roundMoney(depositAmount * PASSIMPAY_FEE_PERCENT / 100);
}

export function getCryptomusFee(depositAmount: number): number {
  return roundMoney(depositAmount * CRYPTOMUS_FEE_PERCENT / 100);
}

/** Amount shown to the user and charged by PassimPay: deposit + TwinBid fee. */
export function getPassimPayChargeAmount(depositAmount: number): number {
  return roundMoney(depositAmount + getPassimPayFee(depositAmount));
}

/** Amount shown to the user and charged by Cryptomus: deposit + TwinBid fee. */
export function getCryptomusChargeAmount(depositAmount: number): number {
  return roundMoney(depositAmount + getCryptomusFee(depositAmount));
}

export function validatePromocodeForTopup(params: {
  promo: ApiPromocode;
  transactions: ApiUserTransaction[];
  userId?: string;
  now?: Date;
}): PromocodeValidationFailure | null {
  const { promo, transactions, userId, now = new Date() } = params;
  if (promo.valid_to && new Date(promo.valid_to) < now) return "expired";
  if (promo.usage_limit != null && promo.usage_count >= promo.usage_limit) return "limit";
  const alreadyUsed = transactions.some(
    transaction => transaction.promocode_id === promo.id
      && (!userId || transaction.user_id === userId)
      && transaction.status !== "draft"
      && transaction.status !== "cancelled",
  );
  return alreadyUsed ? "already_used" : null;
}

export function getTransactionBonusAmount(transaction: Pick<ApiUserTransaction, "deposit_amount" | "bonus_amount" | "total_balance_increase">): number {
  const deposit = Number(transaction.deposit_amount) || 0;
  const total = Number(transaction.total_balance_increase) || 0;
  if (total >= deposit) return roundMoney(total - deposit);
  return Math.max(0, roundMoney(Number(transaction.bonus_amount) || 0));
}

export function buildStaticWalletTopup(params: {
  paymentMethod: string;
  depositAmount: number;
  promoCode?: string | null;
}): ApiCreateTransactionRequest {
  return {
    payment_channel: "static_wallet",
    payment_method: params.paymentMethod,
    deposit_amount: params.depositAmount,
    currency: "USD",
    promocode_id: params.promoCode || null,
  };
}

export function buildPassimPayTopup(params: {
  depositAmount: number;
  promoCode?: string | null;
}): ApiCreateTransactionRequest {
  return {
    provider: "passimpay",
    deposit_amount: params.depositAmount,
    currency: "USD",
    promocode_id: params.promoCode || null,
  };
}

export function buildCryptomusTopup(params: {
  depositAmount: number;
  promoCode?: string | null;
}): ApiCreateTransactionRequest {
  return {
    provider: "cryptomus",
    deposit_amount: params.depositAmount,
    currency: "USD",
    promocode_id: params.promoCode || null,
  };
}

export function isInvoicePaymentChannel(channel?: PaymentChannel | null): boolean {
  return channel === "passimpay_invoice" || channel === "cryptomus_invoice";
}

export function getInvoiceChargeAmount(channel: PaymentChannel | undefined, depositAmount: number): number {
  if (channel === "passimpay_invoice") return getPassimPayChargeAmount(depositAmount);
  if (channel === "cryptomus_invoice") return getCryptomusChargeAmount(depositAmount);
  return roundMoney(depositAmount);
}

export function getInvoiceFee(channel: PaymentChannel | undefined, depositAmount: number): number {
  if (channel === "passimpay_invoice") return getPassimPayFee(depositAmount);
  if (channel === "cryptomus_invoice") return getCryptomusFee(depositAmount);
  return 0;
}

export function getTransactionChannel(transaction: Pick<ApiUserTransaction, "payment_channel">): PaymentChannel {
  if (transaction.payment_channel === "passimpay_invoice") return "passimpay_invoice";
  if (transaction.payment_channel === "cryptomus_invoice") return "cryptomus_invoice";
  return "static_wallet";
}

export function isUnfinishedStaticWalletTransaction(
  transaction: Pick<ApiUserTransaction, "payment_channel" | "status">,
): boolean {
  return transaction.payment_channel === "static_wallet"
    && (transaction.status === "draft" || transaction.status === "pending");
}

export function isTransactionCredited(
  transaction: Pick<ApiUserTransaction, "status" | "credited_at">,
): boolean {
  return transaction.status === "approved" && transaction.credited_at != null;
}

export function isPendingInvoiceTransaction(
  transaction: Pick<ApiUserTransaction, "payment_channel" | "status" | "credited_at" | "provider_status">,
): boolean {
  return isInvoicePaymentChannel(transaction.payment_channel)
    && transaction.status !== "cancelled"
    && transaction.status !== "rejected"
    && transaction.provider_status !== "error"
    && !isTransactionCredited(transaction);
}

export function getTransactionHistoryRefreshInterval(
  transactions: Array<Pick<ApiUserTransaction, "payment_channel" | "status" | "credited_at" | "provider_status">>,
): number {
  return transactions.some(isPendingInvoiceTransaction)
    ? PENDING_INVOICE_HISTORY_REFRESH_MS
    : DEFAULT_HISTORY_REFRESH_MS;
}

export function canOpenFreshInvoicePayment(
  transaction: Pick<ApiUserTransaction, "status" | "credited_at" | "provider_status" | "payment_url">,
): transaction is typeof transaction & { payment_url: string } {
  return transaction.status !== "cancelled"
    && transaction.status !== "rejected"
    && transaction.provider_status !== "error"
    && !isTransactionCredited(transaction)
    && typeof transaction.payment_url === "string"
    && transaction.payment_url.length > 0;
}

export interface InvoicePaymentWindow {
  closed: boolean;
  opener: unknown;
  location: { href: string };
  close: () => void;
}

export async function openFreshInvoicePayment(params: {
  transactionRowId: string;
  getTransaction: (transactionRowId: string) => Promise<ApiUserTransaction>;
  openWindow: () => InvoicePaymentWindow | null;
  onFreshTransaction: (transaction: ApiUserTransaction) => void;
}): Promise<{ transaction: ApiUserTransaction; opened: boolean }> {
  let paymentWindow: InvoicePaymentWindow | null = null;
  try {
    paymentWindow = params.openWindow();
    if (paymentWindow) paymentWindow.opener = null;
  } catch {
    paymentWindow = null;
  }

  try {
    const transaction = await params.getTransaction(params.transactionRowId);
    params.onFreshTransaction(transaction);

    if (!canOpenFreshInvoicePayment(transaction) || !paymentWindow || paymentWindow.closed) {
      if (paymentWindow && !paymentWindow.closed) paymentWindow.close();
      return { transaction, opened: false };
    }

    paymentWindow.location.href = transaction.payment_url;
    return { transaction, opened: true };
  } catch (error) {
    if (paymentWindow && !paymentWindow.closed) paymentWindow.close();
    throw error;
  }
}

export function isInvoicePartial(
  transaction: Pick<ApiUserTransaction, "status" | "credited_at" | "amount_paid">,
): boolean {
  return Number(transaction.amount_paid) > 0 && !isTransactionCredited(transaction);
}
