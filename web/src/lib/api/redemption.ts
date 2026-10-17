// Redemption API hooks — wrappers around the Go backend's
// /api/redemption (admin CRUD) and /api/user/redeem (user self-service)
// endpoints.

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost, apiDelete } from "./http";

// --- Types matching the backend Redemption struct ---

export interface Redemption {
  id: number;
  code: string;
  quota: number; // nano-USD
  status: number; // 1 = unused (enabled), 2 = used (disabled)
  used_by: number;
  used_at: number; // unix seconds
  created_at: number; // unix seconds
  expired_at: number; // 0 = never expires
}

export interface CreateRedemptionInput {
  quota: number; // nano-USD
  count?: number; // batch size 1-100
  expired_at?: number; // unix seconds, 0 = never
  code?: string; // optional custom code
}

export interface CreateRedemptionResult {
  codes: string[];
  count: number;
}

// --- Query keys ---

const REDEMPTION_KEY = ["redemptions"] as const;

// --- Admin hooks ---

export function useRedemptions() {
  return useQuery({
    queryKey: REDEMPTION_KEY,
    queryFn: ({ signal }) => apiGet<Redemption[]>("/api/redemption", signal),
  });
}

export function useCreateRedemption() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateRedemptionInput) =>
      apiPost<CreateRedemptionResult>("/api/redemption", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: REDEMPTION_KEY });
    },
  });
}

export function useDeleteRedemption() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => apiDelete(`/api/redemption/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: REDEMPTION_KEY });
    },
  });
}

// --- User hook ---

export function useRedeemCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (code: string) =>
      apiPost<{ quota: number; message: string }>("/api/user/redeem", { code }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["wallet"] });
      queryClient.invalidateQueries({ queryKey: ["ledger"] });
    },
  });
}
