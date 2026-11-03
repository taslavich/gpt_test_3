import { useEffect, useState, type MouseEvent } from "react";
import { Menu, X } from "lucide-react";
import { AuthDialog } from "@/components/landing/AuthDialog";
import { LanguageSelector } from "@/components/LanguageSelector";
import { useLanguage } from "@/contexts/LanguageContext";
import twinbidLogo from "@/assets/twinbid-logo.svg";

export function PublisherHeader({ onBack, transitioning }: { onBack: (event: MouseEvent<HTMLElement>) => void; transitioning: boolean }) {
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);
  const { t, lang } = useLanguage();

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 24);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const navLinks = [
    { label: t("nav.benefits"), href: "#publisher-benefits" },
    { label: t("nav.formats"), href: "#publisher-formats" },
    { label: t("nav.howToStart"), href: "#publisher-steps" },
  ];
  const advertiserLabel = lang === "ru" ? "Рекламодателям" : lang === "es" ? "Anunciantes" : lang === "fr" ? "Annonceurs" : lang === "zh-CN" ? "广告主" : "Advertisers";
  const solid = scrolled || isMenuOpen;

  return (
    <header className="fixed inset-x-0 top-0 z-50 px-3 pt-3 md:px-6 md:pt-5">
      <div className={`landing-header-panel mx-auto max-w-[1320px] rounded-[22px] border transition-all duration-500 ${solid ? "border-white/10 bg-[hsl(200_18%_7%/0.92)] shadow-[0_18px_70px_rgba(0,0,0,0.4)] backdrop-blur-xl" : "border-white/[0.07] bg-background/45 backdrop-blur-md"}`}>
        <div className="relative flex h-[60px] items-center justify-between px-4 md:px-5">
          <a href="#publisher-top" className="relative z-10 flex shrink-0 items-center gap-2">
            <img src={twinbidLogo} alt="TwinBid" className="h-9 md:h-10" />
          </a>

          <nav className="absolute left-1/2 top-1/2 hidden -translate-x-1/2 -translate-y-1/2 items-center gap-1 min-[1440px]:flex">
            {navLinks.map((link) => (
              <a key={link.href} href={link.href} className="landing-header-nav-link">
                {link.label}
              </a>
            ))}
          </nav>

          <div className="hidden items-center gap-2 md:flex">
            <LanguageSelector className="landing-language-trigger" />
            <button type="button" onClick={onBack} disabled={transitioning} className="landing-header-login disabled:opacity-50">{advertiserLabel}</button>
            <AuthDialog defaultTab="login" defaultRole="publisher" trigger={<button className="landing-header-login">{t("nav.login")}</button>} />
            <AuthDialog defaultTab="register" defaultRole="publisher" trigger={<button className="landing-button landing-button-primary landing-header-register">{t("nav.register")}</button>} />
          </div>

          <button type="button" className="rounded-full border border-white/10 p-2 text-foreground md:hidden" onClick={() => setIsMenuOpen((open) => !open)} aria-label="Menu">
            {isMenuOpen ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>

        {isMenuOpen && (
          <div className="animate-fade-in border-t border-white/10 px-3 py-3 md:hidden">
            <nav className="flex flex-col gap-1">
              {navLinks.map((link) => (
                <a key={link.href} href={link.href} className="rounded-xl px-4 py-3 text-[15px] text-muted-foreground hover:bg-white/[0.05] hover:text-foreground" onClick={() => setIsMenuOpen(false)}>{link.label}</a>
              ))}
              <div className="mt-2 flex flex-col gap-2 border-t border-white/10 pt-3">
                <LanguageSelector className="landing-language-trigger landing-language-trigger-mobile" />
                <button type="button" onClick={(event) => { setIsMenuOpen(false); onBack(event); }} disabled={transitioning} className="landing-button landing-button-ghost w-full justify-center py-3 text-[15px] disabled:opacity-50">{advertiserLabel}</button>
                <AuthDialog defaultTab="login" defaultRole="publisher" trigger={<button className="landing-button landing-button-ghost min-h-12 w-full justify-center px-4 py-3 text-[15px]">{t("nav.login")}</button>} />
                <AuthDialog defaultTab="register" defaultRole="publisher" trigger={<button className="landing-button landing-button-primary min-h-12 w-full justify-center px-4 py-3 text-[15px]">{t("nav.register")}</button>} />
              </div>
            </nav>
          </div>
        )}
      </div>
    </header>
  );
}
