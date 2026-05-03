import { useEffect, useMemo } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { ArrowLeft, FileText, ShieldCheck } from "lucide-react";
import { Header } from "@/components/landing/Header";
import { Footer } from "@/components/landing/Footer";
import { Button } from "@/components/ui/button";
import { useLanguage } from "@/contexts/LanguageContext";
import { LEGAL_CONTENT } from "@/lib/legalContent";

export default function Legal() {
  const { lang, t } = useLanguage();
  const navigate = useNavigate();
  const { hash } = useLocation();
  const content = LEGAL_CONTENT[lang];

  useEffect(() => {
    if (hash) {
      const el = document.getElementById(hash.slice(1));
      if (el) {
        setTimeout(() => el.scrollIntoView({ behavior: "smooth", block: "start" }), 80);
        return;
      }
    }
    window.scrollTo({ top: 0 });
  }, [hash]);

  const sections = useMemo(
    () => [
      { id: "terms", icon: FileText, title: content.terms.title, blocks: content.terms.sections },
      { id: "privacy", icon: ShieldCheck, title: content.privacy.title, blocks: content.privacy.sections },
    ],
    [content],
  );

  return (
    <div className="min-h-screen bg-background">
      <Header />
      <main className="container mx-auto px-4 pt-28 pb-16">
        <div className="max-w-4xl mx-auto">
          <Button
            variant="ghost"
            onClick={() => navigate("/")}
            className="mb-6 -ml-2 text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="h-4 w-4 mr-2" />
            {t("legal.back")}
          </Button>

          <div className="mb-10">
            <h1 className="text-4xl md:text-5xl font-bold tracking-tight">
              <span className="bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
                {t("legal.pageTitle")}
              </span>
            </h1>
            <p className="text-muted-foreground mt-3 text-lg">{t("legal.pageSubtitle")}</p>
          </div>

          {/* Quick nav */}
          <div className="grid sm:grid-cols-2 gap-4 mb-12">
            {sections.map((s) => (
              <a
                key={s.id}
                href={`#${s.id}`}
                className="group rounded-xl border border-border bg-card hover:border-primary/40 hover:bg-card/80 transition-colors p-5 flex items-center gap-4"
              >
                <div className="h-11 w-11 rounded-lg bg-primary/10 flex items-center justify-center text-primary group-hover:scale-105 transition-transform">
                  <s.icon className="h-5 w-5" />
                </div>
                <div>
                  <div className="text-sm text-muted-foreground">TwinBid</div>
                  <div className="font-semibold">{s.title}</div>
                </div>
              </a>
            ))}
          </div>

          {sections.map((s, idx) => (
            <section
              key={s.id}
              id={s.id}
              className="mb-16 scroll-mt-24"
            >
              <div className="flex items-center gap-3 mb-6 pb-4 border-b border-border">
                <div className="h-10 w-10 rounded-lg bg-gradient-to-br from-primary/20 to-accent/20 flex items-center justify-center text-primary">
                  <s.icon className="h-5 w-5" />
                </div>
                <h2 className="text-2xl md:text-3xl font-bold">{s.title}</h2>
              </div>

              <div className="space-y-6">
                {s.blocks.map((b, i) => (
                  <div key={i} className="rounded-xl bg-card/50 border border-border/60 p-5 md:p-6">
                    {b.heading && (
                      <h3 className="text-lg font-semibold mb-3 text-foreground">{b.heading}</h3>
                    )}
                    {b.paragraphs?.map((p, pi) => (
                      <p key={pi} className="text-muted-foreground leading-relaxed mb-3 last:mb-0 whitespace-pre-line">
                        {p}
                      </p>
                    ))}
                    {b.list && (
                      <ol className="list-decimal pl-5 space-y-2 text-muted-foreground marker:text-primary">
                        {b.list.map((li, li_i) => (
                          <li key={li_i} className="leading-relaxed">{li}</li>
                        ))}
                      </ol>
                    )}
                  </div>
                ))}
              </div>

              {idx < sections.length - 1 && (
                <div className="mt-10 flex justify-center">
                  <a
                    href={`#${sections[idx + 1].id}`}
                    className="text-sm text-primary hover:underline"
                  >
                    {sections[idx + 1].title} ↓
                  </a>
                </div>
              )}
            </section>
          ))}

          <div className="rounded-xl border border-border bg-card p-6 text-center">
            <p className="text-muted-foreground mb-2">{t("legal.contactText")}</p>
            <a href="#" onClick={(e) => e.preventDefault()} className="text-primary font-medium">
              twinbid@twinbidex.com
            </a>
          </div>
        </div>
      </main>
      <Footer />
    </div>
  );
}
