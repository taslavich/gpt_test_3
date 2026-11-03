import { describe, expect, it, vi } from "vitest";
import type { ApiUserTransaction } from "@/api/types";
import {
  openFreshInvoicePayment,
  type InvoicePaymentWindow,
} from "@/lib/topup";

const transaction = (overrides: Partial<ApiUserTransaction> = {}): ApiUserTransaction => ({
  id: "row-id",
  user_id: "user-id",
  transaction_id: "public-order-id",
  payment_channel: "passimpay_invoice",
  payment_method: "passimpay",
  promocode_id: null,
  deposit_amount: 100,
  status: "pending",
  currency: "USD",
  payment_url: "https://fresh.example/payment",
  provider_status: "waiting",
  credited_at: null,
  ...overrides,
});

function paymentWindow() {
  const close = vi.fn();
  const windowHandle: InvoicePaymentWindow = {
    closed: false,
    opener: {},
    location: { href: "" },
    close,
  };
  return { windowHandle, close };
}

describe("fresh invoice payment URL", () => {
  it("checks the backend row id and opens only the fresh URL", async () => {
    const { windowHandle } = paymentWindow();
    const openWindow = vi.fn(() => windowHandle);
    const getTransaction = vi.fn(async () => transaction());
    const onFreshTransaction = vi.fn();

    const result = await openFreshInvoicePayment({
      transactionRowId: "row-id",
      getTransaction,
      openWindow,
      onFreshTransaction,
    });

    expect(openWindow.mock.invocationCallOrder[0]).toBeLessThan(getTransaction.mock.invocationCallOrder[0]);
    expect(getTransaction).toHaveBeenCalledWith("row-id");
    expect(onFreshTransaction).toHaveBeenCalledWith(expect.objectContaining({ id: "row-id" }));
    expect(windowHandle.opener).toBeNull();
    expect(windowHandle.location.href).toBe("https://fresh.example/payment");
    expect(result.opened).toBe(true);
  });

  it.each([
    ["cancelled", transaction({ status: "cancelled", provider_status: "expired", payment_url: null })],
    ["rejected", transaction({ status: "rejected" })],
    ["provider error", transaction({ provider_status: "error" })],
    ["credited", transaction({ status: "approved", credited_at: "2026-08-10T12:00:00Z" })],
  ])("does not open a %s invoice", async (_name, freshTransaction) => {
    const { windowHandle, close } = paymentWindow();

    const result = await openFreshInvoicePayment({
      transactionRowId: "row-id",
      getTransaction: vi.fn(async () => freshTransaction),
      openWindow: () => windowHandle,
      onFreshTransaction: vi.fn(),
    });

    expect(result.opened).toBe(false);
    expect(windowHandle.location.href).toBe("");
    expect(close).toHaveBeenCalledOnce();
  });

  it("closes the blank window and never uses a cached URL when GET fails", async () => {
    const { windowHandle, close } = paymentWindow();
    const getTransaction = vi.fn(async () => {
      throw new Error("GET failed");
    });

    await expect(openFreshInvoicePayment({
      transactionRowId: "row-id",
      getTransaction,
      openWindow: () => windowHandle,
      onFreshTransaction: vi.fn(),
    })).rejects.toThrow("GET failed");

    expect(windowHandle.location.href).toBe("");
    expect(close).toHaveBeenCalledOnce();
  });
});
