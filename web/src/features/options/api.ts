import { api } from "@/lib/http";
import type { Option } from "@/lib/types";
export const list = (signal?: AbortSignal) =>
  api<Option[]>("/option", { signal });
export const save = (key: string, value: string) =>
  api("/option", { method: "PUT", body: { key, value } });
