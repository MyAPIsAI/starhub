import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut } from "./http";
import {
  canReadFinance,
  ROLE_FINANCE,
  ROLE_ROOT,
  ROLE_USER,
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";

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
  const scope = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["finance-summary", scope, days],
    enabled:
      scope[1] !== null && canReadFinance(scope[3]) && validFinanceDays(days),
    retry: false,
    queryFn: async ({ signal }) => {
      assertFinanceScope(scope, false);
      if (!validFinanceDays(days))
        throw Error("Finance days must be an integer between 1 and 90");
      const result = await apiGet<unknown>(
        `/api/finance/summary?days=${days}`,
        signal,
      );
      assertFinanceScope(scope, false);
      return parseFinanceSummary(result);
    },
  });
}

function validFinanceDays(days: number) {
  return Number.isSafeInteger(days) && days >= 1 && days <= 90;
}

function assertFinanceScope(
  scope: ReturnType<typeof sessionQueryIdentity>,
  root: boolean,
) {
  const current = useSystemAccess.getState();
  if (
    scope[1] === null ||
    JSON.stringify(scope) !== JSON.stringify(sessionQueryIdentity(current)) ||
    (root ? current.role !== ROLE_ROOT : !canReadFinance(current.role))
  )
    throw Error(
      "A confirmed unchanged server session with finance privileges is required",
    );
}

export function useSetFinanceRole() {
  const client = useQueryClient();
  const scope = useSessionQueryIdentity();
  return useMutation({
    retry: false,
    mutationFn: async (input: {
      id: number;
      enabled: boolean;
      expected_role: number;
    }) => {
      assertFinanceScope(scope, true);
      if (
        !Number.isSafeInteger(input.id) ||
        input.id <= 0 ||
        typeof input.enabled !== "boolean" ||
        ![ROLE_USER, ROLE_FINANCE].includes(input.expected_role)
      )
        throw Error("Invalid finance role change");
      const result = await apiPut<unknown>(
        `/api/user/${input.id}/finance-role`,
        {
          enabled: input.enabled,
          expected_role: input.expected_role,
        },
      );
      assertFinanceScope(scope, true);
      if (result !== null && result !== undefined)
        throw Error("Invalid finance role acknowledgement");
      return result;
    },
    onSettled: () => {
      client.invalidateQueries({ queryKey: ["admin-users"] });
      client.invalidateQueries({ queryKey: ["admin-audit"] });
      client.invalidateQueries({ queryKey: ["finance-summary"] });
    },
  });
}
