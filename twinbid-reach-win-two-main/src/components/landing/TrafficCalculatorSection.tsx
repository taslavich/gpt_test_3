import { useRef, useState, type CSSProperties } from "react";
import { ArrowRight, BarChart3, Crop, Eye, Target } from "lucide-react";
import { AnimatePresence, motion, useInView, useReducedMotion } from "framer-motion";
import { AuthDialog } from "./AuthDialog";
import { useLanguage } from "@/contexts/LanguageContext";
import { TypedValue } from "./TypedValue";

const copy = {
  ru: {
    section: "03 / ИНСТРУМЕНТЫ",
    console: "ТРАФИК / ПОТЕНЦИАЛ",
    live: "АКТУАЛЬНЫЕ ДАННЫЕ",
    title: "Узнайте, сколько трафика доступно вашей кампании",
    subtitle: "TwinBid анализирует последние полностью завершённые сутки, показывает весь доступный объём и помогает оценить потенциал для роста.",
    potential: "Доступные показы",
    received: "Показы кампании",
    share: "Доля кампании",
    bid: "Текущая ставка",
    insight: "Увидьте весь доступный объём",
    description: "Выберите кампанию или задайте таргетинг вручную. Калькулятор покажет доступный объём, долю вашей кампании и позволит сразу скорректировать ставку.",
    cta: "Открыть калькулятор",
  },
  en: {
    section: "03 / CAMPAIGN TOOLS",
    console: "TRAFFIC / OPPORTUNITY",
    live: "LIVE DATA",
    title: "See how much traffic you are missing",
    subtitle: "Look beyond the campaign report: TwinBid takes the latest complete day, shows the full available volume and makes the room for scaling immediately clear.",
    potential: "Available impressions",
    received: "Campaign impressions",
    share: "Share received",
    bid: "Current bid",
    insight: "See more than the traffic you bought",
    description: "Select a campaign or set targeting manually. The calculator shows the available impression volume, what share the campaign received, and lets you update the bid immediately.",
    cta: "Open calculator",
  },
  es: {
    section: "03 / HERRAMIENTAS",
    console: "TRÁFICO / POTENCIAL",
    live: "DATOS ACTUALES",
    title: "Descubre cuánto tráfico estás perdiendo",
    subtitle: "Mira más allá del informe de campaña: TwinBid toma el último día completo, muestra todo el volumen disponible y deja claro cuánto margen queda para crecer.",
    potential: "Impresiones disponibles",
    received: "Impresiones de campaña",
    share: "Cuota obtenida",
    bid: "Puja actual",
    insight: "Ve más que el tráfico comprado",
    description: "Selecciona una campaña o configura la segmentación. La calculadora muestra el volumen de impresiones disponible, qué cuota recibió la campaña y permite actualizar la puja al instante.",
    cta: "Abrir calculadora",
  },
  fr: {
    section: "03 / OUTILS",
    console: "TRAFIC / POTENTIEL",
    live: "DONNÉES ACTUELLES",
    title: "Découvrez le trafic qui vous échappe",
    subtitle: "Allez au-delà du rapport de campagne : TwinBid analyse la dernière journée complète, affiche tout le volume disponible et révèle immédiatement la marge de croissance.",
    potential: "Impressions disponibles",
    received: "Impressions de la campagne",
    share: "Part obtenue",
    bid: "Enchère actuelle",
    insight: "Voyez plus que le trafic déjà acheté",
    description: "Sélectionnez une campagne ou définissez manuellement le ciblage. Le calculateur affiche le volume d’impressions disponible, la part obtenue par la campagne et vous permet d’ajuster immédiatement l’enchère.",
    cta: "Ouvrir le calculateur",
  },
};

const metrics = [
  { key: "potential" as const, value: "128 400" },
  { key: "received" as const, value: "31 250" },
  { key: "share" as const, value: "24,3%" },
  { key: "bid" as const, value: "$0.034" },
];

const moduleCopy = {
  ru: [
    ["Калькулятор трафика", "Показывает доступный объём и долю, которую уже получает кампания."],
    ["Подготовка креативов", "Позволяет редактировать креативы прямо внутри кабинета."],
    ["Живой предпросмотр", "Показывает объявление на странице до отправки на модерацию."],
  ],
  en: [
    ["Traffic calculator", "Shows the full available volume and the share captured by a campaign."],
    ["Creative preparation", "Automatic cropping for required sizes without a separate editor."],
    ["Live preview", "Check every format before submitting a campaign for moderation."],
  ],
  es: [
    ["Calculadora de tráfico", "Muestra el volumen disponible y la cuota que obtiene la campaña."],
    ["Preparación creativa", "Recorte automático a los tamaños necesarios sin otro editor."],
    ["Vista previa", "Comprueba cada formato antes de enviar la campaña a moderación."],
  ],
  fr: [
    ["Calculateur de trafic", "Affiche le volume disponible et la part obtenue par la campagne."],
    ["Préparation des créations", "Recadrage automatique aux formats requis, sans autre éditeur."],
    ["Aperçu en direct", "Contrôlez chaque format avant l’envoi en modération."],
  ],
};

const visualCopy = {
  ru: { source: "Исходное изображение", ready: "3 формата готовы", crop: "Автокадрирование", preview: "Предпросмотр объявления", approved: "Объявление готово к запуску", sponsored: "Реклама", headline: "Больше клиентов для вашего бизнеса", body: "Запускайте и масштабируйте кампании с TwinBid" },
  en: { source: "Source creative", ready: "Sizes ready", crop: "Auto crop", preview: "Ad preview", approved: "Format ready to launch", sponsored: "Sponsored", headline: "Reach more customers", body: "Scale your campaigns with TwinBid" },
  es: { source: "Creatividad original", ready: "Tamaños listos", crop: "Recorte automático", preview: "Vista previa del anuncio", approved: "Formato listo para lanzar", sponsored: "Publicidad", headline: "Llega a más clientes", body: "Escala tus campañas con TwinBid" },
  fr: { source: "Création source", ready: "Formats prêts", crop: "Recadrage auto", preview: "Aperçu de l’annonce", approved: "Format prêt à diffuser", sponsored: "Sponsorisé", headline: "Touchez plus de clients", body: "Développez vos campagnes avec TwinBid" },
};

type ModuleIndex = 0 | 1 | 2;

function TrafficVisual({ text }: { text: (typeof copy)[keyof typeof copy] }) {
  return (
    <>
      <div className="operations-metrics">
        {metrics.map(({ key, value }, index) => (
          <div key={key}><span>{text[key]}</span><strong><TypedValue value={value} delay={index * 90} /></strong></div>
        ))}
      </div>
      <div className="operations-chart">
        <div className="operations-chart-capacity">
          <span>{text.potential}</span>
          <strong><TypedValue value="128 400" /></strong>
          <small><TypedValue value="100%" delay={180} /></small>
        </div>
        <div className="operations-chart-plot">
          <div className="operations-chart-copy"><span>{text.received}</span><strong><TypedValue value="24,3%" /></strong></div>
          <div className="operations-chart-bars" aria-hidden="true">
            {[34, 48, 42, 63, 56, 78, 70, 88, 82, 96].map((height, index) => (
              <i key={index} style={{ height: `${height}%`, "--bar-delay": `${index * 65}ms` } as CSSProperties}><span /></i>
            ))}
          </div>
        </div>
      </div>
    </>
  );
}

function CreativeVisual({ labels }: { labels: (typeof visualCopy)[keyof typeof visualCopy] }) {
  return (
    <div className="operations-creative" aria-label={labels.crop}>
      <div className="operations-creative-source">
        <div><span>{labels.source}</span><small>1200 × 628</small></div>
        <div className="operations-creative-art"><strong>T</strong><i /><i /></div>
      </div>
      <div className="operations-creative-arrow" aria-hidden="true">→</div>
      <div className="operations-creative-sizes">
        <div className="operations-creative-status"><span>{labels.ready}</span><strong>03 / 03</strong></div>
        <div className="operations-crop-grid">
          <div data-size="wide"><i>T</i><small>728 × 90</small></div>
          <div data-size="square"><i>T</i><small>300 × 250</small></div>
          <div data-size="tower"><i>T</i><small>160 × 600</small></div>
        </div>
      </div>
      <div className="operations-creative-badge"><Crop /> {labels.crop}</div>
    </div>
  );
}

function PreviewVisual({ labels }: { labels: (typeof visualCopy)[keyof typeof visualCopy] }) {
  return (
    <div className="operations-preview" aria-label={labels.preview}>
      <div className="operations-preview-browser">
        <div className="operations-preview-top"><i /><i /><i /><span>publisher.site / article</span></div>
        <div className="operations-preview-page">
          <div className="operations-preview-lines"><i /><i /><i /></div>
          <div className="operations-preview-ad">
            <div className="operations-preview-image"><strong>T</strong></div>
            <div><small>{labels.sponsored}</small><strong>{labels.headline}</strong><p>{labels.body}</p><span>TwinBid ↗</span></div>
          </div>
          <div className="operations-preview-lines operations-preview-lines-short"><i /><i /><i /></div>
        </div>
      </div>
      <div className="operations-preview-status"><span><i /> {labels.approved}</span><strong>DESKTOP / NATIVE</strong></div>
    </div>
  );
}

function OperationsConsole({ text, labels, activeModule, activeLabel }: { text: (typeof copy)[keyof typeof copy]; labels: (typeof visualCopy)[keyof typeof visualCopy]; activeModule: ModuleIndex; activeLabel: string }) {
  const consoleRef = useRef<HTMLDivElement | null>(null);
  const isVisible = useInView(consoleRef, { amount: 0.28, once: true });

  return (
    <div
      ref={consoleRef}
      className={`operations-console-visual${isVisible ? " is-visible" : ""}`}
      aria-label={activeLabel}
      role="tabpanel"
      id="operations-visual-panel"
    >
      <div className="operations-console-bar">
        <span>{activeModule === 0 ? <BarChart3 /> : activeModule === 1 ? <Crop /> : <Eye />} {activeLabel}</span>
        <strong><i /> {text.live}</strong>
      </div>
      <AnimatePresence initial={false} mode="wait">
        <motion.div
          className="operations-scene"
          key={activeModule}
          initial={{ opacity: 0, y: 18 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -12 }}
          transition={{ duration: 0.42, ease: [0.22, 1, 0.36, 1] }}
        >
          {activeModule === 0 ? <TrafficVisual text={text} /> : activeModule === 1 ? <CreativeVisual labels={labels} /> : <PreviewVisual labels={labels} />}
        </motion.div>
      </AnimatePresence>
    </div>
  );
}

export function TrafficCalculatorSection() {
  const { lang } = useLanguage();
  const reduceMotion = useReducedMotion();
  const text = copy[lang] ?? copy.en;
  const modules = moduleCopy[lang] ?? moduleCopy.en;
  const labels = visualCopy[lang] ?? visualCopy.en;
  const icons = [Target, Crop, Eye];
  const [activeModule, setActiveModule] = useState<ModuleIndex>(0);

  return (
    <section id="traffic-calculator" className="operations-section" aria-labelledby="operations-title">
      <div className="landing-editorial-shell">
        <div className="operations-heading">
          <div className="operations-heading-main">
            <p className="landing-section-index">{text.section}</p>
            <h2 id="operations-title">{text.insight}</h2>
          </div>
          <p className="operations-heading-speech">{text.subtitle}</p>
        </div>

        <div className="operations-layout">
          <OperationsConsole text={text} labels={labels} activeModule={activeModule} activeLabel={modules[activeModule][0]} />
          <div className="operations-modules" role="tablist" aria-label={text.section}>
            {modules.map(([title, description], index) => {
              const Icon = icons[index];
              return (
                <motion.button
                  key={title}
                  type="button"
                  role="tab"
                  aria-selected={activeModule === index}
                  aria-controls="operations-visual-panel"
                  className={`operations-module${activeModule === index ? " is-active" : ""}`}
                  onClick={() => setActiveModule(index as ModuleIndex)}
                  initial={reduceMotion ? false : { opacity: 0, y: 38, scale: 0.985 }}
                  whileInView={reduceMotion ? undefined : { opacity: 1, y: 0, scale: 1 }}
                  viewport={{ once: true, amount: 0.32 }}
                  transition={{ duration: 0.52, delay: index * 0.06, ease: [0.22, 1, 0.36, 1] }}
                >
                  <div><span>0{index + 1}</span><span className="operations-module-state">{activeModule === index ? "●" : "○"}</span><Icon /></div>
                  <h3>{title}</h3>
                  <p>{description}</p>
                </motion.button>
              );
            })}
            <motion.div
              className="operations-action"
              initial={reduceMotion ? false : { opacity: 0, y: 28 }}
              whileInView={reduceMotion ? undefined : { opacity: 1, y: 0 }}
              viewport={{ once: false, amount: 0.35 }}
              transition={{ duration: 0.6, ease: [0.16, 1, 0.3, 1] }}
            >
              <p>{text.description}</p>
              <AuthDialog
                trigger={<button className="landing-button landing-button-primary">{text.cta}<ArrowRight /></button>}
                defaultTab="register"
              />
            </motion.div>
          </div>
        </div>
      </div>
    </section>
  );
}
