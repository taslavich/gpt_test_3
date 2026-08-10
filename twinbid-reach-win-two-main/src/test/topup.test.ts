import { describe, expect, it } from "vitest";
import type { ApiPromocode, ApiUserTransaction } from "@/api/types";
import {
  buildCryptomusTopup,
  buildPassimPayTopup,
  buildStaticWalletTopup,
  DEFAULT_HISTORY_REFRESH_MS,
  getInvoiceChargeAmount,
  getPassimPayChargeAmount,
  getPassimPayFee,
  getTransactionHistoryRefreshInterval,
  getTransactionBonusAmount,
  getTransactionChannel,
  isInvoicePaymentChannel,
  isInvoicePartial,
  PENDING_INVOICE_HISTORY_REFRESH_MS,
  isTransactionCredited,
  isUnfinishedStaticWalletTransaction,
  parseTopupAmount,
  validatePromocodeForTopup,
} from "@/lib/topup";

const promo: ApiPromocode = {
  id: "promo-id",
  promocode_text: "BONUS25",
  bonus_percent: 25,
  usage_count: 0,
  usage_limit: 10,
  valid_from: null,
  valid_to: "2026-12-31T23:59:59Z",
};

const transaction = (overrides: Partial<ApiUserTransaction> = {}): ApiUserTransaction => ({
  id: "transaction-id",
  user_id: "user-id",
  transaction_time: "2026-08-05T00:00:00Z",
  transaction_id: "public-id",
  payment_channel: "passimpay_invoice",
  payment_method: "passimpay",
  bonus_amount: 25,
  promocode_id: promo.id,
  transaction_hash: null,
  deposit_amount: 100,
  total_balance_increase: 125,
  status: "pending",
  currency: "USD",
  created_at: "2026-08-05T00:00:00Z",
  updated_at: "2026-08-05T00:00:00Z",
  ...overrides,
});

describe("top-up request contract", () => {
  it("keeps the entered PassimPay amount separate from the promo bonus", () => {
    expect(buildPassimPayTopup({ depositAmount: 100.25, promoCode: "BONUS25" })).toEqual({
      provider: "passimpay",
      deposit_amount: 100.25,
      currency: "USD",
      promocode_id: "BONUS25",
    });
  });

  it("creates a zero-fee Cryptomus invoice with the same promo contract", () => {
    expect(buildCryptomusTopup({ depositAmount: 100.25, promoCode: "BONUS25" })).toEqual({
      provider: "cryptomus",
      deposit_amount: 100.25,
      currency: "USD",
      promocode_id: "BONUS25",
    });
    expect(getInvoiceChargeAmount("cryptomus_invoice", 100.25)).toBe(100.25);
    expect(isInvoicePaymentChannel("cryptomus_invoice")).toBe(true);
    expect(getTransactionChannel(transaction({ payment_channel: "cryptomus_invoice" }))).toBe("cryptomus_invoice");
  });

  it("sends the minimal static-wallet payload required by the backend", () => {
    expect(buildStaticWalletTopup({
      paymentMethod: "usdt_trc20",
      depositAmount: 250,
      promoCode: "BONUS25",
    })).toEqual({
      payment_channel: "static_wallet",
      payment_method: "usdt_trc20",
      deposit_amount: 250,
      currency: "USD",
      promocode_id: "BONUS25",
    });
  });

  it("accepts at most two decimal places and calculates the fee from the deposit only", () => {
    expect(parseTopupAmount("100,25")).toBe(100.25);
    expect(parseTopupAmount("100.259")).toBeNull();
    expect(getPassimPayFee(100.25)).toBe(1);
    expect(getPassimPayChargeAmount(100)).toBe(101);
    expect(getPassimPayChargeAmount(100.25)).toBe(101.25);
  });

  it("uses the backend total as the source of truth for the promo bonus", () => {
    expect(getTransactionBonusAmount(transaction())).toBe(25);
  });

  it("treats a payment as credited only after approved plus credited_at", () => {
    expect(isTransactionCredited(transaction({ status: "approved", credited_at: null }))).toBe(false);
    expect(isTransactionCredited(transaction({
      status: "approved",
      credited_at: "2026-08-05T00:05:00Z",
    }))).toBe(true);
  });

  it("detects partial PassimPay payments without marking them credited", () => {
    expect(isInvoicePartial(transaction({ amount_paid: 45 }))).toBe(true);
    expect(isInvoicePartial(transaction({
      status: "approved",
      amount_paid: 100,
      credited_at: "2026-08-05T00:05:00Z",
    }))).toBe(false);
  });

  it("blocks a new static-wallet payment only for unfinished static-wallet transactions", () => {
    expect(isUnfinishedStaticWalletTransaction(transaction({
      payment_channel: "static_wallet",
      status: "draft",
    }))).toBe(true);
    expect(isUnfinishedStaticWalletTransaction(transaction({
      payment_channel: "static_wallet",
      status: "pending",
    }))).toBe(true);
    expect(isUnfinishedStaticWalletTransaction(transaction({
      payment_channel: "passimpay_invoice",
      status: "pending",
    }))).toBe(false);
    expect(isUnfinishedStaticWalletTransaction(transaction({
      payment_channel: "cryptomus_invoice",
      status: "pending",
    }))).toBe(false);
    expect(isUnfinishedStaticWalletTransaction(transaction({
      payment_channel: "another_provider" as ApiUserTransaction["payment_channel"],
      status: "pending",
    }))).toBe(false);
    expect(isUnfinishedStaticWalletTransaction(transaction({
      payment_channel: "static_wallet",
      status: "approved",
    }))).toBe(false);
  });

  it.each(["passimpay_invoice", "cryptomus_invoice"] as const)(
    "refreshes history every 5 seconds for an unfinished %s payment",
    paymentChannel => {
      expect(getTransactionHistoryRefreshInterval([
        transaction({ payment_channel: paymentChannel, status: "pending", credited_at: null }),
      ])).toBe(PENDING_INVOICE_HISTORY_REFRESH_MS);
    },
  );

  it("refreshes history every 5 minutes when no unfinished invoice exists", () => {
    expect(getTransactionHistoryRefreshInterval([])).toBe(DEFAULT_HISTORY_REFRESH_MS);
    expect(getTransactionHistoryRefreshInterval([
      transaction({
        payment_channel: "passimpay_invoice",
        status: "approved",
        credited_at: "2026-08-05T00:05:00Z",
      }),
      transaction({ payment_channel: "cryptomus_invoice", status: "cancelled" }),
    ])).toBe(DEFAULT_HISTORY_REFRESH_MS);
  });

  it("does not enable 5-second history polling for static-wallet payments", () => {
    expect(getTransactionHistoryRefreshInterval([
      transaction({ payment_channel: "static_wallet", status: "pending", credited_at: null }),
    ])).toBe(DEFAULT_HISTORY_REFRESH_MS);
  });
});

describe("front-end promocode availability validation", () => {
  it("allows the same validated promocode for PassimPay when it has not been used", () => {
    expect(validatePromocodeForTopup({
      promo,
      transactions: [],
      userId: "user-id",
      now: new Date("2026-08-05T00:00:00Z"),
    })).toBeNull();
  });

  it("keeps the existing expiry and usage-limit checks", () => {
    expect(validatePromocodeForTopup({
      promo: { ...promo, valid_to: "2026-08-04T00:00:00Z" },
      transactions: [],
      now: new Date("2026-08-05T00:00:00Z"),
    })).toBe("expired");
    expect(validatePromocodeForTopup({
      promo: { ...promo, usage_count: 10 },
      transactions: [],
    })).toBe("limit");
  });

  it("rejects a previously used promo but ignores abandoned drafts and cancelled attempts", () => {
    expect(validatePromocodeForTopup({
      promo,
      transactions: [transaction()],
      userId: "user-id",
    })).toBe("already_used");
    expect(validatePromocodeForTopup({
      promo,
      transactions: [transaction({ status: "draft" }), transaction({ id: "cancelled", status: "cancelled" })],
      userId: "user-id",
    })).toBeNull();
  });
});
