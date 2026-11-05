import { SettledBilling } from "../components/settled-billing";
import { readBillingDetail } from "../lib/request-metadata";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import * as Tabs from "@radix-ui/react-tabs";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Modal, Drawer } from "../components/ui/overlay";
import { apiGet, apiPost } from "../lib/api/http";
import {
  isAdmin,
  canReadFinance,
  useSystemAccess,
  useSessionQueryIdentity,
} from "../lib/system-access";
import {
  AdminFeedback,
  AdminPagination,
  AdminTable,
  adminAmount,
  adminDate,
  useAdminText,
  type AdminCustomer,
  type AdminCall,
  type AdminKey,
} from "./admin-shared";

interface AccountPage {
  items: AdminCustomer[];
  total: number;
}
interface AccountEntry {
  id?: number;
  type?: string;
  order_number?: string;
  amount_nano?: string;
  balance_nano?: string;
  status?: string;
  funding_source?: string;
  channel?: string;
  created_at: number;
  expires_at?: number;
}
type Action =
  "disable" | "enable" | "concurrency" | "revoke" | "key" | "balance";
export function AdminCustomers() {
  const [params] = useSearchParams();
  const id = params.get("customer");
  return id ? <CustomerDetail key={id} id={id} /> : <CustomerList />;
}
function CustomerList() {
  const t = useAdminText(),
    identity = useSessionQueryIdentity();
  const [search, setSearch] = useState(""),
    [status, setStatus] = useState(""),
    [page, setPage] = useState(1);
  const query = useQuery({
    queryKey: ["admin-customer-list", ...identity, search, status, page],
    queryFn: ({ signal }) =>
      apiGet<AccountPage>(
        `/api/admin/customers?search=${encodeURIComponent(search)}&status=${status}&page=${page}`,
        signal,
      ),
    retry: false,
  });
  return (
    <section className="adm-page">
      <header className="adm-heading">
        <div>
          <h1>{t("账号管理", "Customer accounts")}</h1>
          <p>
            {t(
              "查看个人账号及其密钥、调用、账本、订单与登录会话。",
              "Inspect customer accounts, keys, calls, ledger, orders and login sessions.",
            )}
          </p>
        </div>
      </header>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      <div>
        <div className="adm-filters">
          <Input
            aria-label={t("搜索账号", "Search accounts")}
            placeholder={t(
              "用户名 / 手机号 / 用户标识",
              "Username / phone / user ID",
            )}
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
          />
          <select
            aria-label={t("账号状态", "Account status")}
            value={status}
            onChange={(e) => {
              setStatus(e.target.value);
              setPage(1);
            }}
          >
            <option value="">{t("全部状态", "All statuses")}</option>
            <option value="1">{t("正常", "Enabled")}</option>
            <option value="2">{t("已禁用", "Disabled")}</option>
          </select>
        </div>
        {query.data && (
          <>
            <AdminTable label={t("账号列表", "Account list")}>
              <thead>
                <tr>
                  {[
                    t("用户名", "Username"),
                    t("用户标识", "User ID"),
                    t("手机号", "Phone"),
                    t("邮箱", "Email"),
                    t("余额", "Balance"),
                    t("本月消耗", "Monthly spend"),
                    t("密钥数", "Keys"),
                    t("状态", "Status"),
                    t("最近调用", "Last call"),
                    t("注册时间", "Registered"),
                  ].map((x) => (
                    <th key={x}>{x}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {query.data.items.map((user) => (
                  <tr key={user.id}>
                    <td>
                      <Link
                        className="adm-link"
                        to={`/admin?tab=customers&customer=${user.id}`}
                      >
                        {user.username}
                      </Link>
                    </td>
                    <td>
                      <bdi>{user.user_identifier}</bdi>
                    </td>
                    <td>
                      <bdi>{user.phone_masked || "—"}</bdi>
                    </td>
                    <td>{user.email || "—"}</td>
                    <td>
                      <bdi>{adminAmount(user.api_balance_nano)}</bdi>
                    </td>
                    <td>
                      <bdi>{adminAmount(user.monthly_usage_nano)}</bdi>
                    </td>
                    <td>{user.key_count}</td>
                    <td>
                      <span
                        className={`adm-status ${user.status === 1 ? "is-active" : "is-disabled"}`}
                      >
                        {user.status === 1
                          ? t("正常", "Enabled")
                          : t("已禁用", "Disabled")}
                      </span>
                    </td>
                    <td>{adminDate(user.last_call_at, user.timezone)}</td>
                    <td>{adminDate(user.created_at, user.timezone)}</td>
                  </tr>
                ))}
              </tbody>
            </AdminTable>
            {query.data.items.length === 0 && (
              <p>{t("暂无匹配账号。", "No matching accounts.")}</p>
            )}
            <AdminPagination
              page={page}
              total={query.data.total}
              onChange={setPage}
            />
          </>
        )}
      </div>
    </section>
  );
}
function CustomerDetail({ id }: { id: string }) {
  const t = useAdminText(),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient(),
    role = useSystemAccess((s) => s.role);
  const query = useQuery({
    queryKey: ["admin-customer", ...identity, id],
    queryFn: () => apiGet<AdminCustomer>(`/api/admin/customers/${id}`),
    retry: false,
  });
  const keys = useQuery({
    queryKey: ["admin-customer-keys", ...identity, id],
    queryFn: () => apiGet<AdminKey[]>(`/api/admin/customers/${id}/keys`),
    retry: false,
  });
  const [tab, setTab] = useState("keys"),
    [page, setPage] = useState(1),
    [call, setCall] = useState<AdminCall | null>(null);
  const activity = useQuery({
    queryKey: ["admin-customer-activity", ...identity, id, tab, page],
    queryFn: () =>
      apiGet<{ items: (AdminCall | AccountEntry)[]; total: number }>(
        `/api/admin/customers/${id}/activity?section=${tab}&page=${page}`,
      ),
    enabled: tab !== "keys",
    retry: false,
  });
  const [action, setAction] = useState<Action | null>(null),
    [reason, setReason] = useState(""),
    [limit, setLimit] = useState("50"),
    [key, setKey] = useState<AdminKey | null>(null);
  const [amount, setAmount] = useState(""),
    [kind, setKind] = useState("offline_payment"),
    [operation, setOperation] = useState("");
  const begin = (next: Action, target?: AdminKey) => {
    setAction(next);
    setReason("");
    setLimit(String(query.data?.concurrency_limit ?? 50));
    setKey(target ?? null);
    setAmount("");
    setKind("offline_payment");
    setOperation(crypto.randomUUID());
    mutation.reset();
  };
  const mutation = useMutation({
    retry: false,
    mutationFn: () => {
      const base = `/api/admin/customers/${id}`;
      if (action === "balance")
        return apiPost(base + "/adjustments", {
          amount_usd: amount,
          kind,
          reason,
          operation_id: operation,
        });
      if (action === "key")
        return apiPost(base + `/keys/${key!.id}/disable`, { reason });
      if (action === "revoke")
        return apiPost(base + "/sessions/revoke", { reason });
      return apiPost(base + "/control", {
        reason,
        expected_version: query.data!.status_version,
        ...(action === "concurrency"
          ? { concurrency_limit: Number(limit) }
          : { status: action === "enable" ? 1 : 2 }),
      });
    },
    onSuccess: async () => {
      setAction(null);
      for (const name of [
        "admin-customer",
        "admin-customer-list",
        "admin-customer-keys",
        "admin-customer-activity",
        "admin-audit",
      ])
        await cache.invalidateQueries({ queryKey: [name] });
    },
  });
  const user = query.data,
    write = isAdmin(role),
    finance = canReadFinance(role);
  const labels = {
    keys: t("访问密钥", "Access keys"),
    calls: t("调用明细", "Call details"),
    ledger: t("账本流水", "Ledger"),
    orders: t("支付订单", "Payment orders"),
    sessions: t("登录会话", "Login sessions"),
  };
  const actionLabel =
    action === "balance"
      ? t("手动加款 / 扣款", "Manual balance adjustment")
      : action === "enable"
        ? t("启用账号", "Enable account")
        : action === "disable"
          ? t("禁用账号", "Disable account")
          : action === "revoke"
            ? t("强制下线", "Revoke sessions")
            : action === "key"
              ? t("停用密钥", "Disable key")
              : t("修改并发上限", "Change concurrent limit");
  return (
    <section className="adm-page">
      <Link className="adm-link" to="/admin?tab=customers">
        <ArrowLeft size={14} className="directional" />{" "}
        {t("账号管理", "Customer accounts")}
      </Link>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {user && (
        <>
          <header className="adm-heading">
            <div>
              <h1>
                {user.username}{" "}
                <span
                  className={`adm-status ${user.status === 1 ? "is-active" : "is-disabled"}`}
                >
                  {user.status === 1
                    ? t("正常", "Enabled")
                    : t("已禁用", "Disabled")}
                </span>
              </h1>
              <p>
                <bdi>{user.user_identifier}</bdi> · {t("注册于", "Registered")}{" "}
                {adminDate(user.created_at, user.timezone)}
              </p>
            </div>
            <div className="adm-actions">
              {write && (
                <Button
                  variant="secondary"
                  onClick={() =>
                    begin(user.status === 1 ? "disable" : "enable")
                  }
                >
                  {user.status === 1
                    ? t("禁用账号", "Disable account")
                    : t("启用账号", "Enable account")}
                </Button>
              )}
              {finance && (
                <Button variant="primary" onClick={() => begin("balance")}>
                  {t("手动加款", "Adjust balance")}
                </Button>
              )}
            </div>
          </header>
          <div className="adm-detail-grid">
            <article className="adm-card">
              <h2>{t("基本信息", "Account information")}</h2>
              <dl className="adm-info">
                {[
                  [t("手机号", "Phone"), user.phone_masked || "—"],
                  [t("邮箱", "Email"), user.email || "—"],
                  [t("余额", "Balance"), adminAmount(user.api_balance_nano)],
                  [
                    t("本月消耗", "Monthly spend"),
                    adminAmount(user.monthly_usage_nano),
                  ],
                  [t("密钥数", "Keys"), user.key_count],
                  [
                    t("最近调用", "Last call"),
                    adminDate(user.last_call_at, user.timezone),
                  ],
                ].map(([name, value]) => (
                  <div key={name}>
                    <dt>{name}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
              {user.status_reason && (
                <p className="adm-meta">
                  {t("状态原因", "Status reason")}: {user.status_reason}
                </p>
              )}
            </article>
            <article className="adm-card">
              <h2>{t("并发上限", "Concurrent request limit")}</h2>
              <p className="adm-meta">
                {t("默认 50 个并发请求", "Default: 50 concurrent requests")}
              </p>
              <div className="adm-count">
                <bdi>{user.concurrency_limit}</bdi>
              </div>
              {write && (
                <Button
                  variant="secondary"
                  onClick={() => begin("concurrency")}
                >
                  {t("修改上限", "Change limit")}
                </Button>
              )}
            </article>
          </div>
          <Tabs.Root
            value={tab}
            onValueChange={(value) => {
              setTab(value);
              setPage(1);
            }}
          >
            <Tabs.List className="adm-tabs">
              {Object.entries(labels).map(([value, label]) => (
                <Tabs.Trigger className="adm-tab" key={value} value={value}>
                  {label}
                  {value === "keys" && ` (${user.key_count})`}
                </Tabs.Trigger>
              ))}
            </Tabs.List>
            <Tabs.Content value="keys">
              <AdminFeedback
                pending={keys.isPending}
                error={keys.error}
                retry={() => void keys.refetch()}
              />
              {keys.data && (
                <AdminTable label={labels.keys}>
                  <thead>
                    <tr>
                      {[
                        t("名称", "Name"),
                        t("标识", "ID"),
                        t("服务分组", "Group"),
                        t("状态", "Status"),
                        t("操作", "Actions"),
                      ].map((x) => (
                        <th key={x}>{x}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {keys.data.map((k) => (
                      <tr key={k.id}>
                        <td>{k.name}</td>
                        <td>{k.id}</td>
                        <td>{k.group}</td>
                        <td>
                          {k.status === 1
                            ? t("启用", "Enabled")
                            : t("已停用", "Disabled")}
                        </td>
                        <td>
                          {write && k.status === 1 && (
                            <Button
                              size="small"
                              variant="secondary"
                              onClick={() => begin("key", k)}
                            >
                              {t("停用", "Disable")}
                            </Button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </AdminTable>
              )}
              <p className="adm-meta">
                {t(
                  "后台只显示密钥标识及属性，不能查看完整密钥。",
                  "Staff can inspect key identifiers and attributes, not complete credentials.",
                )}
              </p>
            </Tabs.Content>
            {["calls", "ledger", "orders", "sessions"].map((section) => (
              <Tabs.Content key={section} value={section}>
                <AdminFeedback
                  pending={activity.isPending}
                  error={activity.error}
                  retry={() => void activity.refetch()}
                />
                {section === "sessions" && write && (
                  <div className="adm-actions">
                    <Button variant="secondary" onClick={() => begin("revoke")}>
                      {t("强制下线全部会话", "Revoke all sessions")}
                    </Button>
                  </div>
                )}
                {activity.data && (
                  <>
                    <AdminTable label={labels[section as keyof typeof labels]}>
                      <thead>
                        <tr>
                          {(section === "calls"
                            ? [
                                t("时间", "Time"),
                                t("模型", "Model"),
                                "Trace ID",
                                t("状态", "Status"),
                                t("操作", "Actions"),
                              ]
                            : section === "sessions"
                              ? [
                                  t("创建时间", "Created"),
                                  t("到期时间", "Expires"),
                                ]
                              : [
                                  t("时间", "Time"),
                                  section === "orders"
                                    ? t("订单号", "Order")
                                    : t("类型", "Type"),
                                  t("金额", "Amount"),
                                  section === "orders"
                                    ? t("状态", "Status")
                                    : t("余额", "Balance"),
                                  t("资金来源", "Funding source"),
                                ]
                          ).map((x) => (
                            <th key={x}>{x}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {activity.data.items.map((value, index) => {
                          if (section === "calls") {
                            const r = value as AdminCall;
                            return (
                              <tr key={r.id}>
                                <td>
                                  {adminDate(r.created_at, user.timezone)}
                                </td>
                                <td>
                                  <bdi>{r.model}</bdi>
                                </td>
                                <td>
                                  <bdi>{r.trace_id}</bdi>
                                </td>
                                <td>
                                  {r.status === 1
                                    ? t("成功", "Succeeded")
                                    : t("未成功", "Unsuccessful")}
                                </td>
                                <td>
                                  <button
                                    className="adm-link"
                                    onClick={() => setCall(r)}
                                  >
                                    {t("详情", "Details")}
                                  </button>
                                </td>
                              </tr>
                            );
                          }
                          const r = value as AccountEntry;
                          if (section === "sessions")
                            return (
                              <tr key={index}>
                                <td>
                                  {adminDate(r.created_at, user.timezone)}
                                </td>
                                <td>
                                  {adminDate(r.expires_at ?? 0, user.timezone)}
                                </td>
                              </tr>
                            );
                          return (
                            <tr key={r.id ?? index}>
                              <td>{adminDate(r.created_at, user.timezone)}</td>
                              <td>
                                {section === "orders" ? r.order_number : r.type}
                              </td>
                              <td>
                                <bdi>{adminAmount(r.amount_nano)}</bdi>
                              </td>
                              <td>
                                {section === "orders"
                                  ? r.status
                                  : adminAmount(r.balance_nano)}
                              </td>
                              <td>{r.funding_source}</td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </AdminTable>
                    {activity.data.items.length === 0 && (
                      <p>{t("暂无记录。", "No records.")}</p>
                    )}
                    <AdminPagination
                      page={page}
                      total={activity.data.total}
                      onChange={setPage}
                    />
                  </>
                )}
              </Tabs.Content>
            ))}
          </Tabs.Root>
        </>
      )}
      <Modal
        open={!!action}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setAction(null);
        }}
        title={actionLabel}
        description={user?.username ?? ""}
        preventOutsideClose
        footer={
          <>
            <Button
              disabled={mutation.isPending}
              onClick={() => setAction(null)}
            >
              {t("取消", "Cancel")}
            </Button>
            <Button
              variant="primary"
              form="admin-customer-action"
              type="submit"
              disabled={mutation.isPending}
            >
              {t("确认", "Confirm")}
            </Button>
          </>
        }
      >
        <form
          id="admin-customer-action"
          className="adm-form"
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
        >
          {action === "concurrency" && (
            <label className="adm-wide">
              {t("新的并发上限", "New concurrent request limit")}
              <Input
                type="number"
                min={1}
                max={10000}
                step={1}
                required
                value={limit}
                onChange={(e) => setLimit(e.target.value)}
              />
            </label>
          )}
          {action === "balance" && (
            <>
              <label>
                {t("金额（USD）", "Amount (USD)")}
                <Input
                  required
                  inputMode="decimal"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                />
              </label>
              <label>
                {t("调整类型", "Adjustment type")}
                <select value={kind} onChange={(e) => setKind(e.target.value)}>
                  <option value="offline_payment">
                    {t("线下付款", "Offline payment")}
                  </option>
                  <option value="gift">{t("赠送", "Gift")}</option>
                  <option value="compensation">
                    {t("补偿", "Compensation")}
                  </option>
                  <option value="deduction">
                    {t("扣款（负数）", "Deduction (negative amount)")}
                  </option>
                </select>
              </label>
            </>
          )}
          <label className="adm-wide">
            {t("原因", "Reason")}
            <textarea
              required
              rows={3}
              maxLength={500}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </label>
          <div className="adm-wide">
            <AdminFeedback error={mutation.error} />
          </div>
        </form>
      </Modal>
      <AdminCallDrawer
        call={call}
        onClose={() => setCall(null)}
        timezone={user?.timezone}
      />
    </section>
  );
}
export function AdminCallDrawer({
  call,
  onClose,
  timezone,
}: {
  call: AdminCall | null;
  onClose: () => void;
  timezone?: string;
}) {
  const t = useAdminText();
  return (
    <Drawer
      open={!!call}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("调用详情", "Call details")}
      description={call?.trace_id ?? ""}
    >
      {call && (
        <div className="adm-page">
          <dl className="adm-info">
            {[
              [t("费用", "Charge"), adminAmount(call.amount_nano)],
              [t("密钥", "Key"), call.token_name],
              [t("厂商", "Vendor"), call.vendor || "—"],
              [
                t("调用类型 / 协议", "Call type / protocol"),
                `${call.call_type || "—"} / ${call.protocol || "—"}`,
              ],
              [
                t("缓存读取 / 写入词元", "Cache read / write tokens"),
                `${call.cache_read_tokens ?? 0} / ${call.cache_write_tokens ?? 0}`,
              ],
              [
                t("首字延迟", "Time to first token"),
                call.ttft_ms ? `${call.ttft_ms} ms` : "—",
              ],
              [t("受理时间", "Accepted"), adminDate(call.created_at, timezone)],
              [t("请求模型", "Requested model"), call.requested_model],
              [t("实际模型", "Executed model"), call.model],
              [t("服务分组", "Service group"), call.service_group],
              [
                t("资源实例 ID", "Resource instance ID"),
                call.resource_id ||
                  call.resource_ids ||
                  t("未记录 / 非托管资源", "Unrecorded / unmanaged resource"),
              ],
              [
                t("受理时快照序号", "Admission snapshot sequence"),
                call.routing_recorded && call.snapshot_sequence > 0
                  ? call.snapshot_sequence
                  : t("历史未记录", "Historically unrecorded"),
              ],
              [
                t("资源配置版本", "Resource revision"),
                call.routing_recorded ? call.resource_revision : "—",
              ],
              [
                t("亲和命中", "Affinity hit"),
                call.routing_recorded
                  ? call.affinity_used
                    ? t("是", "Yes")
                    : t("否", "No")
                  : "—",
              ],
              [
                t("输入 / 输出词元", "Input / output tokens"),
                `${call.prompt_tokens} / ${call.completion_tokens}`,
              ],
              [t("耗时", "Duration"), `${call.duration_ms} ms`],
              [t("错误码", "Error code"), call.error_code || "—"],
            ].map(([name, value]) => (
              <div key={name}>
                <dt>{name}</dt>
                <dd>
                  <bdi>{value}</bdi>
                </dd>
              </div>
            ))}
          </dl>
          {readBillingDetail(call.billing_detail) && (
            <dl>
              <SettledBilling
                detail={readBillingDetail(call.billing_detail)!}
                timezone={timezone}
              />
            </dl>
          )}
          <p className="adm-meta">
            {t(
              "仅显示调用与路由元数据，不保存或展示请求、响应内容。",
              "Only request and routing metadata are shown; request and response content is not stored or displayed.",
            )}
          </p>
        </div>
      )}
    </Drawer>
  );
}
