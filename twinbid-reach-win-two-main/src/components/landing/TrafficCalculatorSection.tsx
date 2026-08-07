import { ArrowRight, BarChart3, CircleDollarSign, Percent, Target } from "lucide-react";
import { motion } from "framer-motion";
import { AuthDialog } from "./AuthDialog";
import { LineReveal, WordsReveal } from "./CinematicReveal";
import { useLanguage } from "@/contexts/LanguageContext";
import { useIsMobileImmediate } from "@/hooks/use-mobile";

const copy = {
  ru: {
    title: "Узнайте, сколько трафика проходит мимо",
    subtitle: "TwinBid показывает доступный объём за последние полностью закрытые сутки, сравнивает его с результатами кампании и помогает понять потенциал масштабирования.",
    potential: "Доступные показы",
    received: "Показы кампании",
    share: "Полученная доля",
    bid: "Текущая ставка",
    insight: "Видите не только купленный трафик",
    description: "Выберите кампанию или задайте таргетинги вручную. Калькулятор покажет доступный объём показов, какую его долю получила кампания и позволит сразу изменить ставку.",
    cta: "Открыть калькулятор",
  },
  en: {
    title: "See how much traffic you are missing",
    subtitle: "TwinBid shows the available volume from the latest complete day, compares it with campaign results and reveals the room for scaling.",
    potential: "Available impressions",
    received: "Campaign impressions",
    share: "Share received",
    bid: "Current bid",
    insight: "See more than the traffic you bought",
    description: "Select a campaign or set targeting manually. The calculator shows the available impression volume, what share the campaign received, and lets you update the bid immediately.",
    cta: "Open calculator",
  },
  es: {
    title: "Descubre cuánto tráfico estás perdiendo",
    subtitle: "TwinBid muestra el volumen disponible del último día completo, lo compara con los resultados de la campaña y revela el potencial de crecimiento.",
    potential: "Impresiones disponibles",
    received: "Impresiones de campaña",
    share: "Cuota obtenida",
    bid: "Puja actual",
    insight: "Ve más que el tráfico comprado",
    description: "Selecciona una campaña o configura la segmentación. La calculadora muestra el volumen de impresiones disponible, qué cuota recibió la campaña y permite actualizar la puja al instante.",
    cta: "Abrir calculadora",
  },
};

const metrics = [
  { key: "potential" as const, value: "128 400", icon: Target },
  { key: "received" as const, value: "31 250", icon: BarChart3 },
  { key: "share" as const, value: "24,3%", icon: Percent },
  { key: "bid" as const, value: "$0.034", icon: CircleDollarSign },
];

function CalculatorMock({ text }: { text: typeof copy.en }) {
  const isMobile = useIsMobileImmediate();
  return (
    <div className="relative overflow-hidden rounded-[24px] border border-white/[0.09] bg-black/25 p-4 md:p-5">
      <div className="absolute inset-0 bg-[radial-gradient(circle_at_12%_10%,hsl(var(--primary)/0.14),transparent_42%)]" />
      <div className="relative flex items-center justify-between border-b border-white/[0.08] pb-4">
        <div className="flex items-center gap-2 font-mono-eyebrow text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
          <BarChart3 className="h-4 w-4 text-primary" strokeWidth={1.5} />
          Traffic calculator
        </div>
        <span className="h-2 w-2 rounded-full bg-primary shadow-[0_0_16px_hsl(var(--primary))]" />
      </div>

      <div className="relative mt-4 grid grid-cols-2 gap-2.5">
        {metrics.map(({ key, value, icon: Icon }, index) => (
          <motion.div
            key={key}
            initial={isMobile ? false : { opacity: 0, y: 14 }}
            whileInView={isMobile ? undefined : { opacity: 1, y: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.55, delay: index * 0.07 }}
            className="rounded-2xl border border-white/[0.075] bg-background/55 p-3.5 md:p-4"
          >
            <Icon className="h-4 w-4 text-primary" strokeWidth={1.5} />
            <p className="mt-4 text-[10px] leading-tight text-muted-foreground md:text-xs">{text[key]}</p>
            <p className="mt-1 font-display text-2xl font-light tracking-tight text-foreground md:text-3xl">{value}</p>
          </motion.div>
        ))}
      </div>

      <div className="relative mt-3 overflow-hidden rounded-2xl border border-white/[0.075] bg-background/45 px-4 py-4">
        <div className="flex items-center justify-between text-[10px] text-muted-foreground"><span>{text.received}</span><span>{text.potential}</span></div>
        <div className="mt-3 h-2 overflow-hidden rounded-full bg-white/[0.06]"><motion.div initial={isMobile ? { width: "24.3%" } : { width: 0 }} whileInView={isMobile ? undefined : { width: "24.3%" }} viewport={{ once: true }} transition={{ duration: 1, delay: 0.35 }} className="h-full rounded-full bg-primary shadow-[0_0_18px_hsl(var(--primary)/0.5)]" /></div>
      </div>
    </div>
  );
}

export function TrafficCalculatorSection() {
  const { lang } = useLanguage();
  const isMobile = useIsMobileImmediate();
  const text = copy[lang] ?? copy.en;

  return (
    <section id="traffic-calculator" className="landing-section landing-section-grid relative">
      <div className="container mx-auto px-5 md:px-8">
        <div className="mx-auto max-w-[1280px]">
          <div className="grid items-end gap-8 md:grid-cols-12">
            <div className="md:col-span-7">
              <LineReveal><div className="landing-kicker mb-7">TRAFFIC CALCULATOR</div></LineReveal>
              <WordsReveal as="h2" text={text.title} className="text-display block text-foreground" stagger={0.05} />
            </div>
            <div className="md:col-span-5"><LineReveal delay={0.3}><p className="text-lg leading-relaxed text-muted-foreground">{text.subtitle}</p></LineReveal></div>
          </div>

          <motion.div
            initial={isMobile ? false : { opacity: 0, y: 28 }}
            whileInView={isMobile ? undefined : { opacity: 1, y: 0 }}
            viewport={{ once: true, margin: "-60px" }}
            transition={{ duration: 0.8, ease: [0.22, 1, 0.36, 1] }}
            className="landing-panel landing-panel-mint mt-14 overflow-hidden p-5 md:p-8 lg:p-10"
          >
            <div className="grid items-center gap-10 lg:grid-cols-[1.08fr_0.92fr] lg:gap-16">
              <CalculatorMock text={text} />
              <div>
                <span className="inline-flex h-12 w-12 items-center justify-center rounded-full border border-primary/30 bg-primary/[0.08]"><Target className="h-5 w-5 text-primary" strokeWidth={1.5} /></span>
                <h3 className="mt-7 font-display text-4xl font-light leading-[1.05] tracking-tight text-foreground md:text-5xl">{text.insight}</h3>
                <p className="mt-6 text-[15px] leading-relaxed text-muted-foreground md:text-base">{text.description}</p>
                <AuthDialog
                  trigger={<button className="landing-button landing-button-primary mt-8 px-6 py-3.5 text-[13px]">{text.cta}<ArrowRight className="h-4 w-4" /></button>}
                  defaultTab="register"
                />
              </div>
            </div>
          </motion.div>
        </div>
      </div>
    </section>
  );
}
