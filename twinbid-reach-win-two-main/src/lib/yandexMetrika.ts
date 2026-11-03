declare global {
  interface Window {
    ym?: (counterId: number, method: string, ...args: unknown[]) => void;
  }
}

export const YANDEX_METRIKA_ID = 111369515;
export const REGISTRATION_SUCCESS_GOAL = "registration_success";
export const BALANCE_TOPUP_SUCCESS_GOAL = "balance_topup_success";

export function reachYandexGoal(goalId: string): boolean {
  if (typeof window === "undefined" || typeof window.ym !== "function") return false;
  try {
    window.ym(YANDEX_METRIKA_ID, "reachGoal", goalId);
    return true;
  } catch {
    return false;
  }
}

export function trackRegistrationSuccess(): boolean {
  return reachYandexGoal(REGISTRATION_SUCCESS_GOAL);
}
