import { Bot, Gauge, Layers3, ShieldCheck } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import twinbidMark from "@/assets/twinbid-mark.svg";
import { TypedValue } from "./TypedValue";

const defaultTape = [
  ["PLATFORM", "PERFORMANCE TRAFFIC"],
  ["SCALE", "1M+ WEBSITES"],
  ["INVENTORY", "GLOBAL WEB + IN-APP"],
  ["BUYING", "CPC + CPM"],
  ["FORMATS", "NATIVE · BANNER · PUSH"],
  ["CONTROL", "CONVERSION TRACKING"],
  ["WORKSPACE", "ONE ACCOUNT · ONE BALANCE"],
  ["QUALITY", "FILTER · ROUTE · LEARN"],
];

const russianTape = [
  ["ПЛАТФОРМА", "РЕКЛАМНЫЙ ТРАФИК"],
  ["МАСШТАБ", "1M+ САЙТОВ"],
  ["ИНВЕНТАРЬ", "WEB + IN-APP"],
  ["ЗАКУПКА", "CPC + CPM"],
  ["ФОРМАТЫ", "NATIVE · BANNER · PUSH"],
  ["АНАЛИТИКА", "КОНВЕРСИИ И СТАТИСТИКА"],
  ["КАБИНЕТ", "ОДИН АККАУНТ · ОДИН БАЛАНС"],
  ["КАЧЕСТВО", "ФИЛЬТРАЦИЯ · ОПТИМИЗАЦИЯ"],
];

const consoleCopy = {
  ru: { campaign: "Кампания / Momentum 01", overview: "Результаты кампании", signal: "Динамика показов", model: "Модель оплаты", delivery: "Открутка", quality: "Контроль качества", active: "Запущена", learning: "Оптимизация", filtering: "Фильтрация", flow: "Объём трафика", route: "эффективные источники", note: "Пример интерфейса TwinBid", live: "АКТИВНО", create: "+ Кампания" },
  en: { campaign: "Campaign / Momentum 01", overview: "Performance overview", signal: "Campaign signal", model: "Buying model", delivery: "Delivery", quality: "Quality layer", active: "Active", learning: "Learning", filtering: "Filtering", flow: "Traffic flow", route: "optimized route", note: "Illustrative TwinBid interface", live: "LIVE", create: "+ Campaign" },
  es: { campaign: "Campaña / Momentum 01", overview: "Resumen de rendimiento", signal: "Señal de campaña", model: "Modelo de compra", delivery: "Entrega", quality: "Capa de calidad", active: "Activo", learning: "Aprendizaje", filtering: "Filtrado", flow: "Flujo de tráfico", route: "ruta optimizada", note: "Interfaz ilustrativa de TwinBid", live: "ACTIVA", create: "+ Campaña" },
  fr: { campaign: "Campagne / Momentum 01", overview: "Aperçu des performances", signal: "Signal de campagne", model: "Mode d’achat", delivery: "Diffusion", quality: "Couche qualité", active: "Actif", learning: "Apprentissage", filtering: "Filtrage", flow: "Flux de trafic", route: "route optimisée", note: "Interface TwinBid illustrative", live: "EN DIRECT", create: "+ Campagne" },
};

function CampaignConsole() {
  const { lang } = useLanguage();
  const text = consoleCopy[lang] ?? consoleCopy.en;
  const bars = [38, 52, 46, 66, 58, 78, 72, 88, 81, 94];

  return (
    <div className="signal-console" aria-label={text.note}>
      <div className="signal-console-bar">
        <div><i /><span>{text.campaign}</span></div>
        <span className="signal-console-live"><i /> {text.live}</span>
      </div>
      <div className="signal-console-body">
        <aside className="signal-console-sidebar" aria-hidden="true">
          <img src={twinbidMark} alt="" />
          <span className="active"><Gauge /></span>
          <span><Layers3 /></span>
          <span><Bot /></span>
          <span><ShieldCheck /></span>
        </aside>
        <div className="signal-console-main">
          <div className="signal-console-heading">
            <div><small>{text.overview}</small><strong>{text.signal}</strong></div>
            <button type="button" disabled>{text.create}</button>
          </div>
          <div className="signal-console-metrics">
            <div><span>{text.model}</span><strong>CPC / CPM</strong><small>{text.active}</small></div>
            <div><span>{text.delivery}</span><strong>{text.live}</strong><small>{text.learning}</small></div>
            <div><span>{text.quality}</span><strong>ON</strong><small>{text.filtering}</small></div>
          </div>
          <div className="signal-console-chart">
            <div><span>{text.flow}</span><strong>Momentum</strong></div>
            <div className="signal-console-bars" aria-hidden="true">
              {bars.map((height, index) => <i style={{ height: `${height}%` }} key={index} />)}
            </div>
            <span className="signal-console-marker"><i /> {text.route}</span>
          </div>
        </div>
      </div>
      <small className="signal-console-note">{text.note}</small>
    </div>
  );
}

export function SignalProof() {
  const { t, lang } = useLanguage();
  const tape = lang === "ru" ? russianTape : defaultTape;
  const intro = {
    ru: ["Весь рекламный трафик", "В одном кабинете"],
    en: ["Global traffic", "One operating layer"],
    es: ["Tráfico global", "Una sola capa de trabajo"],
    fr: ["Trafic mondial", "Une seule couche de travail"],
  }[lang] ?? ["Global traffic", "One operating layer"];

  const proof = [
    ["1M+", t("hero.statSites")],
    ["CPC / CPM", lang === "ru" ? "Две модели оплаты" : "Flexible buying models"],
    ["Web + in-app", lang === "ru" ? "Сайты и приложения" : "Global ad inventory"],
  ];

  return (
    <section className="signal-proof" id="signal-proof">
      <div className="signal-tape" aria-hidden="true">
        <div className="signal-tape-track">
          {[0, 1].map((group) => (
            <div className="signal-tape-group" key={group}>
              {tape.map(([label, value], index) => (
                <span className="signal-tape-item" data-tone={index % 4} key={`${group}-${label}`}>
                  <small>{label}</small><strong>{value}</strong>
                </span>
              ))}
            </div>
          ))}
        </div>
      </div>

      <div className="landing-editorial-shell signal-proof-intro">
        <div className="signal-proof-copy">
          <p>{intro[0]}<br /><strong>{intro[1]}</strong></p>
          <span>{t("benefits.subtitle")}</span>
        </div>
        <CampaignConsole />
      </div>

      <div className="signal-proof-facts">
        <div className="landing-editorial-shell signal-proof-grid">
          {proof.map(([value, label], index) => (
            <div key={value}><strong><TypedValue value={value} delay={index * 110} /></strong><span>{label}</span></div>
          ))}
        </div>
      </div>
    </section>
  );
}
