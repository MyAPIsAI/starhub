import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Download } from "lucide-react";
import { Button } from "../components/ui/button";
import { useUsageReport, type UsageStats } from "../lib/api/usage-report";
import { useCreateExport } from "../lib/api/exports";
import { formatMoney } from "../lib/money";
import { billingDate, validateBillingRange } from "../lib/billing-export";
import { writtenLanguage } from "../lib/localize";
import { callTypeLabel } from "../lib/request-metadata";
import {
  isUsageReport,
  UsageReportContractError,
} from "../lib/usage-report-contract";
import "../components/usage-reports.css";

export default function UsageReports() {
  const { i18n } = useTranslation();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language.startsWith("zh")
      ? zh
      : i18n.language.startsWith("ar")
        ? ar
        : en;
  const [metric, setMetric] = useState("cost"),
    [error, setError] = useState("");
  const createExport = useCreateExport();
  const query = useUsageReport(params.toString());
  const invalidReport = query.data !== undefined && !isUsageReport(query.data);
  const report = query.isError || invalidReport ? undefined : query.data;
  const reportError =
    invalidReport || query.error instanceof UsageReportContractError
      ? c(
          "报表接口与当前后端版本不兼容，请更新并重启后端后重试。",
          "The report API is incompatible with this backend. Update and restart the backend, then retry.",
          "واجهة التقارير غير متوافقة مع هذا الخادم. حدّث الخادم وأعد تشغيله ثم حاول مجددًا.",
        )
      : query.error?.message;
  const range = params.get("range") ?? "7d",
    dimension = params.get("group_by") ?? "key";
  const labels: Record<string, string> = {
    key: c("密钥", "Key", "المفتاح"),
    model: c("模型", "Model", "النموذج"),
    vendor: c("厂商", "Vendor", "المزوّد"),
    call_type: c("调用类型", "Call type", "نوع الطلب"),
    group: c("服务分组", "Service group", "مجموعة الخدمة"),
  };
  const change = (name: string, value: string) => {
    setError("");
    setParams((old) => {
      const next = new URLSearchParams(old);
      value ? next.set(name, value) : next.delete(name);
      return next;
    });
  };
  const percent = (s: UsageStats) =>
    s.requests ? `${((s.successful / s.requests) * 100).toFixed(2)}%` : "—";
  const share = (value: string, total: string) =>
    BigInt(total) > 0n
      ? Number((BigInt(value) * 10000n) / BigInt(total)) / 100
      : 0;
  const exportReport = () => {
    if (!report) return;
    if (!validateBillingRange(report.start_date, report.end_date)) {
      setError(
        c(
          "单份导出最多 31 天，请缩小时间范围。",
          "Exports allow up to 31 days; choose a shorter range.",
          "حد التصدير 31 يومًا؛ اختر فترة أقصر.",
        ),
      );
      return;
    }
    createExport.mutate(
      {
        type: "usage",
        start_date: report.start_date,
        end_date: report.end_date,
        language: writtenLanguage(i18n.language as "zh" | "en" | "ar"),
        group_by: dimension,
        key_id: params.get("key_id") ? Number(params.get("key_id")) : undefined,
        model: params.get("model") ?? undefined,
        vendor: params.get("vendor") ?? undefined,
        call_type: params.get("call_type") ?? undefined,
        group: params.get("group") ?? undefined,
      },
      {
        onSuccess: () => navigate("/console/exports"),
        onError: (e) => setError(e.message),
      },
    );
  };
  const chart = (report?.series ?? []).map((s) => ({
    ...s,
    value:
      metric === "cost"
        ? Number(s.amount_nano) / 1e9
        : metric === "tokens"
          ? s.input_tokens + s.output_tokens
          : s.requests,
    label: new Date(s.id).toLocaleString(i18n.language, {
      timeZone: report?.timezone,
      month: "2-digit",
      day: "2-digit",
      ...(report?.granularity === "hour"
        ? { hour: "2-digit" as const, minute: "2-digit" as const }
        : {}),
    }),
  }));
  const shares = (rows: UsageStats[], title: string) => (
    <section className="usage-card">
      <h2>{title}</h2>
      {rows.length ? (
        rows.map((row) => (
          <div className="usage-share" key={row.id}>
            <div>
              <span>{row.name || c("未记录", "Not recorded", "غير مسجل")}</span>
              <strong>
                {formatMoney(row.amount_nano, "total")} ·{" "}
                {share(row.amount_nano, report!.totals.amount_nano)}%
              </strong>
            </div>
            <progress
              aria-label={row.name || row.id}
              max="100"
              value={share(row.amount_nano, report!.totals.amount_nano)}
            />
          </div>
        ))
      ) : (
        <p className="usage-muted">
          {c(
            "此范围暂无用量",
            "No usage in this range",
            "لا يوجد استخدام لهذه الفترة",
          )}
        </p>
      )}
    </section>
  );
  return (
    <div className="usage-reports">
      <header className="usage-heading">
        <div>
          <h1>
            {c(
              "用量报表与多维分析",
              "Usage reports and analysis",
              "تقارير الاستخدام والتحليل",
            )}
          </h1>
          <p>
            {c(
              "按真实调用记录统计费用、用量与成功率。",
              "Spend, usage and success rates from gateway records.",
              "التكاليف والاستخدام ومعدلات النجاح من سجلات البوابة.",
            )}
          </p>
        </div>
        <Button
          variant="secondary"
          disabled={!report || createExport.isPending}
          onClick={exportReport}
        >
          <Download size={15} />
          {c("导出报表", "Export report", "تصدير التقرير")}
        </Button>
      </header>
      <section className="usage-card usage-filters">
        <label>
          {c("时间范围", "Date range", "الفترة")}
          <select
            className="select-trigger"
            value={range}
            onChange={(e) => {
              change("range", e.target.value);
              if (e.target.value === "custom" && !params.get("start_date")) {
                const today = billingDate(
                  new Date().toISOString(),
                  report?.timezone,
                );
                setParams((old) => {
                  const next = new URLSearchParams(old);
                  next.set("range", "custom");
                  next.set("start_date", today);
                  next.set("end_date", today);
                  return next;
                });
              }
            }}
          >
            {[
              ["today", c("今天", "Today", "اليوم")],
              ["7d", c("近 7 天", "Last 7 days", "آخر 7 أيام")],
              ["30d", c("近 30 天", "Last 30 days", "آخر 30 يومًا")],
              ["month", c("本月", "This month", "هذا الشهر")],
              [
                "custom",
                c(
                  "自定义（最多一年）",
                  "Custom (up to one year)",
                  "مخصص (حتى سنة)",
                ),
              ],
            ].map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </select>
        </label>
        {range === "custom" && (
          <>
            <label>
              {c("开始日期", "Start date", "تاريخ البداية")}
              <input
                type="date"
                value={params.get("start_date") ?? ""}
                onChange={(e) => change("start_date", e.target.value)}
              />
            </label>
            <label>
              {c("结束日期", "End date", "تاريخ النهاية")}
              <input
                type="date"
                value={params.get("end_date") ?? ""}
                onChange={(e) => change("end_date", e.target.value)}
              />
            </label>
          </>
        )}
        {Object.entries(labels).map(([key, label]) => (
          <label key={key}>
            {label}
            <select
              className="select-trigger"
              value={params.get(key === "key" ? "key_id" : key) ?? ""}
              onChange={(e) =>
                change(key === "key" ? "key_id" : key, e.target.value)
              }
            >
              <option value="">{c("全部", "All", "الكل")}</option>
              {(report?.options[key] ?? []).map((v) => (
                <option key={v.id} value={v.id}>
                  {key === "call_type"
                    ? callTypeLabel(v.name, i18n.language)
                    : v.name || c("未记录", "Not recorded", "غير مسجل")}
                </option>
              ))}
            </select>
          </label>
        ))}
        <label>
          {c("分组维度", "Group by", "التجميع حسب")}
          <select
            className="select-trigger"
            value={dimension}
            onChange={(e) => change("group_by", e.target.value)}
          >
            {Object.entries(labels).map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </select>
        </label>
        <label>
          {c("统计粒度", "Interval", "الفاصل")}
          <select
            className="select-trigger"
            value={params.get("granularity") ?? "day"}
            onChange={(e) => change("granularity", e.target.value)}
          >
            <option value="day">{c("按天", "Daily", "يومي")}</option>
            <option value="hour">{c("按小时", "Hourly", "كل ساعة")}</option>
          </select>
        </label>
        <Button variant="ghost" onClick={() => setParams({})}>
          {c("重置筛选", "Reset filters", "إعادة ضبط")}
        </Button>
      </section>
      {report && (
        <p className="usage-muted">
          {report.start_date} — {report.end_date} · {report.timezone}
        </p>
      )}
      {query.isFetching && (
        <p role="status">
          {c("正在加载真实用量…", "Loading usage…", "جار تحميل الاستخدام…")}
        </p>
      )}
      {(reportError || error) && (
        <p role="alert" className="usage-error">
          {error || reportError}{" "}
          <Button variant="ghost" onClick={() => void query.refetch()}>
            {c("重试", "Retry", "إعادة المحاولة")}
          </Button>
        </p>
      )}
      {report && (
        <>
          <div className="usage-summary">
            {[
              [
                c("调用次数", "Requests", "الطلبات"),
                report.totals.requests.toLocaleString(),
              ],
              [
                c("总费用（美元）", "Spend (USD)", "التكلفة (دولار)"),
                formatMoney(report.totals.amount_nano, "total"),
              ],
              [
                c("成功率", "Success rate", "معدل النجاح"),
                percent(report.totals),
              ],
              [
                c("Token 用量", "Tokens", "الرموز"),
                (
                  report.totals.input_tokens + report.totals.output_tokens
                ).toLocaleString(),
              ],
            ].map(([label, value]) => (
              <section className="usage-card" key={label}>
                <span className="usage-muted">{label}</span>
                <strong className="usage-summary-value">{value}</strong>
              </section>
            ))}
          </div>
          <section className="usage-card">
            <div className="usage-heading">
              <h2>{c("用量趋势", "Usage over time", "الاستخدام عبر الوقت")}</h2>
              <select
                aria-label={c("图表指标", "Chart metric", "مقياس الرسم")}
                className="select-trigger"
                value={metric}
                onChange={(e) => setMetric(e.target.value)}
              >
                <option value="cost">
                  {c("费用（美元）", "Spend (USD)", "التكلفة")}
                </option>
                <option value="requests">
                  {c("调用次数", "Requests", "الطلبات")}
                </option>
                <option value="tokens">
                  {c("Token 用量", "Tokens", "الرموز")}
                </option>
              </select>
            </div>
            {chart.length ? (
              <div className="usage-trend">
                <ResponsiveContainer width="100%" height={260}>
                  <AreaChart data={chart}>
                    <CartesianGrid
                      strokeDasharray="3 3"
                      stroke="var(--border)"
                    />
                    <XAxis
                      dataKey="label"
                      tick={{ fill: "var(--muted)", fontSize: 11 }}
                    />
                    <YAxis
                      tick={{ fill: "var(--muted)", fontSize: 11 }}
                      width={65}
                    />
                    <Tooltip
                      contentStyle={{
                        background: "var(--surface)",
                        borderColor: "var(--border)",
                        color: "var(--text)",
                      }}
                    />
                    <Area
                      type="monotone"
                      dataKey="value"
                      stroke="var(--brand)"
                      fill="var(--brand)"
                      fillOpacity={0.12}
                    />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <p>
                {c(
                  "当前筛选条件下没有调用。",
                  "No requests match these filters.",
                  "لا توجد طلبات تطابق المرشحات.",
                )}
              </p>
            )}
          </section>
          <div className="usage-shares">
            {shares(
              report.by_key,
              c("按 Key 的费用占比", "Spend by Key", "التكلفة حسب المفتاح"),
            )}
            {shares(
              report.by_model,
              c("按模型的费用占比", "Spend by model", "التكلفة حسب النموذج"),
            )}
          </div>
          <section className="usage-card">
            <h2>
              {labels[dimension]} ·{" "}
              {c("用量明细", "Usage breakdown", "تفاصيل الاستخدام")}
            </h2>
            <div className="usage-table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>{labels[dimension]}</th>
                    <th>{c("调用次数", "Requests", "الطلبات")}</th>
                    <th>{c("成功率", "Success rate", "النجاح")}</th>
                    <th>
                      {c(
                        "输入 / 输出 Token",
                        "Input / output tokens",
                        "رموز الإدخال / الإخراج",
                      )}
                    </th>
                    <th>
                      {c(
                        "缓存读 / 写 Token",
                        "Cache read / write",
                        "قراءة / كتابة التخزين",
                      )}
                    </th>
                    <th>
                      {c("费用（美元）", "Spend (USD)", "التكلفة (دولار)")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {report.rows.map((row) => (
                    <tr key={row.id}>
                      <td>
                        {dimension === "call_type"
                          ? callTypeLabel(row.name, i18n.language)
                          : row.name || c("未记录", "Not recorded", "غير مسجل")}
                      </td>
                      <td>{row.requests.toLocaleString()}</td>
                      <td>{percent(row)}</td>
                      <td>
                        {row.input_tokens.toLocaleString()} /{" "}
                        {row.output_tokens.toLocaleString()}
                      </td>
                      <td>
                        {row.cache_read_tokens.toLocaleString()} /{" "}
                        {row.cache_write_tokens.toLocaleString()}
                      </td>
                      <td className="mono">
                        {formatMoney(row.amount_nano, "total")}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
    </div>
  );
}
