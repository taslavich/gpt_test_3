import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import {
  usePublisher,
  type PublisherAdCode,
  type TrafficType,
} from "@/contexts/PublisherContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr, trafficLabels } from "@/lib/publisherUiText";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { PublisherFormatVisual } from "./PublisherFormatVisual";
import {
  ZoneSettingsFields,
  defaultPopunderFields,
  popunderValues,
} from "./ZoneFormFields";
import { PublisherError } from "./PublisherApiState";
export function CreateZoneDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (code: PublisherAdCode) => void;
}) {
  const { lang } = useLanguage(),
    { createAdCode } = usePublisher();
  const [name, setName] = useState(""),
    [traffic, setTraffic] = useState<TrafficType | "">("");
  const [fields, setFields] = useState(defaultPopunderFields),
    [error, setError] = useState<unknown>(null),
    [pending, setPending] = useState(false);
  const busy = useRef(false);
  useEffect(() => {
    if (open) {
      setName("");
      setTraffic("");
      setFields(defaultPopunderFields());
      setError(null);
    }
  }, [open]);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy.current) return;
    if (!traffic || !name.trim()) {
      toast.error(
        tr(
          lang,
          "Укажите название и тип трафика",
          "Enter a name and choose a traffic type",
          "请输入名称并选择流量类型",
        ),
      );
      return;
    }
    busy.current = true;
    setPending(true);
    setError(null);
    try {
      const code = await createAdCode({
        name: name.trim(),
        format: "popunder",
        traffic_type: traffic,
        ...popunderValues(fields),
      });
      toast.success(
        tr(lang, "Рекламный код создан", "Ad code created", "广告代码已创建"),
      );
      onOpenChange(false);
      onCreated?.(code);
    } catch (err) {
      setError(err);
    } finally {
      busy.current = false;
      setPending(false);
    }
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!pending) onOpenChange(next);
      }}
    >
      <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto border-border bg-card">
        <DialogHeader className="pr-7 text-left">
          <DialogTitle>
            {tr(lang, "Новый рекламный код", "New ad code", "新广告代码")}
          </DialogTitle>
          <DialogDescription>
            {tr(
              lang,
              "Установите код на сайт. Сайт и зона появятся после первого реального запроса.",
              "Install the code on a website. The site and zone appear after the first real request.",
              "将代码安装到网站，网站和广告位将在首次真实请求后显示。",
            )}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-5">
          <fieldset disabled={pending} className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="ad-code-name">
                {tr(lang, "Название кода *", "Code name *", "代码名称 *")}
              </Label>
              <Input
                id="ad-code-name"
                value={name}
                maxLength={100}
                required
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label>
                {tr(lang, "Тип трафика *", "Traffic type *", "流量类型 *")}
              </Label>
              <div className="grid gap-2 sm:grid-cols-2">
                {(["adult", "mainstream"] as const).map((type) => (
                  <button
                    key={type}
                    type="button"
                    aria-pressed={traffic === type}
                    onClick={() => setTraffic(type)}
                    className={cn(
                      "rounded-xl border px-4 py-3 text-left text-sm",
                      traffic === type
                        ? "border-primary bg-primary/10"
                        : "border-border hover:border-primary/50",
                    )}
                  >
                    {
                      trafficLabels[type][
                        lang === "ru" ? 0 : lang === "zh-CN" ? 2 : 1
                      ]
                    }
                  </button>
                ))}
              </div>
            </div>
            <div className="max-w-xs rounded-xl border border-primary bg-primary/10 p-3">
              <PublisherFormatVisual format="popunder" active />
              <p className="mt-3 font-medium">Popunder</p>
            </div>
            <ZoneSettingsFields value={fields} onChange={setFields} />
          </fieldset>
          {error && <PublisherError error={error} />}
          <div className="flex flex-wrap justify-end gap-2 border-t border-border pt-4">
            <Button
              type="button"
              variant="ghost"
              disabled={pending}
              onClick={() => onOpenChange(false)}
            >
              {tr(lang, "Отмена", "Cancel", "取消")}
            </Button>
            <Button type="submit" disabled={pending} className="h-auto min-h-10 whitespace-normal py-2 text-center">
              {tr(
                lang,
                "Создать и получить код",
                "Create and get code",
                "创建并获取代码",
              )}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
