import { Toaster } from "@/components/ui/toaster";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { lazy, Suspense } from "react";
import { LanguageProvider } from "./contexts/LanguageContext";
import { AuthProvider } from "./contexts/AuthContext";
import { ProtectedRoute } from "./components/ProtectedRoute";
import { YandexMetrikaTracker } from "./components/YandexMetrikaTracker";
import { LoginIntroOverlay } from "./components/dashboard/LoginIntroOverlay";
import Index from "./pages/Index";

// Keep the public landing lightweight. Dashboard pages (including media
// editing and FFmpeg) are downloaded only after the user opens their route.
const NotFound = lazy(() => import("./pages/NotFound"));
const DashboardProviders = lazy(() => import("./components/dashboard/DashboardProviders"));
const DashboardOverview = lazy(() => import("./pages/DashboardOverview"));
const DashboardCampaigns = lazy(() => import("./pages/DashboardCampaigns"));
const DashboardStatistics = lazy(() => import("./pages/DashboardStatistics"));
const TrafficCalculator = lazy(() => import("./pages/TrafficCalculator"));
const DashboardBalance = lazy(() => import("./pages/DashboardBalance"));
const DashboardSettings = lazy(() => import("./pages/DashboardSettings"));
const CreateCampaign = lazy(() => import("./pages/CreateCampaign"));
const EditCampaign = lazy(() => import("./pages/EditCampaign"));
const Verify = lazy(() => import("./pages/Verify"));
const Legal = lazy(() => import("./pages/Legal"));
const TwinBidMoney = lazy(() => import("./pages/TwinBidMoney"));

const queryClient = new QueryClient();

const RouteFallback = () => (
  <div className="min-h-screen flex items-center justify-center bg-background">
    <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-primary" />
  </div>
);

const App = () => (
  <QueryClientProvider client={queryClient}>
    <TooltipProvider>
      <Toaster />
      <Sonner />
      <BrowserRouter>
        <YandexMetrikaTracker />
        <LanguageProvider>
          <AuthProvider>
            <LoginIntroOverlay />
            <Suspense fallback={<RouteFallback />}>
              <Routes>
                <Route path="/" element={<Index />} />
                <Route path="/verify" element={<Verify />} />
                <Route path="/legal" element={<Legal />} />
                <Route path="/twinbid_money" element={<TwinBidMoney />} />
                <Route path="/twinbid/twinbid_money" element={<TwinBidMoney />} />
                <Route path="/dashboard" element={<ProtectedRoute><DashboardProviders /></ProtectedRoute>}>
                  <Route index element={<DashboardOverview />} />
                  <Route path="campaigns" element={<DashboardCampaigns />} />
                  <Route path="campaigns/create" element={<CreateCampaign />} />
                  <Route path="campaigns/:id/edit" element={<EditCampaign />} />
                  <Route path="statistics" element={<DashboardStatistics />} />
                  <Route path="traffic-calculator" element={<TrafficCalculator />} />
                  <Route path="balance" element={<DashboardBalance />} />
                  <Route path="settings" element={<DashboardSettings />} />
                </Route>
                <Route path="*" element={<NotFound />} />
              </Routes>
            </Suspense>
          </AuthProvider>
        </LanguageProvider>
      </BrowserRouter>
    </TooltipProvider>
  </QueryClientProvider>
);

export default App;
