import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost, ApiError } from "./http";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";
export interface DeactivationState {
  schema: "deactivation_state_v1";
  policy: "dust_below_cent_v1";
  guarded_deactivation: boolean;
  user_id: number;
  account_revision: string;
  wallet_balance_nano: string;
  pending_recharges: number;
  last_active_root: boolean;
  requires_dust_acknowledgement: boolean;
  dust_limit_nano: "10000000";
  blocked_reasons: string[];
}
export interface CloseInput {
  schema: "deactivation_action_v1";
  expected_user_id: number;
  account_revision: string;
  operation_id: string;
  acknowledge_dust: boolean;
  expected_writeoff_nano: string;
}
export function readDeactivationState(
  value: unknown,
  owner: number,
): DeactivationState {
  const v = value as DeactivationState | null;
  if (
    !v ||
    v.schema !== "deactivation_state_v1" ||
    v.policy !== "dust_below_cent_v1" ||
    v.user_id !== owner ||
    typeof v.wallet_balance_nano !== "string" ||
    !/^(0|-?[1-9][0-9]*)$/.test(v.wallet_balance_nano) ||
    BigInt(v.wallet_balance_nano) < -(1n << 63n) ||
    BigInt(v.wallet_balance_nano) >= 1n << 63n ||
    !/^[a-f0-9]{64}$/.test(v.account_revision) ||
    v.dust_limit_nano !== "10000000" ||
    !Number.isSafeInteger(v.pending_recharges) ||
    v.pending_recharges < 0 ||
    typeof v.last_active_root !== "boolean" ||
    typeof v.requires_dust_acknowledgement !== "boolean" ||
    !Array.isArray(v.blocked_reasons) ||
    v.blocked_reasons.some(
      (x) =>
        ![
          "outstanding_balance",
          "balance_requires_resolution",
          "pending_recharges",
          "last_active_root",
        ].includes(x),
    )
  )
    throw Error("Account closure preview is unavailable");
  const amount = BigInt(v.wallet_balance_nano);
  if (
    v.requires_dust_acknowledgement !== (amount > 0n && amount < 10000000n) ||
    (amount < 0n && !v.blocked_reasons.includes("outstanding_balance")) ||
    (amount >= 10000000n &&
      !v.blocked_reasons.includes("balance_requires_resolution")) ||
    (v.pending_recharges > 0 &&
      !v.blocked_reasons.includes("pending_recharges")) ||
    (v.last_active_root && !v.blocked_reasons.includes("last_active_root"))
  )
    throw Error("Account closure preview is inconsistent");
  return {
    ...v,
    guarded_deactivation: v.guarded_deactivation === true,
    blocked_reasons: [...v.blocked_reasons],
  };
}
export function readCloseReceipt(value: unknown, input: CloseInput) {
  const v = value as Record<string, unknown> | null;
  if (
    !v ||
    v.schema !== "deactivation_receipt_v1" ||
    v.user_id !== input.expected_user_id ||
    v.account_revision !== input.account_revision ||
    v.operation_id !== input.operation_id ||
    v.closed !== true ||
    v.keys_disabled !== true ||
    typeof v.written_off_nano !== "string" ||
    v.written_off_nano !== input.expected_writeoff_nano ||
    !/^(0|[1-9][0-9]*)$/.test(v.written_off_nano) ||
    BigInt(v.written_off_nano) >= 10000000n
  )
    throw Error("Account closure was not confirmed");
  return v;
}
type Scope = ReturnType<typeof sessionQueryIdentity>;
function check(scope: Scope) {
  if (
    scope[1] === null ||
    !Number.isSafeInteger(scope[1]) ||
    scope[1] <= 0 ||
    JSON.stringify(scope) !==
      JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
  )
    throw Error("An unchanged confirmed session is required");
}
async function owned<T>(scope: Scope, run: () => Promise<T>) {
  check(scope);
  try {
    const v = await run();
    check(scope);
    return v;
  } catch (e) {
    check(scope);
    if (e instanceof ApiError && e.status === 401)
      useSystemAccess.getState().setSession(null);
    throw e;
  }
}
export function useDeactivationState() {
  const scope = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["deactivation", scope],
    enabled: scope[1] !== null,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) =>
      readDeactivationState(
        await owned(scope, () =>
          apiGet(
            `/api/user/deactivation-state?expected_user_id=${scope[1]}`,
            signal,
            { handleUnauthorizedLocally: true },
          ),
        ),
        scope[1]!,
      ),
  });
}
export function useCloseAccount(recover = false) {
  const scope = useSessionQueryIdentity(),
    client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (input: CloseInput) => {
      check(scope);
      if (input.expected_user_id !== scope[1]) throw Error("Account changed");
      if (!recover) {
        const key = ["deactivation", scope];
        const state = readDeactivationState(
          client.getQueryData(key),
          scope[1]!,
        );
        if (
          client.getQueryState(key)?.status !== "success" ||
          !state.guarded_deactivation ||
          state.account_revision !== input.account_revision ||
          state.wallet_balance_nano !== input.expected_writeoff_nano ||
          state.blocked_reasons.length ||
          (state.requires_dust_acknowledgement && !input.acknowledge_dust)
        )
          throw Error("Reload the account closure preview");
      }
      const { expected_writeoff_nano: _expected, ...request } = input;
      return readCloseReceipt(
        await owned(scope, () =>
          apiPost(
            recover ? "/api/user/deactivation-receipt" : "/api/user/deactivate",
            request,
            { handleUnauthorizedLocally: true },
          ),
        ),
        input,
      );
    },
  });
}
