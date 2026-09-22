import { api } from "@/lib/http";
import type { Usage, Metrics, Row, User } from "@/lib/types";
export const usage = (days: number, global: boolean, signal?: AbortSignal) =>
  api<Usage>(`/user/usage?days=${days}${global ? "&user_id=0" : ""}`, {
    signal,
  });
export const metrics = (signal?: AbortSignal) =>
  api<Metrics>("/metrics", { signal });
export const channels = (signal?: AbortSignal) =>
  api<Row[] | null>("/channel", { signal });
export const self = (signal?: AbortSignal) =>
  api<User>("/user/self", { signal });
