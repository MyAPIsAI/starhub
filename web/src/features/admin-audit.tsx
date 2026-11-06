import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { apiGet } from "../lib/api/http";
import { useStaffText } from "../lib/api/staff";
import { useSessionQueryIdentity, useSystemAccess } from "../lib/system-access";
import {
  auditActionLabel,
  auditAuthResult,
  auditChanges,
  auditHTTPAction,
  auditObjectLabel,
  auditRoleLabel,
  auditSearchActions,
} from "../lib/audit-display";
import { AdminFeedback, AdminTable } from "./admin-shared";

export interface AuditRow {
  id: number;
  created_at: number;
  user_id: number;
  username: string;
  actor_role: number;
  action: string;
  object_type: string;
  object_id: string;
  object_name?: string;
  before_summary: string;
  after_summary: string;
  reason: string;
  ip: string;
  method: string;
  path: string;
  status: number;
}

function AuditChange({ row }: { row: AuditRow }) {
  const t = useStaffText();
  const changes = auditChanges(row.before_summary, row.after_summary, t);
  const result = auditAuthResult(row.action, t);
  if (!changes.length && result) changes.push(result);
  if (!row.action)
    changes.push(
      `${t(...translatedMessage("result_df85486175"))}: ${row.status >= 200 && row.status < 300 ? t(...translatedMessage("succeeded_adfe5eb8a1")) : t(...translatedMessage("unsuccessful_6a914d7adc"))} (HTTP ${row.status})`,
    );
  let reason = row.reason;
  if (row.user_id === 0 && reason === "Deployment initialization")
    reason = t(...translatedMessage("deployment_initialization_9f45276728"));
  if (row.user_id === 0 && reason === "Deployment-side sole Root 2FA recovery")
    reason = t(
      ...translatedMessage(
        "recover_sole_super_administrator_2fa_through__7f23d1c265",
      ),
    );
  if (reason)
    changes.push(`${t(...translatedMessage("reason_7dbf24f62e"))}: ${reason}`);
  const description = changes.join(" · ") || "—";
  return description.length > 160 || changes.length > 3 ? (
    <details className="adm-change-summary">
      <summary>{description.slice(0, 140)}…</summary>
      <div className="adm-audit-change-lines">
        {changes.map((line, index) => (
          <div key={index}>{line}</div>
        ))}
      </div>
    </details>
  ) : (
    <span className="adm-change-summary">{description}</span>
  );
}

export function AdminAudit() {
  const t = useStaffText(),
    identity = useSessionQueryIdentity(),
    root = useSystemAccess((s) => s.role) === 100;
  const [page, setPage] = useState(1),
    [actor, setActor] = useState(""),
    [action, setAction] = useState("");
  const [input, setInput] = useState(""),
    [search, setSearch] = useState("");
  const [range, setRange] = useState("all"),
    [anchor, setAnchor] = useState(() => Date.now());
  const [from, setFrom] = useState(""),
    [until, setUntil] = useState("");
  useEffect(() => {
    if (input.trim() === search) return;
    const timer = setTimeout(() => {
      setSearch(input.trim());
      setPage(1);
    }, 250);
    return () => clearTimeout(timer);
  }, [input, search]);
  const params = new URLSearchParams({ p: String(page), page_size: "20" });
  if (root && actor) params.set("actor", actor);
  if (action) params.set("action", action);
  if (search) {
    params.set("q", search);
    auditSearchActions(search, t).forEach((code) =>
      params.append("search_action", code),
    );
  }
  if (range !== "all" && range !== "custom")
    params.set(
      "from",
      String(Math.floor(anchor / 1000) - Number(range) * 86400),
    );
  if (range === "custom") {
    if (from)
      params.set(
        "from",
        String(Math.floor(Date.parse(`${from}T00:00:00Z`) / 1000)),
      );
    if (until)
      params.set(
        "until",
        String(Math.floor(Date.parse(`${until}T00:00:00Z`) / 1000) + 86400),
      );
  }
  const query = useQuery({
    queryKey: ["admin-audit", ...identity, params.toString()],
    queryFn: ({ signal }) =>
      apiGet<{
        items: AuditRow[];
        total: number;
        actions: string[];
        actors: { id: number; username: string }[];
      }>(`/api/audit?${params}`, signal),
    retry: false,
  });
  const change = (fn: () => void) => {
    fn();
    setPage(1);
  };
  const total = query.data?.total ?? 0,
    pages = Math.max(1, Math.ceil(total / 20));
  useEffect(() => {
    if (query.data && page > pages) setPage(pages);
  }, [query.data, page, pages]);
  return (
    <section className="adm-page adm-audit-page">
      <header className="adm-heading">
        <div>
          <h1>{t(...translatedMessage("audit_log_f73fcc381d"))}</h1>
          <p>
            {t(
              ...translatedMessage(
                "sensitive_back_office_operations_without_pass_d82a9eed65",
              ),
            )}
          </p>
        </div>
      </header>
      <div className="adm-filters adm-audit-filters">
        <label className="adm-audit-search">
          <Search size={15} aria-hidden="true" />
          <Input
            type="search"
            maxLength={200}
            value={input}
            onChange={(event) => setInput(event.target.value)}
            aria-label={t(
              ...translatedMessage("search_audit_records_17d877e97b"),
            )}
            placeholder={t(
              ...translatedMessage(
                "search_objects_actions_or_content_fcd4450888",
              ),
            )}
          />
        </label>
        <select
          aria-label={t(...translatedMessage("action_e1ba7109ce"))}
          value={action}
          onChange={(e) => change(() => setAction(e.target.value))}
        >
          <option value="">
            {t(...translatedMessage("all_actions_c574eadae5"))}
          </option>
          {query.data?.actions.filter(Boolean).map((code) => (
            <option key={code} value={code}>
              {auditActionLabel(code, t)}
            </option>
          ))}
        </select>
        {root ? (
          <select
            aria-label={t(...translatedMessage("actor_d343f22c5e"))}
            value={actor}
            onChange={(e) => change(() => setActor(e.target.value))}
          >
            <option value="">
              {t(...translatedMessage("all_actors_88d93d26d9"))}
            </option>
            {query.data?.actors.map((item) => (
              <option key={`${item.id}/${item.username}`} value={item.id}>
                {item.id === 0
                  ? t(...translatedMessage("deployment_terminal_1e6435a529"))
                  : item.username}
              </option>
            ))}
          </select>
        ) : (
          <span className="adm-meta">
            {t(...translatedMessage("your_actions_only_339c9c26cd"))}
          </span>
        )}
        <select
          aria-label={t(...translatedMessage("time_range_31eaa8de19"))}
          value={range}
          onChange={(e) =>
            change(() => {
              setRange(e.target.value);
              setAnchor(Date.now());
            })
          }
        >
          <option value="all">
            {t(...translatedMessage("all_time_4f27bb07c8"))}
          </option>
          <option value="1">
            {t(...translatedMessage("last_24_hours_de312c887e"))}
          </option>
          <option value="7">
            {t(...translatedMessage("last_7_days_27d531a444"))}
          </option>
          <option value="30">
            {t(...translatedMessage("last_30_days_df52873ad5"))}
          </option>
          <option value="custom">
            {t(...translatedMessage("custom_range_11a7838f93"))}
          </option>
        </select>
        {range === "custom" && (
          <>
            <label>
              {t(...translatedMessage("from_7a16d6bf6a"))}
              <Input
                type="date"
                value={from}
                max={until || undefined}
                onChange={(e) => change(() => setFrom(e.target.value))}
              />
            </label>
            <label>
              {t(...translatedMessage("until_d233f3ca3c"))}
              <Input
                type="date"
                value={until}
                min={from || undefined}
                onChange={(e) => change(() => setUntil(e.target.value))}
              />
            </label>
          </>
        )}
      </div>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {query.data && (
        <div className="adm-audit-results">
          <AdminTable label={t(...translatedMessage("audit_log_f73fcc381d"))}>
            <thead>
              <tr>
                {[
                  t(...translatedMessage("time_d70a9315f6")),
                  t(...translatedMessage("actor_d343f22c5e")),
                  t(...translatedMessage("action_631108cb6b")),
                  t(...translatedMessage("object_091cb607a9")),
                  t(...translatedMessage("change_summary_dcdcbe7961")),
                  t(...translatedMessage("ip_address_729ec1ec0c")),
                ].map((heading) => (
                  <th key={heading}>{heading}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {query.data.items.map((row) => (
                <tr key={row.id}>
                  <td>
                    <time
                      className="adm-audit-time"
                      dateTime={new Date(row.created_at * 1000).toISOString()}
                    >
                      {new Date(row.created_at * 1000)
                        .toISOString()
                        .slice(0, 19)
                        .replace("T", " ")}
                    </time>
                  </td>
                  <td>
                    <div className="adm-audit-actor">
                      <bdi>
                        {row.user_id === 0
                          ? t(
                              ...translatedMessage(
                                "deployment_terminal_1e6435a529",
                              ),
                            )
                          : row.username || "—"}
                      </bdi>
                      <span className="adm-audit-role">
                        {auditRoleLabel(row.actor_role, t)}
                      </span>
                    </div>
                  </td>
                  <td title={row.action || `${row.method} ${row.status}`}>
                    {row.action
                      ? auditActionLabel(row.action, t)
                      : auditHTTPAction(row.method, t)}
                  </td>
                  <td>
                    <bdi className="adm-audit-object">
                      {row.object_name || row.object_id || row.path || "—"}
                    </bdi>
                    {row.object_type && (
                      <small>
                        {auditObjectLabel(row.object_type, t)}
                        {row.object_name &&
                        row.object_id &&
                        row.object_name !== row.object_id ? (
                          <>
                            {" "}
                            · <bdi>#{row.object_id}</bdi>
                          </>
                        ) : null}
                      </small>
                    )}
                  </td>
                  <td>
                    <AuditChange row={row} />
                  </td>
                  <td>
                    <bdi className="adm-audit-ip">{row.ip || "—"}</bdi>
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
          {!total && (
            <p className="adm-audit-empty">
              {t(...translatedMessage("no_matching_audit_records_dff7ef9103"))}
            </p>
          )}
          <footer className="adm-audit-pagination">
            <span>
              {total
                ? `${t(...translatedMessage("showing_a3e16ce64b"))} ${(page - 1) * 20 + 1}–${Math.min(page * 20, total)} ${t(...translatedMessage("of_f3dfd279ea"))} ${total} ${t(...translatedMessage("records_199d09a9b4"))}`
                : t(...translatedMessage("0_records_3619f4daa0"))}
            </span>
            <div>
              <Button
                variant="ghost"
                disabled={page <= 1 || query.isFetching}
                onClick={() => setPage((value) => value - 1)}
              >
                {t(...translatedMessage("previous_2672646f39"))}
              </Button>
              <span>
                {page} / {pages}
              </span>
              <Button
                variant="ghost"
                disabled={page >= pages || query.isFetching}
                onClick={() => setPage((value) => value + 1)}
              >
                {t(...translatedMessage("next_c0fd39244f"))}
              </Button>
            </div>
          </footer>
        </div>
      )}
    </section>
  );
}
