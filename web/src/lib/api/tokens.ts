// Token (API key) API hooks — typed wrappers around the Go backend's
// /api/token endpoints.
//
// Key behavior:
//   - Create and regenerate return the FULL token with plaintext key.
//     Lists return masked keys. Viewing an existing key requires password
//     reauthentication and returns a five-minute reveal window.
//   - Update and delete return { success: true } with NO data field.
//     Mutations invalidate the token list query instead of optimistic
//     updates.  (B5)

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost, ApiError } from "./http";
import { readTokenChoices } from "./token-choices";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";

export interface Token {
  management_schema?: string;
  management_revision?: string;
  id: number;
  user_id: number;
  name: string;
  key: string;
  status: number;
  group: string;
  expired_time: number;
  remain_quota: number;
  unlimited_quota: boolean;
  used_quota: number;
  model_limits: string;
  allow_ips: string;
  auto_groups: string;
  smart_access?: boolean;
  smart_candidates?: string;
  created_at: number;
  quota_period?: "never" | "daily" | "monthly";
  quota_limit?: number;
  quota_timezone?: string;
  period_used_quota?: number;
  period_started_at?: number;
}

export interface TokenCreateInput {
  quota_period?: "never" | "daily" | "monthly";
  quota_limit?: number;
  smart_access?: boolean;
  smart_candidates?: string;
  choices_revision?: string;
  name: string;
  remain_quota?: number;
  unlimited_quota?: boolean;
  expired_time?: number;
  group?: string;
  model_limits?: string;
  allow_ips?: string;
  auto_groups?: string;
}

export interface TokenUpdateInput {
  quota_period?: "never" | "daily" | "monthly";
  quota_limit?: number;
  smart_access?: boolean;
  smart_candidates?: string;
  choices_revision?: string;
  expected_revision?: string;
  id: number;
  name?: string;
  remain_quota?: number;
  expected_quota?: number;
  unlimited_quota?: boolean;
  expired_time?: number;
  group?: string;
  model_limits?: string;
  allow_ips?: string;
  auto_groups?: string;
  status?: number;
}

const TOKENS_KEY = ["tokens"] as const;

// Reveal authorizations stay in page memory and are bound to the confirmed session.
const revealWindows = new Map<string, { grant: string; expires: number }>();
export function currentKeyGrant(
  id: number,
  identity: ReturnType<typeof sessionQueryIdentity>,
) {
  const key = JSON.stringify([identity, id]);
  const window = revealWindows.get(key);
  if (window && window.expires > Date.now()) return window.grant;
  revealWindows.delete(key);
  return undefined;
}

function assertTokenSession(identity: ReturnType<typeof sessionQueryIdentity>) {
  if (
    identity[1] === null ||
    !Number.isSafeInteger(identity[1]) ||
    identity[1] <= 0 ||
    JSON.stringify(identity) !==
      JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
  )
    throw Error("A confirmed unchanged session is required for API keys");
}

export async function verifyTokenPassword(
  tokenID: number,
  password: string,
  identity: ReturnType<typeof sessionQueryIdentity>,
): Promise<{ key: string; expires_at: number; grant: string }> {
  assertTokenSession(identity);
  if (
    !Number.isSafeInteger(tokenID) ||
    tokenID <= 0 ||
    !Number.isSafeInteger(identity[1]) ||
    identity[1]! <= 0 ||
    !password ||
    new TextEncoder().encode(password).length > 72
  )
    throw Error("Invalid API key password verification input");
  let result: unknown;
  try {
    result = await apiPost<unknown>(
      `/api/token/${tokenID}/verify-password`,
      {
        expected_user_id: identity[1],
        password,
      },
      { handleUnauthorizedLocally: true },
    );
  } catch (error) {
    // A late 401 from a superseded request must not sign out the new owner.
    assertTokenSession(identity);
    if (error instanceof ApiError && error.status === 401)
      useSystemAccess.getState().setSession(null);
    throw error;
  }
  assertTokenSession(identity);
  if (
    !result ||
    typeof result !== "object" ||
    (result as Record<string, unknown>).verified !== true ||
    (result as Record<string, unknown>).user_id !== identity[1] ||
    (result as Record<string, unknown>).token_id !== tokenID ||
    typeof (result as Record<string, unknown>).key !== "string" ||
    !(result as Record<string, unknown>).key ||
    typeof (result as Record<string, unknown>).grant !== "string" ||
    !/^[a-f0-9]{64}$/.test(String((result as Record<string, unknown>).grant)) ||
    typeof (result as Record<string, unknown>).expires_at !== "number" ||
    !Number.isSafeInteger((result as Record<string, unknown>).expires_at) ||
    Number((result as Record<string, unknown>).expires_at) * 1000 <= Date.now()
  )
    throw Error("API key password verification was not confirmed");
  const unlocked = result as { key: string; expires_at: number; grant: string };
  revealWindows.set(JSON.stringify([identity, tokenID]), {
    grant: unlocked.grant,
    expires: Math.min(unlocked.expires_at * 1000, Date.now() + 300000),
  });
  return unlocked;
}

export async function revealTokenSecret(
  tokenID: number,
  grant: string,
  identity: ReturnType<typeof sessionQueryIdentity>,
): Promise<{ key: string; expires_at: number }> {
  assertTokenSession(identity);
  const result = await apiPost<{
    key: string;
    token_id: number;
    user_id: number;
    expires_at: number;
  }>(
    `/api/token/${tokenID}/reveal`,
    { expected_user_id: identity[1], grant },
    { handleUnauthorizedLocally: true },
  );
  assertTokenSession(identity);
  if (
    result.user_id !== identity[1] ||
    result.token_id !== tokenID ||
    !result.key ||
    result.expires_at * 1000 <= Date.now()
  )
    throw Error("Key viewing authorization expired");
  return result;
}

export function useTokens() {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: [...TOKENS_KEY, identity],
    enabled: identity[1] !== null,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) => {
      assertTokenSession(identity);
      let tokens: Token[];
      try {
        tokens = await apiGet<Token[]>("/api/token/owned", signal, {
          handleUnauthorizedLocally: true,
        });
      } catch (error) {
        assertTokenSession(identity);
        if (error instanceof ApiError && error.status === 401)
          useSystemAccess.getState().setSession(null);
        throw error;
      }
      assertTokenSession(identity);
      if (
        !Array.isArray(tokens) ||
        tokens.some((token) => !token || token.user_id !== identity[1])
      )
        throw Error("API key ownership could not be verified");
      return tokens;
    },
  });
}

export interface OwnedTokenTarget {
  id: number;
  expected_revision?: string;
}
export async function readOwnedKeyReceipt(
  operation: string,
  identity: ReturnType<typeof sessionQueryIdentity>,
) {
  assertTokenSession(identity);
  const receipt = await apiPost<Record<string, unknown>>(
    "/api/token/owned-receipt",
    {
      schema: "owned_token_action_v1",
      expected_user_id: identity[1],
      operation_id: operation,
    },
    { handleUnauthorizedLocally: true },
  );
  assertTokenSession(identity);
  if (
    receipt.schema !== "owned_token_receipt_v1" ||
    receipt.user_id !== identity[1] ||
    receipt.operation_id !== operation
  )
    throw Error("Key receipt was not confirmed");
  return receipt;
}
function useOwnedTokenMutation<Input extends object, Output>(
  action: string,
  tokenID?: (input: Input) => number,
  returnsSecret = false,
) {
  const client = useQueryClient(),
    identity = useSessionQueryIdentity(),
    queryKey = [...TOKENS_KEY, identity];
  return useMutation({
    retry: false,
    mutationFn: async (input: Input): Promise<Output> => {
      assertTokenSession(identity);
      const raw = input as Record<string, unknown>,
        id = tokenID?.(input),
        choicesKey = ["token-choices", identity];
      const choices = readTokenChoices(
        client.getQueryData(choicesKey),
        identity[1]!,
      );
      if (
        client.getQueryState(choicesKey)?.status !== "success" ||
        (raw.choices_revision !== undefined &&
          raw.choices_revision !== choices.revision)
      )
        throw new ApiError(
          409,
          "Key choices changed; close and reload the form",
        );
      let revision = "";
      if (tokenID) {
        const rows = client.getQueryData<Token[]>(queryKey),
          matches = Array.isArray(rows) ? rows.filter((t) => t.id === id) : [];
        if (
          !Number.isSafeInteger(id) ||
          id! <= 0 ||
          client.getQueryState(queryKey)?.status !== "success" ||
          !Array.isArray(rows) ||
          rows.some((t) => !t || t.user_id !== identity[1]) ||
          matches.length !== 1 ||
          matches[0].management_schema !== "owned_token_v1" ||
          !/^[a-f0-9]{64}$/.test(matches[0].management_revision ?? "")
        )
          throw Error("Reload owned API keys before changing a key");
        revision = matches[0].management_revision!;
        if (raw.expected_revision !== revision)
          throw new ApiError(
            409,
            "Key changed; close and reload the confirmation",
          );
      }
      const operation = crypto.randomUUID().replaceAll("-", "");
      const {
        choices_revision: _choices,
        expected_revision: _revision,
        id: _id,
        ...payload
      } = raw;
      let result: Record<string, unknown>;
      try {
        result = await apiPost(
          "/api/token/owned-action",
          {
            schema: "owned_token_action_v1",
            expected_user_id: identity[1],
            operation_id: operation,
            action,
            token_id: id ?? 0,
            revision,
            choices_revision: choices.revision,
            payload,
          },
          { handleUnauthorizedLocally: true },
        );
      } catch (e) {
        assertTokenSession(identity);
        if (e instanceof ApiError && e.status === 401)
          useSystemAccess.getState().setSession(null);
        throw Object.assign(
          e instanceof Error ? e : new Error("Key action unconfirmed"),
          { operationId: operation },
        );
      }
      assertTokenSession(identity);
      if (
        !result ||
        result.schema !== "owned_token_receipt_v1" ||
        result.user_id !== identity[1] ||
        result.operation_id !== operation ||
        result.action !== action ||
        !Number.isSafeInteger(result.token_id) ||
        Number(result.token_id) <= 0 ||
        (id !== undefined && result.token_id !== id) ||
        typeof result.revision !== "string" ||
        !/^[a-f0-9]{64}$/.test(result.revision)
      )
        throw Object.assign(new Error("Key action was not confirmed"), {
          operationId: operation,
        });
      if (returnsSecret) {
        const token = result.token as Token | null;
        if (
          !token ||
          token.user_id !== identity[1] ||
          token.id !== result.token_id ||
          token.management_revision !== result.revision ||
          typeof token.key !== "string" ||
          !/^sk-[A-Za-z0-9]{48}$/.test(token.key)
        )
          throw Object.assign(
            new Error("API key creation or regeneration was not confirmed"),
            { operationId: operation },
          );
        return token as Output;
      }
      return null as Output;
    },
    onSettled: () => {
      if (
        JSON.stringify(identity) ===
        JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
      )
        void client.invalidateQueries({ queryKey, exact: true });
    },
  });
}
export function useCreateToken() {
  return useOwnedTokenMutation<TokenCreateInput, Token>(
    "create",
    undefined,
    true,
  );
}
export function useUpdateToken() {
  return useOwnedTokenMutation<TokenUpdateInput, null>(
    "update",
    (input) => input.id,
  );
}
export function useDeleteToken() {
  return useOwnedTokenMutation<OwnedTokenTarget, null>(
    "delete",
    (input) => input.id,
  );
}
export function useRegenerateToken() {
  return useOwnedTokenMutation<OwnedTokenTarget, Token>(
    "regenerate",
    (input) => input.id,
    true,
  );
}
