import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { useCallback, useEffect, useRef, useState, type MouseEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { loadPublisherLanding } from "@/lib/landingModules";

interface PortalState {
  x: number;
  y: number;
  radius: number;
  target: "/" | "/publisher";
  instant: boolean;
}

export function useRolePortalTransition() {
  const navigate = useNavigate();
  const reduceMotion = useReducedMotion();
  const timerRef = useRef<number>();
  const requestRef = useRef(0);
  const busyRef = useRef(false);
  const [portal, setPortal] = useState<PortalState | null>(null);
  const [preparing, setPreparing] = useState(false);

  useEffect(() => () => {
    requestRef.current += 1;
    window.clearTimeout(timerRef.current);
    document.body.style.overflow = "";
  }, []);

  const openPortal = useCallback((event: MouseEvent<HTMLElement>, target: "/" | "/publisher") => {
    if (busyRef.current) return;
    busyRef.current = true;
    setPreparing(true);
    const request = ++requestRef.current;

    const rect = event.currentTarget.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    const radius = Math.hypot(
      Math.max(x, window.innerWidth - x),
      Math.max(y, window.innerHeight - y),
    );

    const pageReady = target === "/publisher" ? loadPublisherLanding() : Promise.resolve();
    void pageReady.then(() => {
      if (request !== requestRef.current) return;
      setPreparing(false);
      document.body.style.overflow = "hidden";
      setPortal({ x, y, radius, target, instant: Boolean(reduceMotion) });
      timerRef.current = window.setTimeout(() => {
        if (request !== requestRef.current) return;
        navigate(target, { state: { rolePortalTransition: true } });
        window.scrollTo({ top: 0 });
      }, reduceMotion ? 0 : 780);
    }).catch(() => {
      if (request !== requestRef.current) return;
      setPreparing(false);
      setPortal(null);
      busyRef.current = false;
      document.body.style.overflow = "";
    });
  }, [navigate, reduceMotion]);

  const portalOverlay = (
    <AnimatePresence>
      {portal && (
        <motion.div
          key="role-portal"
          className={`pointer-events-none fixed z-[100] bg-[#05090a] ${portal.instant ? "inset-0" : "rounded-full"}`}
          style={portal.instant ? undefined : {
            left: portal.x - portal.radius,
            top: portal.y - portal.radius,
            width: portal.radius * 2,
            height: portal.radius * 2,
          }}
          initial={portal.instant ? false : { scale: 0.015 }}
          animate={portal.instant ? undefined : { scale: 1.04 }}
          transition={{ duration: 0.78, ease: [0.76, 0, 0.24, 1] }}
        />
      )}
    </AnimatePresence>
  );

  return { openPortal, portalOverlay, transitioning: preparing || Boolean(portal) };
}

export function RolePortalReveal() {
  const location = useLocation();
  const navigate = useNavigate();
  const shouldReveal = Boolean((location.state as { rolePortalTransition?: boolean } | null)?.rolePortalTransition);
  const [visible, setVisible] = useState(shouldReveal);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!visible) return;
    let cancelled = false;
    let firstFrame = 0;
    let secondFrame = 0;
    const schedule = window.requestAnimationFrame ?? ((callback: FrameRequestCallback) => window.setTimeout(() => callback(performance.now()), 16));
    const cancel = window.cancelAnimationFrame ?? window.clearTimeout;

    void Promise.resolve(document.fonts?.ready).then(() => {
      if (cancelled) return;
      firstFrame = schedule(() => {
        secondFrame = schedule(() => {
          if (!cancelled) setReady(true);
        });
      });
    });
    return () => {
      cancelled = true;
      cancel(firstFrame);
      cancel(secondFrame);
    };
  }, [visible]);

  if (!visible) return null;

  return (
    <motion.div
      className="pointer-events-none fixed inset-0 z-[100] bg-[#05090a]"
      initial={{ opacity: 1 }}
      animate={{ opacity: ready ? 0 : 1 }}
      transition={{ duration: 0.72, delay: 0.08, ease: [0.22, 1, 0.36, 1] }}
      onAnimationComplete={() => {
        if (ready) {
          setVisible(false);
          navigate(`${location.pathname}${location.search}`, { replace: true, state: null });
        }
      }}
    />
  );
}
