import { Layers, FileText, LayoutGrid, Bell } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { motion } from "framer-motion";
import { WordsReveal, LineReveal } from "./CinematicReveal";

const formatIcons = [Layers, FileText, LayoutGrid, Bell];
const formatNames = ["Popunder", "Native", "Banner", "In-page Push"];
const formatDescKeys = ["formats.popunder.desc", "formats.native.desc", "formats.banner.desc", "formats.push.desc"];

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
            {formatIcons.map((Icon, index) => (
              <motion.div
                key={index}
                initial={{ opacity: 0, y: 30 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true, margin: "-60px" }}
                transition={{ duration: 0.8, delay: index * 0.08, ease: [0.22, 1, 0.36, 1] }}
                className="bg-background p-10 md:p-14 group relative overflow-hidden hover:bg-secondary/30 transition-colors duration-500"
              >
                <div className="flex items-start justify-between mb-12">
                  <span className="font-mono-eyebrow text-[11px] tracking-[0.22em] text-muted-foreground">
                    Format · 0{index + 1}
                  </span>
                  <Icon className="w-5 h-5 text-primary opacity-70 group-hover:opacity-100 transition-opacity" strokeWidth={1.3} />
                </div>
                <h3 className="font-display text-4xl md:text-5xl font-light text-foreground mb-5 tracking-tight leading-[1.05]">
                  {formatNames[index]}
                </h3>
                <p className="text-muted-foreground text-[15px] leading-relaxed max-w-md">
                  {t(formatDescKeys[index])}
                </p>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
