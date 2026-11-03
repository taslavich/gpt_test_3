const LOGIN_INTRO_STORAGE_KEY = "twinbid.loginIntro.pending";
export const LOGIN_INTRO_REQUEST_EVENT = "twinbid:login-intro-request";

const LOGIN_INTRO_REQUEST_MAX_AGE_MS = 60_000;
let inMemoryRequestAt: number | null = null;

/**
 * Marks a successful interactive login. The event starts the intro immediately,
 * while sessionStorage keeps the intent across a possible route reload.
 */
export function requestLoginIntro(): void {
  if (typeof window === "undefined") return;

  const requestedAt = Date.now();
  inMemoryRequestAt = requestedAt;

  try {
    window.sessionStorage.setItem(LOGIN_INTRO_STORAGE_KEY, String(requestedAt));
  } catch {
    // Storage can be unavailable in privacy modes. The in-page event still works.
  }

  window.dispatchEvent(new Event(LOGIN_INTRO_REQUEST_EVENT));
}

/** Consumes a fresh login request exactly once. */
export function consumeLoginIntroRequest(now = Date.now()): boolean {
  if (typeof window === "undefined") return false;

  let rawRequestedAt: string | null = null;

  try {
    rawRequestedAt = window.sessionStorage.getItem(LOGIN_INTRO_STORAGE_KEY);
    window.sessionStorage.removeItem(LOGIN_INTRO_STORAGE_KEY);
  } catch {
    // Fall back to the in-memory marker created during the same page session.
  }

  const requestedAt = rawRequestedAt ? Number(rawRequestedAt) : inMemoryRequestAt;
  inMemoryRequestAt = null;

  if (requestedAt === null) return false;

  const age = now - requestedAt;
  return Number.isFinite(requestedAt) && age >= 0 && age <= LOGIN_INTRO_REQUEST_MAX_AGE_MS;
}
