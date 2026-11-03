export function publisherUiText(
  lang: string,
  ru: string,
  en: string,
  zh: string,
) {
  return lang === "ru" ? ru : lang === "zh-CN" ? zh : en;
}

export const trafficLabels = {
  adult: ["Adult", "Adult", "成人流量"],
  mainstream: ["Mainstream", "Mainstream", "普通流量"],
} as const;
