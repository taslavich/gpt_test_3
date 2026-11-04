import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { PublisherDirectLinkCode } from "@/api/publisher";
import { CreateZoneDialog } from "@/components/publisher/CreateZoneDialog";
import { EditZoneDialog } from "@/components/publisher/EditZoneDialog";
import { ZoneCodeDialog } from "@/components/publisher/ZoneCodeDialog";
import PublisherZones from "@/pages/PublisherZones";
import { codeFixture, languageFixture, PublisherTestWrapper, publisherServer, reply, userFixture } from "./publisher-fixtures";

vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ user: userFixture, signOut: vi.fn() }) }));
vi.mock("@/contexts/LanguageContext", () => ({ useLanguage: () => languageFixture }));

const link = "https://redirect.example.test/r/tb_server_link?utm_source=test&key=A%2Fb";
const direct: PublisherDirectLinkCode = {
  ad_code_id: "direct-code-id",
  user_id: codeFixture.user_id,
  short_code: "tb_server_link",
  name: "Placement A",
  format: "direct_link",
  traffic_type: "mainstream",
  device: "mobile",
  is_active: true,
  archived: false,
  version: 1,
  synced_version: 1,
  created_at: codeFixture.created_at,
  direct_link_url: link,
};
const writeText = vi.fn();
const clipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
beforeEach(() => {
  localStorage.clear();
  writeText.mockReset().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  if (clipboard) Object.defineProperty(navigator, "clipboard", clipboard);
  else Reflect.deleteProperty(navigator, "clipboard");
});

it("keeps Direct Link unavailable on an API that only supports Popunder", async () => {
  const server = publisherServer();
  render(<CreateZoneDialog open onOpenChange={vi.fn()} />, { wrapper: PublisherTestWrapper });
  fireEvent.click(screen.getByRole("button", { name: "Direct Link" }));
  const notice = screen.getByRole("alert");
  expect(notice).toHaveTextContent('<a href="https://carloverrcvbg.site/go/tb_123" referrerpolicy="origin">');
  expect(notice).toHaveTextContent('Не добавляйте rel="noreferrer"');
  expect(notice).toHaveTextContent("аукцион не запустится");
  expect(screen.getByRole("status")).toHaveTextContent("Создание Direct Link пока недоступно");
  expect(screen.getByRole("button", { name: "Создать и получить ссылку" })).toBeDisabled();
  expect(server.calls.some((request) => request.method === "POST")).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Popunder" }));
  expect(screen.getByLabelText("Интервал, минуты")).toHaveValue(0.1);
});

it("creates a link without Popunder controls and copies the exact server URL", async () => {
  const server = publisherServer();
  let created = false;
  server.overrides.set("GET /api/publisher/ad-codes", () => reply({
    items: created ? [codeFixture, direct] : [codeFixture],
    limit: 50, offset: 0,
    public_script_url: "https://scripts.example.test/sdk.js",
    supported_formats: ["popunder", "direct_link"],
  }));
  server.overrides.set("POST /api/publisher/ad-codes", () => {
    created = true;
    return reply(direct);
  });
  render(<PublisherZones />, { wrapper: PublisherTestWrapper });
  await screen.findByText(codeFixture.name);
  fireEvent.click(screen.getByRole("button", { name: "Создать код" }));
  fireEvent.click(screen.getByRole("button", { name: "Direct Link" }));
  expect(screen.queryByLabelText("Интервал, минуты")).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Название кода *"), { target: { value: direct.name } });
  fireEvent.click(screen.getByRole("button", { name: "Mainstream" }));
  fireEvent.click(screen.getByRole("button", { name: "Мобильные" }));
  fireEvent.click(screen.getByRole("button", { name: "Создать и получить ссылку" }));
  expect(await screen.findByText(link)).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Копировать код" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Копировать ссылку" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith(link));
  fireEvent.click(screen.getByRole("button", { name: "Копировать HTML" }));
  await waitFor(() => expect(writeText).toHaveBeenLastCalledWith(
    '<a href="https://redirect.example.test/r/tb_server_link?utm_source=test&amp;key=A%2Fb" referrerpolicy="origin">\n  Продолжить\n</a>',
  ));

  expect(server.calls.find((req) => req.method === "POST")?.body).toEqual({
    name: direct.name, format: "direct_link", traffic_type: "mainstream", device: "mobile",
  });
});

it("edits a Direct Link using its version and without sending Popunder fields", async () => {
  const server = publisherServer();
  const path = "/api/publisher/ad-codes/" + direct.ad_code_id;
  server.overrides.set("GET " + path, () => reply(direct));
  server.overrides.set("PATCH " + path, (req) => reply({ ...direct, ...req.body, version: 2 }));
  const close = vi.fn();
  render(<EditZoneDialog code={direct} onOpenChange={close} />, { wrapper: PublisherTestWrapper });
  fireEvent.change(await screen.findByLabelText("Название кода"), { target: { value: "Renamed" } });
  fireEvent.click(screen.getByRole("button", { name: "Компьютеры" }));
  fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
  await waitFor(() => expect(close).toHaveBeenCalledWith(false));
  expect(server.calls.find((req) => req.method === "PATCH")?.body).toEqual({
    name: "Renamed", device: "desktop", version: 1,
  });
});

it.each([null, "javascript:alert(1)", "https://user:secret@example.test/r", "/r/tb_server_link", "http://example.test/r"])(
  "does not copy a missing or invalid server URL: %s",
  async (direct_link_url) => {
    publisherServer();
    render(<ZoneCodeDialog code={{ ...direct, direct_link_url }} onOpenChange={vi.fn()} />, { wrapper: PublisherTestWrapper });
    expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось получить ссылку");
    expect(screen.queryByRole("button", { name: "Копировать ссылку" })).not.toBeInTheDocument();
    expect(writeText).not.toHaveBeenCalled();
  },
);
