import { ResponsiveTable } from "../components/responsive-table";
import { ListPagination, useListPage } from "../components/list-pagination";
import { message as translatedMessage } from "../i18n/messages";
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
  Legend,
} from "recharts";
import { Download } from "lucide-react";
import { Button } from "../components/ui/button";
import { useUsageReport, type UsageStats } from "../lib/api/usage-report";
import { useCreateExport } from "../lib/api/exports";
import { formatMoney } from "../lib/money";
import { regionalLocale } from "../lib/locale-format";
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
  const pagination = useListPage(report?.rows ?? [], params.toString());
  const keyPagination = useListPage(report?.by_key ?? [], params.toString());
  const modelPagination = useListPage(
    report?.by_model ?? [],
    params.toString(),
  );
  const reportError =
    invalidReport || query.error instanceof UsageReportContractError
      ? c(
          ...translatedMessage(
            "the_report_api_is_incompatible_with_this_back_5d6e2ec544",
          ),
        )
      : query.error?.message;
  const range = params.get("range") ?? "7d",
    dimension = params.get("group_by") ?? "key";
  const labels: Record<string, string> = {
    key: c(...translatedMessage("key_64846eb2df")),
    model: c(...translatedMessage("model_9c2e5b5065")),
    vendor: c(...translatedMessage("vendor_5dc28b7ea6")),
    call_type: c(...translatedMessage("call_type_481db78ba3")),
    group: c(...translatedMessage("service_group_1c6f8adf41")),
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
          ...translatedMessage(
            "exports_allow_up_to_31_days_choose_a_shorter__45b7d39aed",
          ),
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
    label: new Date(s.id)
      .toLocaleString(regionalLocale(i18n.language), {
        timeZone: report?.timezone,
        month: "2-digit",
        day: "2-digit",
        ...(report?.granularity === "hour"
          ? { hour: "2-digit" as const, minute: "2-digit" as const }
          : {}),
      })
      .replace(/[\u200e\u200f\u061c]/g, ""),
  }));
  const shares = (
    pager: ReturnType<typeof useListPage<UsageStats>>,
    title: string,
  ) => (
    <section className="usage-card">
      <h2>{title}</h2>
      {pager.total ? (
        pager.rows.map((row) => (
          <div className="usage-share" key={row.id}>
            <div>
              <span>
                {row.name || c(...translatedMessage("not_recorded_4bbcb12676"))}
              </span>
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
          {c(...translatedMessage("no_usage_in_this_range_7a96e1f7d0"))}
        </p>
      )}
      <ListPagination {...pager} />
    </section>
  );
  return (
    <div className="usage-reports">
      <header className="usage-heading">
        <div>
          <h1>
            {c(...translatedMessage("usage_reports_and_analysis_e760366356"))}
          </h1>
          <p>
            {c(
              ...translatedMessage(
                "spend_usage_and_success_rates_from_gateway_re_4fe59813c3",
              ),
            )}
          </p>
        </div>
        <Button
          variant="secondary"
          disabled={!report || createExport.isPending}
          onClick={exportReport}
        >
          <Download size={15} />
          {c(...translatedMessage("export_report_d0e5a6756f"))}
        </Button>
      </header>
      <section className="usage-card usage-filters">
        <label>
          {c(...translatedMessage("date_range_d9fad808c9"))}
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
              ["today", c(...translatedMessage("today_3a255d525f"))],
              ["7d", c(...translatedMessage("last_7_days_1365f5fbb3"))],
              ["30d", c(...translatedMessage("last_30_days_24648573ce"))],
              ["month", c(...translatedMessage("this_month_ad96df918e"))],
              [
                "custom",
                c(...translatedMessage("custom_up_to_one_year_7c8fab629b")),
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
              {c(...translatedMessage("start_date_761834f95f"))}
              <input
                type="date"
                value={params.get("start_date") ?? ""}
                onChange={(e) => change("start_date", e.target.value)}
              />
            </label>
            <label>
              {c(...translatedMessage("end_date_ddebce9446"))}
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
              <option value="">
                {c(...translatedMessage("all_ac49939df0"))}
              </option>
              {(report?.options[key] ?? []).map((v) => (
                <option key={v.id} value={v.id}>
                  {key === "call_type"
                    ? callTypeLabel(v.name, i18n.language)
                    : v.name ||
                      c(...translatedMessage("not_recorded_4bbcb12676"))}
                </option>
              ))}
            </select>
          </label>
        ))}
        <label>
          {c(...translatedMessage("group_by_2fd097bc41"))}
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
          {c(...translatedMessage("interval_bb384227aa"))}
          <select
            className="select-trigger"
            value={params.get("granularity") ?? "day"}
            onChange={(e) => change("granularity", e.target.value)}
          >
            <option value="day">
              {c(...translatedMessage("daily_c65ed2c3a3"))}
            </option>
            <option value="hour">
              {c(...translatedMessage("hourly_2855399597"))}
            </option>
          </select>
        </label>
        <Button variant="ghost" onClick={() => setParams({})}>
          {c(...translatedMessage("reset_filters_71a291a252"))}
        </Button>
      </section>
      {report && (
        <p className="usage-muted">
          {report.start_date} — {report.end_date} · {report.timezone}
        </p>
      )}
      {query.isFetching && (
        <p role="status">
          {c(...translatedMessage("loading_usage_2bcb86bb23"))}
        </p>
      )}
      {(reportError || error) && (
        <p role="alert" className="usage-error">
          {error || reportError}{" "}
          <Button variant="ghost" onClick={() => void query.refetch()}>
            {c(...translatedMessage("retry_c16455021c"))}
          </Button>
        </p>
      )}
      {report && (
        <>
          <div className="usage-summary">
            {[
              [
                c(...translatedMessage("requests_6480ea807d")),
                report.totals.requests.toLocaleString(),
              ],
              [
                c(...translatedMessage("spend_usd_d24e6785bf")),
                formatMoney(report.totals.amount_nano, "total"),
              ],
              [
                c(...translatedMessage("success_rate_b8e8689749")),
                percent(report.totals),
              ],
              [
                c(...translatedMessage("tokens_9f06dbbe17")),
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
              <h2>{c(...translatedMessage("usage_over_time_414c3494a8"))}</h2>
              <select
                aria-label={c(...translatedMessage("chart_metric_b95503c8fd"))}
                className="select-trigger"
                value={metric}
                onChange={(e) => setMetric(e.target.value)}
              >
                <option value="cost">
                  {c(...translatedMessage("spend_usd_e98e9e9e19"))}
                </option>
                <option value="requests">
                  {c(...translatedMessage("requests_6480ea807d"))}
                </option>
                <option value="tokens">
                  {c(...translatedMessage("tokens_9f06dbbe17"))}
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
                      reversed={i18n.language.startsWith("ar")}
                      dataKey="label"
                      tick={{ fill: "var(--muted)", fontSize: 11 }}
                    />
                    <YAxis
                      orientation={
                        i18n.language.startsWith("ar") ? "right" : "left"
                      }
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
                    <Legend
                      align={i18n.language === "ar" ? "right" : "left"}
                      wrapperStyle={{
                        direction: i18n.language === "ar" ? "rtl" : "ltr",
                      }}
                    />
                    <Area
                      type="monotone"
                      dataKey="value"
                      name={
                        metric === "cost"
                          ? c(...translatedMessage("cost_usd_9cd3ac1402"))
                          : metric === "tokens"
                            ? c(...translatedMessage("tokens_9f06dbbe17"))
                            : c(...translatedMessage("requests_fe89af9421"))
                      }
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
                  ...translatedMessage(
                    "no_requests_match_these_filters_4f52193b06",
                  ),
                )}
              </p>
            )}
          </section>
          <div className="usage-shares">
            {shares(
              keyPagination,
              c(...translatedMessage("spend_by_key_08e3b37ac7")),
            )}
            {shares(
              modelPagination,
              c(...translatedMessage("spend_by_model_21e3406923")),
            )}
          </div>
          <section className="usage-card">
            <h2>
              {labels[dimension]} ·{" "}
              {c(...translatedMessage("usage_breakdown_eccf106201"))}
            </h2>
            <div className="usage-table-scroll">
              <ResponsiveTable>
                <thead>
                  <tr>
                    <th>{labels[dimension]}</th>
                    <th>{c(...translatedMessage("requests_6480ea807d"))}</th>
                    <th>
                      {c(...translatedMessage("success_rate_cf0d8496e2"))}
                    </th>
                    <th>
                      {c(
                        ...translatedMessage("input_output_tokens_52437a9a3b"),
                      )}
                    </th>
                    <th>
                      {c(...translatedMessage("cache_read_write_8940d40a29"))}
                    </th>
                    <th>{c(...translatedMessage("spend_usd_67b82d4b99"))}</th>
                  </tr>
                </thead>
                <tbody>
                  {pagination.rows.map((row) => (
                    <tr key={row.id}>
                      <td>
                        {dimension === "call_type"
                          ? callTypeLabel(row.name, i18n.language)
                          : row.name ||
                            c(...translatedMessage("not_recorded_4bbcb12676"))}
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
              </ResponsiveTable>
            </div>
            <ListPagination {...pagination} />
          </section>
        </>
      )}
    </div>
  );
}
