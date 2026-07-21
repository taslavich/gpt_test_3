export function formatNumberWithDot(
  value: number,
  options: Intl.NumberFormatOptions = {},
): string {
  if (!Number.isFinite(value)) return "0";
  return value
    .toLocaleString("en-US", options)
    .replace(/,/g, "\u00a0");
}

export function formatStatisticInteger(value: number): string {
  return formatNumberWithDot(value, { maximumFractionDigits: 0 });
}
