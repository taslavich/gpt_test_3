import { describe, expect, it } from "vitest";
import { getPaymentCurrency, PAYMENT_METHODS } from "@/lib/paymentMethods";

describe("payment methods", () => {
  it("contains the configured wallets and currencies", () => {
    expect(PAYMENT_METHODS).toEqual([
      expect.objectContaining({
        id: "usdc_erc20",
        address: "0xaE8c308b5dE66E9D4EC32297893bEF0053De4527",
        currency: "usdc",
      }),
      expect.objectContaining({
        id: "usdt_trc20",
        address: "TMcMNrGaEmTPujLnVMZubfKiSQydC5kTfj",
        currency: "usdt",
      }),
      expect.objectContaining({
        id: "usdt_erc20",
        address: "0xaE8c308b5dE66E9D4EC32297893bEF0053De4527",
        currency: "usdt",
      }),
    ]);
  });

  it("uses the currency of the selected payment method", () => {
    expect(getPaymentCurrency("usdc_erc20")).toBe("usdc");
    expect(getPaymentCurrency("usdt_trc20")).toBe("usdt");
    expect(getPaymentCurrency("usdt_erc20")).toBe("usdt");
  });
});
