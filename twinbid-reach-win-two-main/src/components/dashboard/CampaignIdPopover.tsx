import { useState } from "react";
import { Check, Copy, Fingerprint } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useLanguage } from "@/contexts/LanguageContext";

interface CampaignIdPopoverProps {
  campaignId: string;
}

export function CampaignIdPopover({ campaignId }: CampaignIdPopoverProps) {
  const { t } = useLanguage();
  const [copied, setCopied] = useState(false);

  const copyId = async () => {
    try {
      await navigator.clipboard.writeText(campaignId);
      setCopied(true);
      toast.success(t("campaigns.idCopied"));
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error(t("postback.copyFailed"));
    }
  };

  return (
    <Popover onOpenChange={(open) => { if (!open) setCopied(false); }}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-8 w-8 text-muted-foreground hover:bg-primary/10 hover:text-primary"
          aria-label={t("campaigns.showId")}
          title={t("campaigns.showId")}
        >
          <Fingerprint className="h-4 w-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[min(360px,calc(100vw-1rem))] space-y-3 p-4">
        <div>
          <p className="text-sm font-semibold">{t("campaigns.campaignId")}</p>
          <p className="mt-1 text-xs text-muted-foreground">{t("campaigns.idHint")}</p>
        </div>
        <div className="rounded-md border border-border bg-muted/40 px-3 py-2 font-mono text-xs break-all select-all">
          {campaignId}
        </div>
        <Button type="button" variant="outline" size="sm" className="w-full gap-2" onClick={copyId}>
          {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
          {copied ? t("postback.copied") : t("campaigns.copyId")}
        </Button>
      </PopoverContent>
    </Popover>
  );
}
