import { ArrowDownRight, ArrowRight } from "lucide-react";
import type { CSSProperties } from "react";
import { motion } from "framer-motion";
import { AuthDialog } from "./AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";

function NetworkStage() {
  const formats = [
    {
      name: "Popunder",
      inset: "12%",
      duration: "34s",
      delay: "-2s",
    },
    {
      name: "Native",
      inset: "12%",
      duration: "34s",
      delay: "-11s",
    },
    {
      name: "Banner",
      inset: "25%",
      duration: "25s",
      delay: "-5s",
    },
    {
      name: "In-page Push",
      inset: "25%",
      duration: "25s",
      delay: "-17s",
    },
  ];

  return (
    <motion.div
      initial={{ opacity: 0, y: 28, scale: 0.97 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      transition={{
        duration: 1,
        delay: 0.25,
        ease: [0.22, 1, 0.36, 1],
      }}
      className="landing-network-stage relative mx-auto aspect-square w-full max-w-[620px] overflow-hidden rounded-[40px] border border-white/10"
      aria-hidden
    >
      <div className="landing-global-grid absolute inset-0 opacity-50" />

      <div className="landing-orbit landing-orbit-slow absolute inset-[12%] rounded-full border border-dashed border-white/20" />

      <div className="landing-orbit absolute inset-[25%] rounded-full border border-dashed border-primary/45" />

      <div className="landing-orbit landing-orbit-reverse absolute inset-[37%] rounded-full border border-white/20" />

      <div className="absolute left-1/2 top-1/2 z-10 flex h-32 w-32 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border border-primary/35 bg-background/90 shadow-[0_0_90px_hsl(var(--primary)/0.26)] backdrop-blur-xl md:h-40 md:w-40">
        <span className="font-display text-xl font-medium tracking-[-0.04em] text-foreground md:text-2xl">
          TwinBid
        </span>
      </div>

      {formats.map((format, index) => (
        <div
          key={format.name}
          className="landing-format-orbit absolute z-20 rounded-full"
          style={
            {
              inset: format.inset,
              "--orbit-duration": format.duration,
              "--orbit-delay": format.delay,
            } as CSSProperties
          }
        >
          <div className="landing-format-anchor absolute left-1/2 top-0">
            <div className="landing-format-counter">
              <motion.div
                initial={{ opacity: 0, scale: 0.75 }}
                animate={{ opacity: 1, scale: 1 }}
                transition={{
                  duration: 0.6,
                  delay: 0.55 + index * 0.12,
                }}
                className="landing-data-chip whitespace-nowrap rounded-full border border-white/15 bg-background/90 px-3 py-2 text-[8px] font-mono-eyebrow uppercase tracking-[0.14em] text-foreground/90 backdrop-blur-xl sm:px-4 sm:text-[10px]"
              >
                <span className="mr-2 inline-block h-1.5 w-1.5 rounded-full bg-primary shadow-[0_0_12px_hsl(var(--primary))]" />

                {format.name}
              </motion.div>
            </div>
          </div>
        </div>
      ))}
    </motion.div>
  );
}

export function HeroSection() {
  const { t, lang } = useLanguage();

  const title1 = t("hero.title1").trim();
  const title2 = t("hero.title2").trim();

  return (
    <section className="relative flex min-h-[100svh] items-center overflow-hidden pb-16 pt-32 md:pb-20 md:pt-36">
      <div className="landing-hero-glow pointer-events-none absolute left-[12%] top-[18%] h-72 w-72 rounded-full bg-primary/10 blur-[100px]" />

      <div className="relative z-10 mx-auto grid w-full max-w-[1400px] items-center gap-12 px-5 md:px-8 lg:grid-cols-[1.04fr_0.96fr] lg:gap-14">
        <div className="max-w-[760px]">
          <motion.div
            initial={{ opacity: 0, y: 14 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.7 }}
            className="landing-kicker mb-7 inline-flex items-center gap-3 rounded-full border border-primary/20 bg-primary/[0.06] px-4 py-2"
          >
            <span className="h-1.5 w-1.5 rounded-full bg-primary shadow-[0_0_12px_hsl(var(--primary))]" />

            <span>{t("hero.badge")}</span>
          </motion.div>

          <motion.h1
            initial={{ opacity: 0, y: 30 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{
              duration: 0.9,
              delay: 0.08,
              ease: [0.22, 1, 0.36, 1],
            }}
            className={`landing-hero-title text-foreground ${
              lang === "ru" ? "landing-hero-title-ru" : ""
            }`}
          >
            <span className="block">{title1}</span>

            <span className="landing-outline-text block">
              {title2}
            </span>
          </motion.h1>

          <motion.p
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{
              duration: 0.8,
              delay: 0.2,
            }}
            className="mt-7 max-w-xl text-base leading-relaxed text-muted-foreground md:text-lg"
          >
            {t("hero.subtitle")}{" "}

            <span className="text-foreground">
              {t("hero.subtitleSites")}
            </span>{" "}

            {t("hero.subtitleEnd")}
          </motion.p>

          <motion.div
            initial={{ opacity: 0, y: 18 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{
              duration: 0.8,
              delay: 0.3,
            }}
            className="mt-8 flex flex-col gap-3 sm:flex-row"
          >
            <AuthDialog
              defaultTab="register"
              trigger={
                <button className="landing-button landing-button-primary justify-center px-7 py-4 text-[14px]">
                  {t("hero.cta")}

                  <ArrowRight className="h-4 w-4" />
                </button>
              }
            />

            <a
              href="#benefits"
              className="landing-button landing-button-ghost justify-center px-7 py-4 text-[14px]"
            >
              {t("hero.learnMore")}

              <ArrowDownRight className="h-4 w-4" />
            </a>
          </motion.div>
        </div>

        <NetworkStage />
      </div>
    </section>
  );
}
