import { describe, expect, it } from "vitest";
import type { ApiPromocode, ApiUserTransaction } from "@/api/types";
import {
  buildPassimPayTopup,
  buildStaticWalletTopup,
  getPassimPayFee,
  getTransactionBonusAmount,
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
      payment_channel: "passimpay_invoice",
      deposit_amount: 100.25,
      currency: "USD",
      promocode_id: "BONUS25",
    });
  });

  it("keeps the existing static-wallet method and network currency", () => {
    expect(buildStaticWalletTopup({
      paymentMethod: "usdt_trc20",
      depositAmount: 250,
      promoCode: "BONUS25",
    })).toEqual({
      payment_channel: "static_wallet",
      payment_method: "usdt_trc20",
      deposit_amount: 250,
      currency: "usdt",
      promocode_id: "BONUS25",
      status: "draft",
    });
  });

  it("accepts at most two decimal places and calculates the fee from the deposit only", () => {
    expect(parseTopupAmount("100,25")).toBe(100.25);
    expect(parseTopupAmount("100.259")).toBeNull();
    expect(getPassimPayFee(100.25)).toBe(1);
  });

  it("uses the backend total as the source of truth for the promo bonus", () => {
    expect(getTransactionBonusAmount(transaction())).toBe(25);
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
