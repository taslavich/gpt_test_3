import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useLanguage } from "@/contexts/LanguageContext";
import type { Creative } from "@/contexts/CampaignContext";
import { X, MoreHorizontal, Bell } from "lucide-react";

interface CreativePreviewDialogProps {
  open: boolean;
  onClose: () => void;
  formatKey: string;
  bannerSize?: string;
  brandName?: string;
  creative: Creative | null;
}

/** Fake article shell reused across previews. */
function FakeSite({ children, withSidebar = true, sidebar }: { children: React.ReactNode; withSidebar?: boolean; sidebar?: React.ReactNode }) {
  return (
    <div className="rounded-md overflow-hidden border border-slate-300 bg-white text-slate-800 shadow-sm">
      {/* Browser chrome */}
      <div className="flex items-center gap-1.5 px-3 py-2 bg-slate-100 border-b border-slate-200">
        <div className="h-2.5 w-2.5 rounded-full bg-red-400" />
        <div className="h-2.5 w-2.5 rounded-full bg-yellow-400" />
        <div className="h-2.5 w-2.5 rounded-full bg-green-400" />
        <div className="ml-3 flex-1 h-5 rounded bg-white border border-slate-200 text-[10px] text-slate-500 px-2 flex items-center">
          https://example-news.com/article
        </div>
      </div>
      {/* Nav */}
      <div className="flex items-center gap-3 overflow-hidden border-b border-slate-200 px-3 py-2 text-[11px] text-slate-600 sm:gap-4 sm:px-4">
        <div className="font-bold text-slate-900">Example News</div>
        <span>Home</span><span>World</span><span className="hidden min-[420px]:inline">Tech</span><span className="hidden sm:inline">Sport</span>
      </div>
      <div className={`grid gap-4 p-3 sm:p-4 ${withSidebar ? "grid-cols-1 md:grid-cols-[minmax(0,1fr)_320px]" : "grid-cols-1"}`}>
        <div className="min-w-0">{children}</div>
        {withSidebar && <div className="min-w-0 space-y-3">{sidebar}</div>}
      </div>
    </div>
  );
}

function ArticleBody({ short = false }: { short?: boolean }) {
  return (
    <>
      <h1 className="text-lg font-bold mb-2 leading-snug">Breaking: markets rally as tech stocks lead gains</h1>
      <div className="text-[10px] text-slate-500 mb-3">By John Reporter · 2 hours ago</div>
      <div className="space-y-2 text-[11px] leading-relaxed text-slate-700">
        <p>Global equity markets rallied on Tuesday as investors welcomed a fresh batch of earnings reports and signs of easing inflation pressures across major economies.</p>
        {!short && <p>Analysts pointed to a rotation into technology names, with several megacap firms hitting fresh highs. Bond yields eased modestly while the dollar traded mixed against major peers.</p>}
        {!short && <p>"We're seeing broad participation," said one strategist. "This isn't just a handful of stocks doing the heavy lifting."</p>}
      </div>
    </>
  );
}

function BannerSlot({ size, creative }: { size: string; creative: Creative }) {
  const [w, h] = size.split("x").map(Number);
  const scale = w > 400 ? 400 / w : 1;
  const displayW = w * scale;
  const displayH = h * scale;
  const type = creative.creativeType || "image";

  let content: React.ReactNode;
  if (type === "html" && creative.htmlCode) {
    content = (
      <iframe
        title="html-preview"
        srcDoc={creative.htmlCode}
        sandbox="allow-scripts"
        style={{ width: w, height: h, transform: `scale(${scale})`, transformOrigin: "top left", border: "0" }}
      />
    );
  } else if (type === "iframe") {
    const mode = creative.iframeMode || "url";
    let src = "";
    if (mode === "code") {
      const m = (creative.iframeCode || "").match(/<iframe[^>]*\ssrc\s*=\s*["']([^"']+)["']/i);
      src = m ? m[1] : "";
    } else {
      src = creative.iframeUrl || "";
    }
    if (src) {
      content = (
        <iframe
          title="iframe-preview"
          src={src}
          sandbox="allow-scripts allow-same-origin"
          style={{ width: w, height: h, transform: `scale(${scale})`, transformOrigin: "top left", border: "0" }}
        />
      );
    }
  }
  if (!content) {
    if (creative.imageUrl) {
      const video = creative.mediaType === "video"
        || creative.imageMimeType === "video/mp4"
        || /\.mp4$/i.test(creative.imageFileName || "");
      content = video
        ? <video src={creative.imageUrl} autoPlay muted loop playsInline className="h-full w-full object-cover" />
        : <img src={creative.imageUrl} alt="ad" className="w-full h-full object-cover" />;
    } else {
      content = <span>Advertisement {w}×{h}</span>;
    }
  }

  return (
    <div className="my-3 flex max-w-full justify-center overflow-x-auto overscroll-x-contain">
      <div
        className="relative shrink-0 border border-slate-300 bg-slate-100 flex items-center justify-center text-[10px] text-slate-500 overflow-hidden"
        style={{ width: displayW, height: displayH }}
      >
        {content}
      </div>
    </div>
  );
}

function BannerPreview({ size, creative }: { size: string; creative: Creative }) {
  const [w, h] = size.split("x").map(Number);
  const isLeaderboard = w >= 468 && h <= 120;
  const isSidebar = h >= w;
  const withSidebar = !isLeaderboard;

  const sidebar = isSidebar ? <BannerSlot size={size} creative={creative} /> : (
    <>
      <div className="rounded bg-slate-100 h-24" />
      <div className="rounded bg-slate-100 h-24" />
    </>
  );

  return (
    <FakeSite withSidebar={withSidebar} sidebar={sidebar}>
      {isLeaderboard && <BannerSlot size={size} creative={creative} />}
      <ArticleBody short={isSidebar && h > 400} />
      {!isLeaderboard && !isSidebar && <BannerSlot size={size} creative={creative} />}
    </FakeSite>
  );
}

function PushPreview({ title, description, imageUrl, brandName, variant }: { title?: string; description?: string; imageUrl?: string; brandName?: string; variant: "desktop" | "mobile" }) {
  const brand = brandName?.trim() || "Brand name";
  if (variant === "desktop") {
    return (
      <div className="relative rounded-md border border-slate-300 bg-gradient-to-br from-slate-200 to-slate-300 p-6 min-h-[320px]">
        <div className="absolute bottom-4 left-4 right-4 flex max-w-[360px] gap-3 rounded-lg border border-slate-200 bg-white p-3 shadow-xl sm:left-auto">
          {imageUrl
            ? <img src={imageUrl} alt="" className="h-12 w-12 rounded object-cover shrink-0" />
            : <div className="h-12 w-12 rounded bg-slate-200 shrink-0" />}
          <div className="min-w-0 flex-1">
            <div className="text-[10px] font-medium text-slate-500 truncate">{brand}</div>
            <div className="text-[12px] font-semibold text-slate-900 truncate">{title || "Notification title"}</div>
            <div className="text-[11px] text-slate-600 line-clamp-2">{description || "Notification description shown to the user"}</div>
          </div>
          <button className="text-slate-400 hover:text-slate-600 shrink-0"><X className="h-3.5 w-3.5" /></button>
        </div>
        <div className="text-[10px] text-slate-500">Desktop notification preview</div>
      </div>
    );
  }
  // Mobile
  return (
    <div className="mx-auto w-full max-w-[300px] rounded-[28px] bg-slate-900 p-2 shadow-xl">
      <div className="rounded-[22px] bg-gradient-to-b from-indigo-500 to-purple-600 h-[420px] p-3 relative overflow-hidden">
        <div className="flex justify-between text-[10px] text-white/90 mb-3">
          <span>9:41</span>
          <span>◉ ◉ ◉</span>
        </div>
        <div className="rounded-xl bg-white/95 backdrop-blur p-3 flex gap-3 shadow-lg">
          {imageUrl
            ? <img src={imageUrl} alt="" className="h-10 w-10 rounded object-cover shrink-0" />
            : <div className="h-10 w-10 rounded bg-slate-200 shrink-0" />}
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1 text-[9px] text-slate-500">
              <Bell className="h-2.5 w-2.5" /> {brand}
            </div>
            <div className="text-[11px] font-semibold text-slate-900 truncate">{title || "Notification title"}</div>
            <div className="text-[10px] text-slate-600 line-clamp-2">{description || "Notification description"}</div>
          </div>
        </div>
      </div>
    </div>
  );
}

function NativeCard({ title, description, imageUrl, sponsored = false, brandName }: { title: string; description: string; imageUrl?: string; sponsored?: boolean; brandName?: string }) {
  return (
    <div className="rounded-lg overflow-hidden border border-slate-200 bg-white">
      {imageUrl
        ? <img src={imageUrl} alt="" className="w-full aspect-square object-cover" />
        : <div className="w-full aspect-square bg-slate-100" />}
      <div className="p-2.5">
        <div className="text-[11px] font-semibold text-slate-900 line-clamp-2 leading-snug">{title}</div>
        <div className="text-[10px] text-slate-600 line-clamp-2 mt-1">{description}</div>
        <div className="text-[9px] mt-1.5 uppercase tracking-wide text-slate-400">
          {sponsored ? `Sponsored · ${brandName?.trim() || "Brand name"}` : "example-news.com"}
        </div>
      </div>
    </div>
  );
}

function NativePreview({ title, description, imageUrl, brandName }: { title?: string; description?: string; imageUrl?: string; brandName?: string }) {
  return (
    <FakeSite withSidebar={false}>
      <ArticleBody short />
      <div className="mt-4 mb-1 text-[11px] font-semibold text-slate-700 border-b border-slate-200 pb-1">
        Recommended for you
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 sm:gap-3">
        <NativeCard title="10 travel destinations to visit this summer" description="Explore hidden gems and popular spots" />
        <NativeCard
          title={title || "Your ad title here"}
          description={description || "Your ad description shown as a native card"}
          imageUrl={imageUrl}
          brandName={brandName}
          sponsored
        />
        <NativeCard title="Home workouts that actually work in 20 minutes" description="Backed by trainers and easy to follow" />
        <NativeCard title="Best budget laptops of the year, ranked" description="Performance vs price, tested by our team" />
      </div>
    </FakeSite>
  );
}

export function CreativePreviewDialog({ open, onClose, formatKey, bannerSize, brandName, creative }: CreativePreviewDialogProps) {
  const { t } = useLanguage();
  if (!creative) return null;

  const size = bannerSize && /^\d+x\d+$/.test(bannerSize) ? bannerSize : "300x250";

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="max-w-3xl bg-card border-border">
        <DialogHeader>
          <DialogTitle>{t("create.previewTitle")}</DialogTitle>
        </DialogHeader>

        {formatKey === "banner" && (
          <BannerPreview size={size} creative={creative} />
        )}

        {formatKey === "push" && (
          <Tabs defaultValue="desktop">
            <TabsList className="w-full justify-start overflow-x-auto bg-background border border-border sm:w-auto">
              <TabsTrigger value="desktop">{t("create.previewDesktop")}</TabsTrigger>
              <TabsTrigger value="mobile">{t("create.previewMobile")}</TabsTrigger>
            </TabsList>
            <TabsContent value="desktop" className="mt-3">
              <PushPreview variant="desktop" title={creative.title} description={creative.description} imageUrl={creative.imageUrl} brandName={brandName} />
            </TabsContent>
            <TabsContent value="mobile" className="mt-3">
              <PushPreview variant="mobile" title={creative.title} description={creative.description} imageUrl={creative.imageUrl} brandName={brandName} />
            </TabsContent>
          </Tabs>
        )}

        {formatKey === "native" && (
          <NativePreview title={creative.title} description={creative.description} imageUrl={creative.imageUrl} brandName={brandName} />
        )}

        {formatKey === "popunder" && (
          <div className="text-sm text-muted-foreground p-4 text-center">
            {t("create.previewPopunderNote")}
          </div>
        )}

        <p className="text-xs text-muted-foreground text-center">{t("create.previewDisclaimer")}</p>
      </DialogContent>
    </Dialog>
  );
}
