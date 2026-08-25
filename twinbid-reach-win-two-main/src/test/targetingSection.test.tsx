// @vitest-environment jsdom
import { useState } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { TargetingSection } from "@/components/dashboard/TargetingSection";
import { LanguageProvider } from "@/contexts/LanguageContext";
import type { TargetingState } from "@/contexts/CampaignContext";

function Harness() {
  const [lists, setLists] = useState<Record<string, TargetingState>>({
    country: { mode: "white", items: ["US", "DE"] },
  });

  return (
    <LanguageProvider>
      <TargetingSection
        lists={lists}
        onUpdate={(key, updates) => {
          setLists(current => ({
            ...current,
            [key]: { ...current[key], mode: current[key]?.mode ?? "none", items: current[key]?.items ?? [], ...updates },
          }));
        }}
      />
    </LanguageProvider>
  );
}

function VpnHarness() {
  const [blocked, setBlocked] = useState(false);
  return (
    <LanguageProvider>
      <TargetingSection
        lists={{}}
        onUpdate={() => undefined}
        blockVpnTraffic={blocked}
        onBlockVpnTrafficChange={setBlocked}
      />
    </LanguageProvider>
  );
}

describe("targeting list controls", () => {
  beforeEach(() => {
    window.localStorage.setItem("twinbid_lang", "en");
  });

  it("clears all selected values in one targeting without changing its mode", () => {
    render(<Harness />);

    const countriesCard = screen.getByText("Countries").closest("div.rounded-lg");
    expect(countriesCard).not.toBeNull();
    expect(within(countriesCard as HTMLElement).getByText("United States (US)")).toBeInTheDocument();
    expect(within(countriesCard as HTMLElement).getByText("Germany (DE)")).toBeInTheDocument();

    fireEvent.click(within(countriesCard as HTMLElement).getByRole("button", { name: "Clear" }));

    expect(within(countriesCard as HTMLElement).queryByText("United States (US)")).not.toBeInTheDocument();
    expect(within(countriesCard as HTMLElement).queryByText("Germany (DE)")).not.toBeInTheDocument();
    expect(within(countriesCard as HTMLElement).getByRole("button", { name: "White" })).toHaveClass("bg-green-600");
  });

  it("accepts IPv4 CIDR subnets in IP targeting", () => {
    render(<Harness />);

    const ipCard = screen.getByText("IP addresses").closest("div.rounded-lg");
    expect(ipCard).not.toBeNull();
    fireEvent.click(within(ipCard as HTMLElement).getByRole("button", { name: "White" }));

    const input = within(ipCard as HTMLElement).getByPlaceholderText("192.168.1.1, 10.0.0.0/24");
    fireEvent.change(input, { target: { value: "10.20.0.0/16" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(within(ipCard as HTMLElement).getByText("10.20.0.0/16")).toBeInTheDocument();
  });

  it("keeps VPN filtering disabled by default and lets the advertiser enable it", () => {
    render(<VpnHarness />);

    const toggle = screen.getByRole("switch", { name: "Block VPN traffic" });
    expect(toggle).not.toBeChecked();
    expect(screen.getByText("VPN filtering is disabled")).toBeInTheDocument();

    fireEvent.click(toggle);
    expect(toggle).toBeChecked();
    expect(screen.getByText("VPN, proxy, Tor and datacenter traffic will be excluded")).toBeInTheDocument();
  });
});
