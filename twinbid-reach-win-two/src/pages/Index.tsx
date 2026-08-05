import { Header } from "@/components/landing/Header";
import { HeroSection } from "@/components/landing/HeroSection";
import { StartConditions } from "@/components/landing/StartConditions";
import { BenefitsSection } from "@/components/landing/BenefitsSection";
import { CreativeToolsSection } from "@/components/landing/CreativeToolsSection";
import { TrafficCalculatorSection } from "@/components/landing/TrafficCalculatorSection";

import { FormatsSection } from "@/components/landing/FormatsSection";
import { StepsSection } from "@/components/landing/StepsSection";
import { CTASection } from "@/components/landing/CTASection";
import { Footer } from "@/components/landing/Footer";
import { Marquee } from "@/components/landing/LiveCanvas";
import { AnimatedBackground } from "@/components/landing/AnimatedBackground";
import { useLanguage } from "@/contexts/LanguageContext";
import { useIsMobile } from "@/hooks/use-mobile";
import { MotionConfig } from "framer-motion";

const Index = () => {
  const { t } = useLanguage();
  const isMobile = useIsMobile();
  const marquee1Items = [t("marquee.tryTwinBid"), t("marquee.registerNow")];
  return (
    <MotionConfig reducedMotion={isMobile ? "always" : "never"}>
      <div className="landing-shell min-h-screen bg-background relative overflow-x-hidden">
        <AnimatedBackground />
        <Header />
        <main>
          <HeroSection />
          <Marquee items={marquee1Items} />
          <StartConditions />
          <BenefitsSection />
          <TrafficCalculatorSection />
          <CreativeToolsSection />
          <Marquee items={["Popunder", "Native", "Banner", "In-Page Push", "1M+ Sites", "Antifraud", "24/7 Support", "Real-Time Bidding"]} />
          <FormatsSection />
          <StepsSection />
          <CTASection />
        </main>
        <Footer />
      </div>
    </MotionConfig>
  );
};

export default Index;
