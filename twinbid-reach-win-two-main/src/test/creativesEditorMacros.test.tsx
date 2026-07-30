// @vitest-environment jsdom
import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CreativesEditor } from "@/components/dashboard/CreativesEditor";
import { LanguageProvider } from "@/contexts/LanguageContext";
import type { Creative } from "@/contexts/CampaignContext";

function Harness() {
  const [creatives, setCreatives] = useState<Creative[]>([{
    id: "creative-1",
    name: "Creative",
    url: "",
  }]);

  return (
    <LanguageProvider>
      <CreativesEditor
        formatKey="popunder"
        creatives={creatives}
        onChange={setCreatives}
      />
    </LanguageProvider>
  );
}

describe("creative URL macros", () => {
  it("shows click_id immediately after the first URL character is entered", () => {
    render(<Harness />);
    const input = screen.getByPlaceholderText("https://example.com/landing");

    fireEvent.change(input, { target: { value: "h" } });

    expect(input).toHaveValue("h?click_id={click_id}");
  });
});
