import { useEffect, useRef, useState, type CSSProperties } from "react";
import { cn } from "@/lib/utils";

interface TypedValueProps {
  value: string;
  className?: string;
  delay?: number;
}

export function TypedValue({ value, className, delay = 0 }: TypedValueProps) {
  const ref = useRef<HTMLSpanElement | null>(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    if (!("IntersectionObserver" in window)) {
      setVisible(true);
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry.isIntersecting) return;
        setVisible(true);
        observer.disconnect();
      },
      { threshold: 0.45 },
    );

    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const style = {
    "--typed-steps": Math.max(2, value.length),
    "--typed-delay": `${delay}ms`,
  } as CSSProperties;

  return (
    <span ref={ref} className={cn("typed-value", visible && "is-visible", className)} style={style} aria-label={value}>
      <span aria-hidden="true">{value}</span>
    </span>
  );
}
