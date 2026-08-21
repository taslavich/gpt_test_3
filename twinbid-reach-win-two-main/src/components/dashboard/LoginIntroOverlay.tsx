import { useCallback, useEffect, useRef, useState } from "react";
import {
  consumeLoginIntroRequest,
  LOGIN_INTRO_REQUEST_EVENT,
} from "@/lib/loginIntro";

const FADE_DURATION_MS = 750;
const PLAYBACK_FAILSAFE_MS = 7_000;

type IntroPlayback = {
  id: number;
  source: string;
};

type NetworkInformation = {
  saveData?: boolean;
};

type NavigatorWithConnection = Navigator & {
  connection?: NetworkInformation;
};

function createPlayback(): IntroPlayback | null {
  if (!consumeLoginIntroRequest()) return null;

  const prefersReducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
  const saveDataEnabled = (navigator as NavigatorWithConnection).connection?.saveData === true;

  // Respect explicit accessibility/data-saving preferences and reveal the
  // already-loading dashboard immediately in those cases.
  if (prefersReducedMotion || saveDataEnabled) return null;

  const isMobile = window.matchMedia?.("(max-width: 767px)").matches;

  return {
    id: Date.now(),
    source: isMobile ? "/login-intro-mobile.mp4" : "/login-intro-desktop.mp4",
  };
}

/**
 * Lightweight one-shot intro shown only after a successful interactive login.
 * It lives above the router so dashboard chunks and API data load underneath it.
 */
export function LoginIntroOverlay() {
  const [playback, setPlayback] = useState<IntroPlayback | null>(() => createPlayback());
  const [isFading, setIsFading] = useState(false);
  const fadeTimerRef = useRef<number | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);

  const clearFadeTimer = useCallback(() => {
    if (fadeTimerRef.current !== null) {
      window.clearTimeout(fadeTimerRef.current);
      fadeTimerRef.current = null;
    }
  }, []);

  const finishPlayback = useCallback(() => {
    if (fadeTimerRef.current !== null) return;

    setIsFading(true);
    fadeTimerRef.current = window.setTimeout(() => {
      setPlayback(null);
      setIsFading(false);
      fadeTimerRef.current = null;
    }, FADE_DURATION_MS);
  }, []);

  useEffect(() => {
    const handleLoginIntroRequest = () => {
      clearFadeTimer();
      setIsFading(false);
      setPlayback(createPlayback());
    };

    window.addEventListener(LOGIN_INTRO_REQUEST_EVENT, handleLoginIntroRequest);
    return () => window.removeEventListener(LOGIN_INTRO_REQUEST_EVENT, handleLoginIntroRequest);
  }, [clearFadeTimer]);

  useEffect(() => {
    if (!playback) return;

    const previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    // A broken/slow media response must never keep the user away from the cabinet.
    const failsafeTimer = window.setTimeout(finishPlayback, PLAYBACK_FAILSAFE_MS);

    return () => {
      window.clearTimeout(failsafeTimer);
      document.body.style.overflow = previousBodyOverflow;
    };
  }, [finishPlayback, playback]);

  useEffect(() => {
    if (!playback) return;

    // Muted inline video normally autoplays everywhere. If a browser still
    // rejects playback, reveal the dashboard instead of waiting on a black screen.
    const playPromise = videoRef.current?.play();
    playPromise?.catch(finishPlayback);
  }, [finishPlayback, playback]);

  useEffect(() => clearFadeTimer, [clearFadeTimer]);

  if (!playback) return null;

  return (
    <div
      aria-hidden="true"
      onPointerDown={finishPlayback}
      className={`fixed inset-0 z-[10000] flex items-center justify-center bg-black transition-opacity ease-out ${
        isFading ? "opacity-0" : "opacity-100"
      }`}
      style={{ transitionDuration: `${FADE_DURATION_MS}ms`, willChange: "opacity" }}
    >
      <video
        key={playback.id}
        ref={videoRef}
        autoPlay
        muted
        playsInline
        preload="auto"
        disablePictureInPicture
        controlsList="nodownload noplaybackrate noremoteplayback"
        className="h-full w-full bg-black object-contain"
        src={playback.source}
        onEnded={finishPlayback}
        onError={finishPlayback}
      />
    </div>
  );
}
