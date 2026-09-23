export const QUOTA_PER_USD = 500000;
export function integer(value: string): number {
  if (!/^-?\d+$/.test(value.trim())) throw new Error("Enter a whole number");
  const n = Number(value);
  if (!Number.isSafeInteger(n))
    throw new Error("Value exceeds the safe integer range");
  return n;
}
export function usdToQuota(value: string): number {
  if (!/^\d+(\.\d{1,6})?$/.test(value))
    throw new Error("Use a positive decimal with at most six places");
  const [whole, fraction = ""] = value.split(".");
  const micros = BigInt(whole!) * 1000000n + BigInt(fraction.padEnd(6, "0"));
  if (micros % 2n) throw new Error("Amount must be a multiple of 0.000002 USD");
  const result = Number(micros / 2n);
  if (!Number.isSafeInteger(result))
    throw new Error("Value exceeds the safe integer range");
  return result;
}
export function number(value: unknown, locale = "zh-CN") {
  return typeof value === "number" && Number.isSafeInteger(value)
    ? new Intl.NumberFormat(locale).format(value)
    : "—";
}
export function quota(value: unknown, locale = "zh-CN", unit = "usd") {
  if (typeof value !== "number" || !Number.isSafeInteger(value)) return "—";
  if (unit === "quota") return `${number(value, locale)} quota`;
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  }).format(value / QUOTA_PER_USD);
}
export function date(value: unknown, locale = "zh-CN") {
  return typeof value === "number" && value > 0
    ? new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(value * 1000)
    : "—";
}
export function fillDays(
  items: import("./types").DailyUsage[],
  days: number,
  now = new Date(),
) {
  const end = Date.UTC(
    now.getUTCFullYear(),
    now.getUTCMonth(),
    now.getUTCDate(),
  );
  const byDate = new Map(items.map((item) => [item.date, item]));
  return Array.from({ length: days }, (_, i) => {
    const day = new Date(end - (days - 1 - i) * 86400000)
      .toISOString()
      .slice(0, 10);
    return (
      byDate.get(day) ?? {
        date: day,
        requests: 0,
        quota: 0,
        unfunded_quota: 0,
        prompt_tokens: 0,
        completion_tokens: 0,
      }
    );
  });
}
