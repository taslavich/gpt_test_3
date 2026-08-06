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

export function formatStatisticSpend(value: number): string {
  const truncated = Math.trunc(value * 100) / 100;
  return formatNumberWithDot(truncated, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

export function formatCurrencyAmount(value: number): string {
  if (!Number.isFinite(value)) return "0.00";
  return value.toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}
