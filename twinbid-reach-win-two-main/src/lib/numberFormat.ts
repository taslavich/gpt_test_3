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

export function formatStatisticRate(value: number): string {
  const truncated = Math.trunc(value * 1000) / 1000;
  return formatNumberWithDot(truncated, {
    minimumFractionDigits: 3,
    maximumFractionDigits: 3,
  });
}
