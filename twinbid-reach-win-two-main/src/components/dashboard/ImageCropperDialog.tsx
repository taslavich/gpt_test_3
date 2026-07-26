import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { Minus, Plus } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import { buildDerivedCreativeFilename } from "@/lib/creativeApi";

const MAX_BYTES = 1 * 1024 * 1024;
const MAX_STAGE_W = 560;
const STAGE_ASPECT = 4 / 3;

export interface CropperTarget {
  w: number;
  h: number;
  mode: "fixed" | "square-resizable";
  /** Min side (px) for square-resizable output. */
  minSide?: number;
}

interface Source {
  dataUrl: string;
  naturalWidth: number;
  naturalHeight: number;
}

interface Props {
  open: boolean;
  source: Source | null;
  target: CropperTarget | null;
  fileNameHint?: string;
  onSave: (file: File, dataUrl: string, dimensions: { w: number; h: number }) => void;
  onClose: () => void;
}

export function ImageCropperDialog({ open, source, target, fileNameHint, onSave, onClose }: Props) {
  const { t } = useLanguage();
  const [scale, setScale] = useState(1);
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [frameSide, setFrameSide] = useState(240); // used only for square-resizable
  const [saving, setSaving] = useState(false);
  const [stageW, setStageW] = useState(MAX_STAGE_W);
  const stageH = stageW / STAGE_ASPECT;

  useEffect(() => {
    if (!open) return;
    const updateStageSize = () => {
      const available = Math.max(200, window.innerWidth - 48);
      setStageW(Math.min(MAX_STAGE_W, available));
    };
    updateStageSize();
    window.addEventListener("resize", updateStageSize);
    return () => window.removeEventListener("resize", updateStageSize);
  }, [open]);

  const targetAspect = target ? target.w / target.h : 1;

  // Frame display size on stage
  const { frameW, frameH } = useMemo(() => {
    if (!target) return { frameW: 0, frameH: 0 };
    if (target.mode === "square-resizable") return { frameW: frameSide, frameH: frameSide };
    // Fit target aspect into 80% of stage
    const maxW = stageW * 0.85;
    const maxH = stageH * 0.85;
    let w = maxW;
    let h = w / targetAspect;
    if (h > maxH) { h = maxH; w = h * targetAspect; }
    return { frameW: w, frameH: h };
  }, [target, targetAspect, frameSide, stageW, stageH]);

  // Initial fit when source changes
  useEffect(() => {
    if (!open || !source || !target) return;
    const initScale = Math.max(frameW / source.naturalWidth, frameH / source.naturalHeight);
    setScale(initScale);
    setOffset({ x: 0, y: 0 });
    if (target.mode === "square-resizable") {
      setFrameSide(Math.min(stageW, stageH) * 0.6);
    }
  }, [open, source, target, stageW, stageH]); // eslint-disable-line react-hooks/exhaustive-deps

  // Drag image
  const dragRef = useRef<{ x: number; y: number; ox: number; oy: number } | null>(null);
  const onImgPointerDown = (e: React.PointerEvent) => {
    (e.target as Element).setPointerCapture(e.pointerId);
    dragRef.current = { x: e.clientX, y: e.clientY, ox: offset.x, oy: offset.y };
  };
  const onImgPointerMove = (e: React.PointerEvent) => {
    if (!dragRef.current) return;
    setOffset({
      x: dragRef.current.ox + (e.clientX - dragRef.current.x),
      y: dragRef.current.oy + (e.clientY - dragRef.current.y),
    });
  };
  const onImgPointerUp = () => { dragRef.current = null; };

  // Resize frame (square-resizable)
  const resizeRef = useRef<{ x: number; y: number; side: number } | null>(null);
  const onResizePointerDown = (e: React.PointerEvent) => {
    e.stopPropagation();
    (e.target as Element).setPointerCapture(e.pointerId);
    resizeRef.current = { x: e.clientX, y: e.clientY, side: frameSide };
  };
  const onResizePointerMove = (e: React.PointerEvent) => {
    if (!resizeRef.current) return;
    const delta = Math.max(e.clientX - resizeRef.current.x, e.clientY - resizeRef.current.y);
    const next = Math.min(
      Math.min(stageW, stageH) - 20,
      Math.max(80, resizeRef.current.side + delta * 2),
    );
    setFrameSide(next);
  };
  const onResizePointerUp = () => { resizeRef.current = null; };

  const handleSave = useCallback(async () => {
    if (!source || !target) return;
    setSaving(true);
    try {
      // Frame center in stage = (stageW/2, stageH/2). Frame top-left:
      const frameLeft = stageW / 2 - frameW / 2;
      const frameTop = stageH / 2 - frameH / 2;
      // Image top-left in stage:
      const imgW = source.naturalWidth * scale;
      const imgH = source.naturalHeight * scale;
      const imgLeft = stageW / 2 + offset.x - imgW / 2;
      const imgTop = stageH / 2 + offset.y - imgH / 2;
      // Crop rect relative to image, then convert to natural pixels.
      const sx = Math.max(0, (frameLeft - imgLeft) / scale);
      const sy = Math.max(0, (frameTop - imgTop) / scale);
      const sw = Math.min(source.naturalWidth - sx, frameW / scale);
      const sh = Math.min(source.naturalHeight - sy, frameH / scale);
      if (sw <= 0 || sh <= 0) {
        toast.error(t("create.cropInvalid") || "Invalid crop area");
        setSaving(false);
        return;
      }

      // Output dims
      let outW: number, outH: number;
      if (target.mode === "fixed") {
        outW = target.w; outH = target.h;
      } else {
        const side = Math.max(target.minSide ?? 200, Math.round(Math.min(sw, sh)));
        outW = side; outH = side;
      }

      const img = new Image();
      img.crossOrigin = "anonymous";
      img.src = source.dataUrl;
      await new Promise<void>((res, rej) => { img.onload = () => res(); img.onerror = () => rej(new Error("img load")); });

      const canvas = document.createElement("canvas");
      canvas.width = outW; canvas.height = outH;
      const ctx = canvas.getContext("2d");
      if (!ctx) throw new Error("canvas");
      ctx.imageSmoothingEnabled = true;
      ctx.imageSmoothingQuality = "high";
      ctx.drawImage(img, sx, sy, sw, sh, 0, 0, outW, outH);

      const toBlob = (type: string, q?: number) =>
        new Promise<Blob | null>(res => canvas.toBlob(res, type, q));
      let blob = await toBlob("image/png");
      let ext = "png"; let mime = "image/png";
      if (!blob || blob.size > MAX_BYTES) {
        blob = await toBlob("image/jpeg", 0.9);
        ext = "jpg"; mime = "image/jpeg";
      }
      if (!blob) throw new Error("blob");
      if (blob.size > MAX_BYTES) {
        toast.error(t("create.cropTooLarge"));
        setSaving(false);
        return;
      }
      const file = new File(
        [blob],
        buildDerivedCreativeFilename(fileNameHint, "cropped", ext),
        { type: mime },
      );
      const reader = new FileReader();
      reader.onload = () => {
        onSave(file, String(reader.result), { w: outW, h: outH });
        setSaving(false);
      };
      reader.onerror = () => { toast.error("Failed to read output"); setSaving(false); };
      reader.readAsDataURL(file);
    } catch (err) {
      console.error(err);
      toast.error("Failed to crop image");
      setSaving(false);
    }
  }, [source, target, scale, offset, frameW, frameH, stageW, stageH, fileNameHint, onSave, t]);

  if (!source || !target) return null;

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="max-w-2xl bg-card border-border">
        <DialogHeader>
          <DialogTitle>{t("create.cropTitle")} — {target.w}×{target.h}{target.mode === "square-resizable" ? "+" : ""}</DialogTitle>
        </DialogHeader>

        <div
          className="relative mx-auto overflow-hidden rounded border border-border bg-black/40 touch-none select-none"
          style={{ width: stageW, height: stageH }}
          onPointerDown={onImgPointerDown}
          onPointerMove={onImgPointerMove}
          onPointerUp={onImgPointerUp}
          onPointerCancel={onImgPointerUp}
        >
          {/* Image */}
          <img
            src={source.dataUrl}
            alt=""
            draggable={false}
            style={{
              position: "absolute",
              left: stageW / 2 + offset.x,
              top: stageH / 2 + offset.y,
              width: source.naturalWidth * scale,
              height: source.naturalHeight * scale,
              transform: "translate(-50%, -50%)",
              maxWidth: "none",
              pointerEvents: "none",
            }}
          />
          {/* Dark overlay with cutout via 4 rects */}
          <div className="pointer-events-none absolute inset-0" style={{ boxShadow: `0 0 0 9999px rgba(0,0,0,0.55) inset`, clipPath: `polygon(0 0, 100% 0, 100% 100%, 0 100%, 0 ${(stageH/2 - frameH/2)}px, ${(stageW/2 - frameW/2)}px ${(stageH/2 - frameH/2)}px, ${(stageW/2 - frameW/2)}px ${(stageH/2 + frameH/2)}px, ${(stageW/2 + frameW/2)}px ${(stageH/2 + frameH/2)}px, ${(stageW/2 + frameW/2)}px ${(stageH/2 - frameH/2)}px, 0 ${(stageH/2 - frameH/2)}px)` }} />
          {/* Frame */}
          <div
            className="pointer-events-none absolute border-2 border-primary"
            style={{
              left: stageW / 2 - frameW / 2,
              top: stageH / 2 - frameH / 2,
              width: frameW,
              height: frameH,
            }}
          />
          {/* Resize handle for square-resizable */}
          {target.mode === "square-resizable" && (
            <div
              className="absolute bg-primary rounded-sm cursor-nwse-resize"
              style={{
                left: stageW / 2 + frameW / 2 - 8,
                top: stageH / 2 + frameH / 2 - 8,
                width: 16,
                height: 16,
              }}
              onPointerDown={onResizePointerDown}
              onPointerMove={onResizePointerMove}
              onPointerUp={onResizePointerUp}
              onPointerCancel={onResizePointerUp}
            />
          )}
        </div>

        <p className="text-xs text-muted-foreground text-center">
          {target.mode === "square-resizable" ? t("create.cropHintSquare") : t("create.cropHintFixed")}
        </p>

        <div className="flex min-w-0 flex-wrap items-center gap-2 px-0 sm:flex-nowrap sm:gap-3 sm:px-2">
          <span className="w-full text-xs text-muted-foreground sm:w-16">{t("create.cropZoom")}</span>
          <Button type="button" variant="outline" size="icon" className="h-8 w-8" onClick={() => setScale(s => Math.max(0.1, +(s * 0.9).toFixed(3)))}>
            <Minus className="h-4 w-4" />
          </Button>
          <Slider
            value={[Math.round(scale * 100)]}
            min={10}
            max={400}
            step={1}
            onValueChange={(v) => setScale(v[0] / 100)}
            className="min-w-[100px] flex-1"
          />
          <Button type="button" variant="outline" size="icon" className="h-8 w-8" onClick={() => setScale(s => Math.min(6, +(s * 1.1).toFixed(3)))}>
            <Plus className="h-4 w-4" />
          </Button>
          <span className="text-xs text-muted-foreground w-12 text-right">{Math.round(scale * 100)}%</span>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} disabled={saving}>{t("create.cropCancel")}</Button>
          <Button type="button" onClick={handleSave} disabled={saving} className="bg-primary hover:bg-primary/90 text-primary-foreground">
            {t("create.cropSave")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
