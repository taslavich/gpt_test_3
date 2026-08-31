export const TWINBID_ESTIMATED_MARGIN = 0.2;
export const PARTNER_PROFIT_SHARE = 0.5;

export interface PartnerEarningsInput {
  monthlyAffiliatePayout: number;
  roiPercent: number;
  mediaBuyers: number;
}

export interface PartnerEarningsResult {
  trafficSpend: number;
  twinbidProfit: number;
  partnerIncome: number;
  annualPartnerIncome: number;
}

const finiteOrZero = (value: number) => Number.isFinite(value) ? value : 0;

export function calculatePartnerEarnings({
  monthlyAffiliatePayout,
  roiPercent,
  mediaBuyers,
}: PartnerEarningsInput): PartnerEarningsResult {
  const revenue = Math.max(0, finiteOrZero(monthlyAffiliatePayout));
  const roi = Math.max(0, finiteOrZero(roiPercent)) / 100;
  const buyers = Math.max(0, finiteOrZero(mediaBuyers));
  const trafficSpend = (revenue / (1 + roi)) * buyers;
  const twinbidProfit = trafficSpend * TWINBID_ESTIMATED_MARGIN;
  const partnerIncome = twinbidProfit * PARTNER_PROFIT_SHARE;

  return {
    trafficSpend,
    twinbidProfit,
    partnerIncome,
    annualPartnerIncome: partnerIncome * 12,
  };
}
