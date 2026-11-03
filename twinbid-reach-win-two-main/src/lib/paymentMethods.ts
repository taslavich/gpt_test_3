export const PAYMENT_METHODS = [
  {
    id: "usdc_erc20",
    label: "USDC (ERC-20)",
    desc: "USD Coin on Ethereum",
    address: "0xED961471A377a998df31191A7277006Aa0b04186",
    currency: "usdc",
  },
  {
    id: "usdt_trc20",
    label: "USDT (TRC-20)",
    desc: "Tether on Tron",
    address: "TJr26CGefYQAQ5ryrxETrQPeLYdzmz52ad",
    currency: "usdt",
  },
  {
    id: "usdt_erc20",
    label: "USDT (ERC-20)",
    desc: "Tether on Ethereum",
    address: "0xED961471A377a998df31191A7277006Aa0b04186",
    currency: "usdt",
  },
] as const;

export function getPaymentCurrency(methodId: string): "usdc" | "usdt" {
  return PAYMENT_METHODS.find(method => method.id === methodId)?.currency ?? "usdt";
}
