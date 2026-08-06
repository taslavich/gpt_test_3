import { ACCESS_TOKEN_KEY, API_BASE_URL, REFRESH_TOKEN_KEY } from "./config";

export class ApiError extends Error {
  status: number;
  code?: string;
  fields?: Record<string, string>;
  constructor(status: number, message: string, code?: string, fields?: Record<string, string>) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE" | "PUT";
  body?: unknown;
  query?: Record<string, string | number | boolean | undefined | null>;
  auth?: boolean;
  signal?: AbortSignal;
  retryOnUnauthorized?: boolean;
}

interface AuthenticatedFetchOptions {
  auth?: boolean;
  retryOnUnauthorized?: boolean;
}

let refreshPromise: Promise<boolean> | null = null;

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? value as Record<string, unknown>
    : null;
}

function buildUrl(path: string, query?: RequestOptions["query"]): string {
  const url = new URL(path.startsWith("http") ? path : `${API_BASE_URL}${path}`);
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v === undefined || v === null) continue;
      url.searchParams.set(k, String(v));
    }
  }
  return url.toString();
}

async function refreshAccessToken(): Promise<boolean> {
  const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY);
  if (!refreshToken) return false;

  if (!refreshPromise) {
    refreshPromise = (async () => {
      try {
        const res = await fetch(buildUrl("/api/auth/refresh"), {
          method: "POST",
          headers: { Accept: "application/json", "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });

        const text = await res.text();
        let data: unknown = null;
        if (text) {
          try { data = JSON.parse(text); } catch { data = text; }
        }

        if (!res.ok) return false;

        const parsed = asRecord(data);
        const payload = asRecord(parsed?.data) || parsed;
        const access = payload?.access_token;
        const refresh = payload?.refresh_token;
        if (typeof access !== "string" || typeof refresh !== "string") return false;

        localStorage.setItem(ACCESS_TOKEN_KEY, access);
        localStorage.setItem(REFRESH_TOKEN_KEY, refresh);
        return true;
      } catch {
        return false;
      } finally {
        refreshPromise = null;
      }
    })();
  }

  return refreshPromise;
}

export async function authenticatedFetch(
  input: string,
  init: RequestInit = {},
  options: AuthenticatedFetchOptions = {},
): Promise<Response> {
  const { auth = true, retryOnUnauthorized = true } = options;
  const headers = new Headers(init.headers);
  if (auth) {
    const token = localStorage.getItem(ACCESS_TOKEN_KEY);
    if (token) headers.set("Authorization", `Bearer ${token}`);
    else headers.delete("Authorization");
  } else {
    headers.delete("Authorization");
  }

  let response: Response;
  try {
    response = await fetch(input, { ...init, headers });
  } catch (error) {
    const message =
      error instanceof TypeError
        ? "Network error: failed to reach API. Check VITE_API_BASE_URL, backend availability, CORS, and network settings."
        : "Network error: request failed before receiving a response.";
    throw new ApiError(0, message, "NETWORK_ERROR");
  }

  if (
    response.status === 401
    && auth
    && retryOnUnauthorized
    && await refreshAccessToken()
  ) {
    return authenticatedFetch(input, init, { auth, retryOnUnauthorized: false });
  }
  return response;
}

export async function http<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, query, auth = true, signal, retryOnUnauthorized = true } = opts;
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await authenticatedFetch(buildUrl(path, query), {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    signal,
  }, { auth, retryOnUnauthorized });

  if (res.status === 204) return undefined as T;

  let data: unknown = null;
  const text = await res.text();
  if (text) {
    try { data = JSON.parse(text); } catch { data = text; }
  }

  if (!res.ok) {
    // Backend may return either `{ error: { message, code, fields } }`
    // or a flat `{ success: false, errorMsg: "..." }` envelope. Surface
    // whichever is present.
    const payload = asRecord(data);
    const err = asRecord(payload?.error);
    const flatMsg = payload?.errorMsg;

    // Localized message for expired/invalid sessions when refresh is unavailable or failed.
    if (res.status === 401 && auth) {
      const nav = (typeof navigator !== "undefined" && navigator.language || "").toLowerCase();
      const lang: "ru" | "es" | "en" = nav.startsWith("ru") ? "ru" : nav.startsWith("es") ? "es" : "en";
      const message =
        lang === "ru" ? "Сессия устарела, пожалуйста, войдите заново"
        : lang === "es" ? "Tu sesión ha caducado, vuelve a iniciar sesión"
        : "Your session has expired, please sign in again";
      const code = typeof err?.code === "string" ? err.code : "SESSION_EXPIRED";
      const fields = asRecord(err?.fields);
      const normalizedFields = fields
        ? Object.fromEntries(
            Object.entries(fields).filter((entry): entry is [string, string] => typeof entry[1] === "string"),
          )
        : undefined;
      throw new ApiError(res.status, message, code, normalizedFields);
    }

    const errorMessage = typeof err?.message === "string" ? err.message : undefined;
    const errorCode = typeof err?.code === "string" ? err.code : undefined;
    const errorFields = asRecord(err?.fields);
    throw new ApiError(
      res.status,
      errorMessage
        || (typeof flatMsg === "string" ? flatMsg : undefined)
        || (typeof data === "string" ? data : `HTTP ${res.status}`),
      errorCode,
      errorFields
        ? Object.fromEntries(
            Object.entries(errorFields).filter((entry): entry is [string, string] => typeof entry[1] === "string"),
          )
        : undefined,
    );
  }
  // Return as-is. Envelope handling (`{ success, errorMsg, data }`) is done
  // centrally in `api/index.ts` so both providers behave the same.
  return data as T;
}
