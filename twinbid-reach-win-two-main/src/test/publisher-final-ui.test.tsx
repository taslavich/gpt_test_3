import { selectPublisherFilter } from "./publisher-filter-ui";
import { formatCountryLabel } from "@/lib/countries";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import PublisherStatistics from "@/pages/PublisherStatistics";
import PublisherOverview from "@/pages/PublisherOverview";
import PublisherSites from "@/pages/PublisherSites";
import { publisherApi, type PublisherMetrics } from "@/api/publisher";
import {
  PublisherTestWrapper, publisherServer, userFixture, languageFixture,
  profileFixture, reportFixture, rowFixture, reply,
} from "./publisher-fixtures";

vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ user: userFixture, signOut: vi.fn() }) }));
vi.mock("@/contexts/LanguageContext", () => ({ useLanguage: () => languageFixture }));
vi.mock("@/contexts/ProfileContext", () => ({ useProfile: () => ({ profile: profileFixture, loading: false }) }));
beforeAll(() => {
  // jsdom lacks this layout API used by the actual Radix Select.
  Element.prototype.scrollIntoView = vi.fn();
});
beforeEach(() => { localStorage.clear(); languageFixture.lang = "ru"; });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); languageFixture.lang = "ru"; });
const hiddenLabels = ["Запросы", "Requests", "请求", "С рекламой", "Fills", "填充", "Без рекламы", "No fills", "无填充", "Ошибки", "Errors", "错误"];
function assertNoHiddenMetrics() {
  for (const name of hiddenLabels) expect(screen.queryByText(name, { exact: true })).not.toBeInTheDocument();
}
async function chooseGroup(name: string) {
  const group = await screen.findByRole("group", {name:"Группировка"});
  fireEvent.click(within(group).getByRole("button", {name}));
}
const statCalls = (s: ReturnType<typeof publisherServer>) => s.calls.filter(c => c.url.pathname.endsWith("/stats/query"));

describe("Publisher final UI: server metrics and format grouping", () => {
  it.each([
    ["ru", "Формат", "Итого за весь период", "Показы"],
    ["en", "Format", "Total for the full period", "Impressions"],
    ["zh-CN", "格式", "整个期间的合计", "展示"],
  ])("shows the CPM column and the localized format option in %s", async (lang, formatLabel, totalLabel, impressionLabel) => {
    languageFixture.lang = lang;
    publisherServer();
    render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    await screen.findByText(totalLabel);
    assertNoHiddenMetrics();
    expect(screen.getAllByRole("button", { name: impressionLabel })).toHaveLength(2);
    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("columnheader")).toHaveLength(8);
    for (const row of within(table).getAllByRole("row").slice(2)) expect(within(row).getAllByRole("cell")).toHaveLength(6);
    expect(screen.getByRole("button", {name:formatLabel})).toBeInTheDocument();
  });

  it("uses real format buckets, server totals, filters, sorting and pagination; switching groups resets offset", async () => {
    const s = publisherServer();
    s.overrides.set("POST /api/publisher/stats/query", req => reply({
      ...reportFixture, limit: 5, offset: Number(req.body?.offset || 0),
      rows: req.body?.group_by === "format"
        ? ["POP", "BAN", "NAT", "IPP", "VID"].map((bucket, index) => ({ ...rowFixture, bucket, impressions: (index + 1) * 10, revenue_usd: bucket === "POP" ? "1.23456789" : "0" }))
        : [{ ...rowFixture, bucket: req.body?.group_by === "hour" ? "2026-10-14T12:00:00Z" : "server-" + req.body?.group_by }],
    }));
    render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    await screen.findByText("Итого за весь период");
    await selectPublisherFilter("Сайт (ID)", [rowFixture.hostname]);
    await selectPublisherFilter("Страна", [formatCountryLabel("US","ru"),formatCountryLabel("GB","ru")]);
    fireEvent.click(screen.getByRole("button",{name:"Обновить"}));
    await waitFor(()=>expect(statCalls(s).at(-1)?.body?.site_id).toBe(rowFixture.site_id));
    await chooseGroup("Формат");
    await screen.findByRole("cell", { name: "POP" });
    expect(statCalls(s).at(-1)?.body).toMatchObject({ group_by: "format", offset: 0, site_id: rowFixture.site_id, countries: ["US", "GB"] });
    for (const bucket of ["POP", "BAN", "NAT", "IPP", "VID"]) {
      const row = screen.getByRole("cell", { name: bucket }).closest("tr")!;
      expect(within(row).getAllByRole("cell")).toHaveLength(6);
      expect(within(row).getAllByRole("cell").at(-2)).toHaveTextContent(bucket === "POP" ? "$1,23456789" : "$0,00");
    }
    expect(screen.getAllByText(/\$98\s765,12345678/).length).toBeGreaterThan(0);
    fireEvent.click(within(screen.getByRole("columnheader", { name: "Показы" })).getByRole("button"));
    expect(within(screen.getByRole("table")).getAllByRole("row")[2]).toHaveTextContent("VID");
    fireEvent.click(screen.getByRole("button", { name: "Далее" }));
    await waitFor(() => expect(statCalls(s).at(-1)?.body).toMatchObject({ group_by: "format", offset: 5, site_id: rowFixture.site_id, countries: ["US", "GB"] }));
    for (const [label, value] of [["Дата", "date"], ["Час", "hour"], ["Сайт", "site"], ["Зона", "zone"], ["Код", "ad_code"], ["Страна", "country"], ["Устройство", "device"], ["ОС", "os"], ["Браузер", "browser"], ["Формат", "format"]]) {
      await waitFor(() => expect(screen.getByRole("button", { name: "Обновить" })).toBeEnabled());
      await chooseGroup(label);
      await waitFor(() => expect(statCalls(s).at(-1)?.body).toMatchObject({ group_by: value, offset: 0, site_id: rowFixture.site_id, countries: ["US", "GB"] }));
      await waitFor(() => expect(screen.getByRole("button", { name: "Обновить" })).toBeEnabled());
    }
    assertNoHiddenMetrics();
  }, 15000);

  it("does not manufacture format rows and keeps the correct empty-table colspan and HTTP error", async () => {
    const s = publisherServer();
    s.overrides.set("POST /api/publisher/stats/query", () => reply({ ...reportFixture, rows: [] }));
    render(<PublisherStatistics />, { wrapper: PublisherTestWrapper });
    await screen.findByText(/Нет данных за выбранный период/);
    await chooseGroup("Формат");
    await waitFor(() => expect(statCalls(s).at(-1)?.body?.group_by).toBe("format"));
    expect((await screen.findByText(/Нет данных за выбранный период/)).closest("td")).toHaveAttribute("colspan", "6");
    for (const bucket of ["POP", "BAN", "NAT", "IPP", "VID"]) expect(screen.queryByRole("cell", { name: bucket })).not.toBeInTheDocument();
    s.overrides.set("POST /api/publisher/stats/query", () => reply(null, 503, "real format API failure"));
    await waitFor(() => expect(screen.getByRole("button", { name: "Обновить" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Обновить" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось загрузить данные");
    expect(screen.queryByText("Итого за весь период")).not.toBeInTheDocument();
  });

  it.each(["ru", "en", "zh-CN"])("hides overview metrics and aligns its five columns in %s", async lang => {
    languageFixture.lang = lang;
    publisherServer();
    render(<PublisherOverview />, { wrapper: PublisherTestWrapper });
    await screen.findByText(rowFixture.hostname);
    assertNoHiddenMetrics();
    const rows = within(screen.getByRole("table")).getAllByRole("row");
    expect(within(rows[0]).getAllByRole("columnheader")).toHaveLength(5);
    expect(within(rows[1]).getAllByRole("cell")).toHaveLength(5);
    expect(within(rows[1]).getAllByRole("cell")[1]).toHaveTextContent("40");
    expect(within(rows[1]).getAllByRole("cell")[2]).toHaveTextContent("1");
  });

  it.each(["ru", "en", "zh-CN"])("hides site request counts and aligns zone columns, keeping last_request_at in %s", async lang => {
    languageFixture.lang = lang;
    publisherServer();
    render(<PublisherSites />, { wrapper: PublisherTestWrapper });
    const site = await screen.findByRole("button", { name: /news.example.test/ });
    expect(site).not.toHaveTextContent("98");
    fireEvent.click(site);
    await screen.findByText(rowFixture.zone_id);
    assertNoHiddenMetrics();
    const rows = within(screen.getByRole("table")).getAllByRole("row");
    expect(within(rows[0]).getAllByRole("columnheader")).toHaveLength(9);
    const cells = within(rows[1]).getAllByRole("cell");
    expect(cells).toHaveLength(9);
    expect(cells[3]).toHaveTextContent("Popunder");
    expect(cells[4]).toHaveTextContent("2026");
    expect(cells[5]).toHaveTextContent("40");
    expect(cells[6]).toHaveTextContent("1");
    expect(cells[7]).toHaveTextContent("2");
    expect(screen.getByRole("table").parentElement).toHaveClass("overflow-x-auto");
  });

  it("keeps the four hidden metrics in typed API responses", async () => {
    publisherServer();
    const response = await publisherApi.stats({ from: "2026-10-01T00:00:00Z", to: "2026-10-02T00:00:00Z", group_by: "format" });
    const retained: Pick<PublisherMetrics, "requests" | "fills" | "no_fills" | "errors"> = response.totals;
    expect(retained).toMatchObject({ requests: 9876, fills: 5000, no_fills: 4786, errors: 90 });
  });
});
