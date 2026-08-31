import type { CampaignTypeModel, PricingModel } from "@/api/types";

export function getCampaignPricingLabel(
  pricingModel: PricingModel,
  typeModel: CampaignTypeModel = 1,
): string {
  if (pricingModel === "cpm" && typeModel === 2) return "TwinBid CPM";
  return pricingModel.toUpperCase();
}
