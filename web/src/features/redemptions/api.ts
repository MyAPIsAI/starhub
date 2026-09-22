import { api } from "@/lib/http";
import type { Row } from "@/lib/types";
export const endpoint = "/redemption";
export function list(query = "", signal?: AbortSignal) {
  return api<Row[] | null>(endpoint + query, { signal });
}
