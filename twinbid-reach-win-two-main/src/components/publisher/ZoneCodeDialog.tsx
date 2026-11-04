import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Copy, Check } from "lucide-react";
import { ApiError } from "@/api/http";
import {
  usePublisher,
  type PublisherAdCode,
} from "@/contexts/PublisherContext";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import { publisherFormatLabel } from "@/lib/publisherInventory";
import { buildJsSnippet, publisherDirectLink } from "@/lib/publisherSnippets";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { PublisherError } from "./PublisherApiState";
import { PublisherPlacementNotice } from "./PublisherPlacementNotice";
import { PublisherDiagnostics } from "./PublisherDiagnostics";
export function ZoneCodeDialog({
  code,
  onOpenChange,
}: {
  code: PublisherAdCode | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { lang } = useLanguage(),
    { adCodes, publicScriptUrl, loading, error: apiError, refetch } = usePublisher();
  const direct = code?.format === "direct_link";
  const current = adCodes.find((item) => item.ad_code_id === code?.ad_code_id) ?? code;
  const [copied, setCopied] = useState(false),
    [diagnostics, setDiagnostics] = useState(false),
    [copyError, setCopyError] = useState(false);
  useEffect(() => {
    setCopied(false);
    setDiagnostics(false);
    setCopyError(false);
  }, [code?.ad_code_id]);
  let snippet = "",
    error = direct ? null : apiError;
  if (current?.format === "direct_link") {
    try {
      snippet = publisherDirectLink(current.direct_link_url);
    } catch {
      error = new ApiError(200, "Missing or invalid direct_link_url", "PUBLISHER_DIRECT_LINK");
    }
  } else if (code && publicScriptUrl && !error) {
    try {
      snippet = buildJsSnippet(code.short_code, publicScriptUrl);
    } catch {
      error = new ApiError(
        200,
        "Invalid public_script_url returned by the backend",
      );
    }
  }
  if (code && !direct && !loading && !publicScriptUrl && !error)
    error = new ApiError(200, "The backend did not provide public_script_url");
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(snippet);
      setCopied(true);
      setCopyError(false);
      toast.success(direct
        ? tr(lang, "Ссылка скопирована", "Link copied", "直链已复制")
        : tr(lang, "Код скопирован", "Code copied", "代码已复制"));
    } catch {
      setCopyError(true);
    }
  };
  return (
    <Dialog open={!!code} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto border-border bg-card">
        <DialogHeader className="pr-7 text-left">
          <DialogTitle>
            {direct ? "Direct Link" : tr(
              lang,
              "Установка рекламного кода",
              "Install ad code",
              "安装广告代码",
            )}
          </DialogTitle>
          <DialogDescription>
            {code?.name} · {publisherFormatLabel(code?.format ?? "")}
          </DialogDescription>
        </DialogHeader>
        {error ? (
          <PublisherError error={error} retry={refetch} />
        ) : !snippet ? (
          <p role="status">
            {tr(
              lang,
              "Загрузка кода…",
              "Loading code…",
              "正在加载代码…",
            )}
          </p>
        ) : (
          <Card className="space-y-4 p-4">
            <p className="text-sm">
              {direct ? tr(lang,
                "Скопируйте ссылку и разместите её на кнопке или другом элементе. Переход открывает рекламную страницу. JS-код не нужен.",
                "Copy the link and place it on a button or another element. Following it opens an ad page. No JavaScript is required.",
                "复制直链并放置在按钮或其他元素上。点击直链会打开广告页面，无需安装 JavaScript。"
              ) : tr(
                lang,
                "Добавьте тег один раз перед </head> или </body> в шаблон страниц, где должен работать этот код. Скопируйте его целиком, без изменений. Popunder открывается после выбранного клика пользователя, с учётом устройств, интервала и лимита показов.",
                "Add the tag once before </head> or </body> in the template of pages where this code should run. Copy the entire tag without changes. Popunder opens after the selected user click, subject to device targeting, interval and serving limits.",
                "在需要运行此代码的页面模板中，将标签添加一次，放在 </head> 或 </body> 之前。请完整复制标签，不要修改。Popunder 会在指定的用户点击后打开，并遵循设备、间隔和展示次数限制。",
              )}
            </p>
            <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-4 text-xs">
              <code>{snippet}</code>
            </pre>
            <Button onClick={() => void copy()}>
              {copied ? (
                <Check className="mr-2 h-4 w-4" />
              ) : (
                <Copy className="mr-2 h-4 w-4" />
              )}
              {direct
                ? tr(lang, "Копировать ссылку", "Copy link", "复制直链")
                : tr(lang, "Копировать код", "Copy code", "复制代码")}
            </Button>
            {!direct && (
              <>
                <section aria-label={tr(lang, "Что произойдёт после установки", "What happens after installation", "安装后会发生什么")} className="rounded-xl border border-primary/20 bg-primary/5 p-4 text-sm">
                  <h3 className="font-medium">{tr(lang, "Что произойдёт после установки", "What happens after installation", "安装后会发生什么")}</h3>
                  <p className="mt-1 leading-relaxed text-muted-foreground">
                    {tr(lang,
                      "После первого рекламного запроса TwinBid определит домен: сайт и зона появятся во вкладке «Сайты» автоматически. На одном домене каждому рекламному коду соответствует одна зона. Этот же код на другом домене относится к другому сайту и зоне.",
                      "After the first ad request, TwinBid detects the domain and the site and zone appear automatically under Sites. Each ad code has one zone per domain. The same code on another domain belongs to a different site and zone.",
                      "首次广告请求后，TwinBid 会识别域名，网站和广告位将自动显示在“网站”页面。同一域名上的每个广告代码对应一个广告位。同一代码在其他域名上会归属于另一个网站和广告位。")}
                  </p>
                </section>
                <PublisherPlacementNotice context="installation" />
                <details className="rounded-xl border border-border p-4 text-sm">
                  <summary className="cursor-pointer font-medium">{tr(lang, "Если реклама не открывается", "If ads do not open", "广告无法打开时")}</summary>
                  <ul className="mt-3 list-disc space-y-2 pl-5 leading-relaxed text-muted-foreground">
                    <li>{tr(lang,
                      "Проверьте, что код активен, установлен на нужных страницах и скопирован без изменений.",
                      "Check that the code is active, installed on the intended pages and copied without changes.",
                      "请检查代码是否已启用、安装在正确的页面上，并且完整复制且未修改。")}</li>
                    <li>{tr(lang,
                      "Выполните выбранный первый или второй клик. Учитывайте выбранные устройства, интервал и лимит показов за сессию страницы.",
                      "Perform the selected first or second click. Check device targeting, the interval and the opening limit per page session.",
                      "请执行设置的第一次或第二次点击，并检查设备、展示间隔及每个页面会话的展示上限。")}</li>
                    <li>{tr(lang,
                      "Блокировщик рекламы и настройки браузера могут мешать загрузке кода или открытию Popunder.",
                      "Ad blockers and browser settings may prevent the code from loading or Popunder from opening.",
                      "广告拦截器或浏览器设置可能阻止代码加载或 Popunder 打开。")}</li>
                    <li>{tr(lang,
                      "При установке внутри iframe проверьте передачу адреса основной страницы: домен iframe может отличаться от домена сайта. Ограничения iframe могут блокировать открытие окна.",
                      "For an iframe installation, check how the top-level page URL is passed: the iframe domain may differ from the website domain. Iframe restrictions may prevent the window from opening.",
                      "在 iframe 中安装时，请检查是否传递了顶层页面地址：iframe 域名可能不同于网站域名。iframe 的限制还可能阻止打开窗口。")}</li>
                    <li>{tr(lang,
                      "Откройте «Диагностика установки». Её данные обновляются каждые 5 минут, поэтому новый запрос может появиться с задержкой.",
                      "Open Installation diagnostics. Its data updates every 5 minutes, so a new request may appear after a delay.",
                      "请打开“安装诊断”。数据每 5 分钟更新一次，因此新请求可能会延迟显示。")}</li>
                  </ul>
                </details>
              </>
            )}
          </Card>
        )}
        {copyError && (
          <p role="alert" className="text-sm text-destructive">
            {direct ? tr(lang,
              "Браузер не разрешил копирование. Выделите ссылку и скопируйте вручную.",
              "The browser blocked clipboard access. Select the link and copy it manually.",
              "浏览器阻止了剪贴板访问，请选中直链并手动复制。"
            ) : tr(
              lang,
              "Браузер не разрешил копирование. Выделите код и скопируйте вручную.",
              "The browser blocked clipboard access. Select the code and copy it manually.",
              "浏览器阻止了剪贴板访问，请选中代码并手动复制。",
            )}
          </p>
        )}
        {!direct && (
          <>
            <Button
              variant="outline"
              onClick={() => setDiagnostics((value) => !value)}
            >
              {tr(
                lang,
                "Диагностика установки",
                "Installation diagnostics",
                "安装诊断",
              )}
            </Button>
            {diagnostics && code && (
              <PublisherDiagnostics codeId={code.ad_code_id} />
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
