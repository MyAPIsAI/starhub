import * as tokens from "@/features/tokens/api";
import * as channels from "@/features/channels/api";
import * as users from "@/features/users/api";
import * as logs from "@/features/logs/api";
import * as audit from "@/features/audit/api";
import * as redemptions from "@/features/redemptions/api";
import { api } from "@/lib/http";
import type { Kind } from "./schema";
import type { Row, PageResult } from "@/lib/types";
const domains = { tokens, channels, users, logs, audit, redemptions };
export async function list(
  kind: Kind,
  query: string,
  signal?: AbortSignal,
): Promise<Row[] | PageResult | null> {
  return domains[kind].list(query, signal);
}
export function save(kind: Kind, payload: unknown, editing: boolean) {
  return api<Row & { codes?: string[] }>(domains[kind].endpoint, {
    method: editing ? "PUT" : "POST",
    body: payload,
  });
}
export function remove(kind: Kind, id: number) {
  return api(`${domains[kind].endpoint}/${id}`, { method: "DELETE" });
}
export function probe(id: number, model: string) {
  return api<{
    ok: boolean;
    elapsed_ms: number;
    message: string;
    status: number;
  }>(`/channel/test/${id}?model=${encodeURIComponent(model)}`, {
    method: "POST",
    timeout: 90000,
  });
}
