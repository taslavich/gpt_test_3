import { useEffect, useRef, useState } from "react";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Loader2, ArrowRight } from "lucide-react";
import type { Creative } from "@/contexts/CampaignContext";
import { autoCropImage, computeCoverCrop, isGifDataUrl, isGifFileName } from "@/lib/autoCrop";
import { useLanguage } from "@/contexts/LanguageContext";
import { getTargetDims } from "@/lib/creativeTarget";
import { readVideoMetadata, videoMetadataFromFile } from "@/lib/videoMetadata";
import type { MediaCropRect } from "@/lib/animatedMediaCrop";
import { resolveCropMediaKind } from "@/lib/cropMediaKind";
import { toast } from "sonner";

interface Props {
  open: boolean;
  creatives: Creative[];
  formatKey: string;
  onCancel: () => void;
  onConfirm: (nextCreatives: Creative[]) => void | Promise<void>;
}

interface PreviewEntry {
  creativeId: string;
  label: string;
  beforeUrl: string;
  afterUrl?: string;
  file?: File;
  dimensions?: { w: number; h: number };
  mediaKind: "image" | "gif" | "video";
  crop?: MediaCropRect;
  error?: string;
}

export function AutoCropConfirmDialog({ open, creatives, formatKey, onCancel, onConfirm }: Props) {
  const { t } = useLanguage();
  const [previews, setPreviews] = useState<PreviewEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [progress, setProgress] = useState(0);
  const [processingLabel, setProcessingLabel] = useState("");

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    (async () => {
      const items: PreviewEntry[] = [];
      for (let i = 0; i < creatives.length; i++) {
        const c = creatives[i];
        if (!c.sizeMismatch || !c.imageUrl) continue;
        const target = getTargetDims(formatKey, formatKey === "banner" ? c.bannerSize : undefined);
        if (!target) continue;
        const kind = await resolveCropMediaKind(c.imageUrl, c.imageFileName,
          c.mediaType === "video" || c.imageMimeType === "video/mp4" || formatKey === "video"
            ? "video"
            : c.imageMimeType === "image/gif" || isGifDataUrl(c.imageUrl) || isGifFileName(c.imageFileName)
              ? "gif" : "image");
        const video = kind === "video";
        const gif = kind === "gif";
        const entry: PreviewEntry = {
          creativeId: c.id,
          label: c.name?.trim() || `${t("create.creative")} #${i + 1}`,
          beforeUrl: c.imageUrl,
          mediaKind: video ? "video" : gif ? "gif" : "image",
        };
        if (gif || video) {
          try {
            let width = c.imageWidth || 0;
            let height = c.imageHeight || 0;
            if (!width || !height) {
              if (video) {
                const metadata = await readVideoMetadata(c.imageUrl);
                width = metadata.width;
                height = metadata.height;
              } else {
                const image = new Image();
                image.src = c.imageUrl;
                await image.decode();
                width = image.naturalWidth;
                height = image.naturalHeight;
              }
            }
            entry.crop = computeCoverCrop(width, height, target);
          } catch (error: unknown) {
            entry.error = error instanceof Error ? error.message : "media-load";
          }
        } else {
          try {
            const { dataUrl, file, dimensions } = await autoCropImage(c.imageUrl, target, c.imageFileName);
            entry.afterUrl = dataUrl;
            entry.file = file;
            entry.dimensions = dimensions;
          } catch (error: unknown) {
            entry.error = error instanceof Error ? error.message : "error";
          }
        }
        items.push(entry);
      }
      if (!cancelled) {
        setPreviews(items);
        setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [open, creatives, formatKey, t]);

  const confirmingRef = useRef(false);

  const handleConfirm = async () => {
    setProcessing(true);
    const generatedUrls: string[] = [];
    try {
      const map = new Map(previews.map(p => [p.creativeId, p]));
      const next: Creative[] = [];
      for (const c of creatives) {
        const p = map.get(c.id);
        if (!p) { next.push(c); continue; }
        if (p.error) throw new Error(p.error);
        let result = p.afterUrl && p.file && p.dimensions
          ? { dataUrl: p.afterUrl, file: p.file, dimensions: p.dimensions }
          : undefined;
        if (!result && p.crop) {
          setProcessingLabel(p.label);
          setProgress(0);
          if (p.mediaKind === "video") {
            const { cropMp4Video } = await import("@/lib/animatedMediaCrop");
            result = await cropMp4Video(p.beforeUrl, p.crop, c.imageFileName, setProgress);
          } else if (p.mediaKind === "gif") {
            const { cropAnimatedGif } = await import("@/lib/animatedMediaCrop");
            result = await cropAnimatedGif(p.beforeUrl, p.crop, c.imageFileName, setProgress);
          }
          if (result) generatedUrls.push(result.dataUrl);
        }
        if (!result) throw new Error("crop-preview-missing");
        const metadata = p.mediaKind === "video"
          ? videoMetadataFromFile(result.file, await readVideoMetadata(result.file))
          : undefined;
        next.push({
          ...c, imageUrl: result.dataUrl, pendingFile: result.file, imageFileName: result.file.name,
          imageMimeType: result.file.type, mediaType: p.mediaKind === "video" ? "video" : "image",
          videoMetadata: metadata, imageWidth: result.dimensions.w, imageHeight: result.dimensions.h,
          sizeMismatch: false,
        });
      }
      confirmingRef.current = true;
      await onConfirm(next);
    } catch (error) {
      generatedUrls.forEach(url => URL.revokeObjectURL(url));
      console.error("Automatic crop failed:", error);
      const reason = error instanceof Error ? error.message : "";
      toast.error(t(reason === "video-too-large" ? "create.cropVideoTooLarge"
        : reason === "gif-too-large" ? "create.cropGifTooLarge"
          : reason === "video-codec-unsupported" ? "create.cropCodecUnsupported" : "create.cropFailed"));
    } finally {
      setProcessing(false);
    }
  };

  const hasCroppable = previews.length > 0 && previews.every(p => !p.error && (p.afterUrl || p.crop));

  return (
    <AlertDialog open={open} onOpenChange={(o) => {
      if (!o) {
        if (processing) return;
        if (confirmingRef.current) { confirmingRef.current = false; return; }
        onCancel();
      }
    }}>
      <AlertDialogContent className="bg-card border-border max-w-2xl">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("create.mismatchConfirmTitle")}</AlertDialogTitle>
          <AlertDialogDescription className="text-sm text-muted-foreground">{t("create.autoCropBody")}</AlertDialogDescription>
        </AlertDialogHeader>

        <div className="max-h-[55vh] overflow-y-auto space-y-3 py-2">
          {loading && (
            <div className="flex items-center justify-center py-8 gap-2 text-muted-foreground text-sm">
              <Loader2 className="h-4 w-4 animate-spin" /> {t("create.autoCropPreparing")}
            </div>
          )}
          {!loading && previews.map(p => (
            <div key={p.creativeId} className="rounded-lg border border-border bg-background/40 p-3 sm:p-4">
              <div className="text-xs font-medium text-foreground mb-3 text-center">{p.label}</div>
              {p.error ? (
                <p className="text-xs text-destructive text-center">{t("create.autoCropError")}</p>
              ) : p.mediaKind !== "image" && p.crop ? (
                <div className="flex flex-col items-center justify-center gap-3 min-[460px]:flex-row min-[460px]:gap-4">
                  <div className="flex min-w-0 flex-1 flex-col items-center">
                    <div className="relative w-full max-w-[220px] overflow-hidden rounded border border-border bg-black"
                      style={{ aspectRatio: `${p.crop.sourceWidth}/${p.crop.sourceHeight}` }}>
                      {p.mediaKind === "video"
                        ? <video src={p.beforeUrl} muted loop autoPlay playsInline preload="metadata" className="absolute h-full w-full object-fill" />
                        : <img src={p.beforeUrl} alt="" className="absolute h-full w-full object-fill" />}
                      <div className="pointer-events-none absolute border-2 border-primary"
                        style={{ left: `${100 * p.crop.sx / p.crop.sourceWidth}%`, top: `${100 * p.crop.sy / p.crop.sourceHeight}%`,
                          width: `${100 * p.crop.sw / p.crop.sourceWidth}%`, height: `${100 * p.crop.sh / p.crop.sourceHeight}%`,
                          boxShadow: "0 0 0 999px rgba(0,0,0,0.55)" }} />
                    </div>
                    <div className="mt-2 text-xs text-muted-foreground">{t("create.autoCropBefore")}</div>
                  </div>
                  <ArrowRight className="h-5 w-5 shrink-0 rotate-90 text-muted-foreground min-[460px]:rotate-0" />
                  <div className="flex min-w-0 flex-1 flex-col items-center">
                    <div className="relative w-full max-w-[220px] overflow-hidden rounded border border-primary bg-black"
                      style={{ aspectRatio: `${p.crop.outW}/${p.crop.outH}` }}>
                      {p.mediaKind === "video"
                        ? <video src={p.beforeUrl} controls muted playsInline preload="metadata" className="absolute h-full w-full object-cover object-center" />
                        : <img src={p.beforeUrl} alt="" className="absolute h-full w-full object-cover object-center" />}
                    </div>
                    <div className="mt-2 text-xs text-primary">{t("create.autoCropAfter")} · {p.crop.outW}×{p.crop.outH}</div>
                  </div>
                </div>
              ) : (
                <div className="flex flex-col items-center justify-center gap-3 min-[460px]:flex-row min-[460px]:gap-4">
                  <div className="flex flex-col items-center flex-1 min-w-0">
                    <div className="w-full aspect-square max-w-[220px] flex items-center justify-center rounded border border-border bg-black/30 overflow-hidden">
                      <img src={p.beforeUrl} alt="" className="max-h-full max-w-full object-contain" />
                    </div>
                    <div className="text-[11px] text-muted-foreground mt-2">{t("create.autoCropBefore")}</div>
                  </div>
                  <ArrowRight className="h-5 w-5 shrink-0 rotate-90 text-muted-foreground min-[460px]:rotate-0" />
                  <div className="flex flex-col items-center flex-1 min-w-0">
                    <div className="w-full aspect-square max-w-[220px] flex items-center justify-center rounded border border-primary/60 bg-black/30 overflow-hidden">
                      {p.afterUrl && <img src={p.afterUrl} alt="" className="max-h-full max-w-full object-contain" />}
                    </div>
                    <div className="text-[11px] text-primary mt-2 text-center">
                      {t("create.autoCropAfter")} {p.dimensions && `· ${p.dimensions.w}×${p.dimensions.h}`}
                    </div>
                  </div>
                </div>
              )}
            </div>
          ))}
          {processing && <p role="status" className="text-center text-sm text-muted-foreground">
            <Loader2 className="mr-2 inline h-4 w-4 animate-spin" />{t("create.cropProcessing")} {processingLabel} · {progress}%
          </p>}
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel disabled={processing}>{t("create.mismatchGoEdit")}</AlertDialogCancel>
          <AlertDialogAction disabled={loading || processing || !hasCroppable} onClick={event => {
            event.preventDefault();
            void handleConfirm();
          }}>
            {t("create.autoCropConfirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
