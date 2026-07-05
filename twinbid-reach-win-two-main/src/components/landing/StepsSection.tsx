import { UserPlus, Target, Wallet, Rocket } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { motion } from "framer-motion";
import { WordsReveal, LineReveal } from "./CinematicReveal";

const stepIcons = [UserPlus, Target, Wallet, Rocket];

export function StepsSection() {
  const { t } = useLanguage();

  const steps = stepIcons.map((icon, i) => ({
    icon,
    number: String(i + 1).padStart(2, "0"),
    title: t(`steps.${i + 1}.title`),
    description: t(`steps.${i + 1}.desc`),
  }));

  return (
    <section id="steps" className="py-[140px] relative">
      <div className="container mx-auto px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="grid md:grid-cols-12 gap-8 mb-24 items-end">
            <div className="md:col-span-7">
              <LineReveal>
                <div className="eyebrow mb-8">— 05 / GET STARTED</div>
              </LineReveal>
              <WordsReveal
                as="h2"
                text={t("steps.title1") + t("steps.title2")}
                className="text-display-xl block text-foreground"
                stagger={0.05}
              />
            </div>
            <div className="md:col-span-5">
              <LineReveal delay={0.3}>
                <p className="text-lg text-muted-foreground leading-relaxed">{t("steps.subtitle")}</p>
              </LineReveal>
            </div>
          </div>

          <div className="rule mb-0" />

          <div className="divide-y divide-border">
            {steps.map((step, index) => (
              <motion.div
                key={index}
                initial={{ opacity: 0, y: 30 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true, margin: "-80px" }}
                transition={{ duration: 0.8, delay: index * 0.08, ease: [0.22, 1, 0.36, 1] }}
                className="grid md:grid-cols-12 gap-8 py-14 items-center group"
              >
                <div className="md:col-span-3">
                  <div className="font-display text-7xl md:text-8xl font-extralight text-foreground group-hover:text-primary transition-colors duration-500 leading-none tracking-tight">
                    {step.number}
                  </div>
                </div>
                <h3 className="md:col-span-5 font-display text-3xl md:text-4xl font-light text-foreground leading-[1.1] tracking-tight">
                  {step.title}
                </h3>
                <div className="md:col-span-4 flex items-start gap-4">
                  <step.icon className="w-5 h-5 text-primary mt-1 shrink-0" strokeWidth={1.4} />
                  <p className="text-muted-foreground text-[15px] leading-relaxed">{step.description}</p>
                </div>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
