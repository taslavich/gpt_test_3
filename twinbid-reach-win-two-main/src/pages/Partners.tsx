import { useMemo, useState } from "react";
import { ArrowRight, Info } from "lucide-react";
import { MotionConfig } from "framer-motion";
import { Header } from "@/components/landing/Header";
import { Footer } from "@/components/landing/Footer";
import { AnimatedBackground } from "@/components/landing/AnimatedBackground";
import { AuthDialog } from "@/components/landing/AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";
import { useIsMobileImmediate } from "@/hooks/use-mobile";
import { calculatePartnerEarnings } from "@/lib/partnerEarningsCalculator";

const STEPS = [1, 2, 3, 4] as const;
const TWINBID_HANDLES = ["onboarding", "launch", "support", "optimization", "technical"] as const;

const LOCALES = {
  ru: "ru-RU",
  en: "en-US",
  es: "es-ES",
  fr: "fr-FR",
} as const;

export default function Partners() {
  const { t, lang } = useLanguage();
  const isMobile = useIsMobileImmediate();
  const [affiliatePayout, setAffiliatePayout] = useState(50_000);
  const [roi, setRoi] = useState(25);
  const [mediaBuyers, setMediaBuyers] = useState(5);

  const result = useMemo(() => calculatePartnerEarnings({
    monthlyAffiliatePayout: affiliatePayout,
    roiPercent: roi,
    mediaBuyers,
  }), [affiliatePayout, mediaBuyers, roi]);

  const money = useMemo(() => new Intl.NumberFormat(LOCALES[lang], {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }), [lang]);

  return (
    <MotionConfig reducedMotion={isMobile ? "always" : "never"}>
      <div className="landing-shell partners-b2b-page min-h-screen bg-background">
        <Header />
        <main>
          <section className="partners-b2b-hero" aria-labelledby="partners-b2b-title">
            <AnimatedBackground />
            <div className="partners-b2b-shell partners-b2b-hero-inner">
              <div className="partners-b2b-hero-copy">
                <p className="partners-b2b-kicker"><span />TwinBid Partners</p>
                <h1 id="partners-b2b-title">{t("partners.b2b.hero.title")}</h1>
                <p className="partners-b2b-lead">{t("partners.b2b.hero.subtitle")}</p>

                <div className="partners-b2b-hero-actions">
                  <PartnerRegistrationButton label={t("partners.b2b.hero.cta")} />
                  <span>{t("partners.public.note")}</span>
                </div>
              </div>

              <aside className="partners-b2b-share" aria-label={t("partners.public.modelLabel")}>
                <div className="partners-b2b-share-top">
                  <span>{t("partners.public.modelLabel")}</span>
                  <span>01 / 01</span>
                </div>
                <strong>50%</strong>
                <p>{t("partners.public.modelText")}</p>
              </aside>

              <div className="partners-b2b-signals" aria-label={t("partners.b2b.hero.signalsLabel")}>
                <div><strong>50%</strong><span>{t("partners.b2b.hero.metricProfit")}</span></div>
                <div><strong>{t("partners.b2b.hero.metricLifetimeValue")}</strong><span>{t("partners.b2b.hero.metricLifetime")}</span></div>
                <div><strong>{t("partners.b2b.hero.metricRealtimeValue")}</strong><span>{t("partners.b2b.hero.metricRealtime")}</span></div>
              </div>

              <div className="partners-b2b-handled">
                <div>
                  <p className="partners-b2b-label">{t("partners.b2b.handled.eyebrow")}</p>
                  <h2>{t("partners.b2b.handled.title")}</h2>
                </div>
                <ul>
                  {TWINBID_HANDLES.map((item, index) => (
                    <li key={item}><span>0{index + 1}</span>{t(`partners.b2b.handled.${item}`)}</li>
                  ))}
                </ul>
              </div>
            </div>
          </section>

          <section className="partners-b2b-section partners-b2b-how" aria-labelledby="partners-how-title">
            <div className="partners-b2b-shell">
              <SectionHeading
                eyebrow={t("partners.b2b.how.eyebrow")}
                title={t("partners.b2b.how.title")}
                description={t("partners.b2b.how.description")}
                id="partners-how-title"
              />
              <ol className="partners-b2b-steps">
                {STEPS.map((step) => (
                  <li key={step}>
                    <span>0{step}</span>
                    <h3>{t(`partners.b2b.how.step${step}Title`)}</h3>
                    <p>{t(`partners.b2b.how.step${step}Text`)}</p>
                  </li>
                ))}
              </ol>
            </div>
          </section>

          <section className="partners-b2b-section partners-b2b-mechanics" aria-label={t("partners.b2b.earnings.title")}>
            <div className="partners-b2b-shell partners-b2b-mechanics-grid">
              <article className="partners-b2b-realtime">
                <p className="partners-b2b-label">{t("partners.b2b.earnings.eyebrow")}</p>
                <h2>{t("partners.b2b.earnings.title")}</h2>
                <p>{t("partners.b2b.earnings.text")}</p>
                <div className="partners-b2b-income-flow">
                  <span>{t("partners.b2b.earnings.flowSpend")}</span>
                  <ArrowRight aria-hidden="true" />
                  <span>{t("partners.b2b.earnings.flowProfit")}</span>
                  <ArrowRight aria-hidden="true" />
                  <strong>{t("partners.b2b.earnings.flowShare")}</strong>
                </div>
              </article>

              <article className="partners-b2b-payouts">
                <p className="partners-b2b-label">{t("partners.b2b.payouts.eyebrow")}</p>
                <h2>{t("partners.b2b.payouts.title")}</h2>
                <p>{t("partners.b2b.payouts.text")}</p>
                <ul>
                  <li><span>01</span>{t("partners.b2b.payouts.request")}</li>
                  <li><span>02</span>{t("partners.b2b.payouts.daily")}</li>
                  <li><span>03</span>{t("partners.b2b.payouts.crypto")}</li>
                  <li><span>04</span>{t("partners.b2b.payouts.balance")}</li>
                </ul>
              </article>
            </div>
          </section>

          <section className="partners-b2b-section partners-b2b-calculator-section" aria-labelledby="partners-calculator-title">
            <div className="partners-b2b-shell">
              <SectionHeading
                eyebrow={t("partners.b2b.calculator.eyebrow")}
                title={t("partners.b2b.calculator.title")}
                description={t("partners.b2b.calculator.description")}
                id="partners-calculator-title"
              />

              <div className="partners-b2b-calculator">
                <form className="partners-b2b-calculator-fields" onSubmit={(event) => event.preventDefault()}>
                  <CalculatorField
                    id="affiliate-payout"
                    label={t("partners.b2b.calculator.payout")}
                    prefix="$"
                    value={affiliatePayout}
                    min={0}
                    step={1000}
                    onChange={setAffiliatePayout}
                  />
                  <CalculatorField
                    id="media-buyer-roi"
                    label={t("partners.b2b.calculator.roi")}
                    suffix="%"
                    value={roi}
                    min={0}
                    max={500}
                    step={1}
                    onChange={setRoi}
                    hint={t("partners.b2b.calculator.roiHint")}
                  />
                  <CalculatorField
                    id="media-buyers"
                    label={t("partners.b2b.calculator.buyers")}
                    value={mediaBuyers}
                    min={1}
                    max={10_000}
                    step={1}
                    onChange={setMediaBuyers}
                  />
                  <p className="partners-b2b-calculator-formula">
                    <Info aria-hidden="true" />
                    {t("partners.b2b.calculator.formula")}
                  </p>
                </form>

                <div className="partners-b2b-calculator-result" role="status" aria-live="polite">
                  <div className="partners-b2b-result-supporting">
                    <div>
                      <span>{t("partners.b2b.calculator.trafficSpend")}</span>
                      <strong>{money.format(result.trafficSpend)} <small>/ {t("partners.b2b.calculator.month")}</small></strong>
                    </div>
                    <div>
                      <span>{t("partners.b2b.calculator.twinbidProfit")}</span>
                      <strong>{money.format(result.twinbidProfit)} <small>/ {t("partners.b2b.calculator.month")}</small></strong>
                    </div>
                  </div>
                  <div className="partners-b2b-result-main">
                    <span>{t("partners.b2b.calculator.partnerIncome")}</span>
                    <strong>{money.format(result.partnerIncome)}</strong>
                    <small>/ {t("partners.b2b.calculator.month")}</small>
                  </div>
                  <div className="partners-b2b-result-annual">
                    <span>{t("partners.b2b.calculator.annualIncome")}</span>
                    <strong>{money.format(result.annualPartnerIncome)} / {t("partners.b2b.calculator.year")}</strong>
                  </div>
                  <p>{t("partners.b2b.calculator.disclaimer")}</p>
                </div>
              </div>
            </div>
          </section>

          <section className="partners-b2b-section partners-b2b-example" aria-labelledby="partners-example-title">
            <div className="partners-b2b-shell">
              <SectionHeading
                eyebrow={t("partners.b2b.example.eyebrow")}
                title={t("partners.b2b.example.title")}
                id="partners-example-title"
              />
              <dl className="partners-b2b-example-grid">
                <div><dt>{t("partners.b2b.example.spend")}</dt><dd>$30,000</dd></div>
                <div><dt>{t("partners.b2b.example.profit")}</dt><dd>$6,000</dd></div>
                <div className="is-accent"><dt>{t("partners.b2b.example.share")}</dt><dd>$3,000</dd></div>
                <div><dt>{t("partners.b2b.example.annual")}</dt><dd>$36,000</dd></div>
              </dl>
              <p className="partners-b2b-lifetime">{t("partners.b2b.example.lifetime")}</p>
            </div>
          </section>

          <section className="partners-b2b-final">
            <div className="partners-b2b-shell partners-b2b-final-inner">
              <div>
                <p className="partners-b2b-label">TwinBid Partners</p>
                <h2>{t("partners.public.finalTitle")}</h2>
                <p>{t("partners.public.finalSubtitle")}</p>
              </div>
              <PartnerRegistrationButton label={t("partners.public.finalCta")} />
            </div>
          </section>
        </main>
        <Footer />
      </div>
    </MotionConfig>
  );
}

function SectionHeading({
  eyebrow,
  title,
  description,
  id,
}: {
  eyebrow: string;
  title: string;
  description?: string;
  id: string;
}) {
  return (
    <header className="partners-b2b-section-heading">
      <p className="partners-b2b-label">{eyebrow}</p>
      <div>
        <h2 id={id}>{title}</h2>
        {description && <p>{description}</p>}
      </div>
    </header>
  );
}

function CalculatorField({
  id,
  label,
  prefix,
  suffix,
  value,
  min,
  max,
  step,
  hint,
  onChange,
}: {
  id: string;
  label: string;
  prefix?: string;
  suffix?: string;
  value: number;
  min: number;
  max?: number;
  step: number;
  hint?: string;
  onChange: (value: number) => void;
}) {
  return (
    <label className="partners-b2b-field" htmlFor={id}>
      <span>{label}</span>
      <div>
        {prefix && <span>{prefix}</span>}
        <input
          id={id}
          type="number"
          inputMode="decimal"
          min={min}
          max={max}
          step={step}
          value={value}
          aria-describedby={hint ? `${id}-hint` : undefined}
          onChange={(event) => onChange(Number(event.target.value))}
        />
        {suffix && <span>{suffix}</span>}
      </div>
      {hint && <small id={`${id}-hint`}>{hint}</small>}
    </label>
  );
}

function PartnerRegistrationButton({ label }: { label: string }) {
  return (
    <AuthDialog
      defaultTab="register"
      trigger={(
        <button className="partners-b2b-button">
          {label}
          <ArrowRight aria-hidden="true" />
        </button>
      )}
    />
  );
}
