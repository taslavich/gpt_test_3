import { ArrowRight } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";
import { WordsReveal, LineReveal } from "./CinematicReveal";

export function CTASection() {
  const { t } = useLanguage();

  return (
    <section className="relative py-[180px] frame-immersive">
      <div className="container mx-auto px-8">
        <div className="max-w-[1280px] mx-auto text-center">
          <LineReveal>
            <div className="eyebrow eyebrow-rule inline-flex mb-10">
              <span>{t("cta.badge")}</span>
            </div>
          </LineReveal>

          <WordsReveal
            as="h2"
            text={t("cta.title")}
            className="text-monument block text-foreground"
            stagger={0.06}
            duration={1.1}
          />

          <LineReveal delay={0.6} className="mt-12 max-w-2xl mx-auto">
            <p className="text-lg md:text-xl text-muted-foreground">{t("cta.subtitle")}</p>
          </LineReveal>

          <LineReveal delay={0.85} className="mt-12 flex flex-col sm:flex-row items-center justify-center gap-3">
            <AuthDialog
              defaultTab="register"
              trigger={
                <button className="pill pill-primary px-8 py-4 text-[14px]">
                  {t("cta.register")} <ArrowRight className="w-4 h-4" />
                </button>
              }
            />
            <a
              href="https://t.me/GregTwinbid"
              target="_blank"
              rel="noopener noreferrer"
              className="pill pill-ghost px-8 py-4 text-[14px]"
            >
              {t("cta.contact")}
            </a>
          </LineReveal>

          <LineReveal delay={1.05} className="mt-20">
            <div className="rule max-w-md mx-auto mb-6" />
            <div className="mobile-no-dots flex flex-wrap items-center justify-center gap-x-10 gap-y-2 text-muted-foreground text-[12px] font-mono-eyebrow tracking-[0.2em] uppercase">
              <span>{t("cta.trust1")}</span>
              <span className="w-1 h-1 rounded-full bg-border" />
              <span>{t("cta.trust2")}</span>
              <span className="w-1 h-1 rounded-full bg-border" />
              <span>{t("cta.trust3")}</span>
            </div>
          </LineReveal>
        </div>
      </div>
    </section>
  );
}
