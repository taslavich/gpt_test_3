import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import App from "@/App";

describe("public TwinBid landings", () => {
  beforeEach(() => {
    class Observer {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
    globalThis.IntersectionObserver = Observer as unknown as typeof IntersectionObserver;
    globalThis.ResizeObserver = Observer as unknown as typeof ResizeObserver;
    window.scrollTo = () => {};
    localStorage.clear();
    localStorage.setItem("twinbid_lang", "ru");
  });

  it("opens /publisher directly and returns to advertisers with the hero button", async () => {
    window.history.replaceState(null, "", "/publisher");
    const { container } = render(<App />);
    const publisherHero = await screen.findByText("Монетизируйте");
    expect(publisherHero).toBeInTheDocument();
    expect(screen.queryByText("Для паблишеров")).not.toBeInTheDocument();
    expect(container.querySelector(".landing-network-stage")).toBeNull();
    expect(container.querySelector("#publisher-benefits")).toBeTruthy();
    expect(container.querySelector("#publisher-formats")).toBeTruthy();
    expect(container.querySelector("#publisher-formats .format-editorial-scene")).toBeTruthy();
    expect(container.querySelector("#publisher-steps")).toBeTruthy();
    expect(container.querySelector("#publisher-top")?.textContent).not.toContain("100K");
    expect(container.querySelector("#publisher-benefits")?.textContent).toContain("Быстрый запуск");
    expect(container.querySelector("#publisher-benefits")?.textContent).toContain("Без многоэтапных проверок");
    expect(container.querySelector("#publisher-steps")?.textContent).toContain("3 минуты");
    expect(container.querySelector("#publisher-steps")?.textContent).toContain("Зарегистрируйтесь");
    expect(container.querySelector("#publisher-steps")?.textContent).toContain("Возьмите рекламный код");
    expect(container.querySelector("#publisher-steps")?.textContent).toContain("Разместите на сайте");
    const hero = container.querySelector("#publisher-top");
    expect(hero).toBeTruthy();
    fireEvent.click((hero as HTMLElement).querySelector(".publisher-back-link") as HTMLButtonElement);
    await waitFor(() => expect(window.location.pathname).toBe("/"), { timeout: 3000 });
    expect(screen.getByRole("heading", { name: /TwinBid объединяет рекламный трафик/ })).toBeInTheDocument();
  });

  it("opens the publisher landing from the advertiser hero", async () => {
    window.history.replaceState(null, "", "/");
    render(<App />);
    const publisherButton = screen.getByRole("button", { name: "Монетизировать трафик" });
    expect(publisherButton).toHaveClass("landing-publisher-switch");
    fireEvent.click(publisherButton);
    await waitFor(() => expect(window.location.pathname).toBe("/publisher"), { timeout: 3000 });
    expect(await screen.findByText("Монетизируйте")).toBeInTheDocument();
  });
});
