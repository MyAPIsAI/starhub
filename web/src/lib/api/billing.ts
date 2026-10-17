// Billing API hooks — typed wrappers around the Go backend's /api/wallet,
// /api/ledger and /api/recharge endpoints.
//
// Amounts are in nano-USD (1 USD = 1e9 nano) throughout, matching the
// frontend's existing display convention. The backend stores and returns
// nano-USD directly so no unit conversion is needed.
//
// Key behavior:
//   - Create returns a pending order; resolve(success) credits the wallet
//     and writes a ledger entry atomically. Double-resolve is idempotent.
//   - All mutations invalidate the relevant queries so the UI refetches.

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost } from "./http";

// --- Types ---

export interface Wallet {
  user_id: number;
  balance: number; // nano-USD
  created_at: number;
  updated_at: number;
}

export interface LedgerEntry {
  id: number;
  user_id: number;
  type: "recharge" | "call_summary" | "adjustment" | "refund";
  amount: number; // signed nano-USD
  balance_after: number;
  description: string;
  order_id: string;
  trace_id: string;
  created_at: number; // unix seconds
}

export type RechargeStatus =
  | "pending"
  | "processing"
  | "paid"
  | "expired"
  | "failed"
  | "cancelled";

export interface RechargeOrder {
  id: number;
  order_number: string;
  user_id: number;
  amount: number; // nano-USD
  amount_nano: number; // alias for frontend compatibility
  amount_usd: string; // decimal string like "100.00"
  status: RechargeStatus;
  channel: string;
  failure_reason: string;
  late_payment: boolean;
  created_at: string; // ISO string
  expires_at: string; // ISO string
  paid_at: string | null; // ISO string or null
}

// --- Query keys ---

const WALLET_KEY = ["wallet"] as const;
const LEDGER_KEY = ["ledger"] as const;
const RECHARGE_KEY = ["recharge"] as const;

// --- Hooks: Wallet ---

export function useWallet() {
  return useQuery({
    queryKey: WALLET_KEY,
    queryFn: ({ signal }) => apiGet<Wallet>("/api/wallet", signal),
  });
}

// --- Hooks: Ledger ---

export function useLedger() {
  return useQuery({
    queryKey: LEDGER_KEY,
    queryFn: ({ signal }) => apiGet<LedgerEntry[]>("/api/ledger", signal),
  });
}

// --- Hooks: Recharge orders ---

export function useRechargeOrders() {
  return useQuery({
    queryKey: RECHARGE_KEY,
    queryFn: ({ signal }) => apiGet<RechargeOrder[]>("/api/recharge", signal),
  });
}

/** Fetch a single recharge order by id. Used for precise polling of
 *  the active order's status without refetching the entire list. */
export function useRechargeOrder(id?: number) {
  return useQuery({
    queryKey: [...RECHARGE_KEY, "single", id] as const,
    queryFn: ({ signal }) =>
      apiGet<RechargeOrder>(`/api/recharge/${id}`, signal),
    enabled: id != null && id > 0,
    refetchInterval: (query) => {
      // Auto-poll while the order is still pending/processing.
      const status = query.state.data?.status;
      return status === "pending" || status === "processing" ? 5000 : false;
    },
  });
}

export function useCreateRechargeOrder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { amount_usd: string; channel?: string }) =>
      apiPost<RechargeOrder>("/api/recharge", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RECHARGE_KEY });
    },
  });
}

export function useBeginRechargePayment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      apiPost<RechargeOrder>(`/api/recharge/${id}/begin`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RECHARGE_KEY });
    },
  });
}

export function useResolveRechargeOrder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      outcome,
    }: {
      id: number;
      outcome: "success" | "failed" | "cancelled";
    }) =>
      apiPost<RechargeOrder>(`/api/recharge/${id}/resolve`, { outcome }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RECHARGE_KEY });
      queryClient.invalidateQueries({ queryKey: WALLET_KEY });
      queryClient.invalidateQueries({ queryKey: LEDGER_KEY });
    },
  });
}

// --- Adapter: backend RechargeOrder -> frontend RechargeOrder shape ---

/**
 * The frontend billing components expect a RechargeOrder with string IDs
 * and string nano amounts (for BigInt compatibility). This adapter
 * converts the backend's numeric shape to the frontend's string shape.
 */
export function adaptRechargeOrder(o: RechargeOrder): FrontendRechargeOrder {
  return {
    id: String(o.id),
    amount_nano: String(o.amount_nano),
    amount_usd: o.amount_usd,
    currency: "USD",
    status: o.status,
    channel: "card",
    created_at: o.created_at,
    expires_at: o.expires_at,
    provider_order_id: o.order_number,
    paid_at: o.paid_at ?? undefined,
    late_payment: o.late_payment || undefined,
    failure_reason: (o.failure_reason as any) || undefined,
  };
}

export interface FrontendRechargeOrder {
  id: string;
  amount_nano: string;
  amount_usd: string;
  currency: "USD";
  status: RechargeStatus;
  channel: "card";
  created_at: string;
  expires_at: string;
  provider_order_id: string;
  paid_at?: string;
  late_payment?: boolean;
  failure_reason?: "payment_failed" | "payment_cancelled" | "payment_expired";
}

/**
 * Convert a backend LedgerEntry to the frontend's string-shaped LedgerEntry.
 */
export function adaptLedgerEntry(e: LedgerEntry): FrontendLedgerEntry {
  const sign = e.amount >= 0 ? "+" : "-";
  const absAmount = Math.abs(e.amount);
  return {
    id: String(e.id),
    time: new Date(e.created_at * 1000).toISOString(),
    type: e.type,
    amount_nano: sign + String(absAmount),
    balance_after_nano: String(e.balance_after),
    description: e.description,
    order_id: e.order_id || undefined,
    trace_id: e.trace_id || undefined,
  };
}

export interface FrontendLedgerEntry {
  id: string;
  time: string;
  type: "recharge" | "call_summary" | "adjustment" | "refund";
  amount_nano: string;
  balance_after_nano: string;
  description: string;
  order_id?: string;
  trace_id?: string;
}
