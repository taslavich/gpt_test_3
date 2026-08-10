import { describe, expect, it } from "vitest";
import { mockProvider } from "@/api/mockProvider";

describe("mock payment provider contract", () => {
  it.each([
    {
      request: {
        provider: "passimpay" as const,
        deposit_amount: 100,
        currency: "USD",
        promocode_id: null,
      },
      channel: "passimpay_invoice",
      method: "passimpay",
    },
    {
      request: {
        provider: "cryptomus" as const,
        deposit_amount: 100,
        currency: "USD",
        promocode_id: null,
      },
      channel: "cryptomus_invoice",
      method: "cryptomus",
    },
  ])("derives $channel from the invoice provider", async ({ request, channel, method }) => {
    const result = await mockProvider.createTransaction(request);

    expect(result.success).toBe(true);
    expect(result.data?.payment_channel).toBe(channel);
    expect(result.data?.payment_method).toBe(method);
    expect(result.data?.payment_url).toContain("/invoice/");
  });

  it("keeps the explicit static-wallet channel and has no payment URL", async () => {
    const result = await mockProvider.createTransaction({
      payment_channel: "static_wallet",
      payment_method: "usdt_trc20",
      deposit_amount: 100,
      currency: "USD",
      promocode_id: null,
    });

    expect(result.success).toBe(true);
    expect(result.data?.payment_channel).toBe("static_wallet");
    expect(result.data?.payment_method).toBe("usdt_trc20");
    expect(result.data?.payment_url).toBeNull();
  });
});
