import { useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { useServiceQuality } from "../lib/api/service-quality";
import { usePreferences } from "../lib/preferences";
import { Button } from "./ui/button";
import { Link } from "react-router-dom";
import { useSystemAccess } from "../lib/system-access";

export function ModelRequestTrend({ model }: { model: string }) {
  const language = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [metric, setMetric] = useState<"requests" | "success" | "latency">(
    "requests",
  );
  const query = useServiceQuality(720, model);
  const authenticated = useSystemAccess(
    (state) => state.session.accountId !== null,
  );
  const title =
    metric === "latency"
      ? text(
          "最近 30 天每日 P95 延迟",
          "Daily P95 latency over the last 30 days",
          "زمن P95 اليومي خلال آخر 30 يومًا",
        )
      : metric === "success"
        ? text(
            "最近 30 天每日成功率",
            "Daily success rate over the last 30 days",
            "معدل النجاح اليومي خلال آخر 30 يومًا",
          )
        : text(
            "最近 30 天每日请求数",
            "Daily requests over the last 30 days",
            "الطلبات اليومية خلال آخر 30 يومًا",
          );
  const requests = text("请求数", "Requests", "الطلبات");
  const successRate = text("成功率", "Success rate", "معدل النجاح");
  const latency = text("P95 延迟", "P95 latency", "زمن P95");
  const noLatencyData = text(
    "暂无耗时数据",
    "No latency data",
    "لا توجد بيانات زمن الاستجابة",
  );
  const rows = (query.isError ? [] : (query.data?.by_day ?? [])).map((day) => ({
    ...day,
    success_percent: day.success_rate * 100,
    measured_p95:
      Number.isFinite(day.p95_duration_ms) && day.p95_duration_ms > 0
        ? day.p95_duration_ms
        : null,
  }));
  const dataKey = {
    requests: "total_requests",
    success: "success_percent",
    latency: "measured_p95",
  }[metric];

  return (
    <section className="access-panel model-request-trend" aria-label={title}>
      <h2>{title}</h2>
      <p className="access-muted">
        {text(
          "仅统计当前模型；按 UTC 日期展示有请求记录的日期。",
          "Current model only; dates with recorded requests are shown in UTC.",
          "للنموذج الحالي فقط؛ تُعرض تواريخ الطلبات المسجلة بتوقيت UTC.",
        )}
      </p>
      <label className="access-inline-label">
        {text("指标", "Metric", "المقياس")}
        <select
          value={metric}
          onChange={(event) =>
            setMetric(
              event.target.value === "latency"
                ? "latency"
                : event.target.value === "success"
                  ? "success"
                  : "requests",
            )
          }
        >
          <option value="requests">{requests}</option>
          <option value="success">{successRate}</option>
          <option value="latency">{latency}</option>
        </select>
      </label>
      {!authenticated ? (
        <p className="access-muted">
          <Link to="/login">
            {text(
              "登录后查看请求趋势",
              "Sign in to view request trends",
              "سجّل الدخول لعرض اتجاه الطلبات",
            )}
          </Link>
        </p>
      ) : query.isPending ? (
        <p className="access-muted" role="status">
          {text("正在加载趋势…", "Loading trend…", "جارٍ تحميل الاتجاه…")}
        </p>
      ) : query.isError ? (
        <div role="alert">
          <p>
            {text(
              "请求趋势暂不可用。",
              "Request trend is unavailable.",
              "اتجاه الطلبات غير متاح حاليًا.",
            )}
          </p>
          <Button
            variant="secondary"
            size="small"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {text("重试", "Retry", "إعادة المحاولة")}
          </Button>
        </div>
      ) : rows.length === 0 ? (
        <p className="access-muted">
          {text(
            "最近 30 天暂无请求数据。",
            "No request data in the last 30 days.",
            "لا توجد بيانات طلبات خلال آخر 30 يومًا.",
          )}
        </p>
      ) : (
        <>
          {metric === "latency" && (
            <p className="access-muted">
              {text(
                "纵轴单位为秒，数据表与提示为毫秒；无有效耗时的日期不绘制延迟柱。",
                "Axis in seconds; table and tooltips in milliseconds. Days without measured latency have no bar.",
                "المحور بالثواني والجدول والتلميحات بالمللي ثانية؛ لا تظهر أعمدة للأيام بلا قياس.",
              )}
            </p>
          )}
          {metric === "latency" &&
          rows.every((day) => day.measured_p95 === null) ? (
            <p className="access-muted" role="status">
              {noLatencyData}
            </p>
          ) : (
            <div dir="ltr" aria-hidden="true" style={{ minWidth: 0 }}>
              <ResponsiveContainer width="100%" height={180}>
                <BarChart
                  data={rows}
                  margin={{ top: 8, right: 8, bottom: 0, left: 0 }}
                >
                  <CartesianGrid vertical={false} stroke="var(--border)" />
                  <XAxis
                    dataKey="date"
                    tickFormatter={(date: string) => date.slice(5)}
                    minTickGap={24}
                    tick={{ fill: "var(--muted)", fontSize: 11 }}
                  />
                  <YAxis
                    allowDecimals={false}
                    width={metric === "latency" ? 56 : 44}
                    domain={metric === "success" ? [0, 100] : [0, "auto"]}
                    tickFormatter={
                      metric === "latency"
                        ? (value: number) => `${value / 1000}s`
                        : metric === "success"
                          ? (value: number) => `${value}%`
                          : undefined
                    }
                    tick={{ fill: "var(--muted)", fontSize: 11 }}
                  />
                  <Tooltip
                    labelFormatter={(date) => `${date} (UTC)`}
                    formatter={
                      metric === "latency"
                        ? (value) => `${value}ms`
                        : metric === "success"
                          ? (value) => `${Number(value).toFixed(1)}%`
                          : undefined
                    }
                    contentStyle={{
                      background: "var(--surface)",
                      borderColor: "var(--border)",
                      color: "var(--text)",
                    }}
                    cursor={{ fill: "var(--border)" }}
                  />
                  <Bar
                    dataKey={dataKey}
                    name={
                      metric === "latency"
                        ? latency
                        : metric === "success"
                          ? successRate
                          : requests
                    }
                    fill="var(--brand)"
                    maxBarSize={36}
                    isAnimationActive={false}
                  />
                </BarChart>
              </ResponsiveContainer>
            </div>
          )}
          <details className="access-details">
            <summary>
              {text("查看每日数据", "View daily data", "عرض البيانات اليومية")}
            </summary>
            <div
              className="access-table-wrap"
              tabIndex={0}
              role="region"
              aria-label={title}
            >
              <table className="access-table">
                <thead>
                  <tr>
                    <th>
                      {text("日期（UTC）", "Date (UTC)", "التاريخ (UTC)")}
                    </th>
                    <th>{requests}</th>
                    <th>{successRate}</th>
                    <th>{latency} (ms)</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((day) => (
                    <tr key={day.date}>
                      <th scope="row" dir="ltr">
                        {day.date}
                      </th>
                      <td>{day.total_requests}</td>
                      <td>{day.success_percent.toFixed(1)}%</td>
                      <td
                        style={{
                          color:
                            day.measured_p95 === null
                              ? "var(--muted)"
                              : undefined,
                        }}
                      >
                        {day.measured_p95 === null
                          ? noLatencyData
                          : `${day.measured_p95}ms`}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </details>
        </>
      )}
    </section>
  );
}
