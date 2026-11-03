import { AlertTriangle } from "lucide-react";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr } from "@/lib/publisherUiText";

// Original copy from the last version before the publisher API migration.
const notices = {
  "list": {
    "title": [
      "Разные места — разные коды",
      "Different placements need different codes",
      "不同展示位置需要不同代码"
    ],
    "text": [
      "Если одинаковый код разместить в нескольких местах одного сайта — например, два баннера одного размера или несколько Popunder, — они склеятся в одну зону. Это правило действует для всех форматов.",
      "If the same code is used in multiple locations on one website—such as two banners of the same size or several Popunders—they merge into one zone. This applies to every format.",
      "同一代码用于一个网站的多个位置（例如两个相同尺寸的横幅或多个弹出广告）时，会合并为一个广告位。这适用于所有形式。"
    ]
  },
  "form": {
    "title": [
      "Статистика мест размещения",
      "Placement statistics",
      "广告位统计"
    ],
    "text": [
      "Один код можно использовать несколько раз на одном сайте: данные этих мест объединятся в одну зону. Для раздельной статистики и управления создайте или продублируйте код.",
      "One code can be reused on a website: its placements share one zone. Create or duplicate a code for separate statistics and controls.",
      "同一代码可在网站上重复使用：这些位置共享一个广告位。若需分别统计和控制，请创建或复制代码。"
    ]
  },
  "installation": {
    "title": [
      "Повторное использование кода",
      "Reusing a code",
      "重复使用代码"
    ],
    "text": [
      "Один CODE можно разместить несколько раз, но на одном домене статистика этих мест объединится. Для раздельного учёта создайте или продублируйте код.",
      "You can place one CODE multiple times, but its placements on the same domain share statistics. Create or duplicate a code for separate reporting.",
      "同一个 CODE 可多次放置，但同一域名上的统计会合并。若需分别统计，请创建或复制代码。"
    ]
  }
} as const;

export function PublisherPlacementNotice({ context = "list" }: { context?: keyof typeof notices }) {
  const { lang } = useLanguage();
  const notice = notices[context];
  const title = tr(lang, notice.title[0], notice.title[1], notice.title[2]);
  return (
    <section aria-label={title} className="flex min-w-0 items-start gap-3 rounded-xl border border-amber-500/25 bg-amber-500/5 p-4 text-sm">
      <AlertTriangle aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-amber-700 dark:text-amber-300" />
      <div className="min-w-0">
        <h3 className="font-medium">{title}</h3>
        <p className="mt-1 leading-relaxed text-muted-foreground">{tr(lang, notice.text[0], notice.text[1], notice.text[2])}</p>
      </div>
    </section>
  );
}
