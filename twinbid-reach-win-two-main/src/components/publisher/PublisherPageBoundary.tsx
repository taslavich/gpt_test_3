import { Component, Suspense, useEffect, useState, type ReactNode } from "react";
import { useLocation } from "react-router-dom";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr } from "@/lib/publisherUiText";
import { Button } from "@/components/ui/button";

function PageLoading() {
  const { lang } = useLanguage();
  const [slow, setSlow] = useState(false);
  useEffect(() => {
    const timer = window.setTimeout(() => setSlow(true), 10000);
    return () => window.clearTimeout(timer);
  }, []);
  return <div role="status" className="space-y-3 rounded-xl border border-border p-5 text-sm">
    <p>{slow ? tr(lang, "Раздел загружается дольше обычного. Навигация остаётся доступной.", "This section is taking longer to load. Navigation is still available.", "此部分加载时间较长，您仍可使用导航。") : tr(lang, "Загрузка раздела…", "Loading section…", "正在加载此部分…")}</p>
    {slow && <Button variant="outline" onClick={() => window.location.reload()}>{tr(lang, "Обновить страницу", "Reload page", "刷新页面")}</Button>}
  </div>;
}
class PageError extends Component<{ children: ReactNode; fallback: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() { return this.state.failed ? this.props.fallback : this.props.children; }
}
export function PublisherPageBoundary({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  const { lang } = useLanguage();
  return <PageError key={pathname} fallback={<div role="alert" className="space-y-3 rounded-xl border border-destructive/40 p-5 text-sm">
    <p>{tr(lang, "Не удалось открыть раздел. Обновите страницу или выберите другой раздел.", "Could not open this section. Reload the page or choose another section.", "无法打开此部分，请刷新页面或选择其他部分。")}</p>
    <Button variant="outline" onClick={() => window.location.reload()}>{tr(lang, "Обновить страницу", "Reload page", "刷新页面")}</Button>
  </div>}><Suspense fallback={<PageLoading />}>{children}</Suspense></PageError>;
}
