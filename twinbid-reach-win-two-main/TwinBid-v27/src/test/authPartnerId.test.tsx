import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ApiUser, SignupRequest } from "@/api/types";
import { AuthProvider, useAuth } from "@/contexts/AuthContext";
import { LanguageProvider } from "@/contexts/LanguageContext";
import { capturePartnerCodeFromUrl } from "@/lib/partners";

const apiMock = vi.hoisted(() => ({
  getSession: vi.fn(),
  signup: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
}));

vi.mock("@/api", () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    status = 500;
  },
}));

function createApiUser(body: SignupRequest): ApiUser {
  return {
    login: body.email,
    mail: body.email,
    name: body.full_name || "",
    telegram: body.telegram,
    manager_telegram: body.manager_telegram,
    balance: 0,
    timezone: "utc_3",
    email_notifications: true,
    campaign_status_notifications: true,
    low_balance_notifications: true,
    balance_treshold: 100,
    partner_id: body.partner_id,
    partner: body.partner ?? null,
  };
}

function wrapper({ children }: { children: ReactNode }) {
  return (
    <LanguageProvider>
      <AuthProvider>{children}</AuthProvider>
    </LanguageProvider>
  );
}

describe("signup partner attribution", () => {
  beforeEach(() => {
    localStorage.clear();
    window.history.replaceState({}, "", "/");
    apiMock.getSession.mockReset().mockResolvedValue(null);
    apiMock.signup.mockReset().mockImplementation(async (body: SignupRequest) => ({
      access_token: "access",
      refresh_token: "refresh",
      user: createApiUser(body),
    }));
    apiMock.login.mockReset();
  });

  it("sends an own partner_id and no referring partner without a referral URL", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.signUp("new@example.com", "secret", "New User", "@newuser");
    });

    const payload = apiMock.signup.mock.calls[0][0] as SignupRequest;
    expect(payload.partner_id).toMatch(/^TB[A-Z0-9]{10}$/);
    expect(payload.partner).toBeUndefined();
  });

  it("keeps the referring partner separate from the new user's partner_id", async () => {
    window.history.replaceState({}, "", "/?partner=ABC123");
    capturePartnerCodeFromUrl();
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.signUp("referred@example.com", "secret", undefined, "@referred");
    });

    const payload = apiMock.signup.mock.calls[0][0] as SignupRequest;
    expect(payload.partner_id).toMatch(/^TB[A-Z0-9]{10}$/);
    expect(payload.partner).toBe("ABC123");
    expect(payload.partner_id).not.toBe(payload.partner);
  });

  it("generates different partner ids for separate registrations", async () => {
    const first = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(first.result.current.loading).toBe(false));

    await act(async () => {
      await first.result.current.signUp("same@example.com", "secret", undefined, "@first");
    });
    const firstPartnerId = (apiMock.signup.mock.calls[0][0] as SignupRequest).partner_id;
    first.unmount();
    localStorage.clear();

    const second = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(second.result.current.loading).toBe(false));

    await act(async () => {
      await second.result.current.signUp("same@example.com", "secret", undefined, "@second");
    });
    const secondPartnerId = (apiMock.signup.mock.calls[1][0] as SignupRequest).partner_id;

    expect(firstPartnerId).not.toBe(secondPartnerId);
  });

  it("uses the backend partner_id after login without generating a replacement", async () => {
    apiMock.login.mockResolvedValue({
      access_token: "access",
      refresh_token: "refresh",
      user: {
        ...createApiUser({
          email: "existing@example.com",
          password: "secret",
          telegram: "@existing",
          manager_telegram: "GregTwinbid",
          partner_id: "TBBACKEND42X",
        }),
        partner_id: "TBBACKEND42X",
      },
    });

    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.signIn("existing@example.com", "secret");
    });

    expect(result.current.user?.partner_id).toBe("TBBACKEND42X");
    expect(apiMock.signup).not.toHaveBeenCalled();
  });
});
