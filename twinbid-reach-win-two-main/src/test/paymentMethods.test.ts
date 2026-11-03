import { describe, expect, it } from "vitest";
import { getPaymentCurrency, PAYMENT_METHODS } from "@/lib/paymentMethods";

describe("payment methods", () => {
  it("contains the configured wallets and currencies", () => {
    expect(PAYMENT_METHODS).toEqual([
      expect.objectContaining({
        id: "usdc_erc20",
        address: "0xED961471A377a998df31191A7277006Aa0b04186",
        currency: "usdc",
      }),
      expect.objectContaining({
        id: "usdt_trc20",
        address: "TJr26CGefYQAQ5ryrxETrQPeLYdzmz52ad",
        currency: "usdt",
      }),
      expect.objectContaining({
        id: "usdt_erc20",
        address: "0xED961471A377a998df31191A7277006Aa0b04186",
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
