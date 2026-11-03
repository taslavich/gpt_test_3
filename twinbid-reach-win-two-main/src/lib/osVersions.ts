import type { TargetingState } from "@/contexts/CampaignContext";

type MobileOs = "iOS" | "Android";

// Observed and released major/minor versions. Exclude malformed user agents,
// Android API levels, macOS 10.15, iOS 19, and unverified iOS 26.7.
// Preserve the old major-only options so existing campaign targeting remains
// unchanged and advertisers can still select an entire major branch.
const OS_MINOR_RELEASES: Record<MobileOs, Readonly<Record<number, readonly (number | "L")[]>>> = {
  iOS: {
    27: [0, 2], 26: [0, 1, 2, 3, 4, 5, 6],
    18: [0, 1, 2, 3, 4, 5, 6, 7],
    17: [0, 1, 2, 3, 4, 5, 6, 7],
    16: [0, 1, 2, 3, 4, 5, 6, 7],
    15: [0, 1, 2, 3, 4, 5, 6, 7, 8],
    14: [0, 1, 2, 3, 4, 5, 6, 7, 8],
    13: [0, 1, 2, 3, 4, 5, 6, 7],
    12: [0, 1, 2, 3, 4, 5],
    11: [0, 1, 2, 3, 4],
    10: [0, 1, 2, 3],
    9: [0, 1, 2, 3],
    8: [0, 1, 2, 3, 4],
    7: [0, 1], 6: [0, 1], 5: [0, 1], 4: [2, 3], 3: [2],
  },
  Android: {
    17: [0], 16: [0], 15: [0], 14: [0], 13: [0],
    12: [0, "L"], 11: [0], 10: [0], 9: [0],
    8: [0, 1], 7: [0, 1], 6: [0], 5: [0, 1],
    4: [0, 1, 2, 3, 4], 2: [0, 2, 3], 1: [5, 6],
  },
};

export const osVersionKey = (os: MobileOs, major: number, minor?: number | "L") =>
  `${os} ${major}${minor === "L" ? "L" : minor === undefined ? "" : `.${minor}`}`;

export function availableOsVersions(os: TargetingState | undefined): string[] {
  if (os?.mode !== "white") return [];
  return (["iOS", "Android"] as const)
    .filter(name => os.items.includes(name))
    .flatMap(name => Object.entries(OS_MINOR_RELEASES[name])
      .sort(([a], [b]) => Number(b) - Number(a))
      .flatMap(([major, minors]) => [
        osVersionKey(name, Number(major)),
        ...[...minors].reverse().map(minor => osVersionKey(name, Number(major), minor)),
      ]));
}

/** A selected major release already covers every numeric minor release in that branch. */
export function normalizeSelectedOsVersions(items: string[]): string[] {
  const majors = new Set(items.filter(item => /^(iOS|Android) \d+$/.test(item)));
  return [...new Set(items)].filter(item => {
    const minor = /^(iOS|Android) (\d+)\.\d+$/.exec(item);
    return !minor || !majors.has(`${minor[1]} ${minor[2]}`);
  });
}

export function selectableOsVersions(os: TargetingState | undefined, selected: string[]): string[] {
  const majors = new Set(normalizeSelectedOsVersions(selected).filter(item => /^(iOS|Android) \d+$/.test(item)));
  return availableOsVersions(os).filter(item => {
    const minor = /^(iOS|Android) (\d+)\.\d+$/.exec(item);
    return !minor || !majors.has(`${minor[1]} ${minor[2]}`);
  });
}

export function normalizedOsVersionTargeting(
  targeting: Record<string, TargetingState>,
): TargetingState {
  const current = targeting.osVersion;
  const allowed = new Set(availableOsVersions(targeting.os));
  const items = normalizeSelectedOsVersions(current?.items.filter(item => allowed.has(item)) ?? []);
  return { mode: allowed.size ? current?.mode ?? "none" : "none", items };
}

/** Group exact patch releases by OS and the first digit after the dot. */
export function formatOsVersionGroup(raw: string): string {
  const value = raw.trim();
  const match = /^(iOS|Android)[| :]+(\d+)(?:\.(\d+))?(?:\..*)?$/i.exec(value);
  if (!match) {
    if (/^Android[| :]12L$/i.test(value)) return "Android 12L";
    return value.includes("|") ? value.split("|", 1)[0] : value;
  }
  const os: MobileOs = match[1].toLowerCase() === "ios" ? "iOS" : "Android";
  const major = Number(match[2]);
  const minor = match[3] === undefined ? 0 : Number(match[3]);
  return OS_MINOR_RELEASES[os][major]?.includes(minor)
    ? osVersionKey(os, major, minor)
    : os;
}
