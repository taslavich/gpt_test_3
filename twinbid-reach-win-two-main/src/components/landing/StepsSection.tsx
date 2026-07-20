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
    <section id="steps" className="landing-section relative">
      <div className="container mx-auto px-5 md:px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="mb-14 grid items-end gap-8 md:grid-cols-12 md:mb-20">
            <div className="md:col-span-7">
              <LineReveal>
                <div className="landing-kicker mb-7">04 / GET STARTED</div>
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

          <div className="relative grid gap-4 md:grid-cols-2">
            {steps.map((step, index) => (
              <motion.div
                key={index}
                initial={{ opacity: 0, y: 30 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true, margin: "-80px" }}
                transition={{ duration: 0.8, delay: index * 0.08, ease: [0.22, 1, 0.36, 1] }}
                className="landing-card group relative min-h-[310px] overflow-hidden p-7 md:p-9"
              >
                <div className="mb-14 flex items-start justify-between">
                  <div className="font-display text-7xl font-extralight leading-none tracking-tight text-white/[0.12] transition-colors duration-500 group-hover:text-primary/50 md:text-8xl">
                    {step.number}
                  </div>
                  <div className="flex h-12 w-12 items-center justify-center rounded-full border border-white/10 bg-white/[0.025]">
                    <step.icon className="h-5 w-5 text-primary" strokeWidth={1.4} />
                  </div>
                </div>
                <h3 className="font-display text-3xl font-light leading-[1.1] tracking-tight text-foreground md:text-4xl">
                  {step.title}
                </h3>
                <p className="mt-5 max-w-lg text-[15px] leading-relaxed text-muted-foreground">{step.description}</p>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
