import { useLanguage } from "@/contexts/LanguageContext";
import twinbidSymbol from "@/assets/twinbid-symbol.svg";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { usePinnedScene } from "./usePinnedScene";

const storyCopy = {
  ru: {
    section: "02 / КАК ЭТО РАБОТАЕТ",
    kicker: "От настройки до результата",
    title: "Весь путь кампании — на одной платформе",
    output: "РЕЗУЛЬТАТ",
    layer: ["ЕДИНЫЙ", "КАБИНЕТ"],
    sources: [["ПЛОЩАДКИ", "ИНВЕНТАРЬ"], ["РЕКЛАМНЫЕ", "СЕТИ"], ["НАСТРОЙКИ", "КАМПАНИИ"]],
    steps: [
      { overline: "01 / ДОСТУП", title: "Один кабинет даёт доступ ко всему инвентарю", copy: "TwinBid объединяет трафик сотен рекламных сетей. После регистрации вам доступен инвентарь более чем миллиона сайтов.", label: "Инвентарь доступен", core: "ПОДКЛЮЧЕНИЕ", status: "ПЛОЩАДКИ ПОДКЛЮЧЕНЫ" },
      { overline: "02 / КАЧЕСТВО", title: "Некачественный трафик отсеивается до покупки", copy: "Встроенная антифрод-система анализирует трафик до списания бюджета и блокирует подозрительные показы и клики.", label: "Трафик проверен", core: "ФИЛЬТРАЦИЯ", status: "АНТИФРОД ВКЛЮЧЁН" },
      { overline: "03 / УПРАВЛЕНИЕ", title: "Вы управляете запуском на каждом этапе", copy: "Выберите CPC или CPM, настройте аудиторию и бюджет, добавьте креативы и запустите кампанию — всё в одном кабинете.", label: "Кампания запущена", core: "ЗАПУСК", status: "НАСТРОЙКИ ПРИМЕНЕНЫ" },
      { overline: "04 / РОСТ", title: "Данные показывают, куда направить бюджет", copy: "Статистика, расчёт доступного объёма и AI-оптимизация помогают находить эффективные источники и уверенно масштабировать кампанию.", label: "Источники найдены", core: "ОПТИМИЗАЦИЯ", status: "ГОТОВО К МАСШТАБИРОВАНИЮ" },
    ],
  },
  en: {
    section: "02 / HOW IT WORKS",
    kicker: "From setup to results",
    title: "One platform for the entire campaign journey",
    output: "RESULT",
    layer: ["UNIFIED", "SYSTEM"],
    sources: [["PUBLISHER", "INVENTORY"], ["AD NETWORK", "TRAFFIC"], ["CAMPAIGN", "SETTINGS"]],
    steps: [
      { overline: "01 / ACCESS", title: "One account opens the entire advertising market", copy: "TwinBid brings together traffic from hundreds of ad networks. A single registration gives you access to inventory across more than one million websites.", label: "Inventory connected", core: "CONNECT", status: "PUBLISHER ACCESS ENABLED" },
      { overline: "02 / QUALITY", title: "Low-quality traffic is blocked before you buy it", copy: "Built-in anti-fraud checks activity before budget is spent and filters out suspicious impressions and clicks.", label: "Traffic verified", core: "FILTER", status: "ANTI-FRAUD PROTECTION ON" },
      { overline: "03 / CONTROL", title: "You stay in control at every stage", copy: "Choose CPC or CPM, set the audience and budget, add creatives and launch the campaign from one workspace.", label: "Campaign launched", core: "LAUNCH", status: "SETTINGS APPLIED" },
      { overline: "04 / GROWTH", title: "Data shows where your budget can work harder", copy: "Reporting, available-volume estimates and AI optimization help identify strong sources and scale campaigns with confidence.", label: "Top sources identified", core: "OPTIMIZE", status: "READY TO SCALE" },
    ],
  },
  es: {
    section: "02 / CÓMO FUNCIONA",
    kicker: "De la configuración al resultado",
    title: "Una plataforma para todo el recorrido de la campaña",
    output: "RESULTADO",
    layer: ["SISTEMA", "UNIFICADO"],
    sources: [["SITIOS", "INVENTARIO"], ["REDES", "TRÁFICO"], ["CAMPAÑA", "AJUSTES"]],
    steps: [
      { overline: "01 / ACCESO", title: "Una sola cuenta abre todo el mercado publicitario", copy: "TwinBid reúne el tráfico de cientos de redes. Con un único registro accedes al inventario de más de un millón de sitios web.", label: "Inventario conectado", core: "CONECTAR", status: "ACCESO A SITIOS HABILITADO" },
      { overline: "02 / CALIDAD", title: "El tráfico de baja calidad se bloquea antes de comprarlo", copy: "El sistema antifraude comprueba la actividad antes de gastar el presupuesto y descarta impresiones y clics sospechosos.", label: "Tráfico verificado", core: "FILTRAR", status: "PROTECCIÓN ANTIFRAUDE ACTIVA" },
      { overline: "03 / CONTROL", title: "Tú mantienes el control en cada etapa", copy: "Elige CPC o CPM, define la audiencia y el presupuesto, añade creatividades y lanza la campaña desde un solo panel.", label: "Campaña lanzada", core: "LANZAR", status: "AJUSTES APLICADOS" },
      { overline: "04 / CRECIMIENTO", title: "Los datos muestran dónde invertir mejor", copy: "Las estadísticas, la estimación del volumen disponible y la optimización con IA ayudan a encontrar las mejores fuentes y escalar con seguridad.", label: "Mejores fuentes detectadas", core: "OPTIMIZAR", status: "LISTO PARA ESCALAR" },
    ],
  },
  fr: {
    section: "02 / FONCTIONNEMENT",
    kicker: "Du paramétrage aux résultats",
    title: "Une plateforme pour tout le parcours de la campagne",
    output: "RÉSULTAT",
    layer: ["SYSTÈME", "UNIFIÉ"],
    sources: [["SITES", "INVENTAIRE"], ["RÉGIES", "TRAFIC"], ["CAMPAGNE", "RÉGLAGES"]],
    steps: [
      { overline: "01 / ACCÈS", title: "Un seul compte ouvre tout le marché publicitaire", copy: "TwinBid réunit le trafic de centaines de régies. Une inscription suffit pour accéder à l’inventaire de plus d’un million de sites.", label: "Inventaire connecté", core: "CONNECTER", status: "ACCÈS AUX SITES OUVERT" },
      { overline: "02 / QUALITÉ", title: "Le trafic de faible qualité est bloqué avant l’achat", copy: "Le système antifraude contrôle l’activité avant toute dépense et écarte les impressions et les clics suspects.", label: "Trafic vérifié", core: "FILTRER", status: "PROTECTION ANTIFRAUDE ACTIVE" },
      { overline: "03 / CONTRÔLE", title: "Vous gardez la main à chaque étape", copy: "Choisissez CPC ou CPM, définissez l’audience et le budget, ajoutez les créations et lancez la campagne depuis un seul espace.", label: "Campagne lancée", core: "LANCER", status: "RÉGLAGES APPLIQUÉS" },
      { overline: "04 / CROISSANCE", title: "Les données indiquent où investir davantage", copy: "Les rapports, l’estimation du volume disponible et l’optimisation par IA permettent d’identifier les meilleures sources et de déployer la campagne à plus grande échelle.", label: "Meilleures sources identifiées", core: "OPTIMISER", status: "PRÊT À CHANGER D’ÉCHELLE" },
    ],
  },
};

export function PlatformStory() {
  const { lang } = useLanguage();
  const reduceMotion = useReducedMotion();
  const text = storyCopy[lang] ?? storyCopy.en;
  const { active, sectionRef } = usePinnedScene(text.steps.length);
  const current = text.steps[active];

  return (
    <section ref={sectionRef} className="platform-story pinned-story" id="platform-story" aria-labelledby="platform-story-title">
      <div className="pinned-story-frame">
        <div className="landing-editorial-shell pinned-story-grid">
          <div className="pinned-story-copy">
            <div className="pinned-story-heading">
              <p className="landing-section-index">{text.section}</p>
              <p>{text.kicker}</p>
            </div>
            <div className="pinned-story-active" aria-live="polite">
              <AnimatePresence initial={false} mode="popLayout">
                <motion.div
                  className="pinned-story-active-panel"
                  key={`${lang}-${active}`}
                  initial={reduceMotion ? false : { opacity: 0, y: 22 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={reduceMotion ? undefined : { opacity: 0, y: -14 }}
                  transition={{ duration: reduceMotion ? 0 : 0.5, ease: [0.22, 1, 0.36, 1] }}
                >
                  <span>{current.overline}</span>
                  <h2 id="platform-story-title">{current.title}</h2>
                  <p>{current.copy}</p>
                </motion.div>
              </AnimatePresence>
            </div>
            <div className="pinned-story-progress" aria-hidden="true">
              {text.steps.map((step, index) => (
                <span className={index === active ? "is-active" : ""} key={step.overline}><i />0{index + 1}</span>
              ))}
            </div>
          </div>

          <div className="platform-machine" data-step={active} aria-live="polite">
            <div className="platform-machine-sources" aria-hidden="true">
              {text.sources.map(([first, second], sourceIndex) => (
                <div className={sourceIndex === Math.min(active, 2) ? "is-active" : ""} key={first}><small>0{sourceIndex + 1}</small><strong>{first}<br />{second}</strong><span>→</span></div>
              ))}
            </div>
            <div className="platform-machine-core">
              <img src={twinbidSymbol} alt="" />
              <AnimatePresence initial={false}>
                <motion.div
                  className="platform-machine-card-content"
                  key={`${lang}-${active}-core`}
                  initial={reduceMotion ? false : { opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={reduceMotion ? undefined : { opacity: 0 }}
                  transition={{ duration: reduceMotion ? 0 : 0.32, ease: "easeOut" }}
                >
                  <small>{current.core}</small>
                  <strong>{text.layer[0]}<br />{text.layer[1]}</strong>
                </motion.div>
              </AnimatePresence>
            </div>
            <div className="platform-machine-output">
              <AnimatePresence initial={false}>
                <motion.div
                  className="platform-machine-card-content"
                  key={`${lang}-${active}-output`}
                  initial={reduceMotion ? false : { opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={reduceMotion ? undefined : { opacity: 0 }}
                  transition={{ duration: reduceMotion ? 0 : 0.32, ease: "easeOut" }}
                >
                  <small>{text.output}</small>
                  <strong>{current.label}</strong>
                  <span><i />{current.status}</span>
                </motion.div>
              </AnimatePresence>
            </div>
          </div>
        </div>
      </div>

      <div className="pinned-story-mobile landing-editorial-shell">
        <div className="pinned-story-mobile-heading">
          <p className="landing-section-index">{text.section}</p>
          <h2>{text.title}</h2>
        </div>
        {text.steps.map((step) => (
          <article key={step.overline}>
            <span>{step.overline}</span>
            <h3>{step.title}</h3>
            <p>{step.copy}</p>
          </article>
        ))}
      </div>
    </section>
  );
}
