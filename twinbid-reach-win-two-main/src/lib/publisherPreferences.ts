export const PAYOUT_METHODS = [
  { value: "usdt_trc20", label: "USDT TRC20" },
  { value: "usdt_erc20", label: "USDT ERC20" },
] as const;
export type PayoutMethod = (typeof PAYOUT_METHODS)[number]["value"];
