import { motion } from "framer-motion";
import { useIsMobileImmediate } from "@/hooks/use-mobile";

export function AnimatedBackground() {
  const isMobile = useIsMobileImmediate();
  return (
    <div className="pointer-events-none fixed inset-0 -z-10 overflow-hidden" aria-hidden>
      <div className="landing-global-grid absolute inset-0 opacity-70" />
      {!isMobile && (
        <>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 1.2 }}
            className="absolute -left-[18vw] top-[8vh] h-[58vw] w-[58vw] rounded-full bg-primary/[0.07] blur-[120px]"
          />
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 1.5, delay: 0.2 }}
            className="absolute -right-[18vw] top-[46vh] h-[52vw] w-[52vw] rounded-full bg-accent/[0.055] blur-[130px]"
          />
        </>
      )}
    </div>
  );
}

export function ScrollReveal({ children, delay = 0, y = 30 }: { children: React.ReactNode; delay?: number; y?: number }) {
  const isMobile = useIsMobileImmediate();
  return (
    <motion.div
      initial={isMobile ? false : { opacity: 0, y }}
      whileInView={isMobile ? undefined : { opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.7, delay, ease: "easeOut" }}
    >
      {children}
    </motion.div>
  );
}
