import type { PricingModel, TrafficQuality } from "@/contexts/CampaignContext";

const FORMAT_BID_LIMITS: Record<string, Record<TrafficQuality, { min: number; recommended: number }>> = {
  banner: {
    common: { min: 0.01, recommended: 0.05 },
    high: { min: 0.01, recommended: 0.07 },
    ultra: { min: 0.01, recommended: 0.14 },
  },
  native: {
    common: { min: 0.01, recommended: 0.05 },
    high: { min: 0.01, recommended: 0.07 },
    ultra: { min: 0.01, recommended: 0.14 },
  },
  push: {
    common: { min: 0.005, recommended: 0.01 },
    high: { min: 0.005, recommended: 0.017 },
    ultra: { min: 0.005, recommended: 0.035 },
  },
  popunder: {
    common: { min: 0.3, recommended: 1.8 },
    high: { min: 0.7, recommended: 3.0 },
    ultra: { min: 0.9, recommended: 4.7 },
  },
};

export const CPC_FROM_CPM_MULTIPLIER = 1.7 / 1000;

export function getBidLimits(formatKey: string, quality: TrafficQuality, model: PricingModel) {
  const limits = (FORMAT_BID_LIMITS[formatKey] || FORMAT_BID_LIMITS.banner)[quality];
  if (model === "cpm" || formatKey === "push") return limits;
  return {
    min: Number((limits.min * CPC_FROM_CPM_MULTIPLIER).toFixed(5)),
    recommended: Number((limits.recommended * CPC_FROM_CPM_MULTIPLIER).toFixed(5)),
  };
}

export function getMaximumBid(formatKey: string, model: PricingModel) {
  if (model === "cpc") return 1;
  return formatKey === "popunder" ? 50 : 1000;
}

export function convertRecommendationToModel(value: number, formatKey: string, model: PricingModel) {
  if (model === "cpm" || formatKey === "push") return value;
  return value * CPC_FROM_CPM_MULTIPLIER;
}
