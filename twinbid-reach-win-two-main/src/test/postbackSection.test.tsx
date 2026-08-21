import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { PostbackSection, POSTBACK_URL } from "@/components/dashboard/PostbackSection";
import { LanguageProvider } from "@/contexts/LanguageContext";

describe("PostbackSection", () => {
  beforeEach(() => {
    window.localStorage.removeItem("twinbid_lang");
  });

  it("explains required and optional postback parameters", () => {
    render(
      <LanguageProvider>
        <PostbackSection />
      </LanguageProvider>,
    );

    expect(screen.getByDisplayValue(POSTBACK_URL)).toBeInTheDocument();
    expect(screen.getByText("click_id")).toBeInTheDocument();
    expect(screen.getByText("payout")).toBeInTheDocument();
    expect(screen.getByText("status")).toBeInTheDocument();
    expect(screen.getByText(/Обязательный параметр/)).toBeInTheDocument();
    expect(screen.getAllByText(/Необязательный параметр/)).toHaveLength(2);
    expect(screen.getByText(/можно удалить из ссылки/)).toBeInTheDocument();
  });
});
