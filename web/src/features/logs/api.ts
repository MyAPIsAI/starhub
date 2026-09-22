import { api } from "@/lib/http";
import type { PageResult } from "@/lib/types";
export const endpoint = "/log";
export function list(query = "", signal?: AbortSignal) {
  return api<PageResult>(endpoint + query, { signal });
}
