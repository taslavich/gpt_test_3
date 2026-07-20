import type { RecommendBidRequest } from "@/api";
import type { TargetingState, TrafficType } from "@/contexts/CampaignContext";

export interface BidRecommendation {
  /** Average historical bid returned by the backend. */
  minimumRecommended: number;
  /** Average historical bid multiplied by a stable factor generated per response. */
  optimalRecommended: number;
}

const readList = (lists: Record<string, TargetingState>, key: string) => {
  const list = lists[key];
  return {
    items: list?.items || [],
    mode: list?.mode === "black" ? "exclude" as const : "include" as const,
  };
};

export function buildRecommendBidRequest(
  formatType: string,
  trafficType: TrafficType,
  lists: Record<string, TargetingState>,
): RecommendBidRequest {
  const country = readList(lists, "country");
  const language = readList(lists, "language");
  const deviceType = readList(lists, "deviceType");
  const os = readList(lists, "os");
  const browser = readList(lists, "browser");

  return {
    format_type: formatType as RecommendBidRequest["format_type"],
    traffic_type: trafficType,
    country: country.items,
    country_mode: country.mode,
    language: language.items,
    language_mode: language.mode,
    device_type: deviceType.items,
    device_type_mode: deviceType.mode,
    os: os.items,
    os_mode: os.mode,
    browser: browser.items,
    browser_mode: browser.mode,
  };
}

export function makeBidRecommendation(averageBid: number): BidRecommendation | null {
  if (!Number.isFinite(averageBid) || averageBid <= 0) return null;
  const optimalMultiplier = 1.9 + Math.random() * 0.4;
  return {
    minimumRecommended: averageBid,
    optimalRecommended: averageBid * optimalMultiplier,
  };
}
