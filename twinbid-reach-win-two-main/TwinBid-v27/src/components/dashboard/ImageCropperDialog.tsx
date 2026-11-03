import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { Loader2, Minus, Plus, Pause, Play } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import { buildDerivedCreativeFilename } from "@/lib/creativeApi";
import type { MediaCropRect } from "@/lib/animatedMediaCrop";
import {
  inferCropMediaKind,
  resolveCropMediaKind,
  type CropMediaKind,
} from "@/lib/cropMediaKind";

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
  mediaKind?: CropMediaKind;
}

interface Props {
  open: boolean;
  source: Source | null;
  target: CropperTarget | null;
  fileNameHint?: string;
  onSave: (file: File, dataUrl: string, dimensions: { w: number; h: number }) => void | Promise<void>;
  onClose: () => void;
}

interface CroppedMedia { file: File; dataUrl: string; dimensions: { w: number; h: number } }

export function ImageCropperDialog({ open, source, target, fileNameHint, onSave, onClose }: Props) {
  const { t } = useLanguage();
  const [scale, setScale] = useState(1);
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [frameSide, setFrameSide] = useState(240); // used only for square-resizable
  const [saving, setSaving] = useState(false);
  const [progress, setProgress] = useState(0);
  const [cropped, setCropped] = useState<CroppedMedia | null>(null);
  const croppedRef = useRef<CroppedMedia | null>(null);
  const transferredRef = useRef(false);
  const videoRef = useRef<HTMLVideoElement>(null);
  const livePreviewRef = useRef<HTMLCanvasElement>(null);
  const [playing, setPlaying] = useState(false);
  const [playhead, setPlayhead] = useState(0);
  const [stageW, setStageW] = useState(MAX_STAGE_W);
  const stageH = stageW / STAGE_ASPECT;
  const mediaKind = source
    ? inferCropMediaKind(source.dataUrl, fileNameHint, source.mediaKind)
    : "image";

  const discardCropped = useCallback(() => {
    if (croppedRef.current && !transferredRef.current) URL.revokeObjectURL(croppedRef.current.dataUrl);
    croppedRef.current = null;
    transferredRef.current = false;
    setCropped(null);
  }, []);

  useEffect(() => () => {
    if (croppedRef.current && !transferredRef.current) URL.revokeObjectURL(croppedRef.current.dataUrl);
  }, []);

  useEffect(() => {
    if (open) return;
    discardCropped();
    setPlaying(false);
    setPlayhead(0);
  }, [open, discardCropped]);

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

  const minScale = source ? Math.max(frameW / source.naturalWidth, frameH / source.naturalHeight) : 1;
  useEffect(() => {
    if (open && source && scale < minScale) setScale(minScale);
  }, [open, source, scale, minScale]);
  const imageW = source ? source.naturalWidth * scale : 0;
  const imageH = source ? source.naturalHeight * scale : 0;
  const maxOffsetX = Math.max(0, (imageW - frameW) / 2);
  const maxOffsetY = Math.max(0, (imageH - frameH) / 2);
  const safeOffset = {
    x: Math.max(-maxOffsetX, Math.min(maxOffsetX, offset.x)),
    y: Math.max(-maxOffsetY, Math.min(maxOffsetY, offset.y)),
  };
  const selection = useMemo(() => source && target && frameW && frameH ? {
    sx: (imageW - frameW) / (2 * scale) - safeOffset.x / scale,
    sy: (imageH - frameH) / (2 * scale) - safeOffset.y / scale,
    sw: frameW / scale,
    sh: frameH / scale,
    sourceWidth: source.naturalWidth,
    sourceHeight: source.naturalHeight,
    outW: target.mode === "fixed" ? target.w : Math.max(target.minSide ?? 200, Math.round(frameW / scale)),
    outH: target.mode === "fixed" ? target.h : Math.max(target.minSide ?? 200, Math.round(frameH / scale)),
  } : null, [source, target, frameW, frameH, scale, safeOffset.x, safeOffset.y, imageW, imageH]);

  // One video decoder feeds both the draggable source and the small live crop
  // preview. Drawing only 320×180 keeps the editor responsive while playing.
  useEffect(() => {
    if (!open || mediaKind !== "video" || cropped || !selection) return;
    const video = videoRef.current;
    const canvas = livePreviewRef.current;
    const context = canvas?.getContext("2d");
    if (!video || !canvas || !context) return;
    let frame = 0;
    const draw = () => {
      if (video.readyState >= 2) {
        context.drawImage(video, selection.sx, selection.sy, selection.sw, selection.sh, 0, 0, canvas.width, canvas.height);
      }
      if (!video.paused) frame = requestAnimationFrame(draw);
    };
    draw();
    video.addEventListener("play", draw);
    video.addEventListener("seeked", draw);
    video.addEventListener("loadeddata", draw);
    return () => {
      cancelAnimationFrame(frame);
      video.removeEventListener("play", draw);
      video.removeEventListener("seeked", draw);
      video.removeEventListener("loadeddata", draw);
    };
  }, [open, mediaKind, cropped, selection]);

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
      x: Math.max(-maxOffsetX, Math.min(maxOffsetX, dragRef.current.ox + (e.clientX - dragRef.current.x))),
      y: Math.max(-maxOffsetY, Math.min(maxOffsetY, dragRef.current.oy + (e.clientY - dragRef.current.y))),
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
    if (!source || !target || !selection) return;
    setSaving(true);
    setProgress(0);
    try {
      const { sx, sy, sw, sh, outW, outH } = selection;
      if (sw <= 0 || sh <= 0) {
        toast.error(t("create.cropInvalid") || "Invalid crop area");
        setSaving(false);
        return;
      }

      const crop: MediaCropRect = selection;

      const outputMediaKind = await resolveCropMediaKind(
        source.dataUrl,
        fileNameHint,
        source.mediaKind,
      );

      if (outputMediaKind === "gif") {
        const { cropAnimatedGif } = await import("@/lib/animatedMediaCrop");
        const result = await cropAnimatedGif(source.dataUrl, crop, fileNameHint, setProgress);
        croppedRef.current = result;
        setCropped(result);
        setSaving(false);
        return;
      }

      if (outputMediaKind === "video") {
        const { cropMp4Video } = await import("@/lib/animatedMediaCrop");
        videoRef.current?.pause();
        const result = await cropMp4Video(source.dataUrl, crop, fileNameHint, setProgress);
        croppedRef.current = result;
        setCropped(result);
        setSaving(false);
        return;
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
      await onSave(file, URL.createObjectURL(file), { w: outW, h: outH });
      setSaving(false);
    } catch (err) {
      console.error(err);
      const reason = err instanceof Error ? err.message : "";
      if (reason === "gif-too-large") {
        toast.error(t("create.cropGifTooLarge"));
      } else if (reason === "video-too-large") {
        toast.error(t("create.cropVideoTooLarge"));
      } else if (reason === "video-codec-unsupported") {
        toast.error(t("create.cropCodecUnsupported"));
      } else if (reason.includes("video metadata") || reason.includes("video duration")) {
        toast.error(t("create.videoMetadataError"));
      } else {
        toast.error(t("create.cropFailed"));
      }
      setSaving(false);
    }
  }, [source, target, selection, fileNameHint, onSave, t]);

  const applyCropped = async () => {
    if (!cropped) return;
    setSaving(true);
    try {
      await onSave(cropped.file, cropped.dataUrl, cropped.dimensions);
      transferredRef.current = true;
      croppedRef.current = null;
      setCropped(null);
    } catch (error) {
      console.error(error);
      toast.error(t("create.cropFailed"));
    } finally {
      setSaving(false);
    }
  };

  if (!source || !target) return null;

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o && !saving) { discardCropped(); onClose(); } }}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto bg-card border-border">
        <DialogHeader>
          <DialogTitle>{t("create.cropTitle")} — {target.w}×{target.h}{target.mode === "square-resizable" ? "+" : ""}</DialogTitle>
          <DialogDescription className="sr-only">{t("create.cropHintFixed")}</DialogDescription>
        </DialogHeader>

        {cropped ? (
          <div className="mx-auto w-full max-w-[560px] space-y-2">
            {mediaKind === "video" ? (
              <video src={cropped.dataUrl} controls autoPlay playsInline className="aspect-video w-full rounded border border-primary bg-black object-contain" />
            ) : (
              <img src={cropped.dataUrl} alt={t("create.cropResult")} className="mx-auto max-h-[50vh] max-w-full rounded border border-primary object-contain" />
            )}
            <p className="text-center text-sm text-muted-foreground">{t("create.cropResult")} · {cropped.dimensions.w}×{cropped.dimensions.h}</p>
          </div>
        ) : <div
          className="relative mx-auto overflow-hidden rounded border border-border bg-black/40 touch-none select-none"
          style={{ width: stageW, height: stageH }}
          onPointerDown={onImgPointerDown}
          onPointerMove={onImgPointerMove}
          onPointerUp={onImgPointerUp}
          onPointerCancel={onImgPointerUp}
        >
          {/* Media */}
          {mediaKind === "video" ? (
            <video
              ref={videoRef}
              src={source.dataUrl}
              muted
              loop
              autoPlay
              playsInline
              onPlay={() => setPlaying(true)}
              onPause={() => setPlaying(false)}
              onTimeUpdate={event => setPlayhead(event.currentTarget.currentTime)}
              onLoadedMetadata={() => setPlayhead(0)}
              style={{
                position: "absolute",
                left: stageW / 2 + safeOffset.x,
                top: stageH / 2 + safeOffset.y,
                width: source.naturalWidth * scale,
                height: source.naturalHeight * scale,
                transform: "translate(-50%, -50%)",
                maxWidth: "none",
                pointerEvents: "none",
              }}
            />
          ) : (
            <img
              src={source.dataUrl}
              alt=""
              draggable={false}
              style={{
                position: "absolute",
                left: stageW / 2 + safeOffset.x,
                top: stageH / 2 + safeOffset.y,
                width: source.naturalWidth * scale,
                height: source.naturalHeight * scale,
                transform: "translate(-50%, -50%)",
                maxWidth: "none",
                pointerEvents: "none",
              }}
            />
          )}
          {/* Four masks leave the actual output area clear and visible. */}
          <div className="pointer-events-none absolute left-0 right-0 top-0 bg-black/60" style={{ height: (stageH - frameH) / 2 }} />
          <div className="pointer-events-none absolute bottom-0 left-0 right-0 bg-black/60" style={{ height: (stageH - frameH) / 2 }} />
          <div className="pointer-events-none absolute left-0 bg-black/60" style={{ top: (stageH - frameH) / 2, height: frameH, width: (stageW - frameW) / 2 }} />
          <div className="pointer-events-none absolute right-0 bg-black/60" style={{ top: (stageH - frameH) / 2, height: frameH, width: (stageW - frameW) / 2 }} />
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
        </div>}

        {!cropped && mediaKind === "video" && (
          <div className="flex items-center gap-3">
            <Button type="button" variant="outline" size="icon" className="shrink-0" aria-label={playing ? t("create.cropPause") : t("create.cropPlay")}
              onClick={() => { const video = videoRef.current; if (!video) return; if (video.paused) void video.play(); else video.pause(); }}>
              {playing ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
            </Button>
            <input type="range" aria-label={t("create.cropSeek")} min={0} max={videoRef.current?.duration || 1} step={0.1}
              value={playhead} onChange={event => { if (videoRef.current) videoRef.current.currentTime = Number(event.target.value); }}
              className="min-w-0 flex-1 accent-primary" />
            <span className="text-xs text-muted-foreground">{Math.floor(playhead)}s</span>
          </div>
        )}

        {!cropped && (mediaKind === "video" || mediaKind === "gif") && selection && (
          <div className="mx-auto space-y-1 text-center">
            <p className="text-sm text-muted-foreground">{t("create.cropLivePreview")}</p>
            {mediaKind === "video" ? (
              <canvas ref={livePreviewRef} width={320} height={Math.round(320 * selection.outH / selection.outW)}
                className="mx-auto max-h-44 max-w-full rounded border border-primary bg-black" />
            ) : (
              <div className="relative mx-auto max-h-44 max-w-[320px] overflow-hidden rounded border border-primary bg-black"
                style={{ width: 320, height: 320 * selection.outH / selection.outW }}>
                <img src={source.dataUrl} alt="" className="absolute max-w-none"
                  style={{ width: source.naturalWidth * 320 / selection.sw,
                    height: source.naturalHeight * 320 / selection.sw,
                    left: -selection.sx * 320 / selection.sw,
                    top: -selection.sy * 320 / selection.sw }} />
              </div>
            )}
          </div>
        )}

        {!cropped && <p className="text-xs text-muted-foreground text-center">
          {target.mode === "square-resizable" ? t("create.cropHintSquare") : t("create.cropHintFixed")}
        </p>}

        {!cropped && <div className="flex min-w-0 flex-wrap items-center gap-2 px-0 sm:flex-nowrap sm:gap-3 sm:px-2">
          <span className="w-full text-xs text-muted-foreground sm:w-16">{t("create.cropZoom")}</span>
          <Button type="button" variant="outline" size="icon" className="h-8 w-8" onClick={() => setScale(s => Math.max(minScale, +(s * 0.9).toFixed(3)))}>
            <Minus className="h-4 w-4" />
          </Button>
          <Slider
            value={[Math.round(scale * 100)]}
            min={Math.max(1, Math.floor(minScale * 100))}
            max={Math.max(600, Math.ceil(minScale * 200))}
            step={1}
            onValueChange={(v) => setScale(Math.max(minScale, v[0] / 100))}
            className="min-w-[100px] flex-1"
          />
          <Button type="button" variant="outline" size="icon" className="h-8 w-8" onClick={() => setScale(s => Math.min(Math.max(6, minScale * 2), +(s * 1.1).toFixed(3)))}>
            <Plus className="h-4 w-4" />
          </Button>
          <span className="text-xs text-muted-foreground w-12 text-right">{Math.round(scale * 100)}%</span>
        </div>}

        {saving && (mediaKind === "video" || mediaKind === "gif") && (
          <p role="status" className="text-center text-sm text-muted-foreground">{t("create.cropProcessing")} {progress}%</p>
        )}

        <DialogFooter>
          {cropped && <Button type="button" variant="outline" onClick={discardCropped} disabled={saving}>{t("create.cropChangeArea")}</Button>}
          <Button type="button" variant="outline" onClick={() => { discardCropped(); onClose(); }} disabled={saving}>{t("create.cropCancel")}</Button>
          <Button type="button" onClick={cropped ? applyCropped : handleSave} disabled={saving} className="bg-primary hover:bg-primary/90 text-primary-foreground">
            {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {cropped ? t("create.cropSave") : mediaKind === "video" || mediaKind === "gif" ? t("create.cropShowResult") : t("create.cropSave")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
