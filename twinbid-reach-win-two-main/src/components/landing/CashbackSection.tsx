import { DollarSign, TrendingUp, Zap } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { motion } from "framer-motion";
import { WordsReveal, LineReveal } from "./CinematicReveal";
import { useIsMobileImmediate } from "@/hooks/use-mobile";

const tiers = [
  { min: 1000, percent: 3, icon: DollarSign },
  { min: 5000, percent: 5, icon: TrendingUp },
  { min: 10000, percent: 10, icon: Zap },
];

export function CashbackSection() {
  const { t } = useLanguage();
  const isMobile = useIsMobileImmediate();

  return (
    <section className="landing-section relative">
      <div className="container mx-auto px-5 md:px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="landing-panel landing-panel-coral overflow-hidden p-6 md:p-10 lg:p-14">
          <div className="mb-16 grid items-end gap-8 md:grid-cols-12">
            <div className="md:col-span-7">
            <LineReveal>
              <div className="landing-kicker mb-7">03 / CASHBACK</div>
            </LineReveal>
            <WordsReveal
              as="h2"
              text={t("cashback.title")}
              className="text-display block text-foreground"
              stagger={0.05}
            />
            </div>
            <LineReveal delay={0.4} className="md:col-span-5">
              <p className="text-lg text-muted-foreground">{t("cashback.subtitle")}</p>
            </LineReveal>
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            {tiers.map((tier, i) => (
              <motion.div
                key={tier.min}
                initial={isMobile ? false : { opacity: 0, y: 30 }}
                whileInView={isMobile ? undefined : { opacity: 1, y: 0 }}
                viewport={{ once: true, margin: "-60px" }}
                transition={{ duration: 0.8, delay: i * 0.1, ease: [0.22, 1, 0.36, 1] }}
                className={`group rounded-[26px] border border-white/10 bg-background/70 p-7 backdrop-blur-sm transition-transform duration-500 hover:-translate-y-1 md:p-9 ${i === 1 ? "md:translate-y-5" : i === 2 ? "md:translate-y-10" : ""}`}
              >
                <div className="flex items-center justify-between mb-10">
                  <span className="font-mono-eyebrow text-[11px] tracking-[0.22em] text-muted-foreground">
                    TIER · 0{i + 1}
                  </span>
                  <tier.icon className="w-4 h-4 text-accent opacity-70" strokeWidth={1.4} />
                </div>
                <div className="font-display font-light text-foreground leading-none tracking-tight mb-6">
                  <span className="block text-[86px] lg:text-[120px]">
                    {tier.percent}<span className="text-accent">%</span>
                  </span>
                </div>
                <div className="rule mb-5" />
                <p className="text-foreground text-base mb-1">
                  {t("cashback.from")}{" "}
                  <span className="font-mono-eyebrow tracking-wider">${tier.min.toLocaleString()}</span>
                </p>
                <p className="eyebrow !text-[10px] mt-2">{t("cashback.perWeek")}</p>
              </motion.div>
            ))}
          </div>
          <div className="h-0 md:h-10" />
          </div>
        </div>
      </div>
    </section>
  );
}
