import { message as translatedMessage } from "../i18n/messages";
import type { LedgerEntry, RechargeOrder } from "./product-preview";
import { fundingSource } from "./funding-source";

type BillingLanguage = "zh" | "en" | "ar";
type DateRange = {
  from: string;
  to: string;
  query?: string;
  timezone?: string;
};

const DAY_MS = 86_400_000;
const NANOS_PER_DOLLAR = 1_000_000_000n;

// Dubai has no daylight-saving transitions. Date-only ranges are interpreted
// as Dubai calendar days, independently of the browser or server time zone.
function dateOnlyMs(value: string): number {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return NaN;
  const [, yearText, monthText, dayText] = match;
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > 31) return NaN;
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(0, 0, 0, 0);
  return date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day
    ? date.getTime()
    : NaN;
}

function instantMs(iso: string): number {
  // Require an explicit offset so an export does not depend on local settings.
  const match =
    /^(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,9}))?)?(Z|[+-]\d{2}:\d{2})$/.exec(
      iso,
    );
  if (!match || !Number.isFinite(dateOnlyMs(match[1]))) return NaN;
  const [, , hours, minutes, seconds = "0", , offset] = match;
  if (Number(hours) > 23 || Number(minutes) > 59 || Number(seconds) > 59)
    return NaN;
  if (
    offset !== "Z" &&
    (Number(offset.slice(1, 3)) > 23 || Number(offset.slice(4)) > 59)
  )
    return NaN;
  return Date.parse(iso);
}

function regionalParts(ms: number, timezone: string) {
  const fields = Object.fromEntries(
    new Intl.DateTimeFormat("en-CA", {
      timeZone: timezone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hourCycle: "h23",
    })
      .formatToParts(new Date(ms))
      .map((p) => [p.type, p.value]),
  );
  const wall = new Date(0);
  wall.setUTCFullYear(
    Number(fields.year),
    Number(fields.month) - 1,
    Number(fields.day),
  );
  wall.setUTCHours(
    Number(fields.hour),
    Number(fields.minute),
    Number(fields.second),
    new Date(ms).getUTCMilliseconds(),
  );
  return wall;
}
function dubaiIso(iso: string, timezone = "Asia/Dubai"): string {
  const ms = instantMs(iso);
  if (!Number.isFinite(ms)) return "";
  try {
    const wall = regionalParts(ms, timezone);
    const minutes = Math.round((wall.getTime() - ms) / 60000);
    const sign = minutes < 0 ? "-" : "+";
    const absolute = Math.abs(minutes);
    return wall
      .toISOString()
      .replace(
        "Z",
        `${sign}${String(Math.floor(absolute / 60)).padStart(2, "0")}:${String(absolute % 60).padStart(2, "0")}`,
      );
  } catch {
    return "";
  }
}
/** Returns the account-region calendar date without depending on browser settings. */
export function billingDate(iso: string, timezone = "Asia/Dubai"): string {
  return dubaiIso(iso, timezone).slice(0, 10);
}

export function regionalDayStart(day: string, timezone = "Asia/Dubai"): number {
  const target = dateOnlyMs(day);
  if (!Number.isFinite(target)) return NaN;
  let instant = target;
  try {
    for (let i = 0; i < 4; i++) {
      const difference = target - regionalParts(instant, timezone).getTime();
      if (difference === 0) return instant;
      instant += difference;
    }
  } catch {}
  return NaN;
}

/** Both boundary days count toward the 31-day maximum. */
export function validateBillingRange(from: string, to: string): boolean {
  const start = dateOnlyMs(from);
  const end = dateOnlyMs(to);
  return (
    Number.isFinite(start) &&
    Number.isFinite(end) &&
    end >= start &&
    end - start < 31 * DAY_MS
  );
}

function bounds(filters: DateRange): [number, number] | null {
  if (!validateBillingRange(filters.from, filters.to)) return null;
  return [
    regionalDayStart(filters.from, filters.timezone),
    regionalDayStart(
      new Date(dateOnlyMs(filters.to) + DAY_MS).toISOString().slice(0, 10),
      filters.timezone,
    ),
  ];
}

function matchesQuery(values: (string | undefined)[], query?: string): boolean {
  const needle = query?.trim().toLowerCase();
  return (
    !needle || values.some((value) => value?.toLowerCase().includes(needle))
  );
}

export function filterLedger(
  entries: LedgerEntry[],
  filters: DateRange & { type?: string },
): LedgerEntry[] {
  const range = bounds(filters);
  if (!range) return [];
  return entries
    .filter((entry) => {
      const ms = instantMs(entry.time);
      return (
        ms >= range[0] &&
        ms < range[1] &&
        (!filters.type ||
          filters.type === "all" ||
          entry.type === filters.type) &&
        matchesQuery(
          [
            entry.id,
            entry.type,
            entry.description,
            entry.order_id,
            entry.trace_id,
          ],
          filters.query,
        )
      );
    })
    .sort((a, b) => instantMs(b.time) - instantMs(a.time));
}

export function filterRechargeOrders(
  orders: RechargeOrder[],
  filters: DateRange & { status?: string },
): RechargeOrder[] {
  const range = bounds(filters);
  if (!range) return [];
  return orders
    .filter((order) => {
      const ms = instantMs(order.created_at);
      return (
        ms >= range[0] &&
        ms < range[1] &&
        (!filters.status ||
          filters.status === "all" ||
          order.status === filters.status) &&
        matchesQuery(
          [order.id, order.status, order.channel, order.provider_order_id],
          filters.query,
        )
      );
    })
    .sort((a, b) => instantMs(b.created_at) - instantMs(a.created_at));
}

function nanoValue(value: string): bigint {
  if (!/^[+-]?\d+$/.test(value))
    throw new TypeError("Invalid nano-dollar amount");
  return BigInt(value);
}

function dollarValue(nano: bigint): string {
  const absolute = nano < 0n ? -nano : nano;
  return `${nano < 0n ? "-" : ""}${absolute / NANOS_PER_DOLLAR}.${(absolute % NANOS_PER_DOLLAR).toString().padStart(9, "0")}`;
}

function csvCell(value: string, numeric = false): string {
  // Amount columns contain validated decimal integers/numbers, including valid
  // negative charges. Untrusted text receives a spreadsheet formula guard.
  const guarded =
    !numeric &&
    (/^[\s\u0000-\u001f\u007f-\u009f]*[=+\-@]/u.test(value) ||
      /^[\t\r\n]/.test(value))
      ? `'${value}`
      : value;
  return `"${guarded.replace(/"/g, '""')}"`;
}

function csv(
  headers: string[],
  rows: string[][],
  numericColumns: number[],
): string {
  return `\uFEFF${[
    headers.map((value) => csvCell(value)).join(","),
    ...rows.map((row) =>
      row
        .map((value, index) => csvCell(value, numericColumns.includes(index)))
        .join(","),
    ),
  ].join("\r\n")}\r\n`;
}

const ledgerHeaders: Record<BillingLanguage, string[]> = {
  zh: [
    translatedMessage("ui_ledger_id_5bb4a90f")[0],
    translatedMessage("ui_time_utc_04_00__9af8854f")[0],
    translatedMessage("type_b91a24e5ad")[0],
    translatedMessage("ui_amount_usd__d055fc14")[0],
    translatedMessage("ui_amount_nano_usd__e33a2cde")[0],
    translatedMessage("ui_balance_after_usd__73e7c7ff")[0],
    translatedMessage("ui_balance_after_nano_usd__665080b4")[0],
    translatedMessage("description_c6bae5d4e7")[0],
    translatedMessage("ui_order_id_22c9cc40")[0],
    translatedMessage("ui_trace_id_f2eafc4e")[0],
    translatedMessage("ui_funding_source_8bb95688")[0],
    translatedMessage("ui_api_quota_eligible_cde987ca")[0],
  ],
  en: [
    translatedMessage("ui_ledger_id_5bb4a90f")[1],
    translatedMessage("ui_time_utc_04_00__9af8854f")[1],
    translatedMessage("type_b91a24e5ad")[1],
    translatedMessage("ui_amount_usd__d055fc14")[1],
    translatedMessage("ui_amount_nano_usd__e33a2cde")[1],
    translatedMessage("ui_balance_after_usd__73e7c7ff")[1],
    translatedMessage("ui_balance_after_nano_usd__665080b4")[1],
    translatedMessage("description_c6bae5d4e7")[1],
    translatedMessage("ui_order_id_22c9cc40")[1],
    translatedMessage("ui_trace_id_f2eafc4e")[1],
    translatedMessage("ui_funding_source_8bb95688")[1],
    translatedMessage("ui_api_quota_eligible_cde987ca")[1],
  ],
  ar: [
    translatedMessage("ui_ledger_id_5bb4a90f")[2],
    translatedMessage("ui_time_utc_04_00__9af8854f")[2],
    translatedMessage("type_b91a24e5ad")[2],
    translatedMessage("ui_amount_usd__d055fc14")[2],
    translatedMessage("ui_amount_nano_usd__e33a2cde")[2],
    translatedMessage("ui_balance_after_usd__73e7c7ff")[2],
    translatedMessage("ui_balance_after_nano_usd__665080b4")[2],
    translatedMessage("description_c6bae5d4e7")[2],
    translatedMessage("ui_order_id_22c9cc40")[2],
    translatedMessage("ui_trace_id_f2eafc4e")[2],
    translatedMessage("ui_funding_source_8bb95688")[2],
    translatedMessage("ui_api_quota_eligible_cde987ca")[2],
  ],
};

const rechargeHeaders: Record<BillingLanguage, string[]> = {
  zh: [
    translatedMessage("ui_order_id_22c9cc40")[0],
    translatedMessage("ui_created_at_utc_04_00__95eb49e8")[0],
    translatedMessage("status_dda39dcd63")[0],
    translatedMessage("ui_amount_usd__d055fc14")[0],
    translatedMessage("ui_amount_nano_usd__e33a2cde")[0],
    translatedMessage("ui_paid_at_utc_04_00__a571edb5")[0],
    translatedMessage("ui_expires_at_utc_04_00__4884088e")[0],
    translatedMessage("ui_late_payment_79799659")[0],
    translatedMessage("ui_funding_source_8bb95688")[0],
    translatedMessage("ui_api_quota_eligible_cde987ca")[0],
  ],
  en: [
    translatedMessage("ui_order_id_22c9cc40")[1],
    translatedMessage("ui_created_at_utc_04_00__95eb49e8")[1],
    translatedMessage("status_dda39dcd63")[1],
    translatedMessage("ui_amount_usd__d055fc14")[1],
    translatedMessage("ui_amount_nano_usd__e33a2cde")[1],
    translatedMessage("ui_paid_at_utc_04_00__a571edb5")[1],
    translatedMessage("ui_expires_at_utc_04_00__4884088e")[1],
    translatedMessage("ui_late_payment_79799659")[1],
    translatedMessage("ui_funding_source_8bb95688")[1],
    translatedMessage("ui_api_quota_eligible_cde987ca")[1],
  ],
  ar: [
    translatedMessage("ui_order_id_22c9cc40")[2],
    translatedMessage("ui_created_at_utc_04_00__95eb49e8")[2],
    translatedMessage("status_dda39dcd63")[2],
    translatedMessage("ui_amount_usd__d055fc14")[2],
    translatedMessage("ui_amount_nano_usd__e33a2cde")[2],
    translatedMessage("ui_paid_at_utc_04_00__a571edb5")[2],
    translatedMessage("ui_expires_at_utc_04_00__4884088e")[2],
    translatedMessage("ui_late_payment_79799659")[2],
    translatedMessage("ui_funding_source_8bb95688")[2],
    translatedMessage("ui_api_quota_eligible_cde987ca")[2],
  ],
};

export function serializeLedgerCsv(
  entries: LedgerEntry[],
  language: BillingLanguage,
  timezone = "Asia/Dubai",
): string {
  return csv(
    ledgerHeaders[language].map((header) =>
      timezone === "Asia/Dubai"
        ? header
        : header.replace("UTC+04:00", timezone),
    ),
    entries.map((entry) => {
      const amount = nanoValue(entry.amount_nano);
      const balance = nanoValue(entry.balance_after_nano);
      return [
        entry.id,
        dubaiIso(entry.time, timezone),
        entry.type,
        dollarValue(amount),
        amount.toString(),
        dollarValue(balance),
        balance.toString(),
        entry.description,
        entry.order_id ?? "",
        entry.trace_id ?? "",
        fundingSource(entry.funding_source),
        "false",
      ];
    }),
    [3, 4, 5, 6],
  );
}

export function serializeRechargeCsv(
  orders: RechargeOrder[],
  language: BillingLanguage,
  timezone = "Asia/Dubai",
): string {
  return csv(
    rechargeHeaders[language].map((header) =>
      timezone === "Asia/Dubai"
        ? header
        : header.replace("UTC+04:00", timezone),
    ),
    orders.map((order) => {
      const amount = nanoValue(order.amount_nano);
      return [
        order.id,
        dubaiIso(order.created_at, timezone),
        order.status,
        dollarValue(amount),
        amount.toString(),
        order.paid_at ? dubaiIso(order.paid_at, timezone) : "",
        order.expires_at ? dubaiIso(order.expires_at, timezone) : "",
        order.late_payment === undefined ? "" : String(order.late_payment),
        fundingSource(order.funding_source),
        "false",
      ];
    }),
    [3, 4],
  );
}
