import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut } from "./http";
import { useProductPreview } from "../product-preview";
import { canReadFinance, useSystemAccess } from "../system-access";

export interface FinanceSummary {
  since: number;
  until: number;
  wallet_balance_nano: string;
  paid_recharge_nano: string;
  paid_recharge_count: number;
  paid_orders_without_timestamp: number;
  ledger_credits_nano: string;
  ledger_debits_nano: string;
  api_collected_nano: string;
  api_unfunded_nano: string;
  api_request_count: number;
  api_success_count: number;
  api_failed_count: number;
  api_cancelled_count: number;
  wallet_and_api_quota_linked: boolean;
}

export function parseFinanceSummary(value: unknown): FinanceSummary {
  if (!value || typeof value !== "object")
    throw new Error("Invalid finance summary");
  const data = value as Record<string, unknown>;
  for (const key of [
    "wallet_balance_nano",
    "paid_recharge_nano",
    "ledger_credits_nano",
    "ledger_debits_nano",
    "api_collected_nano",
    "api_unfunded_nano",
  ]) {
    if (typeof data[key] !== "string" || !/^-?\d+$/.test(data[key] as string))
      throw new Error("Finance amounts must be exact integer strings");
  }
  for (const key of [
    "since",
    "until",
    "paid_recharge_count",
    "paid_orders_without_timestamp",
    "api_request_count",
    "api_success_count",
    "api_failed_count",
    "api_cancelled_count",
  ]) {
    if (
      typeof data[key] !== "number" ||
      !Number.isSafeInteger(data[key]) ||
      (data[key] as number) < 0
    )
      throw new Error("Invalid finance summary counter");
  }
  if (
    (data.until as number) <= (data.since as number) ||
    typeof data.wallet_and_api_quota_linked !== "boolean"
  )
    throw new Error("Invalid finance summary scope");
  return value as FinanceSummary;
}

export function useFinanceSummary(days: number) {
  const accountId = useProductPreview((s) => s.user.id);
  const role = useSystemAccess((s) => s.role);
  return useQuery({
    queryKey: ["finance-summary", accountId, days],
    enabled: canReadFinance(role),
    queryFn: async ({ signal }) =>
      parseFinanceSummary(
        await apiGet<unknown>(`/api/finance/summary?days=${days}`, signal),
      ),
  });
}

export function useSetFinanceRole() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      id: number;
      enabled: boolean;
      expected_role: number;
    }) =>
      apiPut<null>(`/api/user/${input.id}/finance-role`, {
        enabled: input.enabled,
        expected_role: input.expected_role,
      }),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["admin-users"] });
      client.invalidateQueries({ queryKey: ["admin-audit"] });
    },
  });
}
