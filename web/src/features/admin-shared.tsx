import type { ReactNode } from "react";
import { useStaffText } from "../lib/api/staff";
import { Button } from "../components/ui/button";
import { formatFinanceAmount } from "../lib/finance-format";
import "./admin-business.css";

export function useAdminText() {
  const text = useStaffText();
  return (zh: string, en: string) => text(zh, en, en);
}
export const adminAmount = (value: string | undefined) =>
  value === undefined ? "—" : formatFinanceAmount(value);
export function adminDate(value: number, zone = "UTC") {
  return value
    ? new Intl.DateTimeFormat(document.documentElement.lang || "en", {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone: zone,
      }).format(new Date(value * 1000))
    : "—";
}
export function AdminFeedback({
  pending,
  error,
  retry,
}: {
  pending?: boolean;
  error?: unknown;
  retry?: () => void;
}) {
  const t = useAdminText();
  if (pending)
    return (
      <p role="status">{t("正在读取服务器数据…", "Loading server data…")}</p>
    );
  if (!error) return null;
  return (
    <div className="adm-notice adm-error" role="alert">
      <span>
        {error instanceof Error
          ? error.message
          : t(
              "无法完成操作，请重试。",
              "Unable to complete this operation. Please retry.",
            )}
      </span>
      {retry && (
        <Button variant="secondary" onClick={retry}>
          {t("重试", "Retry")}
        </Button>
      )}
    </div>
  );
}
export function AdminTable({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div
      className="adm-table-scroll"
      role="region"
      aria-label={label}
      tabIndex={0}
    >
      <table className="adm-table">{children}</table>
    </div>
  );
}
export function AdminPagination({
  page,
  total,
  size = 20,
  onChange,
}: {
  page: number;
  total: number;
  size?: number;
  onChange: (page: number) => void;
}) {
  const t = useAdminText();
  return (
    <div className="adm-pagination">
      <span>
        {t("共", "Total")} {total}
      </span>
      <Button
        variant="ghost"
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
      >
        {t("上一页", "Previous")}
      </Button>
      <span>
        {page} / {Math.max(1, Math.ceil(total / size))}
      </span>
      <Button
        variant="ghost"
        disabled={page * size >= total}
        onClick={() => onChange(page + 1)}
      >
        {t("下一页", "Next")}
      </Button>
    </div>
  );
}
export interface AdminCustomer {
  id: number;
  username: string;
  user_identifier: string;
  email: string;
  phone_masked: string;
  api_balance_nano: string;
  monthly_usage_nano: string;
  key_count: number;
  status: number;
  status_version: number;
  concurrency_limit: number;
  created_at: number;
  last_call_at: number;
  timezone?: string;
  status_reason?: string;
}
export interface AdminCall {
  id: number;
  user_id: number;
  token_name: string;
  model: string;
  requested_model: string;
  trace_id: string;
  status: number;
  quota: number;
  amount_nano?: string;
  created_at: number;
  service_group: string;
  resource_id?: string;
  resource_ids?: string;
  snapshot_sequence: number;
  resource_revision: number;
  affinity_used: boolean;
  routing_recorded: boolean;
  prompt_tokens: number;
  completion_tokens: number;
  duration_ms: number;
  error_code?: string;
  billing_detail?: string;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
  vendor?: string;
  call_type?: string;
  protocol?: string;
  ttft_ms?: number;
}
export interface AdminKey {
  id: number;
  name: string;
  group: string;
  status: number;
  used_quota: number;
}
