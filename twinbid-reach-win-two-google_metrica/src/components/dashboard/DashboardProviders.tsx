import { CampaignProvider } from "@/contexts/CampaignContext";
import { NotificationProvider } from "@/contexts/NotificationContext";
import { PendingPaymentProvider } from "@/contexts/PendingPaymentContext";
import { ProfileProvider } from "@/contexts/ProfileContext";
import { StatisticsProvider } from "@/contexts/StatisticsContext";
import { PendingPaymentDialog } from "@/components/PendingPaymentDialog";
import { YandexTopupGoalTracker } from "@/components/YandexTopupGoalTracker";
import Dashboard from "@/pages/Dashboard";

/** Providers that are only needed inside the authenticated cabinet. */
export default function DashboardProviders() {
  return (
    <ProfileProvider>
      <NotificationProvider>
        <PendingPaymentProvider>
          <CampaignProvider>
            <StatisticsProvider>
              <Dashboard />
              <PendingPaymentDialog />
              <YandexTopupGoalTracker />
            </StatisticsProvider>
          </CampaignProvider>
        </PendingPaymentProvider>
      </NotificationProvider>
    </ProfileProvider>
  );
}
