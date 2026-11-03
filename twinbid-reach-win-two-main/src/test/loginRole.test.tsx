import { render, screen, act } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { rememberLoginRole, RoleProvider, useRole } from "@/contexts/RoleContext";

const session = vi.hoisted(() => ({ user: { email: "a@example.com" } as { email: string } | null }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => session }));

function CurrentRole() {
  const { role } = useRole();
  return <div>{role ?? "choose role at login"}</div>;
}

describe("login cabinet scope", () => {
  beforeEach(() => { localStorage.clear(); session.user = { email: "a@example.com" }; });
  it("keeps the selected cabinet across reloads for that account, without leaking it to another", () => {
    const view = render(<RoleProvider><CurrentRole /></RoleProvider>);
    expect(screen.getByText("choose role at login")).toBeInTheDocument();
    act(() => rememberLoginRole("publisher", "A@Example.com"));
    expect(screen.getByText("publisher")).toBeInTheDocument();
    view.unmount();
    render(<RoleProvider><CurrentRole /></RoleProvider>);
    expect(screen.getByText("publisher")).toBeInTheDocument();
    session.user = { email: "b@example.com" };
    act(() => rememberLoginRole("advertiser", "a@example.com"));
    expect(screen.getByText("choose role at login")).toBeInTheDocument();
  });
});
