import { createContext, useContext, useState, type ReactNode } from "react";
import type { DateRange } from "react-day-picker";

type GroupBy = "dates" | "hours" | "browsers" | "siteid" | "devices" | "os" | "country";
type ChartMetric = "impressions" | "clicks" | "spent";
type SortKey = "label" | "impressions" | "clicks" | "spent";
type SortDir = "asc" | "desc";

interface StatisticsState {
  selectedCampaignIds: Set<string>;
  setSelectedCampaignIds: React.Dispatch<React.SetStateAction<Set<string>>>;
  selectedCreativeIds: Set<string>;
  setSelectedCreativeIds: React.Dispatch<React.SetStateAction<Set<string>>>;
  dateRange: DateRange | undefined;
  setDateRange: React.Dispatch<React.SetStateAction<DateRange | undefined>>;
  clickCount: number;
  setClickCount: React.Dispatch<React.SetStateAction<number>>;
  filterCountry: string;
  setFilterCountry: React.Dispatch<React.SetStateAction<string>>;
  filterBrowser: string;
  setFilterBrowser: React.Dispatch<React.SetStateAction<string>>;
  filterDevice: string;
  setFilterDevice: React.Dispatch<React.SetStateAction<string>>;
  filterOS: string;
  setFilterOS: React.Dispatch<React.SetStateAction<string>>;
  groupBy: GroupBy;
  setGroupBy: React.Dispatch<React.SetStateAction<GroupBy>>;
  chartMetric: ChartMetric;
  setChartMetric: React.Dispatch<React.SetStateAction<ChartMetric>>;
  sortKey: SortKey;
  setSortKey: React.Dispatch<React.SetStateAction<SortKey>>;
  sortDir: SortDir;
  setSortDir: React.Dispatch<React.SetStateAction<SortDir>>;
  appliedCampaignIds: Set<string>;
  setAppliedCampaignIds: React.Dispatch<React.SetStateAction<Set<string>>>;
  appliedCreativeIds: Set<string>;
  setAppliedCreativeIds: React.Dispatch<React.SetStateAction<Set<string>>>;
  appliedDateRange: DateRange | undefined;
  setAppliedDateRange: React.Dispatch<React.SetStateAction<DateRange | undefined>>;
  appliedFilterCountry: string;
  setAppliedFilterCountry: React.Dispatch<React.SetStateAction<string>>;
  appliedFilterBrowser: string;
  setAppliedFilterBrowser: React.Dispatch<React.SetStateAction<string>>;
  appliedFilterDevice: string;
  setAppliedFilterDevice: React.Dispatch<React.SetStateAction<string>>;
  appliedFilterOS: string;
  setAppliedFilterOS: React.Dispatch<React.SetStateAction<string>>;
}

const StatisticsContext = createContext<StatisticsState | null>(null);

export function StatisticsProvider({ children }: { children: ReactNode }) {
  const [selectedCampaignIds, setSelectedCampaignIds] = useState<Set<string>>(new Set());
  const [selectedCreativeIds, setSelectedCreativeIds] = useState<Set<string>>(new Set());
  const [dateRange, setDateRange] = useState<DateRange | undefined>(undefined);
  const [clickCount, setClickCount] = useState(0);
  const [filterCountry, setFilterCountry] = useState("all");
  const [filterBrowser, setFilterBrowser] = useState("all");
  const [filterDevice, setFilterDevice] = useState("all");
  const [filterOS, setFilterOS] = useState("all");
  const [groupBy, setGroupBy] = useState<GroupBy>("dates");
  const [chartMetric, setChartMetric] = useState<ChartMetric>("impressions");
  const [sortKey, setSortKey] = useState<SortKey>("label");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [appliedCampaignIds, setAppliedCampaignIds] = useState<Set<string>>(new Set());
  const [appliedCreativeIds, setAppliedCreativeIds] = useState<Set<string>>(new Set());
  const [appliedDateRange, setAppliedDateRange] = useState<DateRange | undefined>(undefined);
  const [appliedFilterCountry, setAppliedFilterCountry] = useState("all");
  const [appliedFilterBrowser, setAppliedFilterBrowser] = useState("all");
  const [appliedFilterDevice, setAppliedFilterDevice] = useState("all");
  const [appliedFilterOS, setAppliedFilterOS] = useState("all");

  return (
    <StatisticsContext.Provider value={{
      selectedCampaignIds, setSelectedCampaignIds,
      selectedCreativeIds, setSelectedCreativeIds,
      dateRange, setDateRange,
      clickCount, setClickCount,
      filterCountry, setFilterCountry,
      filterBrowser, setFilterBrowser,
      filterDevice, setFilterDevice,
      filterOS, setFilterOS,
      groupBy, setGroupBy,
      chartMetric, setChartMetric,
      sortKey, setSortKey,
      sortDir, setSortDir,
      appliedCampaignIds, setAppliedCampaignIds,
      appliedCreativeIds, setAppliedCreativeIds,
      appliedDateRange, setAppliedDateRange,
      appliedFilterCountry, setAppliedFilterCountry,
      appliedFilterBrowser, setAppliedFilterBrowser,
      appliedFilterDevice, setAppliedFilterDevice,
      appliedFilterOS, setAppliedFilterOS,
    }}>
      {children}
    </StatisticsContext.Provider>
  );
}

export function useStatistics() {
  const ctx = useContext(StatisticsContext);
  if (!ctx) throw new Error("useStatistics must be used within StatisticsProvider");
  return ctx;
}
