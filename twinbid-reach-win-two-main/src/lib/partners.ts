const PARTNER_STORAGE_KEY = "twinbid_partner";
const PARTNER_CODE_PATTERN = /^[A-Za-z0-9_-]{6,64}$/;
const PARTNER_ID_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
const PARTNER_ID_RANDOM_LENGTH = 10;

/**
 * Generate a random public partner id once during signup.
 * It deliberately uses no user or database data as its source.
 */
export function generatePartnerId(): string {
  const cryptoApi = globalThis.crypto;
  if (!cryptoApi?.getRandomValues) {
    throw new Error("Secure random number generation is unavailable");
  }

  const characters: string[] = [];
  const unbiasedLimit = Math.floor(256 / PARTNER_ID_ALPHABET.length) * PARTNER_ID_ALPHABET.length;

  while (characters.length < PARTNER_ID_RANDOM_LENGTH) {
    const randomBytes = new Uint8Array(PARTNER_ID_RANDOM_LENGTH - characters.length);
    cryptoApi.getRandomValues(randomBytes);
    for (const value of randomBytes) {
      if (value >= unbiasedLimit) continue;
      characters.push(PARTNER_ID_ALPHABET[value % PARTNER_ID_ALPHABET.length]);
      if (characters.length === PARTNER_ID_RANDOM_LENGTH) break;
    }
  }

  const randomPart = characters.join("");

  return `TB${randomPart}`;
}

export function createPartnerLinkFromCode(code: string, origin?: string): string {
  const base = (origin || (typeof window !== "undefined" ? window.location.origin : "https://twinbid.io")).replace(/\/$/, "");
  const normalized = normalizePartnerCode(code);
  return `${base}/?partner=${encodeURIComponent(normalized ?? code)}`;
}

export function normalizePartnerCode(code: string): string | null {
  const normalized = code.trim();
  if (!PARTNER_CODE_PATTERN.test(normalized)) return null;
  return normalized;
}

/** Capture `?partner=CODE` independently from marketing UTM parameters. */
export function capturePartnerCodeFromUrl(): void {
  if (typeof window === "undefined") return;
  try {
    const raw = new URLSearchParams(window.location.search).get("partner");
    if (!raw) return;
    const partner = normalizePartnerCode(raw);
    if (partner) localStorage.setItem(PARTNER_STORAGE_KEY, partner);
  } catch { /* localStorage can be unavailable */ }
}

export function getStoredPartnerCode(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return localStorage.getItem(PARTNER_STORAGE_KEY);
  } catch {
    return null;
  }
}
