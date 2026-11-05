import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import { useSessionQueryIdentity } from "../system-access";
import { formatMoney } from "../money";
import { formatFinanceAmount } from "../finance-format";

export function formatOperatingAmount(value: string): string {
  return BigInt(value) > 0n && BigInt(value) < 10_000_000n
    ? formatFinanceAmount(value)
    : formatMoney(value, "balance");
}

export interface OperatingBucket {
  id: string;
  name: string;
  amount_nano: string;
  requests: number;
}
export interface OperatingDashboard {
  timezone: string;
  region_id: string;
  start_date: string;
  end_date: string;
  since: number;
  until: number;
  data_until: number;
  updated_at: number;
  refresh_seconds: number;
  granularity: "day" | "hour";
  totals: {
    recharge_nano: string;
    consumption_nano: string;
    requests: number;
    successful: number;
    failed: number;
    cancelled: number;
    active_accounts: number;
    new_accounts: number;
    paying_accounts: number;
  };
  funding: {
    offline_nano: string;
    simulation_nano: string;
    unverified_nano: string;
    other_credit_nano: string;
  };
  series: { at: number; amount_nano: string; groups: OperatingBucket[] }[];
  groups: OperatingBucket[];
  vendors: OperatingBucket[];
  call_types: OperatingBucket[];
  models: OperatingBucket[];
  top_accounts: OperatingBucket[];
  top_models: OperatingBucket[];
}

const money = (value: unknown): value is string =>
  typeof value === "string" && /^\d+$/.test(value);
const count = (value: unknown): value is number =>
  Number.isSafeInteger(value) && Number(value) >= 0;
const bucket = (row: any): row is OperatingBucket =>
  row &&
  typeof row.id === "string" &&
  typeof row.name === "string" &&
  money(row.amount_nano) &&
  count(row.requests);
export function isOperatingDashboard(
  value: unknown,
): value is OperatingDashboard {
  const data = value as OperatingDashboard | undefined;
  if (
    !data ||
    typeof data !== "object" ||
    typeof data.timezone !== "string" ||
    typeof data.region_id !== "string" ||
    !count(data.updated_at) ||
    !Number.isSafeInteger(data.data_until) ||
    !Number.isSafeInteger(data.since) ||
    !Number.isSafeInteger(data.until) ||
    typeof data.start_date !== "string" ||
    typeof data.end_date !== "string" ||
    !["day", "hour"].includes(data.granularity)
  )
    return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: data.timezone });
  } catch {
    return false;
  }
  if (
    !data.totals ||
    !money(data.totals.recharge_nano) ||
    !money(data.totals.consumption_nano) ||
    ![
      data.totals.requests,
      data.totals.successful,
      data.totals.failed,
      data.totals.cancelled,
      data.totals.active_accounts,
      data.totals.new_accounts,
      data.totals.paying_accounts,
    ].every(count)
  )
    return false;
  if (
    !data.funding ||
    ![
      data.funding.offline_nano,
      data.funding.simulation_nano,
      data.funding.unverified_nano,
      data.funding.other_credit_nano,
    ].every(money)
  )
    return false;
  return (
    [
      data.groups,
      data.vendors,
      data.call_types,
      data.models,
      data.top_accounts,
      data.top_models,
    ].every((rows) => Array.isArray(rows) && rows.every(bucket)) &&
    Array.isArray(data.series) &&
    data.series.every(
      (point) =>
        point !== null &&
        Number.isSafeInteger(point.at) &&
        money(point.amount_nano) &&
        Array.isArray(point.groups) &&
        point.groups.every(bucket),
    )
  );
}

export function useOperatingDashboard(params: string, enabled: boolean) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["operating-dashboard", ...identity, params],
    enabled,
    queryFn: async ({ signal }) => {
      const data = await apiGet<unknown>(
        `/api/admin/dashboard?${params}`,
        signal,
      );
      if (!isOperatingDashboard(data))
        throw new Error("incompatible_dashboard_response");
      return data;
    },
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    refetchInterval: 300_000,
  });
}

export function operatingShare(amount: string, total: string): number {
  return BigInt(total) > 0n
    ? Number((BigInt(amount) * 10000n) / BigInt(total)) / 100
    : 0;
}

export function operatingChart(data: OperatingDashboard) {
  const groups = data.groups
    .filter((row) => BigInt(row.amount_nano) > 0n)
    .slice(0, 7);
  const selected = new Map(groups.map((row, index) => [row.id, `g${index}`]));
  const hasOther =
    data.groups.filter((row) => BigInt(row.amount_nano) > 0n).length >
    groups.length;
  return {
    groups,
    hasOther,
    points: data.series.map((point) => {
      const amounts: Record<string, string> = Object.fromEntries(
        groups.map((_, index) => [`g${index}`, "0"]),
      );
      let other = 0n;
      for (const row of point.groups) {
        const key = selected.get(row.id);
        if (key) amounts[key] = row.amount_nano;
        else other += BigInt(row.amount_nano);
      }
      if (hasOther) amounts.other = String(other);
      // Floating-point values are used only for chart geometry, never totals or tooltips.
      return {
        at: point.at,
        amount_nano: point.amount_nano,
        exact: amounts,
        ...Object.fromEntries(
          Object.entries(amounts).map(([key, amount]) => [
            key,
            Number(amount) / 1e9,
          ]),
        ),
      };
    }),
  };
}
