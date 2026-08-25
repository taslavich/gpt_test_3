import { ArrowRight } from "lucide-react";
import { MotionConfig } from "framer-motion";
import { Header } from "@/components/landing/Header";
import { AnimatedBackground } from "@/components/landing/AnimatedBackground";
import { AuthDialog } from "@/components/landing/AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";
import { useIsMobileImmediate } from "@/hooks/use-mobile";

const AUDIENCES = [1, 2, 3, 4] as const;

export default function Partners() {
  const { t } = useLanguage();
  const isMobile = useIsMobileImmediate();

  return (
    <MotionConfig reducedMotion={isMobile ? "always" : "never"}>
      <div className="landing-shell partners-onepage-shell min-h-screen bg-background">
        <Header />
        <main className="partners-onepage">
          <AnimatedBackground />
          <div className="landing-editorial-shell partners-onepage-inner">
            <header className="partners-onepage-hero">
              <div className="partners-onepage-copy">
                <p className="partners-onepage-label">TwinBid Partners</p>
                <h1>{t("partners.public.title")}</h1>
                <p>{t("partners.public.subtitle")}</p>
                <div className="partners-onepage-action">
                  <PartnerRegistrationButton label={t("partners.public.cta")} />
                  <span>{t("partners.public.note")}</span>
                </div>
              </div>

              <div className="partners-onepage-model" aria-label={t("partners.public.modelLabel")}>
                <div><span>{t("partners.public.modelLabel")}</span><span>01 / 01</span></div>
                <strong>50 / 50</strong>
                <p>{t("partners.public.modelText")}</p>
              </div>
            </header>

            <div className="partners-onepage-details">
              <section className="partners-onepage-flow">
                <p className="partners-onepage-label">{t("partners.details.howTitle")}</p>
                <ol>
                  <li><span>01</span><p>{t("partners.details.step1")} {t("partners.details.step2")}</p></li>
                  <li><span>02</span><p>{t("partners.details.step3")} {t("partners.details.step4")}</p></li>
                  <li><span>03</span><p>{t("partners.details.step5")} {t("partners.details.step6")}</p></li>
                </ol>
              </section>

              <section className="partners-onepage-audience">
                <p className="partners-onepage-label">{t("partners.public.audienceEyebrow")}</p>
                <div>
                  {AUDIENCES.map((item) => (
                    <article key={item}>
                      <span>0{item}</span>
                      <h2>{t(`partners.public.audience${item}Title`)}</h2>
                    </article>
                  ))}
                </div>
              </section>

              <section className="partners-onepage-example">
                <p className="partners-onepage-label">{t("partners.example.title")}</p>
                <dl>
                  <div><dt>{t("partners.example.turnover")}</dt><dd>$30,000</dd></div>
                  <div><dt>{t("partners.example.profit")}</dt><dd>$6,000</dd></div>
                  <div className="is-accent"><dt>{t("partners.example.share")}</dt><dd>$3,000</dd></div>
                </dl>
                <p>{t("partners.details.recurringText")}</p>
                <small>{t("partners.details.terms")}</small>
              </section>
            </div>
          </div>
        </main>
      </div>
    </MotionConfig>
  );
}

function PartnerRegistrationButton({ label }: { label: string }) {
  return (
    <AuthDialog
      defaultTab="register"
      trigger={(
        <button className="partners-onepage-button">
          {label}
          <ArrowRight aria-hidden="true" />
        </button>
      )}
    />
  );
}
