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

let refreshPromise: Promise<boolean> | null = null;

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
        let data: any = null;
        if (text) {
          try { data = JSON.parse(text); } catch { data = text; }
        }

        if (!res.ok) return false;

        const payload = data && typeof data === "object" && "data" in data ? data.data : data;
        const access = payload?.access_token;
        const refresh = payload?.refresh_token;
        if (!access || !refresh) return false;

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

export async function http<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, query, auth = true, signal, retryOnUnauthorized = true } = opts;
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (auth) {
    const token = localStorage.getItem(ACCESS_TOKEN_KEY);
    if (token) headers.Authorization = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(buildUrl(path, query), {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
    });
  } catch (error) {
    const message =
      error instanceof TypeError
        ? "Network error: failed to reach API. Check VITE_API_BASE_URL, backend availability, CORS, and HTTPS certificate."
        : "Network error: request failed before receiving a response.";
    throw new ApiError(0, message, "NETWORK_ERROR");
  }

  if (res.status === 204) return undefined as T;

  let data: any = null;
  const text = await res.text();
  if (text) {
    try { data = JSON.parse(text); } catch { data = text; }
  }

  if (!res.ok) {
    // Backend may return either `{ error: { message, code, fields } }`
    // or a flat `{ success: false, errorMsg: "..." }` envelope. Surface
    // whichever is present.
    const err = data?.error;
    const flatMsg = data?.errorMsg;

    if (res.status === 401 && auth && retryOnUnauthorized && await refreshAccessToken()) {
      return http<T>(path, { method, body, query, auth, signal, retryOnUnauthorized: false });
    }

    // Localized message for expired/invalid sessions when refresh is unavailable or failed.
    if (res.status === 401 && auth) {
      const lang = (typeof navigator !== "undefined" && navigator.language || "").toLowerCase().startsWith("ru") ? "ru" : "en";
      const message = lang === "ru"
        ? "Сессия устарела, пожалуйста, войдите заново"
        : "Your session has expired, please sign in again";
      throw new ApiError(res.status, message, err?.code || "SESSION_EXPIRED", err?.fields);
    }

    throw new ApiError(
      res.status,
      err?.message || flatMsg || (typeof data === "string" ? data : `HTTP ${res.status}`),
      err?.code,
      err?.fields,
    );
  }
  // Return as-is. Envelope handling (`{ success, errorMsg, data }`) is done
  // centrally in `api/index.ts` so both providers behave the same.
  return data as T;
}
