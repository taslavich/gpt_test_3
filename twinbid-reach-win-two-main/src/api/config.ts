// Use the working backend by default, including builds without an ENV file.
const configuredApiUrl = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.trim();

export const API_BASE_URL: string = configuredApiUrl || "https://twinbid.io";

/** Local fixtures are enabled only by an explicit opt-in. */
export const USE_MOCK: boolean =
  (import.meta.env.VITE_USE_MOCK as string | undefined) === "true";

export const ACCESS_TOKEN_KEY = "twinbid_access_token";
export const REFRESH_TOKEN_KEY = "twinbid_refresh_token";
