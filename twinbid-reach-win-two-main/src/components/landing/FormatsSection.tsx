import { Layers, FileText, LayoutGrid, Bell, X } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { motion } from "framer-motion";
import { WordsReveal, LineReveal } from "./CinematicReveal";

const formatIcons = [Layers, FileText, LayoutGrid, Bell];
const formatNames = ["Popunder", "Native", "Banner", "In-page Push"];
const formatDescKeys = ["formats.popunder.desc", "formats.native.desc", "formats.banner.desc", "formats.push.desc"];

function MockFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="relative w-full aspect-[16/10] rounded-xl border border-border/70 bg-background/50 overflow-hidden mb-10">
      {children}
    </div>
  );
}

function PopunderMock() {
  return (
    <MockFrame>
      {/* full-frame popunder rising behind */}
      <div
        className="absolute inset-0"
        style={{
          background:
            "linear-gradient(135deg, hsl(var(--primary) / 0.32) 0%, hsl(var(--primary) / 0.10) 55%, transparent 100%)",
        }}
      />
      {/* browser window on top-left */}
      <div className="absolute top-4 left-4 w-[62%] h-[70%] rounded-md bg-background border border-border/70 p-2.5 shadow-lg">
        <div className="flex items-center gap-2 mb-3">
          <X className="w-3 h-3 text-muted-foreground/70" strokeWidth={1.5} />
          <div className="h-[4px] w-[38%] rounded-full bg-muted-foreground/25" />
        </div>
        <div className="space-y-2">
          <div className="h-[4px] w-[85%] rounded-full bg-muted-foreground/25" />
          <div className="h-[4px] w-[55%] rounded-full bg-muted-foreground/20" />
        </div>
      </div>
    </MockFrame>
  );
}

function NativeMock() {
  return (
    <MockFrame>
      <div className="absolute inset-0 p-4 flex flex-col gap-3">
        {/* sponsored row */}
        <div
          className="flex items-center gap-3 rounded-md p-2.5"
          style={{
            background:
              "linear-gradient(90deg, hsl(var(--primary) / 0.10), transparent 70%)",
          }}
        >
          <div
            className="w-9 h-9 rounded-sm flex-shrink-0"
            style={{
              background:
                "linear-gradient(135deg, hsl(var(--primary) / 0.55), hsl(var(--primary) / 0.15))",
            }}
          />
          <div className="flex-1 space-y-1.5">
            <div className="h-[4px] w-[70%] rounded-full bg-muted-foreground/25" />
            <span className="block font-mono-eyebrow text-[9px] tracking-[0.28em] text-primary">
              SPONSORED
            </span>
          </div>
        </div>
        {/* neutral rows */}
        {[0, 1].map((i) => (
          <div key={i} className="flex items-center gap-3 p-2.5">
            <div className="w-9 h-9 rounded-sm bg-muted-foreground/15 flex-shrink-0" />
            <div className="flex-1 space-y-1.5">
              <div className="h-[4px] w-[65%] rounded-full bg-muted-foreground/20" />
              <div className="h-[4px] w-[45%] rounded-full bg-muted-foreground/15" />
            </div>
          </div>
        ))}
      </div>
    </MockFrame>
  );
}

function BannerMock() {
  return (
    <MockFrame>
      {/* top leaderboard */}
      <div
        className="absolute top-4 left-4 right-4 h-9 rounded-md flex items-center justify-center"
        style={{
          background:
            "linear-gradient(180deg, hsl(var(--primary) / 0.35), hsl(var(--primary) / 0.12))",
        }}
      >
        <span className="font-mono-eyebrow text-[10px] tracking-[0.25em] text-foreground/85">
          728 × 90 · BANNER
        </span>
      </div>
      {/* article lines */}
      <div className="absolute left-4 top-[70px] w-[48%] space-y-2">
        <div className="h-[4px] w-[90%] rounded-full bg-muted-foreground/25" />
        <div className="h-[4px] w-[75%] rounded-full bg-muted-foreground/20" />
        <div className="h-[4px] w-[55%] rounded-full bg-muted-foreground/20" />
      </div>
      {/* sidebar 300x250 */}
      <div
        className="absolute bottom-4 right-4 w-[40%] h-[62%] rounded-md flex items-center justify-center"
        style={{
          background:
            "linear-gradient(180deg, hsl(var(--primary) / 0.35), hsl(var(--primary) / 0.08))",
        }}
      >
        <span className="font-mono-eyebrow text-[10px] tracking-[0.25em] text-foreground/75">
          300 × 250
        </span>
      </div>
    </MockFrame>
  );
}

function PushMock() {
  return (
    <MockFrame>
      {/* page content */}
      <div className="absolute inset-0 p-4 space-y-2.5">
        <div className="h-[4px] w-[65%] rounded-full bg-muted-foreground/20" />
        <div className="h-[4px] w-[50%] rounded-full bg-muted-foreground/15" />
      </div>
      {/* push toast */}
      <div className="absolute bottom-4 right-4 w-[70%] h-11 rounded-md border border-border/80 bg-background/95 flex items-center gap-2.5 pl-2 pr-2.5 shadow-lg">
        <div
          className="w-7 h-7 rounded-sm flex items-center justify-center flex-shrink-0"
          style={{
            background:
              "linear-gradient(135deg, hsl(var(--primary) / 0.6), hsl(var(--primary) / 0.2))",
          }}
        >
          <Bell className="w-3.5 h-3.5 text-foreground/80" strokeWidth={1.5} />
        </div>
        <div className="h-[4px] flex-1 rounded-full bg-muted-foreground/35" />
        <X className="w-3 h-3 text-muted-foreground/60 flex-shrink-0" strokeWidth={1.5} />
      </div>
    </MockFrame>
  );
}

const mockups = [PopunderMock, NativeMock, BannerMock, PushMock];

export function FormatsSection() {
  const { t } = useLanguage();

  return (
    <section id="formats" className="relative py-[140px] frame-immersive">
      <div className="container mx-auto px-8">
        <div className="max-w-[1280px] mx-auto">
          <div className="text-center mb-24">
            <LineReveal>
              <div className="eyebrow mb-8 inline-block">— 04 / AD INVENTORY</div>
            </LineReveal>
            <WordsReveal
              as="h2"
              text={`${t("formats.title").trim()} ${t("formats.title2").trim()}`}
              className="text-display block text-foreground"
              brandWord={t("formats.title2").trim()}
              brandClass="gradient-text"
              stagger={0.06}
            />
            <LineReveal delay={0.5} className="mt-8 max-w-xl mx-auto">
              <p className="text-muted-foreground text-lg">{t("formats.subtitle")}</p>
            </LineReveal>
          </div>

          <div className="grid sm:grid-cols-2 gap-px bg-border">
            {formatIcons.map((Icon, index) => {
              const Mock = mockups[index];
              return (
                <motion.div
                  key={index}
                  initial={{ opacity: 0, y: 30 }}
                  whileInView={{ opacity: 1, y: 0 }}
                  viewport={{ once: true, margin: "-60px" }}
                  transition={{ duration: 0.8, delay: index * 0.08, ease: [0.22, 1, 0.36, 1] }}
                  className="bg-background p-10 md:p-14 group relative overflow-hidden hover:bg-secondary/30 transition-colors duration-500"
                >
                  <div className="flex items-start justify-between mb-8">
                    <span className="font-mono-eyebrow text-[11px] tracking-[0.22em] text-muted-foreground">
                      Format · 0{index + 1}
                    </span>
                    <Icon className="w-5 h-5 text-primary opacity-70 group-hover:opacity-100 transition-opacity" strokeWidth={1.3} />
                  </div>
                  <Mock />
                  <h3 className="font-display text-4xl md:text-5xl font-light text-foreground mb-5 tracking-tight leading-[1.05]">
                    {formatNames[index]}
                  </h3>
                  <p className="text-muted-foreground text-[15px] leading-relaxed max-w-md">
                    {t(formatDescKeys[index])}
                  </p>
                </motion.div>
              );
            })}
          </div>
        </div>
      </div>
    </section>
  );
}
