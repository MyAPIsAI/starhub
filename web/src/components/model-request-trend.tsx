import { message as translatedMessage } from "../i18n/messages";
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
          ...translatedMessage(
            "daily_p95_latency_over_the_last_30_days_12d1fe226a",
          ),
        )
      : metric === "success"
        ? text(
            ...translatedMessage(
              "daily_success_rate_over_the_last_30_days_82cda47fb5",
            ),
          )
        : text(
            ...translatedMessage(
              "daily_requests_over_the_last_30_days_7295fcaa02",
            ),
          );
  const requests = text(...translatedMessage("requests_fe89af9421"));
  const successRate = text(...translatedMessage("success_rate_b8e8689749"));
  const latency = text(...translatedMessage("p95_latency_dc1e193e2a"));
  const noLatencyData = text(
    ...translatedMessage("no_latency_data_3d1e009af8"),
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
          ...translatedMessage(
            "current_model_only_dates_with_recorded_reques_6792f03d28",
          ),
        )}
      </p>
      <label className="access-inline-label">
        {text(...translatedMessage("metric_369adc4737"))}
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
              ...translatedMessage("sign_in_to_view_request_trends_7c349249a1"),
            )}
          </Link>
        </p>
      ) : query.isPending ? (
        <p className="access-muted" role="status">
          {text(...translatedMessage("loading_trend_346a990c10"))}
        </p>
      ) : query.isError ? (
        <div role="alert">
          <p>
            {text(
              ...translatedMessage("request_trend_is_unavailable_d6ded7236d"),
            )}
          </p>
          <Button
            variant="secondary"
            size="small"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {text(...translatedMessage("retry_c16455021c"))}
          </Button>
        </div>
      ) : rows.length === 0 ? (
        <p className="access-muted">
          {text(
            ...translatedMessage(
              "no_request_data_in_the_last_30_days_24419e47f6",
            ),
          )}
        </p>
      ) : (
        <>
          {metric === "latency" && (
            <p className="access-muted">
              {text(
                ...translatedMessage(
                  "axis_in_seconds_table_and_tooltips_in_millise_6c6d0fe316",
                ),
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
                    reversed={language === "ar"}
                    dataKey="date"
                    tickFormatter={(date: string) => date.slice(5)}
                    minTickGap={24}
                    tick={{ fill: "var(--muted)", fontSize: 11 }}
                  />
                  <YAxis
                    orientation={language === "ar" ? "right" : "left"}
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
              {text(...translatedMessage("view_daily_data_ad82e67c6c"))}
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
                    <th>{text(...translatedMessage("date_utc_1d822f1aa7"))}</th>
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
