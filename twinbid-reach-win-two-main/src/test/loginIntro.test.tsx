import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LoginIntroOverlay } from "@/components/dashboard/LoginIntroOverlay";
import {
  consumeLoginIntroRequest,
  LOGIN_INTRO_REQUEST_EVENT,
  requestLoginIntro,
} from "@/lib/loginIntro";

describe("login intro request", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.restoreAllMocks();
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("dispatches the request and consumes it only once", () => {
    const listener = vi.fn();
    window.addEventListener(LOGIN_INTRO_REQUEST_EVENT, listener);

    requestLoginIntro();

    expect(listener).toHaveBeenCalledTimes(1);
    expect(consumeLoginIntroRequest()).toBe(true);
    expect(consumeLoginIntroRequest()).toBe(false);

    window.removeEventListener(LOGIN_INTRO_REQUEST_EVENT, listener);
  });

  it("does not replay a stale login request", () => {
    vi.spyOn(Date, "now").mockReturnValue(1_000);
    requestLoginIntro();

    expect(consumeLoginIntroRequest(61_001)).toBe(false);
    expect(consumeLoginIntroRequest(61_001)).toBe(false);
  });

  it("plays after login and removes the overlay after the fade", () => {
    vi.useFakeTimers();
    const { container } = render(<LoginIntroOverlay />);

    expect(container.querySelector("video")).toBeNull();

    act(() => requestLoginIntro());

    const video = container.querySelector("video");
    expect(video).not.toBeNull();
    expect(video).toHaveAttribute("src", "/login-intro-desktop.mp4");

    fireEvent.ended(video!);
    expect(video?.parentElement).toHaveClass("opacity-0");

    act(() => vi.advanceTimersByTime(750));
    expect(container.querySelector("video")).toBeNull();
  });

  it("skips the intro when the user taps or clicks the overlay", () => {
    vi.useFakeTimers();
    const { container } = render(<LoginIntroOverlay />);

    act(() => requestLoginIntro());

    const video = container.querySelector("video");
    expect(video).not.toBeNull();

    fireEvent.pointerDown(video!.parentElement!);
    expect(video?.parentElement).toHaveClass("opacity-0");

    act(() => vi.advanceTimersByTime(750));
    expect(container.querySelector("video")).toBeNull();
  });
});
