import { PublisherSidebar } from "@/components/dashboard/PublisherSidebar";
import { PublisherHeader } from "@/components/dashboard/PublisherHeader";
import { PublisherProvider } from "@/contexts/PublisherContext";
import { DashboardThemeProvider } from "@/contexts/DashboardThemeContext";
import { Outlet } from "react-router-dom";

export default function PublisherDashboard() {
  return (
    <DashboardThemeProvider>
      <PublisherProvider>
        <div className="dashboard-root min-h-screen bg-background flex">
          <PublisherSidebar />
          <div className="min-w-0 flex-1 min-h-screen">
            <PublisherHeader />
            <main className="p-4 pb-24 sm:p-6 sm:pb-24 lg:pb-6">
              <Outlet />
            </main>
          </div>
        </div>
      </PublisherProvider>
    </DashboardThemeProvider>
  );
}
