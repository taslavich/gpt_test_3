import { useState, useEffect } from "react";
import { Menu, X } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { LanguageSelector } from "@/components/LanguageSelector";
import { useLanguage } from "@/contexts/LanguageContext";
import { useLocation } from "react-router-dom";
import twinbidLogo from "@/assets/twinbid-logo.svg";

export function Header() {
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);
  const { t } = useLanguage();
  const location = useLocation();
  const isHome = location.pathname === "/";
  const sectionHref = (anchor: string) => isHome ? anchor : `/${anchor}`;

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 24);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const navLinks = [
    { label: t("nav.howWorks"), href: sectionHref("#platform-story") },
    { label: t("nav.formats"), href: sectionHref("#formats") },
    { label: t("nav.partners"), href: "/partners" },
    { label: t("nav.howToStart"), href: sectionHref("#steps") },
  ];

  const solid = scrolled || isMenuOpen;

  return (
    <header className="fixed inset-x-0 top-0 z-50 px-3 pt-3 md:px-6 md:pt-5">
      <div
        className={`landing-header-panel mx-auto max-w-[1320px] rounded-[22px] border transition-all duration-500 ${
          solid
            ? "border-white/10 bg-[hsl(200_18%_7%/0.9)] shadow-[0_18px_70px_rgba(0,0,0,0.4)] backdrop-blur-xl"
            : "border-white/[0.07] bg-background/45 backdrop-blur-md"
        }`}
      >
        <div className="relative flex h-[60px] items-center justify-between px-4 md:px-5">
          <a href="/" className="flex items-center gap-2 shrink-0 relative z-10">
            <img src={twinbidLogo} alt="TwinBid" className="h-9 md:h-10" />
          </a>

          <nav className="absolute left-1/2 top-1/2 hidden -translate-x-1/2 -translate-y-1/2 items-center gap-1 lg:flex">
            {navLinks.map((link) => (
              <a
                key={link.href}
                href={link.href}
                className="landing-header-nav-link"
              >
                {link.label}
              </a>
            ))}
          </nav>

          <div className="hidden items-center gap-2 md:flex">
            <LanguageSelector className="landing-language-trigger" />
            <AuthDialog
              trigger={
                <button className="landing-header-login">
                  {t("nav.login")}
                </button>
              }
              defaultTab="login"
            />
            <AuthDialog
              trigger={<button className="landing-button landing-button-primary landing-header-register">{t("nav.register")}</button>}
              defaultTab="register"
            />
          </div>

          <button className="rounded-full border border-white/10 p-2 text-foreground md:hidden" onClick={() => setIsMenuOpen(!isMenuOpen)} aria-label="Menu">
            {isMenuOpen ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>

        {isMenuOpen && (
          <div className="border-t border-white/10 px-3 py-3 md:hidden">
            <nav className="flex flex-col gap-1">
              {navLinks.map((link) => (
                <a key={link.href} href={link.href} className="rounded-xl px-4 py-3 text-[15px] text-muted-foreground hover:bg-white/[0.05] hover:text-foreground" onClick={() => setIsMenuOpen(false)}>
                  {link.label}
                </a>
              ))}
              <div className="mt-2 flex flex-col gap-2 border-t border-white/10 pt-3">
                <LanguageSelector className="landing-language-trigger landing-language-trigger-mobile" />
                <AuthDialog trigger={<button className="landing-button landing-button-ghost w-full justify-center text-[15px]">{t("nav.login")}</button>} defaultTab="login" />
                <AuthDialog trigger={<button className="landing-button landing-button-primary w-full justify-center text-[15px]">{t("nav.register")}</button>} defaultTab="register" />
              </div>
            </nav>
          </div>
        )}
      </div>
    </header>
  );
}
