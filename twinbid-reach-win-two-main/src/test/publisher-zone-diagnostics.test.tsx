import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import {
  PublisherDiagnostics,
  PublisherZoneActivityCards,
} from "@/components/publisher/PublisherDiagnostics";
import {
  PublisherTestWrapper,
  publisherServer,
  userFixture,
  languageFixture,
  profileFixture,
  zoneReportFixture,
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
  useProfile: () => ({ profile: profileFixture, loading: false }),
}));
beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  languageFixture.lang = "en";
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  languageFixture.lang = "ru";
});

describe("Publisher zone diagnostics", () => {
  it("keeps the service warning and request breakdown visible on a narrow screen", async () => {
    const originalWidth = window.innerWidth;
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 375 });
    try {
      const s = publisherServer();
      s.overrides.set(`GET /api/publisher/ad-codes/${codeFixture.ad_code_id}/diagnostics`, () => reply({ ad_code: codeFixture, runtime_ready: false, zones: zoneReportFixture }));
      render(<PublisherDiagnostics codeId={codeFixture.ad_code_id} />, { wrapper: PublisherTestWrapper });
      expect(await screen.findByRole("alert")).toHaveTextContent("temporarily unavailable");
      expect(screen.getByText("Ad found")).toBeInTheDocument();
      expect(screen.getByText("No ad")).toBeInTheDocument();
      expect(screen.getByText("Request errors")).toBeInTheDocument();
      expect(screen.getByText("Service errors")).toBeInTheDocument();
      expect(screen.queryByRole("table")).not.toBeInTheDocument();
    } finally {
      Object.defineProperty(window, "innerWidth", { configurable: true, value: originalWidth });
    }
  });
  it.each(["ru", "en", "zh-CN"])(
    "shows HTTP counters and aligns eight columns in %s",
    async (lang) => {
      languageFixture.lang = lang;
      const s = publisherServer();
      const rows = [
        {
          ...zoneReportFixture.rows[0],
          zone_id: "only-204",
          requests_200: 0,
          requests_204: 19,
        },
        {
          ...zoneReportFixture.rows[0],
          zone_id: "mixed",
          requests_200: 7,
          requests_5xx: 10,
        },
        {
          ...zoneReportFixture.rows[0],
          zone_id: "silent",
          status: "problem",
          requests_200: 0,
          requests_204: 0,
          requests_4xx: 0,
          requests_5xx: 0,
          last_request_at: null,
        },
      ];
      s.overrides.set(
        `GET /api/publisher/ad-codes/${codeFixture.ad_code_id}/diagnostics`,
        () =>
          reply({
            ad_code: codeFixture,
            runtime_ready: true,
            zones: { ...zoneReportFixture, rows },
          }),
      );
      render(<PublisherDiagnostics codeId={codeFixture.ad_code_id} />, {
        wrapper: PublisherTestWrapper,
      });
      const table = await screen.findByRole("table");
      const trs = within(table).getAllByRole("row");
      expect(within(trs[0]).getAllByRole("columnheader")).toHaveLength(8);
      for (const row of trs.slice(1))
        expect(within(row).getAllByRole("cell")).toHaveLength(8);
      expect(within(trs[1]).getAllByRole("cell")[4]).toHaveTextContent("19");
      expect(within(trs[2]).getAllByRole("cell")[6]).toHaveTextContent("10");
      expect(within(trs[3]).getAllByRole("cell")[7]).toHaveTextContent("—");
      expect(table.parentElement).toHaveClass("overflow-x-auto");
    },
  );
  it("uses server global counts and does not fabricate cards after an API error", async () => {
    const s = publisherServer();
    render(<PublisherZoneActivityCards />, { wrapper: PublisherTestWrapper });
    expect(await screen.findByText("701")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(
      s.calls
        .find((r) => r.url.pathname.endsWith("/zone-diagnostics"))
        ?.url.searchParams.get("limit"),
    ).toBe("1");
    cleanup();
    s.overrides.set("GET /api/publisher/zone-diagnostics", () =>
      reply(null, 503, "refresh is stale"),
    );
    render(<PublisherZoneActivityCards />, { wrapper: PublisherTestWrapper });
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not load data",
    );
    expect(screen.queryByText("701")).not.toBeInTheDocument();
  });
  it("pages the existing code endpoint and displays a genuine empty result", async () => {
    const s = publisherServer();
    s.overrides.set(
      `GET /api/publisher/ad-codes/${codeFixture.ad_code_id}/diagnostics`,
      (r) =>
        reply({
          ad_code: codeFixture,
          runtime_ready: true,
          zones: {
            ...zoneReportFixture,
            offset: Number(r.url.searchParams.get("offset")),
            rows:
              r.url.searchParams.get("offset") === "50"
                ? []
                : Array.from({ length: 50 }, (_, i) => ({
                    ...zoneReportFixture.rows[0],
                    zone_id: `z-${i}`,
                  })),
          },
        }),
    );
    render(<PublisherDiagnostics codeId={codeFixture.ad_code_id} />, {
      wrapper: PublisherTestWrapper,
    });
    await screen.findByText("z-49");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("There are no detected zones on this page.");
    expect(s.calls.at(-1)?.url.searchParams.get("offset")).toBe("50");
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});

it.each(["ru","en","zh-CN"])("explains code-level absence without inventing a zone in %s",async lang=>{
 languageFixture.lang=lang;const s=publisherServer();
 s.overrides.set(`GET /api/publisher/ad-codes/${codeFixture.ad_code_id}/diagnostics`,()=>reply({ad_code:codeFixture,runtime_ready:true,zones:{...zoneReportFixture,rows:[],active_zones:0,problem_zones:0,codes_without_zones:1,undiscovered_codes:[{ad_code_id:codeFixture.ad_code_id,status:"no_zones_detected"}]}}));
 render(<PublisherDiagnostics codeId={codeFixture.ad_code_id}/>,{wrapper:PublisherTestWrapper});
 const notice=await screen.findByText(lang==="ru"?/Зоны пока не обнаружены/:lang==="en"?/No zones detected yet/:/尚未发现广告位/);
 expect(notice).toHaveAttribute("role","status");expect(screen.queryByRole("table")).not.toBeInTheDocument();
 expect(notice).toHaveTextContent(lang==="ru"?"нет посещений":lang==="en"?"not have visitors":"暂无访问量");
});
it("keeps undiscovered codes separate from problem zones on overview",async()=>{
 const s=publisherServer();s.overrides.set("GET /api/publisher/zone-diagnostics",()=>reply({...zoneReportFixture,active_zones:701,problem_zones:12,codes_without_zones:37,undiscovered_codes:[]}));
 render(<PublisherZoneActivityCards/>,{wrapper:PublisherTestWrapper});
 expect(await screen.findByText("37")).toBeInTheDocument();expect(screen.getByText("12")).toBeInTheDocument();
 expect(screen.getByText("Codes without detected zones")).toBeInTheDocument();
});
