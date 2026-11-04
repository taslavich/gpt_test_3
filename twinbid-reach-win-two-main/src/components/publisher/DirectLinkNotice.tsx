import { AlertTriangle } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import { buildDirectLinkSnippet } from "@/lib/publisherSnippets";

export function DirectLinkNotice({ html }: { html?: string }) {
  const { lang } = useLanguage();
  const label = tr(lang, "Продолжить", "Continue", "继续");
  return (
    <section role="alert" className="flex min-w-0 gap-3 rounded-xl border-2 border-amber-500/60 bg-amber-500/10 p-4 text-sm">
      <AlertTriangle aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-amber-700 dark:text-amber-300" />
      <div className="min-w-0 space-y-3">
        <h3 className="font-semibold">
          {tr(lang, "Важно: Direct Link требует referrerpolicy=\"origin\"", "Important: Direct Link requires referrerpolicy=\"origin\"", "重要：直链需要 referrerpolicy=\"origin\"")}
        </h3>
        <p>{tr(lang,
          "Добавьте ссылку на кнопку или текст на своём сайте. Скрипт устанавливать не нужно.",
          "Add the link to a button or text on your site. No script is required.",
          "将直链添加到您网站的按钮或文本上，无需安装脚本。")}</p>
        <pre className="whitespace-pre-wrap break-all rounded-lg bg-background/70 p-3 text-xs"><code>
          {html ?? buildDirectLinkSnippet("https://carloverrcvbg.site/go/tb_123", label)}
        </code></pre>
        <ol className="list-decimal space-y-1.5 pl-5">
          {!html && <li>{tr(lang,
            "Замените пример в href на свою ссылку из кабинета. Оставьте referrerpolicy=\"origin\".",
            "Replace the example href with your dashboard link. Keep referrerpolicy=\"origin\".",
            "将 href 中的示例替换为控制台中的直链，并保留 referrerpolicy=\"origin\"。")}</li>}
          <li>{tr(lang,
            "Не добавляйте rel=\"noreferrer\": он скрывает домен сайта.",
            "Do not add rel=\"noreferrer\": it hides your site’s domain.",
            "不要添加 rel=\"noreferrer\"，它会隐藏您网站的域名。")}</li>
          <li>{tr(lang,
            "Проверяйте кликом со страницы своего сайта. Если вставить URL в адресную строку, Referer не передастся и аукцион не запустится.",
            "Test by clicking from a page on your site. Pasting the URL into the address bar sends no Referer, so the auction will not start.",
            "请在自己网站的页面上点击测试。直接将网址粘贴到地址栏不会发送 Referer，竞价也不会启动。")}</li>
        </ol>
      </div>
    </section>
  );
}
