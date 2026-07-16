import { ArrowRight } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";

export function HeroSection() {
  const { t } = useLanguage();
  const title1 = t("hero.title1").trim();
  const title2 = t("hero.title2");
  const words = `${title1}\n${title2}`.split("\n").map((s) => s.trim()).filter(Boolean);

  return (
    <section className="relative min-h-[100svh] flex flex-col justify-center pt-16 pb-10 overflow-hidden">
      <div className="absolute inset-0 blueprint-grid blueprint-mask pointer-events-none hidden md:block" />
      <div className="blueprint-aurora hidden md:block" />

      <div className="relative container mx-auto px-6 z-10">
        <div className="flex justify-center mb-5">
          <div className="eyebrow eyebrow-rule inline-flex">
            <span>{t("hero.badge")}</span>
          </div>
        </div>

        <h1 className="text-hero-monument text-center text-foreground">
          {words.map((w, i) => (
            <span
              key={i}
              className={`block md:whitespace-nowrap break-words ${i === 0 ? "" : "gradient-text"}`}
            >
              {w}
            </span>
          ))}
        </h1>

        <div className="mt-6 max-w-2xl mx-auto text-center">
          <p className="text-base md:text-lg text-muted-foreground leading-relaxed">
            {t("hero.subtitle")}{" "}
            <span className="text-foreground">{t("hero.subtitleSites")}</span>{" "}
            {t("hero.subtitleEnd")}
          </p>
        </div>

        <div className="mt-5 flex flex-col sm:flex-row items-center justify-center gap-3">
          <AuthDialog
            defaultTab="register"
            trigger={
              <button className="pill pill-primary px-7 py-3.5 text-[14px]">
                {t("hero.cta")} <ArrowRight className="w-4 h-4" />
              </button>
            }
          />
          <a href="#benefits" className="pill pill-ghost px-7 py-3.5 text-[14px]">
            {t("hero.learnMore")}
          </a>
        </div>
      </div>
    </section>
  );
}
