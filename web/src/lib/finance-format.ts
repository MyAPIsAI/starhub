import { formatMoney } from "./money";

// Keep small nonzero amounts distinguishable from zero in summary cards.
export function formatFinanceAmount(value: string): string {
  const formatted = formatMoney(value, "total");
  return BigInt(value) !== 0n &&
    (formatted === "$0.0000" || formatted === "−$0.0000")
    ? formatMoney(value, "call")
    : formatted;
}
