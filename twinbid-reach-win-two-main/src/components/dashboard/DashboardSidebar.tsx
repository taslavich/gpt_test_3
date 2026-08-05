import { useState } from "react";
import { LayoutDashboard, Megaphone, Wallet, Settings, LogOut, BarChart3, Calculator, Menu } from "lucide-react";
import { cn } from "@/lib/utils";
import { useNavigate, useLocation } from "react-router-dom";
import { useLanguage } from "@/contexts/LanguageContext";
import { useAuth } from "@/contexts/AuthContext";
import { useDashboardTheme } from "@/contexts/DashboardThemeContext";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet";
import twinbidLogoDark from "@/assets/twinbid-logo.svg";
import twinbidMark from "@/assets/twinbid-mark.svg";

function useDashboardNavigation() {
  const navigate = useNavigate();
  const location = useLocation();
  const { t } = useLanguage();
  const { signOut } = useAuth();

  const menuItems = [
    { icon: LayoutDashboard, label: t("sidebar.overview"), path: "/dashboard" },
    { icon: Megaphone, label: t("sidebar.campaigns"), path: "/dashboard/campaigns" },
    { icon: BarChart3, label: t("sidebar.statistics"), path: "/dashboard/statistics" },
    { icon: Calculator, label: t("sidebar.trafficCalculator"), path: "/dashboard/traffic-calculator" },
    { icon: Wallet, label: t("sidebar.balance"), path: "/dashboard/balance" },
    { icon: Settings, label: t("sidebar.settings"), path: "/dashboard/settings" },
  ];

  const isActive = (path: string) => location.pathname === path
    || (path !== "/dashboard" && location.pathname.startsWith(path));

  return { navigate, t, signOut, menuItems, isActive };
}

function DashboardNavContent({ onNavigate, mobile = false }: { onNavigate?: () => void; mobile?: boolean }) {
  const { navigate, t, signOut, menuItems, isActive } = useDashboardNavigation();
  const { theme } = useDashboardTheme();

  const goTo = (path: string) => {
    navigate(path);
    onNavigate?.();
  };

  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className={cn("shrink-0 border-b border-border", mobile ? "flex h-16 items-center px-5" : "p-6")}>
        {theme === "light" ? (
          <button type="button" onClick={() => goTo("/dashboard")} className="flex items-center gap-2.5" aria-label="TwinBid">
            <img src={twinbidMark} alt="" className="h-7 w-7 shrink-0" />
            <span className="text-xl font-semibold tracking-tight text-foreground">TwinBid</span>
          </button>
        ) : (
          <img src={twinbidLogoDark} alt="TwinBid" className="h-9 cursor-pointer" onClick={() => goTo("/dashboard")} />
        )}
      </div>
      <nav className="min-h-0 flex-1 space-y-2 overflow-y-auto p-4">
        {menuItems.map((item) => (
          <button
            key={item.path}
            onClick={() => goTo(item.path)}
            className={cn(
              "flex w-full items-center gap-3 rounded-lg px-4 py-3 text-left transition-colors",
              isActive(item.path) ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground",
            )}
          >
            <item.icon className="h-5 w-5 shrink-0" />
            <span className="min-w-0 break-words">{item.label}</span>
          </button>
        ))}
      </nav>
      <div className="shrink-0 border-t border-border p-4">
        <button
          onClick={async () => { await signOut(); navigate("/"); onNavigate?.(); }}
          className="flex w-full items-center gap-3 rounded-lg px-4 py-3 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
        >
          <LogOut className="h-5 w-5 shrink-0" />
          <span>{t("sidebar.logout")}</span>
        </button>
      </div>
    </div>
  );
}

export function DashboardSidebar() {
  return (
    <aside className="hidden w-64 shrink-0 flex-col border-r border-border bg-card lg:flex">
      <DashboardNavContent />
    </aside>
  );
}

export function DashboardMobileNavigation() {
  const [open, setOpen] = useState(false);

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button variant="ghost" size="icon" className="shrink-0 lg:hidden" aria-label="Open navigation">
          <Menu className="h-5 w-5" />
        </Button>
      </SheetTrigger>
      <SheetContent side="left" className="w-[min(84vw,300px)] border-border bg-card p-0">
        <DashboardNavContent mobile onNavigate={() => setOpen(false)} />
      </SheetContent>
    </Sheet>
  );
}
