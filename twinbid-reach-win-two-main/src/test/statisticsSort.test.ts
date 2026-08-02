import { describe, expect, it } from "vitest";
import { sortStatisticRowsByLabel } from "@/lib/statisticsSort";

describe("statistics chronological sorting", () => {
  it("sorts dates correctly across month boundaries", () => {
    const rows = [
      { label: "31.07.2026", sortValue: Date.parse("2026-07-31T00:00:00Z") },
      { label: "02.08.2026", sortValue: Date.parse("2026-08-02T00:00:00Z") },
      { label: "01.08.2026", sortValue: Date.parse("2026-08-01T00:00:00Z") },
    ];

    expect(sortStatisticRowsByLabel(rows, "desc").map(row => row.label)).toEqual([
      "02.08.2026",
      "01.08.2026",
      "31.07.2026",
    ]);
  });

  it("sorts hours correctly across day and month boundaries", () => {
    const rows = [
      { label: "31.07.2026 23:00", sortValue: Date.parse("2026-07-31T23:00:00Z") },
      { label: "01.08.2026 01:00", sortValue: Date.parse("2026-08-01T01:00:00Z") },
      { label: "01.08.2026 00:00", sortValue: Date.parse("2026-08-01T00:00:00Z") },
    ];

    expect(sortStatisticRowsByLabel(rows, "asc").map(row => row.label)).toEqual([
      "31.07.2026 23:00",
      "01.08.2026 00:00",
      "01.08.2026 01:00",
    ]);
  });
});
