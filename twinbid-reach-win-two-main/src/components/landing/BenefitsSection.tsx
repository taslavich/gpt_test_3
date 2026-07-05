import { LayoutDashboard, TrendingUp, Eye, ShieldCheck, Brain } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { motion } from "framer-motion";
import { WordsReveal, LineReveal } from "./CinematicReveal";

const benefitIcons = [LayoutDashboard, TrendingUp, Eye, ShieldCheck, Brain];

export function BenefitsSection() {
  const { t } = useLanguage();

  const benefits = benefitIcons.map((icon, i) => ({
    icon,
    title: t(`benefits.${i + 1}.title`),
    description: t(`benefits.${i + 1}.desc`),
  }));

  const headlineRaw = `${t("benefits.title1")}TwinBid${t("benefits.title2")}`;

  return (
    <section id="benefits" className="py-[140px] relative">
      <div className="container mx-auto px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="grid md:grid-cols-12 gap-8 mb-20 items-end">
            <div className="md:col-span-7">
              <LineReveal>
                <div className="eyebrow mb-8">— 02 / {t("nav.benefits")}</div>
              </LineReveal>
              <WordsReveal
                as="h2"
                text={headlineRaw}
                className="text-display-xl block text-foreground"
                stagger={0.05}
                brandWord="TwinBid"
                brandClass="text-primary"
              />
            </div>
            <div className="md:col-span-5">
              <LineReveal delay={0.3}>
                <p className="text-lg text-muted-foreground leading-relaxed">{t("benefits.subtitle")}</p>
              </LineReveal>
            </div>
          </div>

          <div className="rule mb-12" />

          <div className="divide-y divide-border">
            {benefits.map((benefit, index) => (
              <BenefitRow key={index} benefit={benefit} index={index} />
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

function BenefitRow({ benefit, index }: { benefit: { icon: any; title: string; description: string }; index: number }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.7, delay: index * 0.05, ease: [0.22, 1, 0.36, 1] }}
      className="grid md:grid-cols-12 gap-8 py-10 group"
    >
      <div className="md:col-span-2 flex items-start gap-4">
        <span className="font-mono-eyebrow text-[11px] tracking-[0.22em] text-muted-foreground pt-2">
          0{index + 1}
        </span>
        <div className="w-11 h-11 rounded-full border border-border flex items-center justify-center group-hover:border-primary/60 group-hover:bg-primary/5 transition-colors">
          <benefit.icon className="w-[18px] h-[18px] text-primary" strokeWidth={1.4} />
        </div>
      </div>
      <h3 className="md:col-span-5 font-display text-2xl md:text-3xl font-light text-foreground leading-[1.15] tracking-tight">
        {benefit.title}
      </h3>
      <p className="md:col-span-5 text-muted-foreground text-[15px] leading-relaxed">
        {benefit.description}
      </p>
    </motion.div>
  );
}
