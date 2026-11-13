import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost } from "./http";
import { assertTokenSession } from "./tokens";
import {
  useSessionQueryIdentity,
  useSystemAccess,
  ROLE_ROOT,
} from "../system-access";
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

export interface MoneyAdjustmentInput {
  user_id: number;
  operation_id: string;
  amount_nano: string;
  expected_available_nano: string;
  reason: string;
}
export function useMoneyAdjustment() {
  const identity = useSessionQueryIdentity(),
    client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (input: MoneyAdjustmentInput) => {
      assertTokenSession(identity);
      if (useSystemAccess.getState().role !== ROLE_ROOT)
        throw Error("Root authorization is required");
      exactNano(input.amount_nano);
      exactNano(input.expected_available_nano);
      const receipt = await apiPost<{
        schema: string;
        actor_id: number;
        user_id: number;
        operation_id: string;
        entry: MoneyEntry;
      }>("/api/wallet/adjustments", {
        schema: "money_adjustment_v2",
        expected_actor_id: identity[1],
        ...input,
      });
      assertTokenSession(identity);
      if (
        receipt?.schema !== "money_adjustment_receipt_v2" ||
        receipt.actor_id !== identity[1] ||
        receipt.user_id !== input.user_id ||
        receipt.operation_id !== input.operation_id ||
        receipt.entry?.available_delta_nano !== input.amount_nano
      )
        throw Error(
          "Money adjustment was not confirmed. Retry the same operation.",
        );
      validateMoneyLedger(
        {
          schema: "money_ledger_v2",
          amount_unit: "usd_nano",
          user_id: input.user_id,
          page: 1,
          page_size: 1,
          total: 1,
          items: [receipt.entry],
        },
        input.user_id,
        1,
        1,
      );
      return receipt;
    },
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["admin-users"] });
      void client.invalidateQueries({ queryKey: ["wallet"] });
      void client.invalidateQueries({ queryKey: ["ledger"] });
    },
  });
}
