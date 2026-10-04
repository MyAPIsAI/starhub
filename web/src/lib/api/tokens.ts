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
import { apiGet, apiPost, apiPut, apiDelete } from "./http";

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

export function useTokens() {
  return useQuery({
    queryKey: TOKENS_KEY,
    queryFn: ({ signal }) => apiGet<Token[]>("/api/token", signal),
  });
}

/** Create a token. The mutation result contains the plaintext key —
 *  capture it in onSuccess to show the one-time key dialog (B10). */
export function useCreateToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: TokenCreateInput) =>
      apiPost<Token>("/api/token", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: TOKENS_KEY });
    },
  });
}

/** Update a token. The backend returns no data (only {success:true}),
 *  so we invalidate the list to refetch the current state (B5). */
export function useUpdateToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: TokenUpdateInput) => apiPut<null>("/api/token", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: TOKENS_KEY });
    },
  });
}

export function useDeleteToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => apiDelete<null>(`/api/token/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: TOKENS_KEY });
    },
  });
}

/** Regenerate a token key. Like create, the result contains the
 *  plaintext key — capture it in onSuccess (B10). */
export function useRegenerateToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => apiPost<Token>(`/api/token/${id}/regenerate`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: TOKENS_KEY });
    },
  });
}
