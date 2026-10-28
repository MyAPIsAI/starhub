import type { LedgerEntry, RechargeOrder } from "./product-preview";
import { fundingSource } from "./funding-source";

type BillingLanguage = "zh" | "en" | "ar";
type DateRange = { from: string; to: string; query?: string };

const DAY_MS = 86_400_000;
const DUBAI_OFFSET_MS = 4 * 60 * 60 * 1_000;
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

function dubaiIso(iso: string): string {
  const ms = instantMs(iso);
  if (!Number.isFinite(ms)) return "";
  const date = new Date(ms + DUBAI_OFFSET_MS);
  if (date.getUTCFullYear() < 1 || date.getUTCFullYear() > 9999) return "";
  return date.toISOString().replace("Z", "+04:00");
}

/** Returns a Dubai calendar date, or an empty string for an invalid instant. */
export function billingDate(iso: string): string {
  return dubaiIso(iso).slice(0, 10);
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
    dateOnlyMs(filters.from) - DUBAI_OFFSET_MS,
    dateOnlyMs(filters.to) + DAY_MS - DUBAI_OFFSET_MS,
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
    "流水编号",
    "时间（UTC+04:00）",
    "类型",
    "金额（美元）",
    "金额（纳美元）",
    "变更后余额（美元）",
    "变更后余额（纳美元）",
    "说明",
    "订单编号",
    "追踪编号",
    "资金来源",
    "可兑换API配额",
  ],
  en: [
    "Ledger ID",
    "Time (UTC+04:00)",
    "Type",
    "Amount (USD)",
    "Amount (nano-USD)",
    "Balance after (USD)",
    "Balance after (nano-USD)",
    "Description",
    "Order ID",
    "Trace ID",
    "Funding Source",
    "API Quota Eligible",
  ],
  ar: [
    "معرّف الحركة",
    "الوقت (UTC+04:00)",
    "النوع",
    "المبلغ (دولار أمريكي)",
    "المبلغ (نانو دولار)",
    "الرصيد بعد الحركة (دولار أمريكي)",
    "الرصيد بعد الحركة (نانو دولار)",
    "الوصف",
    "معرّف الطلب",
    "معرّف التتبّع",
    "مصدر الأموال",
    "مؤهلة لرصيد API",
  ],
};

const rechargeHeaders: Record<BillingLanguage, string[]> = {
  zh: [
    "订单编号",
    "创建时间（UTC+04:00）",
    "状态",
    "金额（美元）",
    "金额（纳美元）",
    "到账时间（UTC+04:00）",
    "过期时间（UTC+04:00）",
    "晚到标记",
    "资金来源",
    "可兑换API配额",
  ],
  en: [
    "Order ID",
    "Created at (UTC+04:00)",
    "Status",
    "Amount (USD)",
    "Amount (nano-USD)",
    "Paid at (UTC+04:00)",
    "Expires at (UTC+04:00)",
    "Late payment",
    "Funding Source",
    "API Quota Eligible",
  ],
  ar: [
    "معرّف الطلب",
    "وقت الإنشاء (UTC+04:00)",
    "الحالة",
    "المبلغ (دولار أمريكي)",
    "المبلغ (نانو دولار)",
    "وقت وصول الدفع (UTC+04:00)",
    "وقت انتهاء الصلاحية (UTC+04:00)",
    "علامة الدفع المتأخر",
    "مصدر الأموال",
    "مؤهلة لرصيد API",
  ],
};

export function serializeLedgerCsv(
  entries: LedgerEntry[],
  language: BillingLanguage,
): string {
  return csv(
    ledgerHeaders[language],
    entries.map((entry) => {
      const amount = nanoValue(entry.amount_nano);
      const balance = nanoValue(entry.balance_after_nano);
      return [
        entry.id,
        dubaiIso(entry.time),
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
): string {
  return csv(
    rechargeHeaders[language],
    orders.map((order) => {
      const amount = nanoValue(order.amount_nano);
      return [
        order.id,
        dubaiIso(order.created_at),
        order.status,
        dollarValue(amount),
        amount.toString(),
        order.paid_at ? dubaiIso(order.paid_at) : "",
        order.expires_at ? dubaiIso(order.expires_at) : "",
        order.late_payment === undefined ? "" : String(order.late_payment),
        fundingSource(order.funding_source),
        "false",
      ];
    }),
    [3, 4],
  );
}
