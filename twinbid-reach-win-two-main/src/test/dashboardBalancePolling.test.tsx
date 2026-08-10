import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ApiUserTransaction } from "@/api/types";

const mocks = vi.hoisted(() => ({
  listTransactions: vi.fn(),
  registerRefreshHandler: vi.fn(),
  user: { id: "user-id" },
  profile: { balance: 0 },
  translate: (key: string) => key,
}));

vi.mock("@/api", () => ({
  api: {
    listTransactions: mocks.listTransactions,
    getPromocode: vi.fn(),
    createTransaction: vi.fn(),
  },
  ApiError: class ApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => ({ t: mocks.translate }),
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ user: mocks.user }),
}));

vi.mock("@/contexts/ProfileContext", () => ({
  useProfile: () => ({ profile: mocks.profile, loading: false }),
}));

vi.mock("@/contexts/PendingPaymentContext", () => ({
  usePendingPayment: () => ({
    pendingPayment: null,
    openPayment: vi.fn(),
    registerRefreshHandler: mocks.registerRefreshHandler,
  }),
}));

vi.mock("@/integrations/supabase/client", () => ({
  supabase: { from: vi.fn() },
}));

vi.mock("@/lib/yandexMetrikaTopup", () => ({
  rememberTopupForGoal: vi.fn(),
  trackBalanceTopupSuccess: vi.fn(),
}));

import DashboardBalance from "@/pages/DashboardBalance";

const invoice = (overrides: Partial<ApiUserTransaction> = {}): ApiUserTransaction => ({
  id: "row-id",
  user_id: "user-id",
  transaction_id: "public-order-id",
  payment_channel: "passimpay_invoice",
  payment_method: "passimpay",
  promocode_id: null,
  deposit_amount: 100,
  status: "pending",
  currency: "USD",
  payment_url: "https://pay.example/invoice",
  provider_status: "waiting",
  credited_at: null,
  created_at: "2026-08-10T12:00:00Z",
  ...overrides,
});

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
}

describe("DashboardBalance transaction polling", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    mocks.listTransactions.mockReset();
    mocks.registerRefreshHandler.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("keeps the last invoice after a polling error and updates it on the next successful poll", async () => {
    mocks.listTransactions.mockResolvedValueOnce({ items: [invoice()], total: 1 });

    await act(async () => {
      render(<DashboardBalance />);
      await flush();
    });
    expect(screen.getByText(/PassimPay/)).toBeInTheDocument();
    expect(mocks.listTransactions).toHaveBeenCalledTimes(1);

    mocks.listTransactions.mockRejectedValueOnce(new Error("temporary network error"));
    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await flush();
    });

    expect(mocks.listTransactions).toHaveBeenCalledTimes(2);
    expect(screen.getByText(/PassimPay/)).toBeInTheDocument();

    mocks.listTransactions.mockResolvedValueOnce({
      items: [invoice({ status: "cancelled", provider_status: "expired", payment_url: null })],
      total: 1,
    });
    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await flush();
    });

    expect(mocks.listTransactions).toHaveBeenCalledTimes(3);
    expect(screen.getByText("balance.cancelled")).toBeInTheDocument();

    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await flush();
    });
    expect(mocks.listTransactions).toHaveBeenCalledTimes(3);
  });
});
