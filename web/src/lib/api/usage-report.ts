import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import { parseUsageReport } from "../usage-report-contract";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";

export interface UsageStats {
  id: string;
  name: string;
  requests: number;
  successful: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  amount_nano: string;
}
export interface UsageReport {
  timezone: string;
  start_date: string;
  end_date: string;
  group_by: string;
  granularity: string;
  totals: UsageStats;
  rows: UsageStats[];
  series: UsageStats[];
  by_key: UsageStats[];
  by_model: UsageStats[];
  options: Record<string, { id: string; name: string }[]>;
}
export function useUsageReport(search: string) {
  const scope = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["usage-report", scope, search],
    enabled: scope[1] !== null,
    retry: false,
    queryFn: async ({ signal }) => {
      const check = () => {
        if (
          JSON.stringify(scope) !==
          JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
        )
          throw Error("Report account changed");
      };
      check();
      const data = await apiGet<unknown>(`/api/user/reports?${search}`, signal);
      check();
      return parseUsageReport(data);
    },
  });
}
