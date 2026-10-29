// Token (API key) API hooks — typed wrappers around the Go backend's
// /api/token endpoints.
//
// Key behavior:
//   - Create and regenerate return the FULL token with plaintext key.
//     This is the ONLY time the key is visible. Callers MUST capture
//     it from the mutation result and show a one-time dialog; they
//     cannot rely on list (which returns masked keys).  (B10)
//   - Update and delete return { success: true } with NO data field.
//     Mutations invalidate the token list query instead of optimistic
//     updates.  (B5)

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost, apiPut, apiDelete, ApiError } from "./http";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";

export interface Token {
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
  created_at: number;
}

export interface TokenCreateInput {
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
): Promise<void> {
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
    (result as Record<string, unknown>).token_id !== tokenID
  )
    throw Error("API key password verification was not confirmed");
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
        tokens = await apiGet<Token[]>("/api/token", signal, {
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

function useOwnedTokenMutation<Input, Output>(
  write: (input: Input) => Promise<Output>,
  tokenID?: (input: Input) => number,
  returnsSecret = false,
) {
  const queryClient = useQueryClient();
  const identity = useSessionQueryIdentity();
  const queryKey = [...TOKENS_KEY, identity];
  return useMutation({
    retry: false,
    mutationFn: async (input: Input): Promise<Output> => {
      assertTokenSession(identity);
      const id = tokenID?.(input);
      if (tokenID) {
        const tokens = queryClient.getQueryData<Token[]>(queryKey);
        if (
          id === undefined ||
          !Number.isSafeInteger(id) ||
          id <= 0 ||
          queryClient.getQueryState(queryKey)?.status !== "success" ||
          !Array.isArray(tokens) ||
          tokens.some((token) => !token || token.user_id !== identity[1]) ||
          tokens.filter((token) => token.id === id).length !== 1
        )
          throw Error("Reload owned API keys before changing a key");
      }
      let result: Output;
      try {
        result = await write(input);
      } catch (error) {
        assertTokenSession(identity);
        if (error instanceof ApiError && error.status === 401)
          useSystemAccess.getState().setSession(null);
        throw error;
      }
      assertTokenSession(identity);
      if (returnsSecret) {
        const token = result as Token | null;
        if (
          !token ||
          typeof token !== "object" ||
          token.user_id !== identity[1] ||
          !Number.isSafeInteger(token.id) ||
          token.id <= 0 ||
          (id !== undefined && token.id !== id) ||
          typeof token.key !== "string" ||
          !/^sk-[A-Za-z0-9]{48}$/.test(token.key)
        )
          throw Error("API key creation or regeneration was not confirmed");
      }
      return result;
    },
    onSettled: () => {
      // Lost responses also require readback, but an old action must not
      // invalidate the replacement account's list or expose its secret.
      if (
        JSON.stringify(identity) ===
        JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
      )
        void queryClient.invalidateQueries({ queryKey, exact: true });
    },
  });
}

/** Capture the returned secret only after the originating scope is checked. */
export function useCreateToken() {
  return useOwnedTokenMutation(
    (input: TokenCreateInput) =>
      apiPost<Token>("/api/token", input, { handleUnauthorizedLocally: true }),
    undefined,
    true,
  );
}

/** Update a token. The backend returns no data (only {success:true}),
 *  so we invalidate the list to refetch the current state (B5). */
export function useUpdateToken() {
  return useOwnedTokenMutation(
    (input: TokenUpdateInput) =>
      apiPut<null>("/api/token", input, { handleUnauthorizedLocally: true }),
    (input) => input.id,
  );
}

export function useDeleteToken() {
  return useOwnedTokenMutation(
    (id: number) =>
      apiDelete<null>(`/api/token/${id}`, { handleUnauthorizedLocally: true }),
    (id) => id,
  );
}

/** Regenerate a token key. Like create, the result contains the
 *  plaintext key — capture it in onSuccess (B10). */
export function useRegenerateToken() {
  return useOwnedTokenMutation(
    (id: number) =>
      apiPost<Token>(`/api/token/${id}/regenerate`, undefined, {
        handleUnauthorizedLocally: true,
      }),
    (id) => id,
    true,
  );
}
