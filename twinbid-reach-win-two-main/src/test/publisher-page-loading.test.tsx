import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Link, MemoryRouter, Outlet, Route, Routes } from "react-router-dom";
import { PublisherPageBoundary } from "@/components/publisher/PublisherPageBoundary";
vi.mock("@/contexts/LanguageContext", () => ({ useLanguage: () => ({ lang: "ru" }) }));
afterEach(() => { cleanup(); vi.restoreAllMocks(); });
function Shell() {
  return <><nav><Link to="/settings">Настройки</Link><Link to="/sites">Сайты</Link></nav>
    <PublisherPageBoundary><Outlet /></PublisherPageBoundary></>;
}
function mount(element: React.ReactNode) {
  render(<MemoryRouter initialEntries={["/settings"]}><Routes>
    <Route element={<Shell />}>
      <Route path="settings" element={element} />
      <Route path="sites" element={<h1>Список сайтов</h1>} />
    </Route>
  </Routes></MemoryRouter>);
}
it("keeps navigation working when a route module has not loaded", () => {
  const waiting = new Promise(() => {});
  function SlowPage(): never { throw waiting; }
  mount(<SlowPage />);
  expect(screen.getByRole("status")).toHaveTextContent("Загрузка раздела");
  fireEvent.click(screen.getByRole("link", { name: "Сайты" }));
  expect(screen.getByRole("heading", { name: "Список сайтов" })).toBeInTheDocument();
});
it("contains route render/chunk errors and recovers on navigation", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  function FailedPage(): never { throw new Error("Failed to fetch dynamically imported module"); }
  mount(<FailedPage />);
  expect(screen.getByRole("alert")).toHaveTextContent("Не удалось открыть раздел");
  expect(screen.getByRole("button", { name: "Обновить страницу" })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("link", { name: "Сайты" }));
  expect(screen.getByRole("heading", { name: "Список сайтов" })).toBeInTheDocument();
});
