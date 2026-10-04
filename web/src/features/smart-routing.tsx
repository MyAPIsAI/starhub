import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Area,
  CartesianGrid,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  ComposedChart,
} from "recharts";
import {
  Activity,
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  Check,
  ChevronRight,
  CircleAlert,
  CircleCheck,
  Clock3,
  Copy,
  Database,
  Download,
  GitBranch,
  ListFilter,
  Pause,
  Play,
  RefreshCw,
  RotateCcw,
  Route,
  Search,
  Server,
  ShieldCheck,
  SlidersHorizontal,
  Timer,
  Zap,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "../components/ui/button";
import { Drawer, Modal } from "../components/ui/overlay";
import { Switch } from "../components/ui/primitives";
import { copyText } from "./code-example";
import {
  defaultRoutingConfig,
  filterRoutingCandidates,
  findRoutingRequests,
  getRoutingAttempts,
  loadRoutingConfig,
  routingCandidates,
  routingRequests,
  routingSnapshots,
  routingTrend,
  saveRoutingConfig,
  type RoutingCandidate,
  type RoutingConfig,
  type RoutingOutcome,
  type RoutingRequest,
} from "../lib/routing-demo";
import "../components/smart-routing.css";

type View =
  | "overview"
  | "snapshots"
  | "candidates"
  | "trace"
  | "release"
  | "configuration";
type ExitTarget = { view: View } | { href: string };
type CopyFn = (zh: string, en: string, ar: string) => string;
const views: View[] = [
  "overview",
  "snapshots",
  "candidates",
  "trace",
  "release",
  "configuration",
];
const viewIcons = [
  Activity,
  Database,
  ListFilter,
  Route,
  GitBranch,
  SlidersHorizontal,
];
const phaseTraffic = [0, 1, 5, 20, 50, 100];
// Keep unsaved edits across in-app history navigation. A page reload still
// prompts before leaving; only explicit Save writes the routing configuration.
let unsavedRoutingDraft: RoutingConfig | null = null;

function Panel({
  title,
  description,
  action,
  children,
  className = "",
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`sr-panel ${className}`}>
      <header className="sr-panel-heading">
        <div>
          <h2>{title}</h2>
          {description && <p>{description}</p>}
        </div>
        {action}
      </header>
      {children}
    </section>
  );
}
function Pill({
  tone = "blue",
  children,
}: {
  tone?: "blue" | "green" | "amber" | "red" | "muted";
  children: ReactNode;
}) {
  return (
    <span className={`sr-pill is-${tone}`}>
      <i />
      {children}
    </span>
  );
}
function outcomeLabel(outcome: RoutingOutcome, c: CopyFn) {
  return {
    smart: c("智能路由成功", "Smart route success", "نجاح التوجيه الذكي"),
    "pre-fallback": c("前置回退", "Pre-route fallback", "رجوع قبل التوجيه"),
    "post-fallback": c("后置回退", "Post-route fallback", "رجوع بعد التوجيه"),
    bypassed: c("未命中灰度", "Rollout bypassed", "تجاوز النشر التجريبي"),
  }[outcome];
}
function statusLabel(status: RoutingCandidate["status"], c: CopyFn) {
  return {
    eligible: c("可参与路由", "Eligible", "مؤهل"),
    degraded: c("降级观察", "Degraded", "أداء منخفض"),
    circuit: c("已熔断", "Circuit open", "قاطع الدائرة مفتوح"),
  }[status];
}
function RequestTable({
  rows,
  onTrace,
  c,
}: {
  rows: RoutingRequest[];
  onTrace: (row: RoutingRequest) => void;
  c: CopyFn;
}) {
  return (
    <div className="sr-table-scroll">
      <table className="sr-table">
        <thead>
          <tr>
            <th>{c("请求编号", "Request ID", "معرّف الطلب")}</th>
            <th>{c("模型", "Model", "النموذج")}</th>
            <th>{c("路由结果", "Routing outcome", "نتيجة التوجيه")}</th>
            <th>{c("决策耗时", "Route time", "زمن التوجيه")}</th>
            <th>{c("时间（迪拜）", "Time · Dubai", "الوقت · دبي")}</th>
            <th>
              <span className="sr-only">{c("操作", "Action", "الإجراء")}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id}>
              <td>
                <button className="sr-id-link" onClick={() => onTrace(row)}>
                  {row.id}
                </button>
              </td>
              <td>
                <strong>{row.model}</strong>
              </td>
              <td>
                <Pill
                  tone={
                    row.outcome === "smart"
                      ? "green"
                      : row.outcome === "bypassed"
                        ? "muted"
                        : "amber"
                  }
                >
                  {outcomeLabel(row.outcome, c)}
                </Pill>
              </td>
              <td className="sr-number">
                {row.latency} ms{c("（毫秒）", "", "")}
              </td>
              <td className="sr-number">{row.time}</td>
              <td>
                <button
                  className="sr-row-action"
                  onClick={() => onTrace(row)}
                  aria-label={`${c("追踪请求", "Trace request", "تتبع الطلب")} ${row.id}`}
                >
                  <ArrowUpRight size={16} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
function downloadRequests(rows: RoutingRequest[]) {
  const header = [
    "request_id",
    "model",
    "routing_outcome",
    "route_ms",
    "cost_usd",
    "snapshot_id",
  ];
  const escape = (value: string) =>
    `"${(/^[=+\-@]/.test(value) ? "'" : "") + value.replaceAll('"', '""')}"`;
  const body = rows.map((row) =>
    [
      row.id,
      row.model,
      row.outcome,
      String(row.latency),
      String(row.cost),
      row.snapshotId,
    ]
      .map(escape)
      .join(","),
  );
  const url = URL.createObjectURL(
    new Blob(["\uFEFF" + [header.join(","), ...body].join("\n")], {
      type: "text/csv;charset=utf-8",
    }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = "veyra-routing-demo.csv";
  link.click();
  URL.revokeObjectURL(url);
}

export default function SmartRouting() {
  const { i18n } = useTranslation();
  const c: CopyFn = (zh, en, ar) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const view = views.includes(params.get("view") as View)
    ? (params.get("view") as View)
    : "overview";
  const [config, setConfig] = useState<RoutingConfig>(loadRoutingConfig);
  const [draft, setDraft] = useState<RoutingConfig>(() =>
    unsavedRoutingDraft ? { ...unsavedRoutingDraft } : config,
  );
  const [returnView, setReturnView] = useState<View>("overview");
  const [pendingExit, setPendingExit] = useState<ExitTarget | null>(null);
  const dirty = JSON.stringify(draft) !== JSON.stringify(config);
  const [configError, setConfigError] = useState("");
  const [confirmation, setConfirmation] = useState<
    "disable" | "rollback" | "stop" | "discard" | null
  >(null);
  const [refreshedAt, setRefreshedAt] = useState<Date | null>(null);
  const [period, setPeriod] = useState<"30" | "60">("30");
  const [candidateQuery, setCandidateQuery] = useState("");
  const [candidateStatus, setCandidateStatus] = useState("all");
  const [model, setModel] = useState("all");
  const [sort, setSort] = useState("score");
  const [candidateDetail, setCandidateDetail] =
    useState<RoutingCandidate | null>(null);
  const [snapshotId, setSnapshotId] = useState(routingSnapshots[0].id);
  const [inspectSnapshot, setInspectSnapshot] = useState<string | null>(null);
  const [validated, setValidated] = useState(false);
  const traceQuery = params.get("request") || routingRequests[0].id;
  const [requestInput, setRequestInput] = useState(traceQuery);
  const traceResults = useMemo(
    () => findRoutingRequests(traceQuery),
    [traceQuery],
  );
  const [selectedRequestId, setSelectedRequestId] = useState(
    traceResults[0]?.id || "",
  );
  const [outcomeFilter, setOutcomeFilter] = useState("all");
  const [requestPage, setRequestPage] = useState(1);
  const selectedRequest =
    traceResults.find((row) => row.id === selectedRequestId) || traceResults[0];
  const currentSnapshot = routingSnapshots.find(
    (snapshot) => snapshot.id === snapshotId,
  )!;
  const detailSnapshot = routingSnapshots.find(
    (snapshot) => snapshot.id === inspectSnapshot,
  );
  const windowTrend = period === "30" ? routingTrend.slice(-6) : routingTrend;
  const totalRequests = windowTrend.reduce((sum, row) => sum + row.requests, 0);
  const successRate =
    windowTrend.reduce((sum, row) => sum + row.requests * row.success, 0) /
    totalRequests;
  const smartSuccess =
    windowTrend.reduce((sum, row) => sum + row.requests * row.smartSuccess, 0) /
    totalRequests;
  const filteredCandidates = useMemo(
    () =>
      filterRoutingCandidates(candidateQuery, candidateStatus, model).sort(
        (a, b) =>
          sort === "cost"
            ? a.inputPrice - b.inputPrice
            : sort === "latency"
              ? a.ttft - b.ttft
              : b.score - a.score,
      ),
    [candidateQuery, candidateStatus, model, sort],
  );
  const filteredRequests = routingRequests.filter(
    (row) => outcomeFilter === "all" || row.outcome === outcomeFilter,
  );
  const recentRows = filteredRequests.slice(
    (requestPage - 1) * 6,
    requestPage * 6,
  );
  const tabLabels = [
    c("运行概览", "Overview", "نظرة عامة"),
    c("快照中心", "Snapshots", "اللقطات"),
    c("候选诊断", "Candidates", "الموارد المرشحة"),
    c("决策追踪", "Decision trace", "تتبع القرار"),
    c("灰度发布", "Rollout", "النشر التدريجي"),
    c("路由配置", "Configuration", "الإعدادات"),
  ];
  useEffect(() => {
    setRequestInput(traceQuery);
    setSelectedRequestId(findRoutingRequests(traceQuery)[0]?.id || "");
  }, [traceQuery]);
  useEffect(() => {
    setRequestPage(1);
  }, [outcomeFilter]);
  const changeView = (next: View) => {
    const search = new URLSearchParams(params);
    search.set("view", next);
    if (next !== "trace") search.delete("request");
    setParams(search);
  };
  const exitConfiguration = (target: ExitTarget) => {
    unsavedRoutingDraft = null;
    setDraft({ ...config });
    setConfigError("");
    if ("view" in target) changeView(target.view);
    else {
      const url = new URL(target.href, window.location.href);
      if (url.origin === window.location.origin)
        navigate(`${url.pathname}${url.search}${url.hash}`);
      else window.location.assign(url.href);
    }
  };
  const requestExit = (target: ExitTarget) => {
    if (dirty) {
      setPendingExit(target);
      setConfirmation("discard");
    } else exitConfiguration(target);
  };
  const go = (next: View) => {
    if (next === view) return;
    if (view === "configuration") requestExit({ view: next });
    else changeView(next);
  };
  useEffect(() => {
    if (view === "configuration")
      unsavedRoutingDraft = dirty ? { ...draft } : null;
  }, [view, draft, dirty]);
  useEffect(() => {
    if (view !== "configuration" || !dirty) return;
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    const followLink = (event: MouseEvent) => {
      if (
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      )
        return;
      const link =
        event.target instanceof Element
          ? event.target.closest<HTMLAnchorElement>("a[href]")
          : null;
      if (
        !link ||
        link.hasAttribute("download") ||
        (link.target && link.target !== "_self")
      )
        return;
      const url = new URL(link.href, window.location.href);
      if (
        !/^https?:$/.test(url.protocol) ||
        url.href === window.location.href ||
        (url.pathname === window.location.pathname &&
          url.search === window.location.search &&
          url.hash)
      )
        return;
      event.preventDefault();
      event.stopPropagation();
      setPendingExit({ href: url.href });
      setConfirmation("discard");
    };
    window.addEventListener("beforeunload", beforeUnload);
    document.addEventListener("click", followLink, true);
    return () => {
      window.removeEventListener("beforeunload", beforeUnload);
      document.removeEventListener("click", followLink, true);
    };
  }, [view, dirty]);
  const openTrace = (row: RoutingRequest) => {
    const search = new URLSearchParams(params);
    search.set("view", "trace");
    search.set("request", row.id);
    setParams(search);
    setSelectedRequestId(row.id);
  };
  const commit = (next: RoutingConfig) => {
    setConfig(next);
    const saved = saveRoutingConfig(next);
    toast.success(
      saved
        ? c(
            "演示配置已保存到此浏览器",
            "Demo configuration saved in this browser",
            "حُفظ الإعداد التجريبي في هذا المتصفح",
          )
        : c(
            "演示配置已生效；浏览器不允许持久保存",
            "Demo configuration applied; browser storage is unavailable",
            "طُبق الإعداد التجريبي؛ تخزين المتصفح غير متاح",
          ),
    );
  };
  const openConfig = () => {
    if (view === "configuration") return;
    setDraft(
      unsavedRoutingDraft
        ? {
            ...unsavedRoutingDraft,
            enabled: config.enabled,
            experimentRunning: config.experimentRunning,
          }
        : { ...config },
    );
    setConfigError("");
    setReturnView(view);
    changeView("configuration");
  };
  const saveConfig = () => {
    if (
      !Number.isInteger(draft.timeoutMs) ||
      draft.timeoutMs < 500 ||
      draft.timeoutMs > 10000 ||
      !Number.isInteger(draft.maxAttempts) ||
      draft.maxAttempts < 1 ||
      draft.maxAttempts > 3 ||
      !Number.isFinite(draft.traffic) ||
      draft.traffic < 0 ||
      draft.traffic > 100
    ) {
      setConfigError(
        c(
          "超时需为 500–10,000 毫秒整数，最多尝试 1–3 次，灰度流量 0–100%。",
          "Timeout must be 500–10,000 ms, attempts 1–3, and traffic 0–100%.",
          "المهلة 500–10,000 مللي ثانية، والمحاولات 1–3، وحركة المرور 0–100%.",
        ),
      );
      return;
    }
    commit({ ...draft });
    unsavedRoutingDraft = null;
    changeView(returnView);
  };
  const performConfirmation = () => {
    if (confirmation === "discard" && pendingExit) {
      exitConfiguration(pendingExit);
      setPendingExit(null);
    }
    if (confirmation === "disable") commit({ ...config, enabled: false });
    if (confirmation === "stop")
      commit({ ...config, experimentRunning: false });
    if (confirmation === "rollback")
      commit({
        ...config,
        phase: Math.max(0, config.phase - 1),
        traffic: phaseTraffic[Math.max(0, config.phase - 1)],
        experimentRunning: false,
      });
    setConfirmation(null);
  };
  const metrics = [
    {
      title: c("最终成功率", "Customer success", "نجاح الطلبات"),
      value: `${successRate.toFixed(2)}%`,
      note: c(
        `${totalRequests.toLocaleString()} 次请求`,
        `${totalRequests.toLocaleString()} requests`,
        `${totalRequests.toLocaleString()} طلب`,
      ),
      Icon: CircleCheck,
      tone: "green",
    },
    {
      title: c("智能路由成功率", "Smart route success", "نجاح التوجيه الذكي"),
      value: `${smartSuccess.toFixed(2)}%`,
      note: c(
        "路由首次尝试口径",
        "First smart attempt",
        "المحاولة الذكية الأولى",
      ),
      Icon: Zap,
      tone: "blue",
    },
    {
      title: c("总回退率", "Total fallback rate", "إجمالي معدل الرجوع"),
      value: period === "30" ? "1.24%" : "1.42%",
      note: c(
        "前置 + 后置回退",
        "Pre + post-route fallback",
        "رجوع قبل وبعد التوجيه",
      ),
      Icon: GitBranch,
      tone: "amber",
    },
    {
      title: c(
        "P95（95分位）路由耗时",
        "Route time · P95",
        "زمن التوجيه · P95",
      ),
      value: period === "30" ? "22 ms" : "24 ms",
      note: c("P99（99分位）36 毫秒", "P99 · 36 ms", "P99 · 36 مللي ثانية"),
      Icon: Timer,
      tone: "blue",
    },
    {
      title: c("相对成本节省", "Relative savings", "التوفير النسبي"),
      value: "12.5%",
      note: c("相对冻结基线", "Against frozen baseline", "مقارنة بخط الأساس"),
      Icon: Database,
      tone: "purple",
    },
    {
      title: c("最低事件完整率", "Event completeness", "اكتمال الأحداث"),
      value: "99.98%",
      note: c(
        "请求 / 决策 / 尝试",
        "Request / decision / attempt",
        "طلب / قرار / محاولة",
      ),
      Icon: ShieldCheck,
      tone: "green",
    },
  ];

  return (
    <div className="sr-routing">
      <header className="sr-page-heading">
        <div>
          <span className="sr-eyebrow">
            {c(
              "让每次请求，都有更好的选择",
              "THE RIGHT ROUTE, EVERY TIME",
              "المسار الأنسب في كل مرة",
            )}
          </span>
          <h1>{c("智能路由", "Smart routing", "التوجيه الذكي")}</h1>
          <p>
            {c(
              "在性能、成本与稳定性之间，找到适合你的路径。",
              "Find the right balance of performance, cost and reliability.",
              "وازن بين الأداء والتكلفة والموثوقية.",
            )}
          </p>
        </div>
        <div className="sr-heading-actions">
          {view === "configuration" ? (
            <Button onClick={() => requestExit({ view: returnView })}>
              <ArrowLeft className="directional" size={15} />
              {c("返回路由视图", "Back to routing", "العودة إلى التوجيه")}
            </Button>
          ) : (
            <>
              <Button
                onClick={() => {
                  setRefreshedAt(new Date());
                  toast.success(
                    c(
                      "已重新读取本地演示数据",
                      "Local demo data refreshed",
                      "تم تحديث بيانات العرض المحلية",
                    ),
                  );
                }}
              >
                <RefreshCw size={15} />
                {c("刷新", "Refresh", "تحديث")}
              </Button>
              <Button variant="primary" onClick={openConfig}>
                <SlidersHorizontal size={15} />
                {c("路由配置", "Configure routing", "إعداد التوجيه")}
              </Button>
            </>
          )}
        </div>
      </header>
      <div className="sr-demo-note">
        <CircleAlert size={14} />
        <span>
          {c(
            "本地交互演示 · 指标为样例数据，配置仅保存在此浏览器，不影响真实请求。",
            "Local interactive demo · Sample metrics. Configuration stays in this browser and does not affect real requests.",
            "عرض تفاعلي محلي · المقاييس تجريبية. تُحفظ الإعدادات في المتصفح ولا تؤثر على الطلبات الحقيقية.",
          )}
        </span>
        {refreshedAt && (
          <time>
            {refreshedAt.toLocaleTimeString(i18n.language, { hour12: false })}
          </time>
        )}
      </div>
      {view !== "configuration" && (
        <section
          className={`sr-status-bar ${config.enabled ? "" : "is-disabled"}`}
        >
          <div className="sr-status-symbol">
            <Route size={24} />
          </div>
          <div className="sr-status-copy">
            <strong>
              {config.enabled
                ? c(
                    "智能路由已启用",
                    "Smart routing enabled",
                    "التوجيه الذكي مفعّل",
                  )
                : c(
                    "智能路由已关闭",
                    "Smart routing disabled",
                    "التوجيه الذكي متوقف",
                  )}
            </strong>
            <p>
              {config.enabled
                ? c(
                    `灰度阶段 G${config.phase}（第 ${config.phase} 阶段） · ${config.traffic}% 演示流量`,
                    `Rollout G${config.phase} · ${config.traffic}% demo traffic`,
                    `مرحلة G${config.phase} · ${config.traffic}% حركة تجريبية`,
                  )
                : c(
                    "新模拟请求将沿默认路径分发；历史演示记录仍可查看。",
                    "New simulated requests follow the default path. Historical demo records remain available.",
                    "تتبع الطلبات التجريبية الجديدة المسار الافتراضي. تبقى السجلات السابقة متاحة.",
                  )}
            </p>
          </div>
          <div className="sr-snapshot-indicator">
            <span>{c("当前快照", "Current snapshot", "اللقطة الحالية")}</span>
            <button onClick={() => go("snapshots")}>
              {currentSnapshot.id}
              <ChevronRight size={13} />
            </button>
          </div>
          <div className="sr-master-switch">
            <span>{c("总开关", "Master switch", "المفتاح الرئيسي")}</span>
            <Switch
              id="sr-enabled"
              label={c(
                "启用智能路由",
                "Enable smart routing",
                "تفعيل التوجيه الذكي",
              )}
              checked={config.enabled}
              onChange={(enabled) =>
                enabled
                  ? commit({ ...config, enabled: true })
                  : setConfirmation("disable")
              }
            />
          </div>
        </section>
      )}
      <nav
        className="sr-tabs"
        aria-label={c(
          "智能路由视图",
          "Smart routing views",
          "طرق عرض التوجيه الذكي",
        )}
      >
        {views.map((item, index) => {
          const Icon = viewIcons[index];
          return (
            <button
              key={item}
              className={view === item ? "is-active" : ""}
              aria-current={view === item ? "page" : undefined}
              onClick={() =>
                item === "configuration" ? openConfig() : go(item)
              }
            >
              <Icon size={16} />
              <span>{tabLabels[index]}</span>
            </button>
          );
        })}
      </nav>

      {view !== "configuration" && unsavedRoutingDraft && (
        <div className="sr-draft-recovery" role="status">
          <CircleAlert size={17} />
          <div>
            <strong>
              {c(
                "你有尚未保存的路由配置",
                "You have unsaved routing settings",
                "لديك إعدادات توجيه غير محفوظة",
              )}
            </strong>
            <span>
              {c(
                "草稿保留在当前页面会话中，还没有应用。",
                "Your draft is retained in this page session and has not been applied.",
                "المسودة محفوظة في جلسة الصفحة الحالية ولم تُطبق بعد.",
              )}
            </span>
          </div>
          <Button size="small" onClick={openConfig}>
            {c("继续编辑", "Continue editing", "متابعة التعديل")}
          </Button>
        </div>
      )}

      {view === "overview" && (
        <div className="sr-view">
          <div className="sr-section-toolbar">
            <span>
              {c(
                "历史演示窗口 · 2026-09-24，迪拜时区",
                "Historical demo window · Sep 24, 2026, Dubai time",
                "نافذة تجريبية تاريخية · 24 سبتمبر 2026، توقيت دبي",
              )}
            </span>
            <div className="sr-segments">
              {(["30", "60"] as const).map((value) => (
                <button
                  key={value}
                  className={period === value ? "is-active" : ""}
                  aria-pressed={period === value}
                  onClick={() => setPeriod(value)}
                >
                  {c(
                    `最近 ${value} 分钟`,
                    `Last ${value} min`,
                    `آخر ${value} دقيقة`,
                  )}
                </button>
              ))}
            </div>
          </div>
          <div className="sr-metrics">
            {metrics.map(({ title, value, note, Icon, tone }) => (
              <article className="sr-metric" key={title}>
                <div>
                  <span>{title}</span>
                  <Icon className={`sr-tone-${tone}`} size={17} />
                </div>
                <strong>{value}</strong>
                <small>{note}</small>
              </article>
            ))}
          </div>
          <div className="sr-overview-grid">
            <Panel
              title={c("路由运行趋势", "Routing performance", "أداء التوجيه")}
              description={c(
                "每 5 分钟一个窗口，成功率可对照查看。",
                "Five-minute windows, with both success rates in view.",
                "نوافذ من خمس دقائق لمقارنة معدلي النجاح.",
              )}
              action={
                <span className="sr-live-label">
                  <i />
                  {c("历史样例", "Historical sample", "عينة تاريخية")}
                </span>
              }
            >
              <div className="sr-trend-summary">
                <strong>
                  {successRate.toFixed(2)}
                  <small>%</small>
                </strong>
                <div>
                  <span>
                    <i />
                    {c("最终成功率", "Customer success", "نجاح الطلبات")}
                  </span>
                  <span>
                    <i className="is-dashed" />
                    {c(
                      "智能路由成功率",
                      "Smart route success",
                      "نجاح التوجيه الذكي",
                    )}
                  </span>
                </div>
              </div>
              <div className="sr-chart" dir="ltr">
                <ResponsiveContainer width="100%" height="100%">
                  <ComposedChart
                    data={windowTrend}
                    margin={{ top: 15, right: 15, bottom: 0, left: -18 }}
                  >
                    <defs>
                      <linearGradient
                        id="sr-success-fill"
                        x1="0"
                        y1="0"
                        x2="0"
                        y2="1"
                      >
                        <stop
                          offset="0%"
                          stopColor="var(--brand)"
                          stopOpacity={0.16}
                        />
                        <stop
                          offset="100%"
                          stopColor="var(--brand)"
                          stopOpacity={0.01}
                        />
                      </linearGradient>
                    </defs>
                    <CartesianGrid
                      vertical={false}
                      stroke="var(--border)"
                      strokeDasharray="4 5"
                    />
                    <XAxis
                      dataKey="time"
                      axisLine={false}
                      tickLine={false}
                      tick={{ fill: "var(--muted)", fontSize: 10 }}
                      tickMargin={12}
                    />
                    <YAxis
                      domain={[99, 100]}
                      ticks={[99, 99.25, 99.5, 99.75, 100]}
                      tickFormatter={(value) => `${value}%`}
                      axisLine={false}
                      tickLine={false}
                      tick={{ fill: "var(--muted)", fontSize: 10 }}
                    />
                    <Tooltip
                      contentStyle={{
                        background: "var(--surface)",
                        borderColor: "var(--border)",
                        borderRadius: 10,
                        fontSize: 12,
                      }}
                      formatter={(value: number, name: string) => [
                        `${value.toFixed(2)}%`,
                        name === "success"
                          ? c("最终成功率", "Customer success", "نجاح الطلبات")
                          : c("智能成功率", "Smart success", "النجاح الذكي"),
                      ]}
                    />
                    <Area
                      type="monotone"
                      dataKey="success"
                      stroke="var(--brand)"
                      fill="url(#sr-success-fill)"
                      strokeWidth={2.5}
                      isAnimationActive={false}
                    />
                    <Line
                      type="monotone"
                      dataKey="smartSuccess"
                      stroke="#4bb898"
                      strokeDasharray="5 5"
                      strokeWidth={2}
                      dot={false}
                      isAnimationActive={false}
                    />
                  </ComposedChart>
                </ResponsiveContainer>
              </div>
              <div className="sr-chart-footer">
                <span>
                  {c("请求样本", "Request samples", "عينات الطلبات")}
                  <strong>{totalRequests.toLocaleString()}</strong>
                </span>
                <span>
                  {c("窗口粒度", "Window size", "حجم النافذة")}
                  <strong>{c("5 分钟", "5 minutes", "5 دقائق")}</strong>
                </span>
                <span>
                  {c("数据口径", "Scope", "النطاق")}
                  <strong>
                    {c(
                      "最终客户结果",
                      "Final customer outcome",
                      "نتيجة العميل النهائية",
                    )}
                  </strong>
                </span>
              </div>
            </Panel>
            <Panel
              title={c("冻结基线对比", "Frozen baseline", "خط الأساس الثابت")}
              description={c(
                "固定历史基线，不随演示配置改变。",
                "A historical baseline, independent of configuration.",
                "خط أساس تاريخي مستقل عن الإعدادات.",
              )}
              className="sr-baseline"
            >
              <div className="sr-baseline-version">
                <Database size={17} />
                <span>baseline-2026w38</span>
                <Pill tone="muted">{c("已冻结", "Frozen", "ثابت")}</Pill>
              </div>
              {[
                {
                  label: c("最终成功率", "Customer success", "نجاح الطلبات"),
                  before: "99.50%",
                  after: `${successRate.toFixed(2)}%`,
                },
                {
                  label: c(
                    "每千次请求成本",
                    "Cost / 1K requests",
                    "تكلفة ألف طلب",
                  ),
                  before: "$3.20",
                  after: "$2.80",
                },
                {
                  label: c(
                    "后置回退率",
                    "Post-route fallback",
                    "الرجوع بعد التوجيه",
                  ),
                  before: "1.00%",
                  after: "0.41%",
                },
              ].map((row) => (
                <div className="sr-baseline-row" key={row.label}>
                  <span>{row.label}</span>
                  <div>
                    <small>{row.before}</small>
                    <ArrowRight size={12} />
                    <strong>{row.after}</strong>
                  </div>
                </div>
              ))}
              <div className="sr-baseline-pass">
                <ShieldCheck size={19} />
                <p>
                  {c(
                    "样例指标在预设观察范围内。发布扩量仍需手动确认。",
                    "Sample metrics are within guardrails. Expansion still requires a manual decision.",
                    "المقاييس التجريبية ضمن الحدود. التوسعة تحتاج قرارًا يدويًا.",
                  )}
                </p>
              </div>
              <button className="sr-text-button" onClick={() => go("release")}>
                {c(
                  "查看发布与止损规则",
                  "View rollout guardrails",
                  "عرض حدود النشر",
                )}
                <ArrowUpRight size={15} />
              </button>
            </Panel>
          </div>
          <div className="sr-health-grid">
            {[
              {
                title: c("快照健康", "Snapshot health", "صحة اللقطة"),
                Icon: Database,
                value: `${currentSnapshot.instances}/${currentSnapshot.totalInstances}`,
                caption: c("实例版本一致", "Instances aligned", "نسخ متطابقة"),
                action: () => go("snapshots"),
              },
              {
                title: c("候选资源", "Candidate resources", "الموارد المرشحة"),
                Icon: Server,
                value: String(
                  routingCandidates.filter((row) => row.status === "eligible")
                    .length,
                ),
                caption: c(
                  "健康资源可参与路由",
                  "Healthy resources eligible",
                  "موارد سليمة مؤهلة",
                ),
                action: () => go("candidates"),
              },
              {
                title: c("事件回放", "Event replay", "إعادة الأحداث"),
                Icon: Clock3,
                value: "1.2 s",
                caption: c(
                  "异步队列延迟（秒）",
                  "Async queue delay",
                  "تأخير الصف غير المتزامن",
                ),
                action: () => go("trace"),
              },
            ].map(({ title, Icon, value, caption, action }) => (
              <button className="sr-health-card" key={title} onClick={action}>
                <span className="sr-health-icon">
                  <Icon size={19} />
                </span>
                <div>
                  <span>{title}</span>
                  <strong>
                    {value}
                    <small>{caption}</small>
                  </strong>
                </div>
                <ArrowUpRight size={16} />
              </button>
            ))}
          </div>
          <Panel
            title={c(
              "最近路由请求",
              "Recent routed requests",
              "طلبات التوجيه الأخيرة",
            )}
            description={c(
              "代表性请求样例，点击编号可查看对应的完整决策。",
              "Representative samples. Open any ID to inspect that exact decision.",
              "عينات ممثلة. افتح أي معرّف لفحص قراره الكامل.",
            )}
            action={
              <button
                className="sr-text-button"
                onClick={() => {
                  downloadRequests(filteredRequests);
                  toast.success(
                    c(
                      "已导出演示请求",
                      "Demo requests exported",
                      "تم تصدير الطلبات التجريبية",
                    ),
                  );
                }}
              >
                <Download size={14} />
                {c("导出样例", "Export samples", "تصدير العينات")}
              </button>
            }
          >
            <div className="sr-table-toolbar">
              <label>
                {c("路由结果", "Outcome", "النتيجة")}
                <select
                  value={outcomeFilter}
                  onChange={(event) => setOutcomeFilter(event.target.value)}
                >
                  <option value="all">
                    {c("全部结果", "All outcomes", "كل النتائج")}
                  </option>
                  {(
                    [
                      "smart",
                      "pre-fallback",
                      "post-fallback",
                      "bypassed",
                    ] as const
                  ).map((value) => (
                    <option value={value} key={value}>
                      {outcomeLabel(value, c)}
                    </option>
                  ))}
                </select>
              </label>
              <span>
                {c(
                  `${filteredRequests.length} 条样例`,
                  `${filteredRequests.length} samples`,
                  `${filteredRequests.length} عينة`,
                )}
              </span>
            </div>
            <RequestTable rows={recentRows} onTrace={openTrace} c={c} />
            <div className="sr-pagination">
              <span>
                {c(
                  "仅展示本地演示记录",
                  "Local demo records only",
                  "سجلات تجريبية محلية فقط",
                )}
              </span>
              <button
                disabled={requestPage === 1}
                onClick={() => setRequestPage((page) => page - 1)}
              >
                {c("上一页", "Previous", "السابق")}
              </button>
              <span>
                {requestPage} /{" "}
                {Math.max(1, Math.ceil(filteredRequests.length / 6))}
              </span>
              <button
                disabled={requestPage >= Math.ceil(filteredRequests.length / 6)}
                onClick={() => setRequestPage((page) => page + 1)}
              >
                {c("下一页", "Next", "التالي")}
              </button>
            </div>
          </Panel>
        </div>
      )}

      {view === "snapshots" && (
        <div className="sr-view">
          <div className="sr-snapshot-grid">
            <section className="sr-current-snapshot">
              <div>
                <span className="sr-inverse-icon">
                  <Database size={23} />
                </span>
                <Pill tone="green">
                  {c("当前有效", "Active snapshot", "اللقطة النشطة")}
                </Pill>
              </div>
              <p>{c("当前发布版本", "Published version", "النسخة المنشورة")}</p>
              <h2>{currentSnapshot.id}</h2>
              <span>
                {currentSnapshot.publishedAt} · {c("迪拜", "Dubai", "دبي")}
              </span>
              <footer>
                <span>
                  {c("覆盖模型", "Models", "النماذج")}
                  <strong>{currentSnapshot.models}</strong>
                </span>
                <span>
                  {c("资源候选", "Resources", "الموارد")}
                  <strong>{currentSnapshot.resources}</strong>
                </span>
                <span>
                  {c("已加载实例", "Loaded", "المحمّل")}
                  <strong>
                    {currentSnapshot.instances}/{currentSnapshot.totalInstances}
                  </strong>
                </span>
              </footer>
            </section>
            <Panel
              title={c(
                "完整性与加载校验",
                "Integrity & loading",
                "السلامة والتحميل",
              )}
              description={c(
                "本地演示校验，不连接资源平台。",
                "Local demo checks; no resource-platform connection.",
                "فحوص محلية تجريبية دون اتصال بمنصة الموارد.",
              )}
              action={
                <Button
                  size="small"
                  onClick={() => {
                    setValidated(true);
                    toast.success(
                      c(
                        "演示校验通过：8 个实例的版本与摘要一致",
                        "Demo validation passed: versions and hashes match across 8 instances",
                        "نجح التحقق التجريبي: تتطابق النسخ والتجزئات عبر 8 مثيلات",
                      ),
                    );
                  }}
                >
                  <RefreshCw size={13} />
                  {c("重新校验", "Revalidate", "إعادة التحقق")}
                </Button>
              }
            >
              <div className="sr-validation-list">
                {[
                  c(
                    "数据契约与必填字段完整",
                    "Data contract and required fields",
                    "عقد البيانات والحقول المطلوبة",
                  ),
                  c(
                    "完整性摘要一致",
                    "Integrity hash consistency",
                    "اتساق تجزئة السلامة",
                  ),
                  c(
                    "全部实例加载相同版本",
                    "All instances loaded the same version",
                    "حمّلت جميع المثيلات النسخة نفسها",
                  ),
                ].map((label) => (
                  <div key={label}>
                    <CircleCheck size={17} />
                    <span>{label}</span>
                    <strong>{c("通过", "Passed", "نجح")}</strong>
                  </div>
                ))}
              </div>
              <p className="sr-panel-footnote" role="status">
                {validated
                  ? c(
                      "刚刚完成本地校验，结果全部通过。",
                      "Local validation just completed. All checks passed.",
                      "اكتمل التحقق المحلي للتو. نجحت جميع الفحوص.",
                    )
                  : c(
                      "历史样例：有效期与版本以记录产生时为准。",
                      "Historical sample: validity is evaluated at capture time.",
                      "عينة تاريخية: تُقيّم الصلاحية وقت التسجيل.",
                    )}
              </p>
            </Panel>
          </div>
          <Panel
            title={c("版本记录", "Version history", "سجل النسخ")}
            description={c(
              "查看规则版本，或在本地演示中切换快照。",
              "Inspect rule versions or switch the active demo snapshot.",
              "افحص نسخ القواعد أو بدّل اللقطة التجريبية النشطة.",
            )}
          >
            <div className="sr-table-scroll">
              <table className="sr-table">
                <thead>
                  <tr>
                    {[
                      c("快照版本", "Snapshot", "اللقطة"),
                      c("发布时间", "Published", "النشر"),
                      c("规则版本", "Rule version", "نسخة القواعد"),
                      c("实例加载", "Instances", "المثيلات"),
                      c("状态", "Status", "الحالة"),
                      c("操作", "Actions", "الإجراءات"),
                    ].map((label) => (
                      <th key={label}>{label}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {routingSnapshots.map((snapshot) => (
                    <tr key={snapshot.id}>
                      <td className="sr-code">{snapshot.id}</td>
                      <td>{snapshot.publishedAt}</td>
                      <td className="sr-code">{snapshot.rules}</td>
                      <td>
                        {snapshot.instances}/{snapshot.totalInstances}
                      </td>
                      <td>
                        <Pill
                          tone={snapshot.id === snapshotId ? "green" : "muted"}
                        >
                          {snapshot.id === snapshotId
                            ? c("当前", "Active", "نشط")
                            : c("历史有效版本", "Previous valid", "سابق صالح")}
                        </Pill>
                      </td>
                      <td>
                        <button
                          className="sr-text-button"
                          onClick={() => setInspectSnapshot(snapshot.id)}
                        >
                          {c("查看详情", "Details", "التفاصيل")}
                          <ArrowUpRight size={14} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Panel>
          <div className="sr-process-strip">
            {[
              {
                Icon: Database,
                title: c("接收与校验", "Receive & validate", "استلام وتحقق"),
                text: c(
                  "核对有效期、规则版本、数据契约和完整性摘要。",
                  "Check validity, rules, contracts and integrity.",
                  "تحقق من الصلاحية والقواعد والعقود والسلامة.",
                ),
              },
              {
                Icon: Route,
                title: c("本地选择路径", "Choose locally", "اختيار محلي"),
                text: c(
                  "结合模型能力、健康状态与容量余量选择资源。",
                  "Match capability, health and available capacity.",
                  "مطابقة القدرات والصحة والسعة المتاحة.",
                ),
              },
              {
                Icon: RefreshCw,
                title: c(
                  "异步回放事实",
                  "Replay asynchronously",
                  "إعادة غير متزامنة",
                ),
                text: c(
                  "记录请求、决策和尝试，不占用客户同步调用链。",
                  "Record facts outside the synchronous request path.",
                  "سجّل الوقائع خارج مسار الطلب المتزامن.",
                ),
              },
            ].map(({ Icon, title, text }, index) => (
              <section key={title}>
                <span>
                  <Icon size={20} />
                </span>
                <small>0{index + 1}</small>
                <h3>{title}</h3>
                <p>{text}</p>
              </section>
            ))}
          </div>
        </div>
      )}

      {view === "candidates" && (
        <div className="sr-view">
          <div className="sr-candidate-stats">
            {(["eligible", "degraded", "circuit"] as const).map((status) => (
              <button
                className={`sr-candidate-stat is-${status}`}
                key={status}
                onClick={() =>
                  setCandidateStatus(
                    candidateStatus === status ? "all" : status,
                  )
                }
                aria-pressed={candidateStatus === status}
              >
                <span>{statusLabel(status, c)}</span>
                <strong>
                  {
                    routingCandidates.filter((row) => row.status === status)
                      .length
                  }
                  <small>{c("个资源", "resources", "موارد")}</small>
                </strong>
                <p>
                  {status === "eligible"
                    ? c(
                        "能力、预算与健康检查通过",
                        "Capability, budget and health passed",
                        "اجتازت فحوص القدرات والميزانية والصحة",
                      )
                    : status === "degraded"
                      ? c(
                          "保留为候选，降低路由优先级",
                          "Retained at a lower routing priority",
                          "محفوظة بأولوية توجيه أقل",
                        )
                      : c(
                          "不参与路由，等待恢复检查",
                          "Excluded until recovery checks pass",
                          "مستبعدة حتى نجاح فحوص التعافي",
                        )}
                </p>
              </button>
            ))}
          </div>
          <Panel
            title={c(
              "候选资源诊断",
              "Candidate diagnostics",
              "تشخيص الموارد المرشحة",
            )}
            description={c(
              "平台事实与本地判断分开展示，点击解释查看入选或排除原因。",
              "Inspect platform facts and local decisions, including eligibility reasons.",
              "افحص وقائع المنصة والقرارات المحلية وأسباب الأهلية.",
            )}
          >
            <div className="sr-filters">
              <label className="sr-search-field">
                <Search size={16} />
                <input
                  aria-label={c(
                    "搜索候选资源",
                    "Search candidates",
                    "البحث في الموارد",
                  )}
                  placeholder={c(
                    "搜索资源、厂商或模型",
                    "Search resources, providers or models",
                    "ابحث عن مورد أو مزود أو نموذج",
                  )}
                  value={candidateQuery}
                  onChange={(event) => setCandidateQuery(event.target.value)}
                />
              </label>
              <select
                aria-label={c("候选状态", "Candidate status", "حالة المورد")}
                value={candidateStatus}
                onChange={(event) => setCandidateStatus(event.target.value)}
              >
                <option value="all">
                  {c("全部状态", "All statuses", "كل الحالات")}
                </option>
                {(["eligible", "degraded", "circuit"] as const).map(
                  (status) => (
                    <option value={status} key={status}>
                      {statusLabel(status, c)}
                    </option>
                  ),
                )}
              </select>
              <select
                aria-label={c("筛选模型", "Filter model", "تصفية النموذج")}
                value={model}
                onChange={(event) => setModel(event.target.value)}
              >
                <option value="all">
                  {c("全部模型", "All models", "كل النماذج")}
                </option>
                {[...new Set(routingCandidates.map((row) => row.model))].map(
                  (name) => (
                    <option key={name}>{name}</option>
                  ),
                )}
              </select>
              <select
                aria-label={c("候选排序", "Candidate sort", "ترتيب الموارد")}
                value={sort}
                onChange={(event) => setSort(event.target.value)}
              >
                <option value="score">
                  {c("综合得分优先", "Highest score", "أعلى نتيجة")}
                </option>
                <option value="latency">
                  {c("首字延迟优先", "Lowest latency", "أقل زمن استجابة")}
                </option>
                <option value="cost">
                  {c("输入成本优先", "Lowest input cost", "أقل تكلفة إدخال")}
                </option>
              </select>
            </div>
            <div className="sr-table-scroll">
              <table className="sr-table sr-candidate-table">
                <thead>
                  <tr>
                    {[
                      c("资源 / 模型", "Resource / model", "المورد / النموذج"),
                      c("本地判断", "Local decision", "القرار المحلي"),
                      c("首字延迟", "First-token latency", "زمن أول رمز"),
                      c("错误率", "Error rate", "معدل الأخطاء"),
                      c("容量使用", "Capacity used", "السعة المستخدمة"),
                      c(
                        "输入 / 输出成本¹",
                        "Input / output cost¹",
                        "تكلفة الإدخال / الإخراج¹",
                      ),
                      c("得分", "Score", "النتيجة"),
                      c("操作", "Action", "الإجراء"),
                    ].map((label) => (
                      <th key={label}>{label}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {filteredCandidates.map((candidate) => (
                    <tr key={candidate.id}>
                      <td>
                        <div className="sr-resource-cell">
                          <span
                            className={`sr-resource-mark is-${candidate.provider.toLowerCase()}`}
                          >
                            {candidate.provider.charAt(0)}
                          </span>
                          <div>
                            <strong>{candidate.name}</strong>
                            <small>{candidate.model}</small>
                          </div>
                        </div>
                      </td>
                      <td>
                        <Pill
                          tone={
                            candidate.status === "eligible"
                              ? "green"
                              : candidate.status === "degraded"
                                ? "amber"
                                : "red"
                          }
                        >
                          {statusLabel(candidate.status, c)}
                        </Pill>
                      </td>
                      <td className="sr-number">{candidate.ttft} ms</td>
                      <td className="sr-number">
                        {candidate.errorRate.toFixed(2)}%
                      </td>
                      <td>
                        <div
                          className={`sr-capacity ${candidate.capacity > 80 ? "is-high" : ""}`}
                        >
                          <span>{candidate.capacity}%</span>
                          <i>
                            <b style={{ width: `${candidate.capacity}%` }} />
                          </i>
                        </div>
                      </td>
                      <td className="sr-number">
                        ${candidate.inputPrice.toFixed(2)} / $
                        {candidate.outputPrice.toFixed(2)}
                      </td>
                      <td>
                        <span
                          className={`sr-score ${candidate.score < 80 ? "is-low" : ""}`}
                        >
                          {candidate.score}
                        </span>
                      </td>
                      <td>
                        <button
                          className="sr-text-button"
                          onClick={() => setCandidateDetail(candidate)}
                        >
                          {c("解释", "Explain", "تفسير")}
                          <ArrowUpRight size={14} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!filteredCandidates.length && (
              <div className="sr-empty">
                <Search size={29} />
                <h3>
                  {c(
                    "没有匹配的候选资源",
                    "No matching candidates",
                    "لا توجد موارد مطابقة",
                  )}
                </h3>
                <p>
                  {c(
                    "试试其他关键词，或清除筛选条件。",
                    "Try another keyword or clear your filters.",
                    "جرّب كلمة أخرى أو امسح عوامل التصفية.",
                  )}
                </p>
                <Button
                  onClick={() => {
                    setCandidateQuery("");
                    setCandidateStatus("all");
                    setModel("all");
                  }}
                >
                  {c("清除筛选", "Clear filters", "مسح التصفية")}
                </Button>
              </div>
            )}
            <div className="sr-panel-footnote">
              {c(
                `共 ${filteredCandidates.length} 个候选 · ¹ 美元 / 百万字元 · 延迟单位 ms（毫秒）`,
                `${filteredCandidates.length} candidates · ¹ USD per million tokens · Latency in milliseconds`,
                `${filteredCandidates.length} موارد · ¹ دولار لكل مليون رمز · زمن الاستجابة بالمللي ثانية`,
              )}
            </div>
          </Panel>
        </div>
      )}

      {view === "trace" && (
        <div className="sr-view">
          <Panel
            title={c(
              "查找一次路由决策",
              "Find a routing decision",
              "ابحث عن قرار توجيه",
            )}
            description={c(
              "输入请求编号或会话编号，仅查询本地演示记录，不接收原始提示词。",
              "Look up a request or conversation ID in local demo records. No raw prompts are collected.",
              "ابحث بمعرّف الطلب أو المحادثة في السجلات المحلية. لا تُجمع المدخلات الأصلية.",
            )}
          >
            <form
              className="sr-trace-search"
              onSubmit={(event) => {
                event.preventDefault();
                const search = new URLSearchParams(params);
                search.set("view", "trace");
                search.set("request", requestInput.trim() || " ");
                setParams(search);
              }}
            >
              <label className="sr-search-field">
                <Search size={17} />
                <input
                  aria-label={c(
                    "请求或会话编号",
                    "Request or conversation ID",
                    "معرّف الطلب أو المحادثة",
                  )}
                  value={requestInput}
                  onChange={(event) => setRequestInput(event.target.value)}
                  placeholder="req_xy_240912"
                />
              </label>
              <Button type="submit" variant="primary">
                {c("查询决策", "Find decision", "البحث عن القرار")}
                <ArrowRight size={15} />
              </Button>
            </form>
            <div className="sr-sample-links">
              <span>{c("试试样例", "Try a sample", "جرّب عينة")}</span>
              {routingRequests.slice(0, 4).map((row) => (
                <button key={row.id} onClick={() => openTrace(row)}>
                  {outcomeLabel(row.outcome, c)}
                </button>
              ))}
            </div>
          </Panel>
          {!selectedRequest ? (
            <div className="sr-empty sr-panel">
              <Search size={31} />
              <h3>
                {c(
                  "未找到对应记录",
                  "No matching record",
                  "لم يُعثر على سجل مطابق",
                )}
              </h3>
              <p>
                {c(
                  "请使用完整的样例请求编号或会话编号。",
                  "Use a complete sample request or conversation ID.",
                  "استخدم معرّف طلب أو محادثة تجريبيًا كاملًا.",
                )}
              </p>
              <Button onClick={() => openTrace(routingRequests[0])}>
                {c("打开示例决策", "Open sample decision", "فتح قرار تجريبي")}
              </Button>
            </div>
          ) : (
            <>
              {traceResults.length > 1 && (
                <div className="sr-trace-matches">
                  <span>
                    {c(
                      `该会话包含 ${traceResults.length} 个请求`,
                      `${traceResults.length} requests in this conversation`,
                      `${traceResults.length} طلبات في المحادثة`,
                    )}
                  </span>
                  {traceResults.map((row) => (
                    <button
                      className={
                        selectedRequest.id === row.id ? "is-active" : ""
                      }
                      key={row.id}
                      onClick={() => setSelectedRequestId(row.id)}
                    >
                      {row.id}
                    </button>
                  ))}
                </div>
              )}
              <Panel
                title={c(
                  "请求与决策摘要",
                  "Request & decision",
                  "الطلب والقرار",
                )}
                action={
                  <button
                    className="sr-text-button"
                    onClick={() =>
                      copyText(
                        selectedRequest.id,
                        c(
                          "请求编号已复制",
                          "Request ID copied",
                          "نُسخ معرّف الطلب",
                        ),
                        c("复制失败", "Copy failed", "فشل النسخ"),
                      )
                    }
                  >
                    <Copy size={14} />
                    {c("复制编号", "Copy ID", "نسخ المعرّف")}
                  </button>
                }
              >
                <div className="sr-trace-banner">
                  <CircleCheck size={22} />
                  <div>
                    <strong>
                      {c(
                        "请求最终成功",
                        "Request completed successfully",
                        "اكتمل الطلب بنجاح",
                      )}
                    </strong>
                    <span>{outcomeLabel(selectedRequest.outcome, c)}</span>
                  </div>
                  <strong>
                    {selectedRequest.latency}
                    <small> ms{c("（毫秒）", "", "")}</small>
                  </strong>
                </div>
                <dl className="sr-detail-grid">
                  {[
                    [
                      c("请求编号", "Request ID", "معرّف الطلب"),
                      selectedRequest.id,
                    ],
                    [
                      c("会话编号", "Conversation ID", "معرّف المحادثة"),
                      selectedRequest.conversationId,
                    ],
                    [
                      c("决策编号", "Decision ID", "معرّف القرار"),
                      selectedRequest.id.replace("req_", "dec_"),
                    ],
                    [c("模型", "Model", "النموذج"), selectedRequest.model],
                    [
                      c("服务等级", "Service tier", "مستوى الخدمة"),
                      selectedRequest.tier === "Pro"
                        ? c("Pro（专业）", "Pro", "احترافي")
                        : c("Standard（标准）", "Standard", "قياسي"),
                    ],
                    [
                      c("历史快照", "Historical snapshot", "اللقطة التاريخية"),
                      selectedRequest.snapshotId,
                    ],
                    [
                      c("执行尝试", "Attempts", "المحاولات"),
                      String(selectedRequest.attempts),
                    ],
                    [
                      c("请求费用", "Request cost", "تكلفة الطلب"),
                      `$${selectedRequest.cost.toFixed(6)}`,
                    ],
                  ].map(([label, value]) => (
                    <div key={label}>
                      <dt>{label}</dt>
                      <dd dir="auto">{value}</dd>
                    </div>
                  ))}
                </dl>
              </Panel>
              <Panel
                title={c("执行尝试明细", "Attempt details", "تفاصيل المحاولات")}
                description={c(
                  "每条记录对应一次实际演示尝试；执行耗时包含上游响应，与本地路由决策耗时口径不同。",
                  "Each row is one demo attempt. Execution time includes upstream work, unlike local routing decision time.",
                  "كل صف يمثل محاولة تجريبية. يشمل زمن التنفيذ عمل المزود، بخلاف زمن قرار التوجيه المحلي.",
                )}
              >
                <div className="sr-table-scroll">
                  <table className="sr-table">
                    <thead>
                      <tr>
                        {[
                          c("尝试", "Attempt", "المحاولة"),
                          c("资源", "Resource", "المورد"),
                          c("路径", "Path", "المسار"),
                          c("执行耗时", "Execution time", "زمن التنفيذ"),
                          c("结果", "Result", "النتيجة"),
                          c("原因", "Reason", "السبب"),
                        ].map((label) => (
                          <th key={label}>{label}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {getRoutingAttempts(selectedRequest).map((attempt) => (
                        <tr key={attempt.number}>
                          <td className="sr-number">
                            {String(attempt.number).padStart(2, "0")}
                          </td>
                          <td>
                            <strong>
                              {
                                routingCandidates.find(
                                  (candidate) =>
                                    candidate.id === attempt.candidateId,
                                )?.name
                              }
                            </strong>
                            <div className="sr-attempt-resource-id">
                              {attempt.candidateId}
                            </div>
                          </td>
                          <td>
                            {attempt.stage === "smart"
                              ? c("智能路由", "Smart routing", "توجيه ذكي")
                              : attempt.stage === "fallback"
                                ? c("回退路径", "Fallback path", "مسار رجوع")
                                : c(
                                    "默认路径",
                                    "Default path",
                                    "المسار الافتراضي",
                                  )}
                          </td>
                          <td className="sr-number">
                            {attempt.durationMs.toLocaleString()}{" "}
                            {c("毫秒", "ms", "مللي ثانية")}
                          </td>
                          <td>
                            <Pill
                              tone={
                                attempt.status === "success" ? "green" : "amber"
                              }
                            >
                              {attempt.status === "success"
                                ? c("成功", "Succeeded", "نجاح")
                                : c("失败", "Failed", "فشل")}
                            </Pill>
                          </td>
                          <td>
                            {attempt.error === "rate_limited"
                              ? c(
                                  "429 · 上游限流",
                                  "429 · Upstream rate limit",
                                  "429 · تحديد معدل المزود",
                                )
                              : attempt.error === "timeout"
                                ? c(
                                    "首字超时 · 重试未恢复",
                                    "First-token timeout on retry",
                                    "انتهاء مهلة أول رمز عند إعادة المحاولة",
                                  )
                                : c(
                                    "正常完成",
                                    "Completed normally",
                                    "اكتمل بصورة طبيعية",
                                  )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </Panel>
              <div className="sr-trace-grid">
                <Panel
                  title={c("决策路径", "Decision path", "مسار القرار")}
                  description={c(
                    "此记录的完整决策过程，历史数据不随配置修改。",
                    "This record’s decision path remains fixed when configuration changes.",
                    "يبقى مسار هذا القرار ثابتًا عند تغيير الإعدادات.",
                  )}
                >
                  <div className="sr-timeline">
                    {[
                      {
                        title: c(
                          "接收请求",
                          "Request received",
                          "استلام الطلب",
                        ),
                        description: `${selectedRequest.model} · ${selectedRequest.time}`,
                        state: "done",
                      },
                      {
                        title: c(
                          "灰度与快照检查",
                          "Rollout & snapshot checks",
                          "فحوص النشر واللقطة",
                        ),
                        description:
                          selectedRequest.outcome === "bypassed"
                            ? c(
                                "未命中灰度分组，采用默认路由。",
                                "Not assigned to the rollout; default routing selected.",
                                "خارج مجموعة النشر؛ اختير التوجيه الافتراضي.",
                              )
                            : selectedRequest.outcome === "pre-fallback"
                              ? c(
                                  "首选快照候选已失效，在智能尝试前进入回退。",
                                  "Preferred snapshot candidate expired; fallback before smart attempts.",
                                  "انتهت صلاحية المورد المفضل؛ رجوع قبل المحاولة الذكية.",
                                )
                              : c(
                                  "命中灰度分组，快照与能力检查通过。",
                                  "Rollout matched; snapshot and capability checks passed.",
                                  "مطابقة مجموعة النشر واجتياز فحوص اللقطة والقدرات.",
                                ),
                        state:
                          selectedRequest.outcome === "smart"
                            ? "done"
                            : "notice",
                      },
                      {
                        title:
                          selectedRequest.outcome === "smart"
                            ? c(
                                "选择最优健康候选",
                                "Select healthy candidate",
                                "اختيار مورد سليم",
                              )
                            : c(
                                "执行路由尝试",
                                "Execute routing attempts",
                                "تنفيذ محاولات التوجيه",
                              ),
                        description:
                          selectedRequest.outcome === "post-fallback"
                            ? c(
                                "两次智能尝试失败后，第三次使用回退资源并成功。",
                                "Two smart attempts failed. A third fallback attempt succeeded.",
                                "فشلت محاولتان ذكيتان. نجحت محاولة رجوع ثالثة.",
                              )
                            : `${selectedRequest.candidateId} · ${c("已选中资源", "Selected resource", "المورد المختار")}`,
                        state:
                          selectedRequest.outcome === "post-fallback"
                            ? "notice"
                            : "done",
                      },
                      {
                        title: c(
                          "完成与记录",
                          "Complete & record",
                          "إكمال وتسجيل",
                        ),
                        description: c(
                          "结果成功，异步写入请求、决策与尝试事件。",
                          "Success recorded asynchronously as request, decision and attempt events.",
                          "سُجل النجاح بشكل غير متزامن كأحداث طلب وقرار ومحاولة.",
                        ),
                        state: "done",
                      },
                    ].map((step, index) => (
                      <div
                        className={`sr-timeline-step is-${step.state}`}
                        key={step.title}
                      >
                        <span>
                          {step.state === "notice" ? (
                            <GitBranch size={15} />
                          ) : (
                            <Check size={15} />
                          )}
                        </span>
                        <div>
                          <small>0{index + 1}</small>
                          <h3>{step.title}</h3>
                          <p>{step.description}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                </Panel>
                <Panel
                  title={c(
                    "候选过滤与排序",
                    "Candidate filtering & ranking",
                    "تصفية وترتيب الموارد",
                  )}
                  description={c(
                    "仅展示该请求模型对应的资源。",
                    "Resources matching this request’s model only.",
                    "الموارد المطابقة لنموذج هذا الطلب فقط.",
                  )}
                >
                  <div className="sr-ranked-list">
                    {routingCandidates
                      .filter((row) => row.model === selectedRequest.model)
                      .sort((a, b) =>
                        a.id === selectedRequest.candidateId
                          ? -1
                          : b.id === selectedRequest.candidateId
                            ? 1
                            : b.score - a.score,
                      )
                      .map((candidate, index) => (
                        <button
                          key={candidate.id}
                          onClick={() => setCandidateDetail(candidate)}
                        >
                          <span
                            className={`sr-rank ${candidate.id === selectedRequest.candidateId ? "is-selected" : ""}`}
                          >
                            {index + 1}
                          </span>
                          <div>
                            <strong>{candidate.name}</strong>
                            <small>{candidate.id}</small>
                          </div>
                          <Pill
                            tone={
                              candidate.id === selectedRequest.candidateId
                                ? "blue"
                                : candidate.status === "eligible"
                                  ? "green"
                                  : candidate.status === "degraded"
                                    ? "amber"
                                    : "red"
                            }
                          >
                            {candidate.id === selectedRequest.candidateId
                              ? c("最终选中", "Selected", "مختار")
                              : statusLabel(candidate.status, c)}
                          </Pill>
                          <ChevronRight size={15} />
                        </button>
                      ))}
                  </div>
                  <p className="sr-panel-footnote">
                    {selectedRequest.outcome === "bypassed"
                      ? c(
                          "此次请求走默认路由；此处展示资源上下文，不代表进行了智能评分。",
                          "Default routing was used. Resource context does not imply smart ranking.",
                          "استُخدم التوجيه الافتراضي. سياق الموارد لا يعني إجراء ترتيب ذكي.",
                        )
                      : c(
                          "先执行硬过滤，再按请求画像与当时策略排序。",
                          "Hard filters run before request-profile and policy ranking.",
                          "تُنفذ التصفية الصارمة قبل الترتيب حسب الطلب والسياسة.",
                        )}
                  </p>
                </Panel>
              </div>
            </>
          )}
        </div>
      )}

      {view === "release" && (
        <div className="sr-view">
          <section
            className={`sr-release-banner ${config.experimentRunning && config.enabled ? "" : "is-paused"}`}
          >
            <span className="sr-release-icon">
              {config.experimentRunning && config.enabled ? (
                <GitBranch size={24} />
              ) : (
                <Pause size={24} />
              )}
            </span>
            <div>
              <h2>
                {!config.enabled
                  ? c(
                      "路由总开关已关闭",
                      "Master routing is disabled",
                      "التوجيه الرئيسي متوقف",
                    )
                  : config.experimentRunning
                    ? c(
                        "灰度实验运行中",
                        "Rollout experiment running",
                        "تجربة النشر تعمل",
                      )
                    : c(
                        "灰度实验已暂停",
                        "Rollout experiment paused",
                        "تجربة النشر متوقفة",
                      )}
              </h2>
              <p>
                {c(
                  "流量比例不会自动扩大；所有变更只作用于本地演示。",
                  "Traffic never expands automatically. Changes affect the local demo only.",
                  "لا تتوسع الحركة تلقائيًا. التغييرات للعرض المحلي فقط.",
                )}
              </p>
            </div>
            <Button
              disabled={!config.enabled}
              onClick={() =>
                config.experimentRunning
                  ? setConfirmation("stop")
                  : commit({ ...config, experimentRunning: true })
              }
            >
              {config.experimentRunning ? (
                <Pause size={14} />
              ) : (
                <Play size={14} />
              )}{" "}
              {config.experimentRunning
                ? c("暂停实验", "Pause experiment", "إيقاف التجربة")
                : c("恢复实验", "Resume experiment", "استئناف التجربة")}
            </Button>
          </section>
          <Panel
            title={c("发布阶段", "Rollout phases", "مراحل النشر")}
            description={c(
              "按租户与模型稳定分组，逐步扩大验证范围。",
              "Stable tenant and model assignment, expanded in deliberate steps.",
              "توزيع ثابت حسب المستأجر والنموذج وتوسّع تدريجي مدروس.",
            )}
            action={
              <Button size="small" onClick={openConfig}>
                <SlidersHorizontal size={14} />
                {c("调整配置", "Edit configuration", "تعديل الإعدادات")}
              </Button>
            }
          >
            <div className="sr-phases">
              {phaseTraffic.map((traffic, index) => (
                <div
                  className={`${index === config.phase ? "is-current" : ""} ${index < config.phase ? "is-complete" : ""}`}
                  key={index}
                >
                  <span>
                    {index < config.phase ? <Check size={16} /> : index}
                  </span>
                  <strong>
                    {index === 0
                      ? c("内部验证", "Internal", "داخلي")
                      : `${traffic}%`}
                  </strong>
                  <small>
                    {index === config.phase
                      ? c("当前阶段", "Current phase", "المرحلة الحالية")
                      : index < config.phase
                        ? c("已通过", "Completed", "مكتمل")
                        : c("待验证", "Upcoming", "قادم")}
                  </small>
                </div>
              ))}
            </div>
            <div className="sr-release-controls">
              <div>
                <span>{c("当前演示流量", "Demo traffic", "حركة تجريبية")}</span>
                <strong>{config.traffic}%</strong>
              </div>
              <div>
                <span>{c("分配状态", "Assignment", "حالة التوزيع")}</span>
                <strong>
                  {config.enabled && config.experimentRunning
                    ? c(
                        "按当前配置分配",
                        "Current policy active",
                        "السياسة الحالية نشطة",
                      )
                    : c("已暂停分配", "Assignment paused", "التوزيع متوقف")}
                </strong>
              </div>
              <Button
                size="small"
                disabled={config.phase === 0}
                onClick={() => setConfirmation("rollback")}
              >
                <RotateCcw size={14} />
                {c("回退上一阶段", "Previous phase", "المرحلة السابقة")}
              </Button>
            </div>
          </Panel>
          <div className="sr-rollout-grid">
            <Panel
              title={c(
                "基线与发布记录",
                "Baseline & release notes",
                "خط الأساس وسجل النشر",
              )}
            >
              <dl className="sr-stacked-details">
                {[
                  [
                    c("实验编号", "Experiment ID", "معرّف التجربة"),
                    "exp-veyra-route-0924",
                  ],
                  [
                    c("冻结基线", "Frozen baseline", "خط الأساس الثابت"),
                    "baseline-2026w38",
                  ],
                  [
                    c("样本分层", "Sample strata", "تقسيم العينات"),
                    c(
                      "模型 + 服务等级 + 字元分组",
                      "Model + tier + token group",
                      "النموذج + المستوى + مجموعة الرموز",
                    ),
                  ],
                  [
                    c("审批状态", "Approval status", "حالة الموافقة"),
                    c(
                      "本地演示，不连接审批系统",
                      "Local demo; no approval integration",
                      "عرض محلي دون تكامل موافقات",
                    ),
                  ],
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            </Panel>
            <Panel
              title={c(
                "当前路由策略",
                "Current routing policy",
                "سياسة التوجيه الحالية",
              )}
            >
              <dl className="sr-stacked-details">
                {[
                  [
                    c("优先目标", "Priority", "الأولوية"),
                    config.strategy === "balanced"
                      ? c(
                          "性能与成本平衡",
                          "Balanced performance & cost",
                          "توازن الأداء والتكلفة",
                        )
                      : config.strategy === "cost"
                        ? c("成本优先", "Cost first", "التكلفة أولًا")
                        : c("延迟优先", "Latency first", "الاستجابة أولًا"),
                  ],
                  [
                    c("失败回退", "Fallback", "الرجوع"),
                    config.fallback
                      ? c("已启用", "Enabled", "مفعّل")
                      : c("已关闭", "Disabled", "متوقف"),
                  ],
                  [
                    c(
                      "最多智能尝试",
                      "Max smart attempts",
                      "أقصى محاولات ذكية",
                    ),
                    String(config.maxAttempts),
                  ],
                  [
                    c("首字超时", "First-token timeout", "مهلة أول رمز"),
                    `${config.timeoutMs} ms${c("（毫秒）", "", "")}`,
                  ],
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            </Panel>
          </div>
          <Panel
            title={c(
              "止损条件与观察证据",
              "Guardrails & evidence",
              "حدود الأمان والأدلة",
            )}
            description={c(
              "以下为演示规则；样本不足时暂停扩量，不自动触发真实操作。",
              "Demo rules only. Insufficient samples pause expansion; no production actions are triggered.",
              "قواعد تجريبية فقط. نقص العينات يوقف التوسعة دون عمليات إنتاجية.",
            )}
          >
            <div className="sr-table-scroll">
              <table className="sr-table">
                <thead>
                  <tr>
                    {[
                      c("观察指标", "Metric", "المقياس"),
                      c("止损条件", "Stop condition", "شرط الإيقاف"),
                      c("样例窗口", "Sample window", "نافذة العينة"),
                      c("判断", "Assessment", "التقييم"),
                    ].map((label) => (
                      <th key={label}>{label}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {[
                    {
                      name: c("最终成功率", "Customer success", "نجاح الطلبات"),
                      rule: c(
                        "连续两个窗口低于基线 1 个百分点",
                        "Two windows 1 percentage point below baseline",
                        "نافذتان أقل من الأساس بنقطة مئوية",
                      ),
                      value: `${successRate.toFixed(2)}%`,
                    },
                    {
                      name: c("总回退率", "Total fallback", "إجمالي الرجوع"),
                      rule: c(
                        "单窗口 ≥ 5%，且至少 100 次请求",
                        "Window ≥ 5%, with at least 100 requests",
                        "نافذة ≥ 5% مع 100 طلب على الأقل",
                      ),
                      value: "1.24%",
                    },
                    {
                      name: c(
                        "P95（95分位）路由耗时",
                        "P95 route time",
                        "زمن التوجيه P95",
                      ),
                      rule: c(
                        "连续三个窗口 > 50 毫秒",
                        "Three consecutive windows > 50 ms",
                        "ثلاث نوافذ متتالية > 50 مللي ثانية",
                      ),
                      value: "22 ms",
                    },
                    {
                      name: c(
                        "快照一致性",
                        "Snapshot consistency",
                        "اتساق اللقطات",
                      ),
                      rule: c(
                        "任一实例版本或摘要不同",
                        "Any instance version or hash mismatch",
                        "اختلاف نسخة أو تجزئة أي مثيل",
                      ),
                      value: "8 / 8",
                    },
                    {
                      name: c("相对成本", "Relative cost", "التكلفة النسبية"),
                      rule: c(
                        "高于基线 10% 持续 30 分钟",
                        "10% above baseline for 30 minutes",
                        "أعلى من الأساس بـ 10% لمدة 30 دقيقة",
                      ),
                      value: "−12.5%",
                    },
                  ].map((row) => (
                    <tr key={row.name}>
                      <td>
                        <strong>{row.name}</strong>
                      </td>
                      <td className="sr-wrap-cell">{row.rule}</td>
                      <td className="sr-number">{row.value}</td>
                      <td>
                        <Pill tone="green">
                          {c("未触发", "Not triggered", "لم يُفعّل")}
                        </Pill>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>
      )}

      {view === "configuration" && (
        <section className="sr-config-page" aria-labelledby="sr-config-title">
          <header className="sr-config-heading">
            <div>
              <span className="sr-config-eyebrow">
                {c("配置工作区", "CONFIGURATION", "مساحة الإعداد")}
              </span>
              <h2 id="sr-config-title">
                {c(
                  "为你的流量设定路由规则",
                  "Set the rules for your traffic",
                  "حدد قواعد توجيه الحركة",
                )}
              </h2>
              <p>
                {c(
                  "选择目标、设置灰度流量，再定义失败时的保护措施。保存后应用到本地演示。",
                  "Choose an objective, set rollout traffic, and define failure protection. Save to apply to the local demo.",
                  "اختر الهدف وحدد حركة النشر وضوابط الحماية. احفظ لتطبيق الإعداد على العرض المحلي.",
                )}
              </p>
            </div>
            <Pill tone={dirty ? "amber" : "muted"}>
              {dirty
                ? c("有未保存的更改", "Unsaved changes", "تغييرات غير محفوظة")
                : c(
                    "与已保存配置一致",
                    "Matches saved settings",
                    "مطابق للإعداد المحفوظ",
                  )}
            </Pill>
          </header>
          <form
            className="sr-config-form"
            onSubmit={(event) => {
              event.preventDefault();
              saveConfig();
            }}
            noValidate
          >
            <section
              className="sr-config-section sr-config-objective"
              aria-labelledby="sr-objective-title"
            >
              <header className="sr-config-section-heading">
                <span>01</span>
                <div>
                  <h3 id="sr-objective-title">
                    {c("目标策略", "Routing objective", "هدف التوجيه")}
                  </h3>
                  <p>
                    {c(
                      "决定健康候选资源之间的优先顺序。",
                      "Set the priority among healthy candidates.",
                      "حدد الأولوية بين الموارد السليمة.",
                    )}
                  </p>
                </div>
                <Route size={21} />
              </header>
              <div className="sr-policy-options">
                {(["balanced", "cost", "latency"] as const).map((value) => (
                  <label
                    className={`sr-policy-option ${draft.strategy === value ? "is-selected" : ""}`}
                    key={value}
                  >
                    <input
                      type="radio"
                      name="sr-strategy"
                      value={value}
                      checked={draft.strategy === value}
                      onChange={() => setDraft({ ...draft, strategy: value })}
                    />
                    <span>
                      <strong>
                        {value === "balanced"
                          ? c("平衡模式", "Balanced", "متوازن")
                          : value === "cost"
                            ? c("成本优先", "Cost first", "التكلفة أولًا")
                            : c("延迟优先", "Latency first", "الاستجابة أولًا")}
                      </strong>
                      <small>
                        {value === "balanced"
                          ? c(
                              "综合考虑健康、延迟与成本。",
                              "Balance health, latency and cost.",
                              "وازن بين الصحة والاستجابة والتكلفة.",
                            )
                          : value === "cost"
                            ? c(
                                "在满足服务等级的候选中优先控制成本。",
                                "Minimize cost among candidates meeting the service tier.",
                                "قلل التكلفة بين الموارد التي تحقق مستوى الخدمة.",
                              )
                            : c(
                                "优先选择响应更快的健康资源。",
                                "Prefer healthy resources with faster responses.",
                                "فضّل الموارد السليمة ذات الاستجابة الأسرع.",
                              )}
                      </small>
                    </span>
                  </label>
                ))}
              </div>
            </section>
            <section
              className="sr-config-section"
              aria-labelledby="sr-traffic-title"
            >
              <header className="sr-config-section-heading">
                <span>02</span>
                <div>
                  <h3 id="sr-traffic-title">
                    {c("流量分配", "Traffic allocation", "توزيع الحركة")}
                  </h3>
                  <p>
                    {c(
                      "控制有多少演示请求使用智能路由。",
                      "Control the share of demo requests using smart routing.",
                      "تحكم في نسبة الطلبات التجريبية التي تستخدم التوجيه الذكي.",
                    )}
                  </p>
                </div>
                <GitBranch size={21} />
              </header>
              <label className="sr-form-field">
                <span>{c("发布阶段", "Release phase", "مرحلة النشر")}</span>
                <select
                  value={draft.phase}
                  onChange={(event) => {
                    const phase = Number(event.target.value);
                    setDraft({ ...draft, phase, traffic: phaseTraffic[phase] });
                  }}
                >
                  {phaseTraffic.map((traffic, index) => (
                    <option key={index} value={index}>
                      {c(
                        `G${index}（第 ${index} 阶段） · ${index === 0 ? "内部验证" : `${traffic}% 流量`}`,
                        `G${index} · ${index === 0 ? "Internal" : `${traffic}% traffic`}`,
                        `G${index} · ${index === 0 ? "داخلي" : `${traffic}% حركة`}`,
                      )}
                    </option>
                  ))}
                </select>
              </label>
              <label className="sr-form-field">
                <span>
                  {c(
                    "演示流量比例",
                    "Demo traffic share",
                    "نسبة الحركة التجريبية",
                  )}
                  <strong>{draft.traffic}%</strong>
                </span>
                <input
                  type="range"
                  min="0"
                  max="100"
                  step="1"
                  value={draft.traffic}
                  onChange={(event) =>
                    setDraft({ ...draft, traffic: Number(event.target.value) })
                  }
                />
                <small>
                  {c(
                    "自定义比例只影响演示配置，不会改变历史统计。",
                    "Custom shares affect configuration only, not historical statistics.",
                    "النسبة المخصصة تؤثر على الإعداد فقط وليس الإحصاءات السابقة.",
                  )}
                </small>
              </label>
            </section>
            <section
              className="sr-config-section"
              aria-labelledby="sr-protection-title"
            >
              <header className="sr-config-section-heading">
                <span>03</span>
                <div>
                  <h3 id="sr-protection-title">
                    {c("失败防护", "Failure protection", "الحماية من الإخفاق")}
                  </h3>
                  <p>
                    {c(
                      "限制等待时间，并为失败请求设置后续路径。",
                      "Limit waiting time and set the path after a failed attempt.",
                      "حدد وقت الانتظار والمسار التالي للمحاولة غير الناجحة.",
                    )}
                  </p>
                </div>
                <ShieldCheck size={21} />
              </header>
              <div className="sr-switch-row">
                <div>
                  <h3>
                    {c("启用失败回退", "Enable fallback", "تفعيل الرجوع")}
                  </h3>
                  <p>
                    {c(
                      "智能候选失败时允许沿备用路径继续尝试。",
                      "Allow a fallback path if smart candidates fail.",
                      "اسمح بمسار بديل إذا أخفقت الموارد الذكية.",
                    )}
                  </p>
                </div>
                <Switch
                  id="sr-fallback"
                  label={c("启用失败回退", "Enable fallback", "تفعيل الرجوع")}
                  checked={draft.fallback}
                  onChange={(fallback) => setDraft({ ...draft, fallback })}
                />
              </div>
              <div className="sr-form-grid">
                <label className="sr-form-field">
                  <span>
                    {c(
                      "最多智能尝试",
                      "Max smart attempts",
                      "أقصى محاولات ذكية",
                    )}
                  </span>
                  <input
                    type="number"
                    min="1"
                    max="3"
                    value={draft.maxAttempts}
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        maxAttempts: Number(event.target.value),
                      })
                    }
                  />
                </label>
                <label className="sr-form-field">
                  <span>
                    {c(
                      "首字超时（毫秒）",
                      "First-token timeout (ms)",
                      "مهلة أول رمز (مللي ثانية)",
                    )}
                  </span>
                  <input
                    type="number"
                    min="500"
                    max="10000"
                    step="100"
                    value={draft.timeoutMs}
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        timeoutMs: Number(event.target.value),
                      })
                    }
                  />
                </label>
              </div>
            </section>
            {configError && (
              <p className="sr-form-error" role="alert">
                {configError}
              </p>
            )}
            <button
              type="button"
              className="sr-text-button"
              onClick={() => {
                setDraft({
                  ...defaultRoutingConfig,
                  enabled: config.enabled,
                  experimentRunning: config.experimentRunning,
                });
                setConfigError("");
              }}
            >
              <RotateCcw size={14} />
              {c(
                "恢复默认值（保存后生效）",
                "Reset defaults (save to apply)",
                "استعادة الافتراضي (احفظ للتطبيق)",
              )}
            </button>
            <footer className="sr-config-actions">
              <div role="status">
                {dirty ? <CircleAlert size={17} /> : <ShieldCheck size={17} />}
                <div>
                  <strong>
                    {dirty
                      ? c(
                          "更改尚未应用",
                          "Changes are not applied yet",
                          "لم تُطبق التغييرات بعد",
                        )
                      : c(
                          "当前没有未保存的更改",
                          "No unsaved changes",
                          "لا تغييرات غير محفوظة",
                        )}
                  </strong>
                  <span>
                    {c(
                      "保存仅更新演示配置，历史指标和记录保持原样。",
                      "Saving updates demo settings; historical metrics and records stay unchanged.",
                      "يحدث الحفظ إعداد العرض وتبقى المقاييس والسجلات السابقة كما هي.",
                    )}
                  </span>
                </div>
              </div>
              <div className="sr-config-action-buttons">
                <Button
                  type="button"
                  onClick={() => requestExit({ view: returnView })}
                >
                  {c("取消编辑", "Cancel editing", "إلغاء التعديل")}
                </Button>
                <Button variant="primary" type="submit" disabled={!dirty}>
                  <Check size={15} />
                  {c("保存配置", "Save configuration", "حفظ الإعداد")}
                </Button>
              </div>
            </footer>
          </form>
        </section>
      )}
      <Modal
        size="compact"
        preventOutsideClose
        open={!!confirmation}
        onOpenChange={(open) => !open && setConfirmation(null)}
        title={
          confirmation === "discard"
            ? c(
                "放弃未保存的更改？",
                "Discard unsaved changes?",
                "تجاهل التغييرات غير المحفوظة؟",
              )
            : confirmation === "disable"
              ? c(
                  "关闭智能路由？",
                  "Disable smart routing?",
                  "إيقاف التوجيه الذكي؟",
                )
              : confirmation === "rollback"
                ? c(
                    "回退发布阶段？",
                    "Revert to the previous phase?",
                    "العودة إلى المرحلة السابقة؟",
                  )
                : c(
                    "暂停灰度实验？",
                    "Pause the rollout experiment?",
                    "إيقاف تجربة النشر؟",
                  )
        }
        description={
          confirmation === "discard"
            ? c(
                "离开后恢复到上一次保存的路由配置。",
                "Leaving restores the last saved routing configuration.",
                "تعيد المغادرة آخر إعداد توجيه محفوظ.",
              )
            : c(
                "这是本地演示操作，不会修改线上服务。",
                "This changes the local demo only, not any online service.",
                "يؤثر هذا على العرض المحلي فقط دون أي خدمة عبر الإنترنت.",
              )
        }
        footer={
          <>
            <Button onClick={() => setConfirmation(null)}>
              {confirmation === "discard"
                ? c("继续编辑", "Keep editing", "متابعة التعديل")
                : c("取消", "Cancel", "إلغاء")}
            </Button>
            <Button variant="primary" onClick={performConfirmation}>
              {confirmation === "discard"
                ? c("放弃并离开", "Discard and leave", "تجاهل وغادر")
                : c("确认更改", "Confirm change", "تأكيد التغيير")}
            </Button>
          </>
        }
      >
        <div className="sr-routing sr-confirm-content">
          <CircleAlert size={32} />
          <p>
            {confirmation === "discard"
              ? c(
                  "本次目标策略、流量比例和防护设置的修改将被丢弃，已保存配置不会改变。",
                  "Your edits to the objective, traffic share and protection settings will be discarded. Saved settings remain unchanged.",
                  "ستُتجاهل تعديلات الهدف ونسبة الحركة والحماية. لن تتغير الإعدادات المحفوظة.",
                )
              : confirmation === "disable"
                ? c(
                    "总开关关闭后，演示将标记为默认路由；仍可查看历史决策并重新启用。",
                    "The demo will show default routing. Historical decisions remain accessible and routing can be re-enabled.",
                    "سيعرض النموذج التوجيه الافتراضي. تبقى القرارات السابقة متاحة ويمكن إعادة التفعيل.",
                  )
                : confirmation === "rollback"
                  ? c(
                      "将回到上一阶段的默认流量比例，并暂停实验，等待重新确认。",
                      "Traffic returns to the previous phase’s default share and the experiment pauses for review.",
                      "تعود الحركة إلى النسبة الافتراضية للمرحلة السابقة وتتوقف التجربة للمراجعة.",
                    )
                  : c(
                      "保留当前配置与历史数据，暂停演示流量分配。你可以随时恢复。",
                      "Keep configuration and historical records while pausing demo allocation. Resume anytime.",
                      "احفظ الإعداد والسجلات مع إيقاف التوزيع التجريبي. استأنف في أي وقت.",
                    )}
          </p>
        </div>
      </Modal>
      <Drawer
        open={!!candidateDetail}
        onOpenChange={(open) => !open && setCandidateDetail(null)}
        title={c("候选决策解释", "Candidate explanation", "تفسير قرار المورد")}
        description={c(
          "健康、能力与成本信息来自本地样例快照。",
          "Health, capability and cost are from the local sample snapshot.",
          "الصحة والقدرات والتكلفة من لقطة تجريبية محلية.",
        )}
      >
        <div className="sr-routing sr-drawer-content">
          {candidateDetail && (
            <>
              <div className="sr-candidate-drawer-heading">
                <span className="sr-health-icon">
                  <Server size={24} />
                </span>
                <h3>{candidateDetail.name}</h3>
                <span className="sr-code">{candidateDetail.id}</span>
                <Pill
                  tone={
                    candidateDetail.status === "eligible"
                      ? "green"
                      : candidateDetail.status === "degraded"
                        ? "amber"
                        : "red"
                  }
                >
                  {statusLabel(candidateDetail.status, c)}
                </Pill>
              </div>
              <div className="sr-explanation">
                <ShieldCheck size={19} />
                <p>
                  {candidateDetail.reason[
                    i18n.language as "zh" | "en" | "ar"
                  ] || candidateDetail.reason.en}
                </p>
              </div>
              <dl className="sr-stacked-details">
                {[
                  [
                    c("模型映射", "Model mapping", "تطابق النموذج"),
                    candidateDetail.model,
                  ],
                  [
                    c("首字延迟", "First-token latency", "زمن أول رمز"),
                    `${candidateDetail.ttft} ms${c("（毫秒）", "", "")}`,
                  ],
                  [
                    c("综合得分", "Overall score", "النتيجة الإجمالية"),
                    `${candidateDetail.score} / 100`,
                  ],
                  [
                    c("错误率", "Error rate", "معدل الأخطاء"),
                    `${candidateDetail.errorRate}%`,
                  ],
                  [
                    c("容量使用率", "Capacity used", "السعة المستخدمة"),
                    `${candidateDetail.capacity}%`,
                  ],
                  [
                    c(
                      "输入 / 输出成本",
                      "Input / output cost",
                      "تكلفة الإدخال / الإخراج",
                    ),
                    `$${candidateDetail.inputPrice} / $${candidateDetail.outputPrice}`,
                  ],
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
              <p className="sr-panel-footnote">
                {c(
                  "成本单位：美元 / 百万字元。熔断资源不参与当前候选选择。",
                  "Cost: USD per million tokens. Open-circuit resources are excluded from selection.",
                  "التكلفة: دولار لكل مليون رمز. الموارد ذات القاطع المفتوح مستبعدة.",
                )}
              </p>
            </>
          )}
        </div>
      </Drawer>
      <Drawer
        open={!!detailSnapshot}
        onOpenChange={(open) => !open && setInspectSnapshot(null)}
        title={c("快照版本详情", "Snapshot details", "تفاصيل اللقطة")}
        description={c(
          "历史样例版本，可设置为本地演示当前快照。",
          "A historical sample version that can be activated in the local demo.",
          "نسخة تاريخية تجريبية يمكن تفعيلها في العرض المحلي.",
        )}
        footer={
          detailSnapshot && (
            <Button
              variant="primary"
              disabled={detailSnapshot.id === snapshotId}
              onClick={() => {
                setSnapshotId(detailSnapshot.id);
                setInspectSnapshot(null);
                setValidated(false);
                toast.success(
                  c(
                    "已切换本地演示快照",
                    "Local demo snapshot switched",
                    "بُدّلت اللقطة التجريبية المحلية",
                  ),
                );
              }}
            >
              {detailSnapshot.id === snapshotId
                ? c("当前已启用", "Already active", "نشطة بالفعل")
                : c(
                    "使用此演示快照",
                    "Use this demo snapshot",
                    "استخدام هذه اللقطة التجريبية",
                  )}
            </Button>
          )
        }
      >
        <div className="sr-routing sr-drawer-content">
          {detailSnapshot && (
            <dl className="sr-stacked-details">
              {[
                [
                  c("快照编号", "Snapshot ID", "معرّف اللقطة"),
                  detailSnapshot.id,
                ],
                [
                  c("规则版本", "Rule version", "نسخة القواعد"),
                  detailSnapshot.rules,
                ],
                [
                  c("发布时间", "Published", "وقت النشر"),
                  detailSnapshot.publishedAt,
                ],
                [
                  c("完整性摘要", "Integrity hash", "تجزئة السلامة"),
                  detailSnapshot.checksum,
                ],
                [
                  c("数据契约", "Data contract", "عقد البيانات"),
                  "routing-contract-v1.3",
                ],
                [
                  c("实例一致性", "Instance consistency", "اتساق المثيلات"),
                  `${detailSnapshot.instances}/${detailSnapshot.totalInstances}`,
                ],
              ].map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
            </dl>
          )}
        </div>
      </Drawer>
      <footer className="sr-footer">
        <span>
          {c(
            "演示数据 · 2026-09-24",
            "Demo dataset · 2026-09-24",
            "بيانات تجريبية · 2026-09-24",
          )}
        </span>
        <span>
          {c(
            "所有配置仅在本地生效",
            "All configuration is local",
            "كل الإعدادات محلية",
          )}
        </span>
      </footer>
    </div>
  );
}
