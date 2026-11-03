import { motion, useScroll, useTransform, MotionValue } from "framer-motion";
import { useRef, ReactNode, useEffect, useState, createElement } from "react";
import { cn } from "@/lib/utils";

/** True on touch/small screens or when user prefers reduced motion. */
function useLightMode() {
  const [light, setLight] = useState(() => {
    if (typeof window === "undefined") return false;
    return window.matchMedia("(max-width: 900px)").matches
      || window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  });
  useEffect(() => {
    const mqMobile = window.matchMedia("(max-width: 900px)");
    const mqReduced = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setLight(mqMobile.matches || mqReduced.matches);
    update();
    mqMobile.addEventListener("change", update);
    mqReduced.addEventListener("change", update);
    return () => {
      mqMobile.removeEventListener("change", update);
      mqReduced.removeEventListener("change", update);
    };
  }, []);
  return light;
}

export function WordsReveal({
  text,
  className = "",
  delay = 0,
  stagger = 0.045,
  duration = 0.9,
  as: Tag = "span",
  brandWord,
  brandClass = "text-primary",
}: {
  text: string;
  className?: string;
  delay?: number;
  stagger?: number;
  duration?: number;
  as?: "h1" | "h2" | "h3" | "p" | "span" | "div";
  brandWord?: string;
  brandClass?: string;
}) {
  const light = useLightMode();
  const words = text.split(" ");

  if (light) {
    return createElement(
      Tag,
      { className },
      words.map((w, i) => (
        <span key={i} className={cn("inline", brandWord && w === brandWord && brandClass)}>
          {w}
          {i < words.length - 1 ? " " : ""}
        </span>
      ))
    );
  }

  const MotionTag = motion[Tag as keyof typeof motion] as unknown as typeof motion.span;
  return (
    <MotionTag
      key={text}
      initial="hidden"
      whileInView="show"
      viewport={{ once: true, margin: "-15% 0px" }}
      variants={{ hidden: {}, show: { transition: { staggerChildren: stagger, delayChildren: delay } } }}
      className={className}
    >
      {words.map((w, i) => (
        <span key={i}>
          <span
            className="inline-block align-bottom"
            style={{
              lineHeight: "inherit",
              overflow: "hidden",
              paddingTop: "0.22em",
              paddingBottom: "0.12em",
              marginTop: "-0.22em",
              marginBottom: "-0.12em",
            }}
          >
            <motion.span
              className={cn("inline-block", brandWord && w === brandWord && brandClass)}
              variants={{
                hidden: { y: "110%", opacity: 0 },
                show: { y: "0%", opacity: 1, transition: { duration, ease: [0.22, 1, 0.36, 1] } },
              }}
            >
              {w}
            </motion.span>
          </span>
          {i < words.length - 1 ? " " : ""}
        </span>
      ))}
    </MotionTag>
  );
}

export function LineReveal({
  children,
  className = "",
  delay = 0,
  y = 24,
}: { children: ReactNode; className?: string; delay?: number; y?: number }) {
  const light = useLightMode();
  if (light) return <div className={className}>{children}</div>;
  return (
    <motion.div
      initial={{ opacity: 0, y }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-10% 0px" }}
      transition={{ duration: 0.9, delay, ease: [0.22, 1, 0.36, 1] }}
      className={className}
    >
      {children}
    </motion.div>
  );
}

export function CircularBadge({ text, size = 132 }: { text: string; size?: number }) {
  const light = useLightMode();
  const id = "circular-path";
  const label = text.trim().replace(/\s+/g, " ");
  const pathLength = 2 * Math.PI * 38;
  const circularText = `${label}\u2009•\u2009${label}\u2009•\u2009`;
  const inner = (
    <svg viewBox="0 0 100 100" className="w-full h-full">
      <defs>
        <path id={id} d="M 50,50 m -38,0 a 38,38 0 1,1 76,0 a 38,38 0 1,1 -76,0" />
      </defs>
      <text
        fill="hsl(var(--foreground))"
        style={{
          fontSize: "6.5px",
          letterSpacing: "0.28em",
          fontFamily: "JetBrains Mono, monospace",
        }}
      >
        <textPath
          href={`#${id}`}
          startOffset="0"
          textLength={pathLength}
          lengthAdjust="spacing"
        >
          {circularText}
        </textPath>
      </text>
    </svg>
  );
  if (light) {
    return (
      <div style={{ width: size, height: size }} className="relative">
        {inner}
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="w-2 h-2 rounded-full bg-primary" />
        </div>
      </div>
    );
  }
  return (
    <motion.div
      animate={{ rotate: 360 }}
      transition={{ duration: 22, ease: "linear", repeat: Infinity }}
      style={{ width: size, height: size }}
      className="relative"
    >
      {inner}
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="w-2 h-2 rounded-full bg-primary glow-primary" />
      </div>
    </motion.div>
  );
}

export function Parallax({
  children,
  offset = 80,
  className = "",
}: { children: ReactNode; offset?: number; className?: string }) {
  const light = useLightMode();
  const ref = useRef<HTMLDivElement>(null);
  const { scrollYProgress } = useScroll({ target: ref, offset: ["start end", "end start"] });
  const y = useTransform(scrollYProgress, [0, 1], [offset, -offset]);
  if (light) return <div className={className}>{children}</div>;
  return (
    <div ref={ref} className={className}>
      <motion.div style={{ y }}>{children}</motion.div>
    </div>
  );
}

export function useSectionProgress(): [React.RefObject<HTMLDivElement>, MotionValue<number>] {
  const ref = useRef<HTMLDivElement>(null);
  const { scrollYProgress } = useScroll({ target: ref, offset: ["start end", "end start"] });
  return [ref, scrollYProgress];
}
