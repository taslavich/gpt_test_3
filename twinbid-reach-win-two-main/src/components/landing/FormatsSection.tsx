import { ArrowUpRight } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { usePinnedScene } from "./usePinnedScene";

const formats = [
  { id: "popunder", name: "Popunder", desc: "formats.popunder.desc" },
  { id: "native", name: "Native", desc: "formats.native.desc" },
  { id: "banner", name: "Banner", desc: "formats.banner.desc" },
  { id: "inpage", name: "In-page Push", desc: "formats.push.desc" },
] as const;

const formatCopy = {
  ru: {
    section: "04 / ФОРМАТЫ",
    subtitle: "Выберите формат под задачу кампании и привычки аудитории.",
    title: "Форматы рекламы",
    preview: "ПРЕДПРОСМОТР ФОРМАТА",
    browser: "сайт площадки / статья",
    ad: "РЕКЛАМНЫЙ ФОРМАТ",
    sponsored: "РЕКЛАМА",
    nativeHeadline: "Внутри материала",
    nativeCopy: "Встраивается в страницу и воспринимается как часть материала.",
    tags: { popunder: "МАКСИМАЛЬНЫЙ ОХВАТ", native: "ВНУТРИ КОНТЕНТА", banner: "МЕДИЙНЫЙ ФОРМАТ", inpage: "ПОВЕРХ СТРАНИЦЫ" },
  },
  en: {
    section: "04 / AD FORMATS",
    subtitle: "Match the format to the campaign goal and the way people consume content.",
    title: "Advertising formats",
    preview: "LIVE FORMAT PREVIEW",
    browser: "publisher site / article",
    ad: "AD FORMAT",
    sponsored: "SPONSORED",
    nativeHeadline: "Inside the story",
    nativeCopy: "Follows the page structure and feels like part of the content.",
    tags: { popunder: "FULL ATTENTION", native: "IN-CONTENT", banner: "DISPLAY REACH", inpage: "ON-PAGE MESSAGE" },
  },
  es: {
    section: "04 / FORMATOS",
    subtitle: "Elige el formato según el objetivo y la forma en que la audiencia consume el contenido.",
    title: "Formatos publicitarios",
    preview: "VISTA PREVIA DEL FORMATO",
    browser: "sitio del publisher / artículo",
    ad: "FORMATO PUBLICITARIO",
    sponsored: "PUBLICIDAD",
    nativeHeadline: "Dentro del artículo",
    nativeCopy: "Respeta la estructura de la página y se integra en el contenido.",
    tags: { popunder: "ATENCIÓN TOTAL", native: "DENTRO DEL CONTENIDO", banner: "ALCANCE DISPLAY", inpage: "MENSAJE EN PÁGINA" },
  },
  fr: {
    section: "04 / FORMATS",
    subtitle: "Choisissez le format selon l’objectif et la manière dont l’audience consulte le contenu.",
    title: "Formats publicitaires",
    preview: "APERÇU DU FORMAT",
    browser: "site éditeur / article",
    ad: "FORMAT PUBLICITAIRE",
    sponsored: "SPONSORISÉ",
    nativeHeadline: "Au cœur de l’article",
    nativeCopy: "Reprend la structure de la page et s’intègre naturellement au contenu.",
    tags: { popunder: "ATTENTION MAXIMALE", native: "DANS LE CONTENU", banner: "PORTÉE DISPLAY", inpage: "MESSAGE SUR LA PAGE" },
  },
};

type Format = (typeof formats)[number];
type Labels = (typeof formatCopy)[keyof typeof formatCopy];

function FormatScene({ format, labels }: { format: Format; labels: Labels }) {
  const { id: type, name } = format;

  return (
    <div className={`format-editorial-scene format-editorial-${type}`} aria-hidden="true">
      <div className="format-editorial-browser">
        <div className="format-editorial-browser-top"><i /><i /><i /><span>{labels.browser}</span></div>
        <div className="format-editorial-page">
          {type === "native" ? (
            <>
              <div className="format-editorial-copy-lines"><span /><span /><span /></div>
              <div className="format-editorial-native-inline">
                <div className="format-editorial-native-image"><span>T</span></div>
                <div className="format-editorial-native-copy">
                  <small>{labels.sponsored}</small>
                  <strong>{labels.nativeHeadline}</strong>
                  <p>{labels.nativeCopy}</p>
                  <span>TwinBid <ArrowUpRight /></span>
                </div>
              </div>
              <div className="format-editorial-copy-lines format-editorial-copy-lines-short"><span /><span /><span /></div>
            </>
          ) : (
            <><i /><i /><i /><i /></>
          )}
        </div>
      </div>
      {type !== "native" ? (
        <div className="format-editorial-ad">
          <small>{type === "banner" ? "728 × 90" : type === "inpage" ? labels.sponsored : labels.ad}</small>
          <strong>{name === "In-page Push" ? <>IN-PAGE<br />PUSH</> : name}</strong>
          <span>TwinBid <ArrowUpRight /></span>
        </div>
      ) : null}
    </div>
  );
}

export function FormatsSection() {
  const { lang, t } = useLanguage();
  const text = formatCopy[lang] ?? formatCopy.en;
  const { active, sectionRef } = usePinnedScene(formats.length);
  const current = formats[active];

  return (
    <section ref={sectionRef} id="formats" className="formats-editorial pinned-formats" aria-labelledby="formats-editorial-title">
      <div className="pinned-formats-frame">
        <div className="landing-editorial-shell pinned-formats-grid">
          <div className="pinned-formats-copy">
            <div className="pinned-formats-heading">
              <p className="landing-section-index">{text.section}</p>
              <p>{text.subtitle}</p>
            </div>
            <div className="pinned-formats-active" aria-live="polite">
              <span>0{active + 1} / {text.tags[current.id]}</span>
              <h2 id="formats-editorial-title" key={`${current.id}-title`}>{current.name}</h2>
              <p key={`${current.id}-copy`}>{t(current.desc)}</p>
            </div>
            <div className="pinned-story-progress" aria-hidden="true">
              {formats.map((format, index) => (
                <span className={index === active ? "is-active" : ""} key={format.id}><i />0{index + 1}</span>
              ))}
            </div>
          </div>

          <div className="formats-editorial-stage">
            <div className="formats-editorial-stage-label">
              <span>{text.preview}</span>
              <strong>0{active + 1} / 04</strong>
            </div>
            <FormatScene key={`${lang}-${current.id}`} format={current} labels={text} />
            <div className="formats-editorial-stage-foot">
              <strong>{current.name}</strong>
            </div>
          </div>
        </div>
      </div>

      <div className="pinned-formats-mobile landing-editorial-shell">
        <div className="pinned-story-mobile-heading">
          <p className="landing-section-index">{text.section}</p>
          <h2>{text.title}</h2>
        </div>
        {formats.map((format, index) => (
          <article key={format.id}>
            <div><span>0{index + 1}</span><small>{text.tags[format.id]}</small></div>
            <h3>{format.name}</h3>
            <p>{t(format.desc)}</p>
            <div className="formats-editorial-mobile-scene"><FormatScene format={format} labels={text} /></div>
          </article>
        ))}
      </div>
    </section>
  );
}
