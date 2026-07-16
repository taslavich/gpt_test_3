import { useState, useEffect } from "react";
import { Menu, X } from "lucide-react";
import { AuthDialog } from "./AuthDialog";
import { LanguageSelector } from "@/components/LanguageSelector";
import { useLanguage } from "@/contexts/LanguageContext";
import twinbidLogo from "@/assets/twinbid-logo.svg";

export function Header() {
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);
  const { t } = useLanguage();

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 24);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const navLinks = [
    { label: t("nav.benefits"), href: "#benefits" },
    { label: t("nav.formats"), href: "#formats" },
    { label: t("nav.howToStart"), href: "#steps" },
  ];

  const solid = scrolled || isMenuOpen;

  return (
    <header
      className={`fixed top-0 left-0 right-0 z-50 transition-all duration-500 ${
        solid ? "backdrop-blur-md bg-background/65 border-b border-border/50" : "bg-transparent"
      }`}
    >
      <div className="container mx-auto px-8">
        <div className="relative flex items-center justify-between h-20">
          <a href="#" className="flex items-center gap-2 shrink-0 relative z-10">
            <img src={twinbidLogo} alt="TwinBid" className="h-[42px]" />
          </a>

          <nav className="hidden md:flex items-center gap-1 absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2">
            {navLinks.map((link) => (
              <a
                key={link.href}
                href={link.href}
                className="text-[14px] font-mono-eyebrow uppercase tracking-[0.22em] text-muted-foreground hover:text-foreground transition-colors px-3 py-2"
              >
                {link.label}
              </a>
            ))}
          </nav>

          <div className="hidden md:flex items-center gap-3">
            <LanguageSelector className="text-[14px] h-9 px-2.5" />
            <AuthDialog
              trigger={
                <button className="text-[14px] font-mono-eyebrow uppercase tracking-[0.22em] text-muted-foreground hover:text-foreground transition-colors px-3 py-2">
                  {t("nav.login")}
                </button>
              }
              defaultTab="login"
            />
            <AuthDialog
              trigger={<button className="pill pill-primary text-[15px] px-6 py-2.5">{t("nav.register")}</button>}
              defaultTab="register"
            />
          </div>

          <button className="md:hidden p-2 text-foreground" onClick={() => setIsMenuOpen(!isMenuOpen)}>
            {isMenuOpen ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>

        {isMenuOpen && (
          <div className="md:hidden py-4 border-t border-border animate-fade-in">
            <nav className="flex flex-col gap-2">
              {navLinks.map((link) => (
                <a key={link.href} href={link.href} className="pill pill-ghost text-left text-[16px]" onClick={() => setIsMenuOpen(false)}>
                  {link.label}
                </a>
              ))}
              <div className="flex flex-col gap-2 pt-4 border-t border-border">
                <LanguageSelector className="text-[14px] h-9 px-3 w-full justify-center" />
                <AuthDialog trigger={<button className="pill pill-ghost w-full justify-center text-[16px]">{t("nav.login")}</button>} defaultTab="login" />
                <AuthDialog trigger={<button className="pill pill-primary w-full justify-center text-[16px] px-5 py-2.5">{t("nav.register")}</button>} defaultTab="register" />
              </div>
            </nav>
          </div>
        )}
      </div>
    </header>
  );
}
