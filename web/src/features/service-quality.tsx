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
import { DemoControlBar } from "../components/demo-control-bar";
import { usePreferences } from "../lib/preferences";
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
import "../components/service-quality.css";
import { catalogModels } from "../config/models";
import { placeholders } from "../config/placeholders";

type Copy = (zh: string, en: string, ar: string) => string;
const statusWords: Record<QualityStatus, [string, string, string]> = {
  pass: ["达到目标", "Meets target", "يحقق الهدف"],
  near: ["接近阈值", "Near threshold", "قريب من الحد"],
  below: ["当前低于目标", "Currently below target", "أقل من الهدف حاليًا"],
  insufficient: ["样本不足", "Insufficient samples", "عينات غير كافية"],
  incomplete: ["数据待核验", "Evidence unverified", "البيانات غير متحققة"],
  unconfigured: ["未配置合同目标", "No contract target", "لا يوجد هدف تعاقدي"],
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
        <span>{text("日度观测值", "Daily observation", "رصد يومي")}</span>
      </div>
      <svg
        viewBox="0 0 672 221"
        role="img"
        aria-label={text(
          "每日运行趋势，虚线为合同目标",
          "Daily operating trend; dashed line marks the contract target",
          "الاتجاه اليومي؛ الخط المتقطع هو الهدف التعاقدي",
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
              {text("目标", "Target", "الهدف")} {format(target)}
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
          {text("实际观测", "Observed", "القيمة المرصودة")}
        </span>
        {target !== null && (
          <span>
            <i className="is-target" />
            {text("合同目标", "Contract target", "الهدف التعاقدي")}
          </span>
        )}
        <small>
          {text(
            "纵轴根据数据自动缩放",
            "Vertical axis auto-scales",
            "مقياس رأسي تلقائي",
          )}
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
  const allUnits = useMemo(
    () => (months.includes(month) ? createQualityUnits(month) : []),
    [month, months],
  );
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
  const trend = useMemo(
    () => (selected ? createQualityTrend(selected, month) : []),
    [selected, month],
  );
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
      ? text("流式", "Streaming", "متدفق")
      : text("非流式", "Non-streaming", "غير متدفق");
  const groupLabel = (value: string) => {
    const tier = placeholders.tiers.find((item) => item.id === value);
    return tier?.name[language] ?? value;
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
      text("账期", "Period", "الفترة"),
      text("统计起始日", "Period start", "بداية الفترة"),
      text("数据截至日", "Data as of", "البيانات حتى"),
      text("核验状态", "Reconciliation status", "حالة المطابقة"),
      text("用户编号", "User ID", "معرّف المستخدم"),
      text("分组", "Group", "المجموعة"),
      text("标准模型", "Standard model", "النموذج"),
      text("请求类型", "Request type", "نوع الطلب"),
      text("合同版本", "Contract version", "نسخة العقد"),
      text("最终成功率", "Final success rate", "معدل النجاح"),
      text("成功率目标", "Success target", "هدف النجاح"),
      text("逻辑请求数", "Logical requests", "الطلبات المنطقية"),
      text(
        "G0 第95百分位（毫秒）",
        "G0 P95 (ms)",
        "G0 الشريحة 95 (مللي ثانية)",
      ),
      text(
        "G1 第95百分位（毫秒）",
        "G1 P95 (ms)",
        "G1 الشريحة 95 (مللي ثانية)",
      ),
      text(
        "G2 第95百分位（毫秒）",
        "G2 P95 (ms)",
        "G2 الشريحة 95 (مللي ثانية)",
      ),
      text("总体状态", "Overall status", "الحالة العامة"),
      text("数据来源", "Data source", "مصدر البيانات"),
    ];
    const reportUnits = selectedId ? (selected ? [selected] : []) : units;
    const rows = reportUnits.map((unit) => [
      month,
      `${month}-01`,
      periodEnd,
      text(
        "演示数据 / 未最终核验",
        "Demo data / not finally reconciled",
        "بيانات تجريبية / لم تتم المطابقة النهائية",
      ),
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
        "前端演示数据，非真实交付记录",
        "Frontend demo, not production delivery records",
        "بيانات تجريبية وليست سجلات إنتاج",
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
        `已导出 ${reportUnits.length} 个核算单元。`,
        `Exported ${reportUnits.length} accounting units.`,
        `تم تصدير ${reportUnits.length} وحدة محاسبية.`,
      ),
    );
  };
  const summaries = [
    {
      label: text("合同核算单元", "Accounting units", "الوحدات المحاسبية"),
      value: number(summary.total),
      description: text(
        "按合同范围独立核算",
        "Calculated by contract scope",
        "محسوبة حسب نطاق العقد",
      ),
      Icon: Layers3,
      tone: "blue",
    },
    {
      label: text(
        "成功率待关注",
        "Success needs attention",
        "النجاح بحاجة للمراجعة",
      ),
      value: number(summary.successAttention),
      description: text(
        "接近阈值、低于目标或证据不足",
        "Near target, below target, or unverified",
        "قريب من الحد أو دونه أو غير متحقق",
      ),
      Icon: ShieldCheck,
      tone: "amber",
    },
    {
      label: text(
        "性能待关注",
        "Performance needs attention",
        "الأداء بحاجة للمراجعة",
      ),
      value: number(summary.performanceAttention),
      description: text(
        "任一词元桶需关注或样本不足",
        "Buckets requiring review or more samples",
        "حاويات تحتاج مراجعة أو عينات إضافية",
      ),
      Icon: Clock3,
      tone: "purple",
    },
    {
      label: text("本期逻辑请求", "Logical requests", "الطلبات المنطقية"),
      value: number(summary.logicalRequests),
      description: text(
        "按最终状态计数，重试不重复计入",
        "Final requests; retries are not duplicated",
        "الطلبات النهائية دون تكرار المحاولات",
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
          aria-label={text("详情导航", "Detail navigation", "تنقل التفاصيل")}
        >
          <Button variant="ghost" size="small" onClick={returnToList}>
            <ArrowLeft size={15} />
            {text(
              "返回交付列表",
              "Back to delivery list",
              "العودة لقائمة التسليم",
            )}
          </Button>
          <span>/</span>
          <span>
            {selected
              ? `${modelLabel(selected.model)} · ${groupLabel(selected.group)} · ${requestLabel(selected)} · ${month}`
              : text(
                  "单元交付详情",
                  "Unit delivery details",
                  "تفاصيل تسليم الوحدة",
                )}
          </span>
        </nav>
      )}
      <header className="sq-heading">
        <div>
          <span className="sq-eyebrow">
            <ShieldCheck size={14} />
            {text("服务观测", "SERVICE OBSERVABILITY", "مراقبة الخدمة")}
          </span>
          <h1 ref={detailHeading} tabIndex={-1}>
            {selectedId
              ? text(
                  "单元交付详情",
                  "Unit delivery details",
                  "تفاصيل تسليم الوحدة",
                )
              : text("服务质量", "Service quality", "جودة الخدمة")}
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
            {text("指标说明", "Metric guide", "دليل المقاييس")}
          </Button>
          <Button
            variant="primary"
            size="small"
            onClick={exportReport}
            disabled={selectedId ? !selected : !units.length}
          >
            <ArrowDownToLine size={15} />
            {selectedId
              ? text("导出本单元", "Export this unit", "تصدير هذه الوحدة")
              : text(
                  "下载月报",
                  "Export monthly report",
                  "تصدير التقرير الشهري",
                )}
          </Button>
        </div>
      </header>
      <div className="sq-demo-banner">
        <span>
          <Info size={14} />
          <strong>{text("前端演示", "Frontend demo", "عرض تجريبي")}</strong>
          {text(
            "指标、合同及目标均为示例，不代表真实服务表现或最终结算。",
            "Metrics, contracts, and targets are illustrative, not production results or final settlement.",
            "المقاييس والعقود والأهداف أمثلة وليست نتائج فعلية أو تسوية نهائية.",
          )}
        </span>
        {!selectedId && (
          <label>
            {text("演示视角", "Demo view", "وضع العرض")}
            <select
              aria-label={text("演示视角", "Demo view", "وضع العرض")}
              value={view}
              onChange={changeFilter("view")}
            >
              <option value="enterprise">
                {text("我的账号", "My account", "حسابي")}
              </option>
              <option value="admin">
                {text("平台视角", "Platform view", "عرض المنصة")}
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
              "服务质量筛选",
              "Service quality filters",
              "تصفية جودة الخدمة",
            )}
          >
            <div className="sq-card-heading">
              <div>
                <h2>
                  <Filter size={15} />
                  {text(
                    "合同交付总览",
                    "Contract delivery overview",
                    "نظرة عامة على التسليم",
                  )}
                </h2>
                <p>
                  {text(
                    "按客户、分组、模型、请求类型和自然月分别核算。",
                    "Account separately by customer, group, model, request type, and calendar month.",
                    "حساب مستقل حسب العميل والمجموعة والنموذج ونوع الطلب والشهر.",
                  )}
                </p>
              </div>
              <div className="sq-as-of">
                <span>
                  {text(
                    "演示数据截至",
                    "Demo data as of",
                    "البيانات التجريبية حتى",
                  )}{" "}
                  <b>{periodEnd}</b>
                </span>
                <small>
                  {text(
                    "演示结果尚未最终核验",
                    "Demo results have not been finally reconciled",
                    "لم تُطابق النتائج التجريبية نهائيًا",
                  )}
                </small>
              </div>
            </div>
            <div className="sq-filters">
              <label>
                <span>{text("用户编号", "User ID", "معرّف المستخدم")}</span>
                <input
                  type="text"
                  inputMode="numeric"
                  aria-label={text("用户编号", "User ID", "معرّف المستخدم")}
                  value={view === "enterprise" ? userId : customer}
                  disabled={view === "enterprise"}
                  placeholder={text(
                    "所有演示客户",
                    "All demo customers",
                    "كل العملاء التجريبيين",
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
                  {text("核算账期", "Accounting period", "الفترة المحاسبية")}
                </span>
                <select
                  aria-label={text(
                    "核算账期",
                    "Accounting period",
                    "الفترة المحاسبية",
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
                <span>{text("分组", "Group", "المجموعة")}</span>
                <select
                  aria-label={text("分组", "Group", "المجموعة")}
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
                    {text("全部分组", "All groups", "كل المجموعات")}
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
                  {text("标准模型", "Standard model", "النموذج القياسي")}
                </span>
                <select
                  aria-label={text(
                    "标准模型",
                    "Standard model",
                    "النموذج القياسي",
                  )}
                  value={model}
                  onChange={changeFilter("model")}
                >
                  <option value="all">
                    {text("全部模型", "All models", "كل النماذج")}
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
                <span>{text("请求类型", "Request type", "نوع الطلب")}</span>
                <select
                  aria-label={text("请求类型", "Request type", "نوع الطلب")}
                  value={requestType}
                  onChange={changeFilter("type")}
                >
                  <option value="all">
                    {text("全部类型", "All types", "كل الأنواع")}
                  </option>
                  <option value="streaming">
                    {text("流式", "Streaming", "متدفق")}
                  </option>
                  <option value="non-streaming">
                    {text("非流式", "Non-streaming", "غير متدفق")}
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
                    "合同核算单元",
                    "Contract accounting units",
                    "وحدات المحاسبة التعاقدية",
                  )}
                </h2>
                <p>
                  {text(
                    "每个核算单元独立判断；不同单元、不同词元桶的结果不能互相抵消。",
                    "Each accounting unit stands alone; results cannot offset across units or buckets.",
                    "تُقيّم كل وحدة بشكل مستقل ولا تتعادل نتائج الوحدات والحاويات.",
                  )}
                </p>
              </div>
              <span className="sq-count">
                {units.length} {text("个核算单元", "units", "وحدة")}
              </span>
            </div>
            {!units.length ? (
              <div className="sq-empty">
                <Layers3 size={34} />
                <h3>
                  {text(
                    "当前筛选下没有核算单元",
                    "No matching accounting units",
                    "لا توجد وحدات مطابقة",
                  )}
                </h3>
                <p>
                  {text(
                    "试试调整客户、分组或模型，重新查看这个账期。",
                    "Try another customer, group, or model for this period.",
                    "جرّب عميلًا أو مجموعة أو نموذجًا آخر لهذه الفترة.",
                  )}
                </p>
                <Button onClick={resetFilters}>
                  {text("重置筛选", "Reset filters", "إعادة ضبط التصفية")}
                </Button>
              </div>
            ) : (
              <>
                <div className="sq-table-scroll">
                  <table>
                    <thead>
                      <tr>
                        {[
                          text(
                            "分组 / 服务等级",
                            "Group / Service tier",
                            "المجموعة / المستوى",
                          ),
                          text("标准模型", "Standard model", "النموذج"),
                          text("请求类型", "Request type", "نوع الطلب"),
                          text(
                            "最终成功率",
                            "Final success rate",
                            "معدل النجاح",
                          ),
                          text(
                            "首词时延 · 三档分桶",
                            "First-token time · 3 buckets",
                            "زمن أول رمز · 3 حاويات",
                          ),
                          text("样本数", "Samples", "العينات"),
                          text("总体状态", "Overall status", "الحالة"),
                          text("操作", "Action", "الإجراء"),
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
                                ? `${text("目标", "Target", "الهدف")} ≥ ${percent(unit.successTarget)}`
                                : text("未配置", "Not configured", "غير مكوّن")}
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
                              aria-label={`${text("查看", "View", "عرض")} ${unitLabel(unit)}`}
                              data-unit-id={unit.id}
                              onClick={() => openUnit(unit)}
                            >
                              {text("查看详情", "View details", "عرض التفاصيل")}
                              <ArrowRight size={14} />
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
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
                              "最终成功率",
                              "Final success rate",
                              "معدل النجاح",
                            )}
                          </dt>
                          <dd>{percent(unit.successRate)}</dd>
                        </div>
                        <div>
                          <dt>{text("样本数", "Samples", "العينات")}</dt>
                          <dd>{number(unit.requests)}</dd>
                        </div>
                      </dl>
                      <Button
                        size="small"
                        data-unit-id={unit.id}
                        onClick={() => openUnit(unit)}
                      >
                        {text("查看详情", "View details", "عرض التفاصيل")}
                        <ArrowRight size={14} />
                      </Button>
                    </article>
                  ))}
                </div>
                <div className="sq-pagination">
                  <span>
                    {text(
                      `共 ${units.length} 条`,
                      `${units.length} units`,
                      `${units.length} وحدة`,
                    )}
                  </span>
                  <label>
                    {text("每页", "Per page", "لكل صفحة")}
                    <select
                      aria-label={text("每页", "Per page", "لكل صفحة")}
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
                        "上一页",
                        "Previous page",
                        "الصفحة السابقة",
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
                      aria-label={text("下一页", "Next page", "الصفحة التالية")}
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
              "无法找到该核算单元",
              "Accounting unit unavailable",
              "الوحدة غير متاحة",
            )}
          </h2>
          <p>
            {text(
              "账期或单元标识无效，或不在当前客户可见范围内。请返回列表重新选择。",
              "The period or unit is invalid, or outside this customer's scope. Return to the list to choose a unit.",
              "الفترة أو الوحدة غير صالحة أو خارج نطاق العميل. عد إلى القائمة للاختيار.",
            )}
          </p>
          <Button
            onClick={() => {
              setParams({});
              setExportMessage("");
            }}
          >
            {text("重置并返回列表", "Reset and return", "إعادة الضبط والعودة")}
          </Button>
        </section>
      )}
      {selected && (
        <>
          <section
            className="sq-detail-identity"
            aria-label={text(
              "当前核算单元",
              "Current accounting unit",
              "الوحدة الحالية",
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
                [text("客户", "Customer", "العميل"), `#${selected.userId}`],
                [
                  text("服务分组", "Service group", "مجموعة الخدمة"),
                  groupLabel(selected.group),
                ],
                [
                  text("请求类型", "Request type", "نوع الطلب"),
                  requestLabel(selected),
                ],
                [text("核算账期", "Accounting period", "الفترة"), month],
                [
                  text("合同版本", "Contract version", "نسخة العقد"),
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
                "本单元演示数据截至",
                "This unit's demo data as of",
                "بيانات الوحدة التجريبية حتى",
              )}{" "}
              <b>{periodEnd}</b> ·{" "}
              {text(
                "未最终核验",
                "Not finally reconciled",
                "لم تتم المطابقة النهائية",
              )}
            </p>
          </section>
          <div className="sq-unit-summary">
            <section className="sq-card">
              <span>
                {text("最终成功率", "Final success rate", "معدل النجاح")}
              </span>
              <strong>{percent(selected.successRate)}</strong>
              <small>
                {selected.successTarget
                  ? `${text("合同目标", "Contract target", "الهدف")} ≥ ${percent(selected.successTarget)}`
                  : text(
                      "未配置合同目标",
                      "No contract target",
                      "لا يوجد هدف تعاقدي",
                    )}
              </small>
            </section>
            <section className="sq-card">
              <span>
                {text("逻辑请求数", "Logical requests", "الطلبات المنطقية")}
              </span>
              <strong>{number(selected.requests)}</strong>
              <small>
                {text(
                  "重试不重复计数",
                  "Retries counted once",
                  "لا تكرر المحاولات العد",
                )}
              </small>
            </section>
            <section className="sq-card">
              <span>
                {text(
                  "成功交付请求",
                  "Successful deliveries",
                  "الطلبات الناجحة",
                )}
              </span>
              <strong>{number(selected.successes)}</strong>
              <small>
                {text("仅统计当前单元", "This unit only", "هذه الوحدة فقط")}
              </small>
            </section>
          </div>
          <section className="sq-card sq-bucket-section">
            <div className="sq-card-heading">
              <div>
                <h2>
                  {text(
                    "首词时延分桶表现",
                    "First-token latency by bucket",
                    "زمن أول رمز حسب الحاوية",
                  )}
                </h2>
                <p>
                  {text(
                    "按实际输入长度分为三档，分别计算自然月第95百分位时延。",
                    "Three input-length buckets, each with a calendar-month 95th-percentile latency.",
                    "ثلاث حاويات لطول الإدخال، لكل منها زمن الشريحة المئوية 95 الشهري.",
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
                              text("短输入", "Short", "قصير"),
                              text("中输入", "Medium", "متوسط"),
                              text("长输入", "Long", "طويل"),
                            ][index]
                          }
                        </small>
                      </span>
                      <StatusPill status={bucket.status} text={text} />
                    </div>
                    <p className="sq-bucket-range">
                      {["≤ 8,192", "8,193 – 32,768", "> 32,768"][index]}{" "}
                      {text("输入词元", "input tokens", "رمز إدخال")}
                    </p>
                    <div className="sq-bucket-values">
                      <div>
                        <span>
                          {text(
                            "当前第95百分位",
                            "Current P95",
                            "الشريحة المئوية 95",
                          )}
                        </span>
                        <strong>{seconds(bucket.p95)}</strong>
                      </div>
                      <div>
                        <span>
                          {text("合同上限", "Contract limit", "الحد التعاقدي")}
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
                        {text("有效样本", "Valid samples", "العينات الصالحة")}{" "}
                        <b>{number(bucket.samples)}</b>
                      </span>
                      <span>
                        {selected.successTarget
                          ? `${Math.round((bucket.p95 / bucket.limit) * 100)}% ${text("上限占比", "of limit", "من الحد")}`
                          : text(
                              "观测模式",
                              "Observation only",
                              "للمراقبة فقط",
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
                      "非流式请求不计算首词时延",
                      "First-token latency does not apply",
                      "زمن أول رمز لا ينطبق",
                    )}
                  </h3>
                  <p>
                    {text(
                      "当前单元仅展示最终成功率，避免将非流式响应与流式指标混合。",
                      "This non-streaming unit reports final success rate only.",
                      "تعرض هذه الوحدة غير المتدفقة معدل النجاح النهائي فقط.",
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
                      `从请求开始计时，到网关收到生成内容。每档至少需 ${selected.minSamples} 个样本；${selected.unknownInputs} 个输入长度未知的请求不进入分桶。`,
                      `Measured from request start to generated content reaching the gateway. Each bucket needs ${selected.minSamples} samples; ${selected.unknownInputs} requests with unknown input length are excluded.`,
                      `من بدء الطلب حتى وصول المحتوى إلى البوابة. تحتاج كل حاوية إلى ${selected.minSamples} عينة؛ استُبعد ${selected.unknownInputs} طلبًا بطول مجهول.`,
                    )
                  : text(
                      "已知失败且未生成内容的请求计入失败，不生成首词时延样本。",
                      "Known failures without generated content count as failures, not first-token samples.",
                      "الفشل دون محتوى مولّد يُحسب فشلًا وليس عينة لزمن أول رمز.",
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
                      "每日运行趋势",
                      "Daily operating trend",
                      "الاتجاه التشغيلي اليومي",
                    )}
                  </h2>
                  <p>
                    {text(
                      "用于提前发现变化，月度结论仍以同一单元的月度数据为准。",
                      "An early signal; monthly conclusions use this unit’s monthly data.",
                      "إشارة مبكرة؛ تستند النتائج الشهرية إلى بيانات الوحدة الشهرية.",
                    )}
                  </p>
                </div>
              </div>
              <div
                className="sq-trend-tabs"
                role="group"
                aria-label={text("趋势指标", "Trend metric", "مقياس الاتجاه")}
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
                        ? text("成功率", "Success rate", "معدل النجاح")
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
                      "截至当前的核算结果",
                      "Current accounting result",
                      "نتيجة المحاسبة الحالية",
                    )}
                  </h2>
                  <p>
                    {selected.version} ·{" "}
                    {text("演示合同", "Demo contract", "عقد تجريبي")}
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
                        "成功交付合同状态",
                        "Successful delivery",
                        "التسليم الناجح",
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
                        "首词时延性能状态",
                        "First-token performance",
                        "أداء أول رمز",
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
                          "非流式，不适用",
                          "Not applicable to non-streaming",
                          "لا ينطبق على غير المتدفق",
                        )}
                      </span>
                    )}
                  </div>
                </div>
                <div className="sq-result-metrics">
                  <div>
                    <span>
                      {text(
                        "剩余错误预算",
                        "Error budget left",
                        "ميزانية الأخطاء المتبقية",
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
                        "数据完整性",
                        "Data completeness",
                        "اكتمال البيانات",
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
                            "请求证据待核验",
                            "Evidence needs reconciliation",
                            "الأدلة بحاجة للمطابقة",
                          )
                        : text(
                            "演示数据已关联",
                            "Demo evidence correlated",
                            "الأدلة التجريبية مرتبطة",
                          )}
                    </small>
                  </div>
                </div>
                <p className="sq-result-note">
                  {text(
                    "当前低于目标不等同于确认违约。完成月末数据核验后，才可形成正式交付结论。",
                    "A result below target is not a confirmed breach. Formal delivery conclusions require month-end reconciliation.",
                    "النتيجة الأقل من الهدف ليست إخلالًا مؤكدًا؛ تتطلب النتيجة الرسمية مطابقة نهاية الشهر.",
                  )}
                </p>
              </div>
            </section>
          </div>
        </>
      )}
      <div className="demo-inline-footer">
        <DemoControlBar inline />
      </div>
      <Modal
        size="wide"
        open={guideOpen}
        onOpenChange={setGuideOpen}
        title={text(
          "字段与状态说明",
          "Field and status guide",
          "دليل الحقول والحالات",
        )}
        description={text(
          "理解核算范围、指标口径与合同判断边界。",
          "Understand accounting scope, metrics, and contract conclusions.",
          "فهم نطاق المحاسبة والمقاييس وحدود النتائج التعاقدية.",
        )}
      >
        <div className="sq-guide">
          <div className="sq-guide-intro">
            <CircleHelp size={18} />
            {text(
              "本页为前端演示。所有合同版本、阈值及观测数值均是演示配置。",
              "This is a frontend demo. All contracts, thresholds, and observations are illustrative.",
              "هذا عرض تجريبي. العقود والحدود والقيم المرصودة كلها أمثلة.",
            )}
          </div>
          <h3>
            {text("核算口径", "Accounting definitions", "تعريفات المحاسبة")}
          </h3>
          {[
            [
              text("核算单元", "Accounting unit", "الوحدة المحاسبية"),
              text(
                "客户 + 分组 + 标准模型 + 请求类型 + 合同版本 + 自然月。各单元独立计算，不能相互抵消。",
                "Customer + group + model + request type + contract version + calendar month. Units cannot offset one another.",
                "العميل والمجموعة والنموذج ونوع الطلب ونسخة العقد والشهر. تُحسب الوحدات بشكل مستقل.",
              ),
            ],
            [
              text("最终成功率", "Final success rate", "معدل النجاح النهائي"),
              text(
                "成功交付的逻辑请求占比。重试不重复计数；本地校验拒绝、纯客户端取消不进入合同统计。",
                "Share of countable logical requests delivered successfully. Retries are not duplicated; local rejections and pure client cancellations are excluded.",
                "نسبة الطلبات المنطقية الناجحة دون تكرار المحاولات؛ تُستبعد الرفض المحلي وإلغاءات العميل.",
              ),
            ],
            [
              text(
                "首词时延与第95百分位",
                "TTFT and P95",
                "زمن أول رمز والشريحة 95",
              ),
              text(
                "从请求开始到网关收到首个生成内容。同一分桶中 95% 的有效时延不高于此值。仅适用于流式文本请求。",
                "Time from request start to generated content received by the gateway. 95% of valid observations in a bucket do not exceed P95. Streaming text only.",
                "الزمن من بدء الطلب حتى أول محتوى مولّد. لا تتجاوز 95% من القيم هذا الحد؛ للنص المتدفق فقط.",
              ),
            ],
            [
              text("密钥诊断边界", "Key diagnostics", "تشخيص المفاتيح"),
              text(
                "密钥用于定位同一核算单元内的差异；未约定密钥级合同目标时，不据此形成正式交付结论。",
                "Keys help diagnose differences within a unit. Without key-level contractual targets, diagnostics do not establish a delivery conclusion.",
                "تساعد المفاتيح على تشخيص الفروق داخل الوحدة ولا تنشئ نتيجة تعاقدية دون أهداف خاصة بالمفتاح.",
              ),
            ],
          ].map(([title, content]) => (
            <section key={title}>
              <h4>{title}</h4>
              <p>{content}</p>
            </section>
          ))}
          <h3>{text("总体状态", "Overall statuses", "الحالات العامة")}</h3>
          {(
            [
              [
                "pass",
                text(
                  "成功率与各有效分桶均达到合同目标。",
                  "Success rate and each valid bucket meet their contract targets.",
                  "يحقق معدل النجاح وكل حاوية صالحة الهدف التعاقدي.",
                ),
              ],
              [
                "near",
                text(
                  "接近合同阈值，需要关注；告警容差不会降低合同目标。",
                  "Close to a threshold; warning tolerance does not lower the contract target.",
                  "قريب من الحد؛ هامش التحذير لا يخفض الهدف التعاقدي.",
                ),
              ],
              [
                "below",
                text(
                  "成功率低于目标或任一分桶超过上限；这不是确认违约。",
                  "Success is below target or a bucket exceeds its limit; this is not a confirmed breach.",
                  "النجاح دون الهدف أو حاوية فوق الحد؛ ليس إخلالًا مؤكدًا.",
                ),
              ],
              [
                "insufficient",
                text(
                  "至少一档分桶样本数少于演示合同要求，尚不足以判断。",
                  "At least one bucket lacks the minimum samples required by the demo contract.",
                  "حاوية واحدة على الأقل لا تملك الحد الأدنى من العينات.",
                ),
              ],
              [
                "incomplete",
                text(
                  "请求、最终状态与性能事件尚未完成关联核验。",
                  "Request, final-state, and performance evidence has not been reconciled.",
                  "لم تتم مطابقة أدلة الطلب والحالة النهائية والأداء.",
                ),
              ],
              [
                "unconfigured",
                text(
                  "未设置合同成功率目标，数据仅作观测。",
                  "No success-rate target is configured; data is observational only.",
                  "لم يحدد هدف للنجاح؛ البيانات للمراقبة فقط.",
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
                {text(
                  "当前演示合同速查",
                  "Current demo contract",
                  "العقد التجريبي الحالي",
                )}
              </h3>
              <section>
                <h4>{selected.version}</h4>
                <p>
                  {text("成功率目标", "Success target", "هدف النجاح")}:{" "}
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
                    "每档最低样本数",
                    "Minimum samples per bucket",
                    "الحد الأدنى للعينات لكل حاوية",
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
