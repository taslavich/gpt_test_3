import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from "react";
import {
  ArrowDown,
  ArrowRight,
  BarChart3,
  Check,
  CircleDollarSign,
  Crosshair,
  Gauge,
  Globe2,
  Layers3,
  LineChart,
  MousePointerClick,
  Rocket,
  Search,
  Sparkles,
  Target,
  TrendingUp,
  UsersRound,
  Zap,
} from "lucide-react";
import { MoneyRegisterDialog } from "@/components/money/MoneyRegisterDialog";
import type { Lang } from "@/contexts/LanguageContext";
import { moneyText } from "./twinBidMoneyTranslations";
import "./twinbid-money.css";

const money = (value: number, maximumFractionDigits = 0) =>
  new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: maximumFractionDigits,
    maximumFractionDigits,
  }).format(value);

const number = (value: number) => new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 }).format(value);

interface SectionHeadingProps {
  index: string;
  eyebrow: string;
  title: ReactNode;
  description?: string;
  centered?: boolean;
}

function SectionHeading({ index, eyebrow, title, description, centered = false }: SectionHeadingProps) {
  return (
    <div className={centered ? "money-section-heading money-section-heading-centered" : "money-section-heading"}>
      <div className="money-eyebrow"><span>{index}</span>{eyebrow}</div>
      <h2>{title}</h2>
      {description && <p>{description}</p>}
    </div>
  );
}

interface CtaButtonProps {
  children: ReactNode;
  onClick: () => void;
  secondary?: boolean;
}

function CtaButton({ children, onClick, secondary = false }: CtaButtonProps) {
  return (
    <button type="button" onClick={onClick} className={secondary ? "money-cta money-cta-secondary" : "money-cta"}>
      <span>{children}</span>
      <ArrowRight aria-hidden="true" />
    </button>
  );
}

function ProfitChart({ tr }: { tr: (text: string) => string }) {
  return (
    <div className="money-chart">
      <div className="money-chart-head">
        <div>
          <span>{tr("CAMPAIGN REVENUE")}</span>
          <strong>$370</strong>
        </div>
        <div className="money-live"><i /> {tr("LIVE MODEL")}</div>
      </div>
      <div className="money-chart-grid">
        <svg viewBox="0 0 640 230" role="img" aria-label="Revenue rising from 100 dollars to 370 dollars">
          <defs>
            <linearGradient id="moneyArea" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#42e69a" stopOpacity="0.4" />
              <stop offset="100%" stopColor="#42e69a" stopOpacity="0" />
            </linearGradient>
            <filter id="moneyGlow" x="-20%" y="-20%" width="140%" height="140%">
              <feGaussianBlur stdDeviation="5" result="blur" />
              <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
            </filter>
          </defs>
          <path className="money-chart-area" d="M0 205 C60 198 88 185 132 188 C192 192 204 152 265 155 C322 158 352 125 405 118 C470 110 490 73 545 68 C588 63 615 35 640 18 L640 230 L0 230 Z" fill="url(#moneyArea)" />
          <path className="money-chart-line" d="M0 205 C60 198 88 185 132 188 C192 192 204 152 265 155 C322 158 352 125 405 118 C470 110 490 73 545 68 C588 63 615 35 640 18" fill="none" stroke="#42e69a" strokeWidth="4" strokeLinecap="round" filter="url(#moneyGlow)" />
          <circle cx="640" cy="18" r="7" fill="#42e69a" />
          <circle cx="640" cy="18" r="14" fill="none" stroke="#42e69a" strokeOpacity="0.25" />
        </svg>
        <div className="money-chart-start">{tr("$100 spend")}</div>
        <div className="money-chart-result">{tr("+$270 profit")}</div>
      </div>
      <div className="money-chart-stats">
        <div><span>{tr("SPEND")}</span><strong>$100</strong></div>
        <div><span>{tr("REVENUE")}</span><strong>$370</strong></div>
        <div className="profit"><span>ROI</span><strong>+270%</strong></div>
      </div>
    </div>
  );
}

const reasons = [
  { icon: Layers3, title: "Multiple traffic sources", text: "Reach advertising inventory from multiple providers through one TwinBid account." },
  { icon: Gauge, title: "Automatic CPM optimization", text: "Search for a more efficient buying price while preserving the traffic volume you need." },
  { icon: Globe2, title: "Worldwide traffic", text: "Launch campaigns across countries and audiences without switching platforms." },
  { icon: Crosshair, title: "Precise targeting", text: "Control GEO, device, operating system, browser and source-level settings." },
  { icon: MousePointerClick, title: "Multiple ad formats", text: "Test Popunder, Banner, Native, In-Page Push and other available formats." },
  { icon: BarChart3, title: "Detailed analytics", text: "See what spends, what converts and where the strongest opportunities are." },
];

const scenarios = [
  { name: "SMALL TEST", spend: 25, revenue: 42, profit: 17, roi: 68, tone: "soft" },
  { name: "WINNING CAMPAIGN", spend: 100, revenue: 370, profit: 270, roi: 270, tone: "hero" },
  { name: "SCALED CAMPAIGN", spend: 1000, revenue: 2450, profit: 1450, roi: 145, tone: "soft" },
];

export default function TwinBidMoney() {
  const [lang, setLang] = useState<Lang>("en");
  const tr = (text: string) => moneyText(lang, text);
  const [registerOpen, setRegisterOpen] = useState(false);
  const [budget, setBudget] = useState(100);
  const [visitorCost, setVisitorCost] = useState(0.01);
  const [conversionRate, setConversionRate] = useState(2);
  const [payout, setPayout] = useState(1.5);

  const calculation = useMemo(() => {
    const visitors = Math.floor(budget / visitorCost);
    const conversions = Math.floor(visitors * conversionRate / 100);
    const revenue = conversions * payout;
    const profit = revenue - budget;
    const roi = budget > 0 ? profit / budget * 100 : 0;
    return { visitors, conversions, revenue, profit, roi };
  }, [budget, visitorCost, conversionRate, payout]);

  useEffect(() => {
    const previousTitle = document.title;
    const previousLang = document.documentElement.lang;
    const description = document.querySelector<HTMLMetaElement>('meta[name="description"]');
    const previousDescription = description?.content;

    document.title = moneyText(lang, "Turn Internet Traffic Into Profit — TwinBid");
    document.documentElement.lang = lang;
    if (description) {
      description.content = moneyText(lang, "Buy targeted traffic, send visitors to affiliate offers and build profitable campaigns with TwinBid.");
    }

    return () => {
      document.title = previousTitle;
      document.documentElement.lang = previousLang;
      if (description && previousDescription) description.content = previousDescription;
    };
  }, [lang]);

  const openRegistration = () => setRegisterOpen(true);

  return (
    <div className="money-page" id="top">
      <div className="money-global-grid" aria-hidden="true" />
      <header className="money-header">
        <a href="#top" className="money-brand" aria-label="TwinBid money page">
          <span className="money-brand-mark">T</span>
          <span>TwinBid</span>
        </a>
        <div className="money-header-actions">
          <div className="money-language" aria-label="Language">
            {(["en", "ru", "es"] as const).map(code => (
              <button key={code} type="button" className={lang === code ? "active" : ""} onClick={() => setLang(code)}>{code.toUpperCase()}</button>
            ))}
          </div>
          <button type="button" className="money-header-cta" onClick={openRegistration}>
            {tr("START NOW")} <ArrowRight aria-hidden="true" />
          </button>
        </div>
      </header>

      <main>
        <section className="money-hero">
          <div className="money-hero-copy">
            <div className="money-hero-kicker"><span>{tr("ONE-DAY CAMPAIGN MODEL")}</span><i />{tr("ONE WINNING COMBINATION")}</div>
            <h1>{tr("Turn Internet Traffic")} <em>{tr("Into Profit")}</em></h1>
            <p className="money-hero-lead">
              {tr("Buy targeted visitors from around the world, send them to high-paying affiliate offers and keep the difference.")}
            </p>
            <div className="money-hero-equation" aria-label="100 dollars in traffic can model 370 dollars in revenue">
              <div><span>{tr("TRAFFIC")}</span><strong>$100</strong></div>
              <ArrowRight />
              <div><span>{tr("REVENUE")}</span><strong>$370</strong></div>
              <ArrowRight />
              <div className="profit"><span>{tr("YOUR PROFIT")}</span><strong>+$270</strong></div>
            </div>
            <div className="money-hero-actions">
              <CtaButton onClick={openRegistration}>{tr("START MAKING YOUR FIRST CAMPAIGN")}</CtaButton>
              <a href="#how" className="money-text-link">{tr("SEE HOW IT WORKS")} <ArrowDown /></a>
            </div>
            <div className="money-quick-points">
              <span><Check /> {tr("No product needed")}</span>
              <span><Check /> {tr("Worldwide traffic")}</span>
              <span><Check /> {tr("Start in minutes")}</span>
            </div>
          </div>
          <div className="money-hero-visual">
            <div className="money-roi-orbit"><span>+270%</span><small>{tr("ROI MODEL")}</small></div>
            <ProfitChart tr={tr} />
          </div>
        </section>

        <section className="money-strip" aria-label="How traffic arbitrage flows">
          <span>{tr("BUY TRAFFIC")}</span><i />
          <span>{tr("SEND TO OFFER")}</span><i />
          <span>{tr("GET CONVERSIONS")}</span><i />
          <span className="profit">{tr("KEEP THE DIFFERENCE")}</span>
        </section>

        <section className="money-section" id="how">
          <SectionHeading
            index="01"
            eyebrow={tr("THE MONEY FLOW")}
            title={<>{tr("How do people make money")} <span>{tr("with traffic?")}</span></>}
            description={tr("You do not need your own product. You connect an offer that pays with a stream of people ready to act.")}
          />
          <div className="money-steps">
            <article className="money-step-card">
              <div className="money-step-top"><span>01</span><Search /></div>
              <h3>{tr("Find an offer")}</h3>
              <p>{tr("Choose an affiliate offer that pays for registrations, purchases, installs or other actions.")}</p>
              <div className="money-step-example"><span>{tr("DATING OFFER PAYS")}</span><strong>{tr("$8 / registration")}</strong></div>
            </article>
            <div className="money-step-arrow"><ArrowRight /></div>
            <article className="money-step-card">
              <div className="money-step-top"><span>02</span><UsersRound /></div>
              <h3>{tr("Buy traffic")}</h3>
              <p>{tr("Launch a TwinBid campaign and send targeted visitors directly to your chosen offer.")}</p>
              <div className="money-step-example"><span>{tr("YOUR TEST BUDGET")}</span><strong>$100</strong></div>
            </article>
            <div className="money-step-arrow"><ArrowRight /></div>
            <article className="money-step-card money-step-profit">
              <div className="money-step-top"><span>03</span><TrendingUp /></div>
              <h3>{tr("Keep the profit")}</h3>
              <p>{tr("Thirty registrations at $8 generate $240. Subtract the $100 traffic cost.")}</p>
              <div className="money-step-example"><span>{tr("PROFIT")}</span><strong>+$140 · +140% ROI</strong></div>
            </article>
          </div>
          <div className="money-definition">
            <div><Sparkles /><span>{tr("THAT'S TRAFFIC ARBITRAGE")}</span></div>
            <p>{tr("TwinBid gives you the traffic. You find the winning combination.")}</p>
            <CtaButton onClick={openRegistration} secondary>{tr("TRY IT WITH TWINBID")}</CtaButton>
          </div>
        </section>

        <section className="money-section money-example-section">
          <SectionHeading
            index="02"
            eyebrow={tr("CAMPAIGN MODEL")}
            title={<>{tr("What can a winning campaign")} <span>{tr("look like?")}</span></>}
            description={tr("The goal is simple: make the revenue generated by your visitors exceed the price you paid to bring them in.")}
          />
          <div className="money-example-grid">
            <div className="money-example-dashboard">
              <div className="money-dashboard-bar">
                <div><i /><i /><i /></div>
                <span>{tr("EXAMPLE CAMPAIGN")}</span>
                <b>{tr("ACTIVE")}</b>
              </div>
              <div className="money-dashboard-title">
                <div><small>{tr("CAMPAIGN")}</small><strong>{tr("Global Offer Test #01")}</strong></div>
                <div className="money-dashboard-roi"><small>{tr("RETURN ON INVESTMENT")}</small><strong>+270%</strong></div>
              </div>
              <div className="money-dashboard-metrics">
                <div><span>{tr("AD SPEND")}</span><strong>$100</strong><small>{tr("starting capital")}</small></div>
                <div><span>{tr("VISITORS")}</span><strong>12,840</strong><small>{tr("targeted traffic")}</small></div>
                <div><span>{tr("CONVERSIONS")}</span><strong>37</strong><small>{tr("paid actions")}</small></div>
                <div><span>{tr("REVENUE")}</span><strong>$370</strong><small>{tr("affiliate payout")}</small></div>
              </div>
              <div className="money-dashboard-graph">
                <div className="money-bars" aria-hidden="true">
                  {[25, 32, 28, 46, 55, 48, 64, 72, 82, 100].map((height, index) => <i key={index} style={{ height: `${height}%` }} />)}
                </div>
                <div><span>{tr("NET PROFIT")}</span><strong>+$270</strong></div>
              </div>
            </div>
            <div className="money-in-out">
              <div className="money-in-out-item">
                <span>{tr("MONEY IN")}</span>
                <strong>$100</strong>
                <small>{tr("Traffic budget")}</small>
              </div>
              <ArrowDown />
              <div className="money-in-out-machine">
                <span>TWINBID</span>
                <div><i /><i /><i /><i /><i /></div>
                <small>{tr("Target · test · optimize")}</small>
              </div>
              <ArrowDown />
              <div className="money-in-out-item money-out">
                <span>{tr("REVENUE OUT")}</span>
                <strong>$370</strong>
                <small>{tr("+$270 net profit")}</small>
              </div>
            </div>
          </div>
        </section>

        <section className="money-section">
          <SectionHeading
            index="03"
            eyebrow={tr("YOUR TRAFFIC ENGINE")}
            title={<>{tr("Why buy traffic")} <span>{tr("with TwinBid?")}</span></>}
            description={tr("Everything you need to test, understand and scale paid traffic — inside one dashboard.")}
            centered
          />
          <div className="money-reasons">
            {reasons.map(({ icon: Icon, title, text }, index) => (
              <article key={title} className="money-reason-card">
                <div className="money-reason-index">0{index + 1}</div>
                <Icon />
                <h3>{tr(title)}</h3>
                <p>{tr(text)}</p>
              </article>
            ))}
          </div>
          <div className="money-focus-line">
            <span>{tr("YOU FIND THE PROFITABLE COMBINATION.")}</span>
            <strong>{tr("TWINBID HANDLES THE TRAFFIC.")}</strong>
          </div>
        </section>

        <section className="money-section money-first-test">
          <div className="money-first-test-copy">
            <div className="money-eyebrow"><span>04</span>{tr("BUILT FOR YOUR FIRST TEST")}</div>
            <h2>{tr("Never bought traffic before?")} <span>{tr("That’s fine.")}</span></h2>
            <p>{tr("You do not need a website, your own product or an audience. Start with one offer, one campaign and a controlled test budget.")}</p>
            <CtaButton onClick={openRegistration}>{tr("CREATE MY FIRST CAMPAIGN")}</CtaButton>
          </div>
          <div className="money-test-path">
            {[
              ["01", tr("Choose an offer"), tr("Pick what you want to promote")],
              ["02", tr("Create campaign"), tr("Set GEO, device and budget")],
              ["03", tr("Buy traffic"), tr("Send visitors to the offer")],
              ["04", tr("Track results"), tr("See sources and conversions")],
              ["05", tr("Scale winners"), tr("Increase what works")],
            ].map(([index, title, text]) => (
              <div key={index} className="money-path-row">
                <span>{index}</span><div><strong>{title}</strong><small>{text}</small></div><ArrowRight />
              </div>
            ))}
          </div>
        </section>

        <section className="money-section">
          <SectionHeading
            index="05"
            eyebrow={tr("THE MATH")}
            title={<>{tr("One formula.")} <span>{tr("Different scale.")}</span></>}
            description={tr("Test small, find a combination that works and put more budget behind the result.")}
            centered
          />
          <div className="money-scenarios">
            {scenarios.map((scenario) => (
              <article key={scenario.name} className={`money-scenario money-scenario-${scenario.tone}`}>
                <div className="money-scenario-name"><span>{tr(scenario.name)}</span>{scenario.tone === "hero" && <b>{tr("WINNING MODEL")}</b>}</div>
                <div className="money-scenario-row"><span>{tr("Traffic cost")}</span><strong>{money(scenario.spend)}</strong></div>
                <div className="money-scenario-row"><span>{tr("Revenue")}</span><strong>{money(scenario.revenue)}</strong></div>
                <div className="money-scenario-divider" />
                <div className="money-scenario-result"><span>{tr("PROFIT")}</span><strong>+{money(scenario.profit)}</strong></div>
                <div className="money-scenario-roi">+{scenario.roi}% ROI</div>
              </article>
            ))}
          </div>
        </section>

        <section className="money-section money-calculator-section" id="calculator">
          <SectionHeading
            index="06"
            eyebrow={tr("PROFIT CALCULATOR")}
            title={<>{tr("Build your own")} <span>{tr("campaign scenario.")}</span></>}
            description={tr("Move the controls and see how budget, traffic price, conversion rate and payout change the result.")}
          />
          <div className="money-calculator">
            <div className="money-calculator-controls">
              <CalculatorControl label={tr("ADVERTISING BUDGET")} value={money(budget)} min={25} max={2000} step={25} current={budget} onChange={setBudget} />
              <CalculatorControl label={tr("COST PER VISITOR")} value={money(visitorCost, 3)} min={0.005} max={0.1} step={0.005} current={visitorCost} onChange={setVisitorCost} />
              <CalculatorControl label={tr("CONVERSION RATE")} value={`${conversionRate.toFixed(1)}%`} min={0.1} max={10} step={0.1} current={conversionRate} onChange={setConversionRate} />
              <CalculatorControl label={tr("PAYOUT PER CONVERSION")} value={money(payout, 2)} min={0.25} max={20} step={0.25} current={payout} onChange={setPayout} />
            </div>
            <div className="money-calculator-output">
              <div className="money-output-label"><Zap /> {tr("YOUR CAMPAIGN MODEL")}</div>
              <div className="money-output-flow">
                <div><span>{tr("VISITORS")}</span><strong>{number(calculation.visitors)}</strong></div>
                <ArrowRight />
                <div><span>{tr("CONVERSIONS")}</span><strong>{number(calculation.conversions)}</strong></div>
                <ArrowRight />
                <div><span>{tr("REVENUE")}</span><strong>{money(calculation.revenue)}</strong></div>
              </div>
              <div className={calculation.profit >= 0 ? "money-potential-profit positive" : "money-potential-profit negative"}>
                <span>{tr("POTENTIAL PROFIT")}</span>
                <strong>{calculation.profit >= 0 ? "+" : ""}{money(calculation.profit)}</strong>
                <b>{calculation.roi >= 0 ? "+" : ""}{calculation.roi.toFixed(0)}% ROI</b>
              </div>
              <CtaButton onClick={openRegistration}>{tr("BUILD A CAMPAIGN LIKE THIS")}</CtaButton>
            </div>
          </div>
        </section>

        <section className="money-section money-use-cases">
          <SectionHeading
            index="07"
            eyebrow={tr("BUILT FOR ACTION")}
            title={<>{tr("What smart media buyers")} <span>{tr("focus on.")}</span></>}
            centered
          />
          <div className="money-use-grid">
            <article><Target /><span>{tr("FIND WINNERS")}</span><h3>{tr("Test multiple GEOs faster")}</h3><p>{tr("Compare audiences from one dashboard and move budget toward stronger combinations.")}</p></article>
            <article><LineChart /><span>{tr("READ THE DATA")}</span><h3>{tr("Know what works before scaling")}</h3><p>{tr("Use source-level statistics to separate traffic that performs from traffic that only spends.")}</p></article>
            <article><CircleDollarSign /><span>{tr("CONTROL THE TEST")}</span><h3>{tr("Start at a budget you control")}</h3><p>{tr("Build the first result, improve the campaign and scale only when the numbers make sense.")}</p></article>
          </div>
        </section>

        <section className="money-section money-now">
          <div className="money-now-orbit" aria-hidden="true"><i /><i /><i /></div>
          <div className="money-now-content">
            <div className="money-eyebrow"><span>08</span>{tr("YOUR MOVE")}</div>
            <h2>{tr("Stop watching other people buy traffic.")} <span>{tr("Start testing it yourself.")}</span></h2>
            <p>{tr("Every profitable campaign starts as a test. Launch, collect data, improve and scale what works.")}</p>
            <CtaButton onClick={openRegistration}>{tr("START MY FIRST TEST")}</CtaButton>
          </div>
        </section>

        <section className="money-final">
          <div className="money-final-badge"><Rocket /> {tr("READY FOR LAUNCH")}</div>
          <h2>{tr("Your first profitable campaign could start with")} <span>{tr("one test.")}</span></h2>
          <p>{tr("Get access to worldwide traffic through TwinBid and start testing offers today.")}</p>
          <CtaButton onClick={openRegistration}>{tr("CREATE FREE ACCOUNT")}</CtaButton>
          <div className="money-final-points">
            <span><Check /> {tr("One dashboard")}</span>
            <span><Check /> {tr("Multiple traffic sources")}</span>
            <span><Check /> {tr("Built-in optimization")}</span>
          </div>
        </section>
      </main>

      <footer className="money-footer">
        <a href="#top" className="money-brand"><span className="money-brand-mark">T</span><span>TwinBid</span></a>
        <p>© {new Date().getFullYear()} TwinBid. {tr("All rights reserved.")}</p>
        <div><a href="/legal#terms" target="_blank" rel="noreferrer">{tr("Terms")}</a><a href="/legal#privacy" target="_blank" rel="noreferrer">{tr("Privacy")}</a></div>
      </footer>

      <MoneyRegisterDialog open={registerOpen} onOpenChange={setRegisterOpen} lang={lang} />
    </div>
  );
}

interface CalculatorControlProps {
  label: string;
  value: string;
  min: number;
  max: number;
  step: number;
  current: number;
  onChange: (value: number) => void;
}

function CalculatorControl({ label, value, min, max, step, current, onChange }: CalculatorControlProps) {
  const progress = (current - min) / (max - min) * 100;
  return (
    <label className="money-control">
      <span className="money-control-head"><small>{label}</small><strong>{value}</strong></span>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={current}
        onChange={(event) => onChange(Number(event.target.value))}
        style={{ "--money-progress": `${progress}%` } as CSSProperties}
      />
      <span className="money-control-range"><small>{min}</small><small>{max}</small></span>
    </label>
  );
}
