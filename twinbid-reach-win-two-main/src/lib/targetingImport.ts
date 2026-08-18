import type { TargetingState } from "@/contexts/CampaignContext";

export const TARGETING_IMPORT_KEYS = [
  "country",
  "language",
  "deviceType",
  "os",
  "browser",
  "schedule",
  "sites",
  "ip",
] as const;

export type TargetingImportKey = typeof TARGETING_IMPORT_KEYS[number];

export function getImportableTargetingKeys(
  targeting: Record<string, TargetingState> | undefined,
): TargetingImportKey[] {
  if (!targeting) return [];
  return TARGETING_IMPORT_KEYS.filter(key => (targeting[key]?.items?.length ?? 0) > 0);
}

export function importTargetingGroups(
  current: Record<string, TargetingState>,
  source: Record<string, TargetingState>,
  selectedKeys: Iterable<TargetingImportKey>,
): Record<string, TargetingState> {
  const next = { ...current };
  for (const key of selectedKeys) {
    const sourceList = source[key];
    if (!sourceList) continue;
    next[key] = {
      mode: sourceList.mode,
      items: Array.from(new Set(sourceList.items.map(String))),
    };
  }
  return next;
}
