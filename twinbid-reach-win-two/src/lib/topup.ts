import type { ApiCreateTransactionRequest, ApiPromocode, ApiUserTransaction, PaymentChannel } from "@/api/types";

export const PASSIMPAY_FEE_PERCENT = 1;
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
    payment_channel: "passimpay_invoice",
    deposit_amount: params.depositAmount,
    currency: "USD",
    promocode_id: params.promoCode || null,
  };
}

export function getTransactionChannel(transaction: Pick<ApiUserTransaction, "payment_channel">): PaymentChannel {
  return transaction.payment_channel === "passimpay_invoice" ? "passimpay_invoice" : "static_wallet";
}

export function isTransactionCredited(
  transaction: Pick<ApiUserTransaction, "status" | "credited_at">,
): boolean {
  return transaction.status === "approved" && transaction.credited_at != null;
}

export function isPassimPayPartial(
  transaction: Pick<ApiUserTransaction, "status" | "credited_at" | "amount_paid">,
): boolean {
  return Number(transaction.amount_paid) > 0 && !isTransactionCredited(transaction);
}
