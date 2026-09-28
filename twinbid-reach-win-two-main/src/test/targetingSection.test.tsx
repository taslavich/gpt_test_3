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

function OsVersionHarness() {
  const [lists, setLists] = useState<Record<string, TargetingState>>({
    os: { mode: "white", items: ["Windows"] },
    osVersion: { mode: "none", items: [] },
  });
  return <LanguageProvider><TargetingSection lists={lists} onUpdate={(key, updates) => {
    setLists(current => ({ ...current, [key]: { ...current[key], ...updates } }));
  }} /></LanguageProvider>;
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

  it("only enables version targeting for allowlisted iOS or Android", () => {
    render(<OsVersionHarness />);
    const versionCard = screen.getByText("OS versions").closest("div.rounded-lg") as HTMLElement;
    expect(within(versionCard).getByRole("button", { name: "White" })).toBeDisabled();
    expect(within(versionCard).queryByRole("textbox")).not.toBeInTheDocument();

    const osCard = screen.getByText("OS", { exact: true }).closest("div.rounded-lg") as HTMLElement;
    const input = within(osCard).getByRole("textbox");
    fireEvent.change(input, { target: { value: "iOS" } });
    fireEvent.mouseDown(within(osCard).getByRole("button", { name: "iOS" }));
    expect(within(versionCard).getByRole("button", { name: "White" })).toBeEnabled();
    fireEvent.click(within(versionCard).getByRole("button", { name: "White" }));
    fireEvent.change(within(versionCard).getByRole("textbox"), { target: { value: "iOS 13.5" } });
    fireEvent.mouseDown(within(versionCard).getByRole("button", { name: "iOS 13.5" }));
    expect(within(versionCard).getByText("iOS 13.5")).toBeInTheDocument();
    expect(within(versionCard).queryByText("Android 13")).not.toBeInTheDocument();
  });

  it("removes minor choices under a selected major and restores them when it is cleared", () => {
    render(<OsVersionHarness />);
    const osCard = screen.getByText("OS", { exact: true }).closest("div.rounded-lg") as HTMLElement;
    fireEvent.change(within(osCard).getByRole("textbox"), { target: { value: "iOS" } });
    fireEvent.mouseDown(within(osCard).getByRole("button", { name: "iOS" }));
    const card = screen.getByText("OS versions").closest("div.rounded-lg") as HTMLElement;
    fireEvent.click(within(card).getByRole("button", { name: "White" }));
    const input = within(card).getByRole("textbox");
    fireEvent.change(input, { target: { value: "iOS 18" } });
    fireEvent.mouseDown(within(card).getByRole("button", { name: "iOS 18" }));
    expect(within(card).getByText("iOS 18")).toBeInTheDocument();
    fireEvent.change(input, { target: { value: "iOS 18.7" } });
    expect(within(card).queryByRole("button", { name: "iOS 18.7" })).not.toBeInTheDocument();
    fireEvent.click(within(card).getByText("iOS 18").querySelector("svg") as Element);
    fireEvent.change(input, { target: { value: "iOS 18.7" } });
    expect(within(card).getByRole("button", { name: "iOS 18.7" })).toBeInTheDocument();
  });
});
