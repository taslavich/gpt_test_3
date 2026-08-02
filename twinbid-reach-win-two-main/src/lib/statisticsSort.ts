export type StatisticSortDirection = "asc" | "desc";

export function sortStatisticRowsByLabel<T extends { label: string; sortValue?: number }>(
  rows: T[],
  direction: StatisticSortDirection,
): T[] {
  return [...rows].sort((left, right) => {
    const leftValue = left.sortValue;
    const rightValue = right.sortValue;

    if (Number.isFinite(leftValue) && Number.isFinite(rightValue)) {
      return direction === "asc"
        ? (leftValue as number) - (rightValue as number)
        : (rightValue as number) - (leftValue as number);
    }

    return direction === "asc"
      ? left.label.localeCompare(right.label)
      : right.label.localeCompare(left.label);
  });
}
