import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { LanguageProvider, useLanguage } from "@/contexts/LanguageContext";

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
});
