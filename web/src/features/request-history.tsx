import { useSearchParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  CalendarDays,
  Download,
  Search,
  Check,
  CircleAlert,
  ChevronLeft,
  ChevronRight,
  RotateCcw,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { TierBadge } from "../components/tier-badge";
import { formatMoney } from "../lib/money";
import type { PreviewRequest } from "../lib/product-preview";
import {
  dubaiDate,
  filterRequestHistory,
  type RequestTimeRange,
  type RequestHistoryStatus,
} from "../lib/request-history";
import "./request-history.css";
import type { LogPageSummary } from "../lib/api/usage";

const ranges: RequestTimeRange[] = ["today", "7d", "30d", "custom", "all"];
const statuses: RequestHistoryStatus[] = [
  "all",
  "success",
  "failed",
  "cancelled",
];
const PAGE_SIZE = 20;
export default function RequestHistory({
  requests,
  onSelect,
  onExport,
  server,
}: {
  requests: PreviewRequest[];
  onSelect: (row: PreviewRequest) => void;
  onExport: () => void;
  server?: {
    total: number;
    page: number;
    summary?: LogPageSummary;
    loading: boolean;
    error: unknown;
    refetch: () => unknown;
  };
}) {
  const { t, i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [params, setParams] = useSearchParams();
  const rawRange = params.get("range") as RequestTimeRange;
  const range = ranges.includes(rawRange) ? rawRange : "7d";
  const rawStatus = params.get("status") as RequestHistoryStatus;
  const status = statuses.includes(rawStatus) ? rawStatus : "all";
  const query = params.get("query") ?? "";
  const start = params.get("start") ?? "",
    end = params.get("end") ?? "";
  const result = filterRequestHistory(requests, {
    range,
    start,
    end,
    status,
    query,
  });
  const total = server ? server.total : result.rows.length;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const requestedPage = Number(params.get("page"));
  const page = server
    ? server.page
    : Number.isInteger(requestedPage)
      ? Math.max(1, Math.min(pages, requestedPage))
      : 1;
  const visibleRows = server
    ? result.error || server.error || server.loading
      ? []
      : requests
    : result.rows.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
  const change = (updates: Record<string, string | null>, replace = false) => {
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        next.delete("page");
        Object.entries(updates).forEach(([key, value]) =>
          value ? next.set(key, value) : next.delete(key),
        );
        return next;
      },
      { replace },
    );
  };
  const clear = () =>
    change({ range: "all", start: null, end: null, query: null, status: null });
  const hasFilters = range !== "all" || status !== "all" || !!query.trim();
  const rangeLabel = {
    today: c("今日", "Today", "اليوم"),
    "7d": c("近 7 天", "Last 7 days", "آخر 7 أيام"),
    "30d": c("近 30 天", "Last 30 days", "آخر 30 يومًا"),
    custom: c("自定义日期", "Custom dates", "تواريخ مخصصة"),
    all: c("全部时间", "All time", "كل الأوقات"),
  };
  const errorText =
    result.error === "missing_dates"
      ? c(
          "请选择完整的开始和结束日期。",
          "Choose both a start and end date.",
          "اختر تاريخ البداية والنهاية.",
        )
      : result.error === "invalid_date"
        ? c(
            "日期无效，请重新选择。",
            "Invalid date. Choose a valid date.",
            "التاريخ غير صالح. اختر تاريخًا صالحًا.",
          )
        : c(
            "结束日期不能早于开始日期。",
            "The end date cannot precede the start date.",
            "لا يمكن أن يسبق تاريخ النهاية تاريخ البداية.",
          );
  return (
    <section className="request-history">
      <header className="request-history-header">
        <div>
          <h1>{t("pUsage")}</h1>
          <p>{t("requestTableNote")}</p>
        </div>
        <div className="request-export-action">
          <Button size="small" variant="secondary" onClick={onExport}>
            <Download size={14} />
            {c("创建导出任务", "Create export task", "إنشاء مهمة تصدير")}
          </Button>
          <small>
            {c(
              "另选协调世界时范围，不沿用本页筛选",
              "Select an independent UTC export range",
              "يُحدد نطاق UTC للتصدير بشكل مستقل",
            )}
          </small>
        </div>
      </header>
      <div className="request-history-panel">
        {server?.loading && (
          <p role="status">
            {c("正在读取调用记录…", "Loading requests…", "جارٍ تحميل الطلبات…")}
          </p>
        )}
        {server?.error != null && (
          <p role="alert">
            {server.error instanceof Error &&
            server.error.message === "request_log_filters_unavailable"
              ? c(
                  "当前后端尚不支持调用记录筛选，请更新并重启后端。",
                  "Update and restart the backend to enable request filtering.",
                  "حدّث الخادم وأعد تشغيله لتفعيل تصفية الطلبات.",
                )
              : c(
                  "读取调用记录失败，请重试。",
                  "Could not load requests. Try again.",
                  "تعذر تحميل الطلبات. حاول مجددًا.",
                )}
            <Button onClick={() => void server.refetch()}>
              {c("重试", "Retry", "إعادة المحاولة")}
            </Button>
          </p>
        )}
        <div className="request-history-toolbar">
          {server && (
            <Button
              disabled={server.loading || !!result.error}
              onClick={() => void server.refetch()}
            >
              {c("刷新", "Refresh", "تحديث")}
            </Button>
          )}
          <label className="request-history-search">
            <span className="sr-only">
              {c("搜索调用", "Search requests", "بحث الطلبات")}
            </span>
            <Search size={15} aria-hidden="true" />
            <input
              type="search"
              aria-label={c("搜索调用", "Search requests", "بحث الطلبات")}
              value={query}
              maxLength={256}
              onChange={(e) => change({ query: e.target.value }, true)}
              placeholder={c(
                "搜索业务编号、模型或错误代码",
                "Search ID, model or error code",
                "بحث المعرّف أو النموذج أو الخطأ",
              )}
            />
          </label>
          <label className="request-history-field">
            <CalendarDays size={15} aria-hidden="true" />
            <span className="sr-only">
              {c("调用时间", "Request date range", "الفترة الزمنية للطلبات")}
            </span>
            <select
              className="select-trigger"
              aria-label={c(
                "调用时间",
                "Request date range",
                "الفترة الزمنية للطلبات",
              )}
              value={range}
              onChange={(e) =>
                change({
                  range: e.target.value,
                  ...(e.target.value === "custom"
                    ? { start: start || dubaiDate(), end: end || dubaiDate() }
                    : {}),
                })
              }
            >
              {ranges.map((value) => (
                <option key={value} value={value}>
                  {rangeLabel[value]}
                </option>
              ))}
            </select>
          </label>
          <label className="request-history-status">
            <span className="sr-only">
              {c("调用状态", "Request status", "حالة الطلب")}
            </span>
            <select
              className="select-trigger"
              aria-label={c("调用状态", "Request status", "حالة الطلب")}
              value={status}
              onChange={(e) => change({ status: e.target.value })}
            >
              <option value="all">
                {c("全部状态", "All statuses", "كل الحالات")}
              </option>
              <option value="success">{c("成功", "Success", "نجاح")}</option>
              <option value="failed">{c("失败", "Failed", "فشل")}</option>
              <option value="cancelled">
                {c("已取消", "Cancelled", "ملغى")}
              </option>
            </select>
          </label>
          {hasFilters && (
            <button className="request-clear-button" onClick={clear}>
              <RotateCcw size={13} />
              {c("清空筛选", "Clear filters", "مسح المرشحات")}
            </button>
          )}
        </div>
        {range === "custom" && (
          <div className="request-custom-dates">
            <label>
              {c("开始日期", "Start date", "تاريخ البداية")}
              <input
                type="date"
                aria-label={c("开始日期", "Start date", "تاريخ البداية")}
                aria-invalid={!!result.error}
                aria-describedby={
                  result.error ? "request-date-error" : undefined
                }
                value={start}
                onChange={(e) => change({ start: e.target.value || null })}
              />
            </label>
            <span className="request-date-separator" aria-hidden="true">
              —
            </span>
            <label>
              {c("结束日期", "End date", "تاريخ النهاية")}
              <input
                type="date"
                aria-label={c("结束日期", "End date", "تاريخ النهاية")}
                aria-invalid={!!result.error}
                aria-describedby={
                  result.error ? "request-date-error" : undefined
                }
                value={end}
                onChange={(e) => change({ end: e.target.value || null })}
              />
            </label>
            <span>
              {c(
                "包含起止日期的完整一天",
                "Includes both full calendar days",
                "يشمل يومَي البداية والنهاية كاملين",
              )}
            </span>
          </div>
        )}
        {result.error && (
          <p
            className="request-date-error"
            id="request-date-error"
            role="alert"
          >
            <CircleAlert size={14} />
            {errorText}
          </p>
        )}
        <div className="request-history-summary">
          <span aria-live="polite">
            {c(`共 ${total} 条调用`, `${total} requests`, `${total} طلبًا`)}
            {!server && requests.length !== result.rows.length && (
              <small>
                {c(
                  ` / 全部 ${requests.length} 条`,
                  ` / ${requests.length} total`,
                  ` / ${requests.length} إجمالًا`,
                )}
              </small>
            )}
          </span>
          <span>
            {c(
              "日期与时间均按迪拜时区（世界协调时 +4）",
              "Dates and times use Dubai (UTC+4)",
              "التواريخ والأوقات بتوقيت دبي (UTC+4)",
            )}
          </span>
        </div>
        {server?.summary && (
          <div className="request-history-summary" aria-live="polite">
            <span>
              {c(
                `当前筛选：已完成入口请求 ${server.summary.observed_requests} · 上游发送前拒绝或失败 ${server.summary.rejected_requests} · 上游尝试 ${server.summary.upstream_attempts}`,
                `Current filter: completed incoming requests ${server.summary.observed_requests} · rejected or failed before upstream ${server.summary.rejected_requests} · upstream attempts ${server.summary.upstream_attempts}`,
                `التصفية الحالية: الطلبات الواردة المكتملة ${server.summary.observed_requests} · المرفوضة أو الفاشلة قبل الإرسال ${server.summary.rejected_requests} · محاولات المزوّد ${server.summary.upstream_attempts}`,
              )}
            </span>
            <small>
              {c(
                `成功 ${server.summary.success_count} · 失败 ${server.summary.failed_count} · 取消 ${server.summary.cancelled_count}。另有 ${server.summary.historical_requests} 条旧记录未采集入口与尝试次数。`,
                `Succeeded ${server.summary.success_count} · failed ${server.summary.failed_count} · cancelled ${server.summary.cancelled_count}. ${server.summary.historical_requests} older records lack admission and attempt counts.`,
                `ناجحة ${server.summary.success_count} · فاشلة ${server.summary.failed_count} · ملغاة ${server.summary.cancelled_count}. تفتقر ${server.summary.historical_requests} سجلات قديمة إلى أعداد الدخول والمحاولات.`,
              )}
            </small>
          </div>
        )}
        {visibleRows.length > 0 && (
          <div className="request-history-table-wrap">
            <table className="request-history-table">
              <thead>
                <tr>
                  <th>
                    {c("时间（迪拜时区）", "Time (UTC+4)", "الوقت (UTC+4)")}
                  </th>
                  <th>{c("业务编号", "Trace ID", "معرّف الطلب")}</th>
                  <th>
                    {c(
                      "请求模型 → 实际模型",
                      "Requested → actual model",
                      "المطلوب → الفعلي",
                    )}
                  </th>
                  <th className="request-channel-id" scope="col">
                    {c("渠道 ID", "Channel ID", "معرّف القناة")}
                  </th>
                  <th>{c("厂商", "Vendor", "المزوّد")}</th>
                  <th>{c("分组", "Group", "المجموعة")}</th>
                  <th>{c("状态", "Status", "الحالة")}</th>
                  <th className="request-number">
                    {c("耗时", "Duration", "المدة")}
                  </th>
                  <th className="request-number">
                    {c("费用（美元）", "Cost (USD)", "التكلفة (دولار)")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {visibleRows.map((row) => (
                  <tr key={row.trace_id} onClick={() => onSelect(row)}>
                    <td className="mono">
                      {new Date(row.time).toLocaleString("en-GB", {
                        timeZone: "Asia/Dubai",
                        hour12: false,
                      })}
                    </td>
                    <td className="mono">
                      <button
                        className="request-trace-button"
                        onClick={(e) => {
                          e.stopPropagation();
                          onSelect(row);
                        }}
                      >
                        {row.trace_id}
                      </button>
                    </td>
                    <td>
                      <strong>{row.requested_model || row.model || "—"}</strong>
                      <small>
                        →{" "}
                        {row.actual_model ||
                          (row.status === "success" && row.channel_id
                            ? row.model
                            : "—")}
                      </small>
                    </td>
                    <td className="mono request-channel-id">
                      <bdi>{row.channel_id || "—"}</bdi>
                    </td>
                    <td>{row.vendor}</td>
                    <td>{row.tier ? <TierBadge id={row.tier} /> : "—"}</td>
                    <td>
                      <span
                        className={`result-pill ${row.status === "failed" ? "is-failed" : ""}`}
                      >
                        {row.status === "failed" ? (
                          <CircleAlert size={13} />
                        ) : (
                          <Check size={13} />
                        )}
                        {row.outcome === "cancelled"
                          ? c("已取消", "Cancelled", "ملغى")
                          : t(
                              row.status === "failed"
                                ? "pFailed"
                                : "pSucceeded",
                            )}
                      </span>
                      {row.http_status && <small>HTTP {row.http_status}</small>}
                      {row.error_code && (
                        <small>
                          <bdi>{row.error_code}</bdi>
                        </small>
                      )}
                    </td>
                    <td className="mono request-number">
                      {row.duration_ms} {c("毫秒", "ms", "مللي ثانية")}
                    </td>
                    <td className="mono request-number request-cost">
                      {formatMoney(row.cost_nano, "call")}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {!visibleRows.length && !server?.loading && !server?.error && (
          <div className="request-empty" role="status">
            {result.error ? (
              <>
                <strong>
                  {c(
                    "请检查日期范围",
                    "Check the date range",
                    "تحقق من الفترة الزمنية",
                  )}
                </strong>
                <p>{errorText}</p>
              </>
            ) : !requests.length && !hasFilters && page === 1 ? (
              <>
                <strong>
                  {c("还没有调用记录", "No requests yet", "لا توجد طلبات بعد")}
                </strong>
                <p>
                  {c(
                    "接入工具后，可在这里核对费用与异常。",
                    "Connect a tool to review costs and errors here.",
                    "اربط أداة لمراجعة التكاليف والأخطاء هنا.",
                  )}
                </p>
                <Link to="/console/connect">
                  {c("前往接入", "Connect a tool", "ربط أداة")}
                </Link>
              </>
            ) : (
              <>
                <strong>
                  {c(
                    "没有符合筛选条件的调用",
                    "No matching requests",
                    "لا توجد طلبات مطابقة",
                  )}
                </strong>
                <p>
                  {c(
                    "调整时间、状态或搜索词，已有记录不会被删除。",
                    "Adjust the date, status or search. Your records are unchanged.",
                    "عدّل التاريخ أو الحالة أو البحث. لم تتغير سجلاتك.",
                  )}
                </p>
                <Button size="small" variant="secondary" onClick={clear}>
                  {c(
                    "清空筛选，查看全部",
                    "Clear filters and show all",
                    "مسح المرشحات وعرض الكل",
                  )}
                </Button>
              </>
            )}
          </div>
        )}
        <footer className="request-history-pagination">
          <span>
            {total > 0 && visibleRows.length > 0
              ? c(
                  `第 ${(page - 1) * PAGE_SIZE + 1}–${Math.min(page * PAGE_SIZE, total)} 条，共 ${total} 条`,
                  `${(page - 1) * PAGE_SIZE + 1}–${Math.min(page * PAGE_SIZE, total)} of ${total}`,
                  `${(page - 1) * PAGE_SIZE + 1}–${Math.min(page * PAGE_SIZE, total)} من ${total}`,
                )
              : c("0 条记录", "0 records", "0 سجلات")}
          </span>
          <div>
            <Button
              size="small"
              variant="secondary"
              disabled={page <= 1 || server?.loading}
              aria-label={c("上一页", "Previous page", "الصفحة السابقة")}
              onClick={() => change({ page: String(page - 1) })}
            >
              <ChevronLeft size={14} />
            </Button>
            <span>
              {page} / {pages}
            </span>
            <Button
              size="small"
              variant="secondary"
              disabled={
                page >= pages ||
                server?.loading ||
                !!server?.error ||
                !!result.error
              }
              aria-label={c("下一页", "Next page", "الصفحة التالية")}
              onClick={() => change({ page: String(page + 1) })}
            >
              <ChevronRight size={14} />
            </Button>
          </div>
        </footer>
      </div>
    </section>
  );
}
