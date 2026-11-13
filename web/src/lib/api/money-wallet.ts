import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import { assertTokenSession } from "./tokens";
import { useSessionQueryIdentity } from "../system-access";
import { exactNano } from "../money";

export interface SpendableWallet {
  schema: "wallet_v2";
  user_id: number;
  amount_unit: "usd_nano";
  balance_nano: string;
  reserved_nano: string;
  total_nano: string;
  spent_nano: string;
  revision: number;
  api_quota_linked: true;
  simulation_conversion_available: false;
}
export interface MoneyEntry {
  id: number;
  user_id: number;
  sequence: number;
  operation_id: string;
  type: string;
  available_delta_nano: string;
  reserved_delta_nano: string;
  spent_delta_nano: string;
  available_after_nano: string;
  reserved_after_nano: string;
  spent_after_nano: string;
  reservation_id?: string;
  log_id?: number;
  actor_id?: number;
  reason: string;
  funding_source: string;
  created_at: number;
}
export interface MoneyLedgerPage {
  schema: "money_ledger_v2";
  amount_unit: "usd_nano";
  user_id: number;
  page: number;
  page_size: number;
  total: number;
  items: MoneyEntry[];
}
const nonnegative = (value: unknown) => BigInt(exactNano(value)) >= 0n;
export function validateSpendableWallet(
  value: SpendableWallet,
  owner: number,
): SpendableWallet {
  if (
    value?.schema !== "wallet_v2" ||
    value.user_id !== owner ||
    value.amount_unit !== "usd_nano" ||
    value.api_quota_linked !== true ||
    value.simulation_conversion_available !== false ||
    !Number.isSafeInteger(value.revision) ||
    value.revision < 1 ||
    ![
      value.balance_nano,
      value.reserved_nano,
      value.total_nano,
      value.spent_nano,
    ].every(nonnegative) ||
    BigInt(value.balance_nano) + BigInt(value.reserved_nano) !==
      BigInt(value.total_nano)
  )
    throw Error("Spendable wallet was not confirmed");
  return value;
}
export function validateMoneyLedger(
  value: MoneyLedgerPage,
  owner: number,
  page: number,
  size: number,
): MoneyLedgerPage {
  if (
    value?.schema !== "money_ledger_v2" ||
    value.amount_unit !== "usd_nano" ||
    value.user_id !== owner ||
    value.page !== page ||
    value.page_size !== size ||
    !Number.isSafeInteger(value.total) ||
    value.total < 0 ||
    !Array.isArray(value.items) ||
    value.items.length > size ||
    value.items.length > value.total
  )
    throw Error("Money ledger page was not confirmed");
  for (const row of value.items) {
    if (
      !row ||
      row.user_id !== owner ||
      ![row.id, row.sequence, row.created_at].every(
        (n) => Number.isSafeInteger(n) && n > 0,
      ) ||
      ![row.operation_id, row.type, row.reason, row.funding_source].every(
        (s) => typeof s === "string",
      ) ||
      ![
        row.available_after_nano,
        row.reserved_after_nano,
        row.spent_after_nano,
      ].every(nonnegative)
    )
      throw Error("Money ledger ownership or amounts were not confirmed");
    [
      row.available_delta_nano,
      row.reserved_delta_nano,
      row.spent_delta_nano,
    ].forEach(exactNano);
  }
  return value;
}
export function useSpendableWallet() {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["wallet", "v2", identity],
    enabled: identity[1] !== null,
    retry: false,
    refetchInterval: 15000,
    queryFn: async ({ signal }) => {
      assertTokenSession(identity);
      const value = await apiGet<SpendableWallet>("/api/wallet/v2", signal);
      assertTokenSession(identity);
      return validateSpendableWallet(value, identity[1]!);
    },
  });
}
export function useMoneyLedger(page: number) {
  const identity = useSessionQueryIdentity(),
    size = 10;
  return useQuery({
    queryKey: ["ledger", "v2", identity, page],
    enabled: identity[1] !== null,
    retry: false,
    queryFn: async ({ signal }) => {
      assertTokenSession(identity);
      const value = await apiGet<MoneyLedgerPage>(
        `/api/ledger/v2?page=${page}&page_size=${size}`,
        signal,
      );
      assertTokenSession(identity);
      return validateMoneyLedger(value, identity[1]!, page, size);
    },
  });
}
