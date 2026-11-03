import { Header } from "@/components/landing/Header";
import { LandingIntro } from "@/components/landing/LandingIntro";
import { HeroSection } from "@/components/landing/HeroSection";
import { SignalProof } from "@/components/landing/SignalProof";
import { PlatformStory } from "@/components/landing/PlatformStory";
import { StartConditions } from "@/components/landing/StartConditions";
import { TrafficCalculatorSection } from "@/components/landing/TrafficCalculatorSection";
import { FormatsSection } from "@/components/landing/FormatsSection";
import { PartnersLandingSection } from "@/components/landing/PartnersLandingSection";
import { CTASection } from "@/components/landing/CTASection";
import { Footer } from "@/components/landing/Footer";
import { AnimatedBackground } from "@/components/landing/AnimatedBackground";
import { useIsMobileImmediate } from "@/hooks/use-mobile";
import { MotionConfig } from "framer-motion";
import { useLocation } from "react-router-dom";
import { useRef } from "react";
import { RolePortalReveal, useRolePortalTransition } from "@/components/landing/RolePortalTransition";

const Index = () => {
  const isMobile = useIsMobileImmediate();
  const location = useLocation();
  const { openPortal, portalOverlay, transitioning } = useRolePortalTransition();
  const fromPublisher = useRef(Boolean((location.state as { rolePortalTransition?: boolean } | null)?.rolePortalTransition)).current;

  return (
    <MotionConfig reducedMotion={isMobile ? "always" : "never"}>
      <div className={`landing-shell min-h-screen bg-background relative ${fromPublisher ? "role-portal-arrival" : ""}`}>
        {fromPublisher ? <RolePortalReveal /> : <LandingIntro />}
        <AnimatedBackground />
        <Header onPublisher={(event) => openPortal(event, "/publisher")} transitioning={transitioning} />
        <main>
          <HeroSection onPublisher={(event) => openPortal(event, "/publisher")} transitioning={transitioning} />
          <SignalProof />
          <PlatformStory />
          <TrafficCalculatorSection />
          <PartnersLandingSection />
          <FormatsSection />
          <StartConditions />
          <CTASection />
        </main>
        <Footer />
        {portalOverlay}
      </div>
    </MotionConfig>
  );
};

export default Index;
