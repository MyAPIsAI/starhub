import type { UsageReport, UsageStats } from "./api/usage-report";

const object = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);
const count = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const stats = (value: unknown): value is UsageStats =>
  object(value) &&
  typeof value.id === "string" &&
  typeof value.name === "string" &&
  count(value.requests) &&
  count(value.successful) &&
  value.successful <= value.requests &&
  [
    "input_tokens",
    "output_tokens",
    "cache_read_tokens",
    "cache_write_tokens",
  ].every((name) => count(value[name])) &&
  typeof value.amount_nano === "string" &&
  /^(0|[1-9]\d*)$/.test(value.amount_nano);

export class UsageReportContractError extends Error {
  constructor() {
    super("The server did not return a compatible usage report");
    this.name = "UsageReportContractError";
  }
}

// A successful legacy envelope can contain user-list data instead of a report.
// Validate the contract before exposing anything to the report renderer.
export function isUsageReport(value: unknown): value is UsageReport {
  if (
    !object(value) ||
    !stats(value.totals) ||
    !["rows", "series", "by_key", "by_model"].every(
      (name) => Array.isArray(value[name]) && value[name].every(stats),
    ) ||
    !object(value.options) ||
    !Object.values(value.options).every(
      (options) =>
        Array.isArray(options) &&
        options.every(
          (option) =>
            object(option) &&
            typeof option.id === "string" &&
            typeof option.name === "string",
        ),
    ) ||
    typeof value.timezone !== "string" ||
    typeof value.start_date !== "string" ||
    !/^\d{4}-\d{2}-\d{2}$/.test(value.start_date) ||
    typeof value.end_date !== "string" ||
    !/^\d{4}-\d{2}-\d{2}$/.test(value.end_date) ||
    !["key", "model", "vendor", "call_type", "group"].includes(
      value.group_by as string,
    ) ||
    !["day", "hour"].includes(value.granularity as string)
  )
    return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value.timezone });
    return true;
  } catch {
    return false;
  }
}

export function parseUsageReport(value: unknown): UsageReport {
  if (!isUsageReport(value)) throw new UsageReportContractError();
  return value;
}
