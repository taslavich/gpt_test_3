import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import PublisherPayouts from "@/pages/PublisherPayouts";
import PublisherOverview from "@/pages/PublisherOverview";
import { usePublisher } from "@/contexts/PublisherContext";
import {
  PublisherTestWrapper,
  publisherServer,
  userFixture,
  languageFixture,
  settingsFixture,
  balanceFixture,
  payoutFixture,
  reply,
} from "./publisher-fixtures";
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ user: userFixture, signOut: vi.fn() }),
}));
vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => languageFixture,
}));
beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function Refresh() {
  const { refreshPublisher } = usePublisher();
  return <button onClick={() => void refreshPublisher()}>refresh-test</button>;
}
describe("Backend-authoritative balance and payouts", () => {
  it("displays separate ledger values and full overview totals instead of calculating money from zones", async () => {
    publisherServer();
    render(<PublisherOverview />, { wrapper: PublisherTestWrapper });
    await screen.findByText(/9.*876.*543.*210,12345678/);
    expect(screen.getAllByText("$200,12345678")).toHaveLength(2);
    expect(screen.getByText(/\$98\s765,12345678/)).toBeInTheDocument();
    expect(screen.getByText("$0,00")).toBeInTheDocument();
    expect(screen.getByText("$40,00")).toBeInTheDocument();
    expect(screen.queryByText("Доход синхронизирован")).not.toBeInTheDocument();
  });
  it("retains a real UUID after a lost response and retries the same payload even after funds were reserved", async () => {
    const s = publisherServer();
    let attempts = 0;
    s.overrides.set("POST /api/publisher/payouts", () => {
      if (++attempts === 1) {
        s.setBalance({
          ...balanceFixture,
          reserved: "200.12345678",
          available_for_payout: "0",
        });
        throw new TypeError("response lost after commit");
      }
      return reply(payoutFixture);
    });
    s.overrides.set("GET /api/publisher/payouts", () =>
      reply({ items: attempts ? [payoutFixture] : [], limit: 50, offset: 0 }),
    );
    render(
      <>
        <PublisherPayouts />
        <Refresh />
      </>,
      { wrapper: PublisherTestWrapper },
    );
    await screen.findAllByText("$200,12345678");
    fireEvent.click(screen.getByRole("button", { name: "Запросить выплату" }));
    fireEvent.change(screen.getByLabelText("Сумма, USD"), {
      target: { value: "50" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Создать заявку" }));
    await screen.findByText(/Предыдущая заявка ещё не подтверждена/);
    const first = s.calls.find(
      (c) => c.method === "POST" && c.url.pathname.endsWith("/payouts"),
    )!.body!;
    expect(first.request_id).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
    expect(first.amount).toBe("50");
    expect(first.method).toBe("usdt_erc20");
    expect(first.wallet).toBe(settingsFixture.wallet);
    expect(screen.getByLabelText("Сумма, USD")).toBeDisabled();
    fireEvent.click(
      screen.getByRole("button", { name: "refresh-test", hidden: true }),
    );
    await waitFor(() =>
      expect(
        s.calls.filter((c) => c.url.pathname.endsWith("/balance")).length,
      ).toBeGreaterThan(1),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Повторить ту же заявку" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    const posts = s.calls.filter(
      (c) => c.method === "POST" && c.url.pathname.endsWith("/payouts"),
    );
    expect(posts).toHaveLength(2);
    expect(posts[1].body).toEqual(first);
    expect(screen.getByText(payoutFixture.id)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Отменить заявку/ }),
    ).not.toBeInTheDocument();
    expect(s.calls.some((c) => c.url.pathname.endsWith("/cancel"))).toBe(false);
  });
  it("coalesces double submission into one POST and one shared result", async () => {
    const s = publisherServer();
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    s.overrides.set("POST /api/publisher/payouts", async () => {
      await gate;
      return reply(payoutFixture);
    });
    function Probe() {
      const p = usePublisher(),
        [done, setDone] = useState(false);
      return (
        <button
          onClick={() => {
            const payload = {
              amount: "50",
              method: "usdt_erc20" as const,
              wallet: settingsFixture.wallet,
            };
            const a = p.createPayout(payload),
              b = p.createPayout(payload);
            expect(a).toBe(b);
            void Promise.all([a, b]).then(() => setDone(true));
          }}
        >
          {done ? "done" : "send-twice"}
        </button>
      );
    }
    render(<Probe />, { wrapper: PublisherTestWrapper });
    fireEvent.click(screen.getByRole("button", { name: "send-twice" }));
    await waitFor(() =>
      expect(
        s.calls.filter(
          (c) => c.method === "POST" && c.url.pathname.endsWith("/payouts"),
        ),
      ).toHaveLength(1),
    );
    release();
    await screen.findByText("done");
  });
  it("shows ledger 503 rather than zeros and financial 409 rather than a fake successful payout", async () => {
    const s = publisherServer();
    s.overrides.set("GET /api/publisher/balance", () =>
      reply(null, 503, "ledger unavailable"),
    );
    render(<PublisherPayouts />, { wrapper: PublisherTestWrapper });
    await screen.findByRole("alert");
    expect(screen.queryByText("$0,00")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Запросить выплату" }),
    ).toBeDisabled();
    cleanup();
    s.overrides.delete("GET /api/publisher/balance");
    s.overrides.set("POST /api/publisher/payouts", () =>
      reply(null, 409, "insufficient available balance"),
    );
    render(<PublisherPayouts />, { wrapper: PublisherTestWrapper });
    await screen.findAllByText("$200,12345678");
    fireEvent.click(screen.getByRole("button", { name: "Запросить выплату" }));
    fireEvent.change(screen.getByLabelText("Сумма, USD"), {
      target: { value: "50" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Создать заявку" }));
    await screen.findByText("Недостаточно средств для выплаты.");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.queryByText(payoutFixture.id)).not.toBeInTheDocument();
  });
});
