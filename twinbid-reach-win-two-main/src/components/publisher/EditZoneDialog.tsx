import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { publisherApi, type PublisherTraffic } from "@/api/publisher";
import {
  usePublisher,
  usePublisherResource,
  type PublisherAdCode,
} from "@/contexts/PublisherContext";
import { publisherFormatLabel } from "@/lib/publisherInventory";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr, trafficLabels } from "@/lib/publisherUiText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { PublisherAsync, PublisherError } from "./PublisherApiState";
import {
  ZoneSettingsFields,
  PublisherDeviceFields,
  defaultPopunderFields,
  popunderValues,
} from "./ZoneFormFields";
export function EditZoneDialog({
  code,
  onOpenChange,
}: {
  code: PublisherAdCode | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { lang } = useLanguage(),
    { updateAdCode } = usePublisher();
  const current = usePublisherResource(
    ["code", code?.ad_code_id],
    (signal) => publisherApi.code(code!.ad_code_id, signal),
    !!code,
  );
  const [name, setName] = useState(""),
    [traffic, setTraffic] = useState<PublisherTraffic>("mainstream"),
    [fields, setFields] = useState(defaultPopunderFields);
  const [error, setError] = useState<unknown>(null),
    [pending, setPending] = useState(false);
  const busy = useRef(false);
  useEffect(() => {
    const c = current.data;
    if (c) {
      setName(c.name);
      setTraffic(c.traffic_type);
      setFields(c.format === "direct_link" ? { ...defaultPopunderFields(), device: c.device } : {
        device: c.device,
        trigger_click: c.trigger_click,
        frequency_minutes: String(c.frequency_minutes),
        max_per_session:
          c.max_per_session === null ? null : String(c.max_per_session),
      });
      setError(null);
    }
  }, [current.data]);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!current.data || busy.current) return;
    busy.current = true;
    setPending(true);
    setError(null);
    try {
      await updateAdCode(current.data.ad_code_id, {
        name: name.trim(),
        ...(current.data.format === "direct_link" ? { device: fields.device } : popunderValues(fields)),
        version: current.data.version,
        ...(traffic !== current.data.traffic_type
          ? { traffic_type: traffic }
          : {}),
      });
      toast.success(
        tr(lang, "Настройки сохранены", "Settings saved", "设置已保存"),
      );
      onOpenChange(false);
    } catch (err) {
      setError(err);
    } finally {
      busy.current = false;
      setPending(false);
    }
  };
  return (
    <Dialog
      open={!!code}
      onOpenChange={(next) => {
        if (!pending) onOpenChange(next);
      }}
    >
      <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto border-border bg-card">
        <DialogHeader className="pr-7 text-left">
          <DialogTitle>
            {tr(
              lang,
              "Настройки рекламного кода",
              "Ad-code settings",
              "广告代码设置",
            )}
          </DialogTitle>
          <DialogDescription>{publisherFormatLabel(code?.format ?? "")} · {code?.name}</DialogDescription>
        </DialogHeader>
        <PublisherAsync
          loading={current.isPending}
          error={current.error}
          retry={current.refetch}
        >
          {current.data && (
            <form onSubmit={submit} className="space-y-5">
              <fieldset
                disabled={pending || current.isFetching}
                className="space-y-4"
              >
                <div className="space-y-2">
                  <Label htmlFor="edit-code-name">
                    {tr(lang, "Название кода", "Code name", "代码名称")}
                  </Label>
                  <Input
                    id="edit-code-name"
                    value={name}
                    required
                    maxLength={100}
                    onChange={(e) => setName(e.target.value)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-traffic">
                    {tr(lang, "Тип трафика", "Traffic type", "流量类型")}
                  </Label>
                  <Select
                    value={traffic}
                    onValueChange={(value) =>
                      setTraffic(value as PublisherTraffic)
                    }
                  >
                    <SelectTrigger id="edit-traffic">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {(["adult", "mainstream"] as const).map((t) => (
                        <SelectItem key={t} value={t}>
                          {
                            trafficLabels[t][
                              lang === "ru" ? 0 : lang === "zh-CN" ? 2 : 1
                            ]
                          }
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                {current.data.format === "direct_link" ? (
                  <PublisherDeviceFields value={fields.device} onChange={(device) => setFields({ ...fields, device })} />
                ) : <ZoneSettingsFields value={fields} onChange={setFields} />}
              </fieldset>
              {error && (
                <PublisherError error={error} retry={current.refetch} />
              )}
              <div className="flex flex-wrap justify-end gap-2 border-t border-border pt-4">
                <Button
                  type="button"
                  variant="ghost"
                  disabled={pending}
                  onClick={() => onOpenChange(false)}
                >
                  {tr(lang, "Отмена", "Cancel", "取消")}
                </Button>
                <Button type="submit" disabled={pending || current.isFetching}>
                  {tr(lang, "Сохранить", "Save", "保存")}
                </Button>
              </div>
            </form>
          )}
        </PublisherAsync>
      </DialogContent>
    </Dialog>
  );
}
