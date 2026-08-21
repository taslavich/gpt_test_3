import { ArrowRight, Gift } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";

const sectionLabels = { ru: "05 / КАК НАЧАТЬ", en: "05 / GET STARTED", es: "05 / CÓMO EMPEZAR", fr: "05 / BIEN DÉMARRER" };

export function StartConditions() {
  const { t, lang } = useLanguage();
  const sectionLabel = sectionLabels[lang];
  const steps = [1, 2, 3, 4].map((index) => ({
    number: String(index).padStart(2, "0"),
    title: t(`steps.${index}.title`),
    description: t(`steps.${index}.desc`),
  }));

  return (
    <section id="steps" className="launch-section" aria-labelledby="launch-title">
      <div className="landing-editorial-shell">
        <div className="launch-heading">
          <p className="landing-section-index">{sectionLabel}</p>
          <h2 id="launch-title">{t("steps.title1")}{t("steps.title2")}</h2>
          <p>{t("steps.subtitle")}</p>
        </div>

        <div className="launch-grid">
          <div className="launch-offer">
            <div className="launch-deposit">
              <span>{t("start.conditionsLabel")}</span>
              <strong><i>$</i>100</strong>
              <p>{t("start.minDeposit")}. {t("start.startSmall")}</p>
            </div>
            <div className="launch-bonus">
              <span><Gift /> {t("start.bonusBadge")}</span>
              <strong>+25%</strong>
              <p>{t("start.bonusDesc")}</p>
            </div>
          </div>

          <div className="launch-steps">
            {steps.map((step) => (
              <article key={step.number}>
                <span>{step.number}</span>
                <div><h3>{step.title}</h3><p>{step.description}</p></div>
              </article>
            ))}
            <AuthDialog
              defaultTab="register"
              trigger={<button className="landing-button landing-button-primary launch-button">{t("start.getBonus")}<ArrowRight /></button>}
            />
          </div>
        </div>
      </div>
    </section>
  );
}
