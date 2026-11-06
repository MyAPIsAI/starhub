import { messagePair } from "../i18n/messages";
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
          <h1>{t(...messagePair("customer_accounts_9413e58186"))}</h1>
          <p>
            {t(
              ...messagePair(
                "inspect_customer_accounts_keys_calls_ledger_o_2bf995506b",
              ),
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
            aria-label={t(...messagePair("search_accounts_03a853d444"))}
            placeholder={t(...messagePair("username_phone_user_id_57617944ed"))}
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
          />
          <select
            aria-label={t(...messagePair("account_status_f0999dcc05"))}
            value={status}
            onChange={(e) => {
              setStatus(e.target.value);
              setPage(1);
            }}
          >
            <option value="">
              {t(...messagePair("all_statuses_9fda56fbf6"))}
            </option>
            <option value="1">{t(...messagePair("enabled_c2ea99f45b"))}</option>
            <option value="2">
              {t(...messagePair("disabled_79b1b987fa"))}
            </option>
          </select>
        </div>
        {query.data && (
          <>
            <AdminTable label={t(...messagePair("account_list_0dc352a191"))}>
              <thead>
                <tr>
                  {[
                    t(...messagePair("username_6b74910d23")),
                    t(...messagePair("user_id_9f6e3c663a")),
                    t(...messagePair("phone_a90f318a7f")),
                    t(...messagePair("email_fd11b7ed4b")),
                    t(...messagePair("balance_e1258fb4af")),
                    t(...messagePair("monthly_spend_039f31d3cd")),
                    t(...messagePair("keys_98f373eeaf")),
                    t(...messagePair("status_05fdd567d6")),
                    t(...messagePair("last_call_ea054b6e23")),
                    t(...messagePair("registered_9fc53d0c88")),
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
                          ? t(...messagePair("enabled_c2ea99f45b"))
                          : t(...messagePair("disabled_79b1b987fa"))}
                      </span>
                    </td>
                    <td>{adminDate(user.last_call_at, user.timezone)}</td>
                    <td>{adminDate(user.created_at, user.timezone)}</td>
                  </tr>
                ))}
              </tbody>
            </AdminTable>
            {query.data.items.length === 0 && (
              <p>{t(...messagePair("no_matching_accounts_de02419bf3"))}</p>
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
    keys: t(...messagePair("access_keys_2a98cb0b87")),
    calls: t(...messagePair("call_details_09feda52cb")),
    ledger: t(...messagePair("ledger_f3b8cadd56")),
    orders: t(...messagePair("payment_orders_e9b93dd9df")),
    sessions: t(...messagePair("login_sessions_8d57b0e8ec")),
  };
  const actionLabel =
    action === "balance"
      ? t(...messagePair("manual_balance_adjustment_fccfd63d1b"))
      : action === "enable"
        ? t(...messagePair("enable_account_fc76ab91b6"))
        : action === "disable"
          ? t(...messagePair("disable_account_d2f78c3c65"))
          : action === "revoke"
            ? t(...messagePair("revoke_sessions_223666dee4"))
            : action === "key"
              ? t(...messagePair("disable_key_19208f1779"))
              : t(...messagePair("change_concurrent_limit_e32ebd813e"));
  return (
    <section className="adm-page">
      <Link className="adm-link" to="/admin?tab=customers">
        <ArrowLeft size={14} className="directional" />{" "}
        {t(...messagePair("customer_accounts_9413e58186"))}
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
                    ? t(...messagePair("enabled_c2ea99f45b"))
                    : t(...messagePair("disabled_79b1b987fa"))}
                </span>
              </h1>
              <p>
                <bdi>{user.user_identifier}</bdi> ·{" "}
                {t(...messagePair("registered_6e4cea9360"))}{" "}
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
                    ? t(...messagePair("disable_account_d2f78c3c65"))
                    : t(...messagePair("enable_account_fc76ab91b6"))}
                </Button>
              )}
              {finance && (
                <Button variant="primary" onClick={() => begin("balance")}>
                  {t(...messagePair("adjust_balance_c5b55b6e5d"))}
                </Button>
              )}
            </div>
          </header>
          <div className="adm-detail-grid">
            <article className="adm-card">
              <h2>{t(...messagePair("account_information_bdc1bcbab1"))}</h2>
              <dl className="adm-info">
                {[
                  [
                    t(...messagePair("phone_a90f318a7f")),
                    user.phone_masked || "—",
                  ],
                  [t(...messagePair("email_fd11b7ed4b")), user.email || "—"],
                  [
                    t(...messagePair("balance_e1258fb4af")),
                    adminAmount(user.api_balance_nano),
                  ],
                  [
                    t(...messagePair("monthly_spend_039f31d3cd")),
                    adminAmount(user.monthly_usage_nano),
                  ],
                  [t(...messagePair("keys_98f373eeaf")), user.key_count],
                  [
                    t(...messagePair("last_call_ea054b6e23")),
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
                  {t(...messagePair("status_reason_fcf5a5c53e"))}:{" "}
                  {user.status_reason}
                </p>
              )}
            </article>
            <article className="adm-card">
              <h2>
                {t(...messagePair("concurrent_request_limit_176a964e79"))}
              </h2>
              <p className="adm-meta">
                {t(...messagePair("default_50_concurrent_requests_d79d89b0ae"))}
              </p>
              <div className="adm-count">
                <bdi>{user.concurrency_limit}</bdi>
              </div>
              {write && (
                <Button
                  variant="secondary"
                  onClick={() => begin("concurrency")}
                >
                  {t(...messagePair("change_limit_c80f38fc9c"))}
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
                        t(...messagePair("name_76540d04d3")),
                        t(...messagePair("id_6c3f3567fd")),
                        t(...messagePair("group_16578f28f0")),
                        t(...messagePair("status_05fdd567d6")),
                        t(...messagePair("actions_2806edaa15")),
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
                            ? t(...messagePair("enabled_34fecc2957"))
                            : t(...messagePair("disabled_a0b61d0d86"))}
                        </td>
                        <td>
                          {write && k.status === 1 && (
                            <Button
                              size="small"
                              variant="secondary"
                              onClick={() => begin("key", k)}
                            >
                              {t(...messagePair("disable_6468329ed8"))}
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
                  ...messagePair(
                    "staff_can_inspect_key_identifiers_and_attribu_7ef49755f1",
                  ),
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
                      {t(...messagePair("revoke_all_sessions_de30c90c3e"))}
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
                                t(...messagePair("time_1da26dad48")),
                                t(...messagePair("model_b80025508f")),
                                "Trace ID",
                                t(...messagePair("status_05fdd567d6")),
                                t(...messagePair("actions_2806edaa15")),
                              ]
                            : section === "sessions"
                              ? [
                                  t(...messagePair("created_93a8df1dcf")),
                                  t(...messagePair("expires_adf5d52fbd")),
                                ]
                              : [
                                  t(...messagePair("time_1da26dad48")),
                                  section === "orders"
                                    ? t(...messagePair("order_0ffede7771"))
                                    : t(...messagePair("type_be00a90d64")),
                                  t(...messagePair("amount_3b0bcc85c2")),
                                  section === "orders"
                                    ? t(...messagePair("status_05fdd567d6"))
                                    : t(...messagePair("balance_e1258fb4af")),
                                  t(
                                    ...messagePair("funding_source_fc9f235e9c"),
                                  ),
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
                                    ? t(...messagePair("succeeded_ec2a95382a"))
                                    : t(
                                        ...messagePair(
                                          "unsuccessful_0e799a464a",
                                        ),
                                      )}
                                </td>
                                <td>
                                  <button
                                    className="adm-link"
                                    onClick={() => setCall(r)}
                                  >
                                    {t(...messagePair("details_91cd6ded0d"))}
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
                      <p>{t(...messagePair("no_records_0d134b9bff"))}</p>
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
              {t(...messagePair("cancel_82ac496be8"))}
            </Button>
            <Button
              variant="primary"
              form="admin-customer-action"
              type="submit"
              disabled={mutation.isPending}
            >
              {t(...messagePair("confirm_f262a471a4"))}
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
              {t(...messagePair("new_concurrent_request_limit_0e3b91bf5b"))}
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
                {t(...messagePair("amount_usd_568c286447"))}
                <Input
                  required
                  inputMode="decimal"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                />
              </label>
              <label>
                {t(...messagePair("adjustment_type_89365bbd64"))}
                <select value={kind} onChange={(e) => setKind(e.target.value)}>
                  <option value="offline_payment">
                    {t(...messagePair("offline_payment_e9aa9cce96"))}
                  </option>
                  <option value="gift">
                    {t(...messagePair("gift_ddad176e3f"))}
                  </option>
                  <option value="compensation">
                    {t(...messagePair("compensation_74719de2d0"))}
                  </option>
                  <option value="deduction">
                    {t(...messagePair("deduction_negative_amount_5e9f0fca55"))}
                  </option>
                </select>
              </label>
            </>
          )}
          <label className="adm-wide">
            {t(...messagePair("reason_6879e84de6"))}
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
      title={t(...messagePair("call_details_ab6874b516"))}
      description={call?.trace_id ?? ""}
    >
      {call && (
        <div className="adm-page">
          <dl className="adm-info">
            {[
              [
                t(...messagePair("charge_349f2ff7f6")),
                adminAmount(call.amount_nano),
              ],
              [t(...messagePair("key_d6b66f7d17")), call.token_name],
              [t(...messagePair("vendor_02b6ea9581")), call.vendor || "—"],
              [
                t(...messagePair("call_type_protocol_c1c8ec4cf1")),
                `${call.call_type || "—"} / ${call.protocol || "—"}`,
              ],
              [
                t(...messagePair("cache_read_write_tokens_6564c5a032")),
                `${call.cache_read_tokens ?? 0} / ${call.cache_write_tokens ?? 0}`,
              ],
              [
                t(...messagePair("time_to_first_token_5a815ab455")),
                call.ttft_ms ? `${call.ttft_ms} ms` : "—",
              ],
              [
                t(...messagePair("accepted_33204fd7ab")),
                adminDate(call.created_at, timezone),
              ],
              [
                t(...messagePair("requested_model_e942dbbab4")),
                call.requested_model,
              ],
              [t(...messagePair("executed_model_2907879ddc")), call.model],
              [
                t(...messagePair("service_group_90b4e6ccd2")),
                call.service_group,
              ],
              [
                t(...messagePair("resource_instance_id_fe96718d32")),
                call.resource_id ||
                  call.resource_ids ||
                  t(...messagePair("unrecorded_unmanaged_resource_061fba0c99")),
              ],
              [
                t(...messagePair("admission_snapshot_sequence_f289553111")),
                call.routing_recorded && call.snapshot_sequence > 0
                  ? call.snapshot_sequence
                  : t(...messagePair("historically_unrecorded_2d458508a1")),
              ],
              [
                t(...messagePair("resource_revision_f0a722b115")),
                call.routing_recorded ? call.resource_revision : "—",
              ],
              [
                t(...messagePair("affinity_hit_4bbc4c343d")),
                call.routing_recorded
                  ? call.affinity_used
                    ? t(...messagePair("yes_e6025d7cb9"))
                    : t(...messagePair("no_5d7c678bfd"))
                  : "—",
              ],
              [
                t(...messagePair("input_output_tokens_f807770abd")),
                `${call.prompt_tokens} / ${call.completion_tokens}`,
              ],
              [
                t(...messagePair("duration_8f65bdb42b")),
                `${call.duration_ms} ms`,
              ],
              [
                t(...messagePair("error_code_6c3b677c4d")),
                call.error_code || "—",
              ],
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
              ...messagePair(
                "only_request_and_routing_metadata_are_shown_r_c96b0cda25",
              ),
            )}
          </p>
        </div>
      )}
    </Drawer>
  );
}
