import { BarChart3, Code2, Globe, Wallet, Settings, LogOut, LayoutDashboard } from "lucide-react";
import { cn } from "@/lib/utils";
import { useNavigate, useLocation } from "react-router-dom";
import { useLanguage } from "@/contexts/LanguageContext";
import { useAuth } from "@/contexts/AuthContext";
import { useDashboardTheme } from "@/contexts/DashboardThemeContext";
import twinbidLogo from "@/assets/twinbid-logo.svg";
import twinbidMark from "@/assets/twinbid-mark.svg";

export function PublisherSidebar() {
  const navigate = useNavigate();
  const location = useLocation();
  const { t } = useLanguage();
  const { signOut } = useAuth();
  const { theme } = useDashboardTheme();

  const menuItems = [
    { icon: LayoutDashboard, label: t("sidebar.overview"), path: "/publisher/overview" },
    { icon: BarChart3, label: t("pub.sidebar.statistics"), path: "/publisher/statistics" },
    { icon: Globe, label: t("pub.sidebar.sites"), path: "/publisher/sites" },
    { icon: Code2, label: t("pub.sidebar.adCodes"), path: "/publisher/ad-codes" },
    { icon: Wallet, label: t("pub.sidebar.payouts"), path: "/publisher/payouts" },
    { icon: Settings, label: t("pub.sidebar.settings"), path: "/publisher/settings" },
  ];

  return (
    <>
    <aside className="hidden lg:flex w-64 shrink-0 bg-card border-r border-border flex-col sticky top-0 h-screen">
      <div className="p-6 border-b border-border">
        {theme === "light" ? (
          <button type="button" onClick={() => navigate("/publisher/overview")} className="flex items-center gap-2.5" aria-label="TwinBid">
            <img src={twinbidMark} alt="" className="h-7 w-7 shrink-0" />
            <span className="text-xl font-semibold tracking-tight text-foreground">TwinBid</span>
          </button>
        ) : (
          <img src={twinbidLogo} alt="TwinBid" className="h-9 cursor-pointer" onClick={() => navigate("/publisher/overview")} />
        )}
      </div>
      <nav className="flex-1 p-4 space-y-2">
        {menuItems.map((item) => {
          const isActive = location.pathname === item.path
            || location.pathname.startsWith(`${item.path}/`);
          return (
            <button
              key={item.path}
              onClick={() => navigate(item.path)}
              className={cn(
                "w-full flex items-center gap-3 px-4 py-3 rounded-lg text-left transition-colors",
                isActive ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground",
              )}
            >
              <item.icon className="h-5 w-5" />
              <span>{item.label}</span>
            </button>
          );
        })}
      </nav>
      <div className="p-4 border-t border-border">
        <button
          onClick={async () => { await signOut(); navigate("/"); }}
          className="w-full flex items-center gap-3 px-4 py-3 rounded-lg text-muted-foreground hover:bg-destructive/10 hover:text-destructive transition-colors"
        >
          <LogOut className="h-5 w-5" /><span>{t("sidebar.logout")}</span>
        </button>
      </div>
    </aside>
    <nav className="fixed inset-x-0 bottom-0 z-40 grid grid-cols-5 border-t border-border bg-card/95 px-1 pt-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] backdrop-blur lg:hidden">
      {menuItems.filter((item) => item.path !== "/publisher/settings").map((item) => {
        const active = location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);
        return <button key={item.path} onClick={() => navigate(item.path)} className={cn("flex min-w-0 flex-col items-center gap-1 rounded-md px-0.5 py-1 text-[9px] leading-tight sm:text-[10px]", active ? "text-primary" : "text-muted-foreground")}><item.icon className="h-4 w-4 shrink-0" /><span className="max-w-full text-center">{item.label}</span></button>;
      })}
    </nav>
    </>
  );
}
