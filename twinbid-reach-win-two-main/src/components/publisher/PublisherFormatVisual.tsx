import { Bell, Clapperboard, Image, MousePointer2, Newspaper, PictureInPicture2, Play, Video } from "lucide-react";
import type { ZoneFormat } from "@/contexts/PublisherContext";
import { cn } from "@/lib/utils";

const icons: Record<ZoneFormat, typeof Image> = {
  banner: Image,
  inpage_push: Bell,
  popunder: MousePointer2,
  native: Newspaper,
  video_popup: PictureInPicture2,
  instream: Clapperboard,
  outstream: Video,
};

export function PublisherFormatVisual({ format, compact = false, active = false }: { format: ZoneFormat; compact?: boolean; active?: boolean }) {
  const Icon = icons[format];
  if (compact) return <span className={cn("relative flex h-9 w-12 shrink-0 items-center justify-center overflow-hidden rounded-lg border bg-background", active ? "border-primary/50" : "border-border")}><MiniPlacement format={format} /><Icon className="absolute bottom-1 right-1 h-3 w-3 text-primary" /></span>;

  return (
    <div className={cn("relative h-[74px] overflow-hidden rounded-lg border bg-background/70", active ? "border-primary/40" : "border-border/70")}>
      <div className="flex h-5 items-center gap-1 border-b border-border/60 px-2"><span className="h-1.5 w-1.5 rounded-full bg-primary/70" /><span className="h-1 w-8 rounded-full bg-muted-foreground/20" /><Icon className="ml-auto h-3 w-3 text-primary" /></div>
      <div className="absolute inset-x-2 bottom-2 top-7 overflow-hidden rounded border border-border/50 bg-card/60">
        <span className="absolute left-2 top-2 h-1 w-1/2 rounded-full bg-muted-foreground/20" /><span className="absolute left-2 top-4 h-1 w-2/3 rounded-full bg-muted-foreground/15" />
        <Placement format={format} />
      </div>
    </div>
  );
}

function Placement({ format }: { format: ZoneFormat }) {
  if (format === "banner") return <span className="absolute inset-x-2 bottom-2 h-3 rounded-sm border border-primary/50 bg-primary/15" />;
  if (format === "inpage_push") return <span className="absolute bottom-1.5 right-1.5 flex h-6 w-14 items-center gap-1 rounded border border-primary/55 bg-background p-1"><span className="h-4 w-4 rounded-sm bg-primary/25" /><span className="space-y-1"><i className="block h-1 w-7 rounded bg-muted-foreground/30" /><i className="block h-1 w-5 rounded bg-muted-foreground/20" /></span></span>;
  if (format === "popunder") return <><span className="absolute bottom-1.5 left-2 rounded border border-primary/50 px-1.5 py-0.5 text-[6px] text-primary">CLICK</span><span className="absolute bottom-0 right-0 h-8 w-11 translate-x-1 translate-y-1 rounded-tl border border-accent/40 bg-accent/10" /></>;
  if (format === "native") return <span className="absolute inset-x-2 bottom-1.5 flex h-6 items-center gap-1.5 rounded border border-primary/35 p-1"><span className="h-4 w-4 rounded-sm bg-primary/25" /><span className="flex-1 space-y-1"><i className="block h-1 w-4/5 rounded bg-muted-foreground/30" /><i className="block h-1 w-full rounded bg-muted-foreground/15" /></span></span>;
  if (format === "video_popup") return <span className="absolute bottom-1 right-1 flex h-7 w-11 items-center justify-center rounded border border-primary/50 bg-primary/15"><Play className="h-3 w-3 fill-primary text-primary" /></span>;
  if (format === "instream") return <span className="absolute inset-x-2 bottom-1.5 flex h-7 items-center justify-center rounded border border-primary/40 bg-primary/10"><Play className="h-3 w-3 fill-primary text-primary" /><i className="absolute inset-x-1 bottom-1 h-0.5 rounded bg-primary/40" /></span>;
  return <span className="absolute inset-x-2 bottom-1.5 flex h-5 items-center justify-center rounded border border-primary/40 bg-primary/10"><Play className="h-2.5 w-2.5 fill-primary text-primary" /></span>;
}

function MiniPlacement({ format }: { format: ZoneFormat }) {
  const base = "absolute rounded-sm border border-primary/50 bg-primary/15";
  if (format === "banner") return <span className={cn(base, "inset-x-2 bottom-2 h-1.5")} />;
  if (format === "inpage_push" || format === "video_popup") return <span className={cn(base, "bottom-1.5 right-1.5 h-3 w-5")} />;
  if (format === "popunder") return <span className={cn(base, "left-1.5 top-1.5 h-4 w-6 shadow-[4px_4px_0_hsl(var(--accent)/0.18)]")} />;
  if (format === "native") return <span className={cn(base, "inset-x-2 top-1.5 h-4")} />;
  return <span className={cn(base, "inset-x-2 top-1.5 flex h-4 items-center justify-center")}><Play className="h-2 w-2 fill-primary text-primary" /></span>;
}
