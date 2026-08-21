import { useEffect, useRef } from "react";

export function SignalFlowField() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const container = canvas?.parentElement;
    const motion = window.matchMedia("(prefers-reduced-motion: no-preference)");
    if (!canvas || !container || !motion.matches) return;

    const context = canvas.getContext("2d");
    if (!context) return;

    const mobile = window.matchMedia("(max-width: 760px)").matches;
    const streamCount = mobile ? 12 : 22;
    const streams = Array.from({ length: streamCount }, (_, index) => ({
      offset: index / Math.max(1, streamCount - 1),
      speed: 0.43 + (index % 5) * 0.06,
      phase: index * 0.71,
      width: index % 5 === 0 ? 1.8 : 0.75,
    }));

    let width = 0;
    let height = 0;
    let frame = 0;
    let time = 0;
    let isVisible = true;
    let lastPaint = 0;
    const pointer = { x: 0.64, y: 0.52, targetX: 0.64, targetY: 0.52 };

    const resize = () => {
      const rect = container.getBoundingClientRect();
      const ratio = mobile ? 1 : Math.min(window.devicePixelRatio || 1, 1.5);
      width = rect.width;
      height = rect.height;
      canvas.width = Math.round(width * ratio);
      canvas.height = Math.round(height * ratio);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      context.setTransform(ratio, 0, 0, ratio, 0, 0);
    };

    const onPointerMove = (event: PointerEvent) => {
      const rect = container.getBoundingClientRect();
      pointer.targetX = Math.max(0.5, Math.min(0.76, (event.clientX - rect.left) / rect.width));
      pointer.targetY = Math.max(0.3, Math.min(0.72, (event.clientY - rect.top) / rect.height));
    };

    const paint = (timestamp: number) => {
      const frameInterval = mobile ? 1000 / 30 : 0;
      if (isVisible && timestamp - lastPaint >= frameInterval) {
        lastPaint = timestamp;
        time += mobile ? 0.018 : 0.012;
        pointer.x += (pointer.targetX - pointer.x) * 0.035;
        pointer.y += (pointer.targetY - pointer.y) * 0.035;
        context.clearRect(0, 0, width, height);
        context.globalCompositeOperation = "lighter";

        streams.forEach((stream, index) => {
          const startY = height * (0.1 + stream.offset * 0.8);
          const focusX = width * pointer.x;
          const focusY = height * pointer.y + Math.sin(time * 1.2 + stream.phase) * 12;
          const endY = focusY + (stream.offset - 0.5) * height * 0.22;
          const isCoral = index % 7 === 0;
          const gradient = context.createLinearGradient(0, 0, width, 0);

          gradient.addColorStop(0, "rgba(62, 219, 186, 0)");
          gradient.addColorStop(0.42, isCoral ? "rgba(237, 126, 99, .34)" : "rgba(62, 219, 186, .22)");
          gradient.addColorStop(0.66, isCoral ? "rgba(237, 126, 99, .68)" : "rgba(92, 238, 207, .7)");
          gradient.addColorStop(1, "rgba(62, 219, 186, 0)");

          context.beginPath();
          context.moveTo(-40, startY);
          context.bezierCurveTo(width * 0.24, startY, focusX - width * 0.18, focusY, focusX, focusY);
          context.bezierCurveTo(focusX + width * 0.12, focusY, width * 0.84, endY, width + 40, endY);
          context.strokeStyle = gradient;
          context.lineWidth = stream.width;
          context.stroke();

          const travel = (time * stream.speed + stream.phase) % 1;
          const firstLeg = travel < 0.62;
          const amount = firstLeg ? travel / 0.62 : (travel - 0.62) / 0.38;
          const x = firstLeg ? width * amount * pointer.x : focusX + width * amount * (1 - pointer.x);
          const y = firstLeg
            ? startY + (focusY - startY) * Math.pow(amount, 1.65)
            : focusY + (endY - focusY) * Math.pow(amount, 0.72);

          context.beginPath();
          context.arc(x, y, index % 5 === 0 ? 2.6 : 1.4, 0, Math.PI * 2);
          context.fillStyle = isCoral ? "rgba(255, 172, 147, .92)" : "rgba(139, 255, 226, .9)";
          context.fill();
        });

        const halo = context.createRadialGradient(
          width * pointer.x,
          height * pointer.y,
          0,
          width * pointer.x,
          height * pointer.y,
          Math.min(width, height) * 0.25,
        );
        halo.addColorStop(0, "rgba(62, 219, 186, .17)");
        halo.addColorStop(1, "rgba(62, 219, 186, 0)");
        context.fillStyle = halo;
        context.fillRect(0, 0, width, height);
        context.globalCompositeOperation = "source-over";
      }
      frame = requestAnimationFrame(paint);
    };

    const observer = new IntersectionObserver(([entry]) => {
      isVisible = entry.isIntersecting && !document.hidden;
    });
    const onVisibilityChange = () => {
      isVisible = !document.hidden && container.getBoundingClientRect().bottom > 0;
    };

    resize();
    observer.observe(container);
    frame = requestAnimationFrame(paint);
    window.addEventListener("resize", resize, { passive: true });
    container.addEventListener("pointermove", onPointerMove, { passive: true });
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
      window.removeEventListener("resize", resize);
      container.removeEventListener("pointermove", onPointerMove);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, []);

  return <canvas ref={canvasRef} className="landing-flow-field" aria-hidden="true" />;
}
