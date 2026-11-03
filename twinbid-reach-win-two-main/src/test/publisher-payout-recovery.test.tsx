import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, useState, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePublisherPayout } from "@/contexts/usePublisherPayout";
import {
  clearPayoutOperation,
  payoutOperationKey,
  readPayoutOperation,
  writePayoutOperation,
  withPayoutLock,
  type PayoutOperation,
} from "@/lib/publisherPayoutOperation";
import PublisherPayouts from "@/pages/PublisherPayouts";
import {
  PublisherTestWrapper,
  publisherServer,
  userFixture,
  languageFixture,
  payoutFixture,
  settingsFixture,
  reply,
} from "./publisher-fixtures";
import type { CreatePublisherPayout, PublisherPayout } from "@/api/publisher";
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ user: userFixture, signOut: vi.fn() }),
}));
vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => languageFixture,
}));
const account = userFixture.email;
const input = {
  amount: "50",
  method: "usdt_erc20" as const,
  wallet: settingsFixture.wallet,
};
const refresh = vi.fn().mockResolvedValue(undefined);
const operation = (): PayoutOperation => ({
  version: 1,
  request: { ...input, request_id: crypto.randomUUID() },
  attempted: true,
});
const posts = (s: ReturnType<typeof publisherServer>) =>
  s.calls.filter(
    (c) => c.method === "POST" && c.url.pathname.endsWith("/payouts"),
  );
const resultFor = (body: Record<string, unknown>): PublisherPayout => ({
  ...payoutFixture,
  amount: String(body.amount),
  method: body.method as PublisherPayout["method"],
  wallet: String(body.wallet),
});
function Wrapper({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      }),
  );
  return createElement(QueryClientProvider, { client }, children);
}
function mount(accountValue = account, scope = "server-user:" + accountValue) {
  return renderHook(
    ({ owner, key }) => usePublisherPayout(owner, key, refresh),
    { initialProps: { owner: accountValue, key: scope }, wrapper: Wrapper },
  );
}
beforeEach(() => {
  localStorage.clear();
  refresh.mockClear();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("Durable Publisher payout operations", () => {
  it("persists a whitelist before POST and clears it only after the authoritative response", async () => {
    const s = publisherServer();
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    s.overrides.set("POST /api/publisher/payouts", async (req) => {
      const saved = readPayoutOperation(account)!;
      expect(saved.request).toEqual(req.body);
      expect(saved.attempted).toBe(true);
      expect(Object.keys(saved.request).sort()).toEqual([
        "amount",
        "method",
        "request_id",
        "wallet",
      ]);
      expect(JSON.stringify(saved)).not.toMatch(
        /token|authorization|password|test-access/i,
      );
      await gate;
      return reply(resultFor(req.body!));
    });
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    let p!: Promise<PublisherPayout>;
    act(() => {
      p = h.result.current.createPayout({
        ...input,
        jwt: "do-not-save",
      } as typeof input);
    });
    await waitFor(() => expect(posts(s)).toHaveLength(1));
    release();
    await act(async () => {
      await p;
    });
    expect(localStorage.getItem(payoutOperationKey(account))).toBeNull();
    expect(h.result.current.payoutIntent).toBeNull();
  });
  it("keeps the UUID after response loss, reloads, checks history without guessing from equal details, and retries it once", async () => {
    const s = publisherServer();
    let attempts = 0;
    s.overrides.set("POST /api/publisher/payouts", (req) => {
      if (++attempts === 1) throw new TypeError("response lost after commit");
      return reply(resultFor(req.body!));
    });
    // An equal-looking row is not an idempotency correlation; GET has no request_id.
    s.overrides.set("GET /api/publisher/payouts", () =>
      reply({ items: [payoutFixture], limit: 50, offset: 0 }),
    );
    const first = mount();
    await waitFor(() => expect(first.result.current.payoutBusy).toBe(false));
    await act(async () => {
      await expect(
        first.result.current.createPayout(input),
      ).rejects.toMatchObject({ status: 0 });
    });
    const saved = readPayoutOperation(account)!;
    expect(saved.request.request_id).toBe(posts(s)[0].body!.request_id);
    first.unmount();
    const reload = mount();
    await waitFor(() => expect(reload.result.current.payoutBusy).toBe(false));
    expect(reload.result.current.payoutIntent).toEqual(saved.request);
    expect(posts(s)).toHaveLength(1);
    expect(
      s.calls.some(
        (c) => c.method === "GET" && c.url.pathname.endsWith("/payouts"),
      ),
    ).toBe(true);
    let a!: Promise<PublisherPayout>, b!: Promise<PublisherPayout>;
    act(() => {
      a = reload.result.current.createPayout(saved.request);
      b = reload.result.current.createPayout(saved.request);
      expect(a).toBe(b);
    });
    await act(async () => {
      await a;
    });
    expect(posts(s)).toHaveLength(2);
    expect(posts(s)[1].body).toEqual(posts(s)[0].body);
    expect(readPayoutOperation(account)).toBeNull();
  });
  it("restores the existing payout UI after a reload and shows the server-created record", async () => {
    const s = publisherServer(),
      saved = operation();
    writePayoutOperation(account, saved);
    s.overrides.set("GET /api/publisher/payouts", () =>
      reply({ items: [payoutFixture], limit: 50, offset: 0 }),
    );
    render(<PublisherPayouts />, { wrapper: PublisherTestWrapper });
    const retry = await screen.findByRole("button", {
      name: "Повторить прежнюю заявку",
    });
    await waitFor(() => expect(retry).toBeEnabled());
    expect(screen.getByText(payoutFixture.id)).toBeInTheDocument();
    expect(posts(s)).toHaveLength(0);
    fireEvent.click(retry);
    expect(screen.getByLabelText("Сумма, USD")).toHaveValue(
      saved.request.amount,
    );
    expect(screen.getByLabelText("Сумма, USD")).toBeDisabled();
    fireEvent.click(
      screen.getByRole("button", { name: "Повторить ту же заявку" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(posts(s)[0].body).toEqual(saved.request);
  });
  it("reconciles a known exact server ID after a cleanup interruption without another POST", async () => {
    const s = publisherServer();
    s.overrides.set("GET /api/publisher/payouts", () =>
      reply({ items: [payoutFixture], limit: 50, offset: 0 }),
    );
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    const original = Storage.prototype.removeItem;
    const remove = vi
      .spyOn(Storage.prototype, "removeItem")
      .mockImplementation(function (key: string) {
        if (key === payoutOperationKey(account))
          throw new DOMException("blocked", "SecurityError");
        return original.call(this, key);
      });
    await act(async () => {
      await expect(h.result.current.createPayout(input)).rejects.toMatchObject({
        code: "PAYOUT_STORAGE",
      });
    });
    expect(readPayoutOperation(account)?.payout_id).toBe(payoutFixture.id);
    remove.mockRestore();
    h.unmount();
    const reload = mount();
    await waitFor(() => expect(reload.result.current.payoutBusy).toBe(false));
    expect(reload.result.current.payoutIntent).toBeNull();
    expect(readPayoutOperation(account)).toBeNull();
    expect(posts(s)).toHaveLength(1);
  });
  it("does not treat an empty history page or a failed history read as proof of absence", async () => {
    const s = publisherServer(),
      saved = operation();
    writePayoutOperation(account, saved);
    s.overrides.set("GET /api/publisher/payouts", () =>
      reply({ items: [], limit: 50, offset: 0 }),
    );
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    expect(readPayoutOperation(account)).toEqual(saved);
    h.unmount();
    s.overrides.set("GET /api/publisher/payouts", () =>
      reply(null, 503, "history unavailable"),
    );
    const reload = mount();
    await waitFor(() => expect(reload.result.current.payoutBusy).toBe(false));
    expect(readPayoutOperation(account)).toEqual(saved);
    expect(posts(s)).toHaveLength(0);
    await act(async () => {
      await reload.result.current.createPayout(saved.request);
    });
    expect(posts(s)[0].body).toEqual(saved.request);
    expect(readPayoutOperation(account)).toBeNull();
  });
  it("does not match a known ID to different details or assume it is absent on another page", async () => {
    const s = publisherServer(),
      saved = { ...operation(), payout_id: payoutFixture.id };
    writePayoutOperation(account, saved);
    s.overrides.set("GET /api/publisher/payouts", () =>
      reply({
        items: [{ ...payoutFixture, amount: "75" }],
        limit: 50,
        offset: 0,
      }),
    );
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    expect(readPayoutOperation(account)).toEqual(saved);
    expect(posts(s)).toHaveLength(0);
  });
  it.each([
    { amount: "75" },
    { method: "usdt_trc20" as const },
    { wallet: "another-wallet" },
  ])("never reuses a pending key for different details %j", async (changed) => {
    const s = publisherServer(),
      saved = operation();
    writePayoutOperation(account, saved);
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    await act(async () => {
      await expect(
        h.result.current.createPayout({ ...input, ...changed }),
      ).rejects.toMatchObject({ code: "PAYOUT_PENDING" });
    });
    expect(posts(s)).toHaveLength(0);
    expect(readPayoutOperation(account)).toEqual(saved);
  });
  it("keeps uncertain operations even when a retry gets 401 or 409", async () => {
    const s = publisherServer(),
      saved = operation();
    writePayoutOperation(account, saved);
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    for (const status of [401, 409]) {
      s.overrides.set("POST /api/publisher/payouts", () =>
        reply(null, status, "refused"),
      );
      await act(async () => {
        await expect(
          h.result.current.createPayout(saved.request),
        ).rejects.toMatchObject({ status });
      });
      expect(readPayoutOperation(account)).toEqual(saved);
    }
    expect(
      posts(s).every((c) => c.body?.request_id === saved.request.request_id),
    ).toBe(true);
  });
  it("allows a corrected fresh request only after its first POST has a definite business rejection", async () => {
    const s = publisherServer();
    let n = 0;
    s.overrides.set("POST /api/publisher/payouts", (req) =>
      ++n === 1 ? reply(null, 400, "bad input") : reply(resultFor(req.body!)),
    );
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    await act(async () => {
      await expect(h.result.current.createPayout(input)).rejects.toMatchObject({
        status: 400,
      });
    });
    expect(readPayoutOperation(account)).toBeNull();
    await act(async () => {
      await h.result.current.createPayout({ ...input, amount: "75" });
    });
    expect(posts(s)[0].body!.request_id).not.toBe(posts(s)[1].body!.request_id);
    expect(posts(s)[1].body!.amount).toBe("75");
  });
  it("retains the operation after a real client timeout", async () => {
    const s = publisherServer();
    s.overrides.set(
      "POST /api/publisher/payouts",
      (req) =>
        new Promise((_resolve, reject) => {
          req.signal!.addEventListener("abort", () =>
            reject(new DOMException("timeout", "AbortError")),
          );
        }),
    );
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    vi.useFakeTimers();
    let p!: Promise<PublisherPayout>;
    act(() => {
      p = h.result.current.createPayout(input);
    });
    const rejected = expect(p).rejects.toMatchObject({ status: 0 });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(20001);
      await rejected;
    });
    vi.useRealTimers();
    expect(readPayoutOperation(account)?.request.request_id).toBe(
      posts(s)[0].body!.request_id,
    );
  });
  it("fails before POST if storage is corrupt or unavailable, instead of losing idempotency", async () => {
    const s = publisherServer();
    localStorage.setItem(payoutOperationKey(account), "corrupt");
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    await act(async () => {
      await expect(h.result.current.createPayout(input)).rejects.toMatchObject({
        code: "PAYOUT_STORAGE",
      });
    });
    expect(localStorage.getItem(payoutOperationKey(account))).toBe("corrupt");
    localStorage.removeItem(payoutOperationKey(account));
    const set = vi
      .spyOn(Storage.prototype, "setItem")
      .mockImplementation(() => {
        throw new DOMException("quota", "QuotaExceededError");
      });
    await act(async () => {
      await expect(h.result.current.createPayout(input)).rejects.toMatchObject({
        code: "PAYOUT_STORAGE",
      });
    });
    set.mockRestore();
    expect(posts(s)).toHaveLength(0);
  });
  it("serializes concurrent instances and honors a busy Web Lock without replacing the journal", async () => {
    const s = publisherServer(),
      saved = operation();
    writePayoutOperation(account, saved);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const first = withPayoutLock(account, () => gate);
    await expect(
      withPayoutLock(account, async () => undefined),
    ).rejects.toMatchObject({ code: "PAYOUT_BUSY" });
    release();
    await first;
    vi.stubGlobal("navigator", {
      language: "en",
      locks: {
        request: vi.fn(async (_name, _options, callback) => callback(null)),
      },
    });
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    await act(async () => {
      await expect(
        h.result.current.createPayout(saved.request),
      ).rejects.toMatchObject({ code: "PAYOUT_BUSY" });
    });
    expect(readPayoutOperation(account)).toEqual(saved);
    expect(posts(s)).toHaveLength(0);
  });
  it("releases the in-page guard when the browser lock API rejects synchronously", async () => {
    publisherServer();
    const request = vi.fn(() => {
      throw new DOMException("not allowed", "SecurityError");
    });
    vi.stubGlobal("navigator", { language: "en", locks: { request } });
    await expect(withPayoutLock(account, async () => "ok")).rejects.toThrow(
      "not allowed",
    );
    await expect(withPayoutLock(account, async () => "ok")).rejects.toThrow(
      "not allowed",
    );
    expect(request).toHaveBeenCalledTimes(2);
  });
  it("isolates users, ignores late results for a former user, and restores across the existing login ID hydration", async () => {
    const s = publisherServer(),
      accountB = "second@example.test";
    let releaseA!: () => void;
    let rejectB!: () => void;
    const gateA = new Promise<void>((resolve) => {
        releaseA = resolve;
      }),
      gateB = new Promise<void>((_resolve, reject) => {
        rejectB = () => reject(new TypeError("lost B response"));
      });
    s.overrides.set("POST /api/publisher/payouts", async (req) => {
      if (req.body!.wallet === input.wallet) await gateA;
      else await gateB;
      return reply(resultFor(req.body!));
    });
    const h = mount(account, "mock-user:" + account);
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    let pA!: Promise<PublisherPayout>;
    act(() => {
      pA = h.result.current.createPayout(input);
    });
    await waitFor(() => expect(posts(s)).toHaveLength(1));
    const savedA = readPayoutOperation(account)!;
    h.rerender({ owner: accountB, key: "server-B:" + accountB });
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    expect(h.result.current.payoutIntent).toBeNull();
    let pB!: Promise<PublisherPayout>;
    act(() => {
      pB = h.result.current.createPayout({ ...input, wallet: "second-wallet" });
    });
    const errorB = expect(pB).rejects.toMatchObject({ status: 0 });
    await waitFor(() => expect(posts(s)).toHaveLength(2));
    const savedB = readPayoutOperation(accountB)!;
    expect(savedA.request.request_id).not.toBe(savedB.request.request_id);
    releaseA();
    await act(async () => {
      await pA;
    });
    expect(h.result.current.payoutIntent).toEqual(savedB.request);
    expect(readPayoutOperation(account)).toBeNull();
    rejectB();
    await act(async () => {
      await errorB;
    });
    h.rerender({ owner: account, key: "server-A:" + account });
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    expect(h.result.current.payoutIntent).toBeNull();
    h.rerender({ owner: accountB, key: "hydrated-B:" + accountB });
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    expect(h.result.current.payoutIntent).toEqual(savedB.request);
  });
  it("cancels queued transmission when the authorized account changes before POST", async () => {
    const s = publisherServer(),
      h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    let p!: Promise<PublisherPayout>;
    act(() => {
      p = h.result.current.createPayout(input);
      h.rerender({ owner: "new@example.test", key: "new-user" });
    });
    await act(async () => {
      await expect(p).rejects.toMatchObject({ code: "PAYOUT_OWNER_CHANGED" });
    });
    expect(posts(s)).toHaveLength(0);
    expect(readPayoutOperation(account)).toBeNull();
    expect(h.result.current.payoutIntent).toBeNull();
  });
  it("does not submit a restored former user's request after an awaited history read", async () => {
    const s = publisherServer(),
      saved = operation();
    writePayoutOperation(account, saved);
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    s.overrides.set("GET /api/publisher/payouts", async () => {
      await gate;
      return reply({ items: [], limit: 50, offset: 0 });
    });
    const before = s.calls.length;
    let p!: Promise<PublisherPayout>;
    act(() => {
      p = h.result.current.createPayout(saved.request);
    });
    await waitFor(() =>
      expect(
        s.calls.slice(before).some((c) => c.url.pathname.endsWith("/payouts")),
      ).toBe(true),
    );
    h.rerender({ owner: "new@example.test", key: "new-user" });
    release();
    await act(async () => {
      await expect(p).rejects.toMatchObject({ code: "PAYOUT_OWNER_CHANGED" });
    });
    expect(posts(s)).toHaveLength(0);
    expect(readPayoutOperation(account)).toEqual(saved);
    expect(h.result.current.payoutIntent).toBeNull();
  });
  it("rejects a stale former-user callback without changing the new user's state", async () => {
    const s = publisherServer(),
      h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    const stale = h.result.current.createPayout;
    h.rerender({ owner: "new@example.test", key: "new-user" });
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    await act(async () => {
      await expect(stale(input)).rejects.toMatchObject({
        code: "PAYOUT_OWNER_CHANGED",
      });
    });
    expect(posts(s)).toHaveLength(0);
    expect(readPayoutOperation(account)).toBeNull();
    expect(h.result.current.payoutIntent).toBeNull();
    expect(h.result.current.payoutBusy).toBe(false);
  });
  it("uses the old request_id for a stale retry after another tab already cleared the operation", async () => {
    const s = publisherServer(),
      saved = operation();
    writePayoutOperation(account, saved);
    const h = mount();
    await waitFor(() => expect(h.result.current.payoutBusy).toBe(false));
    clearPayoutOperation(account, saved.request.request_id);
    act(() => {
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: payoutOperationKey(account),
          newValue: null,
        }),
      );
    });
    await waitFor(() =>
      expect(h.result.current.payoutIntent).toEqual(saved.request),
    );
    await act(async () => {
      await h.result.current.createPayout(h.result.current.payoutIntent!);
    });
    expect(posts(s)[0].body!.request_id).toBe(saved.request.request_id);
  });
});
