import { DollarSign, TrendingUp, Zap } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { motion } from "framer-motion";
import { WordsReveal, LineReveal } from "./CinematicReveal";

const tiers = [
  { min: 1000, percent: 3, icon: DollarSign },
  { min: 5000, percent: 5, icon: TrendingUp },
  { min: 10000, percent: 10, icon: Zap },
];

export function CashbackSection() {
  const { t } = useLanguage();

  return (
    <section className="relative py-[140px] frame-coral">
      <div className="container mx-auto px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="text-center mb-24">
            <LineReveal>
              <div className="eyebrow mb-8 inline-block">— 03 / CASHBACK</div>
            </LineReveal>
            <WordsReveal
              as="h2"
              text={t("cashback.title")}
              className="text-display-xl block text-foreground"
              stagger={0.05}
            />
            <LineReveal delay={0.4} className="mt-8 max-w-xl mx-auto">
              <p className="text-lg text-muted-foreground">{t("cashback.subtitle")}</p>
            </LineReveal>
          </div>

          <div className="grid md:grid-cols-3 gap-px bg-border">
            {tiers.map((tier, i) => (
              <motion.div
                key={tier.min}
                initial={{ opacity: 0, y: 30 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true, margin: "-60px" }}
                transition={{ duration: 0.8, delay: i * 0.1, ease: [0.22, 1, 0.36, 1] }}
                className="bg-background p-10 md:p-12 group hover:bg-secondary/30 transition-colors duration-500"
              >
                <div className="flex items-center justify-between mb-10">
                  <span className="font-mono-eyebrow text-[11px] tracking-[0.22em] text-muted-foreground">
                    TIER · 0{i + 1}
                  </span>
                  <tier.icon className="w-4 h-4 text-accent opacity-70" strokeWidth={1.4} />
                </div>
                <div className="font-display font-light text-foreground leading-none tracking-tight mb-6">
                  <span className="text-[96px] md:text-[140px] block">
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
        </div>
      </div>
    </section>
  );
}
