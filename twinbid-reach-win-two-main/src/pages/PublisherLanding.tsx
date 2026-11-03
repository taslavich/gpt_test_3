import { useEffect, useRef, useState } from "react";
import { ArrowDown, ArrowRight, Globe2, Gauge, Layers3, BarChart3, Headphones, MousePointerClick } from "lucide-react";
import { MotionConfig } from "framer-motion";
import { PublisherHeader } from "@/components/publisher-landing/PublisherHeader";
import { PublisherFormatPreview } from "@/components/publisher-landing/PublisherFormatPreview";
import { Footer } from "@/components/landing/Footer";
import { AnimatedBackground } from "@/components/landing/AnimatedBackground";
import { SignalFlowField } from "@/components/landing/SignalFlowField";
import { AuthDialog } from "@/components/landing/AuthDialog";
import { RolePortalReveal, useRolePortalTransition } from "@/components/landing/RolePortalTransition";
import { useLanguage } from "@/contexts/LanguageContext";
import { useIsMobileImmediate } from "@/hooks/use-mobile";
import type { ZoneFormat } from "@/contexts/PublisherContext";
import twinbidMark from "@/assets/twinbid-mark.svg";

const formats: { name: string; format: ZoneFormat }[] = [
  { name: "Popunder", format: "popunder" },
  { name: "Native Ads", format: "native" },
  { name: "Banner", format: "banner" },
  { name: "In-page Push", format: "inpage_push" },
  { name: "Video Pop-up", format: "video_popup" },
  { name: "In-Stream VAST", format: "instream" },
  { name: "Out-Stream", format: "outstream" },
];
const benefitIcons = [Layers3, Gauge, MousePointerClick, Globe2, BarChart3, Headphones];
const copy = {
  ru: {
    heroKicker: "МОНЕТИЗАЦИЯ САЙТОВ / TWINBID",
    back: "Вернуться к рекламодателям",
    scroll: "Листайте дальше",
    proofLead: "Разместите код.", proofAccent: "Начните зарабатывать.",
    proofNote: "Получите готовый рекламный код и добавьте его на сайт. После первых показов трафик начнёт приносить доход.",
    consoleTop: "TwinBid / Площадки", consoleLabel: "Обзор монетизации", consoleChart: "Динамика показов", consoleActive: "АКТИВНО", consoleNote: "Пример интерфейса TwinBid",
    proofFacts: ["до первой прибыли", "комиссии на трафик партнёрских сетей", "от стоимости рекламы TwinBid — паблишеру"],
    benefitIndex: "02 / ВОЗМОЖНОСТИ", benefitLead: "Получайте максимальный", benefitAccent: "доход с вашего трафика", benefitNote: "Подключайте площадки, управляйте форматами и следите за результатом в одном кабинете.",
    formatIndex: "03 / ФОРМАТЫ", formatLead: "Форматы", formatAccent: "монетизации", formatNote: "Выберите формат для каждого места на сайте. От баннера до видео.", formatPreview: "ПРЕДПРОСМОТР ФОРМАТА",
    stepsIndex: "04 / КАК НАЧАТЬ", stepsLead: "Три шага", stepsAccent: "до дохода", stepsNote: "Зарегистрируйтесь, возьмите готовый рекламный код и разместите его на сайте. Больше ничего подключать не нужно.",
    launchLabel: "БЫСТРЫЙ СТАРТ", launchTime: "3 минуты", launchPromise: "до первой прибыли", launchNote: "Без долгих проверок и ожидания модерации. Разместите код — и сайт готов к монетизации.",
    ctaLead: "Ваш трафик.", ctaAccent: "Ваш доход.", ctaText: "Подключите сайт к TwinBid и управляйте монетизацией в одном кабинете.", ctaContact: "Написать менеджеру", ctaTape: "ПЛОЩАДКА → РЕКЛАМНЫЙ КОД → МОНЕТИЗАЦИЯ",
  },
  en: {
    heroKicker: "PUBLISHER MONETIZATION / TWINBID",
    back: "Back to advertisers", scroll: "Keep exploring",
    proofLead: "Place the code.", proofAccent: "Start earning.",
    proofNote: "Get a ready-made ad code and add it to your website. Your traffic can start earning after the first ad impressions.",
    consoleTop: "TwinBid / Publishers", consoleLabel: "Monetization overview", consoleChart: "Impression trends", consoleActive: "ACTIVE", consoleNote: "Illustrative TwinBid interface",
    proofFacts: ["to your first earnings", "commission on partner-network traffic", "of TwinBid ad spend paid to publishers"],
    benefitIndex: "02 / CAPABILITIES", benefitLead: "Maximize the revenue", benefitAccent: "from your traffic", benefitNote: "Connect websites, manage formats and follow the results in one dashboard.",
    formatIndex: "03 / FORMATS", formatLead: "Monetization", formatAccent: "formats", formatNote: "Pick the right format for each placement, from banners to video.", formatPreview: "FORMAT PREVIEW",
    stepsIndex: "04 / GET STARTED", stepsLead: "Three steps", stepsAccent: "to earnings", stepsNote: "Sign up, grab a ready-made ad code and place it on your site. There is nothing else to integrate.",
    launchLabel: "QUICK START", launchTime: "3 minutes", launchPromise: "to your first earnings", launchNote: "No lengthy checks or moderation queues. Place the code and your site is ready to monetize.",
    ctaLead: "Your traffic.", ctaAccent: "Your revenue.", ctaText: "Connect your website to TwinBid and manage monetization from one dashboard.", ctaContact: "Talk to a manager", ctaTape: "WEBSITE → AD CODE → REVENUE",
  },
  zh: {
    heroKicker: "网站变现 / TWINBID",
    back: "返回广告主页面", scroll: "继续浏览",
    proofLead: "安装代码。", proofAccent: "开始获利。",
    proofNote: "获取现成的广告代码并添加到网站。首次展示后，流量即可开始带来收益。",
    consoleTop: "TwinBid / 发布商", consoleLabel: "变现概览", consoleChart: "展示趋势", consoleActive: "已启用", consoleNote: "TwinBid 界面示意",
    proofFacts: ["获得首笔收益", "合作网络流量佣金", "TwinBid 广告支出支付给发布商"],
    benefitIndex: "02 / 功能", benefitLead: "让您的流量", benefitAccent: "创造更多收益", benefitNote: "在一个控制台中连接网站、管理广告形式并查看结果。",
    formatIndex: "03 / 广告形式", formatLead: "网站", formatAccent: "变现形式", formatNote: "为网站上的每个展示位置选择合适的形式，从横幅到视频。", formatPreview: "广告形式预览",
    stepsIndex: "04 / 开始使用", stepsLead: "只需三步", stepsAccent: "开始获利", stepsNote: "注册、获取现成的广告代码并安装到网站，无需额外接入。",
    launchLabel: "快速开始", launchTime: "3 分钟", launchPromise: "获得首笔收益", launchNote: "无需长时间审核或排队等待。安装代码后，网站即可开始变现。",
    ctaLead: "您的流量。", ctaAccent: "您的收益。", ctaText: "将网站接入 TwinBid，并在一个控制台中管理变现。", ctaContact: "联系经理", ctaTape: "网站 → 广告代码 → 变现",
  },
};

function PublisherProof() {
  const { t, lang } = useLanguage();
  const c = lang === "ru" ? copy.ru : lang === "zh-CN" ? copy.zh : copy.en;
  const tape = lang === "ru"
    ? ["ПЛОЩАДКИ", "3 МИНУТЫ ДО ПЕРВОЙ ПРИБЫЛИ", "7 ФОРМАТОВ", "БЫСТРЫЙ СТАРТ", "СТАТИСТИКА", "РЕКЛАМНЫЕ КОДЫ"]
    : lang === "zh-CN"
    ? ["发布商", "3 分钟获得首笔收益", "7 种形式", "快速开始", "统计数据", "广告代码"]
    : ["PUBLISHERS", "3 MINUTES TO FIRST EARNINGS", "7 FORMATS", "QUICK START", "REPORTING", "AD CODES"];
  const commissionValue = lang === "ru" ? "до 0%" : lang === "zh-CN" ? "低至 0%" : "as low as 0%";
  const threeMinutes = lang === "ru" ? "3 мин" : lang === "zh-CN" ? "3 分钟" : "3 min";
  const bars = [36, 45, 40, 58, 54, 65, 61, 76, 72, 89];
  return (
    <section id="publisher-proof" className="signal-proof publisher-proof">
      <div className="signal-tape" aria-hidden="true"><div className="signal-tape-track">
        {Array.from({ length: 8 }, (_, group) => <div className="signal-tape-group" key={group}>{tape.map((item, index) => <span className="signal-tape-item" data-tone={index % 4} key={`${group}-${item}`}><strong>{item}</strong></span>)}</div>)}
      </div></div>
      <div className="landing-editorial-shell signal-proof-intro">
        <div className="signal-proof-copy"><p>{c.proofLead}<br /><strong>{c.proofAccent}</strong></p><span>{c.proofNote}</span></div>
        <div className="signal-console" aria-label={c.consoleNote}>
          <div className="signal-console-bar"><div><i /><span>{c.consoleTop}</span></div><span className="signal-console-live"><i />{c.consoleActive}</span></div>
          <div className="signal-console-body">
            <aside className="signal-console-sidebar" aria-hidden="true"><img src={twinbidMark} alt="" /><span className="active"><BarChart3 /></span><span><Globe2 /></span><span><Layers3 /></span></aside>
            <div className="signal-console-main">
              <div className="signal-console-heading"><div><small>{c.consoleTop}</small><strong>{c.consoleLabel}</strong></div></div>
              <div className="signal-console-metrics">
                <div><span>{c.proofFacts[0]}</span><strong>{threeMinutes}</strong><small>{c.consoleActive}</small></div>
                <div><span>{c.proofFacts[1]}</span><strong>{commissionValue}</strong><small>{c.consoleActive}</small></div>
                <div><span>{c.proofFacts[2]}</span><strong>80%+</strong><small>{c.consoleActive}</small></div>
              </div>
              <div className="signal-console-chart"><div><span>{c.consoleChart}</span><strong>TwinBid</strong></div><div className="signal-console-bars" aria-hidden="true">{bars.map((height, index) => <i style={{ height: `${height}%` }} key={index} />)}</div></div>
            </div>
          </div>
          <small className="signal-console-note">{c.consoleNote}</small>
        </div>
      </div>
      <div className="signal-proof-facts"><div className="landing-editorial-shell signal-proof-grid">
        {[threeMinutes, commissionValue, "80%+"].map((value, index) => <div key={index}><strong>{value}</strong><span>{c.proofFacts[index]}</span></div>)}
      </div></div>
    </section>
  );
}

function PublisherBenefits() {
  const { t, lang } = useLanguage();
  const c = lang === "ru" ? copy.ru : lang === "zh-CN" ? copy.zh : copy.en;
  return (
    <section id="publisher-benefits" className="platform-story publisher-benefits" aria-labelledby="publisher-benefits-title">
      <div className="landing-editorial-shell">
        <div className="platform-story-heading"><p className="landing-section-index">{c.benefitIndex}</p><h2 id="publisher-benefits-title">{c.benefitLead}<br /><span>{c.benefitAccent}</span></h2><p>{c.benefitNote}</p></div>
        <div className="publisher-benefits-grid">
          {benefitIcons.map((Icon, index) => <article key={index}>
            <div className="publisher-benefit-head"><span>0{index + 1}</span><Icon aria-hidden="true" /></div>
            <h3>{t(`pubLanding.benefit.${index + 1}.title`)}</h3>
            <p>{t(`pubLanding.benefit.${index + 1}.desc`)}</p>
          </article>)}
        </div>
      </div>
    </section>
  );
}

function PublisherFormats() {
  const { t, lang } = useLanguage();
  const c = lang === "ru" ? copy.ru : lang === "zh-CN" ? copy.zh : copy.en;
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const items = listRef.current?.querySelectorAll<HTMLElement>("article[data-format-index]");
    if (!items || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(entries => {
      const visible = entries.find(entry => entry.isIntersecting);
      if (visible) setActive(Number((visible.target as HTMLElement).dataset.formatIndex));
    }, { rootMargin: "-35% 0px -35% 0px" });
    items.forEach(item => observer.observe(item));
    return () => observer.disconnect();
  }, []);
  return (
    <section id="publisher-formats" className="formats-editorial publisher-formats" aria-labelledby="publisher-formats-title">
      <div className="landing-editorial-shell">
        <div className="formats-editorial-heading"><p className="landing-section-index">{c.formatIndex}</p><h2 id="publisher-formats-title">{c.formatLead}<br /><span>{c.formatAccent}</span></h2><p>{c.formatNote}</p></div>
        <div className="formats-editorial-grid">
          <div className="formats-editorial-list" ref={listRef}>{formats.map(({ name, format }, index) => <article className={active === index ? "is-active" : ""} key={format} data-format-index={index} tabIndex={0} onMouseEnter={() => setActive(index)} onFocus={() => setActive(index)}>
            <div><span>{String(index + 1).padStart(2, "0")}</span>{c.formatPreview}</div>
            <h3>{lang === "zh-CN" ? t(`pub.zone.format.${format}`) : name}</h3><p>{t(`pubLanding.format.${index + 1}.desc`)}</p>
            <div className="formats-editorial-mobile-scene"><PublisherFormatPreview format={format} /></div>
          </article>)}</div>
          <div className="formats-editorial-stage-wrap"><div className="formats-editorial-stage publisher-format-sticky">
            <div className="formats-editorial-stage-label"><span>{c.formatPreview}</span><strong>{String(active + 1).padStart(2, "0")} / 07</strong></div>
            <PublisherFormatPreview format={formats[active].format} key={formats[active].format} />
            <div className="formats-editorial-stage-foot"><strong>{lang === "zh-CN" ? t(`pub.zone.format.${formats[active].format}`) : formats[active].name}</strong></div>
          </div></div>
        </div>
      </div>
    </section>
  );
}

function PublisherSteps() {
  const { t, lang } = useLanguage();
  const c = lang === "ru" ? copy.ru : lang === "zh-CN" ? copy.zh : copy.en;
  return (
    <section id="publisher-steps" className="launch-section publisher-launch" aria-labelledby="publisher-steps-title"><div className="landing-editorial-shell">
      <div className="launch-heading"><p className="landing-section-index">{c.stepsIndex}</p><h2 id="publisher-steps-title">{c.stepsLead}<br /><span>{c.stepsAccent}</span></h2><p>{c.stepsNote}</p></div>
      <div className="launch-grid">
        <div className="publisher-launch-offer publisher-launch-fast"><span>{c.launchLabel}</span><div className="publisher-launch-promise"><strong>{c.launchTime}</strong><span>{c.launchPromise}</span></div><p>{c.launchNote}</p></div>
        <div className="launch-steps">{[1, 2, 3].map(index => <article key={index}><span>0{index}</span><div><h3>{t(`pubLanding.step.${index}.title`)}</h3><p>{t(`pubLanding.step.${index}.desc`)}</p></div></article>)}
          <AuthDialog defaultTab="register" defaultRole="publisher" trigger={<button className="landing-button landing-button-primary launch-button">{t("pubLanding.hero.cta")}<ArrowRight aria-hidden="true" /></button>} />
        </div>
      </div>
    </div></section>
  );
}

function PublisherCTA() {
  const { t, lang } = useLanguage();
  const c = lang === "ru" ? copy.ru : lang === "zh-CN" ? copy.zh : copy.en;
  return (
    <section className="editorial-cta publisher-cta" aria-labelledby="publisher-cta-title">
      <div className="editorial-cta-backdrop" aria-hidden="true"><div className="editorial-cta-backdrop-track"><span>{c.ctaTape}</span><span>{c.ctaTape}</span></div></div>
      <div className="editorial-cta-signal" aria-hidden="true"><i /><i /><i /><i /><i /></div>
      <div className="landing-editorial-shell editorial-cta-inner">
        <h2 id="publisher-cta-title" className="editorial-cta-title"><span>{c.ctaLead}</span><strong>{c.ctaAccent}</strong></h2>
        <div className="editorial-cta-bottom"><div className="editorial-cta-copy"><p>{c.ctaText}</p></div><div className="editorial-cta-actions">
          <AuthDialog defaultTab="register" defaultRole="publisher" trigger={<button className="landing-button landing-button-primary">{t("pubLanding.cta.connect")}<ArrowRight aria-hidden="true" /></button>} />
          <a href="https://t.me/GregTwinbid" target="_blank" rel="noopener noreferrer" className="landing-button landing-button-ghost">{c.ctaContact}<ArrowRight aria-hidden="true" /></a>
        </div></div>
      </div>
    </section>
  );
}

export default function PublisherLanding() {
  const { t, lang } = useLanguage();
  const c = lang === "ru" ? copy.ru : lang === "zh-CN" ? copy.zh : copy.en;
  const isMobile = useIsMobileImmediate();
  const { openPortal, portalOverlay, transitioning } = useRolePortalTransition();
  return (
    <MotionConfig reducedMotion={isMobile ? "always" : "never"}>
      <div className="landing-shell publisher-landing min-h-screen bg-background">
        <RolePortalReveal />
        <AnimatedBackground />
        <PublisherHeader onBack={event => openPortal(event, "/")} transitioning={transitioning} />
        <main>
          <section id="publisher-top" className="landing-obsidian-hero publisher-editorial-hero" aria-labelledby="publisher-hero-title">
            <SignalFlowField /><div className="landing-hero-shade" aria-hidden="true" /><div className="landing-hero-noise" aria-hidden="true" />
            <div className="landing-editorial-shell landing-obsidian-content">
              <p className="landing-obsidian-kicker"><span />{c.heroKicker}</p>
              <h1 id="publisher-hero-title" className="landing-obsidian-title publisher-editorial-title">
                <span className="landing-title-row"><span>{t("pubLanding.hero.title1")}</span></span>
                <span className="landing-title-row landing-title-row-accent"><span>{t("pubLanding.hero.title2")}</span></span>
              </h1>
              <div className="landing-obsidian-bottom"><p>{t("pubLanding.hero.subtitle")}</p><div className="landing-obsidian-actions">
                <AuthDialog defaultTab="register" defaultRole="publisher" trigger={<button className="landing-button landing-button-primary landing-hero-cta">{t("pubLanding.hero.cta")}<ArrowRight aria-hidden="true" /></button>} />
                <button type="button" onClick={event => openPortal(event, "/")} disabled={transitioning} className="landing-hero-text-link publisher-back-link">{c.back}<ArrowRight aria-hidden="true" /></button>
              </div></div>
              <div className="landing-obsidian-metric" aria-label={`${c.launchTime} ${c.proofFacts[0]} ${lang === "ru" ? "с TwinBid" : lang === "zh-CN" ? "使用 TwinBid" : "with TwinBid"}`}><strong>{lang === "ru" ? "3 мин" : lang === "zh-CN" ? "3 分钟" : "3 min"}</strong><span>{c.proofFacts[0]}<br />{lang === "ru" ? "с TwinBid" : lang === "zh-CN" ? "使用 TwinBid" : "with TwinBid"}</span></div>
            </div>
            <a className="landing-hero-scroll" href="#publisher-proof"><span>{c.scroll}</span><ArrowDown aria-hidden="true" /></a>
          </section>
          <PublisherProof />
          <PublisherBenefits />
          <PublisherFormats />
          <PublisherSteps />
          <PublisherCTA />
        </main>
        <Footer />
        {portalOverlay}
      </div>
    </MotionConfig>
  );
}
