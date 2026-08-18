// @vitest-environment jsdom
import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { CreativesEditor } from "@/components/dashboard/CreativesEditor";
import { LanguageProvider } from "@/contexts/LanguageContext";
import type { Creative } from "@/contexts/CampaignContext";

function Harness({ formatKey = "popunder" }: { formatKey?: string }) {
  const [creatives, setCreatives] = useState<Creative[]>([{
    id: "creative-1",
    name: "Creative",
    url: "",
  }]);

  return (
    <LanguageProvider>
      <CreativesEditor
        formatKey={formatKey}
        creatives={creatives}
        onChange={setCreatives}
      />
    </LanguageProvider>
  );
}

describe("creative URL macros", () => {
  beforeEach(() => {
    window.localStorage.setItem("twinbid_lang", "en");
  });

  it("shows click_id immediately after the first URL character is entered", () => {
    render(<Harness />);
    const input = screen.getByPlaceholderText("https://example.com/landing");

    fireEvent.change(input, { target: { value: "h" } });

    expect(input).toHaveValue("h?click_id={click_id}");
  });

  it("shows the 80-character recommendation for Native and IPP visible text", () => {
    render(<Harness formatKey="native" />);

    const title = screen.getByPlaceholderText("Ad headline");
    fireEvent.change(title, { target: { value: "a".repeat(81) } });

    expect(screen.getByText("81/80")).toHaveClass("text-destructive");
    expect(screen.getAllByText("We recommend no more than 80 characters")).toHaveLength(2);
  });

  it("mounts the banner editor without requiring unsupported browser APIs", () => {
    const originalMatchAll = String.prototype.matchAll;
    Object.defineProperty(String.prototype, "matchAll", {
      configurable: true,
      value: undefined,
    });

    try {
      render(<Harness formatKey="banner" />);

      expect(screen.getByText(/Banner size/)).toBeInTheDocument();
      expect(screen.getByText("Creative type")).toBeInTheDocument();
      expect(screen.getByPlaceholderText("https://example.com/landing")).toBeInTheDocument();
    } finally {
      Object.defineProperty(String.prototype, "matchAll", {
        configurable: true,
        value: originalMatchAll,
      });
    }
  });
});
