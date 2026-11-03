import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { useRolePortalTransition } from "@/components/landing/RolePortalTransition";
import { loadPublisherLanding } from "@/lib/landingModules";

vi.mock("@/lib/landingModules", () => ({ loadPublisherLanding: vi.fn() }));

function AdvertiserLink() {
  const { openPortal, portalOverlay } = useRolePortalTransition();
  return <><button onClick={event => openPortal(event, "/publisher")}>Паблишерам</button>{portalOverlay}</>;
}

describe("publisher portal", () => {
  it("keeps the advertiser page until the publisher route has loaded", async () => {
    window.scrollTo = vi.fn();
    let finishLoading!: (value: unknown) => void;
    vi.mocked(loadPublisherLanding).mockReturnValue(new Promise(resolve => { finishLoading = resolve; }) as ReturnType<typeof loadPublisherLanding>);

    render(<MemoryRouter initialEntries={["/"]}><Routes>
      <Route path="/" element={<AdvertiserLink />} />
      <Route path="/publisher" element={<h1>Лендинг паблишеров</h1>} />
    </Routes></MemoryRouter>);

    fireEvent.click(screen.getByRole("button", { name: "Паблишерам" }));
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 830)); });
    expect(screen.getByRole("button", { name: "Паблишерам" })).toBeInTheDocument();
    expect(screen.queryByText("Лендинг паблишеров")).not.toBeInTheDocument();

    await act(async () => { finishLoading({ default: () => null }); });
    expect(screen.getByRole("button", { name: "Паблишерам" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("Лендинг паблишеров")).toBeInTheDocument());
  });
});
