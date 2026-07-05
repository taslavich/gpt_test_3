import { motion } from "framer-motion";

/* Minimal blueprint backdrop: one soft cool aurora at top.
   Hidden on mobile to remove the expensive blur(40px) compositing. */
export function AnimatedBackground() {
  return (
    <div className="fixed inset-0 -z-10 overflow-hidden pointer-events-none hidden md:block">
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 1.2 }}
        className="absolute top-0 left-1/2 -translate-x-1/2 w-[80vw] h-[60vh] rounded-full"
        style={{
          background:
            "radial-gradient(ellipse at center, hsla(168, 70%, 55%, 0.08) 0%, transparent 60%)",
          filter: "blur(40px)",
        }}
      />
    </div>
  );
}

export function ScrollReveal({ children, delay = 0, y = 30 }: { children: React.ReactNode; delay?: number; y?: number }) {
  return (
    <motion.div
      initial={{ opacity: 0, y }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.7, delay, ease: "easeOut" }}
    >
      {children}
    </motion.div>
  );
}
