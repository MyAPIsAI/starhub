import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { RefreshCw } from "lucide-react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { useStaffText } from "../lib/api/staff";
import { usePreferences } from "../lib/preferences";
import { formatMoney } from "../lib/money";
import { callTypeLabel } from "../lib/request-metadata";
import {
  operatingChart,
  formatOperatingAmount,
  operatingShare,
  useOperatingDashboard,
  type OperatingBucket,
} from "../lib/api/operating-dashboard";
import "./operating-dashboard.css";

const colors = [
  "#6366f1",
  "#f07838",
  "#16a886",
  "#e8a109",
  "#3b82f6",
  "#bf6ae0",
  "#db6387",
  "#8592a8",
];

function Distribution({
  title,
  rows,
  total,
  label,
}: {
  title: string;
  rows: OperatingBucket[];
  total: string;
  label: (row: OperatingBucket) => string;
}) {
  const t = useStaffText();
  const [expanded, setExpanded] = useState(false);
  const positive = rows.filter((row) => BigInt(row.amount_nano) > 0n);
  const visible = expanded ? positive : positive.slice(0, 5);
  const remaining = positive
    .slice(5)
    .reduce((sum, row) => sum + BigInt(row.amount_nano), 0n)
    .toString();
  const row = (name: string, amount: string, key: string, other = false) => (
    <div className={`op-distribution-row${other ? " is-other" : ""}`} key={key}>
      <div>
        <span>{name}</span>
        <bdi title={formatMoney(amount, "detail")}>
          {formatOperatingAmount(amount)}
        </bdi>
        <bdi>{operatingShare(amount, total).toFixed(1)}%</bdi>
      </div>
      <div className="op-share-track" aria-hidden="true">
        <span style={{ width: `${operatingShare(amount, total)}%` }} />
      </div>
    </div>
  );
  return (
    <article className="op-card op-distribution">
      <h2>{title}</h2>
      {!positive.length && (
        <p className="op-empty">
          {t(...translatedMessage("no_consumption_yet_2db84d6a3c"))}
        </p>
      )}
      {visible.map((item) => row(label(item), item.amount_nano, item.id))}
      {!expanded &&
        positive.length > 5 &&
        row(
          t(
            ...translatedMessage("other_items_ab98177a82", {
              p0: positive.length - 5,
            }),
          ),
          remaining,
          "__remaining",
          true,
        )}
      {positive.length > 5 && (
        <Button
          variant="ghost"
          size="small"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded
            ? t(...translatedMessage("show_less_de99c9aeb4"))
            : t(...translatedMessage("show_all_07b8e337c9"))}
        </Button>
      )}
    </article>
  );
}

export function OperatingDashboard() {
  const t = useStaffText(),
    language = usePreferences((s) => s.language);
  const [params, setParams] = useSearchParams();
  const rawRange = params.get("range") || "7d",
    range = ["today", "7d", "30d", "custom"].includes(rawRange)
      ? rawRange
      : "7d";
  const [startDraft, setStartDraft] = useState(params.get("start_date") || ""),
    [endDraft, setEndDraft] = useState(params.get("end_date") || "");
  const [showModels, setShowModels] = useState(false);
  const [dateError, setDateError] = useState(false);
  const request = new URLSearchParams({ range });
  if (range === "custom") {
    request.set("start_date", params.get("start_date") || "");
    request.set("end_date", params.get("end_date") || "");
  }
  const valid =
    range !== "custom" ||
    Boolean(request.get("start_date") && request.get("end_date"));
  const query = useOperatingDashboard(request.toString(), valid);
  const data = query.error ? undefined : query.data;
  const chart = data ? operatingChart(data) : undefined;
  const label = (row: OperatingBucket) =>
    row.name || row.id || t(...translatedMessage("not_recorded_4bbcb12676"));
  const updateRange = (value: string) => {
    setDateError(false);
    setParams((old) => {
      const next = new URLSearchParams(old);
      next.set("range", value);
      if (value !== "custom") {
        next.delete("start_date");
        next.delete("end_date");
      }
      return next;
    });
  };
  const applyDates = (event: React.FormEvent) => {
    event.preventDefault();
    if (!startDraft || !endDraft || startDraft > endDraft) {
      setDateError(true);
      return;
    }
    setDateError(false);
    setParams((old) => {
      const next = new URLSearchParams(old);
      next.set("range", "custom");
      next.set("start_date", startDraft);
      next.set("end_date", endDraft);
      return next;
    });
  };
  const formatTime = (at: number, axis = false) => {
    const value = new Intl.DateTimeFormat(regionalLocale(language), {
      timeZone: data?.timezone || "UTC",
      ...(axis
        ? data?.granularity === "hour"
          ? { hour: "2-digit" as const, minute: "2-digit" as const }
          : { month: "numeric" as const, day: "numeric" as const }
        : { dateStyle: "short" as const, timeStyle: "short" as const }),
    }).format(new Date(at * 1000));
    return axis ? value.replace(/[\u200e\u200f]/g, "") : value;
  };
  const rank = (title: string, rows: OperatingBucket[], accounts: boolean) => (
    <article className="op-card op-ranking">
      <header>
        <h2>{title}</h2>
        <p>
          {t(...translatedMessage("top_20_settled_consumption_3043ca6083"))}
        </p>
      </header>
      {!rows.some((row) => BigInt(row.amount_nano) > 0n) ? (
        <p className="op-empty">
          {t(...translatedMessage("no_consumption_ranking_yet_d751e956d6"))}
        </p>
      ) : (
        <ol>
          {rows
            .filter((row) => BigInt(row.amount_nano) > 0n)
            .map((row, index) => (
              <li key={row.id}>
                <span className="op-rank-number">{index + 1}</span>
                {accounts ? (
                  <Link
                    to={`/admin?tab=customers&customer=${encodeURIComponent(row.id)}`}
                  >
                    <bdi>{label(row)}</bdi>
                  </Link>
                ) : (
                  <span>
                    <bdi>{label(row)}</bdi>
                    {row.name && row.name !== row.id && (
                      <small>
                        <bdi>{row.id}</bdi>
                      </small>
                    )}
                  </span>
                )}
                <bdi
                  className="op-money"
                  title={formatMoney(row.amount_nano, "detail")}
                >
                  {formatOperatingAmount(row.amount_nano)}
                </bdi>
              </li>
            ))}
        </ol>
      )}
    </article>
  );
  return (
    <section className="op-dashboard">
      <header className="op-heading">
        <div>
          <h1>{t(...translatedMessage("operations_dashboard_7d445d520b"))}</h1>
          <p>
            {t(...translatedMessage("customer_business_overview_22a29ba80a"))}
            {data && (
              <>
                {" "}
                · <bdi>{data.region_id}</bdi> · <bdi>{data.timezone}</bdi>
              </>
            )}
          </p>
        </div>
        <div className="op-toolbar">
          <Button
            variant="ghost"
            size="small"
            disabled={query.isFetching || !valid}
            onClick={() => void query.refetch()}
            aria-label={t(...translatedMessage("refresh_dashboard_711c0a6aea"))}
          >
            <RefreshCw
              size={14}
              className={query.isFetching ? "op-spinning" : ""}
            />
            {t(...translatedMessage("refresh_9c83c3600a"))}
          </Button>
          <div
            className="op-periods"
            role="group"
            aria-label={t(...translatedMessage("reporting_period_bc110d4c4c"))}
          >
            {[
              ["today", t(...translatedMessage("today_3a255d525f"))],
              ["7d", t(...translatedMessage("7_days_630bef729b"))],
              ["30d", t(...translatedMessage("30_days_f526252351"))],
              ["custom", t(...translatedMessage("custom_0bf4a1d1c3"))],
            ].map(([value, name]) => (
              <button
                key={value}
                aria-pressed={range === value}
                onClick={() => updateRange(value)}
              >
                {name}
              </button>
            ))}
          </div>
        </div>
      </header>
      {range === "custom" && (
        <form className="op-custom" onSubmit={applyDates}>
          <label>
            {t(...translatedMessage("start_date_761834f95f"))}
            <Input
              type="date"
              required
              value={startDraft}
              max={endDraft || undefined}
              onChange={(e) => setStartDraft(e.target.value)}
            />
          </label>
          <label>
            {t(...translatedMessage("end_date_ddebce9446"))}
            <Input
              type="date"
              required
              value={endDraft}
              min={startDraft || undefined}
              onChange={(e) => setEndDraft(e.target.value)}
            />
          </label>
          <Button type="submit" variant="secondary">
            {t(...translatedMessage("apply_1cfc3807dc"))}
          </Button>
          <span>
            {t(
              ...translatedMessage(
                "regional_timezone_up_to_one_year_6e94fae25e",
              ),
            )}
          </span>
        </form>
      )}
      {dateError && (
        <p role="alert">
          {t(
            ...translatedMessage("select_valid_start_and_end_dates_8242993706"),
          )}
        </p>
      )}
      {query.isPending && valid && (
        <p role="status" className="op-empty">
          {t(...translatedMessage("loading_business_data_e941beb3c9"))}
        </p>
      )}
      {query.error && (
        <div role="alert" className="op-error">
          {t(
            ...translatedMessage(
              "business_data_is_unavailable_check_the_date_r_cdfafafa42",
            ),
          )}
          <Button variant="secondary" onClick={() => void query.refetch()}>
            {t(...translatedMessage("retry_c16455021c"))}
          </Button>
        </div>
      )}
      {data && chart && (
        <>
          <div className="op-data-time">
            <span>
              <bdi>{data.start_date}</bdi> — <bdi>{data.end_date}</bdi>
            </span>
            <span>
              {t(...translatedMessage("every_5_minutes_updated_84a126b600"))}{" "}
              <bdi>{formatTime(data.updated_at)}</bdi>
            </span>
          </div>
          <div className="op-kpis">
            {[
              [
                t(...translatedMessage("top_ups_710ff622fa")),
                formatOperatingAmount(data.totals.recharge_nano),
                t(
                  ...translatedMessage(
                    "registered_offline_receipts_see_funding_detai_229d5b148d",
                  ),
                ),
              ],
              [
                t(...translatedMessage("consumption_9ff97a41c3")),
                formatOperatingAmount(data.totals.consumption_nano),
                t(
                  ...translatedMessage(
                    "actual_settled_charges_from_call_records_26fdb8d786",
                  ),
                ),
              ],
              [
                t(...translatedMessage("calls_d84729ef0c")),
                data.totals.requests.toLocaleString(language),
                t(
                  ...translatedMessage(
                    "recorded_completed_calls_including_failures_a_757266d604",
                  ),
                ),
              ],
              [
                t(...translatedMessage("final_success_rate_2c5f80901e")),
                data.totals.requests
                  ? `${((data.totals.successful / data.totals.requests) * 100).toFixed(2)}%`
                  : "—",
                t(
                  ...translatedMessage(
                    "successful_completed_calls_pending_asynchrono_466bffff72",
                  ),
                ),
              ],
              [
                t(...translatedMessage("active_accounts_f4443969e5")),
                data.totals.active_accounts.toLocaleString(language),
                t(
                  ...translatedMessage(
                    "distinct_customers_with_calls_in_this_period_bc7092aa4e",
                  ),
                ),
              ],
              [
                t(...translatedMessage("new_accounts_6e39edd7ae")),
                data.totals.new_accounts.toLocaleString(language),
                t(
                  ...translatedMessage(
                    "customers_registered_in_this_period_d13a4eca33",
                  ),
                ),
              ],
              [
                t(...translatedMessage("paying_accounts_39c75640b6")),
                data.totals.paying_accounts.toLocaleString(language),
                t(
                  ...translatedMessage(
                    "distinct_customers_with_registered_receipts_i_37a7444689",
                  ),
                ),
              ],
            ].map(([name, value, hint]) => (
              <article className="op-card op-kpi" key={name} title={hint}>
                <h2>{name}</h2>
                <strong>
                  <bdi>{value}</bdi>
                </strong>
              </article>
            ))}
          </div>
          <article className="op-card op-trend">
            <h2>{t(...translatedMessage("consumption_trend_57ba5c4dc0"))}</h2>
            <p>
              {t(...translatedMessage("stacked_by_service_group_9f4ff0ed51"))}
            </p>
            <div className="op-legend">
              {chart.groups.map((row, index) => (
                <span key={row.id}>
                  <i style={{ background: colors[index] }} />
                  <bdi>{label(row)}</bdi>
                </span>
              ))}
              {chart.hasOther && (
                <span>
                  <i style={{ background: colors[7] }} />
                  {t(...translatedMessage("other_f6ed33d295"))}
                </span>
              )}
            </div>
            {BigInt(data.totals.consumption_nano) === 0n ? (
              <div className="op-chart-empty">
                {t(
                  ...translatedMessage(
                    "no_settled_consumption_in_this_period_80bd69da54",
                  ),
                )}
              </div>
            ) : (
              <div className="op-chart" dir="ltr">
                <ResponsiveContainer width="100%" height={280}>
                  <BarChart
                    data={chart.points}
                    margin={{ top: 12, right: 12, left: 8, bottom: 4 }}
                    accessibilityLayer
                  >
                    <CartesianGrid
                      vertical={false}
                      stroke="var(--border)"
                      strokeDasharray="2 4"
                    />
                    <XAxis
                      dataKey="at"
                      tickFormatter={(value: number) => formatTime(value, true)}
                      tickLine={false}
                      axisLine={{ stroke: "var(--border)" }}
                      minTickGap={20}
                      tick={{ fill: "var(--secondary)", fontSize: 11 }}
                    />
                    <YAxis
                      tickFormatter={(value: number) =>
                        new Intl.NumberFormat(regionalLocale(language), {
                          style: "currency",
                          currency: "USD",
                          notation: "compact",
                          maximumFractionDigits: 1,
                        }).format(value)
                      }
                      tickLine={false}
                      axisLine={false}
                      width={65}
                      tick={{ fill: "var(--secondary)", fontSize: 11 }}
                    />
                    <Tooltip
                      cursor={{ fill: "var(--subtle)" }}
                      content={({ active, payload }) => {
                        const point = payload?.[0]?.payload as
                          (typeof chart.points)[number] | undefined;
                        return active && point ? (
                          <div
                            className="op-chart-tooltip"
                            dir={language === "ar" ? "rtl" : "ltr"}
                          >
                            <strong>{formatTime(point.at)}</strong>
                            {chart.groups.map((row, index) => (
                              <div key={row.id}>
                                <span>{label(row)}</span>
                                <bdi>
                                  {formatMoney(
                                    point.exact[`g${index}`],
                                    "detail",
                                  )}
                                </bdi>
                              </div>
                            ))}
                            {chart.hasOther && (
                              <div>
                                <span>
                                  {t(...translatedMessage("other_f6ed33d295"))}
                                </span>
                                <bdi>
                                  {formatMoney(point.exact.other, "detail")}
                                </bdi>
                              </div>
                            )}
                            <div>
                              <strong>
                                {t(...translatedMessage("total_d1e199d65d"))}
                              </strong>
                              <bdi>
                                {formatMoney(point.amount_nano, "detail")}
                              </bdi>
                            </div>
                          </div>
                        ) : null;
                      }}
                    />
                    {chart.groups.map((row, index) => (
                      <Bar
                        key={row.id}
                        dataKey={`g${index}`}
                        name={label(row)}
                        stackId="spend"
                        fill={colors[index]}
                        maxBarSize={40}
                        isAnimationActive={false}
                      />
                    ))}
                    {chart.hasOther && (
                      <Bar
                        dataKey="other"
                        stackId="spend"
                        fill={colors[7]}
                        maxBarSize={40}
                        isAnimationActive={false}
                      />
                    )}
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}
          </article>
          <div className="op-distributions">
            <Distribution
              title={t(...translatedMessage("by_service_group_06692a3bb8"))}
              rows={data.groups}
              total={data.totals.consumption_nano}
              label={label}
            />
            <Distribution
              title={t(...translatedMessage("by_vendor_649a66b027"))}
              rows={data.vendors}
              total={data.totals.consumption_nano}
              label={label}
            />
            <Distribution
              title={t(...translatedMessage("by_call_type_fa3489bed8"))}
              rows={data.call_types}
              total={data.totals.consumption_nano}
              label={(row) =>
                row.id ? callTypeLabel(row.id, language) : label(row)
              }
            />
          </div>
          <div className="op-rankings">
            {rank(
              t(
                ...translatedMessage("consumption_ranking_accounts_097c772fb8"),
              ),
              data.top_accounts,
              true,
            )}
            <div>
              {rank(
                t(
                  ...translatedMessage("consumption_ranking_models_9130c87364"),
                ),
                data.top_models,
                false,
              )}
              {data.models.length > 0 && (
                <Button
                  variant="ghost"
                  onClick={() => setShowModels(!showModels)}
                >
                  {showModels
                    ? t(
                        ...translatedMessage(
                          "hide_model_distribution_e2a9461bd4",
                        ),
                      )
                    : t(
                        ...translatedMessage(
                          "all_model_distribution_cbf9e1e13d",
                        ),
                      )}
                </Button>
              )}
            </div>
          </div>
          {showModels && (
            <Distribution
              title={t(...translatedMessage("by_model_31a31dc6e4"))}
              rows={data.models}
              total={data.totals.consumption_nano}
              label={label}
            />
          )}
          <details className="op-method">
            <summary>
              {t(
                ...translatedMessage(
                  "definitions_and_funding_sources_a480c67a43",
                ),
              )}
            </summary>
            <p>
              {t(
                ...translatedMessage(
                  "customer_accounts_only_internal_employees_are_16425ff120",
                ),
              )}
            </p>
            <p>
              {t(
                ...translatedMessage(
                  "top_ups_and_paying_accounts_currently_reflect_91fee0e62a",
                ),
              )}
            </p>
            <dl>
              {[
                [
                  t(
                    ...translatedMessage(
                      "registered_offline_receipts_ef3d9b0dec",
                    ),
                  ),
                  data.funding.offline_nano,
                ],
                [
                  t(...translatedMessage("simulated_top_ups_df7dda4967")),
                  data.funding.simulation_nano,
                ],
                [
                  t(
                    ...translatedMessage(
                      "unverified_historical_credits_161a10d02f",
                    ),
                  ),
                  data.funding.unverified_nano,
                ],
                [
                  t(
                    ...translatedMessage(
                      "gifts_compensation_credits_2daa7f2d65",
                    ),
                  ),
                  data.funding.other_credit_nano,
                ],
              ].map(([name, value]) => (
                <div key={name}>
                  <dt>{name}</dt>
                  <dd>
                    <bdi>{formatOperatingAmount(value)}</bdi>
                  </dd>
                </div>
              ))}
            </dl>
          </details>
        </>
      )}
    </section>
  );
}
