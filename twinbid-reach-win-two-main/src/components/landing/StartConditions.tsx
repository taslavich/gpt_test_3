import { Gift } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";
import { WordsReveal, LineReveal } from "./CinematicReveal";
import { motion } from "framer-motion";

export function StartConditions() {
  const { t } = useLanguage();

  return (
    <section className="py-[140px] relative">
      <div className="container mx-auto px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="grid md:grid-cols-12 gap-12 items-center">
            <div className="md:col-span-7">
              <LineReveal>
                <div className="eyebrow mb-8">— 01 / {t("start.conditionsLabel")}</div>
              </LineReveal>
              <motion.div
                initial={{ opacity: 0, scale: 0.96 }}
                whileInView={{ opacity: 1, scale: 1 }}
                viewport={{ once: true, margin: "-100px" }}
                transition={{ duration: 1.1, ease: [0.22, 1, 0.36, 1] }}
                className="font-display font-extralight text-foreground leading-[0.82] tracking-tight"
              >
                <span className="text-[120px] md:text-[200px] block">
                  <span className="text-primary">$</span>100
                </span>
              </motion.div>
              <LineReveal delay={0.4}>
                <div className="rule mt-8 mb-6 max-w-md" />
                <p className="text-foreground text-xl mb-2">{t("start.minDeposit")}</p>
                <p className="text-muted-foreground text-base max-w-md">{t("start.startSmall")}</p>
              </LineReveal>
            </div>

            <div className="md:col-span-5">
              <motion.div
                initial={{ opacity: 0, y: 30 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true, margin: "-80px" }}
                transition={{ duration: 0.9, delay: 0.2, ease: [0.22, 1, 0.36, 1] }}
                className="border border-border p-10"
              >
                <div className="flex items-center gap-3 mb-8">
                  <Gift className="w-4 h-4 text-primary" strokeWidth={1.4} />
                  <span className="eyebrow !text-[11px]">{t("start.bonusBadge")}</span>
                </div>
                <WordsReveal
                  text="+25%"
                  className="font-display font-extralight text-foreground leading-none tracking-tight text-[88px] md:text-[120px] block mb-6"
                  stagger={0.04}
                />
                <div className="rule mb-6" />
                <p className="text-muted-foreground text-[14px] leading-relaxed mb-8">{t("start.bonusDesc")}</p>
                <AuthDialog
                  defaultTab="register"
                  trigger={<button className="pill pill-primary w-full justify-center py-3.5">{t("start.getBonus")}</button>}
                />
              </motion.div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
