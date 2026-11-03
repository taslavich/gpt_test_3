import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CreateZoneDialog } from "@/components/publisher/CreateZoneDialog";
import { EditZoneDialog } from "@/components/publisher/EditZoneDialog";
import {
  codeFixture,
  languageFixture,
  userFixture,
  PublisherTestWrapper,
  publisherServer,
  reply,
} from "./publisher-fixtures";
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ user: userFixture, signOut: vi.fn() }),
}));
vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => languageFixture,
}));
beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
describe("Real Popunder forms", () => {
  it("requires an explicit traffic choice and exposes no unsupported formats", async () => {
    const s = publisherServer();
    render(<CreateZoneDialog open onOpenChange={vi.fn()} />, {
      wrapper: PublisherTestWrapper,
    });
    expect(screen.getByRole("button", { name: "Adult" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    expect(screen.getByRole("button", { name: "Mainstream" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    expect(
      screen.queryByRole("button", { name: /Mixed|Banner|Native|VAST|IPP/i }),
    ).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Название кода *"), {
      target: { value: "Real Code" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Создать и получить код" }),
    );
    await waitFor(() =>
      expect(s.calls.filter((c) => c.method === "POST")).toHaveLength(0),
    );
  });
  it("explains the live click, interval, page-session cap and device settings", () => {
    publisherServer();
    render(<CreateZoneDialog open onOpenChange={vi.fn()} />, { wrapper: PublisherTestWrapper });
    const summary = () => screen.getByRole("heading", { name: "Как будет работать рекламный код" }).parentElement!;
    expect(summary()).toHaveTextContent("Первый показ после первого клика");
    expect(summary()).toHaveTextContent("не чаще одного раза в 0,1 мин.");
    expect(summary()).toHaveTextContent("без ограничения числа показов за сессию страницы");
    expect(summary()).toHaveTextContent("на всех устройствах");
    fireEvent.click(screen.getByRole("button", { name: "После второго клика" }));
    fireEvent.change(screen.getByLabelText("Интервал, минуты"), { target: { value: "2.5" } });
    fireEvent.click(screen.getByRole("switch", { name: "Без ограничения" }));
    fireEvent.change(screen.getByLabelText("Максимум за сессию страницы"), { target: { value: "10" } });
    fireEvent.click(screen.getByRole("button", { name: "Мобильные" }));
    expect(summary()).toHaveTextContent("Первый показ после второго клика, не чаще одного раза в 2,5 мин., не более 10 показов за сессию страницы, на мобильных устройствах.");
    fireEvent.change(screen.getByLabelText("Интервал, минуты"), { target: { value: "" } });
    expect(summary()).toHaveTextContent("укажите интервал между показами");
    expect(summary()).not.toHaveTextContent("NaN");
  });
  it("creates a code with a 0.1 minute interval by default", async () => {
    const s = publisherServer();
    render(<CreateZoneDialog open onOpenChange={vi.fn()} />, { wrapper: PublisherTestWrapper });
    expect(screen.getByLabelText("Интервал, минуты")).toHaveValue(0.1);
    expect(screen.getByRole("region", { name: "Статистика мест размещения" })).toHaveTextContent("Для раздельной статистики и управления создайте или продублируйте код.");
    fireEvent.change(screen.getByLabelText("Название кода *"), { target: { value: "Default interval" } });
    fireEvent.click(screen.getByRole("button", { name: "Adult" }));
    fireEvent.click(screen.getByRole("button", { name: "Создать и получить код" }));
    await waitFor(() => expect(s.calls.find(req=>req.method==="POST")?.body).toMatchObject({frequency_minutes:0.1,trigger_click:1,max_per_session:null}));
  });
  it("keeps a saved interval when editing and shows the placement warning", async () => {
    const s = publisherServer();
    s.overrides.set("GET /api/publisher/ad-codes/" + codeFixture.ad_code_id, () => reply({...codeFixture,frequency_minutes:10}));
    render(<EditZoneDialog code={codeFixture} onOpenChange={vi.fn()} />, { wrapper: PublisherTestWrapper });
    await screen.findByLabelText("Интервал, минуты");
    await waitFor(()=>expect(screen.getByLabelText("Интервал, минуты")).toHaveValue(10));
    expect(screen.getByRole("region", {name:"Статистика мест размещения"})).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name:"Сохранить"}));
    await waitFor(()=>expect(s.calls.find(req=>req.method==="PATCH")?.body?.frequency_minutes).toBe(10));
  });
  it.each(["adult", "mainstream"] as const)(
    "creates %s through POST and keeps only server identifiers",
    async (traffic) => {
      const s = publisherServer(),
        created = vi.fn();
      render(
        <CreateZoneDialog open onOpenChange={vi.fn()} onCreated={created} />,
        { wrapper: PublisherTestWrapper },
      );
      fireEvent.change(screen.getByLabelText("Название кода *"), {
        target: { value: "Real Code" },
      });
      fireEvent.click(
        screen.getByRole("button", {
          name: traffic === "adult" ? "Adult" : "Mainstream",
        }),
      );
      fireEvent.change(screen.getByLabelText("Интервал, минуты"), {
        target: { value: "0.1" },
      });
      fireEvent.click(
        screen.getByRole("button", { name: "После второго клика" }),
      );
      fireEvent.click(
        screen.getByRole("button", { name: "Создать и получить код" }),
      );
      await waitFor(() => expect(created).toHaveBeenCalled());
      expect(s.calls.find((c) => c.method === "POST")?.body).toEqual({
        name: "Real Code",
        format: "popunder",
        traffic_type: traffic,
        device: "all",
        trigger_click: 2,
        frequency_minutes: 0.1,
        max_per_session: null,
      });
      expect(created.mock.calls[0][0]).toMatchObject({
        short_code: "tb_created_by_server",
        ad_code_id: "44444444-4444-4444-8444-444444444444",
      });
    },
  );
  it("omits unchanged traffic from PATCH, includes version and refreshes after 409", async () => {
    const s = publisherServer();
    let version = 7;
    s.overrides.set(
      "GET /api/publisher/ad-codes/" + codeFixture.ad_code_id,
      () => reply({ ...codeFixture, version }),
    );
    s.overrides.set(
      "PATCH /api/publisher/ad-codes/" + codeFixture.ad_code_id,
      (req) => {
        if (version === 7) {
          version = 8;
          return reply(null, 409, "version conflict");
        }
        return reply({ ...codeFixture, ...req.body, version: 9 });
      },
    );
    const close = vi.fn();
    render(<EditZoneDialog code={codeFixture} onOpenChange={close} />, {
      wrapper: PublisherTestWrapper,
    });
    const input = await screen.findByLabelText("Название кода");
    fireEvent.change(input, { target: { value: "Edited" } });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText(/Данные изменились или операция конфликтует/);
    const first = s.calls.find((c) => c.method === "PATCH")!.body!;
    expect(first.version).toBe(7);
    expect(first).not.toHaveProperty("traffic_type");
    expect(close).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Обновить данные" }));
    await waitFor(() => expect(screen.getByLabelText("Название кода")).toHaveValue(codeFixture.name));
    fireEvent.change(screen.getByLabelText("Название кода"), {
      target: { value: "Edited again" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    await waitFor(() => expect(close).toHaveBeenCalledWith(false));
    expect(s.calls.filter((c) => c.method === "PATCH")[1].body?.version).toBe(
      8,
    );
  });
});
