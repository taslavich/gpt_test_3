import { ArrowRight } from "lucide-react";
import { Link } from "react-router-dom";
import { useLanguage } from "@/contexts/LanguageContext";

export function PartnersLandingSection() {
  const { t } = useLanguage();
  const points = [t("partners.landing.point1"), t("partners.landing.point2"), t("partners.landing.point3")];

  return (
    <section id="partners" className="partners-spotlight-section" aria-labelledby="partners-spotlight-title">
      <div className="landing-editorial-shell partners-spotlight-inner">
        <header className="partners-spotlight-heading">
          <p className="partners-spotlight-label">TwinBid Partners</p>
          <div className="partners-spotlight-copy">
            <h2 id="partners-spotlight-title">{t("partners.public.title")}</h2>
            <p>{t("partners.landing.subtitle")}</p>
          </div>
        </header>

        <div className="partners-spotlight-layout">
          <div className="partners-spotlight-share" aria-label={t("partners.public.modelLabel")}>
            <span>{t("partners.public.modelLabel")}</span>
            <strong>50 / 50</strong>
            <p>{t("partners.public.modelText")}</p>
          </div>

          <div className="partners-spotlight-content">
            <ol className="partners-spotlight-points">
              {points.map((point, index) => (
                <li key={point}><span>0{index + 1}</span><p>{point}</p></li>
              ))}
            </ol>

            <div className="partners-spotlight-result">
              <dl>
                <div><dt>{t("partners.landing.turnover")}</dt><dd>$30,000</dd></div>
                <div><dt>{t("partners.landing.profit")}</dt><dd>$6,000</dd></div>
                <div className="is-accent"><dt>{t("partners.landing.share")}</dt><dd>$3,000</dd></div>
              </dl>
              <Link to="/partners" className="partners-spotlight-link">
                {t("partners.landing.cta")}
                <ArrowRight aria-hidden="true" />
              </Link>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
