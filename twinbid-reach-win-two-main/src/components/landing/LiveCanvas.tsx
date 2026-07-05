import { motion, useMotionValue, useSpring, useScroll, useTransform } from "framer-motion";
import { useEffect, useState } from "react";

export function LiveCanvas() {
  const mx = useMotionValue(typeof window !== "undefined" ? window.innerWidth / 2 : 600);
  const my = useMotionValue(typeof window !== "undefined" ? window.innerHeight / 2 : 400);
  const sx = useSpring(mx, { stiffness: 60, damping: 20, mass: 0.6 });
  const sy = useSpring(my, { stiffness: 60, damping: 20, mass: 0.6 });

  const { scrollY } = useScroll();
  const hueShift = useTransform(scrollY, [0, 4000], [0, 80]);
  const bgY1 = useTransform(scrollY, [0, 3000], [0, -400]);
  const bgY2 = useTransform(scrollY, [0, 3000], [0, 300]);
  const bgY3 = useTransform(scrollY, [0, 3000], [0, -200]);

  useEffect(() => {
    const onMove = (e: PointerEvent) => {
      mx.set(e.clientX);
      my.set(e.clientY);
    };
    window.addEventListener("pointermove", onMove);
    return () => window.removeEventListener("pointermove", onMove);
  }, [mx, my]);

  return (
    <div className="fixed inset-0 -z-10 overflow-hidden pointer-events-none" aria-hidden>
      <motion.div
        style={{ y: bgY1, filter: useTransform(hueShift, (h) => `hue-rotate(${h}deg)`) }}
        animate={{ scale: [1, 1.15, 1], rotate: [0, 25, 0] }}
        transition={{ duration: 28, repeat: Infinity, ease: "easeInOut" }}
        className="absolute -top-1/3 -left-1/4 w-[80vw] h-[80vw] rounded-full bg-primary/30 blur-[160px]"
      />
      <motion.div
        style={{ y: bgY2, filter: useTransform(hueShift, (h) => `hue-rotate(${-h}deg)`) }}
        animate={{ scale: [1, 0.85, 1.1, 1], rotate: [0, -30, 0] }}
        transition={{ duration: 34, repeat: Infinity, ease: "easeInOut" }}
        className="absolute top-1/2 -right-1/4 w-[90vw] h-[90vw] rounded-full bg-accent/25 blur-[180px]"
      />
      <motion.div
        style={{ y: bgY3 }}
        animate={{ scale: [1, 1.25, 1], x: [-100, 100, -100] }}
        transition={{ duration: 40, repeat: Infinity, ease: "easeInOut" }}
        className="absolute bottom-[-30%] left-1/4 w-[70vw] h-[70vw] rounded-full bg-primary/20 blur-[180px]"
      />

      <motion.div
        style={{ x: sx, y: sy, translateX: "-50%", translateY: "-50%" }}
        className="absolute w-[45vw] h-[45vw] rounded-full pointer-events-none"
      >
        <div
          className="w-full h-full rounded-full"
          style={{
            background:
              "radial-gradient(circle, hsl(var(--primary) / 0.55) 0%, hsl(var(--primary) / 0.28) 25%, hsl(var(--primary) / 0.12) 50%, transparent 75%)",
            filter: "blur(60px)",
          }}
        />
      </motion.div>
    </div>
  );
}

/** Infinite scrolling marquee strip — seamlessly loops with no jump. */
export function Marquee({ items }: { items: string[] }) {
  const group = Array.from({ length: 3 }, () => items).flat();

  return (
    <div className="relative overflow-hidden border-y border-border/60 bg-background/80 py-6">
      <div className="marquee-track flex w-max whitespace-nowrap will-change-transform">
        {[0, 1].map((copyIndex) => (
          <div key={copyIndex} className="flex shrink-0" aria-hidden={copyIndex === 1}>
            {group.map((t, i) => (
              <span
                key={`${copyIndex}-${i}`}
                className="flex items-center gap-10 pr-10 font-mono text-[13px] tracking-[0.18em] uppercase shrink-0 text-muted-foreground"
              >
                <span>{t}</span>
                <span className="w-1 h-1 rounded-full bg-primary shrink-0" />
              </span>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}
