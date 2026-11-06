import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
  act,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import PublisherSettings from "@/pages/PublisherSettings";
import { api } from "@/api";
import { useProfile } from "@/contexts/ProfileContext";
import {
  PublisherTestWrapper,
  publisherServer,
  userFixture,
  languageFixture,
  profileFixture,
  settingsFixture,
} from "./publisher-fixtures";
vi.mock("@/api", () => ({ api: { changePassword: vi.fn() } }));
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ user: userFixture, signOut: vi.fn() }),
}));
vi.mock("@/contexts/ProfileContext", () => ({ useProfile: vi.fn() }));
vi.mock("@/contexts/LanguageContext", () => ({
  useLanguage: () => languageFixture,
}));
vi.mock("@/contexts/DashboardThemeContext", () => ({
  useDashboardTheme: () => ({ theme: "dark", setTheme: vi.fn() }),
}));
const updateProfile = vi.fn().mockResolvedValue(undefined);
beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
  vi.mocked(useProfile).mockReturnValue({
    profile: profileFixture,
    loading: false,
    updateProfile,
    refetch: vi.fn(),
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
const tab = (name: string) =>
  fireEvent.mouseDown(screen.getByRole("tab", { name }), {
    button: 0,
    ctrlKey: false,
  });
describe("Real Publisher preferences with retained account controls", () => {
  it("loads and saves the complete settings object without business localStorage", async () => {
    const s = publisherServer();
    render(<PublisherSettings />, { wrapper: PublisherTestWrapper });
    await waitFor(() =>
      expect(s.calls.some((c) => c.url.pathname.endsWith("/settings"))).toBe(
        true,
      ),
    );
    tab("Выплаты");
    await screen.findByDisplayValue(settingsFixture.wallet);
    fireEvent.change(screen.getByLabelText("Адрес кошелька"), {
      target: { value: "0x" + "b".repeat(40) },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    await waitFor(() =>
      expect(
        s.calls.find(
          (c) => c.method === "PATCH" && c.url.pathname.endsWith("/settings"),
        )?.body,
      ).toEqual({ ...settingsFixture, wallet: "0x" + "b".repeat(40) }),
    );
    tab("Уведомления");
    fireEvent.click(
      screen.getByRole("switch", { name: "Проблемы с рекламным кодом" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    await waitFor(() =>
      expect(
        s.calls
          .filter(
            (c) => c.method === "PATCH" && c.url.pathname.endsWith("/settings"),
          )
          .at(-1)?.body?.codeAlerts,
      ).toBe(false),
    );
    expect(
      Object.keys(localStorage).some((key) => key.includes("publisher")),
    ).toBe(false);
  });
  it("defaults an unset profile timezone to UTC+0 and saves it through the profile API", async () => {
    publisherServer();
    vi.mocked(useProfile).mockReturnValue({ profile: { ...profileFixture, timezone: null }, loading: false, updateProfile, refetch: vi.fn() });
    render(<PublisherSettings />, { wrapper: PublisherTestWrapper });
    expect(screen.getByRole("combobox", { name: "Часовой пояс" })).toHaveTextContent("UTC+0");
    fireEvent.click(screen.getByRole("combobox", { name: "Часовой пояс" }));
    fireEvent.click(await screen.findByRole("option", { name: "UTC+5:30 (IST)" }));
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    await waitFor(() => expect(updateProfile).toHaveBeenCalledWith(expect.objectContaining({ timezone: "Asia/Kolkata" })));
  });
  it("preserves profile, password and theme tabs using the existing APIs", async () => {
    publisherServer();
    vi.mocked(api.changePassword).mockResolvedValue(undefined);
    render(<PublisherSettings />, { wrapper: PublisherTestWrapper });
    expect(screen.getByRole("tab", { name: "Профиль" })).toBeInTheDocument();
    expect(
      screen.getByRole("tab", { name: "Цвет интерфейса" }),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Имя"), {
      target: { value: "Updated" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith({
        fullName: "Updated",
        telegram: "",
        timezone: "Etc/GMT-3",
      }),
    );
    tab("Безопасность");
    fireEvent.change(screen.getByLabelText("Новый пароль"), {
      target: { value: "new-secret" },
    });
    fireEvent.change(screen.getByLabelText("Повторите пароль"), {
      target: { value: "new-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сменить пароль" }));
    await waitFor(() =>
      expect(api.changePassword).toHaveBeenCalledWith({
        new_password: "new-secret",
      }),
    );
  });
});


it("keeps the settings page and tabs visible while the profile is slow", () => {
  vi.useFakeTimers();
  publisherServer();
  vi.mocked(useProfile).mockReturnValue({ profile: null, loading: true, updateProfile, refetch: vi.fn() });
  render(<PublisherSettings />, { wrapper: PublisherTestWrapper });
  expect(screen.getByRole("tab", { name: "Профиль" })).toBeInTheDocument();
  expect(screen.getByRole("tab", { name: "Безопасность" })).toBeInTheDocument();
  expect(screen.getByLabelText("Имя")).toBeDisabled();
  expect(screen.getByRole("status")).toHaveTextContent("Загрузка профиля");
  act(() => { vi.advanceTimersByTime(12000); });
  expect(screen.getByRole("status")).toHaveTextContent("дольше обычного");
  expect(screen.getByRole("button", { name: "Обновить страницу" })).toBeInTheDocument();
});

it("shows a retry action instead of a blank profile when the API failed", () => {
  publisherServer();
  const refetch = vi.fn();
  vi.mocked(useProfile).mockReturnValue({ profile: null, loading: false, updateProfile, refetch });
  render(<PublisherSettings />, { wrapper: PublisherTestWrapper });
  expect(screen.getByRole("alert")).toHaveTextContent("Не удалось загрузить профиль");
  fireEvent.click(screen.getByRole("button", { name: "Повторить" }));
  expect(refetch).toHaveBeenCalledOnce();
});
