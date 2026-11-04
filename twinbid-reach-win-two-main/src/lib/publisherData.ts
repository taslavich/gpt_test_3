import { ApiError } from "@/api/http";
export const publisherLocale = (lang: string) =>
  lang === "ru" ? "ru-RU" : lang === "zh-CN" ? "zh-CN" : "en-US";
function decimal(value: string) {
  const m = /^(-?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(value);
  if (!m || value.length > 100 || Math.abs(Number(m[4] || 0)) > 100)
    throw new Error("Invalid decimal string from backend");
  const digits = m[2] + (m[3] || ""),
    point = m[2].length + Number(m[4] || 0);
  const integer =
    point < 1
      ? "0"
      : (
          digits.slice(0, point) +
          "0".repeat(Math.max(0, point - digits.length))
        ).replace(/^0+(?=\d)/, "");
  const fraction = (
    point < 0
      ? "0".repeat(-point) + digits
      : point === 0
        ? digits
        : digits.slice(point)
  ).replace(/0+$/, "");
  return { negative: m[1] === "-", integer, fraction };
}
export function comparePublisherMoney(a: string, b: string): number {
  const x = decimal(a),
    y = decimal(b),
    places = Math.max(x.fraction.length, y.fraction.length);
  const toInt = (v: typeof x) =>
    (v.negative ? -1n : 1n) *
    BigInt(v.integer + v.fraction.padEnd(places, "0"));
  const left = toInt(x),
    right = toInt(y);
  return left < right ? -1 : left > right ? 1 : 0;
}
export function formatPublisherMoney(value: string, lang: string): string {
  const n = decimal(value),
    locale = publisherLocale(lang),
    separator = new Intl.NumberFormat(locale)
      .formatToParts(1.1)
      .find((p) => p.type === "decimal")!.value;
  return (
    (n.negative ? "-" : "") +
    new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(
      BigInt(n.integer),
    ) +
    separator +
    n.fraction.padEnd(2, "0")
  );
}
export function normalizePublisherAmount(value: string): string {
  const raw = value.trim().replace(",", ".");
  if (
    raw.length > 32 ||
    !/^\d+(?:\.\d{1,8})?$/.test(raw) ||
    comparePublisherMoney(raw, "0") <= 0
  )
    throw new ApiError(
      400,
      "Enter a positive amount with at most 8 decimal places",
    );
  const n = decimal(raw);
  return n.integer + (n.fraction ? "." + n.fraction : "");
}
export function publisherTimezone(value?: string | null): string {
  let tz = value || "UTC";
  if (tz === "utc_m5") tz = "Etc/GMT+5";
  else if (tz === "utc_5_30") tz = "Asia/Kolkata";
  else if (/^utc_-?\d+$/.test(tz)) {
    const offset = Number(tz.slice(4));
    tz =
      offset === 0
        ? "UTC"
        : `Etc/GMT${offset > 0 ? "-" : "+"}${Math.abs(offset)}`;
  }
  try {
    new Intl.DateTimeFormat("en", { timeZone: tz }).format();
  } catch {
    throw new ApiError(400, "Invalid timezone: " + tz);
  }
  return tz;
}
/** Display UTC notation while retaining IANA identifiers in API requests. */
export function publisherTimezoneLabel(timezone = "UTC", instant: Date = new Date()): string {
  const zone = new Intl.DateTimeFormat("en", { timeZone: publisherTimezone(timezone), timeZoneName: "shortOffset" }).formatToParts(instant).find(part => part.type === "timeZoneName")?.value || "GMT";
  return zone.replace(/^GMT/, "UTC");
}
export function publisherDate(
  value: string | null | undefined,
  lang: string,
  timezone = "UTC",
) {
  if (!value) return "—";
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "—";
  return new Intl.DateTimeFormat(publisherLocale(lang), {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(date) + " " + publisherTimezoneLabel(timezone, date);
}
function midnight(day: string, timezone: string): number {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) throw new ApiError(400, "Invalid date");
  const [y, m, d] = day.split("-").map(Number),
    target = Date.UTC(y, m - 1, d);
  if (new Date(target).toISOString().slice(0, 10) !== day)
    throw new ApiError(400, "Invalid date");
  let instant = target;
  const formatter = new Intl.DateTimeFormat("en-GB", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  });
  for (let i = 0; i < 5; i++) {
    const p = Object.fromEntries(
      formatter.formatToParts(instant).map((part) => [part.type, part.value]),
    );
    const local = Date.UTC(
      Number(p.year),
      Number(p.month) - 1,
      Number(p.day),
      Number(p.hour),
      Number(p.minute),
      Number(p.second),
    );
    const diff = target - local;
    if (diff === 0) return instant;
    instant += diff;
  }
  throw new ApiError(
    400,
    "The selected local date has no midnight in this timezone",
  );
}
export function publisherPeriod(first: string, last: string, timezone: string) {
  const tz = publisherTimezone(timezone),
    next = new Date(last + "T00:00:00Z");
  if (!Number.isFinite(next.getTime()))
    throw new ApiError(400, "Invalid period");
  next.setUTCDate(next.getUTCDate() + 1);
  const from = midnight(first, tz),
    to = midnight(next.toISOString().slice(0, 10), tz);
  if (to <= from || to - from > 366 * 86400000)
    throw new ApiError(400, "Period must be positive and at most 366 days");
  return { from: new Date(from).toISOString(), to: new Date(to).toISOString() };
}
export function publisherErrorText(error: unknown, lang: string) {
  const languageIndex = lang === "ru" ? 0 : lang === "zh-CN" ? 2 : 1;
  if (error instanceof ApiError) {
    const details: Array<[boolean, [string, string, string]]> = [
      [error.code === "PUBLISHER_DIRECT_LINK", ["Не удалось получить ссылку. Обновите данные и попробуйте снова.", "Could not retrieve the link. Refresh the data and try again.", "无法获取直链，请刷新数据后重试。"]],
      [error.code === "PAYOUT_STORAGE", ["Не удалось сохранить заявку. Проверьте настройки хранения данных в браузере.", "Could not save the request. Check your browser storage settings.", "无法保存申请，请检查浏览器存储设置。"]],
      [error.code === "PAYOUT_PENDING", ["Другая заявка на выплату ещё обрабатывается.", "Another payout request is still being processed.", "另一笔提现申请仍在处理中。"]],
      [error.code === "PAYOUT_BUSY", ["Заявка на выплату уже обрабатывается. Повторите попытку позже.", "A payout request is already being processed. Please try again later.", "提现申请正在处理中，请稍后重试。"]],
      [/insufficient available balance/i.test(error.message), ["Недостаточно средств для выплаты.", "Insufficient available balance for this payout.", "可提现余额不足。"]],
      [/positive amount with at most 8 decimal/i.test(error.message), ["Укажите сумму больше нуля, не более 8 знаков после запятой.", "Enter a positive amount with up to 8 decimal places.", "请输入大于零且最多有 8 位小数的金额。"]],
      [/^Invalid timezone:/.test(error.message), ["Проверьте часовой пояс.", "Check the timezone.", "请检查时区。"]],
      [/^frequency_minutes:/.test(error.message), ["Проверьте интервал и ограничение количества показов.", "Check the interval and display limit.", "请检查间隔和展示次数上限。"]],
    ];
    const matched = details.find(([condition]) => condition);
    if (matched) return matched[1][languageIndex];
    const localMessages = [
      "Минимум $50 в пределах доступного баланса", "Minimum $50 within the available balance", "最低 50 美元，且不得超过可用余额",
      "Выберите сеть и укажите адрес кошелька", "Select a network and enter a wallet address", "请选择网络并输入钱包地址",
      "Выберите сеть выплат", "Choose a payout network", "请选择提现网络",
    ];
    if (localMessages.includes(error.message)) return error.message;
  }
  const words: Record<number, [string, string, string]> = {
    0: [
      "Не удалось связаться с сервером. Проверьте подключение и повторите запрос.",
      "Cannot reach the server. Check your connection and retry.",
      "无法连接服务器，请检查网络后重试。",
    ],
    200: [
      "Сервер вернул некорректный ответ.",
      "The server returned an invalid response.",
      "服务器返回了无效响应。",
    ],
    400: [
      "Проверьте введённые данные.",
      "Check the entered values.",
      "请检查输入的数据。",
    ],
    401: [
      "Сессия истекла. Войдите снова.",
      "Your session expired. Sign in again.",
      "会话已过期，请重新登录。",
    ],
    403: [
      "Недостаточно прав для этого действия.",
      "You do not have permission for this action.",
      "您没有执行此操作的权限。",
    ],
    404: [
      "Объект не найден или недоступен этому аккаунту.",
      "This object was not found or is unavailable to this account.",
      "未找到该对象，或当前账户无法访问。",
    ],
    409: [
      "Данные изменились или операция конфликтует с текущим состоянием. Обновите данные.",
      "The data changed or the operation conflicts with the current state. Refresh the data.",
      "数据已更改或操作与当前状态冲突，请刷新数据。",
    ],
    503: [
      "Не удалось загрузить данные. Повторите попытку позже.",
      "Could not load data. Please try again later.",
      "无法加载数据，请稍后重试。",
    ],
  };
  const message =
    words[error instanceof ApiError ? error.status : 0] || words[503];
  return message[languageIndex];
}
