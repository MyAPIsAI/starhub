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
): string {
  const n = BigInt(value);
  const absolute = n < 0n ? -n : n;
  if (mode === "call" && absolute > 0n && absolute < 1000n)
    return n < 0n ? "−<$0.000001" : "<$0.000001";
  const places = digits[mode];
  const divisor = 10n ** BigInt(9 - places);
  const rounded = (absolute + divisor / 2n) / divisor;
  const base = 10n ** BigInt(places);
  const whole = (rounded / base)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return `${n < 0n ? "−" : ""}$${whole}.${(rounded % base).toString().padStart(places, "0")}`;
}

export function sumNano(values: string[]): NanoString {
  return values
    .reduce((sum, value) => sum + BigInt(value), 0n)
    .toString() as NanoString;
}

export function usdToNano(usd: number | string): string {
  if (typeof usd === "string") return decimalUsdToNano(usd);
  const num = usd;
  if (isNaN(num)) return "0";
  const nano = BigInt(Math.round(num * 1_000_000_000));
  return nano.toString();
}

export function nanoToUsdNumber(nano: string): number {
  return Number(BigInt(nano)) / 1_000_000_000;
}

export function exactNano(value: unknown): string {
  if (
    typeof value !== "string" ||
    value.length > 64 ||
    !/^(0|-?[1-9][0-9]*)$/.test(value)
  )
    throw Error("Invalid exact nano-USD amount");
  return value;
}

export function quotaMoneyNano(exact: unknown, legacy: number): string {
  if (exact !== undefined) return exactNano(exact);
  if (!Number.isSafeInteger(legacy)) throw Error("Unsafe legacy quota amount");
  return (BigInt(legacy) * 2000n).toString();
}

export function decimalUsdToNano(value: string): string {
  if (value.length > 64 || !/^-?[0-9]+(\.[0-9]{1,9})?$/.test(value))
    throw Error("invalid_amount");
  const negative = value.startsWith("-");
  const [whole, fraction = ""] = (negative ? value.slice(1) : value).split(".");
  const amount =
    BigInt(whole) * 1_000_000_000n + BigInt(fraction.padEnd(9, "0"));
  return (negative ? -amount : amount).toString();
}
