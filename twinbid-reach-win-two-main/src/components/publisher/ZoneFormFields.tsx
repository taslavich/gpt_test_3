import { ApiError } from "@/api/http";
import type { CreatePublisherCode, PublisherDevice } from "@/api/publisher";
import { useLanguage } from "@/contexts/LanguageContext";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import { PublisherPlacementNotice } from "./PublisherPlacementNotice";
import { cn } from "@/lib/utils";
import { Monitor, MonitorSmartphone, Smartphone, Sparkles } from "lucide-react";
export interface PopunderFields {
  device: PublisherDevice;
  trigger_click: 1 | 2;
  frequency_minutes: string;
  max_per_session: string | null;
}
export const defaultPopunderFields = (): PopunderFields => ({
  device: "all",
  trigger_click: 1,
  frequency_minutes: "0.1",
  max_per_session: null,
});
export function popunderValues(
  fields: PopunderFields,
): Pick<
  CreatePublisherCode,
  "device" | "trigger_click" | "frequency_minutes" | "max_per_session"
> {
  const frequency = Number(fields.frequency_minutes),
    cap =
      fields.max_per_session === null
        ? null
        : Number(fields.max_per_session.trim() || "invalid");
  if (
    !fields.frequency_minutes.trim() ||
    !Number.isFinite(frequency) ||
    frequency < 0.1 ||
    frequency > 10080 ||
    (cap !== null && (!Number.isInteger(cap) || cap < 1 || cap > 100000))
  )
    throw new ApiError(
      400,
      "frequency_minutes: 0.1–10080; max_per_session: 1–100000 or null",
    );
  return {
    device: fields.device,
    trigger_click: fields.trigger_click,
    frequency_minutes: frequency,
    max_per_session: cap,
  };
}
export function ZoneSettingsFields({
  value,
  onChange,
}: {
  value: PopunderFields;
  onChange: (value: PopunderFields) => void;
}) {
  const { lang } = useLanguage();
  const frequency = Number(value.frequency_minutes);
  const validFrequency = value.frequency_minutes.trim() && Number.isFinite(frequency) && frequency >= 0.1 && frequency <= 10080;
  const cap = value.max_per_session === null ? null : Number(value.max_per_session);
  const first = value.trigger_click === 1
    ? tr(lang, "Первый показ после первого клика", "First opening after the first click", "首次点击后展示")
    : tr(lang, "Первый показ после второго клика", "First opening after the second click", "第二次点击后首次展示");
  const interval = validFrequency
    ? tr(lang, `не чаще одного раза в ${frequency.toLocaleString("ru")} мин.`, `at most once every ${frequency} min.`, `每 ${frequency} 分钟最多展示一次`)
    : tr(lang, "укажите интервал между показами", "set the interval between openings", "请设置展示间隔");
  const maximum = cap === null
    ? tr(lang, "без ограничения числа показов за сессию страницы", "with no opening limit per page session", "每个页面会话不限展示次数")
    : Number.isInteger(cap) && cap >= 1
      ? tr(lang, `не более ${cap} показов за сессию страницы`, `at most ${cap} openings per page session`, `每个页面会话最多展示 ${cap} 次`)
      : tr(lang, "укажите число показов за сессию страницы", "set the opening limit per page session", "请设置每个页面会话的展示上限");
  const devices = value.device === "all"
    ? tr(lang, "на всех устройствах", "on all devices", "在所有设备上")
    : value.device === "desktop"
      ? tr(lang, "на компьютерах", "on desktop devices", "在电脑上")
      : tr(lang, "на мобильных устройствах", "on mobile devices", "在移动设备上");
  return (
    <Card className="space-y-5 p-4">
      <div className="space-y-2">
        <Label>{tr(lang, "Устройства", "Devices", "设备")}</Label>
        <div className="grid grid-cols-1 gap-2 min-[420px]:grid-cols-3">
          {(
            [
              ["all", "Все", "All", "全部", MonitorSmartphone],
              ["desktop", "Компьютеры", "Desktop", "电脑", Monitor],
              ["mobile", "Мобильные", "Mobile", "移动设备", Smartphone],
            ] as const
          ).map(([device, ru, en, zh, Icon]) => (
            <button
              key={device}
              type="button"
              aria-pressed={value.device === device}
              className={cn(
                "flex min-w-0 items-center justify-center gap-2 rounded-lg border p-3 text-sm",
                value.device === device
                  ? "border-primary bg-primary/10"
                  : "border-border",
              )}
              onClick={() => onChange({ ...value, device })}
            >
              <Icon className="h-4 w-4 shrink-0" />
              <span>{tr(lang, ru, en, zh)}</span>
            </button>
          ))}
        </div>
      </div>
      <div className="space-y-2">
        <Label>{tr(lang, "Первый показ", "First opening", "首次展示")}</Label>
        <div className="grid gap-2 sm:grid-cols-2">
          {([1, 2] as const).map((click) => (
            <button
              key={click}
              type="button"
              aria-pressed={value.trigger_click === click}
              className={cn(
                "rounded-lg border p-3 text-sm",
                value.trigger_click === click
                  ? "border-primary bg-primary/10"
                  : "border-border",
              )}
              onClick={() => onChange({ ...value, trigger_click: click })}
            >
              {click === 1
                ? tr(
                    lang,
                    "После первого клика",
                    "After the first click",
                    "第一次点击后",
                  )
                : tr(
                    lang,
                    "После второго клика",
                    "After the second click",
                    "第二次点击后",
                  )}
            </button>
          ))}
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="pop-frequency">
            {tr(lang, "Интервал, минуты", "Interval, minutes", "间隔（分钟）")}
          </Label>
          <Input
            id="pop-frequency"
            type="number"
            min="0.1"
            max="10080"
            step="any"
            required
            value={value.frequency_minutes}
            onChange={(e) =>
              onChange({ ...value, frequency_minutes: e.target.value })
            }
          />
          <p className="text-xs text-muted-foreground">0.1–10080</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="pop-cap">
            {tr(
              lang,
              "Максимум за сессию страницы",
              "Maximum per page session",
              "每个页面会话的上限",
            )}
          </Label>
          <Input
            id="pop-cap"
            type="number"
            min="1"
            max="100000"
            step="1"
            disabled={value.max_per_session === null}
            value={value.max_per_session ?? ""}
            onChange={(e) =>
              onChange({ ...value, max_per_session: e.target.value })
            }
          />
          <div className="flex items-center gap-2">
            <Switch
              id="pop-unlimited"
              checked={value.max_per_session === null}
              onCheckedChange={(unlimited) =>
                onChange({ ...value, max_per_session: unlimited ? null : "1" })
              }
            />
            <Label htmlFor="pop-unlimited">
              {tr(lang, "Без ограничения", "Unlimited", "不限次数")}
            </Label>
          </div>
        </div>
      </div>
      <div className="flex gap-3 rounded-xl border border-primary/25 bg-primary/5 p-4" aria-live="polite">
        <Sparkles className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
        <div className="min-w-0">
          <h3 className="font-medium">{tr(lang, "Как будет работать рекламный код", "How the ad code will work", "广告代码的工作方式")}</h3>
          <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{first}, {interval}, {maximum}, {devices}.</p>
        </div>
      </div>
      <PublisherPlacementNotice context="form" />
    </Card>
  );
}
