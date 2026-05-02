import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { api, ApiError } from "@/api";
import { ACCESS_TOKEN_KEY, REFRESH_TOKEN_KEY, API_BASE_URL } from "@/api/config";
import { DEFAULT_MANAGER_TELEGRAM } from "@/lib/constants";
import { useLanguage } from "@/contexts/LanguageContext";

/** Minimal user shape consumed by the rest of the UI. */
export interface AuthUser {
  id: string;
  email: string;
  full_name?: string;
}

interface AuthContextType {
  user: AuthUser | null;
  loading: boolean;
  signUp: (email: string, password: string, fullName: string | undefined, telegram: string) => Promise<{ error: string | null }>;
  signIn: (email: string, password: string) => Promise<{ error: string | null }>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | null>(null);

function storeTokens(access: string, refresh: string) {
  localStorage.setItem(ACCESS_TOKEN_KEY, access);
  localStorage.setItem(REFRESH_TOKEN_KEY, refresh);
}
function clearTokens() {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);
  const { t } = useLanguage();

  // Hydrate session on boot.
  useEffect(() => {
    let cancelled = false;

    // Force a refresh-token round-trip before hitting /session, so we don't
    // rely on the backend returning 401 for an expired access token (some
    // endpoints return success with empty data instead, which would log us
    // out on every reload after the access token expires).
    async function forceRefresh(): Promise<boolean> {
      const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY);
      if (!refreshToken) return false;
      try {
        const res = await fetch(`${API_BASE_URL}/api/auth/refresh`, {
          method: "POST",
          headers: { Accept: "application/json", "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
        if (!res.ok) return false;
        const text = await res.text();
        const data = text ? JSON.parse(text) : null;
        const payload = data && typeof data === "object" && "data" in data ? data.data : data;
        const access = payload?.access_token;
        const refresh = payload?.refresh_token;
        if (!access || !refresh) return false;
        localStorage.setItem(ACCESS_TOKEN_KEY, access);
        localStorage.setItem(REFRESH_TOKEN_KEY, refresh);
        return true;
      } catch {
        return false;
      }
    }

    async function hydrate(): Promise<AuthUser | null> {
      try {
        const session = await api.getSession();
        if (session) return { id: session.user_id, email: session.email, full_name: session.full_name };
      } catch { /* fallthrough to refresh */ }

      // Either no session or the call failed — try refreshing once and retry.
      const refreshed = await forceRefresh();
      if (!refreshed) return null;
      try {
        const session = await api.getSession();
        return session ? { id: session.user_id, email: session.email, full_name: session.full_name } : null;
      } catch {
        return null;
      }
    }

    (async () => {
      const next = await hydrate();
      if (cancelled) return;
      setUser(next);
      setLoading(false);
    })();
    return () => { cancelled = true; };
  }, []);

  const signUp = async (email: string, password: string, fullName: string | undefined, telegram: string) => {
    try {
      const res = await api.signup({ email, password, full_name: fullName, telegram, manager_telegram: DEFAULT_MANAGER_TELEGRAM });
      storeTokens(res.access_token, res.refresh_token);
      setUser({ id: "mock-user", email: res.user.mail, full_name: res.user.name });
      return { error: null };
    } catch (e: any) {
      return { error: e?.message || "Sign up failed" };
    }
  };

  const signIn = async (email: string, password: string) => {
    try {
      const res = await api.login({ email, password });
      storeTokens(res.access_token, res.refresh_token);
      setUser({ id: "mock-user", email: res.user.mail, full_name: res.user.name });
      return { error: null };
    } catch (e: any) {
      if (e instanceof ApiError) {
        if (e.status === 403) return { error: t("auth.error.confirmEmail") };
        if (e.status === 401 || e.status === 404) return { error: t("auth.error.invalidCredentials") };
      }
      return { error: e?.message || t("auth.error.loginFailed") };
    }
  };

  const signOut = async () => {
    try { await api.logout(); } catch { /* ignore */ }
    clearTokens();
    setUser(null);
  };

  return (
    <AuthContext.Provider value={{ user, loading, signUp, signIn, signOut }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
