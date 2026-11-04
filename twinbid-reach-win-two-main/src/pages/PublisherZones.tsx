import { useMemo, useState } from "react";
import { ChevronDown, Copy, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { publisherCodeFormatOptions, publisherFormatLabel } from "@/lib/publisherInventory";
import { PublisherPlacementNotice } from "@/components/publisher/PublisherPlacementNotice";
import { PublisherStatsFilter } from "@/components/publisher/PublisherStatsFilter";
import { Switch } from "@/components/ui/switch";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
} from "@/components/ui/alert-dialog";
import { useLanguage } from "@/contexts/LanguageContext";
import {
  usePublisher,
  type PublisherAdCode,
} from "@/contexts/PublisherContext";
import { publisherUiText as tr, trafficLabels } from "@/lib/publisherUiText";
import { CreateZoneDialog } from "@/components/publisher/CreateZoneDialog";
import { EditZoneDialog } from "@/components/publisher/EditZoneDialog";
import { ZoneCodeDialog } from "@/components/publisher/ZoneCodeDialog";
import { PublisherZoneTable } from "@/components/publisher/PublisherInventoryTable";
import {
  PublisherAsync,
  PublisherEmpty,
  PublisherError,
  PublisherPagination,
} from "@/components/publisher/PublisherApiState";
export default function PublisherZones() {
  const { lang } = useLanguage(),
    p = usePublisher();
  const [format, setFormat] = useState("");
  const [search, setSearch] = useState(""),
    [createOpen, setCreateOpen] = useState(false),
    [expanded, setExpanded] = useState<string | null>(null);
  const [selected, setSelected] = useState<PublisherAdCode | null>(null),
    [edit, setEdit] = useState<PublisherAdCode | null>(null),
    [archive, setArchive] = useState<PublisherAdCode | null>(null),
    [error, setError] = useState<unknown>(null);
  const items = useMemo(
    () =>
      p.adCodes.filter((c) =>
        (!format || c.format === format) && `${c.name} ${c.short_code} ${c.ad_code_id} ${c.traffic_type}`
          .toLowerCase()
          .includes(search.toLowerCase().trim()),
      ),
    [p.adCodes, search, format],
  );
  const action = async (run: () => Promise<unknown>) => {
    setError(null);
    try {
      await run();
    } catch (err) {
      setError(err);
    }
  };
  return (
    <div className="mx-auto max-w-[1440px] space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">
            {tr(lang, "Рекламные коды", "Ad codes", "广告代码")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {tr(
              lang,
              "Управляйте рекламными кодами и прямыми ссылками.",
              "Manage ad codes and direct links.",
              "管理广告代码和直链。",
            )}
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          {tr(lang, "Создать код", "Create code", "创建代码")}
        </Button>
      </div>
      {error && (
        <PublisherError
          error={error}
          retry={() => {
            setError(null);
            return p.refetch();
          }}
        />
      )}
      <PublisherPlacementNotice />
      <Card className="flex flex-col gap-4 p-4 sm:flex-row sm:items-end">
        <div className="relative min-w-0 flex-1">
          <Search className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
          <Input
            className="pl-9"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={tr(
              lang,
              "Название или ID кода",
              "Code name or ID",
              "代码名称或 ID",
            )}
          />
        </div>
        <PublisherStatsFilter id="publisher-code-format" allLabel={tr(lang,"Все форматы","All formats","全部格式")} label={tr(lang,"Формат","Format","格式")} options={publisherCodeFormatOptions} value={format} onChange={setFormat} />
      </Card>
      <PublisherAsync loading={p.loading} error={p.error} retry={p.refetch}>
        {!items.length ? (
          <PublisherEmpty>
            {tr(
              lang,
              "На этой странице нет рекламных кодов.",
              "There are no ad codes on this page.",
              "本页暂无广告代码。",
            )}
          </PublisherEmpty>
        ) : (
          <Card className="divide-y divide-border overflow-hidden">
            {items.map((code) => (
              <div key={code.ad_code_id}>
                <div className="flex min-w-0 flex-col gap-3 p-4 xl:flex-row xl:items-start xl:justify-between">
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 items-start gap-3 pt-1.5 text-left"
                    aria-expanded={expanded === code.ad_code_id}
                    onClick={() =>
                      setExpanded(
                        expanded === code.ad_code_id ? null : code.ad_code_id,
                      )
                    }
                  >
                    <ChevronDown className={`mt-1 h-4 w-4 shrink-0 transition-transform ${expanded === code.ad_code_id ? "rotate-180" : ""}`} />
                    <span className="min-w-0">
                      <span className="block break-words font-medium [overflow-wrap:anywhere]">
                        {code.name}
                      </span>
                      <span className="mt-1 block break-words text-xs text-muted-foreground">
                        {publisherFormatLabel(code.format)} ·{" "}
                        {
                          trafficLabels[code.traffic_type][
                            lang === "ru" ? 0 : lang === "zh-CN" ? 2 : 1
                          ]
                        }{" "}
                        · {code.device === "all" ? tr(lang, "Все устройства", "All devices", "所有设备") : code.device === "desktop" ? tr(lang, "Компьютеры", "Desktop", "电脑") : tr(lang, "Мобильные устройства", "Mobile devices", "移动设备")}
                      </span>
                    </span>
                  </button>
                  <div className="flex min-w-0 flex-wrap items-center gap-2 xl:shrink-0 xl:justify-end">
                    <label className="flex items-center gap-2 text-xs">
                      <Switch
                        checked={code.is_active}
                        disabled={p.mutationPending || code.archived}
                        aria-label={tr(
                          lang,
                          "Активность кода",
                          "Code activity",
                          "代码状态",
                        )}
                        onCheckedChange={(is_active) =>
                          void action(() =>
                            p.updateAdCode(code.ad_code_id, {
                              is_active,
                              version: code.version,
                            }),
                          )
                        }
                      />
                      {code.archived
                        ? tr(lang, "Архивирован", "Archived", "已归档")
                        : code.is_active
                          ? tr(lang, "Код активен", "Code active", "代码已启用")
                          : tr(lang, "Код неактивен", "Code inactive", "代码未启用")}
                    </label>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setSelected(code)}
                    >
                      <Copy className="mr-1 h-4 w-4" />
                      {code.format === "direct_link" ? tr(lang, "Ссылка", "Link", "直链") : tr(lang, "Код", "Code", "代码")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={p.mutationPending}
                      onClick={() =>
                        void action(async () => {
                          const copy = await p.duplicateAdCode(code.ad_code_id);
                          setSelected(copy);
                          p.setCodePage({ ...p.codePage, offset: 0 });
                        })
                      }
                    >
                      {tr(lang, "Дублировать", "Duplicate", "复制")}
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      disabled={code.archived || p.mutationPending}
                      aria-label={tr(
                        lang,
                        "Настроить код",
                        "Edit code",
                        "编辑代码",
                      )}
                      onClick={() => setEdit(code)}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      disabled={code.archived || p.mutationPending}
                      aria-label={tr(
                        lang,
                        "Архивировать код",
                        "Archive code",
                        "归档代码",
                      )}
                      onClick={() => setArchive(code)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                </div>
                {expanded === code.ad_code_id && (
                  <div className="border-t border-border bg-muted/20 p-4">
                    <PublisherZoneTable codeId={code.ad_code_id} />
                  </div>
                )}
              </div>
            ))}
          </Card>
        )}
        <PublisherPagination
          {...p.codePage}
          count={p.adCodes.length}
          onChange={(offset) => p.setCodePage({ ...p.codePage, offset })}
        />
      </PublisherAsync>
      <CreateZoneDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={setSelected}
      />
      <ZoneCodeDialog
        code={selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      />
      <EditZoneDialog
        code={edit}
        onOpenChange={(open) => {
          if (!open) setEdit(null);
        }}
      />
      <AlertDialog
        open={!!archive}
        onOpenChange={(open) => {
          if (!open) setArchive(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {tr(
                lang,
                "Архивировать код и прекратить показы?",
                "Archive the code and stop serving?",
                "归档代码并停止投放？",
              )}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {tr(
                lang,
                "История статистики сохранится.",
                "Historical statistics remain available.",
                "历史统计数据将保留。",
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>
              {tr(lang, "Оставить", "Keep", "保留")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (archive)
                  void action(() => p.deleteAdCode(archive.ad_code_id));
                setArchive(null);
              }}
            >
              {tr(lang, "Архивировать", "Archive", "归档")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
