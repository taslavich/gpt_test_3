import { ApiError } from "@/api/http";
import type { CreatePublisherPayout, PublisherPayout } from "@/api/publisher";
import { comparePublisherMoney } from "./publisherData";

export interface PayoutOperation {
  version: 1;
  request: CreatePublisherPayout;
  attempted: boolean;
  payout_id?: string;
}
export const payoutOperationKey = (account: string) =>
  "twinbid:publisher:payout:v1:" +
  encodeURIComponent(account.trim().toLowerCase());
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const storageError = () =>
  new ApiError(
    409,
    "Cannot safely read or save the pending payout. Check browser storage before retrying.",
    "PAYOUT_STORAGE",
  );
export function isPayoutRequestId(value: unknown): value is string {
  return typeof value === "string" && uuid.test(value);
}
export function samePayoutInput(
  a: Omit<CreatePublisherPayout, "request_id">,
  b: Omit<CreatePublisherPayout, "request_id">,
) {
  return (
    a.amount === b.amount && a.method === b.method && a.wallet === b.wallet
  );
}
export function payoutMatchesRequest(
  payout: PublisherPayout,
  request: CreatePublisherPayout,
) {
  return (
    payout.method === request.method &&
    payout.wallet === request.wallet &&
    comparePublisherMoney(payout.amount, request.amount) === 0
  );
}
export function readPayoutOperation(account: string): PayoutOperation | null {
  try {
    const raw = localStorage.getItem(payoutOperationKey(account));
    if (raw === null) return null;
    const v = JSON.parse(raw),
      r = v?.request;
    if (
      v?.version !== 1 ||
      typeof v.attempted !== "boolean" ||
      !r ||
      !isPayoutRequestId(r.request_id) ||
      typeof r.amount !== "string" ||
      r.amount.length > 32 ||
      !/^\d+(?:\.\d{1,8})?$/.test(r.amount) ||
      comparePublisherMoney(r.amount, "0") <= 0 ||
      !["usdt_trc20", "usdt_erc20"].includes(r.method) ||
      typeof r.wallet !== "string" ||
      !r.wallet ||
      (v.payout_id !== undefined && !isPayoutRequestId(v.payout_id))
    )
      throw storageError();
    // Whitelist request fields: auth tokens and arbitrary caller properties are never persisted.
    return {
      version: 1,
      request: {
        amount: r.amount,
        method: r.method,
        wallet: r.wallet,
        request_id: r.request_id,
      },
      attempted: v.attempted,
      ...(v.payout_id ? { payout_id: v.payout_id } : {}),
    };
  } catch {
    throw storageError();
  }
}
export function writePayoutOperation(
  account: string,
  operation: PayoutOperation,
) {
  try {
    const r = operation.request;
    const raw = JSON.stringify({
      version: 1,
      request: {
        request_id: r.request_id,
        amount: r.amount,
        method: r.method,
        wallet: r.wallet,
      },
      attempted: operation.attempted,
      ...(operation.payout_id ? { payout_id: operation.payout_id } : {}),
    });
    localStorage.setItem(payoutOperationKey(account), raw);
    if (localStorage.getItem(payoutOperationKey(account)) !== raw)
      throw storageError();
  } catch {
    throw storageError();
  }
}
export function clearPayoutOperation(account: string, requestId: string) {
  const current = readPayoutOperation(account);
  if (!current) return;
  if (current.request.request_id !== requestId)
    throw new ApiError(
      409,
      "A different payout is pending in this account",
      "PAYOUT_PENDING",
    );
  try {
    localStorage.removeItem(payoutOperationKey(account));
    if (localStorage.getItem(payoutOperationKey(account)) !== null)
      throw storageError();
  } catch {
    throw storageError();
  }
}
const active = new Set<string>();
export function withPayoutLock<T>(
  account: string,
  action: () => Promise<T>,
): Promise<T> {
  const key = payoutOperationKey(account);
  const busy = () =>
    new ApiError(
      409,
      "A payout operation is already in progress. Retry after it finishes.",
      "PAYOUT_BUSY",
    );
  if (active.has(key)) return Promise.reject(busy());
  active.add(key);
  return Promise.resolve()
    .then(() =>
      navigator.locks?.request
        ? navigator.locks.request(key, { ifAvailable: true }, (lock) => {
            if (!lock) throw busy();
            return action();
          })
        : action(),
    )
    .finally(() => active.delete(key));
}
