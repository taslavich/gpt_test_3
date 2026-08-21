import twinbidLogo from "@/assets/twinbid-logo.svg";
import { useLanguage } from "@/contexts/LanguageContext";

export function LandingIntro() {
  const { lang } = useLanguage();
  const tagline = {
    ru: "Новый подход к закупке трафика",
    en: "Performance traffic, reimagined",
    es: "Una nueva forma de comprar tráfico",
    fr: "Une nouvelle façon d’acheter du trafic",
  }[lang];

  return (
    <div className="landing-intro" aria-hidden="true">
      <div className="landing-intro-panel landing-intro-panel-obsidian" />
      <div className="landing-intro-panel landing-intro-panel-ink" />
      <div className="landing-intro-brand">
        <img src={twinbidLogo} alt="" />
        <span>{tagline}</span>
      </div>
    </div>
  );
}
