import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  BALANCE_TOPUP_SUCCESS_GOAL,
  REGISTRATION_SUCCESS_GOAL,
  trackRegistrationSuccess,
  YANDEX_METRIKA_ID,
} from "@/lib/yandexMetrika";
import {
  discardTopupFromGoal,
  hasPendingTopupsForGoal,
  rememberTopupForGoal,
  trackBalanceTopupSuccess,
} from "@/lib/yandexMetrikaTopup";

describe("Yandex Metrika conversion goals", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.ym = vi.fn();
  });

  it("sends the registration goal with the configured identifier", () => {
    expect(trackRegistrationSuccess()).toBe(true);
    expect(window.ym).toHaveBeenCalledWith(
      YANDEX_METRIKA_ID,
      "reachGoal",
      REGISTRATION_SUCCESS_GOAL,
    );
  });

  it("sends the top-up goal only after a remembered transaction is credited", () => {
    rememberTopupForGoal("row-1");

    expect(trackBalanceTopupSuccess({ id: "row-1", status: "pending", credited_at: null })).toBe(false);
    expect(window.ym).not.toHaveBeenCalled();

    expect(trackBalanceTopupSuccess({ id: "row-1", status: "approved", credited_at: "2026-08-07T10:00:00Z" })).toBe(true);
    expect(window.ym).toHaveBeenCalledWith(
      YANDEX_METRIKA_ID,
      "reachGoal",
      BALANCE_TOPUP_SUCCESS_GOAL,
    );
  });

  it("does not resend the top-up goal for the same backend row id", () => {
    rememberTopupForGoal("row-2");
    const credited = { id: "row-2", status: "approved" as const, credited_at: "2026-08-07T10:00:00Z" };

    expect(trackBalanceTopupSuccess(credited)).toBe(true);
    expect(trackBalanceTopupSuccess(credited)).toBe(false);
    expect(window.ym).toHaveBeenCalledTimes(1);
  });

  it("does not create retroactive conversions from unrelated transaction history", () => {
    expect(trackBalanceTopupSuccess({
      id: "old-row",
      status: "approved",
      credited_at: "2026-01-01T10:00:00Z",
    })).toBe(false);
    expect(window.ym).not.toHaveBeenCalled();
  });

  it("stops tracking a terminal unsuccessful transaction", () => {
    rememberTopupForGoal("rejected-row");
    expect(hasPendingTopupsForGoal()).toBe(true);

    discardTopupFromGoal("rejected-row");
    expect(trackBalanceTopupSuccess({
      id: "rejected-row",
      status: "approved",
      credited_at: "2026-08-07T10:00:00Z",
    })).toBe(false);
  });
});
