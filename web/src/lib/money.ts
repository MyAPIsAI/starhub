export type NanoString = `${bigint}`;
export type MoneyMode = "balance" | "total" | "call" | "detail";
const digits: Record<MoneyMode, number> = {
  balance: 2,
  total: 4,
  call: 6,
  detail: 9,
};

export function formatMoney(
  value: string,
  mode: MoneyMode = "balance",
  language = typeof document === "undefined"
    ? "en"
    : document.documentElement.lang,
): string {
  const n = BigInt(value);
  const absolute = n < 0n ? -n : n;
  if (mode === "call" && absolute > 0n && absolute < 1000n) {
    if (language === "ar")
      return `\u2066${n < 0n ? "−" : ""}<${formatMoney("1000", "call", "ar").replace(/[\u2066\u2069]/g, "")}\u2069`;
    return n < 0n ? "−<$0.000001" : "<$0.000001";
  }
  const places = digits[mode];
  const divisor = 10n ** BigInt(9 - places);
  const rounded = (absolute + divisor / 2n) / divisor;
  const base = 10n ** BigInt(places);
  if (language === "ar") {
    // Format integer parts without converting exact ledger amounts to floating point.
    const parts = new Intl.NumberFormat("ar-AE-u-nu-latn", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: places,
      maximumFractionDigits: places,
    }).formatToParts(rounded / base);
    const localized = parts
      .map((part) =>
        part.type === "fraction"
          ? (rounded % base).toString().padStart(places, "0")
          : part.value.replace(/[\u200e\u200f\u061c]/g, ""),
      )
      .join("");
    return `\u2066${n < 0n ? "−" : ""}${localized}\u2069`;
  }
  const whole = (rounded / base)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const amount = `${n < 0n ? "−" : ""}$${whole}.${(rounded % base).toString().padStart(places, "0")}`;
  return amount;
}

export function sumNano(values: string[]): NanoString {
  return values
    .reduce((sum, value) => sum + BigInt(value), 0n)
    .toString() as NanoString;
}

export function usdToNano(usd: number | string): string {
  const num = typeof usd === "string" ? parseFloat(usd) : usd;
  if (isNaN(num)) return "0";
  const nano = BigInt(Math.round(num * 1_000_000_000));
  return nano.toString();
}

export function nanoToUsdNumber(nano: string): number {
  return Number(BigInt(nano)) / 1_000_000_000;
}
