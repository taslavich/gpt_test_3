import { Gift } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";
import { WordsReveal, LineReveal } from "./CinematicReveal";
import { motion } from "framer-motion";
import { useIsMobileImmediate } from "@/hooks/use-mobile";

export function StartConditions() {
  const { t } = useLanguage();
  const isMobile = useIsMobileImmediate();

  return (
    <section className="landing-section relative">
      <div className="container mx-auto px-5 md:px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="landing-panel landing-panel-mint grid overflow-hidden md:grid-cols-12">
            <div className="relative border-b border-white/[0.08] p-7 md:col-span-7 md:border-b-0 md:border-r md:p-12 lg:p-16">
              <div className="pointer-events-none absolute right-0 top-0 hidden h-56 w-56 rounded-full bg-primary/10 blur-[80px] md:block" />
              <LineReveal>
                <div className="landing-kicker mb-12">01 / {t("start.conditionsLabel")}</div>
              </LineReveal>
              <motion.div
                initial={isMobile ? false : { opacity: 0, scale: 0.96 }}
                whileInView={isMobile ? undefined : { opacity: 1, scale: 1 }}
                viewport={{ once: true, margin: "-100px" }}
                transition={{ duration: 1.1, ease: [0.22, 1, 0.36, 1] }}
                className="relative font-display font-extralight text-foreground leading-[0.82] tracking-tight"
              >
                <span className="block text-[110px] sm:text-[150px] lg:text-[190px]">
                  <span className="text-primary">$</span>100
                </span>
              </motion.div>
              <LineReveal delay={0.4}>
                <div className="mb-6 mt-9 h-px max-w-md bg-gradient-to-r from-primary/70 to-transparent" />
                <p className="mb-2 text-xl text-foreground md:text-2xl">{t("start.minDeposit")}</p>
                <p className="max-w-md text-base leading-relaxed text-muted-foreground">{t("start.startSmall")}</p>
              </LineReveal>
            </div>

            <div className="flex items-stretch p-4 md:col-span-5 md:p-6 lg:p-8">
              <motion.div
                initial={isMobile ? false : { opacity: 0, y: 30 }}
                whileInView={isMobile ? undefined : { opacity: 1, y: 0 }}
                viewport={{ once: true, margin: "-80px" }}
                transition={{ duration: 0.9, delay: 0.2, ease: [0.22, 1, 0.36, 1] }}
                className="flex w-full flex-col justify-between rounded-[28px] border border-white/10 bg-black/20 p-7 shadow-[inset_0_1px_0_rgba(255,255,255,0.05)] md:p-9"
              >
                <div className="mb-12 flex items-center gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-full border border-primary/25 bg-primary/[0.08]">
                    <Gift className="h-4 w-4 text-primary" strokeWidth={1.4} />
                  </div>
                  <span className="landing-kicker">{t("start.bonusBadge")}</span>
                </div>
                <WordsReveal
                  text="+25%"
                  className="mb-7 block font-display text-[82px] font-extralight leading-none tracking-tight text-foreground md:text-[108px]"
                  stagger={0.04}
                />
                <div className="mb-6 h-px bg-white/10" />
                <p className="mb-8 text-[14px] leading-relaxed text-muted-foreground">{t("start.bonusDesc")}</p>
                <AuthDialog
                  defaultTab="register"
                  trigger={<button className="landing-button landing-button-primary w-full justify-center py-3.5">{t("start.getBonus")}</button>}
                />
              </motion.div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
