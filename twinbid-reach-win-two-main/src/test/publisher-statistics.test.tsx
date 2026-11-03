import { selectPublisherFilter } from "./publisher-filter-ui";
import { formatCountryLabel } from "@/lib/countries";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useProfile } from "@/contexts/ProfileContext";
import PublisherStatistics from "@/pages/PublisherStatistics";
import {
  PublisherTestWrapper,
  publisherServer,
  userFixture,
  languageFixture,
  profileFixture,
  reportFixture,
  rowFixture,
  codeFixture,
  reply,
} from "./publisher-fixtures";
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ user: userFixture, signOut: vi.fn() }),
}));
vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => languageFixture,
}));
vi.mock("@/contexts/ProfileContext", () => ({
  useProfile: vi.fn(),
}));
beforeEach(() => {
  localStorage.clear();
  vi.mocked(useProfile).mockReturnValue({ profile: profileFixture, loading: false, updateProfile: vi.fn(), refetch: vi.fn() });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
describe("Server reports, filters and hour boundaries", () => {
  it("renders full server totals/CTR and effective boundaries, not sums calculated from the current page", async () => {
    const s = publisherServer();
    render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    await screen.findByText("Итого за весь период");
    expect(screen.getAllByText(/\$98\s765,12345678/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("0,8%").length).toBeGreaterThan(0);
    expect(screen.getByText(/Период:.*07\.10\.2026.*14\.10\.2026/)).toBeInTheDocument();
    expect(
      s.calls.find((c) => c.url.pathname.endsWith("/stats/query"))?.body,
    ).toMatchObject({
      group_by: "date",
      timezone: "Etc/GMT-3",
      limit: 50,
      offset: 0,
    });
  });
  it("sends filters and exclusive UTC dates using the profile timezone", async () => {
    vi.mocked(useProfile).mockReturnValue({ profile: { ...profileFixture, timezone: "Asia/Kolkata" }, loading: false, updateProfile: vi.fn(), refetch: vi.fn() });
    const s = publisherServer();
    render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    await screen.findByText("Итого за весь период");
    await selectPublisherFilter("Сайт (ID)", [rowFixture.hostname]);
    await selectPublisherFilter("Рекламный код (ID)", [codeFixture.name]);
    await selectPublisherFilter("Зона (ID)", [`${rowFixture.hostname} · ${rowFixture.zone_id}`]);
    await selectPublisherFilter("Страна", [formatCountryLabel("US","ru"),formatCountryLabel("GB","ru")]);
    await selectPublisherFilter("Устройство", ["mobile"]);
    await selectPublisherFilter("ОС", ["Android"]);
    await selectPublisherFilter("Браузер", ["Chrome"]);
    expect(screen.queryByLabelText("Часовой пояс")).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Обновить" }),
    );
    await waitFor(() =>
      expect(
        s.calls.filter((c) => c.url.pathname.endsWith("/stats/query")).at(-1)
          ?.body,
      ).toMatchObject({
        timezone: "Asia/Kolkata",
        site_id: rowFixture.site_id,
        zone_id: rowFixture.zone_id,
        ad_code_id: codeFixture.ad_code_id,
        countries: ["US", "GB"],
        devices: ["mobile"],
        oses: ["Android"],
        browsers: ["Chrome"],
      }),
    );
    const body = s.calls
      .filter((c) => c.url.pathname.endsWith("/stats/query"))
      .at(-1)!.body!;
    expect(String(body.from)).toMatch(/18:30:00.000Z$/);
    expect(String(body.to)).toMatch(/18:30:00.000Z$/);
  });
  it("defaults to UTC+0 without a saved profile timezone and waits for the profile", async () => {
    const s = publisherServer();
    const profile = { ...profileFixture, timezone: null };
    vi.mocked(useProfile).mockReturnValue({ profile: null, loading: true, updateProfile: vi.fn(), refetch: vi.fn() });
    const view = render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    expect(s.calls.filter(c => c.url.pathname.endsWith("/stats/query"))).toHaveLength(0);
    vi.mocked(useProfile).mockReturnValue({ profile, loading: false, updateProfile: vi.fn(), refetch: vi.fn() });
    view.rerender(<PublisherStatistics />);
    await screen.findByText("Итого за весь период");
    const body = s.calls.find(c => c.url.pathname.endsWith("/stats/query"))!.body!;
    expect(body.timezone).toBe("UTC");
    expect(String(body.from)).toMatch(/00:00:00.000Z$/);
    expect(String(body.to)).toMatch(/00:00:00.000Z$/);
    expect(screen.queryByLabelText("Часовой пояс")).not.toBeInTheDocument();
  });
  it("calculates Today in the profile timezone at a UTC day boundary", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-27T23:30:00Z"));
    const s = publisherServer();
    render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    await screen.findByText("Итого за весь период");
    fireEvent.click(screen.getByRole("button", { name: "Сегодня" }));
    fireEvent.click(screen.getByRole("button", { name: "Обновить" }));
    await waitFor(() => expect(s.calls.filter(c => c.url.pathname.endsWith("/stats/query")).at(-1)?.body).toMatchObject({
      timezone: "Etc/GMT-3", from: "2026-10-27T21:00:00.000Z", to: "2026-10-28T21:00:00.000Z",
    }));
  });
  it("re-queries applied days and filters after the profile timezone changes", async () => {
    const s = publisherServer();
    const view = render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    await screen.findByText("Итого за весь период");
    await selectPublisherFilter("Устройство", ["mobile"]);
    fireEvent.click(screen.getByRole("button", { name: "Обновить" }));
    await waitFor(() => expect(s.calls.filter(c => c.url.pathname.endsWith("/stats/query")).at(-1)?.body?.devices).toEqual(["mobile"]));
    const previous = s.calls.filter(c => c.url.pathname.endsWith("/stats/query")).at(-1)!.body!;
    vi.mocked(useProfile).mockReturnValue({ profile: { ...profileFixture, timezone: "utc_0" }, loading: false, updateProfile: vi.fn(), refetch: vi.fn() });
    view.rerender(<PublisherStatistics />);
    await waitFor(() => expect(s.calls.filter(c => c.url.pathname.endsWith("/stats/query")).at(-1)?.body?.timezone).toBe("UTC"));
    const body = s.calls.filter(c => c.url.pathname.endsWith("/stats/query")).at(-1)!.body!;
    expect(body.devices).toEqual(["mobile"]);
    expect(new Date(String(body.from)).getTime() - new Date(String(previous.from)).getTime()).toBe(3 * 60 * 60 * 1000);
    expect(new Date(String(body.to)).getTime() - new Date(String(previous.to)).getTime()).toBe(3 * 60 * 60 * 1000);
  });
  it("shows successful empty rows and keeps server totals", async () => {
    const s = publisherServer();
    s.overrides.set("POST /api/publisher/stats/query", () =>
      reply({ ...reportFixture, rows: [] }),
    );
    render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    await screen.findByText(/Нет данных за выбранный период/);
    expect(screen.getAllByText(/\$98\s765,12345678/).length).toBeGreaterThan(0);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
  it.each([401, 503])(
    "surfaces stats HTTP %i instead of false no-data/zero revenue",
    async (status) => {
      const s = publisherServer();
      s.overrides.set("POST /api/publisher/stats/query", () =>
        reply(null, status, "statistics unavailable"),
      );
      render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
      await screen.findByRole("alert");
      expect(
        screen.queryByText("Итого за весь период"),
      ).not.toBeInTheDocument();
      if (status === 401)
        expect(
          screen.getByRole("link", { name: "Войти снова" }),
        ).toHaveAttribute("href", "/publisher");
    },
  );
});
