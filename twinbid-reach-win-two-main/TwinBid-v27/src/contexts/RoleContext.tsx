import { createContext, useContext, useSyncExternalStore, type ReactNode } from "react";
import { useAuth } from "@/contexts/AuthContext";

export type UserRole = "advertiser" | "publisher";
const KEY = "twinbid_login_role_v2";
const EVENT = "twinbid:login-role";

function read(): { email: string; role: UserRole } | null {
  try {
    const value = JSON.parse(localStorage.getItem(KEY) || "null");
    if (typeof value?.email === "string" && (value.role === "advertiser" || value.role === "publisher")) return value;
  } catch { /* old or unavailable storage */ }
  return null;
}

/** Save the cabinet selected at a successful login, never during signup. */
export function rememberLoginRole(role: UserRole, email: string) {
  localStorage.setItem(KEY, JSON.stringify({ email: email.trim().toLowerCase(), role }));
  window.dispatchEvent(new Event(EVENT));
}

const subscribe = (listener: () => void) => {
  window.addEventListener(EVENT, listener);
  window.addEventListener("storage", listener);
  return () => { window.removeEventListener(EVENT, listener); window.removeEventListener("storage", listener); };
};
let previousRaw: string | null = null;
let cached = read();
function snapshot() {
  const raw = localStorage.getItem(KEY);
  if (raw !== previousRaw) { previousRaw = raw; cached = read(); }
  return cached;
}

const RoleContext = createContext<UserRole | null>(null);
export function RoleProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const selected = useSyncExternalStore(subscribe, snapshot, () => null);
  const role = user && selected?.email === user.email.trim().toLowerCase() ? selected.role : null;
  return <RoleContext.Provider value={role}>{children}</RoleContext.Provider>;
}
export function useRole() { return { role: useContext(RoleContext) }; }
