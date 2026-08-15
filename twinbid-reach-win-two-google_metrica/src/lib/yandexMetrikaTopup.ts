import type { ApiUserTransaction } from "@/api/types";
import { BALANCE_TOPUP_SUCCESS_GOAL, reachYandexGoal } from "@/lib/yandexMetrika";

const TOPUP_GOAL_STATE_KEY = "twinbid_metrika_topup_goal_state";
const MAX_STORED_TRANSACTION_IDS = 200;

interface TopupGoalState {
  pending: string[];
  sent: string[];
}

let memoryTopupGoalState: TopupGoalState = { pending: [], sent: [] };

function normalizeIds(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string" && item.length > 0);
}

function readTopupGoalState(): TopupGoalState {
  if (typeof window === "undefined") return memoryTopupGoalState;
  try {
    const raw = window.localStorage.getItem(TOPUP_GOAL_STATE_KEY);
    if (!raw) return memoryTopupGoalState;
    const parsed = JSON.parse(raw) as Partial<TopupGoalState>;
    memoryTopupGoalState = {
      pending: normalizeIds(parsed.pending),
      sent: normalizeIds(parsed.sent),
    };
  } catch {
    // Keep the in-memory fallback when storage is unavailable or corrupted.
  }
  return memoryTopupGoalState;
}

function writeTopupGoalState(state: TopupGoalState): void {
  memoryTopupGoalState = {
    pending: state.pending.slice(-MAX_STORED_TRANSACTION_IDS),
    sent: state.sent.slice(-MAX_STORED_TRANSACTION_IDS),
  };
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(TOPUP_GOAL_STATE_KEY, JSON.stringify(memoryTopupGoalState));
  } catch {
    // The in-memory state still prevents duplicate events during this session.
  }
}

/** Remember only transactions created by this frontend, avoiding retroactive goals for old history. */
export function rememberTopupForGoal(transactionRowId: string | null | undefined): void {
  if (!transactionRowId) return;
  const state = readTopupGoalState();
  if (state.pending.includes(transactionRowId) || state.sent.includes(transactionRowId)) return;
  writeTopupGoalState({ ...state, pending: [...state.pending, transactionRowId] });
}

export function hasPendingTopupsForGoal(): boolean {
  return readTopupGoalState().pending.length > 0;
}

export function discardTopupFromGoal(transactionRowId: string): void {
  const state = readTopupGoalState();
  if (!state.pending.includes(transactionRowId)) return;
  writeTopupGoalState({
    ...state,
    pending: state.pending.filter(id => id !== transactionRowId),
  });
}

/** Send one goal per credited backend transaction, even across polling and page reloads. */
export function trackBalanceTopupSuccess(
  transaction: Pick<ApiUserTransaction, "id" | "status" | "credited_at">,
): boolean {
  const credited = transaction.status === "approved" && transaction.credited_at != null;
  if (!transaction.id || !credited) return false;

  const state = readTopupGoalState();
  if (state.sent.includes(transaction.id) || !state.pending.includes(transaction.id)) return false;
  if (!reachYandexGoal(BALANCE_TOPUP_SUCCESS_GOAL)) return false;

  writeTopupGoalState({
    pending: state.pending.filter(id => id !== transaction.id),
    sent: [...state.sent, transaction.id],
  });
  return true;
}
