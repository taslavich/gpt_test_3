import { ArrowDown, ArrowDownRight, Send } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { SignalFlowField } from "./SignalFlowField";
import { useLanguage } from "@/contexts/LanguageContext";
import { TypedValue } from "./TypedValue";

export function HeroSection() {
  const { t, lang } = useLanguage();
  const title1 = t("hero.title1").trim();
  const title2 = t("hero.title2").trim();

  return (
    <section className="landing-obsidian-hero" aria-labelledby="landing-hero-title">
      <SignalFlowField />
      <div className="landing-hero-shade" aria-hidden="true" />
      <div className="landing-hero-noise" aria-hidden="true" />

      <div className="landing-editorial-shell landing-obsidian-content">
        <p className="landing-obsidian-kicker">
          <span />
          {t("hero.badge")}
        </p>

        <h1
          id="landing-hero-title"
          className={`landing-obsidian-title ${lang === "fr" ? "landing-obsidian-title-fr" : ""}`}
        >
          <span className="landing-title-row"><span>{title1}</span></span>
          <span className="landing-title-row landing-title-row-accent"><span>{title2}</span></span>
        </h1>

        <div className="landing-obsidian-bottom">
          <p>
            {t("hero.subtitle")} <strong>{t("hero.subtitleSites")}</strong> {t("hero.subtitleEnd")}
          </p>
          <div className="landing-obsidian-actions">
            <AuthDialog
              defaultTab="register"
              trigger={
                <button className="landing-button landing-button-primary landing-hero-cta">
                  {t("hero.cta")}
                  <ArrowDownRight aria-hidden="true" />
                </button>
              }
            />
            <a className="landing-hero-telegram" href="https://t.me/twinbid" target="_blank" rel="noopener noreferrer">
              {t("hero.telegram")}
              <Send aria-hidden="true" />
            </a>
          </div>
        </div>

        <div className="landing-obsidian-metric" aria-label={`1M+ ${t("hero.statSites")}`}>
          <strong><TypedValue value="1M+" /></strong>
          <span>{t("hero.statSites")}<br />global inventory</span>
        </div>
      </div>

      <a className="landing-hero-scroll" href="#signal-proof" aria-label="Scroll">
        <span>{lang === "ru" ? "Листайте дальше" : lang === "es" ? "Sigue explorando" : lang === "fr" ? "Continuer" : "Follow the signal"}</span>
        <ArrowDown aria-hidden="true" />
      </a>
    </section>
  );
}
