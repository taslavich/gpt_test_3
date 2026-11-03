import { ArrowUpRight } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import type { ZoneFormat } from "@/contexts/PublisherContext";

const sceneTypes: Record<ZoneFormat, string> = {
  popunder: "popunder",
  native: "native",
  banner: "banner",
  inpage_push: "inpage",
  video_popup: "video-popup",
  instream: "video-instream",
  outstream: "video-outstream",
};
const names: Record<ZoneFormat, string> = {
  popunder: "Popunder", native: "Native Ads", banner: "Banner", inpage_push: "In-page Push",
  video_popup: "Video Pop-up", instream: "In-Stream VAST", outstream: "Out-Stream",
};
const labels = {
  ru: { browser: "сайт площадки / статья", ad: "РЕКЛАМНЫЙ ФОРМАТ", sponsored: "РЕКЛАМА", headline: "Внутри материала", description: "Нативный блок становится частью страницы." },
  en: { browser: "publisher site / article", ad: "AD FORMAT", sponsored: "SPONSORED", headline: "Inside the story", description: "A native unit fits the page's content." },
  es: { browser: "sitio del publisher / artículo", ad: "FORMATO PUBLICITARIO", sponsored: "PUBLICIDAD", headline: "Dentro del artículo", description: "Un formato nativo integrado en la página." },
  fr: { browser: "site éditeur / article", ad: "FORMAT PUBLICITAIRE", sponsored: "SPONSORISÉ", headline: "Au cœur de l’article", description: "Un format natif intégré à la page." },
  "zh-CN": { browser: "发布商网站 / 文章", ad: "广告形式", sponsored: "广告", headline: "文章内容中", description: "原生广告自然融入页面内容。" },
};

export function PublisherFormatPreview({ format }: { format: ZoneFormat }) {
  const { lang } = useLanguage();
  const text = labels[lang] ?? labels.en;
  const type = sceneTypes[format];
  const name = names[format];
  const isVideo = type.startsWith("video-");
  return (
    <div className={`format-editorial-scene format-editorial-${type}`} aria-hidden="true">
      <div className="format-editorial-browser">
        <div className="format-editorial-browser-top"><i /><i /><i /><span>{text.browser}</span></div>
        <div className="format-editorial-page">
          {format === "native" ? <>
            <div className="format-editorial-copy-lines"><span /><span /><span /></div>
            <div className="format-editorial-native-inline">
              <div className="format-editorial-native-image"><span>T</span></div>
              <div className="format-editorial-native-copy"><small>{text.sponsored}</small><strong>{text.headline}</strong><p>{text.description}</p><span>TwinBid <ArrowUpRight /></span></div>
            </div>
            <div className="format-editorial-copy-lines format-editorial-copy-lines-short"><span /><span /><span /></div>
          </> : isVideo ? <>
            <div className="format-editorial-video-player"><span>▶</span><small>{name}</small><i /></div>
            <div className="format-editorial-copy-lines"><span /><span /><span /></div>
          </> : <><i /><i /><i /><i /></>}
        </div>
      </div>
      {format === "video_popup" && <div className="format-editorial-video-float"><b>×</b><span>▶</span><small>{name}</small></div>}
      {!isVideo && format !== "native" && <div className="format-editorial-ad">
        <small>{format === "banner" ? "728 × 90" : format === "inpage_push" ? text.sponsored : text.ad}</small>
        <strong>{format === "inpage_push" ? <>IN-PAGE<br />PUSH</> : name}</strong>
        <span>TwinBid <ArrowUpRight /></span>
      </div>}
    </div>
  );
}
