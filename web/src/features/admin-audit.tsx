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
      `${t("操作结果", "Result", "النتيجة")}: ${row.status >= 200 && row.status < 300 ? t("成功", "Succeeded", "نجحت") : t("未成功", "Unsuccessful", "لم تنجح")} (HTTP ${row.status})`,
    );
  let reason = row.reason;
  if (row.user_id === 0 && reason === "Deployment initialization")
    reason = t("部署初始化", "Deployment initialization", "تهيئة النشر");
  if (row.user_id === 0 && reason === "Deployment-side sole Root 2FA recovery")
    reason = t(
      "通过部署终端恢复唯一超级管理员的双重验证",
      "Recover sole super administrator 2FA through deployment terminal",
      "استعادة المصادقة الثنائية للمسؤول الأعلى الوحيد عبر طرفية النشر",
    );
  if (reason) changes.push(`${t("原因", "Reason", "السبب")}: ${reason}`);
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
          <h1>{t("审计日志", "Audit log", "سجل التدقيق")}</h1>
          <p>
            {t(
              "记录后台敏感操作，不记录密码、完整密钥或凭证。保留最近一年记录，时间按 UTC 显示。",
              "Sensitive back-office operations, without passwords, full keys or credentials. Records are retained for one year; times are shown in UTC.",
              "عمليات الإدارة الحساسة دون كلمات المرور أو المفاتيح الكاملة أو بيانات الاعتماد. تُحفظ السجلات لعام واحد وتُعرض الأوقات بالتوقيت العالمي UTC.",
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
              "搜索审计记录",
              "Search audit records",
              "البحث في سجلات التدقيق",
            )}
            placeholder={t(
              "搜索对象、操作或内容",
              "Search objects, actions or content",
              "البحث عن كائن أو إجراء أو محتوى",
            )}
          />
        </label>
        <select
          aria-label={t("操作类型", "Action", "الإجراء")}
          value={action}
          onChange={(e) => change(() => setAction(e.target.value))}
        >
          <option value="">
            {t("全部操作", "All actions", "كل الإجراءات")}
          </option>
          {query.data?.actions.filter(Boolean).map((code) => (
            <option key={code} value={code}>
              {auditActionLabel(code, t)}
            </option>
          ))}
        </select>
        {root ? (
          <select
            aria-label={t("操作人", "Actor", "المنفّذ")}
            value={actor}
            onChange={(e) => change(() => setActor(e.target.value))}
          >
            <option value="">
              {t("全部操作人", "All actors", "كل المنفّذين")}
            </option>
            {query.data?.actors.map((item) => (
              <option key={`${item.id}/${item.username}`} value={item.id}>
                {item.id === 0
                  ? t("部署终端", "Deployment terminal", "طرفية النشر")
                  : item.username}
              </option>
            ))}
          </select>
        ) : (
          <span className="adm-meta">
            {t("仅显示本人的操作", "Your actions only", "إجراءاتك فقط")}
          </span>
        )}
        <select
          aria-label={t("时间范围", "Time range", "الفترة الزمنية")}
          value={range}
          onChange={(e) =>
            change(() => {
              setRange(e.target.value);
              setAnchor(Date.now());
            })
          }
        >
          <option value="all">{t("全部时间", "All time", "كل الأوقات")}</option>
          <option value="1">
            {t("最近 24 小时", "Last 24 hours", "آخر 24 ساعة")}
          </option>
          <option value="7">
            {t("最近 7 天", "Last 7 days", "آخر 7 أيام")}
          </option>
          <option value="30">
            {t("最近 30 天", "Last 30 days", "آخر 30 يومًا")}
          </option>
          <option value="custom">
            {t("自定义时间", "Custom range", "فترة مخصصة")}
          </option>
        </select>
        {range === "custom" && (
          <>
            <label>
              {t("开始日期", "From", "من")}
              <Input
                type="date"
                value={from}
                max={until || undefined}
                onChange={(e) => change(() => setFrom(e.target.value))}
              />
            </label>
            <label>
              {t("结束日期", "Until", "إلى")}
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
          <AdminTable label={t("审计日志", "Audit log", "سجل التدقيق")}>
            <thead>
              <tr>
                {[
                  t("时间", "Time", "الوقت"),
                  t("操作人", "Actor", "المنفّذ"),
                  t("动作", "Action", "الإجراء"),
                  t("对象", "Object", "الكائن"),
                  t("变更摘要", "Change summary", "ملخص التغيير"),
                  t("IP（地址）", "IP address", "عنوان IP"),
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
                          ? t("部署终端", "Deployment terminal", "طرفية النشر")
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
              {t(
                "没有匹配的审计记录。",
                "No matching audit records.",
                "لا توجد سجلات تدقيق مطابقة.",
              )}
            </p>
          )}
          <footer className="adm-audit-pagination">
            <span>
              {total
                ? `${t("第", "Showing", "عرض")} ${(page - 1) * 20 + 1}–${Math.min(page * 20, total)} ${t("条，共", "of", "من")} ${total} ${t("条", "records", "سجلًا")}`
                : t("共 0 条", "0 records", "0 سجلات")}
            </span>
            <div>
              <Button
                variant="ghost"
                disabled={page <= 1 || query.isFetching}
                onClick={() => setPage((value) => value - 1)}
              >
                {t("上一页", "Previous", "السابق")}
              </Button>
              <span>
                {page} / {pages}
              </span>
              <Button
                variant="ghost"
                disabled={page >= pages || query.isFetching}
                onClick={() => setPage((value) => value + 1)}
              >
                {t("下一页", "Next", "التالي")}
              </Button>
            </div>
          </footer>
        </div>
      )}
    </section>
  );
}
