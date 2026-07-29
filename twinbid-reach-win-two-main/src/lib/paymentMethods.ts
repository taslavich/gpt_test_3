export const PAYMENT_METHODS = [
  {
    id: "usdc_erc20",
    label: "USDC (ERC-20)",
    desc: "USD Coin on Ethereum",
    address: "0xaE8c308b5dE66E9D4EC32297893bEF0053De4527",
    currency: "usdc",
  },
  {
    id: "usdt_trc20",
    label: "USDT (TRC-20)",
    desc: "Tether on Tron",
    address: "TMcMNrGaEmTPujLnVMZubfKiSQydC5kTfj",
    currency: "usdt",
  },
  {
    id: "usdt_erc20",
    label: "USDT (ERC-20)",
    desc: "Tether on Ethereum",
    address: "0xaE8c308b5dE66E9D4EC32297893bEF0053De4527",
    currency: "usdt",
  },
] as const;

export function getPaymentCurrency(methodId: string): "usdc" | "usdt" {
  return PAYMENT_METHODS.find(method => method.id === methodId)?.currency ?? "usdt";
}
