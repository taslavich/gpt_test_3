// Captures utm_source from the URL on first visit and persists it in
// localStorage so we can attach it to the signup request even if the user
// registers minutes later after browsing the site.
const STORAGE_KEY = "twinbid_utm_source";

/** Read `utm_source` from the current URL and, if present, persist it. */
export function captureUtmSourceFromUrl(): void {
  if (typeof window === "undefined") return;
  try {
    const params = new URLSearchParams(window.location.search);
    const raw = params.get("utm_source");
    if (!raw) return;
    const value = raw.trim().slice(0, 255);
    if (!value) return;
    localStorage.setItem(STORAGE_KEY, value);
  } catch { /* ignore */ }
}

/** Return the persisted utm_source (or null if never captured). */
export function getStoredUtmSource(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}
