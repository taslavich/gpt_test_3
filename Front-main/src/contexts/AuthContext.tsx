import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { getSession, signIn as apiSignIn, signOut as apiSignOut, signUp as apiSignUp, type ApiSession, type ApiUser } from "@/lib/api";

interface AuthContextType {
  user: ApiUser | null;
  session: ApiSession | null;
  loading: boolean;
  signUp: (email: string, password: string, fullName?: string) => Promise<{ error: any }>;
  signIn: (email: string, password: string) => Promise<{ error: any }>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<ApiUser | null>(null);
  const [session, setSession] = useState<ApiSession | null>(null);
  const [loading, setLoading] = useState(true);

  const refreshSession = async () => {
    const current = await getSession();
    setSession(current);
    setUser(current?.user ?? null);
    setLoading(false);
  };

  useEffect(() => {
    refreshSession();
  }, []);

  const signUp = async (email: string, password: string, fullName?: string) => {
    const { error } = await apiSignUp(email, password, fullName);
    await refreshSession();
    return { error };
  };

  const signIn = async (email: string, password: string) => {
    const { error } = await apiSignIn(email, password);
    await refreshSession();
    return { error };
  };

  const signOut = async () => {
    await apiSignOut();
    await refreshSession();
  };

  return <AuthContext.Provider value={{ user, session, loading, signUp, signIn, signOut }}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
