import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import TwinBidMoney from "@/pages/TwinBidMoney";
import { LanguageProvider } from "@/contexts/LanguageContext";

vi.mock("@/components/money/MoneyRegisterDialog", () => ({
  MoneyRegisterDialog: ({ open }: { open: boolean }) => open ? <div role="dialog">Registration</div> : null,
}));

describe("TwinBid money pre-landing", () => {
  const renderPage = () => render(<LanguageProvider><TwinBidMoney /></LanguageProvider>);

  it("renders the direct-response funnel and opens registration", () => {
    renderPage();

    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Turn Internet Traffic Into Profit");
    expect(screen.getAllByText("+$270").length).toBeGreaterThan(0);
    expect(screen.queryByText(/guaranteed/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /start now/i }));
    expect(screen.getByRole("dialog")).toHaveTextContent("Registration");
  });

  it("recalculates the campaign model when a control changes", () => {
    renderPage();
    const sliders = screen.getAllByRole("slider");

    expect(screen.getByText("+$200")).toBeInTheDocument();
    fireEvent.change(sliders[0], { target: { value: "200" } });
    expect(screen.getByText("+$400")).toBeInTheDocument();
  });

  it("switches the pre-landing between Russian and Spanish", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "RU" }));
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Превратите интернет-трафик в прибыль");

    fireEvent.click(screen.getByRole("button", { name: "ES" }));
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Convierte el tráfico de Internet en ganancias");
  });
});
