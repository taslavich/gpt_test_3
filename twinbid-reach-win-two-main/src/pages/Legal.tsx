import { useEffect, useMemo } from "react";
import { Link, useLocation } from "react-router-dom";
import { ArrowLeft, ArrowUpRight, FileText, Mail, ShieldCheck } from "lucide-react";
import { Footer } from "@/components/landing/Footer";
import { LanguageSelector } from "@/components/LanguageSelector";
import { useLanguage } from "@/contexts/LanguageContext";
import { LEGAL_CONTENT } from "@/lib/legalContent";
import twinbidLogo from "@/assets/twinbid-logo.svg";

export default function Legal() {
  const { lang, t } = useLanguage();
  const { hash } = useLocation();
  // Legal documents are currently available in Russian and English; other UI languages use English.
  const content = LEGAL_CONTENT[lang === "ru" ? "ru" : "en"];

  useEffect(() => {
    if (hash) {
      const element = document.getElementById(hash.slice(1));
      if (element) {
        window.setTimeout(() => element.scrollIntoView({ behavior: "smooth", block: "start" }), 80);
        return;
      }
    }
    window.scrollTo({ top: 0 });
  }, [hash]);

  const sections = useMemo(
    () => [
      { id: "terms", index: "01", icon: FileText, title: content.terms.title, blocks: content.terms.sections },
      { id: "privacy", index: "02", icon: ShieldCheck, title: content.privacy.title, blocks: content.privacy.sections },
    ],
    [content],
  );

  return (
    <div className="landing-shell min-h-screen overflow-x-clip bg-background text-foreground selection:bg-primary/25">
      <div className="pointer-events-none fixed inset-0" aria-hidden="true">
        <div className="absolute left-1/2 top-[-24rem] h-[48rem] w-[48rem] -translate-x-1/2 rounded-full bg-primary/[0.08] blur-[150px]" />
        <div className="absolute inset-x-0 top-0 h-[34rem] bg-[linear-gradient(180deg,hsl(200_18%_5%/0.12),transparent)]" />
      </div>

      <header className="fixed inset-x-0 top-0 z-50 px-3 pt-3 md:px-6 md:pt-5">
        <div className="landing-header-panel mx-auto flex h-[60px] max-w-[1320px] items-center justify-between rounded-[14px] border border-white/10 bg-[hsl(200_18%_7%/0.92)] px-4 shadow-[0_18px_70px_rgba(0,0,0,0.4)] backdrop-blur-xl md:px-5">
          <Link to="/" className="shrink-0" aria-label="TwinBid">
            <img src={twinbidLogo} alt="TwinBid" className="h-9 md:h-10" />
          </Link>

          <div className="hidden items-center gap-2 font-mono text-[10px] uppercase tracking-[0.2em] text-muted-foreground sm:flex">
            <span className="h-1.5 w-1.5 rounded-full bg-primary" />
            {t("legal.pageTitle")}
          </div>

          <div className="flex items-center gap-2">
            <LanguageSelector className="landing-language-trigger" />
            <Link to="/" className="landing-header-login hidden sm:inline-flex">
              <ArrowLeft className="h-3.5 w-3.5" />
              {t("legal.back")}
            </Link>
          </div>
        </div>
      </header>

      <main className="relative z-10 mx-auto max-w-[1180px] px-4 pb-24 pt-32 sm:px-6 md:pt-40">
        <div className="max-w-4xl">
          <div className="mb-5 flex items-center gap-3 font-mono text-[10px] font-semibold uppercase tracking-[0.22em] text-primary sm:text-[11px]">
            <span className="h-px w-9 bg-primary/60" />
            TwinBid · Legal
          </div>
          <h1 className="max-w-4xl text-balance text-[clamp(2.8rem,7vw,6.4rem)] font-medium leading-[0.92] tracking-[-0.06em]">
            {t("legal.pageTitle")}
          </h1>
          <p className="mt-6 max-w-2xl text-base leading-7 text-muted-foreground sm:text-lg">
            {t("legal.pageSubtitle")}
          </p>
        </div>

        <div className="mt-14 grid gap-3 sm:grid-cols-2 md:mt-20">
          {sections.map(({ id, index, icon: Icon, title }) => (
            <a
              key={id}
              href={`#${id}`}
              className="group flex min-h-28 items-center gap-5 border border-white/[0.09] bg-white/[0.025] p-5 transition-colors hover:border-primary/40 hover:bg-primary/[0.035] sm:p-6"
            >
              <div className="flex h-11 w-11 shrink-0 items-center justify-center border border-primary/20 bg-primary/[0.07] text-primary">
                <Icon className="h-5 w-5" />
              </div>
              <div className="min-w-0 flex-1">
                <div className="font-mono text-[9px] uppercase tracking-[0.2em] text-muted-foreground">{index} / TwinBid</div>
                <div className="mt-1 text-lg font-medium leading-tight">{title}</div>
              </div>
              <ArrowUpRight className="h-4 w-4 shrink-0 text-muted-foreground transition group-hover:-translate-y-0.5 group-hover:translate-x-0.5 group-hover:text-primary" />
            </a>
          ))}
        </div>

        <div className="mt-20 grid gap-12 lg:grid-cols-[220px_minmax(0,1fr)] lg:gap-16">
          <aside className="hidden lg:block">
            <div className="sticky top-28 border-l border-white/10 pl-5">
              <div className="mb-5 font-mono text-[9px] uppercase tracking-[0.2em] text-muted-foreground">Documents</div>
              <nav className="space-y-1">
                {sections.map(({ id, index, title }) => (
                  <a key={id} href={`#${id}`} className="group flex items-start gap-3 py-2 text-sm text-muted-foreground transition hover:text-foreground">
                    <span className="font-mono text-[9px] text-primary/70">{index}</span>
                    <span>{title}</span>
                  </a>
                ))}
              </nav>
            </div>
          </aside>

          <div className="min-w-0">
            {sections.map(({ id, index, icon: Icon, title, blocks }, sectionIndex) => (
              <section key={id} id={id} className="mb-24 scroll-mt-28 last:mb-0">
                <div className="mb-8 border-b border-white/10 pb-7">
                  <div className="mb-4 flex items-center gap-3 font-mono text-[10px] uppercase tracking-[0.2em] text-primary">
                    <Icon className="h-4 w-4" />
                    {index} / TwinBid
                  </div>
                  <h2 className="text-balance text-3xl font-medium leading-tight tracking-[-0.04em] sm:text-5xl">{title}</h2>
                </div>

                <div className="divide-y divide-white/[0.08] border-y border-white/[0.08]">
                  {blocks.map((block, blockIndex) => (
                    <article key={`${id}-${blockIndex}`} className="grid gap-4 py-6 sm:grid-cols-[44px_minmax(0,1fr)] sm:gap-6 sm:py-8">
                      <div className="font-mono text-[9px] text-primary/55">{String(blockIndex + 1).padStart(2, "0")}</div>
                      <div className="min-w-0">
                        {block.heading ? <h3 className="mb-4 text-lg font-medium tracking-[-0.02em] text-foreground sm:text-xl">{block.heading}</h3> : null}
                        {block.paragraphs?.map((paragraph, paragraphIndex) => (
                          <p key={paragraphIndex} className="mb-3 whitespace-pre-line text-sm leading-7 text-muted-foreground last:mb-0 sm:text-[15px]">
                            {paragraph}
                          </p>
                        ))}
                        {block.list ? (
                          <ol className="mt-4 space-y-3">
                            {block.list.map((item, itemIndex) => (
                              <li key={itemIndex} className="grid grid-cols-[24px_minmax(0,1fr)] gap-3 text-sm leading-7 text-muted-foreground sm:text-[15px]">
                                <span className="font-mono text-[9px] text-primary/65">{String(itemIndex + 1).padStart(2, "0")}</span>
                                <span>{item}</span>
                              </li>
                            ))}
                          </ol>
                        ) : null}
                      </div>
                    </article>
                  ))}
                </div>

                {sectionIndex < sections.length - 1 ? (
                  <a href={`#${sections[sectionIndex + 1].id}`} className="mt-7 inline-flex items-center gap-2 text-sm text-primary transition hover:text-primary/80">
                    {sections[sectionIndex + 1].title}
                    <ArrowUpRight className="h-3.5 w-3.5" />
                  </a>
                ) : null}
              </section>
            ))}
          </div>
        </div>

        <div className="mt-20 flex flex-col justify-between gap-5 border border-primary/20 bg-primary/[0.04] p-6 sm:flex-row sm:items-center sm:p-8">
          <div>
            <div className="font-mono text-[9px] uppercase tracking-[0.2em] text-primary">TwinBid support</div>
            <p className="mt-2 text-sm text-muted-foreground">{t("legal.contactText")}</p>
          </div>
          <a href="mailto:twinbid@twinbidex.com" className="landing-button landing-button-ghost inline-flex min-h-12 items-center justify-center gap-2 px-5 text-sm">
            <Mail className="h-4 w-4" />
            twinbid@twinbidex.com
          </a>
        </div>
      </main>

      <Footer />
    </div>
  );
}
