import { useEffect, useRef, useState } from "react";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Loader2, ArrowRight } from "lucide-react";
import type { Creative } from "@/contexts/CampaignContext";
import { autoCropImage, isGifDataUrl, isGifFileName } from "@/lib/autoCrop";
import type { CropperTarget } from "@/components/dashboard/ImageCropperDialog";
import { useLanguage } from "@/contexts/LanguageContext";

interface Props {
  open: boolean;
  creatives: Creative[];
  target: CropperTarget | null;
  onCancel: () => void;
  /** Called with a new creatives array where mismatched non-GIF images are auto-cropped. */
  onConfirm: (nextCreatives: Creative[]) => void;
}

interface PreviewEntry {
  creativeId: string;
  label: string;
  beforeUrl: string;
  afterUrl?: string;
  file?: File;
  isGif: boolean;
  error?: string;
}

export function AutoCropConfirmDialog({ open, creatives, target, onCancel, onConfirm }: Props) {
  const { t } = useLanguage();
  const [previews, setPreviews] = useState<PreviewEntry[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open || !target) return;
    let cancelled = false;
    setLoading(true);
    (async () => {
      const items: PreviewEntry[] = [];
      for (let i = 0; i < creatives.length; i++) {
        const c = creatives[i];
        if (!c.sizeMismatch || !c.imageUrl) continue;
        const gif = isGifDataUrl(c.imageUrl) || isGifFileName(c.imageFileName);
        const entry: PreviewEntry = {
          creativeId: c.id,
          label: c.name?.trim() || `${t("create.creative")} #${i + 1}`,
          beforeUrl: c.imageUrl,
          isGif: gif,
        };
        if (!gif) {
          try {
            const { dataUrl, file } = await autoCropImage(c.imageUrl, target, c.imageFileName);
            entry.afterUrl = dataUrl;
            entry.file = file;
          } catch (e: any) {
            entry.error = e?.message || "error";
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
  }, [open, creatives, target, t]);

  const confirmingRef = useRef(false);

  const handleConfirm = () => {
    confirmingRef.current = true;
    const map = new Map(previews.map(p => [p.creativeId, p]));
    const next = creatives.map(c => {
      const p = map.get(c.id);
      if (!p || !p.afterUrl || !p.file) return c;
      return { ...c, imageUrl: p.afterUrl, pendingFile: p.file, imageFileName: p.file.name, sizeMismatch: false };
    });
    onConfirm(next);
  };

  const hasCroppable = previews.some(p => p.afterUrl);
  const hasGif = previews.some(p => p.isGif);

  return (
    <AlertDialog open={open} onOpenChange={(o) => {
      if (!o) {
        if (confirmingRef.current) { confirmingRef.current = false; return; }
        onCancel();
      }
    }}>
      <AlertDialogContent className="bg-card border-border max-w-2xl">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("create.mismatchConfirmTitle")}</AlertDialogTitle>
          <p className="text-sm text-muted-foreground">{t("create.autoCropBody")}</p>
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
              {p.isGif ? (
                <div className="flex flex-col items-center gap-2">
                  <img src={p.beforeUrl} alt="" className="max-h-64 max-w-full rounded border border-border object-contain bg-black/20" />
                  <p className="text-xs text-yellow-500 text-center">{t("create.autoCropGifSkip")}</p>
                </div>
              ) : p.error ? (
                <p className="text-xs text-destructive text-center">{t("create.autoCropError")}</p>
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
                      {t("create.autoCropAfter")} {target && `· ${target.w}×${target.h}${target.mode === "square-resizable" ? "+" : ""}`}
                    </div>
                  </div>
                </div>
              )}
            </div>
          ))}
          {!loading && hasGif && (
            <p className="text-xs text-yellow-500">{t("create.autoCropGifNote")}</p>
          )}
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel>{t("create.mismatchGoEdit")}</AlertDialogCancel>
          <AlertDialogAction disabled={loading || !hasCroppable || hasGif} onClick={handleConfirm}>
            {t("create.autoCropConfirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
