import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Copy, Check } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";

export const POSTBACK_URL =
  "https://server4.twinbidexchange.com/curl?click_id={click_id}&payout={payout}&status={status}";

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
        <div className="flex min-w-0 flex-col gap-2 min-[440px]:flex-row">
          <Input value={POSTBACK_URL} readOnly className="min-w-0 bg-background border-border font-mono text-sm" />
          <Button
            type="button"
            variant="outline"
            onClick={handleCopy}
            className="shrink-0 gap-2 border-border"
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

      <div className="space-y-3 rounded-lg border border-border bg-background/50 p-4">
        <h3 className="font-medium text-foreground">{t("postback.parametersTitle")}</h3>
        <dl className="space-y-3 text-sm">
          <div className="grid gap-1 sm:grid-cols-[110px_1fr] sm:gap-4">
            <dt className="font-mono font-medium text-foreground">click_id</dt>
            <dd className="text-muted-foreground">{t("postback.clickIdDescription")}</dd>
          </div>
          <div className="grid gap-1 sm:grid-cols-[110px_1fr] sm:gap-4">
            <dt className="font-mono font-medium text-foreground">payout</dt>
            <dd className="text-muted-foreground">{t("postback.payoutDescription")}</dd>
          </div>
          <div className="grid gap-1 sm:grid-cols-[110px_1fr] sm:gap-4">
            <dt className="font-mono font-medium text-foreground">status</dt>
            <dd className="text-muted-foreground">{t("postback.statusDescription")}</dd>
          </div>
        </dl>
        <p className="border-t border-border pt-3 text-sm leading-relaxed text-foreground/80">
          {t("postback.optionalParametersHint")}
        </p>
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
