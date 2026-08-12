import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { LanguageProvider, useLanguage } from "@/contexts/LanguageContext";
import { ES_TRANSLATIONS } from "@/lib/translations-es";
import { FR_TRANSLATIONS } from "@/lib/translations-fr";

function CurrentLanguage() {
  const { lang } = useLanguage();
  return <span>{lang}</span>;
}

describe("LanguageProvider", () => {
  beforeEach(() => {
    window.localStorage.removeItem("twinbid_lang");
  });

  it("uses Russian by default for a new visitor", () => {
    render(
      <LanguageProvider>
        <CurrentLanguage />
      </LanguageProvider>,
    );

    expect(screen.getByText("ru")).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("ru");
  });

  it("keeps an existing saved language choice", () => {
    window.localStorage.setItem("twinbid_lang", "en");

    render(
      <LanguageProvider>
        <CurrentLanguage />
      </LanguageProvider>,
    );

    expect(screen.getByText("en")).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("en");
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
});
