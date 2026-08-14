// @vitest-environment jsdom
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useTouchScrollSelectionGuard } from "@/hooks/use-touch-scroll-selection-guard";

function Harness({ onSelect }: { onSelect: () => void }) {
  useTouchScrollSelectionGuard();
  return (
    <div data-traffic-calculator-root>
      <button type="button" onClick={onSelect}>Field</button>
    </div>
  );
}

describe("traffic calculator touch scroll guard", () => {
  it("blocks the synthetic click after a scroll gesture", () => {
    const onSelect = vi.fn();
    render(<Harness onSelect={onSelect} />);
    const field = screen.getByRole("button", { name: "Field" });

    fireEvent.touchStart(field, { touches: [{ clientX: 20, clientY: 100 }] });
    fireEvent.touchMove(field, { touches: [{ clientX: 20, clientY: 140 }] });
    fireEvent.touchEnd(field, { changedTouches: [{ clientX: 20, clientY: 140 }] });
    fireEvent.click(field);

    expect(onSelect).not.toHaveBeenCalled();
  });

  it("keeps a normal tap selectable", () => {
    const onSelect = vi.fn();
    render(<Harness onSelect={onSelect} />);
    const field = screen.getByRole("button", { name: "Field" });

    fireEvent.touchStart(field, { touches: [{ clientX: 20, clientY: 100 }] });
    fireEvent.touchEnd(field, { changedTouches: [{ clientX: 20, clientY: 100 }] });
    fireEvent.click(field);

    expect(onSelect).toHaveBeenCalledTimes(1);
  });
});
