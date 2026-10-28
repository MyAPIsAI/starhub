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

export function ModelRequestTrend({ model }: { model: string }) {
  const language = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [metric, setMetric] = useState<"requests" | "success">("requests");
  const query = useServiceQuality(720, model);
  const title =
    metric === "success"
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
  const rows = (query.data?.by_day ?? []).map((day) => ({
    ...day,
    success_percent: day.success_rate * 100,
  }));

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
            setMetric(event.target.value === "success" ? "success" : "requests")
          }
        >
          <option value="requests">{requests}</option>
          <option value="success">{successRate}</option>
        </select>
      </label>
      {query.isPending ? (
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
                  width={44}
                  domain={metric === "success" ? [0, 100] : [0, "auto"]}
                  tickFormatter={
                    metric === "success"
                      ? (value: number) => `${value}%`
                      : undefined
                  }
                  tick={{ fill: "var(--muted)", fontSize: 11 }}
                />
                <Tooltip
                  labelFormatter={(date) => `${date} (UTC)`}
                  formatter={
                    metric === "success"
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
                  dataKey={
                    metric === "success" ? "success_percent" : "total_requests"
                  }
                  name={metric === "success" ? successRate : requests}
                  fill="var(--brand)"
                  maxBarSize={36}
                  isAnimationActive={false}
                />
              </BarChart>
            </ResponsiveContainer>
          </div>
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
