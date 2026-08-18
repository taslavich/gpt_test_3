import { useEffect, useRef } from "react";
import { useLocation } from "react-router-dom";
import { YANDEX_METRIKA_ID } from "@/lib/yandexMetrika";

export function YandexMetrikaTracker() {
  const location = useLocation();
  const isInitialPage = useRef(true);
  const previousUrl = useRef(document.referrer);

  useEffect(() => {
    const currentUrl = window.location.href;

    if (isInitialPage.current) {
      isInitialPage.current = false;
      previousUrl.current = currentUrl;
      return;
    }

    window.ym?.(YANDEX_METRIKA_ID, "hit", currentUrl, {
      referer: previousUrl.current,
      title: document.title,
    });

    previousUrl.current = currentUrl;
  }, [location.pathname, location.search, location.hash]);

  return null;
}
