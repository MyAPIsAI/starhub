import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost } from "./http";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";

export interface ExportRecord {
  id: number;
  user_id: number;
  type: "requests" | "ledger" | "usage";
  range_days: number;
  row_count: number;
  size_kb: number;
  status: "completed" | "processing" | "failed";
  filename: string;
  created_at: number;
  expires_at: number;
  start_date: string;
  end_date: string;
  timezone: string;
  language: string;
  download_url: string;
  dictionary_url: string;
  failure?: string;
}
export interface ExportInput {
  type: "requests" | "ledger" | "usage";
  range_days?: number;
  start_date?: string;
  end_date?: string;
  language?: "zh" | "en" | "ar";
  status?: string;
  query?: string;
  ledger_type?: string;
}
function unchanged(scope: ReturnType<typeof sessionQueryIdentity>) {
  if (
    scope[1] === null ||
    JSON.stringify(scope) !==
      JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
  )
    throw Error("The export account changed. Reopen the export.");
}
export function useExports() {
  const scope = useSessionQueryIdentity();
  return useQuery<ExportRecord[]>({
    queryKey: ["exports", scope],
    enabled: scope[1] !== null,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some((row) => row.status === "processing")
        ? 1000
        : false,
    queryFn: async ({ signal }) => {
      unchanged(scope);
      const rows = await apiGet<ExportRecord[]>("/api/export?scope=owned", signal);
      unchanged(scope);
      if (rows.some((row) => row.user_id !== scope[1]))
        throw Error("Export response belongs to another account");
      return rows;
    },
  });
}
export function useCreateExport() {
  const scope = useSessionQueryIdentity();
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: ExportInput) => {
      unchanged(scope);
      const row = await apiPost<ExportRecord>("/api/export", input);
      unchanged(scope);
      if (row.user_id !== scope[1])
        throw Error("Export response belongs to another account");
      return row;
    },
    onSettled: () => {
      if (
        JSON.stringify(scope) ===
        JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
      )
        void client.invalidateQueries({ queryKey: ["exports", scope] });
    },
  });
}
export async function downloadExport(
  id: number,
  filename: string,
  downloadURL?: string,
) {
  const scope = sessionQueryIdentity(useSystemAccess.getState());
  unchanged(scope);
  const url = downloadURL ?? `/api/export/${id}/download`;
  if (
    !/^\/api\/export\/(?:files\/[a-f0-9]{64}\/(?:download|dictionary)|\d+\/download)$/.test(
      url,
    )
  )
    throw Error("Invalid export download location");
  const response = await fetch(url, {
    credentials: "same-origin",
    headers: {
      Accept: "text/csv",
      "X-StarHub-Expected-User-ID": String(scope[1]),
    },
  });
  unchanged(scope);
  if (!response.ok) throw Error(`Export download failed (${response.status})`);
  if (response.headers.get("X-StarHub-User-ID") !== String(scope[1]))
    throw Error("Download account was not confirmed");
  const blob = await response.blob();
  unchanged(scope);
  const objectURL = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectURL;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
}
