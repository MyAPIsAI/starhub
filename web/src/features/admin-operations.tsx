import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Drawer } from "../components/ui/overlay";
import { auditSummary } from "../lib/audit-display";
import { useStaffText } from "../lib/api/staff";
import { apiGet, apiPost } from "../lib/api/http";
import { useSessionQueryIdentity } from "../lib/system-access";
import {
  AdminFeedback,
  AdminPagination,
  AdminTable,
  adminAmount,
  adminDate,
  useAdminText,
} from "./admin-shared";

export function ChangeSummary({ value }: { value: string }) {
  const text = useStaffText();
  const description = auditSummary(value, text);
  return description.length > 160 ? (
    <details className="adm-change-summary">
      <summary>{description.slice(0, 140)}…</summary>
      {description}
    </details>
  ) : (
    <span className="adm-change-summary">{description}</span>
  );
}
export { AdminAudit } from "./admin-audit";

interface PriceChange {
  id: number;
  user_id: number;
  created_at: number;
  action: string;
  object_id: string;
  before_summary: string;
  after_summary: string;
  username: string;
}
export function PricingHistory({
  kind,
  id,
}: {
  kind: "models" | "service-group";
  id: string;
}) {
  const t = useAdminText(),
    identity = useSessionQueryIdentity();
  const query = useQuery({
    queryKey: ["pricing-history", ...identity, kind, id],
    queryFn: () =>
      apiGet<PriceChange[]>(
        `/api/admin/pricing-history?kind=${kind}&object=${encodeURIComponent(id)}`,
      ),
    retry: false,
  });
  return (
    <div className="adm-page">
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {query.data?.map((h) => (
        <article className="adm-card" key={h.id}>
          <strong>
            {adminDate(h.created_at)} · {h.username}
          </strong>
          <p>{t("修改前", "Before")}</p>
          <ChangeSummary value={h.before_summary} />
          <p>{t("修改后", "After")}</p>
          <ChangeSummary value={h.after_summary} />
        </article>
      ))}
      {query.data?.length === 0 && (
        <p>{t("暂无价格变更记录。", "No recorded pricing changes.")}</p>
      )}
    </div>
  );
}

interface Difference {
  kind: string;
  funding_source: string;
  expected_nano: string;
  actual_nano: string;
  delta_nano: string;
}
interface Report {
  id: number;
  date: string;
  user_id: number;
  timezone: string;
  usage_nano: string;
  api_ledger_nano: string;
  order_nano: string;
  recharge_ledger_nano: string;
  differences: Difference[];
  provisional: boolean;
  created_at: number;
}
export function AdminReconciliation() {
  const t = useAdminText(),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient();
  const [user, setUser] = useState(""),
    [end, setEnd] = useState(""),
    [selected, setSelected] = useState<Report | null>(null),
    [page, setPage] = useState(1);
  const params = new URLSearchParams({ days: "14" });
  if (end) params.set("end", end);
  if (user) params.set("user_id", user);
  const query = useQuery({
    queryKey: ["admin-reconciliation", ...identity, params.toString()],
    queryFn: () =>
      apiGet<{ items: Report[]; timezone: string; end: string; days: number }>(
        `/api/admin/reconciliation?${params}`,
      ),
    retry: false,
  });
  const customers = useQuery({
    queryKey: ["reconciliation-customers", ...identity],
    queryFn: () =>
      apiGet<{ items: { id: number; username: string }[]; total: number }>(
        "/api/admin/customers?page_size=100",
      ),
    retry: false,
  });
  const mutation = useMutation({
    mutationFn: () => apiPost(`/api/admin/reconciliation?${params}`, {}),
    onSuccess: () =>
      cache.invalidateQueries({ queryKey: ["admin-reconciliation"] }),
    retry: false,
  });
  return (
    <section className="adm-page">
      <header className="adm-heading">
        <div>
          <h1>{t("账务对账", "Financial reconciliation")}</h1>
          <p>
            {t(
              "按地区自然日比对调用用量、API 账本和充值订单。各资金来源独立比对。",
              "Compare daily usage, API ledger and recharge orders in the regional timezone. Funding sources are reconciled separately.",
            )}
          </p>
        </div>
        <Button
          variant="primary"
          disabled={mutation.isPending || !query.data}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending
            ? t("正在对账…", "Reconciling…")
            : t("重新对账", "Reconcile")}
        </Button>
      </header>
      <div className="adm-filters">
        <label>
          {t("账号", "Account")}
          <select
            value={user}
            onChange={(e) => {
              setUser(e.target.value);
              setPage(1);
            }}
          >
            <option value="">{t("全部账号", "All accounts")}</option>
            {customers.data?.items.map((c) => (
              <option key={c.id} value={c.id}>
                {c.username} · {c.id}
              </option>
            ))}
          </select>
        </label>
        {(customers.data?.total ?? 0) > 100 && (
          <label>
            {t("账号 ID", "Account ID")}
            <Input
              type="number"
              min={1}
              value={user}
              onChange={(e) => {
                setUser(e.target.value);
                setPage(1);
              }}
            />
          </label>
        )}
        <label>
          {t("结束日期（最近 14 天）", "End date (last 14 days)")}
          <Input
            type="date"
            value={end}
            onChange={(e) => {
              setEnd(e.target.value);
              setPage(1);
            }}
          />
        </label>
        <span>{query.data?.timezone}</span>
      </div>
      <div className="adm-notice">
        {t(
          "这是平台内部账本对账；尚未接入支付机构对账单或厂商外部账单。当天结果为进行中。",
          "This reconciles internal platform records. Payment-provider statements and vendor invoices are not connected. Today's results are provisional.",
        )}
      </div>
      <AdminFeedback
        pending={query.isPending}
        error={query.error ?? mutation.error ?? customers.error}
        retry={() => void query.refetch()}
      />
      {query.data && (
        <>
          <AdminTable label={t("每日对账报告", "Daily reconciliation reports")}>
            <thead>
              <tr>
                {[
                  t("日期 / 账号", "Day / account"),
                  t("调用用量", "Usage"),
                  t("API 账本扣款", "API ledger debits"),
                  t("已付订单 / 充值账本", "Paid orders / recharge credits"),
                  t("状态", "Status"),
                  t("操作", "Actions"),
                ].map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {query.data.items.slice((page - 1) * 20, page * 20).map((r) => (
                <tr key={r.id}>
                  <td>
                    {r.date}
                    <small>usr_{r.user_id}</small>
                  </td>
                  <td>{adminAmount(r.usage_nano)}</td>
                  <td>{adminAmount(r.api_ledger_nano)}</td>
                  <td>
                    {adminAmount(r.order_nano)}
                    <small>{adminAmount(r.recharge_ledger_nano)}</small>
                  </td>
                  <td>
                    {r.provisional
                      ? t("进行中", "Provisional")
                      : r.differences.length
                        ? t("存在差异", "Differences")
                        : t("一致", "Matched")}
                  </td>
                  <td>
                    <Button variant="ghost" onClick={() => setSelected(r)}>
                      {t("查看明细", "Details")}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
          {!query.data.items.length && (
            <p>
              {t(
                "暂无报告，点击重新对账生成。",
                "No reports yet. Choose Reconcile to generate them.",
              )}
            </p>
          )}
          <AdminPagination
            page={page}
            total={query.data.items.length}
            onChange={setPage}
          />
        </>
      )}
      <Drawer
        open={!!selected}
        onOpenChange={(v) => {
          if (!v) setSelected(null);
        }}
        title={t("对账明细", "Reconciliation details")}
        description={
          selected ? `${selected.date} · usr_${selected.user_id}` : ""
        }
      >
        {selected && (
          <div className="adm-page">
            <p>
              {adminDate(selected.created_at, selected.timezone)} ·{" "}
              {selected.timezone}
            </p>
            {selected.differences.map((d, i) => (
              <article className="adm-card" key={i}>
                <h2>
                  {d.kind} · {d.funding_source || "API"}
                </h2>
                <dl className="adm-info">
                  <div>
                    <dt>{t("预期", "Expected")}</dt>
                    <dd>{adminAmount(d.expected_nano)}</dd>
                  </div>
                  <div>
                    <dt>{t("实际", "Actual")}</dt>
                    <dd>{adminAmount(d.actual_nano)}</dd>
                  </div>
                  <div>
                    <dt>{t("差额", "Difference")}</dt>
                    <dd>{adminAmount(d.delta_nano)}</dd>
                  </div>
                </dl>
              </article>
            ))}
            {!selected.differences.length && (
              <p>{t("内部记录一致。", "Internal records match.")}</p>
            )}
          </div>
        )}
      </Drawer>
    </section>
  );
}

interface SystemInfo {
  version: string;
  go_version: string;
  timezone: string;
  instances: {
    id: string;
    version: string;
    configuration_version: number;
    last_seen: number;
    status: string;
    health_checks: Record<string, boolean>;
  }[];
}
export function AdminSystem() {
  const t = useAdminText(),
    identity = useSessionQueryIdentity();
  const query = useQuery({
    queryKey: ["staff-system", ...identity],
    queryFn: () => apiGet<SystemInfo>("/api/admin/system"),
    retry: false,
    refetchInterval: 10000,
  });
  return (
    <section className="adm-page">
      <header className="adm-heading">
        <div>
          <h1>{t("系统信息", "System information")}</h1>
          <p>
            {t(
              "实际实例版本与最近健康检查，每 10 秒刷新。",
              "Actual instance versions and recent health checks, refreshed every 10 seconds.",
            )}
          </p>
        </div>
        <Button onClick={() => void query.refetch()}>
          {t("刷新", "Refresh")}
        </Button>
      </header>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {query.data && (
        <>
          <div className="adm-card">
            {query.data.version} · {query.data.go_version} ·{" "}
            {query.data.timezone}
          </div>
          <AdminTable label={t("运行实例", "Running instances")}>
            <thead>
              <tr>
                {[
                  t("实例", "Instance"),
                  t("版本", "Version"),
                  t("配置版本", "Configuration"),
                  t("最近心跳", "Last heartbeat"),
                  t("健康检查", "Health checks"),
                ].map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {query.data.instances.map((i) => (
                <tr key={i.id}>
                  <td>{i.id}</td>
                  <td>{i.version || t("未报告", "Not reported")}</td>
                  <td>{i.configuration_version}</td>
                  <td>{adminDate(i.last_seen)}</td>
                  <td>
                    {i.status}
                    <small>
                      {Object.entries(i.health_checks)
                        .map(([name, ok]) => `${name}: ${ok ? "✓" : "×"}`)
                        .join(" · ")}
                    </small>
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
        </>
      )}
    </section>
  );
}
