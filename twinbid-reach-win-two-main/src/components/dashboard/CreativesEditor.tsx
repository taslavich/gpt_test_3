import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Upload, Plus, Trash2, Loader2, Pencil, AlertTriangle, Eye, Info } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import type { Creative, CreativeType } from "@/contexts/CampaignContext";
import { ImageCropperDialog } from "@/components/dashboard/ImageCropperDialog";
import { CreativePreviewDialog } from "@/components/dashboard/CreativePreviewDialog";
import {
  extractIframeSrc,
  hasInsecureHttpReference,
  isInsecureHttpUrl,
  isValidCreativeUrl,
  sanitizeCreativeFilename,
  validateCreativeFile,
} from "@/lib/creativeApi";

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

function loadImageDims(dataUrl: string): Promise<{ w: number; h: number }> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve({ w: img.naturalWidth, h: img.naturalHeight });
    img.onerror = () => reject(new Error("Failed to load image"));
    img.src = dataUrl;
  });
}

function loadVideoDims(dataUrl: string): Promise<{ w: number; h: number }> {
  return new Promise((resolve, reject) => {
    const video = document.createElement("video");
    video.preload = "metadata";
    video.onloadedmetadata = () => {
      resolve({ w: video.videoWidth, h: video.videoHeight });
      video.src = "";
    };
    video.onerror = () => reject(new Error("Failed to load video"));
    video.src = dataUrl;
  });
}

const URL_MACROS = [
  "click_id", "site_id", "country_code", "creative_id",
  "campaign_id", "browser", "device", "device_os", "ip_address",
] as const;

interface CreativesEditorProps {
  formatKey: string;
  bannerSize?: string;
  brandName?: string;
  creatives: Creative[];
  onChange: (creatives: Creative[]) => void;
  errors?: Record<string, string>;
  onClearError?: (...keys: string[]) => void;
}

export interface CreativesEditorHandle {
  /** Open the crop editor for a given creative (or first mismatched if omitted). */
  openCropperFor: (creativeId?: string) => Promise<void>;
}

const generateId = () => String(Date.now()) + Math.random().toString(36).slice(2, 6);

const MAX_CREATIVES = 10;

import { getTargetDims } from "@/lib/creativeTarget";

/**
 * Hidden iframe used to measure the intrinsic content size of user-provided
 * HTML or an external iframe URL. Reports measured size + whether measurement
 * was blocked by cross-origin restrictions.
 */
function HiddenSizeProbe({
  html, url, targetW, targetH, onMeasured,
}: {
  html?: string; url?: string; targetW: number; targetH: number;
  onMeasured: (result: { w: number; h: number; crossOrigin: boolean }) => void;
}) {
  const ref = useRef<HTMLIFrameElement | null>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    let done = false;
    const measure = () => {
      if (done) return;
      done = true;
      try {
        const doc = el.contentDocument;
        if (!doc) throw new Error("no-doc");
        const rootW = Math.max(
          doc.documentElement?.scrollWidth || 0,
          doc.body?.scrollWidth || 0,
        );
        const rootH = Math.max(
          doc.documentElement?.scrollHeight || 0,
          doc.body?.scrollHeight || 0,
        );
        // If body has no explicit content, fall back to target dims.
        onMeasured({ w: rootW || 0, h: rootH || 0, crossOrigin: false });
      } catch {
        onMeasured({ w: 0, h: 0, crossOrigin: true });
      }
    };
    el.addEventListener("load", measure);
    // Safety timeout for slow / never-loading iframes.
    const timer = window.setTimeout(measure, 3000);
    return () => {
      el.removeEventListener("load", measure);
      window.clearTimeout(timer);
    };
  }, [html, url, onMeasured]);

  const style: React.CSSProperties = {
    position: "absolute", left: -99999, top: 0,
    width: targetW, height: targetH, border: 0, visibility: "hidden",
    pointerEvents: "none",
  };
  if (html !== undefined) {
    return <iframe ref={ref} title="html-size-probe" srcDoc={html} sandbox="allow-same-origin" style={style} />;
  }
  return <iframe ref={ref} title="iframe-size-probe" src={url} sandbox="allow-same-origin" style={style} />;
}

export const CreativesEditor = forwardRef<CreativesEditorHandle, CreativesEditorProps>(function CreativesEditor(
  { formatKey, bannerSize, brandName, creatives, onChange, errors = {}, onClearError },
  ref,
) {
  const { t } = useLanguage();
  const fileInputRefs = useRef<Record<string, HTMLInputElement | null>>({});
  const htmlFileInputRefs = useRef<Record<string, HTMLInputElement | null>>({});

  const MAX_HTML_BYTES = 1 * 1024 * 1024;
  const handleHtmlFileUpload = async (creativeId: string, e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const name = file.name.toLowerCase();
    if (!name.endsWith(".html") && !name.endsWith(".htm") && file.type && !file.type.includes("html")) {
      toast.error(t("create.htmlFileFormatError"));
      e.target.value = "";
      return;
    }
    if (file.size > MAX_HTML_BYTES) {
      toast.error(t("create.htmlFileSizeError"));
      e.target.value = "";
      return;
    }
    try {
      const text = await file.text();
      updateCreative(creativeId, { htmlCode: text });
      if (text.trim()) {
        onClearError?.(`creative_${creativeId}_html`);
      }
      toast.success(t("create.htmlFileUploaded"));
    } catch (err) {
      console.error("HTML upload error:", err);
      toast.error(t("create.htmlFileFormatError"));
    } finally {
      e.target.value = "";
    }
  };
  const [uploadingId, setUploadingId] = useState<string | null>(null);
  const [previewMedia, setPreviewMedia] = useState<{ url: string; video: boolean } | null>(null);
  // Original source per creative (for re-opening cropper)
  const [origSources, setOrigSources] = useState<Record<string, { dataUrl: string; naturalWidth: number; naturalHeight: number; fileName: string; isGif: boolean; isVideo: boolean }>>({});
  const [cropperCreativeId, setCropperCreativeId] = useState<string | null>(null);
  const [previewCreativeId, setPreviewCreativeId] = useState<string | null>(null);
  // Measured content size per creative for html/iframe (for the human-readable mismatch message).
  const [measured, setMeasured] = useState<Record<string, { w: number; h: number; crossOrigin: boolean }>>({});

  const target = getTargetDims(formatKey, bannerSize);
  const isBanner = formatKey === "banner";

  // Refs to always read the latest props/state from within effects and async
  // handlers — prevents stale-closure writes that could clobber sibling
  // creatives' flags (e.g. flipping sizeMismatch on unrelated valid images).
  const creativesRef = useRef(creatives);
  useEffect(() => { creativesRef.current = creatives; }, [creatives]);
  const origSourcesRef = useRef(origSources);
  useEffect(() => { origSourcesRef.current = origSources; }, [origSources]);

  const mediaSourceSignature = creatives
    .map(c => `${c.id}:${c.imageUrl}:${c.imageFileName}:${c.imageMimeType}:${c.mediaType}`)
    .join("|");

  // Permanent image_url values returned by the API are usable directly.
  // Load their dimensions for the existing editor/crop checks without
  // downloading and converting the asset back into a File.
  useEffect(() => {
    let cancelled = false;
    creativesRef.current.forEach((creative) => {
      if (!creative.imageUrl || origSourcesRef.current[creative.id]) return;
      const video = creative.mediaType === "video"
        || creative.imageMimeType === "video/mp4"
        || /\.mp4$/i.test(creative.imageFileName || "");
      const gif = !video && (
        creative.imageUrl.startsWith("data:image/gif")
        || /\.gif$/i.test(creative.imageFileName || "")
      );
      void (video ? loadVideoDims(creative.imageUrl) : loadImageDims(creative.imageUrl))
        .then(({ w, h }) => {
          if (cancelled) return;
          setOrigSources((previous) => previous[creative.id] ? previous : {
            ...previous,
            [creative.id]: {
              dataUrl: creative.imageUrl!,
              naturalWidth: w,
              naturalHeight: h,
              fileName: creative.imageFileName || (video ? "video.mp4" : "image"),
              isGif: gif,
              isVideo: video,
            },
          });
          if (!creative.imageWidth || !creative.imageHeight) {
            const latest = creativesRef.current;
            const next = latest.map(item => item.id === creative.id
              ? { ...item, imageWidth: w, imageHeight: h }
              : item);
            creativesRef.current = next;
            onChange(next);
          }
        })
        .catch(() => {
          // The media remains displayable even if its intrinsic dimensions
          // cannot be read (for example because the remote server blocks it).
        });
    });
    return () => { cancelled = true; };
  }, [mediaSourceSignature]);

  // Recompute image sizeMismatch ONLY when format/bannerSize changes.
  // Per-creative mismatch on upload is set inside handleImageUpload; we must
  // not re-touch other creatives when an unrelated one is uploaded (otherwise
  // a stale-closure onChange could clobber sibling flags and surface warnings
  // on creatives whose own images are perfectly valid).
  useEffect(() => {
    const list = creativesRef.current;
    if (!list.some(c => c.imageUrl)) return;
    let changed = false;
    const next = list.map(c => {
      if ((c.creativeType || "image") !== "image") return c;
      if (!c.imageUrl) return c;
      const src = origSourcesRef.current[c.id];
      if (!src) return c;
      const mismatch = target && !src.isVideo ? (target.mode === "fixed"
        ? src.naturalWidth !== target.w || src.naturalHeight !== target.h
        : src.naturalWidth !== src.naturalHeight || src.naturalWidth < (target.minSide ?? 200))
        : false;
      if (mismatch !== !!c.sizeMismatch) {
        changed = true;
        return { ...c, sizeMismatch: mismatch };
      }
      return c;
    });
    if (changed) onChange(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [formatKey, bannerSize]);

  const showTitle = formatKey === "native" || formatKey === "push";
  const showDescription = formatKey === "native" || formatKey === "push";
  const showImage = formatKey !== "popunder";

  const updateCreative = (id: string, updates: Partial<Creative>) => {
    // Read from ref so async handlers (image uploads etc.) can't overwrite
    // recent sibling edits with a stale creatives snapshot.
    const list = creativesRef.current;
    onChange(list.map(c => c.id === id ? { ...c, ...updates } : c));
  };

  const addCreative = () => {
    const list = creativesRef.current;
    if (list.length >= MAX_CREATIVES) {
      toast.error(t("create.creativeLimit").replace("{max}", String(MAX_CREATIVES)));
      return;
    }
    onChange([...list, { id: generateId(), url: "", creativeType: isBanner ? "image" : undefined, sizeMismatch: false }]);
  };

  const removeCreative = (id: string) => {
    const list = creativesRef.current;
    if (list.length <= 1) return;
    onChange(list.filter(c => c.id !== id));
    setOrigSources(prev => { const n = { ...prev }; delete n[id]; return n; });
    setMeasured(prev => { const n = { ...prev }; delete n[id]; return n; });
  };


  const checkMismatch = (natW: number, natH: number): boolean => {
    if (!target) return false;
    if (target.mode === "fixed") return natW !== target.w || natH !== target.h;
    return natW !== natH || natW < (target.minSide ?? 200);
  };

  const handleImageUpload = async (creativeId: string, e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const ext = file.name.toLowerCase().slice(file.name.lastIndexOf("."));
    const validation = validateCreativeFile(file, isBanner);
    if (!validation.valid) {
      const messageKey = validation.reason === "video-size"
        ? "create.videoSizeError"
        : validation.reason === "image-size"
          ? "create.imageSizeError"
          : isBanner
            ? "create.bannerMediaFormatError"
            : "create.imageFormatError";
      toast.error(t(messageKey));
      e.target.value = "";
      return;
    }
    const video = validation.mediaType === "video";
    setUploadingId(creativeId);
    try {
      const dataUrl = await readFileAsDataUrl(file);
      const { w, h } = video ? await loadVideoDims(dataUrl) : await loadImageDims(dataUrl);
      const isGif = file.type === "image/gif" || ext === ".gif";
      // MP4 is rendered into the selected banner slot dimensions by ADM.
      // Image creatives retain the existing crop/mismatch workflow.
      const mismatch = video ? false : checkMismatch(w, h);
      setOrigSources(prev => ({
        ...prev,
        [creativeId]: { dataUrl, naturalWidth: w, naturalHeight: h, fileName: file.name, isGif, isVideo: video },
      }));
      updateCreative(creativeId, {
        imageUrl: dataUrl,
        pendingFile: file,
        imageFileName: sanitizeCreativeFilename(file.name),
        imageMimeType: video ? "video/mp4" : file.type,
        mediaType: video ? "video" : "image",
        imageWidth: w,
        imageHeight: h,
        sizeMismatch: mismatch,
      });
      onClearError?.(`creative_${creativeId}_image`);
      toast.success(t(video ? "create.videoUploaded" : "create.imageUploaded"));
    } catch (err) {
      console.error("Image upload error:", err);
      toast.error(t("create.imageFormatError"));
    } finally {
      setUploadingId(null);
      e.target.value = "";
    }
  };

  const appendMacro = (url: string, macro: string) => {
    if (url.includes(`{${macro}}`)) return url;
    const token = `${macro}={${macro}}`;
    const separator = url.includes("?") ? "&" : "?";
    return url + separator + token;
  };

  const toggleMacro = (creativeId: string, macro: string, currentUrl: string) => {
    if (macro === "click_id") {
      updateCreative(creativeId, { url: appendMacro(currentUrl, macro) });
      return;
    }
    if (currentUrl.includes(`{${macro}}`)) {
      let newUrl = currentUrl;
      const regexAmp = new RegExp(`[&?]${macro}=\\{${macro}\\}`, "g");
      newUrl = newUrl.replace(regexAmp, "");
      if (newUrl.includes("&") && !newUrl.includes("?")) {
        newUrl = newUrl.replace("&", "?");
      }
      updateCreative(creativeId, { url: newUrl });
    } else {
      updateCreative(creativeId, { url: appendMacro(currentUrl, macro) });
    }
  };

  const ensureClickId = (creativeId: string, url: string) => {
    if (!url.trim()) return;
    if (!url.includes("{click_id}")) {
      updateCreative(creativeId, { url: appendMacro(url, "click_id") });
    }
  };

  const openCropper = (creativeId: string) => {
    if (!origSources[creativeId]) return;
    setCropperCreativeId(creativeId);
  };

  const ensureSource = async (creative: Creative) => {
    if (origSources[creative.id]) return origSources[creative.id];
    if (!creative.imageUrl) return null;
    const isVideo = creative.mediaType === "video"
      || creative.imageMimeType === "video/mp4"
      || /\.mp4$/i.test(creative.imageFileName || "");
    const isGif = !isVideo && (creative.imageUrl.startsWith("data:image/gif") || /\.gif$/i.test(creative.imageFileName || ""));
    try {
      const { w, h } = isVideo
        ? await loadVideoDims(creative.imageUrl)
        : await loadImageDims(creative.imageUrl);
      const entry = { dataUrl: creative.imageUrl, naturalWidth: w, naturalHeight: h, fileName: creative.imageFileName || "image", isGif, isVideo };
      setOrigSources(prev => ({ ...prev, [creative.id]: entry }));
      return entry;
    } catch {
      return null;
    }
  };

  useImperativeHandle(ref, () => ({
    openCropperFor: async (creativeId?: string) => {
      const targetCreative = creativeId
        ? creatives.find(c => c.id === creativeId)
        : creatives.find(c => c.sizeMismatch && (c.creativeType || "image") === "image") || creatives.find(c => c.imageUrl);
      if (!targetCreative) return;
      if ((targetCreative.creativeType || "image") !== "image") return;
      const src = await ensureSource(targetCreative);
      if (!src || src.isGif || src.isVideo) {
        toast.error(t("create.autoCropGifSkip"));
        return;
      }
      setCropperCreativeId(targetCreative.id);
    },
  }), [creatives, origSources, t]);

  const activeSource = cropperCreativeId ? origSources[cropperCreativeId] : null;

  /** Effective iframe URL used for probe/preview, from either mode. */
  const getEffectiveIframeUrl = (c: Creative): string => {
    const mode = c.iframeMode || "url";
    if (mode === "code") return extractIframeSrc(c.iframeCode || "");
    return (c.iframeUrl || "").trim();
  };

  const setCreativeType = (creativeId: string, type: CreativeType) => {
    const c = creatives.find(x => x.id === creativeId);
    if (!c) return;
    updateCreative(creativeId, {
      creativeType: type,
      // Clear per-type validation flags; recomputed by effects below.
      sizeMismatch: false,
    });
    onClearError?.(
      `creative_${creativeId}_image`,
      `creative_${creativeId}_url`,
      `creative_${creativeId}_html`,
      `creative_${creativeId}_iframe`,
    );
  };

  // Update measured mismatch for html/iframe when target changes or measurements arrive.
  useEffect(() => {
    if (!target || target.mode !== "fixed") return;
    let changed = false;
    const next = creatives.map(c => {
      const type = c.creativeType || "image";
      if (type === "image") return c;
      if (type === "html") {
        const m = measured[c.id];
        if (!c.htmlCode?.trim()) {
          if (c.sizeMismatch) { changed = true; return { ...c, sizeMismatch: false }; }
          return c;
        }
        if (!m) return c;
        const mismatch = m.w !== target.w || m.h !== target.h;
        if (mismatch !== !!c.sizeMismatch) { changed = true; return { ...c, sizeMismatch: mismatch }; }
        return c;
      }
      if (type === "iframe") {
        const eff = getEffectiveIframeUrl(c);
        if (!eff || !isValidCreativeUrl(eff)) {
          if (c.sizeMismatch) { changed = true; return { ...c, sizeMismatch: false }; }
          return c;
        }
        const m = measured[c.id];
        let mismatch = false;
        if (m && !m.crossOrigin) {
          mismatch = m.w !== target.w || m.h !== target.h;
        } else {
          // Cross-origin or not yet measured — require explicit user confirmation.
          mismatch = !c.iframeSizeConfirmed;
        }
        if (mismatch !== !!c.sizeMismatch) { changed = true; return { ...c, sizeMismatch: mismatch }; }
        return c;
      }
      return c;
    });
    if (changed) onChange(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [measured, formatKey, bannerSize, creatives.map(c => `${c.id}:${c.creativeType}:${c.iframeMode}:${c.htmlCode}:${c.iframeUrl}:${c.iframeCode}:${c.iframeSizeConfirmed}`).join("|")]);

  return (
    <>
    <div className="space-y-4">
      {creatives.map((creative, idx) => {
        const activeMacros = new Set(URL_MACROS.filter(m => creative.url.includes(`{${m}}`)));
        const src = origSources[creative.id];
        const type: CreativeType = (isBanner ? (creative.creativeType || "image") : "image");
        const canCrop = type === "image" && !!src && !src.isGif && !src.isVideo && !!target;
        const meas = measured[creative.id];

        return (
          <div key={creative.id} className="min-w-0 space-y-4 rounded-lg border border-border bg-background/30 p-3 sm:p-4">
            <div className="flex items-center justify-between">
              <p className="text-sm font-medium text-muted-foreground">
                {t("create.creative")} #{idx + 1}
              </p>
              {creatives.length > 1 && (
                <Button type="button" variant="ghost" size="icon" onClick={() => removeCreative(creative.id)}
                  className="h-7 w-7 text-destructive hover:text-destructive">
                  <Trash2 className="h-4 w-4" />
                </Button>
              )}
            </div>

            <div className="space-y-2">
              <Label>{t("create.creativeName")} *</Label>
              <Input value={creative.name || ""} onChange={e => { updateCreative(creative.id, { name: e.target.value }); if (e.target.value.trim()) onClearError?.(`creative_${creative.id}_name`); }}
                placeholder={t("create.creativeNamePlaceholder")}
                className={`bg-background border-border ${errors[`creative_${creative.id}_name`] ? "border-destructive" : ""}`} />
              <p className="text-xs text-muted-foreground">{t("create.creativeNameHint")}</p>
              {errors[`creative_${creative.id}_name`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_name`]}</p>}
            </div>

            {isBanner && (
              <div className="space-y-2">
                <Label>{t("create.creativeType")}</Label>
                <Tabs value={type} onValueChange={(v) => setCreativeType(creative.id, v as CreativeType)}>
                  <TabsList className="w-full justify-start overflow-x-auto bg-background border border-border sm:w-auto">
                    <TabsTrigger value="image">{t("create.creativeTypeImage")}</TabsTrigger>
                    <TabsTrigger value="html">{t("create.creativeTypeHtml")}</TabsTrigger>
                    <TabsTrigger value="iframe">{t("create.creativeTypeIframe")}</TabsTrigger>
                  </TabsList>
                </Tabs>
              </div>
            )}

            {showTitle && (
              <div className="space-y-2">
                <Label>{t("create.creativeTitle")} *</Label>
                <Input value={creative.title || ""} onChange={e => { updateCreative(creative.id, { title: e.target.value }); if (e.target.value.trim()) onClearError?.(`creative_${creative.id}_title`); }}
                  placeholder={t("create.titlePlaceholder")}
                  className={`bg-background border-border ${errors[`creative_${creative.id}_title`] ? "border-destructive" : ""}`} />
                {errors[`creative_${creative.id}_title`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_title`]}</p>}
              </div>
            )}

            {showDescription && (
              <div className="space-y-2">
                <Label>{t("create.creativeDescription")} *</Label>
                <Textarea value={creative.description || ""} onChange={e => { updateCreative(creative.id, { description: e.target.value }); if (e.target.value.trim()) onClearError?.(`creative_${creative.id}_description`); }}
                  placeholder={t("create.descriptionPlaceholder")}
                  className={`bg-background border-border resize-none ${errors[`creative_${creative.id}_description`] ? "border-destructive" : ""}`} rows={2} />
                {errors[`creative_${creative.id}_description`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_description`]}</p>}
              </div>
            )}

            {/* URL + macros for banner image type. Non-banner formats render URL in the block below. */}
            {isBanner && type === "image" && (
              <div className="space-y-2">
                <Label>{t("create.creativeUrl")} *</Label>
                <Input value={creative.url} onChange={e => { updateCreative(creative.id, { url: e.target.value }); if (e.target.value.trim()) onClearError?.(`creative_${creative.id}_url`); }}
                  onBlur={e => ensureClickId(creative.id, e.target.value)}
                  placeholder="https://example.com/landing"
                  className={`bg-background border-border ${errors[`creative_${creative.id}_url`] ? "border-destructive" : ""}`} />
                {errors[`creative_${creative.id}_url`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_url`]}</p>}
                {isInsecureHttpUrl(creative.url) && (
                  <div className="flex items-start gap-2 rounded border border-yellow-500/30 bg-yellow-500/10 p-2">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-500" />
                    <p className="text-xs text-yellow-500">{t("create.httpUrlWarning")}</p>
                  </div>
                )}
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">{t("create.urlMacrosHint")}</p>
                  <div className="flex flex-wrap gap-1.5">
                    {URL_MACROS.map(macro => {
                      const isActive = activeMacros.has(macro);
                      const isRequired = macro === "click_id";
                      return (
                        <Badge
                          key={macro}
                          variant="outline"
                          className={`cursor-pointer text-xs font-mono transition-colors ${
                            isRequired
                              ? "bg-primary/20 border-primary/60 text-primary"
                              : isActive
                                ? "bg-primary/15 border-primary/40 text-primary hover:bg-primary/25"
                                : "hover:bg-primary/10 hover:border-primary/30"
                          }`}
                          onClick={() => toggleMacro(creative.id, macro, creative.url)}
                          title={isRequired ? "Required" : undefined}
                        >
                          {`{${macro}}`}{isRequired ? " *" : ""}
                        </Badge>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}

            {/* Image upload for banner image type. Non-banner formats render their image block below. */}
            {showImage && isBanner && type === "image" && (
              <div className="space-y-2">
                <Label>{t("create.uploadBannerMedia")} *</Label>
                <input
                  ref={el => { fileInputRefs.current[creative.id] = el; }}
                  type="file" accept=".png,.jpg,.jpeg,.gif,.mp4,image/png,image/jpeg,image/gif,video/mp4" className="hidden"
                  onChange={e => handleImageUpload(creative.id, e)} />
                <p className="text-xs text-muted-foreground">
                  {t("create.bannerMediaFormatHint")}
                  {target && (target.mode === "fixed"
                    ? ` · ${target.w}×${target.h}px`
                    : ` · ≥ ${target.minSide ?? 200}×${target.minSide ?? 200}px (1:1)`)}
                </p>
                <div className="flex items-center gap-3 flex-wrap">
                  <Button type="button" variant="outline" disabled={uploadingId === creative.id}
                    onClick={() => fileInputRefs.current[creative.id]?.click()} className="border-border gap-2">
                    {uploadingId === creative.id
                      ? <Loader2 className="h-4 w-4 animate-spin" />
                      : <Upload className="h-4 w-4" />}
                    {t("create.uploadBannerMedia")}
                  </Button>
                  {canCrop && (
                    <Button type="button" variant="outline" onClick={() => openCropper(creative.id)} className="border-border gap-2">
                      <Pencil className="h-4 w-4" />
                      {t("create.editImage")}
                    </Button>
                  )}
                  {creative.imageUrl && (
                    <Button type="button" variant="outline" onClick={() => setPreviewCreativeId(creative.id)} className="border-border gap-2">
                      <Eye className="h-4 w-4" />
                      {t("create.previewCreative")}
                    </Button>
                  )}
                  {creative.imageFileName && <span className="min-w-0 break-all text-sm text-muted-foreground">{creative.imageFileName}</span>}
                </div>
                {creative.sizeMismatch && target && (
                  <div className="flex items-start gap-2 p-2 rounded border border-yellow-500/30 bg-yellow-500/10">
                    <AlertTriangle className="h-4 w-4 text-yellow-500 shrink-0 mt-0.5" />
                    <p className="text-xs text-yellow-500">
                      {src?.isGif
                        ? t("create.gifExactSize").replace("{w}", String(target.w)).replace("{h}", String(target.h))
                        : t("create.imageWrongSize").replace("{w}", String(target.w)).replace("{h}", String(target.h))}
                    </p>
                  </div>
                )}
                {creative.imageUrl && (
                  <button type="button" onClick={() => setPreviewMedia({
                    url: creative.imageUrl!,
                    video: creative.mediaType === "video" || creative.imageMimeType === "video/mp4" || /\.mp4$/i.test(creative.imageFileName || ""),
                  })} className="block">
                    {creative.mediaType === "video" || creative.imageMimeType === "video/mp4" || /\.mp4$/i.test(creative.imageFileName || "")
                      ? <video src={creative.imageUrl} muted loop autoPlay playsInline className="mt-2 max-h-32 rounded border border-border cursor-zoom-in hover:opacity-90 transition-opacity" />
                      : <img src={creative.imageUrl} alt="Preview" className="mt-2 max-h-32 rounded border border-border cursor-zoom-in hover:opacity-90 transition-opacity" />}
                  </button>
                )}
                {errors[`creative_${creative.id}_image`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_image`]}</p>}
              </div>
            )}

            {/* HTML code editor */}
            {isBanner && type === "html" && (
              <div className="space-y-2">
                <Label>{t("create.htmlCode")}</Label>
                <Textarea
                  value={creative.htmlCode || ""}
                  onChange={e => {
                    const htmlCode = e.target.value;
                    updateCreative(creative.id, { htmlCode });
                    if (htmlCode.trim()) {
                      onClearError?.(`creative_${creative.id}_html`);
                    }
                  }}
                  placeholder={t("create.htmlCodePlaceholder")}
                  rows={10}
                  className={`bg-background border-border font-mono text-xs ${errors[`creative_${creative.id}_html`] ? "border-destructive" : ""}`}
                />
                {target && target.mode === "fixed" && (
                  <p className="text-xs text-muted-foreground">
                    {t("create.htmlCodeHint").replace("{w}", String(target.w)).replace("{h}", String(target.h))}
                  </p>
                )}
                <input
                  ref={el => { htmlFileInputRefs.current[creative.id] = el; }}
                  type="file" accept=".html,.htm,text/html" className="hidden"
                  onChange={e => handleHtmlFileUpload(creative.id, e)}
                />
                <div className="flex items-center gap-3 flex-wrap">
                  <Button type="button" variant="outline"
                    onClick={() => htmlFileInputRefs.current[creative.id]?.click()}
                    className="border-border gap-2">
                    <Upload className="h-4 w-4" />
                    {t("create.uploadHtmlFile")}
                  </Button>
                  {creative.htmlCode?.trim() && (
                    <Button type="button" variant="outline" onClick={() => setPreviewCreativeId(creative.id)} className="border-border gap-2">
                      <Eye className="h-4 w-4" />
                      {t("create.previewCreative")}
                    </Button>
                  )}
                </div>
                {hasInsecureHttpReference(creative.htmlCode) && (
                  <div className="flex items-start gap-2 rounded border border-yellow-500/30 bg-yellow-500/10 p-2">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-500" />
                    <p className="text-xs text-yellow-500">{t("create.httpUrlWarning")}</p>
                  </div>
                )}
                {creative.sizeMismatch && target && target.mode === "fixed" && meas && (
                  <div className="flex items-start gap-2 p-2 rounded border border-yellow-500/30 bg-yellow-500/10">
                    <AlertTriangle className="h-4 w-4 text-yellow-500 shrink-0 mt-0.5" />
                    <p className="text-xs text-yellow-500">
                      {t("create.htmlSizeMismatch")
                        .replace("{actualW}", String(meas.w))
                        .replace("{actualH}", String(meas.h))
                        .replace("{w}", String(target.w))
                        .replace("{h}", String(target.h))}
                    </p>
                  </div>
                )}
                {errors[`creative_${creative.id}_html`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_html`]}</p>}
                {target && target.mode === "fixed" && creative.htmlCode?.trim() && (
                  <HiddenSizeProbe
                    html={creative.htmlCode}
                    targetW={target.w}
                    targetH={target.h}
                    onMeasured={(res) => setMeasured(prev => ({ ...prev, [creative.id]: res }))}
                  />
                )}
              </div>
            )}

            {/* iframe (URL or code snippet) */}
            {isBanner && type === "iframe" && (() => {
              const iframeMode = creative.iframeMode || "url";
              const effUrl = getEffectiveIframeUrl(creative);
              const hasValidEff = effUrl && isValidCreativeUrl(effUrl);
              return (
              <div className="space-y-2">
                <div className="flex items-start gap-2 rounded border border-border bg-muted/40 p-2">
                  <Info className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                  <p className="text-xs text-muted-foreground">{t("create.iframeExplanation")}</p>
                </div>
                <Tabs
                  value={iframeMode}
                  onValueChange={(v) => {
                    updateCreative(creative.id, { iframeMode: v as "url" | "code", iframeSizeConfirmed: false });
                    onClearError?.(`creative_${creative.id}_iframe`);
                    setMeasured(prev => { const n = { ...prev }; delete n[creative.id]; return n; });
                  }}
                >
                  <TabsList className="w-full justify-start overflow-x-auto bg-background border border-border sm:w-auto">
                    <TabsTrigger value="url">{t("create.iframeModeUrl")}</TabsTrigger>
                    <TabsTrigger value="code">{t("create.iframeModeCode")}</TabsTrigger>
                  </TabsList>
                </Tabs>

                {iframeMode === "url" ? (
                  <>
                    <Label>{t("create.iframeUrl")}</Label>
                    <Input
                      value={creative.iframeUrl || ""}
                      onChange={e => {
                        updateCreative(creative.id, { iframeUrl: e.target.value, iframeSizeConfirmed: false });
                        if (isValidCreativeUrl(e.target.value)) {
                          onClearError?.(`creative_${creative.id}_iframe`);
                        }
                        setMeasured(prev => { const n = { ...prev }; delete n[creative.id]; return n; });
                      }}
                      placeholder={t("create.iframeUrlPlaceholder")}
                      className={`bg-background border-border ${errors[`creative_${creative.id}_iframe`] ? "border-destructive" : ""}`}
                    />
                  </>
                ) : (
                  <>
                    <Label>{t("create.iframeCode")}</Label>
                    <Textarea
                      value={creative.iframeCode || ""}
                      onChange={e => {
                        updateCreative(creative.id, { iframeCode: e.target.value, iframeSizeConfirmed: false });
                        if (isValidCreativeUrl(extractIframeSrc(e.target.value))) {
                          onClearError?.(`creative_${creative.id}_iframe`);
                        }
                        setMeasured(prev => { const n = { ...prev }; delete n[creative.id]; return n; });
                      }}
                      placeholder={t("create.iframeCodePlaceholder")}
                      rows={5}
                      className={`bg-background border-border font-mono text-xs ${errors[`creative_${creative.id}_iframe`] ? "border-destructive" : ""}`}
                    />
                    {creative.iframeCode?.trim() && !extractIframeSrc(creative.iframeCode) && (
                      <p className="text-xs text-destructive">{t("create.iframeCodeNoSrc")}</p>
                    )}
                  </>
                )}

                {target && target.mode === "fixed" && (
                  <p className="text-xs text-muted-foreground">
                    {t("create.iframeUrlHint").replace("{w}", String(target.w)).replace("{h}", String(target.h))}
                  </p>
                )}
                {hasValidEff && (
                  <>
                    <div className="flex items-center gap-3 flex-wrap">
                      <Button type="button" variant="outline" onClick={() => setPreviewCreativeId(creative.id)} className="border-border gap-2">
                        <Eye className="h-4 w-4" />
                        {t("create.previewCreative")}
                      </Button>
                    </div>
                    {target && target.mode === "fixed" && (
                      <HiddenSizeProbe
                        url={effUrl}
                        targetW={target.w}
                        targetH={target.h}
                        onMeasured={(res) => setMeasured(prev => ({ ...prev, [creative.id]: res }))}
                      />
                    )}
                    {target && target.mode === "fixed" && meas?.crossOrigin && (
                      <>
                        <p className="text-xs text-muted-foreground">{t("create.iframeSizeUnknown")}</p>
                        <label className="flex items-center gap-2 text-xs cursor-pointer">
                          <Checkbox
                            checked={!!creative.iframeSizeConfirmed}
                            onCheckedChange={(v) => updateCreative(creative.id, { iframeSizeConfirmed: !!v })}
                          />
                          <span>
                            {t("create.iframeSizeConfirm").replace("{w}", String(target.w)).replace("{h}", String(target.h))}
                          </span>
                        </label>
                      </>
                    )}
                    {creative.sizeMismatch && target && target.mode === "fixed" && meas && !meas.crossOrigin && (
                      <div className="flex items-start gap-2 p-2 rounded border border-yellow-500/30 bg-yellow-500/10">
                        <AlertTriangle className="h-4 w-4 text-yellow-500 shrink-0 mt-0.5" />
                        <p className="text-xs text-yellow-500">
                          {t("create.iframeSizeMismatch")
                            .replace("{actualW}", String(meas.w))
                            .replace("{actualH}", String(meas.h))
                            .replace("{w}", String(target.w))
                            .replace("{h}", String(target.h))}
                        </p>
                      </div>
                    )}
                  </>
                )}
                {iframeMode === "url" && creative.iframeUrl && !isValidCreativeUrl(creative.iframeUrl) && (
                  <p className="text-xs text-destructive">{t("create.iframeUrlInvalid")}</p>
                )}
                {iframeMode === "code" && effUrl && !isValidCreativeUrl(effUrl) && (
                  <p className="text-xs text-destructive">{t("create.iframeUrlInvalid")}</p>
                )}
                {isInsecureHttpUrl(effUrl) && (
                  <div className="flex items-start gap-2 rounded border border-yellow-500/30 bg-yellow-500/10 p-2">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-500" />
                    <p className="text-xs text-yellow-500">{t("create.httpUrlWarning")}</p>
                  </div>
                )}
                {errors[`creative_${creative.id}_iframe`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_iframe`]}</p>}
              </div>
              );
            })()}


            {/* URL + macros for non-banner formats (push/native), or popunder URL */}
            {!isBanner && (
              <div className="space-y-2">
                <Label>{t("create.creativeUrl")} *</Label>
                <Input value={creative.url} onChange={e => { updateCreative(creative.id, { url: e.target.value }); if (e.target.value.trim()) onClearError?.(`creative_${creative.id}_url`); }}
                  onBlur={e => ensureClickId(creative.id, e.target.value)}
                  placeholder="https://example.com/landing"
                  className={`bg-background border-border ${errors[`creative_${creative.id}_url`] ? "border-destructive" : ""}`} />
                {errors[`creative_${creative.id}_url`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_url`]}</p>}
                {isInsecureHttpUrl(creative.url) && (
                  <div className="flex items-start gap-2 rounded border border-yellow-500/30 bg-yellow-500/10 p-2">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-500" />
                    <p className="text-xs text-yellow-500">{t("create.httpUrlWarning")}</p>
                  </div>
                )}
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">{t("create.urlMacrosHint")}</p>
                  <div className="flex flex-wrap gap-1.5">
                    {URL_MACROS.map(macro => {
                      const isActive = activeMacros.has(macro);
                      const isRequired = macro === "click_id";
                      return (
                        <Badge
                          key={macro}
                          variant="outline"
                          className={`cursor-pointer text-xs font-mono transition-colors ${
                            isRequired
                              ? "bg-primary/20 border-primary/60 text-primary"
                              : isActive
                                ? "bg-primary/15 border-primary/40 text-primary hover:bg-primary/25"
                                : "hover:bg-primary/10 hover:border-primary/30"
                          }`}
                          onClick={() => toggleMacro(creative.id, macro, creative.url)}
                          title={isRequired ? "Required" : undefined}
                        >
                          {`{${macro}}`}{isRequired ? " *" : ""}
                        </Badge>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}

            {/* Image field for push/native (kept below URL like before) */}
            {showImage && !isBanner && (
              <div className="space-y-2">
                <Label>{t("create.uploadImage")} *</Label>
                <input
                  ref={el => { fileInputRefs.current[creative.id] = el; }}
                  type="file" accept=".png,.jpg,.jpeg,.gif,image/png,image/jpeg,image/gif" className="hidden"
                  onChange={e => handleImageUpload(creative.id, e)} />
                <p className="text-xs text-muted-foreground">
                  {t("create.imageFormatHint")}
                  {target && (target.mode === "fixed"
                    ? ` · ${target.w}×${target.h}px`
                    : ` · ≥ ${target.minSide ?? 200}×${target.minSide ?? 200}px (1:1)`)}
                </p>
                <div className="flex items-center gap-3 flex-wrap">
                  <Button type="button" variant="outline" disabled={uploadingId === creative.id}
                    onClick={() => fileInputRefs.current[creative.id]?.click()} className="border-border gap-2">
                    {uploadingId === creative.id
                      ? <Loader2 className="h-4 w-4 animate-spin" />
                      : <Upload className="h-4 w-4" />}
                    {t("create.uploadImage")}
                  </Button>
                  {canCrop && (
                    <Button type="button" variant="outline" onClick={() => openCropper(creative.id)} className="border-border gap-2">
                      <Pencil className="h-4 w-4" />
                      {t("create.editImage")}
                    </Button>
                  )}
                  {creative.imageUrl && formatKey !== "popunder" && (
                    <Button type="button" variant="outline" onClick={() => setPreviewCreativeId(creative.id)} className="border-border gap-2">
                      <Eye className="h-4 w-4" />
                      {t("create.previewCreative")}
                    </Button>
                  )}
                  {creative.imageFileName && <span className="min-w-0 break-all text-sm text-muted-foreground">{creative.imageFileName}</span>}
                </div>
                {creative.sizeMismatch && target && (
                  <div className="flex items-start gap-2 p-2 rounded border border-yellow-500/30 bg-yellow-500/10">
                    <AlertTriangle className="h-4 w-4 text-yellow-500 shrink-0 mt-0.5" />
                    <p className="text-xs text-yellow-500">
                      {src?.isGif
                        ? t("create.gifExactSize").replace("{w}", String(target.w)).replace("{h}", String(target.h))
                        : t("create.imageWrongSize").replace("{w}", String(target.w)).replace("{h}", String(target.h))}
                    </p>
                  </div>
                )}
                {creative.imageUrl && (
                  <button type="button" onClick={() => setPreviewMedia({ url: creative.imageUrl!, video: false })} className="block">
                    <img src={creative.imageUrl} alt="Preview" className="mt-2 max-h-32 rounded border border-border cursor-zoom-in hover:opacity-90 transition-opacity" />
                  </button>
                )}
                {errors[`creative_${creative.id}_image`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_image`]}</p>}
              </div>
            )}
          </div>
        );
      })}

      {formatKey !== "popunder" && (
        <Button type="button" variant="outline" onClick={addCreative}
          disabled={creatives.length >= MAX_CREATIVES}
          className="border-border gap-2 w-full">
          <Plus className="h-4 w-4" /> {t("create.addCreative")} ({creatives.length}/{MAX_CREATIVES})
        </Button>
      )}
    </div>
    <Dialog open={!!previewMedia} onOpenChange={(o) => { if (!o) setPreviewMedia(null); }}>
      <DialogContent className="max-w-4xl p-2 bg-card border-border">
        {previewMedia && (
          previewMedia.video
            ? <video src={previewMedia.url} controls autoPlay muted playsInline className="h-auto max-h-[85vh] w-full rounded object-contain" />
            : <img src={previewMedia.url} alt="Preview" className="w-full h-auto max-h-[85vh] object-contain rounded" />
        )}
      </DialogContent>
    </Dialog>
    <ImageCropperDialog
      open={!!cropperCreativeId}
      source={activeSource ? { dataUrl: activeSource.dataUrl, naturalWidth: activeSource.naturalWidth, naturalHeight: activeSource.naturalHeight } : null}
      target={target}
      fileNameHint={activeSource?.fileName}
      onClose={() => setCropperCreativeId(null)}
      onSave={(file, dataUrl, dimensions) => {
        if (!cropperCreativeId) return;
        updateCreative(cropperCreativeId, {
          imageUrl: dataUrl,
          pendingFile: file,
          imageFileName: file.name,
          imageWidth: dimensions.w,
          imageHeight: dimensions.h,
          sizeMismatch: false,
        });
        setCropperCreativeId(null);
      }}
    />
    <CreativePreviewDialog
      open={!!previewCreativeId}
      onClose={() => setPreviewCreativeId(null)}
      formatKey={formatKey}
      bannerSize={bannerSize}
      brandName={brandName}
      creative={creatives.find(c => c.id === previewCreativeId) || null}
    />
    </>
  );
});
