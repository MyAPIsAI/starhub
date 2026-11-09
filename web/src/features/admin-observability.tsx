import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { messagePair } from "../i18n/messages";
import { apiGet } from "../lib/api/http";
import { useSessionQueryIdentity } from "../lib/system-access";
import { Button } from "../components/ui/button";
import { Drawer } from "../components/ui/overlay";
import {
  AdminFeedback,
  AdminPagination,
  AdminTable,
  adminAmount,
  adminDate,
  useAdminText,
} from "./admin-shared";
import "./admin-observability.css";

const copy = {
  nfr_accounting_failed: (values: Record<string, unknown>) =>
    messagePair("nfr_accounting_failed", values),
  nfr_active: (values: Record<string, unknown>) =>
    messagePair("nfr_active", values),
  nfr_active_alerts: (values: Record<string, unknown>) =>
    messagePair("nfr_active_alerts", values),
  nfr_alert_note: (values: Record<string, unknown>) =>
    messagePair("nfr_alert_note", values),
  nfr_alert_records: (values: Record<string, unknown>) =>
    messagePair("nfr_alert_records", values),
  nfr_all: (values: Record<string, unknown>) => messagePair("nfr_all", values),
  nfr_attempts: (values: Record<string, unknown>) =>
    messagePair("nfr_attempts", values),
  nfr_callbacks: (values: Record<string, unknown>) =>
    messagePair("nfr_callbacks", values),
  nfr_channel_id: (values: Record<string, unknown>) =>
    messagePair("nfr_channel_id", values),
  nfr_charge: (values: Record<string, unknown>) =>
    messagePair("nfr_charge", values),
  nfr_checked: (values: Record<string, unknown>) =>
    messagePair("nfr_checked", values),
  nfr_circuits: (values: Record<string, unknown>) =>
    messagePair("nfr_circuits", values),
  nfr_configured: (values: Record<string, unknown>) =>
    messagePair("nfr_configured", values),
  nfr_critical: (values: Record<string, unknown>) =>
    messagePair("nfr_critical", values),
  nfr_delivered: (values: Record<string, unknown>) =>
    messagePair("nfr_delivered", values),
  nfr_delivery: (values: Record<string, unknown>) =>
    messagePair("nfr_delivery", values),
  nfr_delivery_note: (values: Record<string, unknown>) =>
    messagePair("nfr_delivery_note", values),
  nfr_dimension: (values: Record<string, unknown>) =>
    messagePair("nfr_dimension", values),
  nfr_disk: (values: Record<string, unknown>) =>
    messagePair("nfr_disk", values),
  nfr_duration: (values: Record<string, unknown>) =>
    messagePair("nfr_duration", values),
  nfr_failed: (values: Record<string, unknown>) =>
    messagePair("nfr_failed", values),
  nfr_gateway_time: (values: Record<string, unknown>) =>
    messagePair("nfr_gateway_time", values),
  nfr_general: (values: Record<string, unknown>) =>
    messagePair("nfr_general", values),
  nfr_general_channel: (values: Record<string, unknown>) =>
    messagePair("nfr_general_channel", values),
  nfr_hours: (values: Record<string, unknown>) =>
    messagePair("nfr_hours", values),
  nfr_im: (values: Record<string, unknown>) => messagePair("nfr_im", values),
  nfr_instances: (values: Record<string, unknown>) =>
    messagePair("nfr_instances", values),
  nfr_ledger: (values: Record<string, unknown>) =>
    messagePair("nfr_ledger", values),
  nfr_level: (values: Record<string, unknown>) =>
    messagePair("nfr_level", values),
  nfr_log: (values: Record<string, unknown>) => messagePair("nfr_log", values),
  nfr_memory: (values: Record<string, unknown>) =>
    messagePair("nfr_memory", values),
  nfr_metric_note: (values: Record<string, unknown>) =>
    messagePair("nfr_metric_note", values),
  nfr_metrics: (values: Record<string, unknown>) =>
    messagePair("nfr_metrics", values),
  nfr_no_alerts: (values: Record<string, unknown>) =>
    messagePair("nfr_no_alerts", values),
  nfr_no_charge: (values: Record<string, unknown>) =>
    messagePair("nfr_no_charge", values),
  nfr_no_samples: (values: Record<string, unknown>) =>
    messagePair("nfr_no_samples", values),
  nfr_observer: (values: Record<string, unknown>) =>
    messagePair("nfr_observer", values),
  nfr_pending: (values: Record<string, unknown>) =>
    messagePair("nfr_pending", values),
  nfr_pending_ledger: (values: Record<string, unknown>) =>
    messagePair("nfr_pending_ledger", values),
  nfr_pending_notifications: (values: Record<string, unknown>) =>
    messagePair("nfr_pending_notifications", values),
  nfr_pending_settlement: (values: Record<string, unknown>) =>
    messagePair("nfr_pending_settlement", values),
  nfr_pending_task: (values: Record<string, unknown>) =>
    messagePair("nfr_pending_task", values),
  nfr_phone: (values: Record<string, unknown>) =>
    messagePair("nfr_phone", values),
  nfr_posted: (values: Record<string, unknown>) =>
    messagePair("nfr_posted", values),
  nfr_rate_basis: (values: Record<string, unknown>) =>
    messagePair("nfr_rate_basis", values),
  nfr_requests: (values: Record<string, unknown>) =>
    messagePair("nfr_requests", values),
  nfr_requests_label: (values: Record<string, unknown>) =>
    messagePair("nfr_requests_label", values),
  nfr_resolved: (values: Record<string, unknown>) =>
    messagePair("nfr_resolved", values),
  nfr_retry_pending: (values: Record<string, unknown>) =>
    messagePair("nfr_retry_pending", values),
  nfr_rule: (values: Record<string, unknown>) =>
    messagePair("nfr_rule", values),
  nfr_rule_database: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_database", values),
  nfr_rule_gateway: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_gateway", values),
  nfr_rule_ledger: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_ledger", values),
  nfr_rule_model: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_model", values),
  nfr_rule_payment: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_payment", values),
  nfr_rule_platform: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_platform", values),
  nfr_rule_reconciliation: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_reconciliation", values),
  nfr_rule_resources: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_resources", values),
  nfr_rule_settlement: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_settlement", values),
  nfr_rule_snapshot: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_snapshot", values),
  nfr_rule_sync: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_sync", values),
  nfr_rule_vendor: (values: Record<string, unknown>) =>
    messagePair("nfr_rule_vendor", values),
  nfr_scope: (values: Record<string, unknown>) =>
    messagePair("nfr_scope", values),
  nfr_search: (values: Record<string, unknown>) =>
    messagePair("nfr_search", values),
  nfr_serious: (values: Record<string, unknown>) =>
    messagePair("nfr_serious", values),
  nfr_skipped: (values: Record<string, unknown>) =>
    messagePair("nfr_skipped", values),
  nfr_snapshot_age: (values: Record<string, unknown>) =>
    messagePair("nfr_snapshot_age", values),
  nfr_stage_admission: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_admission", values),
  nfr_stage_auth: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_auth", values),
  nfr_stage_gateway: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_gateway", values),
  nfr_stage_ledger: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_ledger", values),
  nfr_stage_metering: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_metering", values),
  nfr_stage_precharge: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_precharge", values),
  nfr_stage_refund: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_refund", values),
  nfr_stage_routing: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_routing", values),
  nfr_stage_settlement: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_settlement", values),
  nfr_stage_upstream: (values: Record<string, unknown>) =>
    messagePair("nfr_stage_upstream", values),
  nfr_state: (values: Record<string, unknown>) =>
    messagePair("nfr_state", values),
  nfr_subtitle: (values: Record<string, unknown>) =>
    messagePair("nfr_subtitle", values),
  nfr_succeeded: (values: Record<string, unknown>) =>
    messagePair("nfr_succeeded", values),
  nfr_success: (values: Record<string, unknown>) =>
    messagePair("nfr_success", values),
  nfr_sync_gap: (values: Record<string, unknown>) =>
    messagePair("nfr_sync_gap", values),
  nfr_time: (values: Record<string, unknown>) =>
    messagePair("nfr_time", values),
  nfr_title: (values: Record<string, unknown>) =>
    messagePair("nfr_title", values),
  nfr_trace_description: (values: Record<string, unknown>) =>
    messagePair("nfr_trace_description", values),
  nfr_trace_id: (values: Record<string, unknown>) =>
    messagePair("nfr_trace_id", values),
  nfr_trace_placeholder: (values: Record<string, unknown>) =>
    messagePair("nfr_trace_placeholder", values),
  nfr_trace_title: (values: Record<string, unknown>) =>
    messagePair("nfr_trace_title", values),
  nfr_ttft: (values: Record<string, unknown>) =>
    messagePair("nfr_ttft", values),
  nfr_unconfigured: (values: Record<string, unknown>) =>
    messagePair("nfr_unconfigured", values),
  nfr_upstream_id: (values: Record<string, unknown>) =>
    messagePair("nfr_upstream_id", values),
  nfr_view_delivery: (values: Record<string, unknown>) =>
    messagePair("nfr_view_delivery", values),
  refresh_b1cae2bbd8: (values: Record<string, unknown>) =>
    messagePair("refresh_b1cae2bbd8", values),
};
type Key = keyof typeof copy;
type Metric = {
  protocol: string;
  model: string;
  vendor: string;
  service_group: string;
  requests: number;
  eligible: number;
  failures: number;
  success_rate: number | null;
  ttft_ms: number;
  duration_ms: number;
  gateway_ms: number;
};
type Alert = {
  id: string;
  rule: string;
  scope: string;
  severity: string;
  active: boolean;
  value: number;
  threshold: number;
  first_seen: number;
  last_seen: number;
  resolved_at: number;
};
type Delivery = {
  id: string;
  alert_id: string;
  channel: string;
  severity: string;
  transition: string;
  created_at: number;
  delivered_at: number;
  attempts: number;
  last_error: string;
};
type Summary = {
  at: number;
  all: Metric[];
  dimensions: Metric[];
  active: Record<string, number>;
  pending_deliveries: number;
  observations: Record<string, { total: number; failed: number }>;
  instances: {
    gateway_id: string;
    at: number;
    cpu_percent: number | null;
    memory_percent: number | null;
    disk_percent: number | null;
  }[];
  routing_states: {
    gateway_id: string;
    at: number;
    open: number;
    degraded: number;
  }[];
  sync: {
    version_gap: number | null;
    snapshot: { age_seconds: number | null };
  };
  notifications: {
    phone_configured: boolean;
    im_configured: boolean;
    general_configured: boolean;
    observer_configured: boolean;
    timezone: string;
    work_start: number;
    work_end: number;
  };
};
type Trace = {
  request: {
    trace_id: string;
    model: string;
    protocol: string;
    vendor: string;
    service_group: string;
    duration_ms: number;
    status: number;
  };
  accounting_status: string;
  charged_nano: string;
  log_id: number;
  ledger_id: number;
  complete: boolean;
  steps: {
    id: string;
    stage: string;
    status: string;
    code?: string;
    started_at: number;
    ended_at: number;
    channel_id?: number;
    upstream_id?: string;
    reference_id?: number;
  }[];
};

const rules: Record<string, Key> = {
  platform_success: "nfr_rule_platform",
  model_success: "nfr_rule_model",
  vendor_success: "nfr_rule_vendor",
  snapshot_age: "nfr_rule_snapshot",
  sync_alignment: "nfr_rule_sync",
  reconciliation_difference: "nfr_rule_reconciliation",
  payment_failure: "nfr_rule_payment",
  ledger_failure: "nfr_rule_ledger",
  settlement_failure: "nfr_rule_settlement",
  gateway_unavailable: "nfr_rule_gateway",
  database_failure: "nfr_rule_database",
  resource_usage: "nfr_rule_resources",
};
const stages: Record<string, Key> = {
  upstream_connect: "nfr_stage_upstream",
  upstream_poll: "nfr_stage_upstream",
  gateway: "nfr_stage_gateway",
  authentication: "nfr_stage_auth",
  admission: "nfr_stage_admission",
  precharge: "nfr_stage_precharge",
  routing: "nfr_stage_routing",
  upstream: "nfr_stage_upstream",
  metering: "nfr_stage_metering",
  settlement: "nfr_stage_settlement",
  ledger: "nfr_stage_ledger",
  refund: "nfr_stage_refund",
};
const accounting: Record<string, Key> = {
  accounting_failed: "nfr_accounting_failed",
  posted: "nfr_posted",
  pending_ledger: "nfr_pending_ledger",
  pending_settlement: "nfr_pending_settlement",
  pending_task: "nfr_pending_task",
  no_charge: "nfr_no_charge",
  not_applicable: "nfr_no_charge",
};
const severity: Record<string, Key> = {
  critical: "nfr_critical",
  serious: "nfr_serious",
  general: "nfr_general",
};
const percent = (value: number | null | undefined) =>
  value == null ? "—" : `${(value * 100).toFixed(2)}%`;

export function AdminObservability() {
  const t = useAdminText(),
    identity = useSessionQueryIdentity(),
    [params] = useSearchParams();
  const text = (key: Key, values: Record<string, unknown> = {}) =>
    t(...copy[key](values));
  const [level, setLevel] = useState(""),
    [state, setState] = useState("active"),
    [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Alert | null>(null);
  const [input, setInput] = useState(params.get("trace") ?? ""),
    [traceID, setTraceID] = useState(params.get("trace") ?? "");
  const [traceOpen, setTraceOpen] = useState(!!params.get("trace"));
  const summary = useQuery({
    queryKey: ["observability", ...identity],
    queryFn: () => apiGet<Summary>("/api/admin/observability"),
    refetchInterval: 10000,
    retry: false,
  });
  const alerts = useQuery({
    queryKey: ["operational-alerts", ...identity, level, state, page],
    queryFn: () =>
      apiGet<{
        items: Alert[];
        deliveries: Delivery[];
        total: number;
        page: number;
        page_size: number;
      }>(`/api/admin/alerts?severity=${level}&state=${state}&page=${page}`),
    refetchInterval: 10000,
    retry: false,
  });
  const trace = useQuery({
    queryKey: ["operational-trace", ...identity, traceID],
    queryFn: () =>
      apiGet<Trace>(`/api/admin/traces/${encodeURIComponent(traceID)}`),
    enabled: traceOpen && !!traceID,
    retry: false,
  });
  const data = summary.isError ? undefined : summary.data;
  const rows = alerts.isError ? undefined : alerts.data;
  const refresh = () => {
    void summary.refetch();
    void alerts.refetch();
  };
  const metric = data?.all[0];
  const badge = (kind: string) => (
    <span className={`nfr-badge nfr-${kind}`}>
      {text(severity[kind] ?? "nfr_general")}
    </span>
  );
  const setting = (name: Key, configured: boolean) => (
    <span className="nfr-channel">
      <span>{text(name)}</span>
      <strong className={configured ? "nfr-good" : "nfr-muted"}>
        {text(configured ? "nfr_configured" : "nfr_unconfigured")}
      </strong>
    </span>
  );
  return (
    <section className="adm-page nfr-page">
      <header className="adm-heading">
        <div>
          <h1>{text("nfr_title")}</h1>
          <p>{text("nfr_subtitle")}</p>
        </div>
        <Button onClick={refresh}>{text("refresh_b1cae2bbd8")}</Button>
      </header>
      <AdminFeedback
        pending={summary.isPending}
        error={summary.error}
        retry={() => void summary.refetch()}
      />
      {data && (
        <>
          <div className="nfr-summary">
            <article className="nfr-card">
              <h2>{text("nfr_success")}</h2>
              <strong className="nfr-number">
                {percent(metric?.success_rate)}
              </strong>
              <p>
                {metric?.eligible
                  ? text("nfr_requests", {
                      count: metric.requests,
                      eligible: metric.eligible,
                    })
                  : text("nfr_no_samples")}
              </p>
              <small>{text("nfr_rate_basis")}</small>
            </article>
            <article className="nfr-card">
              <h2>{text("nfr_active_alerts")}</h2>
              <div className="nfr-severity-counts">
                {["critical", "serious", "general"].map((level) => (
                  <div key={level}>
                    {badge(level)}
                    <strong>{data.active[level] ?? 0}</strong>
                  </div>
                ))}
              </div>
              <p>{text("nfr_checked", { at: adminDate(data.at) })}</p>
            </article>
            <article className="nfr-card">
              <h2>{text("nfr_delivery")}</h2>
              <strong className="nfr-number">{data.pending_deliveries}</strong>
              <p>{text("nfr_pending_notifications")}</p>
              <small>{text("nfr_delivery_note")}</small>
            </article>
          </div>
          <section className="nfr-card nfr-notifications">
            <div className="nfr-channel-list">
              {setting("nfr_phone", data.notifications.phone_configured)}
              {setting("nfr_im", data.notifications.im_configured)}
              {setting(
                "nfr_general_channel",
                data.notifications.general_configured,
              )}
              {setting("nfr_observer", data.notifications.observer_configured)}
            </div>
            <p>
              {text("nfr_hours", {
                zone: data.notifications.timezone,
                start: data.notifications.work_start,
                end: data.notifications.work_end,
              })}
            </p>
          </section>
        </>
      )}
      <section className="nfr-card nfr-alert-list">
        <div className="nfr-section-heading">
          <div>
            <h2>{text("nfr_alert_records")}</h2>
            <p>{text("nfr_alert_note")}</p>
          </div>
          <div className="nfr-filters">
            <label>
              {text("nfr_level")}
              <select
                aria-label={text("nfr_level")}
                value={level}
                onChange={(event) => {
                  setLevel(event.target.value);
                  setPage(1);
                }}
              >
                <option value="">{text("nfr_all")}</option>
                {["critical", "serious", "general"].map((level) => (
                  <option key={level} value={level}>
                    {text(severity[level])}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {text("nfr_state")}
              <select
                aria-label={text("nfr_state")}
                value={state}
                onChange={(event) => {
                  setState(event.target.value);
                  setPage(1);
                }}
              >
                <option value="active">{text("nfr_active")}</option>
                <option value="resolved">{text("nfr_resolved")}</option>
                <option value="">{text("nfr_all")}</option>
              </select>
            </label>
          </div>
        </div>
        <AdminFeedback
          pending={alerts.isPending}
          error={alerts.error}
          retry={() => void alerts.refetch()}
        />
        {rows &&
          (rows.items.length ? (
            <>
              <AdminTable label={text("nfr_alert_records")}>
                <thead>
                  <tr>
                    {[
                      "nfr_level",
                      "nfr_rule",
                      "nfr_scope",
                      "nfr_state",
                      "nfr_time",
                      "nfr_delivery",
                    ].map((key) => (
                      <th key={key}>{text(key as Key)}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.items.map((row) => (
                    <tr key={row.id}>
                      <td>{badge(row.severity)}</td>
                      <td>{text(rules[row.rule] ?? "nfr_alert_records")}</td>
                      <td>
                        <bdi>{row.scope}</bdi>
                      </td>
                      <td>
                        {text(row.active ? "nfr_active" : "nfr_resolved")}
                      </td>
                      <td>
                        {adminDate(
                          row.active ? row.first_seen : row.resolved_at,
                        )}
                      </td>
                      <td>
                        <Button
                          variant="secondary"
                          onClick={() => setSelected(row)}
                        >
                          {text("nfr_view_delivery")}
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </AdminTable>
              <AdminPagination
                page={rows.page}
                total={rows.total}
                size={rows.page_size}
                onChange={setPage}
              />
            </>
          ) : (
            <p className="nfr-empty">{text("nfr_no_alerts")}</p>
          ))}
      </section>
      <section className="nfr-card">
        <h2>{text("nfr_trace_title")}</h2>
        <p>{text("nfr_trace_description")}</p>
        <form
          className="nfr-trace-form"
          onSubmit={(event) => {
            event.preventDefault();
            setTraceID(input.trim());
            setTraceOpen(true);
          }}
        >
          <label htmlFor="nfr-trace">{text("nfr_trace_id")}</label>
          <div>
            <input
              id="nfr-trace"
              dir="ltr"
              value={input}
              onChange={(event) => setInput(event.target.value)}
              required
              minLength={16}
              maxLength={64}
              pattern="[a-zA-Z0-9_.:\-]+"
              autoComplete="off"
              placeholder={text("nfr_trace_placeholder")}
            />
            <Button type="submit">{text("nfr_search")}</Button>
          </div>
        </form>
      </section>
      {data && (
        <details className="nfr-card">
          <summary>{text("nfr_metrics")}</summary>
          <p>{text("nfr_metric_note")}</p>
          <div className="nfr-small-metrics">
            {Object.entries(data.observations).map(([kind, value]) => (
              <div key={kind}>
                <span>
                  {text(
                    (
                      {
                        precharge: "nfr_stage_precharge",
                        settlement: "nfr_stage_settlement",
                        ledger: "nfr_stage_ledger",
                        payment_callback: "nfr_callbacks",
                      } as Record<string, Key>
                    )[kind],
                  )}
                </span>
                <strong>
                  {value.total === 0
                    ? "—"
                    : kind === "payment_callback"
                      ? percent((value.total - value.failed) / value.total)
                      : `${value.failed} / ${value.total}`}
                </strong>
              </div>
            ))}
            <div>
              <span>{text("nfr_sync_gap")}</span>
              <strong>{data.sync.version_gap ?? "—"}</strong>
            </div>
            <div>
              <span>{text("nfr_snapshot_age")}</span>
              <strong>
                {data.sync.snapshot.age_seconds == null
                  ? "—"
                  : `${data.sync.snapshot.age_seconds}s`}
              </strong>
            </div>
          </div>
          <AdminTable label={text("nfr_metrics")}>
            <thead>
              <tr>
                {[
                  "nfr_dimension",
                  "nfr_requests_label",
                  "nfr_success",
                  "nfr_ttft",
                  "nfr_duration",
                  "nfr_gateway_time",
                ].map((key) => (
                  <th key={key}>{text(key as Key)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.dimensions.map((row, index) => (
                <tr key={index}>
                  <td>
                    <bdi>
                      {[row.protocol, row.model, row.vendor, row.service_group]
                        .filter(Boolean)
                        .join(" · ")}
                    </bdi>
                  </td>
                  <td>{row.requests}</td>
                  <td>{percent(row.success_rate)}</td>
                  <td>{row.ttft_ms.toFixed(1)} ms</td>
                  <td>{row.duration_ms.toFixed(1)} ms</td>
                  <td>{row.gateway_ms.toFixed(1)} ms</td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
          <h3>{text("nfr_instances")}</h3>
          {!data.instances.length && <p>{text("nfr_no_samples")}</p>}
          <div className="nfr-instance-grid">
            {data.instances.map((instance) => {
              const routing = data.routing_states.find(
                (state) => state.gateway_id === instance.gateway_id,
              );
              return (
                <article className="nfr-instance" key={instance.gateway_id}>
                  <bdi>{instance.gateway_id}</bdi>
                  <p>{text("nfr_checked", { at: adminDate(instance.at) })}</p>
                  <dl>
                    {[
                      ["CPU", instance.cpu_percent],
                      [text("nfr_memory"), instance.memory_percent],
                      [text("nfr_disk"), instance.disk_percent],
                    ].map(([name, value]) => (
                      <div key={name}>
                        <dt>{name}</dt>
                        <dd>
                          {value == null || data.at - instance.at > 30
                            ? "—"
                            : `${Number(value).toFixed(1)}%`}
                        </dd>
                      </div>
                    ))}
                    <div>
                      <dt>{text("nfr_circuits")}</dt>
                      <dd>
                        {routing && data.at - routing.at <= 15
                          ? `${routing.open} / ${routing.degraded}`
                          : "—"}
                      </dd>
                    </div>
                  </dl>
                </article>
              );
            })}
          </div>
        </details>
      )}
      <Drawer
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
        title={text("nfr_delivery")}
        description={
          selected ? text(rules[selected.rule] ?? "nfr_alert_records") : ""
        }
      >
        {selected && (
          <div className="nfr-page">
            <p>
              {badge(selected.severity)} <bdi>{selected.scope}</bdi>
            </p>
            <p>{text("nfr_delivery_note")}</p>
            {rows?.deliveries
              .filter((delivery) => delivery.alert_id === selected.id)
              .map((delivery) => (
                <article className="nfr-card" key={delivery.id}>
                  <h3>
                    {text(
                      delivery.channel === "phone"
                        ? "nfr_phone"
                        : delivery.channel === "im"
                          ? "nfr_im"
                          : "nfr_general_channel",
                    )}
                  </h3>
                  <strong>
                    {text(
                      delivery.delivered_at ? "nfr_delivered" : "nfr_pending",
                    )}
                  </strong>
                  <p>
                    {text(
                      delivery.transition === "resolved"
                        ? "nfr_resolved"
                        : "nfr_active",
                    )}{" "}
                    · {adminDate(delivery.created_at)}
                  </p>
                  <p>{text("nfr_attempts", { count: delivery.attempts })}</p>
                  {delivery.last_error && <p>{text("nfr_retry_pending")}</p>}
                </article>
              ))}
          </div>
        )}
      </Drawer>
      <Drawer
        open={traceOpen}
        onOpenChange={setTraceOpen}
        title={text("nfr_trace_title")}
        description={text("nfr_trace_id")}
      >
        <AdminFeedback
          pending={trace.isPending}
          error={trace.error}
          retry={() => void trace.refetch()}
        />
        {trace.data && !trace.isError && (
          <div className="nfr-page">
            <div className="nfr-card">
              <p className="nfr-trace-identifier">
                <bdi>{traceID}</bdi>
              </p>
              <strong>
                {text(
                  accounting[trace.data.accounting_status] ?? "nfr_pending",
                )}
              </strong>
              <p>
                <bdi>{trace.data.request.model || "—"}</bdi> ·{" "}
                {trace.data.request.protocol} · HTTP {trace.data.request.status}
              </p>
              <dl className="nfr-trace-facts">
                <div>
                  <dt>{text("nfr_charge")}</dt>
                  <dd>{adminAmount(trace.data.charged_nano)}</dd>
                </div>
                <div>
                  <dt>{text("nfr_log")}</dt>
                  <dd>{trace.data.log_id || "—"}</dd>
                </div>
                <div>
                  <dt>{text("nfr_ledger")}</dt>
                  <dd>{trace.data.ledger_id || "—"}</dd>
                </div>
              </dl>
              <Button variant="secondary" onClick={() => void trace.refetch()}>
                {text("refresh_b1cae2bbd8")}
              </Button>
            </div>
            <ol className="nfr-timeline">
              {trace.data.steps.map((step) => (
                <li key={step.id}>
                  <div>
                    <strong>
                      {text(stages[step.stage] ?? "nfr_trace_title")}
                    </strong>
                    <span
                      className={
                        step.status === "failed" ? "nfr-failure" : "nfr-muted"
                      }
                    >
                      {text(
                        step.status === "failed"
                          ? "nfr_failed"
                          : step.status === "skipped"
                            ? "nfr_skipped"
                            : "nfr_succeeded",
                      )}
                    </span>
                  </div>
                  <p>
                    {adminDate(step.started_at / 1000)} ·{" "}
                    {Math.max(0, step.ended_at - step.started_at)} ms
                  </p>
                  {!!step.channel_id && (
                    <p>
                      {text("nfr_channel_id")} <bdi>{step.channel_id}</bdi>
                    </p>
                  )}
                  {step.upstream_id && (
                    <p>
                      {text("nfr_upstream_id")} <bdi>{step.upstream_id}</bdi>
                    </p>
                  )}
                  {step.code && (
                    <p>
                      <bdi>{step.code}</bdi>
                    </p>
                  )}
                </li>
              ))}
            </ol>
          </div>
        )}
      </Drawer>
    </section>
  );
}
