import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type DashboardTheme = "dark" | "light";

interface DashboardThemeContextValue {
  theme: DashboardTheme;
  setTheme: (theme: DashboardTheme) => void;
}

const STORAGE_KEY = "twinbid-dashboard-theme";

const DashboardThemeContext = createContext<DashboardThemeContextValue | null>(null);

function getInitialTheme(): DashboardTheme {
  if (typeof window === "undefined") return "dark";
  return window.localStorage.getItem(STORAGE_KEY) === "light" ? "light" : "dark";
}

export function DashboardThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<DashboardTheme>(getInitialTheme);

  const setTheme = (nextTheme: DashboardTheme) => {
    setThemeState(nextTheme);
    window.localStorage.setItem(STORAGE_KEY, nextTheme);
  };

  useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle("dashboard-light", theme === "light");
    root.classList.toggle("dashboard-dark", theme === "dark");
    root.style.colorScheme = theme;

    return () => {
      root.classList.remove("dashboard-light", "dashboard-dark");
      root.style.removeProperty("color-scheme");
    };
  }, [theme]);

  const value = useMemo(() => ({ theme, setTheme }), [theme]);

  return <DashboardThemeContext.Provider value={value}>{children}</DashboardThemeContext.Provider>;
}

export function useDashboardTheme() {
  const context = useContext(DashboardThemeContext);
  if (!context) throw new Error("useDashboardTheme must be used inside DashboardThemeProvider");
  return context;
}
