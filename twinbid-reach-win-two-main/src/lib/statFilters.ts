// Frontend grouping maps for statistics filters.
//
// The backend expects exact raw values (e.g. "Chromium", "JSChromeBrowser").
// The UI shows human-readable group keys (e.g. "Chrome"). Selection is expanded
// to raw values before the request; grouped result rows are collapsed back to
// group keys, and anything unknown falls into "other".

export const OTHER_KEY = "other";

export const BROWSER_FILTER_MAP: Record<string, string[]> = {
  "Chrome": [
    "Chrome", "Chromium", "Ubuntu Chromium", "Raspbian Chromium",
    "Kiwi Chrome", "Iron", "Comodo_Dragon", "JSChromeBrowser",
  ],
  "Safari": ["Safari"],
  "Edge": ["Edge"],
  "Samsung Browser": ["Samsung Browser"],
  "Firefox": ["Firefox"],
  "Opera": ["Opera", "Opera Touch", "Opera Mini"],
  "Yandex Browser": [
    "YaBrowser", "YaApp_Android", "YandexSearch", "YaSearchBrowser", "YaSearchApp",
  ],
  "Huawei / Honor Browser": ["Huawei Browser", "HonorBrowser"],
  "Miui Browser": ["Miui Browser"],
  "HeyTap Browser": ["HeyTapBrowser"],
  "Android Browser": ["Android browser"],
  "Internet Explorer / Trident": ["Internet Explorer", "Trident"],
  "UC Browser": ["UCBrowser", "UCMobile", "UCPC", "UCTurbo", "UBrowser"],
  "QQ / Tencent Browser": [
    "QQBrowser", "MQQBrowser", "Mobile MQQBrowser", "QQ", "Qzone", "QZONEJSSDK",
  ],
  "Quark": ["Quark", "QuarkPC"],
  "DuckDuckGo": ["DuckDuckGo", "Mobile DuckDuckGo"],
  "Brave": ["Brave"],
  "Vivaldi": ["Vivaldi"],
  "Yahoo / YJApp": [
    "YJApp-IOS jp.co.yahoo.ipn.appli",
    "YJApp-ANDROID jp.co.yahoo.android.yjtop",
    "YJApp-IOS jp.co.yahoo.yjtrend01",
    "YJApp-ANDROID jp.co.yahoo.android.ybrowser",
    "YahooSearch",
    "YnoteiOS",
  ],
  "Facebook App": ["Facebook App"],
  "Instagram App": ["Instagram App"],
  "TikTok / ByteDance": ["TikTok App", "bytedancewebview", "TTWebView"],
  "Twitter / X App": ["Twitter for iPhone"],
  "WeChat / WeCom": ["MicroMessenger", "wxwork"],
  "LINE App": ["Safari Line", "Line"],
  "Snapchat": ["Snapchat"],
  "Pinterest": ["Pinterest"],
  "DingTalk": ["DingTalk"],
  "KakaoTalk": ["KAKAOTALK"],
  "Zalo": ["Zalo iOS", "Zalo android"],
  "Douban": ["com.douban.frodo"],
  "Baidu": ["swan", "tieba", "haokan", "ZhihuHybrid DefaultBrowser com.zhihu.android"],
  "Privacy Browsers": [
    "Avast", "AVG", "Norton", "Avira", "CCleaner", "MacKeeper",
    "ADG", "Phantom", "Blue Proxy",
  ],
  "Smart TV": [
    "SmartTV", "Smart TV Build", "TV Bro", "TSBNetTV", "TeslaBrowser",
    "Tesla", "WebOS", "inext TV", "Changhong Andr0id TV Build",
  ],
};

export const OS_FILTER_MAP: Record<string, string[]> = {
  "iOS": ["iOS"],
  "Android": ["Android"],
  "Windows": ["Windows"],
  "macOS": ["macOS"],
  "Linux": ["Linux"],
  "ChromeOS": ["ChromeOS"],
  "Harmony": ["Harmony"],
  "BlackBerry": ["BlackBerry"],
  "FreeBSD": ["FreeBSD"],
  "Windows Phone": ["Windows Phone"],
};

export const DEVICE_FILTER_MAP: Record<string, string[]> = {
  "mobile": ["mobile"],
  "tablet": ["tablet"],
  "desktop": ["desktop"],
};

/** Reverse index: raw value → UI group key. */
function buildReverse(map: Record<string, string[]>): Map<string, string> {
  const r = new Map<string, string>();
  for (const [key, values] of Object.entries(map)) {
    for (const v of values) r.set(v, key);
  }
  return r;
}
export const BROWSER_REVERSE = buildReverse(BROWSER_FILTER_MAP);
export const OS_REVERSE = buildReverse(OS_FILTER_MAP);
export const DEVICE_REVERSE = buildReverse(DEVICE_FILTER_MAP);

/** Group keys + "other" for UI dropdowns. */
export const BROWSER_FILTER_KEYS = [...Object.keys(BROWSER_FILTER_MAP), OTHER_KEY];
export const OS_FILTER_KEYS = [...Object.keys(OS_FILTER_MAP), OTHER_KEY];
export const DEVICE_FILTER_KEYS = [...Object.keys(DEVICE_FILTER_MAP), OTHER_KEY];

/**
 * Expand selected UI keys into raw backend values.
 * Returns `null` when "other" is in the selection — the caller MUST omit the
 * filter for that dimension (we can't enumerate all unknown values) and, when
 * grouping by that dimension, post-filter rows client-side against the picked
 * keys after mapping raw → group.
 */
export function expandFilter(
  selected: Set<string> | string[],
  map: Record<string, string[]>,
): string[] | null {
  const arr = Array.isArray(selected) ? selected : Array.from(selected);
  if (arr.length === 0) return [];
  if (arr.includes(OTHER_KEY)) return null;
  return Array.from(new Set(arr.flatMap(k => map[k] ?? [])));
}

/** Raw value → UI group key. Unknown values fall into "other". */
export function mapRawToGroup(raw: string, reverse: Map<string, string>): string {
  return reverse.get(raw) ?? OTHER_KEY;
}
