import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import RequestHistory from "./request-history";
import { apiGet } from "../lib/api/http";
import { adaptLogRow, type LogsResponse } from "../lib/api/usage";
import {
  requestTimeBounds,
  type RequestTimeRange,
} from "../lib/request-history";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import type { PreviewRequest } from "../lib/product-preview";
import { useUsageReport } from "../lib/api/usage-report";
import type { ExportInput } from "../lib/api/exports";

export default function RequestHistoryPage({
  timezone = "Asia/Dubai",
  onSelect,
  onExport,
}: {
  timezone?: string;
  onSelect: (row: PreviewRequest) => void;
  onExport: (
    selection?: {
      start_date: string;
      end_date: string;
      status?: string;
      query?: string;
    } & Pick<ExportInput, "key_id" | "model" | "trace_id">,
  ) => void;
}) {
  const [params] = useSearchParams();
  const scope = useSessionQueryIdentity();
  const selected = params.get("range") ?? "7d";
  const range = ["today", "7d", "30d", "custom", "all"].includes(selected)
    ? (selected as RequestTimeRange)
    : "7d";
  const bounds = requestTimeBounds({
    range,
    start: params.get("start") ?? "",
    end: params.get("end") ?? "",
    timezone,
  });
  const page = Number(params.get("page")) || 1;
  const rawStatus = params.get("status") ?? "all";
  const status = ["all", "success", "failed", "cancelled"].includes(rawStatus)
    ? rawStatus
    : "all";
  const query = params.get("query") ?? "";
  const options = useUsageReport("range=7d").data?.options;
  const search = new URLSearchParams({
    scope: "owned",
    p: String(Math.max(1, Math.trunc(page))),
    page_size: "20",
    status,
    trace_id: query,
  });
  for (const name of ["key_id", "model"])
    if (params.get(name)) search.set(name, params.get(name)!);
  if (Number.isFinite(bounds.from))
    search.set("from", String(Math.floor(bounds.from / 1000)));
  if (Number.isFinite(bounds.until))
    search.set("until", String(Math.floor(bounds.until / 1000)));
  const url = `/api/log?${search}`;
  const state = useQuery({
    queryKey: ["request-page", scope, url],
    enabled: scope[1] !== null && !bounds.error,
    retry: false,
    queryFn: async ({ signal }) => {
      const check = () => {
        if (
          JSON.stringify(scope) !==
          JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
        )
          throw Error("Request-list account changed");
      };
      check();
      const data = await apiGet<LogsResponse>(url, signal);
      check();
      if (data.items.some((row) => row.user_id !== scope[1]))
        throw Error("Request-list response belongs to another account");
      return data;
    },
  });
  const rows = state.isError
    ? []
    : ((state.data?.items ?? []).map(adaptLogRow) as PreviewRequest[]);
  return (
    <RequestHistory
      requests={rows}
      timezone={timezone}
      onSelect={onSelect}
      onExport={onExport}
      options={options}
      serverPage={{
        total: state.isError ? 0 : (state.data?.total ?? 0),
        loading: state.isFetching,
        error: state.error?.message,
      }}
    />
  );
}
