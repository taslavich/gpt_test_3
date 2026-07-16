## Goal

Extend the existing "Columns" popover in the Statistics table so the user can hide/show **every** metric column, not just CPM / CPC / Confirmed. The first column (grouping label — Date, Country, Campaign, etc.) stays always visible as an anchor.

## Toggleable columns

Grouped exactly like the existing header groups:

- **Traffic**: Impressions, Clicks, CTR
- **Cost**: Spent, CPM, CPC
- **Conversions** (only when conversion mode is on): Conversions, Confirmed conversions, CR, Income, Confirmed income, ROI

All checkboxes default to **on**. State is persisted in `StatisticsContext` alongside the current `showCpm` / `showCpc` / `showConfirmedConversions` / `showConfirmedIncome` flags so it survives navigation, just like the existing ones.

## UX

- Same "Columns" popover next to the "Rows" selector (no new button).
- Popover content becomes a grouped list with small group headings (Traffic / Cost / Conversions) matching the table's group header, so users map checkbox → column visually.
- Conversion-group checkboxes stay disabled/greyed when conversion mode is off (same pattern as today's Confirmed toggles).
- Reset guard: if a user unchecks every column in a group, that group's header cell simply collapses (colspan recomputed). If literally all metric columns are hidden, the table still shows the sticky label column plus totals row for that label — no crash.
- Column group header row hides a group entirely when all its columns are off.

## Scope (frontend only)

### 1. `src/contexts/StatisticsContext.tsx`
Add persisted boolean flags + setters (all default `true`):
`showImpressions`, `showClicks`, `showCtr`, `showSpent`, `showConversionsCol`, `showCr`, `showIncome`, `showRoi`.
(Existing `showConversions` stays as the conversion-mode master switch; the new `showConversionsCol` toggles only the "Conversions" count column within that mode.)

### 2. `src/pages/DashboardStatistics.tsx`
- Read the new flags from context.
- Gate each `<th>` and `<td>` (data rows + totals row) on its flag, matching the pattern already used for CPM/CPC/Confirmed.
- Recompute `costCols` / `trafficCols` / `convCols` colspans from the flags so the group header row stays aligned; skip a group header cell when its count is 0.
- Extend the "Columns" popover with the new checkboxes, grouped visually:
  ```
  Traffic
    ☑ Impressions
    ☑ Clicks
    ☑ CTR
  Cost
    ☑ Spent
    ☑ CPM
    ☑ CPC
  Conversions            (disabled group when conversion mode off)
    ☑ Conversions
    ☑ Confirmed conv.
    ☑ CR
    ☑ Income
    ☑ Confirmed income
    ☑ ROI
  ```
- CSV export: keep exporting all metrics regardless of visibility (visibility is a view preference, not a data filter). If you'd prefer CSV to mirror visible columns, say so and I'll switch it.

### 3. Translations — `src/contexts/LanguageContext.tsx` + `src/lib/translations-es.ts`
Add short group-label keys for the popover if not already present: `stats.groupTraffic`, `stats.groupCost`, `stats.groupConversions` (already exist per plan.md — reuse). No new labels needed for individual metrics — reuse existing `stats.impressions`, `stats.clicks`, `stats.ctr`, `stats.spent`, `stats.conversions`, `stats.cr`, `stats.income`, `stats.roi`.

## Out of scope

- Backend, sorting behaviour, KPI cards, chart. Hiding a column doesn't remove it from sort options or from the chart metric selector.
- Per-user server-side persistence — flags live in context/memory like the current ones.
