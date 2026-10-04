import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { RefreshCw } from "lucide-react";
import { Button } from "../components/ui/button";
import { Badge, Segmented } from "../components/ui/primitives";
import { DataStateView } from "../components/ui/states";
import { TierBadge } from "../components/tier-badge";
import { usePreferences } from "../lib/preferences";
import { formatMoney, sumNano } from "../lib/money";
import { getActivity } from "../mocks/service";
import { placeholders, type TierId } from "../config/placeholders";

export function Activity() {
  const { t } = useTranslation();
  const { language, dataState, setDataState } = usePreferences();
  const [period, setPeriod] = useState("7");
  const [metric, setMetric] = useState("cost");
  const [tierId, setTierId] = useState<TierId>("pro");
  const query = useQuery({
    queryKey: ["activity"],
    queryFn: ({ signal }) => getActivity(signal),
  });
  const points = useMemo(
    () => (query.data ?? []).slice(-Number(period)),
    [query.data, period],
  );
  const chart = points.map((point) => ({
    ...point,
    value:
      metric === "cost"
        ? Number(BigInt(point.cost_nano) / 10000n) / 100000
        : point.requests,
    comparison:
      metric === "cost"
        ? Number(BigInt(point.previous_nano) / 10000n) / 100000
        : point.previousRequests,
  }));
  const tier = placeholders.tiers.find((t) => t.id === tierId)!;
  const state = query.isPending
    ? "loading"
    : query.isError
      ? "error"
      : dataState;
  const reset = () => {
    setDataState("ready");
    void query.refetch();
  };
  const locale =
    language === "ar"
      ? "ar-AE-u-nu-latn"
      : language === "zh"
        ? "zh-CN"
        : "en-US";
  const formatDay = (date: string) =>
    new Intl.DateTimeFormat(locale, {
      month: "short",
      day: "numeric",
      timeZone: placeholders.region.timeZone,
    }).format(new Date(date));
  return (
    <div className="activity-grid">
      <div className="activity-panel">
        <div className="panel-header">
          <div>
            <h3>{t("activityTitle")}</h3>
            <p>{t("activityNote")}</p>
          </div>
          <Badge>{t("sampleData")}</Badge>
        </div>
        <div className="chart-toolbar">
          <Segmented
            label={t("chartMetric")}
            value={metric}
            onChange={setMetric}
            options={[
              { value: "cost", label: t("cost") },
              { value: "requests", label: t("requests") },
            ]}
          />
          <div className="inline-controls">
            <Segmented
              label={t("period")}
              value={period}
              onChange={setPeriod}
              options={[
                { value: "7", label: t("last7") },
                { value: "30", label: t("last30") },
              ]}
            />
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("refresh")}
              onClick={() => void query.refetch()}
            >
              <RefreshCw className={query.isFetching ? "spin" : ""} />
            </Button>
          </div>
        </div>
        {state !== "ready" ? (
          <DataStateView state={state} onReset={reset} />
        ) : (
          <>
            <div className="metrics-strip">
              <div className="main-metric">
                <span>{t("totalCost")}</span>
                <strong className="numeric ltr">
                  {formatMoney(
                    sumNano(points.map((p) => p.cost_nano)),
                    "total",
                  )}
                </strong>
              </div>
              <div>
                <span>{t("requests")}</span>
                <strong className="numeric ltr">
                  {points
                    .reduce((n, p) => n + p.requests, 0)
                    .toLocaleString("en-US")}
                </strong>
              </div>
            </div>
            <div
              className="chart"
              role="img"
              aria-label={`${t("chartLabel")}. ${t("chartSummary", { count: points.reduce((n, p) => n + p.requests, 0), days: period })}`}
            >
              <ResponsiveContainer width="100%" height="100%">
                <LineChart
                  data={chart}
                  margin={{ top: 16, right: 12, bottom: 4, left: 0 }}
                  accessibilityLayer
                >
                  <CartesianGrid
                    vertical={false}
                    stroke="var(--grid)"
                    strokeDasharray="3 4"
                  />
                  <XAxis
                    reversed={language === "ar"}
                    dataKey="date"
                    tickFormatter={formatDay}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={32}
                    tick={{ fill: "var(--muted)", fontSize: "var(--text-xs)" }}
                    tickMargin={12}
                  />
                  <YAxis
                    orientation={language === "ar" ? "right" : "left"}
                    axisLine={false}
                    tickLine={false}
                    width={48}
                    tickFormatter={(v) =>
                      metric === "cost" ? `$${v}` : v.toLocaleString("en-US")
                    }
                    tick={{ fill: "var(--muted)", fontSize: "var(--text-xs)" }}
                  />
                  <Tooltip
                    content={({ active, payload, label }) =>
                      active && payload?.length ? (
                        <div className="chart-tooltip">
                          <span>{formatDay(String(label))}</span>
                          {payload.map((item) => (
                            <div key={String(item.dataKey)}>
                              <span>
                                {t(
                                  item.dataKey === "value"
                                    ? "current"
                                    : "previous",
                                )}
                              </span>
                              <b dir="ltr">
                                {metric === "cost"
                                  ? formatMoney(
                                      item.dataKey === "value"
                                        ? item.payload.cost_nano
                                        : item.payload.previous_nano,
                                      "total",
                                    )
                                  : item.value?.toLocaleString("en-US")}
                              </b>
                            </div>
                          ))}
                        </div>
                      ) : null
                    }
                  />
                  <Line
                    name={t("previous")}
                    type="monotone"
                    dataKey="comparison"
                    stroke="var(--chart-4)"
                    strokeDasharray="5 5"
                    strokeWidth={1.5}
                    dot={false}
                    activeDot={false}
                    isAnimationActive={false}
                  />
                  <Line
                    name={t("current")}
                    type="monotone"
                    dataKey="value"
                    stroke="var(--chart-1)"
                    strokeWidth={2.5}
                    dot={false}
                    activeDot={{
                      r: 4,
                      fill: "var(--chart-1)",
                      stroke: "var(--surface)",
                      strokeWidth: 2,
                    }}
                    isAnimationActive={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
            <div className="chart-footer">
              <div className="chart-legend">
                <span>
                  <i className="legend-current" />
                  {t("current")}
                </span>
                <span>
                  <i className="legend-previous" />
                  {t("previous")}
                </span>
              </div>
              <small>{t("chartNote")}</small>
            </div>
          </>
        )}
      </div>
      <div className="tier-panel">
        <div className="panel-header">
          <div>
            <h3>{t("policyTitle")}</h3>
            <p>{t("policySubtitle")}</p>
          </div>
        </div>
        <div
          className="tier-options"
          role="radiogroup"
          aria-label={t("selectTier")}
        >
          {placeholders.tiers.map((option) => (
            <button
              key={option.id}
              className="tier-option"
              role="radio"
              aria-checked={tierId === option.id}
              onClick={() => setTierId(option.id)}
              onKeyDown={(event) => {
                if (["ArrowDown", "ArrowUp"].includes(event.key)) {
                  event.preventDefault();
                  const index = placeholders.tiers.findIndex(
                    (item) => item.id === tierId,
                  );
                  const next =
                    placeholders.tiers[
                      (index + (event.key === "ArrowDown" ? 1 : 3)) % 4
                    ];
                  setTierId(next.id);
                  (
                    event.currentTarget.parentElement?.children[
                      (index + (event.key === "ArrowDown" ? 1 : 3)) % 4
                    ] as HTMLElement
                  )?.focus();
                }
              }}
              tabIndex={tierId === option.id ? 0 : -1}
            >
              <TierBadge id={option.id} />
              <span className="radio-indicator" />
            </button>
          ))}
        </div>
        <dl className="tier-metrics">
          <div>
            <dt>{t("target")}</dt>
            <dd className="numeric ltr">≥ {tier.success}%</dd>
          </div>
          <div>
            <dt>{t("latency")}</dt>
            <dd className="numeric ltr">
              ≤ {tier.p95.toLocaleString("en-US")} ms
            </dd>
          </div>
        </dl>
        <p className="tier-footnote">
          {t("tierFootnote")}
          {tierId === "lite" && <> {t("tierCapacity")}</>}
        </p>
      </div>
    </div>
  );
}
