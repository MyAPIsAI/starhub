import type { PreviewRequest } from "./product-preview";
import { billingDate, regionalDayStart } from "./billing-export";

export type RequestTimeRange = "today" | "7d" | "30d" | "custom" | "all";
export type RequestHistoryStatus = "all" | "success" | "failed" | "cancelled";
export type RequestHistoryFilters = {
  timezone?: string;
  range: RequestTimeRange;
  start: string;
  end: string;
  query: string;
  status: RequestHistoryStatus;
};
export type RequestRangeError =
  "missing_dates" | "invalid_date" | "reversed_dates";
const DAY = 86_400_000;
export function dubaiDate(
  value: number | string | Date = Date.now(),
  timezone = "Asia/Dubai",
): string {
  return billingDate(new Date(value).toISOString(), timezone);
}
function parseDay(value: string, timezone = "Asia/Dubai"): number | null {
  const ms = regionalDayStart(value, timezone);
  return Number.isFinite(ms) ? ms : null;
}
export function requestTimeBounds(
  filters: Pick<RequestHistoryFilters, "range" | "start" | "end" | "timezone">,
  now: number = Date.now(),
): { from: number; until: number; error?: RequestRangeError } {
  if (filters.range === "all") return { from: -Infinity, until: Infinity };
  if (filters.range === "custom") {
    if (!filters.start || !filters.end)
      return { from: 0, until: 0, error: "missing_dates" };
    const from = parseDay(filters.start, filters.timezone),
      end = parseDay(filters.end, filters.timezone);
    if (from === null || end === null)
      return { from: 0, until: 0, error: "invalid_date" };
    if (from > end) return { from: 0, until: 0, error: "reversed_dates" };
    return {
      from,
      until: regionalDayStart(
        new Date(Date.parse(filters.end + "T00:00:00Z") + DAY)
          .toISOString()
          .slice(0, 10),
        filters.timezone,
      ),
    };
  }
  const todayDate = dubaiDate(now, filters.timezone);
  const days = filters.range === "today" ? 1 : filters.range === "30d" ? 30 : 7;
  return {
    from: regionalDayStart(
      new Date(Date.parse(todayDate + "T00:00:00Z") - (days - 1) * DAY)
        .toISOString()
        .slice(0, 10),
      filters.timezone,
    ),
    until: regionalDayStart(
      new Date(Date.parse(todayDate + "T00:00:00Z") + DAY)
        .toISOString()
        .slice(0, 10),
      filters.timezone,
    ),
  };
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
          (filters.status === "all" ||
            (row.outcome ?? row.status) === filters.status) &&
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
