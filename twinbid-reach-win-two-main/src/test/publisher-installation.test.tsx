import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ZoneCodeDialog } from "@/components/publisher/ZoneCodeDialog";
import { PublisherTestWrapper, publisherServer, codeFixture, userFixture, languageFixture, reply } from "./publisher-fixtures";

vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ user: userFixture, signOut: vi.fn() }) }));
vi.mock("@/contexts/LanguageContext", () => ({ useLanguage: () => languageFixture }));
const writeText = vi.fn();
const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");

beforeEach(() => {
  localStorage.clear();
  writeText.mockReset().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  if (originalClipboard) Object.defineProperty(navigator, "clipboard", originalClipboard);
  else Reflect.deleteProperty(navigator, "clipboard");
});

function serverWithUrl(url: string) {
  const server = publisherServer();
  server.overrides.set("GET /api/publisher/ad-codes", () => reply({
    items: [codeFixture], limit: 50, offset: 0, public_script_url: url,
  }));
  return server;
}

function mount() {
  return render(<ZoneCodeDialog code={codeFixture} onOpenChange={vi.fn()} />, { wrapper: PublisherTestWrapper });
}

describe("Publisher installation display and clipboard", () => {
  it.each([
    ["https://carloverrcvbg.site/static/pageHelper.js", '<script async src="https://carloverrcvbg.site/static/pageHelper.js" food="tb_server_only"></script>'],
    ["https://stable.example.test/sdk.js", '<script async src="https://stable.example.test/sdk.js" food="tb_server_only"></script>'],
    ['https://stable.example.test/sdk.js?a=1&b="<>', '<script async src="https://stable.example.test/sdk.js?a=1&amp;b=&quot;&lt;&gt;" food="tb_server_only"></script>'],
  ])("displays and copies the same exact server-backed tag for %s", async (url, expected) => {
    const server = serverWithUrl(url);
    const { container } = mount();
    await screen.findByRole("button", { name: "Копировать код" });
    expect(container.ownerDocument.querySelector("pre code")?.textContent).toBe(expected);
    expect(expected).not.toContain(codeFixture.ad_code_id);
    expect(expected).not.toContain("data-ad-code-id");
    fireEvent.click(screen.getByRole("button", { name: "Копировать код" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith(expected));
    expect(server.calls.some(req => req.method === "GET" && req.url.pathname === "/api/publisher/ad-codes")).toBe(true);
  });

  it.each([
    "https://pub.example.test/static/adManager.js",
    "https://twinbid.io/static/adManager.js?version=1",
  ])("shows the existing error and no copy action for %s, then recovers from updated API metadata", async url => {
    const server = serverWithUrl(url);
    mount();
    await screen.findByRole("alert");
    expect(document.querySelector("pre code")).toBeNull();
    expect(screen.queryByRole("button", { name: "Копировать код" })).not.toBeInTheDocument();
    expect(writeText).not.toHaveBeenCalled();

    server.overrides.set("GET /api/publisher/ad-codes", () => reply({
      items: [codeFixture], limit: 50, offset: 0, public_script_url: "https://stable.example.test/sdk.js",
    }));
    fireEvent.click(screen.getByRole("button", { name: "Обновить данные" }));
    fireEvent.click(await screen.findByRole("button", { name: "Копировать код" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith(
      '<script async src="https://stable.example.test/sdk.js" food="tb_server_only"></script>',
    ));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
