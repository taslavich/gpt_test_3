import { useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Upload, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import type { Creative } from "@/contexts/CampaignContext";

const URL_MACROS = [
  "click_id", "site_id", "country_code", "creative_id",
  "campaign_id", "browser", "device", "device_os", "ip_address",
] as const;

interface CreativesEditorProps {
  formatKey: string;
  creatives: Creative[];
  onChange: (creatives: Creative[]) => void;
  errors?: Record<string, string>;
}

const generateId = () => String(Date.now()) + Math.random().toString(36).slice(2, 6);

export function CreativesEditor({ formatKey, creatives, onChange, errors = {} }: CreativesEditorProps) {
  const { t } = useLanguage();
  const fileInputRefs = useRef<Record<string, HTMLInputElement | null>>({});

  const showTitle = formatKey === "native" || formatKey === "push";
  const showDescription = formatKey === "native" || formatKey === "push";
  const showImage = formatKey !== "popunder";

  const updateCreative = (id: string, updates: Partial<Creative>) => {
    onChange(creatives.map(c => c.id === id ? { ...c, ...updates } : c));
  };

  const addCreative = () => {
    onChange([...creatives, { id: generateId(), url: "" }]);
  };

  const removeCreative = (id: string) => {
    if (creatives.length <= 1) return;
    onChange(creatives.filter(c => c.id !== id));
  };

  const handleImageUpload = (creativeId: string, e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const url = URL.createObjectURL(file);
      updateCreative(creativeId, { imageUrl: url, imageFileName: file.name });
      toast.success(t("create.imageUploaded"));
    }
  };

  return (
    <div className="space-y-4">
      {creatives.map((creative, idx) => (
        <div key={creative.id} className="p-4 rounded-lg border border-border bg-background/30 space-y-4">
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
            <Input value={creative.name || ""} onChange={e => updateCreative(creative.id, { name: e.target.value })}
              placeholder={t("create.creativeNamePlaceholder")}
              className={`bg-background border-border ${errors[`creative_${creative.id}_name`] ? "border-destructive" : ""}`} />
            <p className="text-xs text-muted-foreground">{t("create.creativeNameHint")}</p>
            {errors[`creative_${creative.id}_name`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_name`]}</p>}
          </div>

          {showTitle && (
            <div className="space-y-2">
              <Label>Title *</Label>
              <Input value={creative.title || ""} onChange={e => updateCreative(creative.id, { title: e.target.value })}
                placeholder={t("create.titlePlaceholder")}
                className={`bg-background border-border ${errors[`creative_${creative.id}_title`] ? "border-destructive" : ""}`} />
              {errors[`creative_${creative.id}_title`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_title`]}</p>}
            </div>
          )}

          {showDescription && (
            <div className="space-y-2">
              <Label>Description ({t("create.optional")})</Label>
              <Textarea value={creative.description || ""} onChange={e => updateCreative(creative.id, { description: e.target.value })}
                placeholder={t("create.descriptionPlaceholder")} className="bg-background border-border resize-none" rows={2} />
            </div>
          )}

          <div className="space-y-2">
            <Label>URL *</Label>
            <Input value={creative.url} onChange={e => updateCreative(creative.id, { url: e.target.value })}
              placeholder="https://example.com/landing"
              className={`bg-background border-border ${errors[`creative_${creative.id}_url`] ? "border-destructive" : ""}`} />
            {errors[`creative_${creative.id}_url`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_url`]}</p>}
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">{t("create.urlMacrosHint") || "Click to add tracking macros:"}</p>
              <div className="flex flex-wrap gap-1.5">
                {URL_MACROS.map(macro => (
                  <Badge
                    key={macro}
                    variant="outline"
                    className="cursor-pointer text-xs font-mono hover:bg-primary/10 hover:border-primary/30 transition-colors"
                    onClick={() => {
                      const separator = creative.url.includes("?") ? "&" : "?";
                      const token = `${macro}={${macro}}`;
                      if (creative.url.includes(`{${macro}}`)) return;
                      updateCreative(creative.id, {
                        url: creative.url + separator + token,
                      });
                    }}
                  >
                    {`{${macro}}`}
                  </Badge>
                ))}
              </div>
            </div>
          </div>

          {showImage && (
            <div className="space-y-2">
              <Label>{t("create.uploadImage")} *</Label>
              <input
                ref={el => { fileInputRefs.current[creative.id] = el; }}
                type="file" accept="image/*" className="hidden"
                onChange={e => handleImageUpload(creative.id, e)} />
              <div className="flex items-center gap-3">
                <Button type="button" variant="outline" onClick={() => fileInputRefs.current[creative.id]?.click()} className="border-border gap-2">
                  <Upload className="h-4 w-4" /> {t("create.uploadImage")}
                </Button>
                {creative.imageFileName && <span className="text-sm text-muted-foreground">{creative.imageFileName}</span>}
              </div>
              {creative.imageUrl && (
                <img src={creative.imageUrl} alt="Preview" className="mt-2 max-h-32 rounded border border-border" />
              )}
              {errors[`creative_${creative.id}_image`] && <p className="text-xs text-destructive">{errors[`creative_${creative.id}_image`]}</p>}
            </div>
          )}
        </div>
      ))}

      {formatKey !== "popunder" && (
        <Button type="button" variant="outline" onClick={addCreative} className="border-border gap-2 w-full">
          <Plus className="h-4 w-4" /> {t("create.addCreative")}
        </Button>
      )}
    </div>
  );
}
