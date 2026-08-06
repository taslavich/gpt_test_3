import { DashboardSidebar } from "@/components/dashboard/DashboardSidebar";
import { DashboardHeader } from "@/components/dashboard/DashboardHeader";
import { Outlet } from "react-router-dom";
import { DashboardThemeProvider } from "@/contexts/DashboardThemeContext";

export default function Dashboard() {
  return (
    <DashboardThemeProvider>
      <div className="dashboard-root flex min-h-screen min-w-0 bg-background">
        <DashboardSidebar />
        <div className="min-h-screen min-w-0 flex-1">
          <DashboardHeader />
          <main className="min-w-0 overflow-x-clip p-3 sm:p-4 lg:p-6">
            <Outlet />
          </main>
        </div>
      </div>
    </DashboardThemeProvider>
  );
}
