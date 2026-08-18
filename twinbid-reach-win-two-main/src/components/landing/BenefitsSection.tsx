import { LayoutDashboard, TrendingUp, Eye, ShieldCheck, Brain, type LucideIcon } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { motion } from "framer-motion";
import { WordsReveal, LineReveal } from "./CinematicReveal";
import { useIsMobileImmediate } from "@/hooks/use-mobile";

const benefitIcons = [LayoutDashboard, TrendingUp, Eye, ShieldCheck, Brain];

function StatsChart() {
  const isMobile = useIsMobileImmediate();
  const bars = [34, 52, 43, 70, 61, 86, 74, 104, 92, 126, 112, 142];

  return (
    <div className="relative mb-8 h-40 overflow-hidden rounded-[20px] border border-white/[0.08] bg-black/20 p-4 md:h-44">
      <div className="absolute inset-0 bg-[radial-gradient(circle_at_70%_0%,hsl(var(--primary)/0.12),transparent_48%)]" />
      <svg viewBox="0 0 640 176" className="relative h-full w-full" preserveAspectRatio="none" aria-hidden>
        <defs>
          <linearGradient id="statsArea" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="hsl(168 70% 55%)" stopOpacity="0.32" />
            <stop offset="100%" stopColor="hsl(168 70% 55%)" stopOpacity="0" />
          </linearGradient>
          <linearGradient id="statsBars" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="hsl(168 70% 55%)" stopOpacity="0.62" />
            <stop offset="100%" stopColor="hsl(168 70% 55%)" stopOpacity="0.08" />
          </linearGradient>
        </defs>
        {[32, 80, 128].map((y) => (
          <line key={y} x1="0" x2="640" y1={y} y2={y} stroke="rgba(255,255,255,0.08)" strokeDasharray="4 8" />
        ))}
        {bars.map((height, index) => (
          <motion.rect
            key={index}
            x={index * 53 + 8}
            y={168 - height}
            width="27"
            height={height}
            rx="5"
            fill="url(#statsBars)"
            initial={isMobile ? false : { opacity: 0, scaleY: 0 }}
            whileInView={isMobile ? undefined : { opacity: 1, scaleY: 1 }}
            viewport={{ once: true }}
            transition={{ duration: 0.55, delay: index * 0.035 }}
            style={{ transformOrigin: "bottom" }}
          />
        ))}
        <path d="M20 145 C80 130, 98 136, 150 110 S230 124, 280 82 S362 96, 414 55 S510 70, 620 22 L620 176 L20 176 Z" fill="url(#statsArea)" />
        <motion.path
          d="M20 145 C80 130, 98 136, 150 110 S230 124, 280 82 S362 96, 414 55 S510 70, 620 22"
          fill="none"
          stroke="hsl(168 70% 60%)"
          strokeWidth="3"
          strokeLinecap="round"
          initial={isMobile ? false : { pathLength: 0, opacity: 0 }}
          whileInView={isMobile ? undefined : { pathLength: 1, opacity: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 1.25, ease: [0.22, 1, 0.36, 1] }}
        />
        <circle cx="620" cy="22" r="5" fill="hsl(168 70% 60%)" />
        <circle cx="620" cy="22" r="11" fill="none" stroke="hsl(168 70% 60% / 0.3)" />
      </svg>
    </div>
  );
}

export function BenefitsSection() {
  const { t } = useLanguage();

  const benefits = benefitIcons.map((icon, i) => ({
    icon,
    title: t(`benefits.${i + 1}.title`),
    description: t(`benefits.${i + 1}.desc`),
  }));

  const headlineRaw = `${t("benefits.title1")}TwinBid${t("benefits.title2")}`;

  return (
    <section id="benefits" className="landing-section relative">
      <div className="container mx-auto px-5 md:px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="mb-14 grid items-end gap-8 md:grid-cols-12 md:mb-20">
            <div className="md:col-span-7">
              <LineReveal>
                <div className="landing-kicker mb-7">02 / {t("nav.benefits")}</div>
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

          <div className="grid gap-4 md:grid-cols-12">
            {benefits.map((benefit, index) => (
              <BenefitRow key={index} benefit={benefit} index={index} />
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

function BenefitRow({ benefit, index }: { benefit: { icon: LucideIcon; title: string; description: string }; index: number }) {
  const isMobile = useIsMobileImmediate();
  const spans = ["md:col-span-7 md:row-span-2", "md:col-span-5", "md:col-span-5", "md:col-span-6", "md:col-span-6"];
  return (
    <motion.div
      initial={isMobile ? false : { opacity: 0, y: 20 }}
      whileInView={isMobile ? undefined : { opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.7, delay: index * 0.05, ease: [0.22, 1, 0.36, 1] }}
      className={`landing-card group relative flex min-h-[240px] flex-col overflow-hidden p-7 md:p-9 ${spans[index]}`}
    >
      <div className={`${index === 0 ? "mb-5" : "mb-8"} flex items-center justify-between`}>
        <span className="font-mono-eyebrow text-[10px] tracking-[0.22em] text-muted-foreground">
          0{index + 1}
        </span>
        <div className="flex h-11 w-11 items-center justify-center rounded-full border border-white/10 bg-white/[0.025] transition-colors group-hover:border-primary/50 group-hover:bg-primary/[0.08]">
          <benefit.icon className="w-[18px] h-[18px] text-primary" strokeWidth={1.4} />
        </div>
      </div>
      {index === 0 && <StatsChart />}
      <h3 className={`${index === 0 ? "md:text-5xl" : "md:text-3xl"} font-display text-3xl font-light leading-[1.08] tracking-tight text-foreground`}>
        {benefit.title}
      </h3>
      <p className="mt-5 max-w-xl text-[15px] leading-relaxed text-muted-foreground">
        {benefit.description}
      </p>
    </motion.div>
  );
}
