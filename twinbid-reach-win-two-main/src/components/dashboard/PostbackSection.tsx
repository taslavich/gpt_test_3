import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Copy, Check } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";

export const POSTBACK_URL =
  "https://track.twinbid.com/postback?cid={click_id}&payout={payout}&status={status}";

interface PostbackSectionProps {
  payout?: string;
  onPayoutChange?: (value: string) => void;
}

export function PostbackSection({ payout, onPayoutChange }: PostbackSectionProps = {}) {
  const { t } = useLanguage();
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(POSTBACK_URL);
      setCopied(true);
      toast.success(t("postback.copied"));
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error(t("postback.copyFailed"));
    }
  };

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>{t("postback.urlLabel")}</Label>
        <div className="flex gap-2">
          <Input value={POSTBACK_URL} readOnly className="bg-background border-border font-mono text-sm" />
          <Button
            type="button"
            variant="outline"
            onClick={handleCopy}
            className="border-border shrink-0 gap-2"
          >
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            {copied ? t("postback.copied") : t("postback.copy")}
          </Button>
        </div>
      </div>

      <div className="space-y-3 text-sm text-muted-foreground leading-relaxed">
        <p>{t("postback.help1")}</p>
        <p>{t("postback.help2")}</p>
        <p className="text-foreground/80">{t("postback.help3")}</p>
      </div>

      {onPayoutChange && (
        <div className="space-y-2 pt-2 border-t border-border">
          <Label>{t("postback.payoutLabel")}</Label>
          <div className="relative">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground pointer-events-none">$</span>
            <Input
              type="number"
              inputMode="decimal"
              min="0"
              step="0.01"
              value={payout ?? ""}
              onChange={(e) => onPayoutChange(e.target.value)}
              placeholder="0.00"
              className="bg-background border-border pl-7"
            />
          </div>
          <p className="text-xs text-muted-foreground leading-relaxed">
            {t("postback.payoutHint")}
          </p>
        </div>
      )}
    </div>
  );
}

