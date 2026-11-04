import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Link2 } from "lucide-react";
import type { PublisherCodeFormat } from "@/api/publisher";
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
  PublisherDeviceFields,
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
    { createAdCode, directLinkAvailable } = usePublisher();
  const [format, setFormat] = useState<PublisherCodeFormat>("popunder");
  const direct = format === "direct_link";
  const unavailable = direct && !directLinkAvailable;
  const [name, setName] = useState(""),
    [traffic, setTraffic] = useState<TrafficType | "">("");
  const [fields, setFields] = useState(defaultPopunderFields),
    [error, setError] = useState<unknown>(null),
    [pending, setPending] = useState(false);
  const busy = useRef(false);
  useEffect(() => {
    if (open) {
      setName("");
      setFormat("popunder");
      setTraffic("");
      setFields(defaultPopunderFields());
      setError(null);
    }
  }, [open]);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy.current || unavailable) return;
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
      const common = { name: name.trim(), traffic_type: traffic };
      const code = await createAdCode(direct
        ? { ...common, format: "direct_link", device: fields.device }
        : { ...common, format: "popunder", ...popunderValues(fields) });
      toast.success(direct
        ? tr(lang, "Direct Link создан", "Direct Link created", "直链已创建")
        : tr(lang, "Рекламный код создан", "Ad code created", "广告代码已创建"));
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
            {direct ? tr(lang,
              "Разместите ссылку на кнопке или в другом месте, где пользователь сможет по ней перейти. JS-код не нужен.",
              "Place the link on a button or anywhere a user can follow it. No JavaScript is required.",
              "将直链放在按钮或用户可点击的位置，无需安装 JavaScript。"
            ) : tr(
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
            <div role="group" aria-label={tr(lang, "Формат рекламы", "Ad format", "广告形式")} className="grid gap-2 sm:grid-cols-2">
              {(["popunder", "direct_link"] as const).map((option) => (
                <button
                  key={option}
                  type="button"
                  aria-pressed={format === option}
                  onClick={() => { setFormat(option); setError(null); }}
                  className={cn("flex items-center gap-3 rounded-xl border p-4 text-left",
                    format === option ? "border-primary bg-primary/10" : "border-border hover:border-primary/50")}
                >
                  {option === "direct_link" && <Link2 className="h-5 w-5 shrink-0" aria-hidden="true" />}
                  <span className="font-medium">{option === "popunder" ? "Popunder" : "Direct Link"}</span>
                </button>
              ))}
            </div>
            {unavailable ? (
              <p role="status" className="rounded-lg border border-border p-3 text-sm text-muted-foreground">
                {tr(lang, "Создание Direct Link пока недоступно.", "Creating Direct Links is not available yet.", "暂时无法创建直链。")}
              </p>
            ) : direct ? (
              <PublisherDeviceFields value={fields.device} onChange={(device) => setFields({ ...fields, device })} />
            ) : (
              <>
                <div className="max-w-xs rounded-xl border border-primary bg-primary/10 p-3">
                  <PublisherFormatVisual format="popunder" active />
                </div>
                <ZoneSettingsFields value={fields} onChange={setFields} />
              </>
            )}
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
            <Button type="submit" disabled={pending || unavailable} className="h-auto min-h-10 whitespace-normal py-2 text-center">
              {direct ? tr(lang, "Создать и получить ссылку", "Create and get link", "创建并获取直链") : tr(
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
