import type { ListMode, TargetingState } from "@/contexts/CampaignContext";

export type TrafficCleanerMode = Exclude<ListMode, "none">;

export interface TrafficCleanerRow {
  label: string;
  spent: number;
  conversions: number;
}

export function selectUnconvertedSiteIds(
  rows: TrafficCleanerRow[],
  minimumSpend: number,
  excludedSiteIds: Iterable<string> = [],
): string[] {
  const threshold = Number.isFinite(minimumSpend) ? Math.max(0, minimumSpend) : 0;
  const excluded = new Set(Array.from(excludedSiteIds, (siteId) => siteId.trim()).filter(Boolean));
  return Array.from(new Set(
    rows
      .filter((row) => {
        const siteId = row.label.trim();
        return siteId && !excluded.has(siteId) && row.conversions <= 0 && row.spent > threshold;
      })
      .map((row) => row.label.trim()),
  ));
}

export function buildTrafficCleanerTargeting(
  current: TargetingState | undefined,
  selectedSiteIds: Iterable<string>,
  mode: TrafficCleanerMode,
): { next: TargetingState; replacesExisting: boolean; addedCount: number } {
  const selected = Array.from(new Set(
    Array.from(selectedSiteIds, (siteId) => siteId.trim()).filter(Boolean),
  ));
  const currentItems = Array.from(new Set((current?.items ?? []).map((siteId) => siteId.trim()).filter(Boolean)));
  const replacesExisting = currentItems.length > 0 && current?.mode !== mode;
  const newItems = replacesExisting
    ? selected
    : selected.filter((siteId) => !currentItems.includes(siteId));

  return {
    next: {
      mode,
      items: replacesExisting
        ? newItems
        : [...currentItems, ...newItems],
    },
    replacesExisting,
    addedCount: newItems.length,
  };
}
