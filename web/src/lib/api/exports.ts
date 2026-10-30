// Export API hooks — wrappers around the Go backend's /api/export
// endpoints.
//
// The backend generates CSV files server-side from the Log and
// LedgerEntry tables and stores them with a 7-day retention. The
// download endpoint serves the raw CSV bytes.

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost } from "./http";

export interface ExportRecord {
  id: number;
  user_id: number;
  type: "requests" | "ledger" | "usage";
  range_days: number;
  row_count: number;
  size_kb: number;
  status: "completed" | "processing";
  filename: string;
  created_at: number; // unix seconds
}

const EXPORTS_KEY = ["exports"] as const;

export function useExports() {
  return useQuery({
    queryKey: EXPORTS_KEY,
    queryFn: ({ signal }) => apiGet<ExportRecord[]>("/api/export", signal),
  });
}

export function useCreateExport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { type: string; range_days: number }) =>
      apiPost<ExportRecord>("/api/export", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: EXPORTS_KEY });
    },
  });
}

/**
 * Download an export's CSV file. Triggers a browser download by
 * fetching the raw bytes and creating a Blob URL.
 */
export async function downloadExport(id: number, filename: string) {
  const res = await fetch(`/api/export/${id}/download`, {
    credentials: "same-origin",
    headers: { Accept: "text/csv" },
  });
  if (!res.ok) throw new Error(`download failed: ${res.status}`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
