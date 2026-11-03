import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { AuthDialog } from "@/components/landing/AuthDialog";

const auth = vi.hoisted(() => ({
  signIn: vi.fn(), signUp: vi.fn(), signOut: vi.fn(),
}));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => auth }));
vi.mock("@/contexts/LanguageContext", () => ({ useLanguage: () => ({ t: (key: string) => key }) }));

function mount(defaultTab: "login" | "register", defaultRole: "advertiser" | "publisher") {
  return render(<MemoryRouter initialEntries={["/publisher"]}><Routes>
    <Route path="/publisher" element={<AuthDialog defaultTab={defaultTab} defaultRole={defaultRole} trigger={<button>Open auth</button>} />} />
    <Route path="/publisher/overview" element={<div>Publisher cabinet</div>} />
    <Route path="/dashboard" element={<div>Advertiser cabinet</div>} />
  </Routes></MemoryRouter>);
}

describe("one account, cabinet selection at login", () => {
  beforeEach(() => {
    vi.stubGlobal("ResizeObserver", class {
      observe() {}
      unobserve() {}
      disconnect() {}
    });
    localStorage.clear();
    auth.signIn.mockReset().mockResolvedValue({ error: null });
    auth.signUp.mockReset().mockResolvedValue({ error: null });
    auth.signOut.mockReset().mockResolvedValue(undefined);
  });

  it("authenticates a publisher through the same password login and routes to the publisher cabinet", async () => {
    mount("login", "publisher");
    fireEvent.click(screen.getByRole("button", { name: "Open auth" }));
    expect(screen.getByRole("radio", { name: "auth.role.publisher" })).toBeChecked();
    fireEvent.change(screen.getByLabelText("auth.email"), { target: { value: "Pub@Example.com" } });
    fireEvent.change(screen.getByLabelText("auth.password"), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "auth.loginBtn" }));
    await waitFor(() => expect(screen.getByText("Publisher cabinet")).toBeInTheDocument());
    expect(auth.signIn).toHaveBeenCalledWith("Pub@Example.com", "secret");
    expect(JSON.parse(localStorage.getItem("twinbid_login_role_v2") || "null")).toEqual({ email: "pub@example.com", role: "publisher" });
  });

  it("uses one registration form without a role and does not retain a logged-in role", async () => {
    mount("register", "publisher");
    fireEvent.click(screen.getByRole("button", { name: "Open auth" }));
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("auth.email"), { target: { value: "new@example.com" } });
    fireEvent.change(screen.getByLabelText("auth.password"), { target: { value: "secret123" } });
    fireEvent.change(screen.getByLabelText("auth.confirmPassword"), { target: { value: "secret123" } });
    fireEvent.change(screen.getByLabelText("auth.telegram"), { target: { value: "@new" } });
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "auth.registerBtn" }));
    await waitFor(() => expect(auth.signUp).toHaveBeenCalledWith("new@example.com", "secret123", "", "@new"));
    await waitFor(() => expect(auth.signOut).toHaveBeenCalledOnce());
    expect(localStorage.getItem("twinbid_login_role_v2")).toBeNull();
  });
});
