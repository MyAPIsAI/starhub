import { ResponsiveTable } from "../components/responsive-table";
import { message as translatedMessage } from "../i18n/messages";

import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import {
  Activity,
  ArrowDownToLine,
  ArrowRight,
  ArrowLeft,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Clock3,
  Filter,
  Info,
  Layers3,
  ShieldCheck,
  TriangleAlert,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { usePreferences } from "../lib/preferences";
import { writtenLanguage } from "../lib/localize";
import {
  createQualityTrend,
  createQualityUnits,
  getQualityMonths,
  filterQualityUnits,
  serializeQualityCsv,
  summarizeQualityUnits,
  type QualityMetric,
  type QualityStatus,
  type QualityUnit,
  type QualityTrend,
} from "../lib/service-quality-demo";
import {
  useServiceQuality,
  type ServiceQualitySummary,
  type ServiceQualityDailyMetric,
} from "../lib/api/service-quality";
import "../components/service-quality.css";
import { catalogModels } from "../config/models";
import { placeholders, type TierId } from "../config/placeholders";

type Copy = (zh: string, en: string, ar: string) => string;
const statusWords: Record<QualityStatus, [string, string, string]> = {
  pass: translatedMessage("meets_target_9d67fb7463"),
  near: translatedMessage("near_threshold_cfb9f88dd1"),
  below: translatedMessage("currently_below_target_b93d9ffb66"),
  insufficient: translatedMessage("insufficient_samples_c012179ddb"),
  incomplete: translatedMessage("evidence_unverified_732b4a0836"),
  unconfigured: translatedMessage("no_contract_target_dad1dc3d11"),
};
const number = (value: number) => new Intl.NumberFormat("en-US").format(value);
const percent = (value: number) => `${value.toFixed(2)}%`;
const seconds = (value: number) => `${(value / 1000).toFixed(2)} s`;

function StatusPill({ status, text }: { status: QualityStatus; text: Copy }) {
  return (
    <span className={`sq-status is-${status}`}>
      {status === "pass" ? (
        <Check size={11} />
      ) : status === "below" ? (
        <TriangleAlert size={11} />
      ) : (
        <span className="sq-status-dot" />
      )}
      {text(...statusWords[status])}
    </span>
  );
}

function TrendChart({
  rows,
  metric,
  target,
  text,
}: {
  rows: QualityTrend[];
  metric: QualityMetric;
  target: number | null;
  text: Copy;
}) {
  const [hovered, setHovered] = useState<number | null>(null);
  const values = rows.map((row) =>
    metric === "success" ? row.success : row[metric] / 1000,
  );
  const boundValues = [...values, ...(target === null ? [] : [target])];
  const padding = metric === "success" ? 0.07 : 0.1;
  const minimum = Math.max(0, Math.min(...boundValues) - padding);
  const maximum =
    metric === "success"
      ? Math.min(100, Math.max(...boundValues) + padding)
      : Math.max(...boundValues) + padding;
  const range = Math.max(maximum - minimum, 0.01);
  const x = (index: number) =>
    rows.length === 1 ? 340 : 58 + (index * 582) / (rows.length - 1);
  const y = (value: number) => 180 - ((value - minimum) / range) * 150;
  const points = values
    .map((value, index) => `${x(index)},${y(value)}`)
    .join(" ");
  const activeIndex =
    hovered !== null && hovered < rows.length ? hovered : rows.length - 1;
  const active = rows[activeIndex];
  const format = (value: number) =>
    metric === "success" ? percent(value) : `${value.toFixed(2)} s`;
  return (
    <div className="sq-trend-chart">
      <div className="sq-trend-current">
        <span>{active?.date}</span>
        <strong>
          {values[activeIndex] !== undefined
            ? format(values[activeIndex])
            : "—"}
        </strong>
        <span>
          {text(...translatedMessage("daily_observation_eb8ccec574"))}
        </span>
      </div>
      <svg
        viewBox="0 0 672 221"
        role="img"
        aria-label={text(
          ...translatedMessage(
            "daily_operating_trend_dashed_line_marks_the_c_e02d7a6730",
          ),
        )}
      >
        {[0, 1, 2, 3].map((tick) => {
          const value = minimum + (range * tick) / 3;
          return (
            <g key={tick}>
              <line
                x1="58"
                x2="640"
                y1={y(value)}
                y2={y(value)}
                className="sq-chart-grid"
              />
              <text x="48" y={y(value) + 3} textAnchor="end">
                {metric === "success"
                  ? `${value.toFixed(1)}%`
                  : value.toFixed(2)}
              </text>
            </g>
          );
        })}
        {target !== null && (
          <>
            <line
              x1="58"
              x2="640"
              y1={y(target)}
              y2={y(target)}
              className="sq-chart-target"
            />
            <text
              x="640"
              y={Math.max(12, y(target) - 8)}
              textAnchor="end"
              className="sq-target-label"
            >
              {text(...translatedMessage("target_4ee8b0a32e"))} {format(target)}
            </text>
          </>
        )}
        {rows.length > 1 && (
          <polygon
            points={`58,180 ${points} 640,180`}
            className="sq-chart-fill"
          />
        )}
        <polyline points={points} className="sq-chart-line" />
        {rows.map((row, index) => (
          <g key={row.date}>
            <circle
              cx={x(index)}
              cy={y(values[index])}
              r={activeIndex === index ? 4.5 : 3}
              className="sq-chart-point"
              tabIndex={0}
              onFocus={() => setHovered(index)}
              onMouseEnter={() => setHovered(index)}
              onMouseLeave={() => setHovered(null)}
              onBlur={() => setHovered(null)}
              aria-label={`${row.date}: ${format(values[index])}`}
            >
              <title>
                {row.date}: {format(values[index])}
              </title>
            </circle>
            {(index === 0 || index === rows.length - 1 || index % 3 === 0) && (
              <text x={x(index)} y="207" textAnchor="middle">
                {row.date.slice(5)}
              </text>
            )}
          </g>
        ))}
      </svg>
      <div className="sq-chart-legend">
        <span>
          <i />
          {text(...translatedMessage("observed_4211a76613"))}
        </span>
        {target !== null && (
          <span>
            <i className="is-target" />
            {text(...translatedMessage("contract_target_2289ca3587"))}
          </span>
        )}
        <small>
          {text(...translatedMessage("vertical_axis_auto_scales_44d4f1cc56"))}
        </small>
      </div>
    </div>
  );
}

export default function ServiceQuality({
  isAdmin = false,
  userId = 1001,
}: {
  isAdmin?: boolean;
  userId?: number;
}) {
  const language = usePreferences((state) => state.language);
  const text: Copy = (zh, en, ar) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const months = useMemo(getQualityMonths, []);
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const navigate = useNavigate();
  const month = params.get("month") || months[0];
  const group = params.get("group") || "all";
  const model = params.get("model") || "all";
  const requestType = params.get("type") || "all";
  const view =
    params.get("view") === "admin"
      ? "admin"
      : params.get("view") === "enterprise"
        ? "enterprise"
        : isAdmin
          ? "admin"
          : "enterprise";
  const customer = params.get("customer") || "";
  const selectedId = params.get("unit") || "";
  const metric = (
    ["success", "g0", "g1", "g2"].includes(params.get("metric") || "")
      ? params.get("metric")
      : "success"
  ) as QualityMetric;
  const page = Math.max(1, Math.floor(Number(params.get("page")) || 1));
  const pageSize = params.get("size") === "10" ? 10 : 5;
  const change = (updates: Record<string, string | null>, replace = false) => {
    const next = new URLSearchParams(params);
    for (const [key, value] of Object.entries(updates)) {
      if (value === null || value === "") next.delete(key);
      else next.set(key, value);
    }
    setParams(next, { replace, state: location.state });
  };
  const [guideOpen, setGuideOpen] = useState(false);
  const [exportMessage, setExportMessage] = useState("");
  useEffect(() => setExportMessage(""), [language]);
  const detailHeading = useRef<HTMLHeadingElement>(null);
  const previousUnit = useRef(selectedId);
  const returnPosition = useRef<{ scroll: number; unit: string } | null>(
    location.state?.qualityReturn ?? null,
  );
  const qualityQuery = useServiceQuality(168); // last 7 days
  // When backend data is available, adapt it to QualityUnit[].
  // Otherwise fall back to the demo seeds.
  const adaptToUnits = (summary: ServiceQualitySummary): QualityUnit[] =>
    summary.by_model.map((m, i) => ({
      id: `sq_${i}`,
      userId: 0,
      group: "default" as TierId,
      tier: "Pro" as const,
      model: m.model,
      stream: false,
      version: "1",
      requests: m.total_requests,
      successes: m.success_count,
      successRate: m.success_rate * 100,
      successTarget: 99.5,
      completeness: null,
      minSamples: 100,
      unknownInputs: 0,
      buckets: [
        {
          id: "g0" as const,
          p95: m.p95_duration_ms,
          limit: 3000,
          samples: m.total_requests,
          status: (m.p95_duration_ms > 0 && m.p95_duration_ms < 3000
            ? "pass"
            : m.p95_duration_ms === 0
              ? "insufficient"
              : "below") as QualityStatus,
        },
      ],
      successStatus: (m.success_rate >= 0.995
        ? "pass"
        : m.success_rate >= 0.97
          ? "near"
          : m.total_requests === 0
            ? "insufficient"
            : "below") as QualityStatus,
      performanceStatus:
        m.p95_duration_ms > 0
          ? ((m.p95_duration_ms < 3000 ? "pass" : "below") as QualityStatus)
          : null,
      status: (m.success_rate >= 0.995
        ? "pass"
        : m.success_rate >= 0.97
          ? "near"
          : m.total_requests === 0
            ? "insufficient"
            : "below") as QualityStatus,
    }));
  const allUnits = useMemo(
    () =>
      qualityQuery.data
        ? adaptToUnits(qualityQuery.data)
        : months.includes(month)
          ? createQualityUnits(month)
          : [],
    [qualityQuery.data, month, months],
  );
  // Cache per-model SQ metrics so the model detail page can show a quick
  // success/P95 summary without a separate fetch. Keyed by model id only,
  // since the backend aggregates per model, not per model+group.
  useEffect(() => {
    const byModel = qualityQuery.data?.by_model;
    if (!byModel) return;
    try {
      for (const m of byModel) {
        localStorage.setItem(
          `sq-${m.model}`,
          JSON.stringify({
            success: m.success_rate,
            p95: m.p95_duration_ms,
          }),
        );
      }
    } catch {
      /* localStorage unavailable */
    }
  }, [qualityQuery.data]);
  const units = useMemo(
    () =>
      filterQualityUnits(allUnits, {
        userId:
          view === "enterprise"
            ? userId
            : customer
              ? Number(customer)
              : undefined,
        group: group === "all" ? undefined : group,
        model: model === "all" ? undefined : model,
        stream: requestType === "all" ? undefined : requestType === "streaming",
      }),
    [allUnits, view, userId, customer, group, model, requestType],
  );
  const summary = summarizeQualityUnits(units);
  const selected = params.has("month")
    ? allUnits.find(
        (unit) =>
          unit.id === selectedId &&
          (view === "admin" || unit.userId === userId),
      )
    : undefined;
  const currentPage = Math.max(
    1,
    Math.min(page, Math.ceil(units.length / pageSize)),
  );
  const visibleUnits = units.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  const trend = useMemo(() => {
    // Use real backend by_day data when available; fall back to demo.
    const byDay = qualityQuery.data?.by_day;
    if (byDay && byDay.length > 0) {
      return byDay.map((d: ServiceQualityDailyMetric) => ({
        date: d.date,
        success: d.success_rate * 100,
        g0: d.avg_duration_ms,
        g1: 0,
        g2: 0,
      })) as QualityTrend[];
    }
    return selected ? createQualityTrend(selected, month) : [];
  }, [qualityQuery.data, selected, month]);
  const chartMetric: QualityMetric = !selected?.stream ? "success" : metric;
  const target = selected?.successTarget
    ? chartMetric === "success"
      ? selected.successTarget
      : (selected.buckets.find((bucket) => bucket.id === chartMetric)?.limit ??
          0) / 1000
    : null;
  const openUnit = (unit: QualityUnit) => {
    returnPosition.current = { scroll: window.scrollY, unit: unit.id };
    const next = new URLSearchParams(params);
    next.set("month", month);
    next.set("unit", unit.id);
    next.delete("metric");
    setExportMessage("");
    setParams(next, {
      state: {
        qualityReturn: returnPosition.current,
        qualityReturnSearch: params.toString(),
      },
    });
  };
  const returnToList = () => {
    if (typeof location.state?.qualityReturnSearch === "string") navigate(-1);
    else change({ unit: null, metric: null }, true);
  };
  useEffect(() => {
    const last = previousUnit.current;
    previousUnit.current = selectedId;
    const frame = requestAnimationFrame(() => {
      if (selectedId) {
        window.scrollTo({ top: 0 });
        detailHeading.current?.focus({ preventScroll: true });
      } else if (last) {
        const target = Array.from(
          document.querySelectorAll<HTMLButtonElement>(
            ".sq-row-view, .sq-mobile-unit button",
          ),
        ).find(
          (button) =>
            button.dataset.unitId === (returnPosition.current?.unit || last) &&
            button.getClientRects().length,
        );
        if (returnPosition.current)
          window.scrollTo({ top: returnPosition.current.scroll });
        else target?.scrollIntoView({ block: "center" });
        target?.focus({ preventScroll: true });
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [selectedId]);
  const changeFilter =
    (key: string) =>
    (event: React.ChangeEvent<HTMLSelectElement | HTMLInputElement>) => {
      change({
        [key]: event.target.value,
        unit: null,
        page: null,
        metric: null,
      });
      setExportMessage("");
    };
  const requestLabel = (unit: QualityUnit) =>
    unit.stream
      ? text(...translatedMessage("streaming_b104d369da"))
      : text(...translatedMessage("non_streaming_30b0b5fa15"));
  const groupLabel = (value: string) => {
    const tier = placeholders.tiers.find((item) => item.id === value);
    return tier?.name[writtenLanguage(language)] ?? value;
  };
  const modelLabel = (value: string) =>
    catalogModels.find((item) => item.id === value)?.name ?? value;
  const unitLabel = (unit: QualityUnit) =>
    `#${unit.userId} · ${groupLabel(unit.group)} · ${unit.model} · ${requestLabel(unit)}`;
  const periodEnd = !months.includes(month)
    ? "—"
    : month === months[0]
      ? new Date().toISOString().slice(0, 10)
      : new Date(Date.UTC(Number(month.slice(0, 4)), Number(month.slice(5)), 0))
          .toISOString()
          .slice(0, 10);
  const resetFilters = () => {
    setParams({});
    setExportMessage("");
  };
  const exportReport = () => {
    const header = [
      text(...translatedMessage("period_7709fd631e")),
      text(...translatedMessage("period_start_7fc96fa14f")),
      text(...translatedMessage("data_as_of_6841eec7bf")),
      text(...translatedMessage("reconciliation_status_fc9f3a80b7")),
      text(...translatedMessage("user_id_cf211a0bf6")),
      text(...translatedMessage("group_528d77dde5")),
      text(...translatedMessage("standard_model_7e296caf71")),
      text(...translatedMessage("request_type_881c7ae173")),
      text(...translatedMessage("contract_version_8271dccca7")),
      text(...translatedMessage("final_success_rate_beac9517b2")),
      text(...translatedMessage("success_target_363d8b54c8")),
      text(...translatedMessage("logical_requests_175a71490c")),
      text(...translatedMessage("g0_p95_ms_c823f97ec6")),
      text(...translatedMessage("g1_p95_ms_9d27b86312")),
      text(...translatedMessage("g2_p95_ms_8c33c6d1ea")),
      text(...translatedMessage("overall_status_014db241c3")),
      text(...translatedMessage("data_source_9e4e61f0d7")),
    ];
    const reportUnits = selectedId ? (selected ? [selected] : []) : units;
    const rows = reportUnits.map((unit) => [
      month,
      `${month}-01`,
      periodEnd,
      text(...translatedMessage("demo_data_not_finally_reconciled_9d4e318cd6")),
      unit.userId,
      groupLabel(unit.group),
      unit.model,
      requestLabel(unit),
      unit.version,
      percent(unit.successRate),
      unit.successTarget ? percent(unit.successTarget) : "—",
      unit.requests,
      ...[0, 1, 2].map((index) => unit.buckets[index]?.p95 ?? ""),
      text(...statusWords[unit.status]),
      text(
        ...translatedMessage(
          "frontend_demo_not_production_delivery_records_77ef44aaa5",
        ),
      ),
    ]);
    const csv = serializeQualityCsv([header, ...rows]);
    const url = URL.createObjectURL(
      new Blob([csv], { type: "text/csv;charset=utf-8" }),
    );
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `veyra-service-quality-${month}${selected ? `-${selected.id}` : ""}-as-of-${periodEnd}.csv`;
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    setExportMessage(
      text(
        ...translatedMessage("exported_accounting_units_1868844832", {
          p0: reportUnits.length,
        }),
      ),
    );
  };
  const summaries = [
    {
      label: text(...translatedMessage("accounting_units_1dd223488b")),
      value: number(summary.total),
      description: text(
        ...translatedMessage("calculated_by_contract_scope_960b09b93e"),
      ),
      Icon: Layers3,
      tone: "blue",
    },
    {
      label: text(...translatedMessage("success_needs_attention_1e2495426c")),
      value: number(summary.successAttention),
      description: text(
        ...translatedMessage(
          "near_target_below_target_or_unverified_4b5652add5",
        ),
      ),
      Icon: ShieldCheck,
      tone: "amber",
    },
    {
      label: text(
        ...translatedMessage("performance_needs_attention_9b2bff4208"),
      ),
      value: number(summary.performanceAttention),
      description: text(
        ...translatedMessage(
          "buckets_requiring_review_or_more_samples_a6ebe21154",
        ),
      ),
      Icon: Clock3,
      tone: "purple",
    },
    {
      label: text(...translatedMessage("logical_requests_cbda782b3e")),
      value: number(summary.logicalRequests),
      description: text(
        ...translatedMessage(
          "final_requests_retries_are_not_duplicated_9500860441",
        ),
      ),
      Icon: Activity,
      tone: "green",
    },
  ];

  return (
    <div className="sq-page">
      {selectedId && (
        <nav
          className="sq-detail-breadcrumb"
          aria-label={text(
            ...translatedMessage("detail_navigation_4bdef7f019"),
          )}
        >
          <Button variant="ghost" size="small" onClick={returnToList}>
            <ArrowLeft size={15} />
            {text(...translatedMessage("back_to_delivery_list_0ff83705b1"))}
          </Button>
          <span>/</span>
          <span>
            {selected
              ? `${modelLabel(selected.model)} · ${groupLabel(selected.group)} · ${requestLabel(selected)} · ${month}`
              : text(...translatedMessage("unit_delivery_details_79a6a639fc"))}
          </span>
        </nav>
      )}
      <header className="sq-heading">
        <div>
          <span className="sq-eyebrow">
            <ShieldCheck size={14} />
            {text(...translatedMessage("service_observability_a6336cdfb3"))}
          </span>
          <h1 ref={detailHeading} tabIndex={-1}>
            {selectedId
              ? text(...translatedMessage("unit_delivery_details_79a6a639fc"))
              : text(...translatedMessage("service_quality_87d4d242c9"))}
          </h1>
          <p>
            {text(
              selectedId
                ? "以下指标仅属于当前核算单元。"
                : "按核算单元比较交付表现，进入详情查看分桶、趋势与核算结果。",
              selectedId
                ? "All metrics below belong to this accounting unit only."
                : "Compare accounting units, then open details for latency buckets, trends and results.",
              selectedId
                ? "تخص جميع المقاييس أدناه هذه الوحدة فقط."
                : "قارن الوحدات وافتح التفاصيل لمراجعة الحاويات والاتجاهات والنتائج.",
            )}
          </p>
        </div>
        <div className="sq-heading-actions">
          <Button size="small" onClick={() => setGuideOpen(true)}>
            <BookOpen size={15} />
            {text(...translatedMessage("metric_guide_b1722c2adc"))}
          </Button>
          <Button
            variant="primary"
            size="small"
            onClick={exportReport}
            disabled={selectedId ? !selected : !units.length}
          >
            <ArrowDownToLine size={15} />
            {selectedId
              ? text(...translatedMessage("export_this_unit_1c962e840f"))
              : text(...translatedMessage("export_monthly_report_a00078c37e"))}
          </Button>
        </div>
      </header>
      <div className="sq-demo-banner">
        <span>
          <Info size={14} />
          <strong>
            {text(...translatedMessage("frontend_demo_fe7f06c736"))}
          </strong>
          {text(
            ...translatedMessage(
              "metrics_contracts_and_targets_are_illustrativ_9a8714d236",
            ),
          )}
        </span>
        {!selectedId && (
          <label>
            {text(...translatedMessage("demo_view_c976909d72"))}
            <select
              aria-label={text(...translatedMessage("demo_view_c976909d72"))}
              value={view}
              onChange={changeFilter("view")}
            >
              <option value="enterprise">
                {text(...translatedMessage("my_account_960c2b2794"))}
              </option>
              <option value="admin">
                {text(...translatedMessage("platform_view_4c2504c37e"))}
              </option>
            </select>
          </label>
        )}
      </div>
      {exportMessage && (
        <div className="sq-export-message" role="status">
          <CheckCircle2 size={14} />
          {exportMessage}
        </div>
      )}
      {!selectedId && (
        <>
          <section
            className="sq-card sq-filter-card"
            aria-label={text(
              ...translatedMessage("service_quality_filters_977f25bfc6"),
            )}
          >
            <div className="sq-card-heading">
              <div>
                <h2>
                  <Filter size={15} />
                  {text(
                    ...translatedMessage(
                      "contract_delivery_overview_200886336b",
                    ),
                  )}
                </h2>
                <p>
                  {text(
                    ...translatedMessage(
                      "account_separately_by_customer_group_model_re_16ccb6943c",
                    ),
                  )}
                </p>
              </div>
              <div className="sq-as-of">
                <span>
                  {text(...translatedMessage("demo_data_as_of_5748618226"))}{" "}
                  <b>{periodEnd}</b>
                </span>
                <small>
                  {text(
                    ...translatedMessage(
                      "demo_results_have_not_been_finally_reconciled_07ee39162a",
                    ),
                  )}
                </small>
              </div>
            </div>
            <div className="sq-filters">
              <label>
                <span>{text(...translatedMessage("user_id_cf211a0bf6"))}</span>
                <input
                  type="text"
                  inputMode="numeric"
                  aria-label={text(...translatedMessage("user_id_cf211a0bf6"))}
                  value={view === "enterprise" ? userId : customer}
                  disabled={view === "enterprise"}
                  placeholder={text(
                    ...translatedMessage("all_demo_customers_c447d3e1c3"),
                  )}
                  onChange={(event) => {
                    if (/^\d*$/.test(event.target.value)) {
                      change(
                        { customer: event.target.value, page: null },
                        true,
                      );
                    }
                  }}
                />
              </label>
              <label>
                <span>
                  {text(...translatedMessage("accounting_period_b55b69782a"))}
                </span>
                <select
                  aria-label={text(
                    ...translatedMessage("accounting_period_b55b69782a"),
                  )}
                  value={month}
                  onChange={changeFilter("month")}
                >
                  {months.map((value) => (
                    <option key={value} value={value}>
                      {value}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span>{text(...translatedMessage("group_528d77dde5"))}</span>
                <select
                  aria-label={text(...translatedMessage("group_528d77dde5"))}
                  value={group}
                  onChange={(event) => {
                    const value = event.target.value;
                    let nextModel = model;
                    if (
                      model !== "all" &&
                      value !== "all" &&
                      !catalogModels.find((item) => item.id === model)
                        ?.tierAvailability[
                        value as keyof (typeof catalogModels)[number]["tierAvailability"]
                      ]
                    )
                      nextModel = "all";
                    change({ group: value, model: nextModel, page: null });
                  }}
                >
                  <option value="all">
                    {text(...translatedMessage("all_groups_051837cdeb"))}
                  </option>
                  {placeholders.tiers
                    .map((item) => item.id)
                    .map((value) => (
                      <option key={value} value={value}>
                        {groupLabel(value)}
                      </option>
                    ))}
                </select>
              </label>
              <label>
                <span>
                  {text(...translatedMessage("standard_model_18b603bd24"))}
                </span>
                <select
                  aria-label={text(
                    ...translatedMessage("standard_model_18b603bd24"),
                  )}
                  value={model}
                  onChange={changeFilter("model")}
                >
                  <option value="all">
                    {text(...translatedMessage("all_models_aa0962e85e"))}
                  </option>
                  {catalogModels
                    .filter(
                      (item) =>
                        group === "all" ||
                        item.tierAvailability[
                          group as keyof typeof item.tierAvailability
                        ],
                    )
                    .map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name}
                      </option>
                    ))}
                </select>
              </label>
              <label>
                <span>
                  {text(...translatedMessage("request_type_881c7ae173"))}
                </span>
                <select
                  aria-label={text(
                    ...translatedMessage("request_type_881c7ae173"),
                  )}
                  value={requestType}
                  onChange={changeFilter("type")}
                >
                  <option value="all">
                    {text(...translatedMessage("all_types_a17f90eb5e"))}
                  </option>
                  <option value="streaming">
                    {text(...translatedMessage("streaming_b104d369da"))}
                  </option>
                  <option value="non-streaming">
                    {text(...translatedMessage("non_streaming_30b0b5fa15"))}
                  </option>
                </select>
              </label>
            </div>
          </section>
          <div className="sq-summary-grid">
            {summaries.map(({ label, value, description, Icon, tone }) => (
              <section className="sq-card sq-summary" key={label}>
                <div>
                  <span>{label}</span>
                  <span className={`sq-summary-icon is-${tone}`}>
                    <Icon size={17} />
                  </span>
                </div>
                <strong>{value}</strong>
                <p>{description}</p>
              </section>
            ))}
          </div>
          <section className="sq-card sq-table-card">
            <div className="sq-card-heading">
              <div>
                <h2>
                  {text(
                    ...translatedMessage(
                      "contract_accounting_units_e8b119504d",
                    ),
                  )}
                </h2>
                <p>
                  {text(
                    ...translatedMessage(
                      "each_accounting_unit_stands_alone_results_can_54cb7a6497",
                    ),
                  )}
                </p>
              </div>
              <span className="sq-count">
                {units.length} {text(...translatedMessage("units_87cfe69ca2"))}
              </span>
            </div>
            {!units.length ? (
              <div className="sq-empty">
                <Layers3 size={34} />
                <h3>
                  {text(
                    ...translatedMessage(
                      "no_matching_accounting_units_0257bb964d",
                    ),
                  )}
                </h3>
                <p>
                  {text(
                    ...translatedMessage(
                      "try_another_customer_group_or_model_for_this__495971175c",
                    ),
                  )}
                </p>
                <Button onClick={resetFilters}>
                  {text(...translatedMessage("reset_filters_a7e85112c6"))}
                </Button>
              </div>
            ) : (
              <>
                <div className="sq-table-scroll">
                  <ResponsiveTable>
                    <thead>
                      <tr>
                        {[
                          text(
                            ...translatedMessage(
                              "group_service_tier_62e2a9c6ed",
                            ),
                          ),
                          text(
                            ...translatedMessage("standard_model_7e296caf71"),
                          ),
                          text(...translatedMessage("request_type_881c7ae173")),
                          text(
                            ...translatedMessage(
                              "final_success_rate_beac9517b2",
                            ),
                          ),
                          text(
                            ...translatedMessage(
                              "first_token_time_3_buckets_3b98bd9f7e",
                            ),
                          ),
                          text(...translatedMessage("samples_03a4a40574")),
                          text(
                            ...translatedMessage("overall_status_bf18e4a396"),
                          ),
                          text(...translatedMessage("action_2a8e78b55a")),
                        ].map((label) => (
                          <th key={label}>{label}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {visibleUnits.map((unit) => (
                        <tr
                          key={unit.id}
                          className={
                            unit.id === selected?.id ? "is-selected" : ""
                          }
                        >
                          <td>
                            <strong>{groupLabel(unit.group)}</strong>
                            <small>
                              #{unit.userId} · {unit.version}
                            </small>
                          </td>
                          <td>
                            <span className="sq-model-name">
                              {modelLabel(unit.model)}
                            </span>
                            <small>{unit.model}</small>
                          </td>
                          <td>
                            <span className="sq-type">
                              {requestLabel(unit)}
                            </span>
                          </td>
                          <td>
                            <strong
                              className={
                                unit.successStatus === "below"
                                  ? "sq-negative"
                                  : ""
                              }
                            >
                              {percent(unit.successRate)}
                            </strong>
                            <small>
                              {unit.successTarget
                                ? `${text(...translatedMessage("target_4ee8b0a32e"))} ≥ ${percent(unit.successTarget)}`
                                : text(
                                    ...translatedMessage(
                                      "not_configured_57e6a4a78d",
                                    ),
                                  )}
                            </small>
                          </td>
                          <td>
                            {unit.stream ? (
                              <div className="sq-inline-buckets">
                                {unit.buckets.map((bucket) => (
                                  <span
                                    key={bucket.id}
                                    className={
                                      bucket.status === "below"
                                        ? "is-below"
                                        : ""
                                    }
                                    title={text(...statusWords[bucket.status])}
                                  >
                                    <b>{bucket.id.toUpperCase()}</b>
                                    {seconds(bucket.p95)}
                                  </span>
                                ))}
                              </div>
                            ) : (
                              <span className="sq-muted">—</span>
                            )}
                          </td>
                          <td className="sq-numeric">
                            {number(unit.requests)}
                          </td>
                          <td>
                            <StatusPill status={unit.status} text={text} />
                          </td>
                          <td>
                            <button
                              className="sq-row-view"
                              aria-label={`${text(...translatedMessage("view_baad6e0094"))} ${unitLabel(unit)}`}
                              data-unit-id={unit.id}
                              onClick={() => openUnit(unit)}
                            >
                              {text(
                                ...translatedMessage("view_details_8c9210e282"),
                              )}
                              <ArrowRight size={14} />
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </ResponsiveTable>
                </div>
                <div className="sq-mobile-units">
                  {visibleUnits.map((unit) => (
                    <article className="sq-mobile-unit" key={unit.id}>
                      <div>
                        <strong>{modelLabel(unit.model)}</strong>
                        <StatusPill status={unit.status} text={text} />
                      </div>
                      <p>
                        {groupLabel(unit.group)} · {requestLabel(unit)}
                      </p>
                      <small>
                        #{unit.userId} · {unit.version}
                      </small>
                      <dl>
                        <div>
                          <dt>
                            {text(
                              ...translatedMessage(
                                "final_success_rate_beac9517b2",
                              ),
                            )}
                          </dt>
                          <dd>{percent(unit.successRate)}</dd>
                        </div>
                        <div>
                          <dt>
                            {text(...translatedMessage("samples_03a4a40574"))}
                          </dt>
                          <dd>{number(unit.requests)}</dd>
                        </div>
                      </dl>
                      <Button
                        size="small"
                        data-unit-id={unit.id}
                        onClick={() => openUnit(unit)}
                      >
                        {text(...translatedMessage("view_details_8c9210e282"))}
                        <ArrowRight size={14} />
                      </Button>
                    </article>
                  ))}
                </div>
                <div className="sq-pagination">
                  <span>
                    {text(
                      ...translatedMessage("units_ef2a7b9ca7", {
                        p0: units.length,
                      }),
                    )}
                  </span>
                  <label>
                    {text(...translatedMessage("per_page_339ed6f784"))}
                    <select
                      aria-label={text(
                        ...translatedMessage("per_page_339ed6f784"),
                      )}
                      value={pageSize}
                      onChange={(event) => {
                        change({ size: event.target.value, page: null });
                      }}
                    >
                      <option value={5}>5</option>
                      <option value={10}>10</option>
                    </select>
                  </label>
                  <div>
                    <button
                      disabled={currentPage === 1}
                      onClick={() => change({ page: String(currentPage - 1) })}
                      aria-label={text(
                        ...translatedMessage("previous_page_69cac17b46"),
                      )}
                    >
                      <ChevronLeft size={15} />
                    </button>
                    <span>
                      {currentPage} /{" "}
                      {Math.max(1, Math.ceil(units.length / pageSize))}
                    </span>
                    <button
                      disabled={currentPage * pageSize >= units.length}
                      onClick={() => change({ page: String(currentPage + 1) })}
                      aria-label={text(
                        ...translatedMessage("next_page_8a29f0d0d2"),
                      )}
                    >
                      <ChevronRight size={15} />
                    </button>
                  </div>
                </div>
              </>
            )}
          </section>
        </>
      )}
      {selectedId && !selected && (
        <section className="sq-card sq-empty" role="status">
          <CircleHelp size={32} />
          <h2>
            {text(
              ...translatedMessage("accounting_unit_unavailable_7e1b66df85"),
            )}
          </h2>
          <p>
            {text(
              ...translatedMessage(
                "the_period_or_unit_is_invalid_or_outside_this_09f81e6034",
              ),
            )}
          </p>
          <Button
            onClick={() => {
              setParams({});
              setExportMessage("");
            }}
          >
            {text(...translatedMessage("reset_and_return_2ea4d4f41c"))}
          </Button>
        </section>
      )}
      {selected && (
        <>
          <section
            className="sq-detail-identity"
            aria-label={text(
              ...translatedMessage("current_accounting_unit_4c0688ccd2"),
            )}
          >
            <div className="sq-detail-identity-main">
              <span>
                <Layers3 size={20} />
              </span>
              <div>
                <h2>{modelLabel(selected.model)}</h2>
                <code>{selected.model}</code>
              </div>
              <StatusPill status={selected.status} text={text} />
            </div>
            <dl>
              {[
                [
                  text(...translatedMessage("customer_b6fee18c46")),
                  `#${selected.userId}`,
                ],
                [
                  text(...translatedMessage("service_group_1c6f8adf41")),
                  groupLabel(selected.group),
                ],
                [
                  text(...translatedMessage("request_type_881c7ae173")),
                  requestLabel(selected),
                ],
                [
                  text(...translatedMessage("accounting_period_0429b35b74")),
                  month,
                ],
                [
                  text(...translatedMessage("contract_version_8271dccca7")),
                  selected.version,
                ],
              ].map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
            </dl>
            <p>
              {text(
                ...translatedMessage("this_unit_s_demo_data_as_of_e5a2ceacc9"),
              )}{" "}
              <b>{periodEnd}</b> ·{" "}
              {text(...translatedMessage("not_finally_reconciled_9d3aeeeac9"))}
            </p>
          </section>
          <div className="sq-unit-summary">
            <section className="sq-card">
              <span>
                {text(...translatedMessage("final_success_rate_beac9517b2"))}
              </span>
              <strong>{percent(selected.successRate)}</strong>
              <small>
                {selected.successTarget
                  ? `${text(...translatedMessage("contract_target_43987ff4ae"))} ≥ ${percent(selected.successTarget)}`
                  : text(...translatedMessage("no_contract_target_dad1dc3d11"))}
              </small>
            </section>
            <section className="sq-card">
              <span>
                {text(...translatedMessage("logical_requests_175a71490c"))}
              </span>
              <strong>{number(selected.requests)}</strong>
              <small>
                {text(...translatedMessage("retries_counted_once_41737dd2da"))}
              </small>
            </section>
            <section className="sq-card">
              <span>
                {text(...translatedMessage("successful_deliveries_85d8e6c7c7"))}
              </span>
              <strong>{number(selected.successes)}</strong>
              <small>
                {text(...translatedMessage("this_unit_only_277a88f71b"))}
              </small>
            </section>
          </div>
          <section className="sq-card sq-bucket-section">
            <div className="sq-card-heading">
              <div>
                <h2>
                  {text(
                    ...translatedMessage(
                      "first_token_latency_by_bucket_8ccb96d93d",
                    ),
                  )}
                </h2>
                <p>
                  {text(
                    ...translatedMessage(
                      "three_input_length_buckets_each_with_a_calend_aba008e17d",
                    ),
                  )}
                </p>
              </div>
            </div>
            {selected.stream ? (
              <div className="sq-bucket-grid">
                {selected.buckets.map((bucket, index) => (
                  <article key={bucket.id}>
                    <div className="sq-bucket-top">
                      <span className="sq-bucket-id">
                        {bucket.id.toUpperCase()}{" "}
                        <small>
                          {
                            [
                              text(...translatedMessage("short_c6d2f76fc1")),
                              text(...translatedMessage("medium_8a4f8f4071")),
                              text(...translatedMessage("long_5238b324a7")),
                            ][index]
                          }
                        </small>
                      </span>
                      <StatusPill status={bucket.status} text={text} />
                    </div>
                    <p className="sq-bucket-range">
                      {["≤ 8,192", "8,193 – 32,768", "> 32,768"][index]}{" "}
                      {text(...translatedMessage("input_tokens_f6e9c8023d"))}
                    </p>
                    <div className="sq-bucket-values">
                      <div>
                        <span>
                          {text(...translatedMessage("current_p95_5a853e1bd1"))}
                        </span>
                        <strong>{seconds(bucket.p95)}</strong>
                      </div>
                      <div>
                        <span>
                          {text(
                            ...translatedMessage("contract_limit_9fc9c2cdec"),
                          )}
                        </span>
                        <b>
                          {selected.successTarget
                            ? `≤ ${seconds(bucket.limit)}`
                            : "—"}
                        </b>
                      </div>
                    </div>
                    <div className="sq-bucket-sample">
                      <span>
                        {text(...translatedMessage("valid_samples_acdba769e3"))}{" "}
                        <b>{number(bucket.samples)}</b>
                      </span>
                      <span>
                        {selected.successTarget
                          ? `${Math.round((bucket.p95 / bucket.limit) * 100)}% ${text(...translatedMessage("of_limit_63350724dc"))}`
                          : text(
                              ...translatedMessage(
                                "observation_only_ef0b1bf5e8",
                              ),
                            )}
                      </span>
                    </div>
                    <div className="sq-progress" aria-hidden="true">
                      <span
                        className={
                          bucket.status === "below"
                            ? "is-below"
                            : bucket.status === "insufficient"
                              ? "is-warning"
                              : ""
                        }
                        style={{
                          width: `${Math.min(100, (bucket.p95 / bucket.limit) * 100)}%`,
                        }}
                      />
                    </div>
                  </article>
                ))}
              </div>
            ) : (
              <div className="sq-nonstream">
                <Clock3 size={26} />
                <div>
                  <h3>
                    {text(
                      ...translatedMessage(
                        "first_token_latency_does_not_apply_960445f28b",
                      ),
                    )}
                  </h3>
                  <p>
                    {text(
                      ...translatedMessage(
                        "this_non_streaming_unit_reports_final_success_77fa16b458",
                      ),
                    )}
                  </p>
                </div>
              </div>
            )}
            <div className="sq-section-note">
              <Info size={14} />
              <span>
                {selected.stream
                  ? text(
                      ...translatedMessage(
                        "measured_from_request_start_to_generated_cont_c30e38b8e0",
                        { p0: selected.minSamples, p1: selected.unknownInputs },
                      ),
                    )
                  : text(
                      ...translatedMessage(
                        "known_failures_without_generated_content_coun_e22db5ff1c",
                      ),
                    )}
              </span>
            </div>
          </section>
          <div className="sq-analysis-grid">
            <section className="sq-card">
              <div className="sq-card-heading">
                <div>
                  <h2>
                    {text(
                      ...translatedMessage("daily_operating_trend_ccdcef2497"),
                    )}
                  </h2>
                  <p>
                    {text(
                      ...translatedMessage(
                        "an_early_signal_monthly_conclusions_use_this__d913c28810",
                      ),
                    )}
                  </p>
                </div>
              </div>
              <div
                className="sq-trend-tabs"
                role="group"
                aria-label={text(
                  ...translatedMessage("trend_metric_fed8d6b3e6"),
                )}
              >
                {(["success", "g0", "g1", "g2"] as QualityMetric[]).map(
                  (value) => (
                    <button
                      key={value}
                      className={chartMetric === value ? "is-active" : ""}
                      aria-pressed={chartMetric === value}
                      disabled={value !== "success" && !selected.stream}
                      onClick={() => change({ metric: value }, true)}
                    >
                      {value === "success"
                        ? text(...translatedMessage("success_rate_b8e8689749"))
                        : value.toUpperCase()}
                    </button>
                  ),
                )}
              </div>
              <TrendChart
                rows={trend}
                metric={chartMetric}
                target={target}
                text={text}
              />
            </section>
            <section className="sq-card sq-result-card">
              <div className="sq-card-heading">
                <div>
                  <h2>
                    {text(
                      ...translatedMessage(
                        "current_accounting_result_2392524d74",
                      ),
                    )}
                  </h2>
                  <p>
                    {selected.version} ·{" "}
                    {text(...translatedMessage("demo_contract_a34b9ebb8b"))}
                  </p>
                </div>
                <ShieldCheck size={21} />
              </div>
              <div className="sq-result-body">
                <div className="sq-result-row">
                  <span className="sq-result-icon">
                    <ShieldCheck size={19} />
                  </span>
                  <div>
                    <h3>
                      {text(
                        ...translatedMessage("successful_delivery_d7aae6863c"),
                      )}
                    </h3>
                    <StatusPill status={selected.successStatus} text={text} />
                  </div>
                </div>
                <div className="sq-result-row">
                  <span className="sq-result-icon is-purple">
                    <Clock3 size={19} />
                  </span>
                  <div>
                    <h3>
                      {text(
                        ...translatedMessage(
                          "first_token_performance_01d68cbd34",
                        ),
                      )}
                    </h3>
                    {selected.performanceStatus ? (
                      <StatusPill
                        status={selected.performanceStatus}
                        text={text}
                      />
                    ) : (
                      <span className="sq-muted">
                        {text(
                          ...translatedMessage(
                            "not_applicable_to_non_streaming_98aaf87bf3",
                          ),
                        )}
                      </span>
                    )}
                  </div>
                </div>
                <div className="sq-result-metrics">
                  <div>
                    <span>
                      {text(
                        ...translatedMessage("error_budget_left_434d0b013e"),
                      )}
                    </span>
                    <strong>
                      {selected.successTarget > 0 &&
                      selected.successTarget < 100
                        ? percent(
                            Math.max(
                              0,
                              (1 -
                                (100 - selected.successRate) /
                                  (100 - selected.successTarget)) *
                                100,
                            ),
                          )
                        : "—"}
                    </strong>
                  </div>
                  <div>
                    <span>
                      {text(
                        ...translatedMessage("data_completeness_5b07bf51f0"),
                      )}
                    </span>
                    <strong>
                      {selected.completeness === null
                        ? "—"
                        : percent(selected.completeness)}
                    </strong>
                    <small>
                      {selected.completeness === null
                        ? text(
                            ...translatedMessage(
                              "evidence_needs_reconciliation_0716f4b1f8",
                            ),
                          )
                        : text(
                            ...translatedMessage(
                              "demo_evidence_correlated_0ead02c172",
                            ),
                          )}
                    </small>
                  </div>
                </div>
                <p className="sq-result-note">
                  {text(
                    ...translatedMessage(
                      "a_result_below_target_is_not_a_confirmed_brea_b9632ad710",
                    ),
                  )}
                </p>
              </div>
            </section>
          </div>
        </>
      )}
      <Modal
        size="wide"
        open={guideOpen}
        onOpenChange={setGuideOpen}
        title={text(...translatedMessage("field_and_status_guide_ecd6df75c6"))}
        description={text(
          ...translatedMessage(
            "understand_accounting_scope_metrics_and_contr_8d1f48e750",
          ),
        )}
      >
        <div className="sq-guide">
          <div className="sq-guide-intro">
            <CircleHelp size={18} />
            {text(
              ...translatedMessage(
                "this_is_a_frontend_demo_all_contracts_thresho_7ff32d65b9",
              ),
            )}
          </div>
          <h3>
            {text(...translatedMessage("accounting_definitions_112a433242"))}
          </h3>
          {[
            [
              text(...translatedMessage("accounting_unit_68dc8a2e92")),
              text(
                ...translatedMessage(
                  "customer_group_model_request_type_contract_ve_f7f2fd69eb",
                ),
              ),
            ],
            [
              text(...translatedMessage("final_success_rate_a798c0798b")),
              text(
                ...translatedMessage(
                  "share_of_countable_logical_requests_delivered_c77070c178",
                ),
              ),
            ],
            [
              text(...translatedMessage("ttft_and_p95_03be46a1ab")),
              text(
                ...translatedMessage(
                  "time_from_request_start_to_generated_content__e7bef158c9",
                ),
              ),
            ],
            [
              text(...translatedMessage("key_diagnostics_d31a279d1c")),
              text(
                ...translatedMessage(
                  "keys_help_diagnose_differences_within_a_unit__6288d652f0",
                ),
              ),
            ],
          ].map(([title, content]) => (
            <section key={title}>
              <h4>{title}</h4>
              <p>{content}</p>
            </section>
          ))}
          <h3>{text(...translatedMessage("overall_statuses_1f1bc78703"))}</h3>
          {(
            [
              [
                "pass",
                text(
                  ...translatedMessage(
                    "success_rate_and_each_valid_bucket_meet_their_e64ed3f4a2",
                  ),
                ),
              ],
              [
                "near",
                text(
                  ...translatedMessage(
                    "close_to_a_threshold_warning_tolerance_does_n_c33a2c0508",
                  ),
                ),
              ],
              [
                "below",
                text(
                  ...translatedMessage(
                    "success_is_below_target_or_a_bucket_exceeds_i_b1fd76c1e3",
                  ),
                ),
              ],
              [
                "insufficient",
                text(
                  ...translatedMessage(
                    "at_least_one_bucket_lacks_the_minimum_samples_2edbd247f5",
                  ),
                ),
              ],
              [
                "incomplete",
                text(
                  ...translatedMessage(
                    "request_final_state_and_performance_evidence__7fe506cabd",
                  ),
                ),
              ],
              [
                "unconfigured",
                text(
                  ...translatedMessage(
                    "no_success_rate_target_is_configured_data_is__928fb95108",
                  ),
                ),
              ],
            ] as [QualityStatus, string][]
          ).map(([status, description]) => (
            <section key={status}>
              <StatusPill status={status} text={text} />
              <p>{description}</p>
            </section>
          ))}
          {selected && (
            <>
              <h3>
                {text(...translatedMessage("current_demo_contract_e5d09db873"))}
              </h3>
              <section>
                <h4>{selected.version}</h4>
                <p>
                  {text(...translatedMessage("success_target_363d8b54c8"))}:{" "}
                  {selected.successTarget
                    ? `≥ ${percent(selected.successTarget)}`
                    : "—"}
                </p>
                {selected.buckets.map((bucket) => (
                  <p key={bucket.id}>
                    {bucket.id.toUpperCase()} ·{" "}
                    {selected.successTarget
                      ? `≤ ${seconds(bucket.limit)}`
                      : "—"}
                  </p>
                ))}
                <p>
                  {text(
                    ...translatedMessage(
                      "minimum_samples_per_bucket_2246d2bed6",
                    ),
                  )}
                  : {selected.minSamples}
                </p>
              </section>
            </>
          )}
        </div>
      </Modal>
    </div>
  );
}
