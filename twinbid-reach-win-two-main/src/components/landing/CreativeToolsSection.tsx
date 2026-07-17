import { Crop, Eye, Image as ImageIcon, Monitor, Move, Scan, Smartphone } from "lucide-react";
import { motion } from "framer-motion";
import { useLanguage } from "@/contexts/LanguageContext";
import { LineReveal, WordsReveal } from "./CinematicReveal";

function CropperMock() {
  return (
    <div className="relative aspect-[16/11] overflow-hidden rounded-xl border border-border/70 bg-background/70">
      <div className="flex h-10 items-center justify-between border-b border-border/70 px-4">
        <div className="flex items-center gap-2 text-[10px] font-mono-eyebrow uppercase tracking-[0.18em] text-muted-foreground">
          <Crop className="h-3.5 w-3.5 text-primary" strokeWidth={1.5} />
          300 × 250
        </div>
        <div className="flex items-center gap-1.5">
          <span className="h-1.5 w-1.5 rounded-full bg-primary/70" />
          <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/30" />
          <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/30" />
        </div>
      </div>

      <div className="absolute inset-x-5 bottom-5 top-14 overflow-hidden rounded-lg bg-muted/50">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_72%_26%,hsl(var(--accent)/0.38),transparent_32%),linear-gradient(135deg,hsl(var(--primary)/0.34),hsl(var(--secondary))_55%,hsl(var(--accent)/0.2))]" />
        <div className="absolute left-[18%] top-[13%] h-[73%] w-[64%] border border-white/90 shadow-[0_0_0_999px_rgba(0,0,0,0.45)]">
          <span className="absolute -left-1 -top-1 h-2.5 w-2.5 border-l-2 border-t-2 border-white" />
          <span className="absolute -right-1 -top-1 h-2.5 w-2.5 border-r-2 border-t-2 border-white" />
          <span className="absolute -bottom-1 -left-1 h-2.5 w-2.5 border-b-2 border-l-2 border-white" />
          <span className="absolute -bottom-1 -right-1 h-2.5 w-2.5 border-b-2 border-r-2 border-white" />
          <Move className="absolute left-1/2 top-1/2 h-5 w-5 -translate-x-1/2 -translate-y-1/2 text-white/80" strokeWidth={1.3} />
        </div>
      </div>
    </div>
  );
}

function PreviewMock() {
  return (
    <div className="relative aspect-[16/11] overflow-hidden rounded-xl border border-border/70 bg-background/70">
      <div className="flex h-10 items-center justify-between border-b border-border/70 px-4">
        <div className="flex items-center gap-2 text-[10px] font-mono-eyebrow uppercase tracking-[0.18em] text-muted-foreground">
          <Eye className="h-3.5 w-3.5 text-primary" strokeWidth={1.5} />
          Live preview
        </div>
        <div className="flex items-center gap-2 text-muted-foreground">
          <Monitor className="h-3.5 w-3.5 text-primary" strokeWidth={1.5} />
          <Smartphone className="h-3.5 w-3.5" strokeWidth={1.5} />
        </div>
      </div>

      <div className="absolute inset-x-5 bottom-5 top-14 rounded-lg border border-border/50 bg-card/70 p-4">
        <div className="mb-4 flex items-center gap-2">
          <span className="h-2 w-2 rounded-full bg-primary/70" />
          <span className="h-1.5 w-20 rounded-full bg-muted-foreground/20" />
          <span className="ml-auto h-1.5 w-10 rounded-full bg-muted-foreground/15" />
        </div>
        <div className="space-y-2">
          <div className="h-2 w-3/5 rounded-full bg-muted-foreground/20" />
          <div className="h-1.5 w-4/5 rounded-full bg-muted-foreground/15" />
          <div className="h-1.5 w-2/3 rounded-full bg-muted-foreground/15" />
        </div>

        <div className="absolute bottom-4 right-4 flex w-[72%] items-center gap-3 rounded-lg border border-primary/35 bg-background/95 p-2.5 shadow-xl">
          <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-md bg-gradient-to-br from-primary/55 to-accent/35">
            <ImageIcon className="h-4 w-4 text-foreground/80" strokeWidth={1.4} />
          </div>
          <div className="min-w-0 flex-1 space-y-1.5">
            <div className="h-1.5 w-4/5 rounded-full bg-foreground/45" />
            <div className="h-1.5 w-full rounded-full bg-muted-foreground/20" />
            <div className="h-1.5 w-2/3 rounded-full bg-muted-foreground/20" />
          </div>
          <div className="h-6 w-14 shrink-0 rounded-full bg-primary/80" />
        </div>
      </div>
    </div>
  );
}

export function CreativeToolsSection() {
  const { t } = useLanguage();

  const tools = [
    {
      icon: Crop,
      badge: t("creativeTools.edit.badge"),
      title: t("creativeTools.edit.title"),
      description: t("creativeTools.edit.desc"),
      Mock: CropperMock,
    },
    {
      icon: Eye,
      badge: t("creativeTools.preview.badge"),
      title: t("creativeTools.preview.title"),
      description: t("creativeTools.preview.desc"),
      Mock: PreviewMock,
    },
  ];

  return (
    <section className="landing-section landing-section-grid relative">
      <div className="container mx-auto px-5 md:px-8">
        <div className="mx-auto max-w-[1280px]">
          <div className="grid items-end gap-8 md:grid-cols-12">
            <div className="md:col-span-7">
              <LineReveal>
                <div className="landing-kicker mb-7">CREATIVE WORKSPACE</div>
              </LineReveal>
              <WordsReveal
                as="h2"
                text={t("creativeTools.title")}
                className="text-display block text-foreground"
                stagger={0.05}
              />
            </div>
            <div className="md:col-span-5">
              <LineReveal delay={0.3}>
                <p className="text-lg leading-relaxed text-muted-foreground">{t("creativeTools.subtitle")}</p>
              </LineReveal>
            </div>
          </div>

          <div className="mt-14 grid gap-5 md:grid-cols-2">
            {tools.map(({ icon: Icon, badge, title, description, Mock }, index) => (
              <motion.article
                key={badge}
                initial={{ opacity: 0, y: 30 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true, margin: "-60px" }}
                transition={{ duration: 0.8, delay: index * 0.1, ease: [0.22, 1, 0.36, 1] }}
                className={`landing-card group p-5 transition-transform duration-500 hover:-translate-y-1 md:p-7 ${index === 1 ? "md:mt-16" : ""}`}
              >
                <div className="rounded-[22px] bg-black/20 p-2"><Mock /></div>
                <div className="mb-5 mt-8 flex items-center gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-full border border-border transition-colors group-hover:border-primary/60 group-hover:bg-primary/5">
                    <Icon className="h-[17px] w-[17px] text-primary" strokeWidth={1.4} />
                  </div>
                  <span className="font-mono-eyebrow text-[10px] uppercase tracking-[0.22em] text-muted-foreground">{badge}</span>
                </div>
                <h3 className="mb-5 font-display text-3xl font-light leading-[1.08] tracking-tight text-foreground md:text-[42px]">
                  {title}
                </h3>
                <p className="max-w-lg text-[15px] leading-relaxed text-muted-foreground">{description}</p>
              </motion.article>
            ))}
          </div>

          <LineReveal delay={0.2} className="mt-10">
            <div className="flex flex-wrap items-center justify-center gap-3 text-[10px] font-mono-eyebrow uppercase tracking-[0.16em] text-muted-foreground">
              <span className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.025] px-4 py-2"><Scan className="h-3.5 w-3.5 text-primary" />{t("creativeTools.note.fit")}</span>
              <span className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.025] px-4 py-2"><Eye className="h-3.5 w-3.5 text-primary" />{t("creativeTools.note.preview")}</span>
            </div>
          </LineReveal>
        </div>
      </div>
    </section>
  );
}
