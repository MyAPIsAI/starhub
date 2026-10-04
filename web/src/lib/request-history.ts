import type { PreviewRequest } from "./product-preview";

export type RequestTimeRange = "today" | "7d" | "30d" | "custom" | "all";
export type RequestHistoryStatus = "all" | "success" | "failed";
export type RequestHistoryFilters = {
  range: RequestTimeRange;
  start: string;
  end: string;
  query: string;
  status: RequestHistoryStatus;
};
export type RequestRangeError =
  "missing_dates" | "invalid_date" | "reversed_dates";
const DAY = 86_400_000;
const DUBAI_OFFSET = 4 * 60 * 60 * 1000;

/** Dubai has a fixed UTC+4 offset; use calendar days rather than a rolling 24-hour window. */
export function dubaiDate(value: number | string | Date = Date.now()): string {
  return new Date(new Date(value).getTime() + DUBAI_OFFSET)
    .toISOString()
    .slice(0, 10);
}
function parseDay(value: string): number | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
  const midnight = Date.parse(`${value}T00:00:00.000+04:00`);
  if (!Number.isFinite(midnight) || dubaiDate(midnight) !== value) return null;
  return midnight;
}
export function requestTimeBounds(
  filters: Pick<RequestHistoryFilters, "range" | "start" | "end">,
  now: number = Date.now(),
): { from: number; until: number; error?: RequestRangeError } {
  if (filters.range === "all") return { from: -Infinity, until: Infinity };
  if (filters.range === "custom") {
    if (!filters.start || !filters.end)
      return { from: 0, until: 0, error: "missing_dates" };
    const from = parseDay(filters.start),
      end = parseDay(filters.end);
    if (from === null || end === null)
      return { from: 0, until: 0, error: "invalid_date" };
    if (from > end) return { from: 0, until: 0, error: "reversed_dates" };
    return { from, until: end + DAY };
  }
  const today = parseDay(dubaiDate(now))!;
  const days = filters.range === "today" ? 1 : filters.range === "30d" ? 30 : 7;
  return { from: today - (days - 1) * DAY, until: today + DAY };
}
export function filterRequestHistory(
  rows: PreviewRequest[],
  filters: RequestHistoryFilters,
  now = Date.now(),
) {
  const bounds = requestTimeBounds(filters, now);
  const query = filters.query.trim().toLocaleLowerCase();
  if (bounds.error) return { rows: [], error: bounds.error };
  return {
    rows: rows
      .filter((row) => {
        const timestamp = Date.parse(row.time);
        return (
          Number.isFinite(timestamp) &&
          timestamp >= bounds.from &&
          timestamp < bounds.until &&
          (filters.status === "all" || row.status === filters.status) &&
          [
            row.trace_id,
            row.requested_model,
            row.actual_model,
            row.model,
            row.error_code,
          ]
            .join(" ")
            .toLocaleLowerCase()
            .includes(query)
        );
      })
      .sort((a, b) => Date.parse(b.time) - Date.parse(a.time)),
    error: undefined,
  };
}
