import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { LanguageProvider, languageFromBrowser, useLanguage } from "@/contexts/LanguageContext";
import { ES_TRANSLATIONS } from "@/lib/translations-es";
import { FR_TRANSLATIONS } from "@/lib/translations-fr";
import { ZH_TRANSLATIONS } from "@/lib/translations-zh";
import { PUBLISHER_TRANSLATIONS } from "@/lib/publisherTranslations";
import { ZH_PUBLISHER_TRANSLATIONS } from "@/lib/publisherTranslationsZh";
import { LEGAL_CONTENT } from "@/lib/legalContent";

function CurrentLanguage() {
  const { lang, setLang, t } = useLanguage();
  return <><span>{lang}</span><span>{t("nav.login")}</span><button onClick={() => setLang("ru")}>Select Russian</button></>;
}

describe("LanguageProvider", () => {
  beforeEach(() => {
    window.localStorage.removeItem("twinbid_lang");
  });

  afterEach(() => {
    Reflect.deleteProperty(navigator, "languages");
  });

  it("selects simplified Chinese for a new visitor with a Chinese browser locale", () => {
    Object.defineProperty(navigator, "languages", { configurable: true, value: ["zh-Hans-CN", "en-US"] });
    render(
      <LanguageProvider>
        <CurrentLanguage />
      </LanguageProvider>,
    );

    expect(screen.getByText("zh-CN")).toBeInTheDocument();
    expect(screen.getByText("登录")).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("zh-CN");
  });

  it("follows the browser language order and falls back to English", () => {
    expect(languageFromBrowser(["de-DE", "ru-RU", "zh-CN"])).toBe("ru");
    expect(languageFromBrowser(["zh-TW", "en-US"])).toBe("zh-CN");
    expect(languageFromBrowser(["es-MX"])).toBe("es");
    expect(languageFromBrowser(["fr-CA"])).toBe("fr");
    expect(languageFromBrowser(["de-DE"])).toBe("en");
  });

  it("keeps an existing saved language choice", () => {
    Object.defineProperty(navigator, "languages", { configurable: true, value: ["zh-CN"] });
    window.localStorage.setItem("twinbid_lang", "en");

    render(
      <LanguageProvider>
        <CurrentLanguage />
      </LanguageProvider>,
    );

    expect(screen.getByText("en")).toBeInTheDocument();
    expect(screen.getByText("Log in")).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("en");
  });

  it("keeps a manual language change for future visits", () => {
    Object.defineProperty(navigator, "languages", { configurable: true, value: ["zh-CN"] });
    render(<LanguageProvider><CurrentLanguage /></LanguageProvider>);
    fireEvent.click(screen.getByText("Select Russian"));
    expect(screen.getByText("ru")).toBeInTheDocument();
    expect(window.localStorage.getItem("twinbid_lang")).toBe("ru");
    expect(document.documentElement.lang).toBe("ru");
  });

  it("restores French and exposes a complete translated interface", () => {
    window.localStorage.setItem("twinbid_lang", "fr");

    render(
      <LanguageProvider>
        <CurrentLanguage />
      </LanguageProvider>,
    );

    expect(screen.getByText("fr")).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("fr");
    expect(Object.keys(ES_TRANSLATIONS).every(key => FR_TRANSLATIONS[key] !== undefined)).toBe(true);
  });

  it("preserves interpolation placeholders in French translations", () => {
    const placeholders = (value: string) => [...value.matchAll(/\{[^}]+\}/g)].map(match => match[0]).sort();

    Object.entries(ES_TRANSLATIONS).forEach(([key, value]) => {
      expect(placeholders(FR_TRANSLATIONS[key]), key).toEqual(placeholders(value));
    });
  });

  it("covers every Spanish and French overlay key in Chinese", () => {
    for (const key of new Set([...Object.keys(ES_TRANSLATIONS), ...Object.keys(FR_TRANSLATIONS)])) {
      expect(ZH_TRANSLATIONS[key], key).toBeDefined();
    }
  });

  it("translates publisher keys and preserves integration-code placeholders", () => {
    const placeholders = (value: string) => [...value.matchAll(/\{[^}]+\}/g)].map(match => match[0]).sort();
    for (const [key, original] of Object.entries(PUBLISHER_TRANSLATIONS)) {
      const translated = ZH_PUBLISHER_TRANSLATIONS[key];
      expect(translated, key).toBeDefined();
      expect(placeholders(translated), key).toEqual(placeholders(original.en));
    }
    window.localStorage.setItem("twinbid_lang", "zh-CN");
    function PublisherLabel() {
      const { t } = useLanguage();
      return <span>{t("pub.sidebar.adCodes")}</span>;
    }
    render(<LanguageProvider><PublisherLabel /></LanguageProvider>);
    expect(screen.getByText("广告代码")).toBeInTheDocument();
  });

  it("provides Chinese legal sections with the same structure as the originals", () => {
    for (const section of ["terms", "privacy"] as const) {
      const original = LEGAL_CONTENT.en[section].sections;
      const translated = LEGAL_CONTENT["zh-CN"][section].sections;
      expect(translated).toHaveLength(original.length);
      translated.forEach((block, index) => {
        expect(block.paragraphs?.length ?? 0).toBe(original[index].paragraphs?.length ?? 0);
        expect(block.list?.length ?? 0).toBe(original[index].list?.length ?? 0);
      });
    }
  });
});
