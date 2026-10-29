import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import BillingCenter from "../features/billing-center";
import AccountCenter from "../features/account-center";
import KeyList, {
  KeyDetails,
  useKeyDisplayTime,
  type KeySecretAction,
} from "../features/key-list";
import { keyQuotaSummary } from "../lib/key-quota-summary";
import RechargeFlow from "../features/recharge-flow";
import { useTranslation } from "react-i18next";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as ChartTooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  Activity,
  ArrowRight,
  Check,
  CircleAlert,
  CircleCheck,
  CircleDollarSign,
  Copy,
  Network,
  KeyRound,
  LayoutDashboard,
  Plus,
  Terminal,
  Wallet,
  BarChart3,
  Download,
  Settings,
  ShieldCheck,
  Sparkles,
  AlertTriangle,
  ArrowUpRight,
  ChevronRight,
  Menu,
  X,
  BookOpen,
  MoreHorizontal,
  Zap,
  CreditCard,
  Boxes,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Drawer, Modal, ConfirmDialog } from "../components/ui/overlay";
import { Input } from "../components/ui/primitives";
import {
  Preferences,
  ProductBrand,
  ProductSearch,
  Quickstart,
} from "../components/product-shell";
import { copyText } from "../features/code-example";
import { formatMoney } from "../lib/money";
import {
  useProductPreview,
  periodRequests,
  summarizeRequests,
  breakdownByModel,
  activityFor,
  type PreviewKey,
  type PreviewRequest,
} from "../lib/product-preview";
import { modelCatalog } from "../lib/platform-catalog";
import "../components/console-redesign.css";
import "../components/key-task.css";
import { useModelPricing } from "../lib/model-pricing";
import { placeholders, type TierId } from "../config/placeholders";
import EditKey from "../features/edit-key";
import CreateKey from "../features/create-key";
import Playground from "../features/request-playground";
import AccessCenter from "../features/access-center";
import { isSmartModelId } from "../lib/smart-model";
import RequestDetails from "../features/request-inspector";
import RequestHistory from "../features/request-history";
import ServiceQuality from "../features/service-quality";
import SmartRouting from "../features/resource-routing";
import { StandardModels } from "../features/standard-models";
import { Channels } from "../features/channels";
import { ServiceGroups } from "../features/service-groups";
import { useSystemAccess, isAdmin, canReadFinance, sessionUsername, sessionQueryIdentity, useSessionQueryIdentity } from "../lib/system-access";
import { useBackendKeys } from "../lib/api/key-adapter";
import {
  useDeleteToken,
  useRegenerateToken,
  useUpdateToken,
  verifyTokenPassword,
} from "../lib/api/tokens";
import { useWallet } from "../lib/api/billing";
import {
  useExports,
  useCreateExport,
  downloadExport as downloadBackendExport,
  type ExportRecord,
} from "../lib/api/exports";
import { ApiError } from "../lib/api/http";
import { deactivateSelf } from "../lib/api/user";
import {
  useUserUsage,
  useLogs,
  useBackendRequests,
  quotaToNanoUsd,
  resolveVendor,
  type LogRow,
} from "../lib/api/usage";
import { useAdminMetrics } from "../lib/api/admin";

const navigation = [
  { path: "", key: "pOverview", Icon: LayoutDashboard },
  { path: "models", key: "navModels", Icon: Boxes },
  { path: "keys", key: "pKeys", Icon: KeyRound },
  { path: "connect", key: "navConnect", Icon: BookOpen },
  { path: "playground", key: "pPlayground", Icon: Terminal },
  { path: "service-quality", key: "navServiceQuality", Icon: ShieldCheck },
  { path: "usage", key: "usageReportsTitle", Icon: BarChart3 },
  { path: "requests", key: "pUsage", Icon: Activity },
  { path: "billing", key: "billingTitle", Icon: Wallet },
  { path: "exports", key: "exportsTitle", Icon: Download },
  { path: "settings", key: "settingsTitle", Icon: Settings },
];

function ResultPill({ failed }: { failed: boolean }) {
  const { t } = useTranslation();
  return (
    <span className={`result-pill ${failed ? "is-failed" : ""}`}>
      {failed ? <CircleAlert size={13} /> : <Check size={13} />}{" "}
      {t(failed ? "pFailed" : "pSucceeded")}
    </span>
  );
}

export default function ConsolePreview() {
  const identity = useSessionQueryIdentity();
  return <ConsoleWorkspace key={JSON.stringify(identity)} />;
}

function ConsoleWorkspace() {
  const { t, i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const sidebarNav = useRef<HTMLElement>(null);
  const catalogModels = modelCatalog(useModelPricing((s) => s.config));
  const role = useSystemAccess((s) => s.role);
  const isSystemAdmin = isAdmin(role);
  const location = useLocation();
  const navigate = useNavigate();

  const section = location.pathname
    .replace(/^\/(?:preview\/)?console\/?/, "")
    .split("/")[0];
  useEffect(() => {
    const nav = sidebarNav.current;
    const current = nav?.querySelector<HTMLElement>('[aria-current="page"]');
    if (!nav || !current) return;
    const boundary = nav.getBoundingClientRect(),
      item = current.getBoundingClientRect();
    if (item.bottom > boundary.bottom)
      nav.scrollTop += item.bottom - boundary.bottom + 12;
    else if (item.top < boundary.top)
      nav.scrollTop -= boundary.top - item.top + 12;
  }, [section, role, mobileNavOpen]);

  const session = useSystemAccess((s) => s.session);
  const sessionScope = sessionQueryIdentity({session, role});
  const isCurrentSession = () => JSON.stringify(sessionScope) === JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()));
  const settingsLabel = c("账号中心", "Account center", "مركز الحساب");
  const profileName = sessionUsername(session) ?? settingsLabel;
  const profileInitials = Array.from(profileName.trim())
    .slice(0, 2)
    .join("")
    .toUpperCase();
  // Keys, balance, and overview metrics come from backend API endpoints.
  // The demo store still owns requests (for non-overview sections),
  // ledger, exports, and demo-mode UI state.
  // Plaintext keys captured from create/regenerate — the only time the
  // backend returns the unmasked key. Merged into the adapted PreviewKey
  // so the reveal/copy actions work for keys created in this session.
  const [plaintextKeys, setPlaintextKeys] = useState<Record<string, string>>(
    {},
  );
  const {
    keys,
    loading: tokensLoading,
    error: tokensError,
  } = useBackendKeys(plaintextKeys);
  const deleteTokenMut = useDeleteToken();
  const regenerateTokenMut = useRegenerateToken();
  const updateTokenMut = useUpdateToken();
  // One-time dialog for the regenerated plaintext key.
  const [regeneratedKey, setRegeneratedKey] = useState<{
    name: string;
    plaintext: string;
  } | null>(null);
  // Unknown wallet state never falls back to a browser demo balance.
  const walletQuery = useWallet();
  const balanceKnown = Boolean(walletQuery.data && !walletQuery.error);
  const balance = balanceKnown ? walletQuery.data!.balance_nano : "0";
  const requests = useProductPreview((s) => s.requests);
  // Exports come from the backend /api/export endpoints.
  const exportsQuery = useExports();
  const createExportMut = useCreateExport();
  const backendExports: ExportRecord[] = exportsQuery.data ?? [];
  // Overview metrics come from the backend usage summary and logs.
  const [days, setDays] = useState<"7" | "30">("7");
  const usageQuery = useUserUsage(days === "7" ? 7 : 30);
  const logsQuery = useLogs(1, 5);
  const backendLogs: LogRow[] = logsQuery.data?.items ?? [];
  // Backend logs for the requests page (larger page size).
  const { rows: backendRequests, refetch: refreshRequests } =
    useBackendRequests(100);
  // Admin metrics for the health card; gracefully degrades to null
  // when the current user is not an admin.
  const metricsQuery = useAdminMetrics();
  const chooseKey = useProductPreview((s) => s.chooseKey);
  // Onboarding completion is derived from real backend state: the
  // user has at least one key and a positive balance. A local
  // "dismissed" flag lets the user hide the banner for the session
  // without needing a backend field.
  const [onboardingDismissed, setOnboardingDismissed] = useState(false);
  const onboardingComplete =
    onboardingDismissed ||
    keys.length > 0 ||
    (balanceKnown && BigInt(balance) > 0n);
  // Unlock only the current owner's locally captured key material. Verification
  // does not create a new login session or recover old masked credentials.
  const [revealedKeyIds, setRevealedKeyIds] = useState<Record<string, number>>(
    {},
  );
  const verifyAndReveal = async (keyId: string, password: string): Promise<boolean> => {
    if (tokensError || !keys.some((key) => key.id === keyId)) return false;
    try {
      await verifyTokenPassword(Number(keyId), password, sessionScope);
      if (!isCurrentSession()) return false;
      setRevealedKeyIds((state) => ({...state, [keyId]: Date.now() + 300000}));
      return true;
    } catch {
      return false;
    }
  };
  const keyDisplayNow = useKeyDisplayTime(revealedKeyIds);
  const quotaAt = new Date(keyDisplayNow).toISOString();
  const keysNearQuota = keys.filter(
    (key) => keyQuotaSummary(key, quotaAt).nearLimit,
  );
  const deactivateAccount = async () => {
    try {
      await deactivateSelf();
    } catch {
      // Best-effort: proceed with local cleanup even if the server is unreachable.
    }
    // Hard redirect to clear all client-side state (React Query cache,
    // Zustand stores, sessionStorage drafts).
    window.location.href = "/";
  };

  const [chartMetric, setChartMetric] = useState<
    "requests" | "cost" | "latency"
  >("requests");
  const [selected, setSelected] = useState<PreviewRequest | null>(null);
  const previousTrace = useRef<string | null>(null);
  useEffect(() => {
    const trace = new URLSearchParams(location.search).get("trace");
    if (trace && section === "requests")
      setSelected(
        (backendRequests as any[]).find(
          (r) => r.trace_id === trace,
        ) as PreviewRequest | null,
      );
    else if (previousTrace.current) setSelected(null);
    previousTrace.current = section === "requests" ? trace : null;
  }, [location.search, section, backendRequests]);
  const [createOpen, setCreateOpen] = useState(false);
  const requestedModel = new URLSearchParams(location.search).get("model");
  const initialCreateModel =
    requestedModel &&
    (isSmartModelId(requestedModel) ||
      catalogModels.some((model) => model.id === requestedModel))
      ? requestedModel
      : undefined;
  const requestedGroup = new URLSearchParams(location.search).get("group");
  const initialCreateGroup =
    requestedGroup &&
    placeholders.tiers.some((tier) => tier.id === requestedGroup)
      ? (requestedGroup as TierId)
      : undefined;
  const creationFromUrl = useRef(false);
  const creationPath = useRef(location.pathname);
  useEffect(() => {
    const requested =
      section === "keys" &&
      new URLSearchParams(location.search).get("create") === "1";
    if (requested) setCreateOpen(true);
    else if (
      creationFromUrl.current ||
      creationPath.current !== location.pathname
    )
      setCreateOpen(false);
    creationFromUrl.current = requested;
    creationPath.current = location.pathname;
  }, [section, location.pathname, location.search]);
  const changeCreateOpen = (open: boolean) => {
    setCreateOpen(open);
    if (!open && new URLSearchParams(location.search).has("create")) {
      const params = new URLSearchParams(location.search);
      params.delete("create");
      params.delete("model");
      navigate(
        { pathname: location.pathname, search: params.toString() },
        { replace: true },
      );
    }
  };
  const [guideOpen, setGuideOpen] = useState(false);
  const isRechargePage = location.pathname.endsWith("/billing/recharge");
  const rechargeOrderId =
    new URLSearchParams(location.search).get("order") || undefined;
  const rechargeReturnTo =
    typeof location.state?.returnTo === "string" &&
    /^\/(?:preview\/)?console(?:\/|$|\?)/.test(location.state.returnTo) &&
    !location.state.returnTo.split(/[?#]/)[0].endsWith("/billing/recharge")
      ? location.state.returnTo
      : "/console/billing";
  const openRecharge = (orderId?: string) => {
    navigate(
      {
        pathname: "/console/billing/recharge",
        search: orderId ? `?order=${encodeURIComponent(orderId)}` : "",
      },
      {
        state: {
          returnTo: isRechargePage
            ? rechargeReturnTo
            : location.pathname + location.search,
        },
      },
    );
  };

  // Key reveal with password
  const [authRevealKeyId, setAuthRevealKeyId] = useState<string | null>(null);
  const [authPassword, setAuthPassword] = useState("");
  const [authKeyIntent, setAuthKeyIntent] = useState<KeySecretAction>("view");
  const [visibleKeyIds, setVisibleKeyIds] = useState<Record<string, boolean>>(
    {},
  );
  const [authError, setAuthError] = useState("");
  const [authPending, setAuthPending] = useState(false);
  const authPendingRef = useRef(false);
  const authInput = useRef<HTMLInputElement>(null);
  const [keyConfirmation, setKeyConfirmation] = useState<{
    action: "delete" | "regenerate";
    key: PreviewKey;
  } | null>(null);

  // Key usage drawer
  const [editingKey, setEditingKey] = useState<PreviewKey | null>(null);
  const [usageKeyId, setUsageKeyId] = useState<string | null>(null);
  const usageKey = keys.find((key) => key.id === usageKeyId) ?? null;

  // Exports drawer
  const [exportModalOpen, setExportModalOpen] = useState(false);
  const [exportType, setExportType] = useState<"requests" | "ledger" | "usage">(
    "requests",
  );
  const [exportDays, setExportDays] = useState(7);
  const [exportError, setExportError] = useState("");

  // Deactivate modal
  const [deactivateOpen, setDeactivateOpen] = useState(false);
  const [acknowledgeDust, setAcknowledgeDust] = useState(false);

  // Overview metrics derived from the backend usage summary
  // (GET /api/user/usage). Falls back to demo store data when the
  // query is loading or errors so the overview degrades gracefully.
  const usageItems = usageQuery.data?.items ?? [];
  // periodRows from demo store — used as fallback for model breakdown
  // and latency when the backend usage API has no data.
  const periodRows = useMemo(() => {
    return periodRequests(requests, days === "7" ? 7 : 30);
  }, [requests, days]);
  const chartActivity = useMemo(() => {
    if (usageItems.length) {
      return usageItems.map((item) => ({
        date: item.date.slice(5, 10).replace("-", "/"),
        requests: item.requests,
        // The chart and tooltip expect USD (small numbers like 0.0042),
        // not nano-USD. Convert quota -> USD.
        cost: Number(quotaToNanoUsd(item.quota)) / 1e9,
        // The backend usage API has no latency; default to 0 so the
        // latency chart shows a flat line instead of crashing.
        latency: 0,
      }));
    }
    // Fallback to demo data.
    return activityFor(periodRequests(requests, days === "7" ? 7 : 30), days === "7" ? 7 : 30);
  }, [usageItems, requests, days]);

  const summary = useMemo(() => {
    if (usageItems.length) {
      const totalRequests = usageItems.reduce((sum, i) => sum + i.requests, 0);
      const totalQuota = usageItems.reduce((sum, i) => sum + i.quota, 0);
      const totalSuccess = usageItems.reduce((sum, i) => sum + (i.success_count ?? 0), 0);
      return {
        count: totalRequests,
        cost: quotaToNanoUsd(totalQuota),
        // Compute the real success rate from the backend's per-day
        // success_count. Falls back to 100% when success_count is 0
        // (old logs without the status field).
        success: totalRequests > 0
          ? totalSuccess > 0
            ? Number(((totalSuccess / totalRequests) * 100).toFixed(2))
            : 100
          : 0,
      };
    }
    // Fallback to demo data.
    return summarizeRequests(periodRequests(requests, days === "7" ? 7 : 30));
  }, [usageItems, requests, days]);

  const modelRows = useMemo(() => {
    // Use backend by_model when available; fall back to demo store.
    const byModel = usageQuery.data?.by_model;
    if (byModel && byModel.length > 0) {
      const buckets = new Map<string, { count: number; quota: number; tokens: number }>();
      for (const row of byModel) {
        const existing = buckets.get(row.model);
        if (existing) {
          existing.count += row.requests;
          existing.quota += row.quota;
          existing.tokens += row.prompt_tokens + row.completion_tokens;
        } else {
          buckets.set(row.model, {
            count: row.requests,
            quota: row.quota,
            tokens: row.prompt_tokens + row.completion_tokens,
          });
        }
      }
      return [...buckets.entries()]
        .map(([model, agg]) => ({
          model,
          vendor: resolveVendor(model),
          count: agg.count,
          cost: quotaToNanoUsd(agg.quota),
          success: agg.count > 0 ? "100" : "—",
        }))
        .sort((a, b) => b.count - a.count);
    }
    return breakdownByModel(periodRequests(requests, days === "7" ? 7 : 30));
  }, [usageQuery.data, requests, days]);

  const isLowBalance = balanceKnown && BigInt(balance) < 5000000000n; // < $5.00
  const isZeroBalance = balanceKnown && BigInt(balance) <= 0n;

  const requestKeySecret = (key: PreviewKey, intent: KeySecretAction) => {
    if (authPendingRef.current || !isCurrentSession()) return;
    if (revealedKeyIds[key.id] > Date.now()) {
      // Use the plaintext key if captured in this session (create/
      // regenerate); otherwise fall back to the masked value from the
      // backend list endpoint.
      const secret = plaintextKeys[key.id] ?? key.keySecret;
      if (intent === "view")
        setVisibleKeyIds((visible) => ({ ...visible, [key.id]: true }));
      else void copyText(secret, t("keyCopied"), t("copyFailed"));
      return;
    }
    // A new unlock must not revive a previous reveal when the intent is copy.
    setVisibleKeyIds((visible) => ({ ...visible, [key.id]: false }));
    setAuthRevealKeyId(key.id);
    setAuthKeyIntent(intent);
    setAuthPassword("");
    setAuthError("");
  };

  const handleRevealSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!authRevealKeyId || authPendingRef.current) return;
    authPendingRef.current = true;
    setAuthPending(true);
    setAuthError("");
    try {
      if (tokensError || !keys.some((key) => key.id === authRevealKeyId))
        throw Error("The current account's API key could not be verified");
      await verifyTokenPassword(Number(authRevealKeyId), authPassword, sessionScope);
      if (!isCurrentSession()) return;
      setRevealedKeyIds((s) => ({ ...s, [authRevealKeyId]: Date.now() + 300000 }));
      if (authKeyIntent === "view")
        setVisibleKeyIds((visible) => ({ ...visible, [authRevealKeyId]: true }));
      else {
        const secret =
          plaintextKeys[authRevealKeyId] ??
          keys.find((k) => k.id === authRevealKeyId)?.keySecret ??
          "";
        void copyText(secret, t("keyCopied"), t("copyFailed"));
      }
      setAuthRevealKeyId(null);
      setAuthPassword("");
      setAuthError("");
    } catch (error) {
      setAuthError(error instanceof ApiError && error.status === 403
        ? t("invalidPassword")
        : error instanceof ApiError && error.status === 429
          ? c("验证次数过多，请稍后重试。", "Too many verification attempts. Try again later.", "محاولات تحقق كثيرة. حاول لاحقًا.")
          : c("未能确认验证结果，请重试；登录状态或密钥变化后请重新打开。", "Verification could not be confirmed. Retry, or reopen after a session or key change.", "تعذر تأكيد التحقق. أعد المحاولة أو افتح النافذة مجددًا بعد تغيير الجلسة أو المفتاح."));
    } finally {
      authPendingRef.current = false;
      setAuthPending(false);
    }
  };

  const handleExportSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!Number.isInteger(exportDays) || exportDays < 1 || exportDays > 31) {
      setExportError(t("max31DaysWarning"));
      return;
    }
    createExportMut.mutate(
      { type: exportType, range_days: exportDays },
      {
        onSuccess: () => {
          setExportModalOpen(false);
          setExportError("");
        },
        onError: (err) => {
          setExportError(
            err instanceof Error ? err.message : String(err),
          );
        },
      },
    );
  };

  const downloadExportFile = (exp: ExportRecord) => {
    void downloadBackendExport(exp.id, exp.filename);
  };

  const activeSection =
    navigation.find((item) => item.path === section) || navigation[0];
  const systemNavigation = [
    {
      path: "model-pricing",
      label: c("标准模型", "Standard models", "النماذج القياسية"),
      Icon: CircleDollarSign,
    },
    {
      path: "channels",
      label: c("渠道管理", "Channels", "القنوات"),
      Icon: Network,
    },
    {
      path: "service-groups",
      label: c("服务分组", "Service groups", "مجموعات الخدمة"),
      Icon: Boxes,
    },
  ];
  const systemSection = systemNavigation.find((item) => item.path === section);
  const activeKeyCount = keys.filter((key) => key.active).length;
  const overviewMetric =
    chartMetric === "cost"
      ? formatMoney(summary.cost, "total")
      : chartMetric === "latency"
        ? `${periodRows.length ? Math.round(periodRows.reduce((total, row) => total + row.duration_ms, 0) / periodRows.length) : 0} ms`
        : summary.count.toLocaleString();
  // Model breakdown: use backend by_model when available, else demo.
  const byModel = usageQuery.data?.by_model ?? [];
  const modelBreakdown = byModel.length
    ? Object.entries(
        byModel.reduce<Record<string, number>>((acc, item) => {
          acc[item.model] = (acc[item.model] || 0) + item.requests;
          return acc;
        }, {}),
      ).sort((a, b) => b[1] - a[1])
    : Object.entries(
        periodRows.reduce<Record<string, number>>((acc, row) => {
          acc[row.vendor] = (acc[row.vendor] || 0) + 1;
          return acc;
        }, {}),
      ).sort((a, b) => b[1] - a[1]);
  const breakdownTotal = byModel.length
    ? byModel.reduce((sum, item) => sum + item.requests, 0)
    : periodRows.length;
  const modelShares = modelBreakdown.map(([, count]) =>
    Math.floor((count / (breakdownTotal || 1)) * 100),
  );
  const remainingShare = breakdownTotal
    ? 100 - modelShares.reduce((sum, value) => sum + value, 0)
    : 0;
  // Distribute rounding remainders so the visible shares always total 100%.
  modelBreakdown
    .map(([, count], index) => ({
      index,
      remainder: (count / (breakdownTotal || 1)) * 100 - modelShares[index],
    }))
    .sort((a, b) => b.remainder - a.remainder)
    .slice(0, remainingShare)
    .forEach(({ index }) => modelShares[index]++);

  return (
    <div className="console-preview xy-console">
      {mobileNavOpen && (
        <button
          className="xy-sidebar-shade"
          aria-label={t("close")}
          onClick={() => setMobileNavOpen(false)}
        />
      )}
      <aside className={`xy-sidebar ${mobileNavOpen ? "is-open" : ""}`}>
        <div className="xy-sidebar-brand">
          <ProductBrand compact />
          <button
            className="xy-mobile-close"
            aria-label={t("close")}
            onClick={() => setMobileNavOpen(false)}
          >
            <X size={18} />
          </button>
        </div>
        <div className="xy-workspace">
          <span className="xy-workspace-avatar">V</span>
          <div>
            <strong>{c("工作台", "Console", "لوحة العمل")}</strong>
            <small>
              {isSystemAdmin
                ? c(
                    "系统管理员 · 演示",
                    "System admin · demo",
                    "مسؤول النظام · عرض",
                  )
                : c("普通用户 · 演示", "Customer · demo", "مستخدم · عرض")}
            </small>
          </div>
        </div>
        <nav
          ref={sidebarNav}
          className="xy-sidebar-nav"
          aria-label={t("pNavigation")}
        >
          <span className="xy-nav-label">
            {c("工作台", "CONSOLE", "لوحة العمل")}
          </span>
          {navigation.map(({ path, key, Icon }) => (
            <div key={path}>
              {path === "service-quality" && (
                <span className="xy-nav-label xy-nav-divider">
                  {t("navObservability")}
                </span>
              )}
              {path === "billing" && (
                <span className="xy-nav-label xy-nav-divider">
                  {c("账号", "ACCOUNT", "الحساب")}
                </span>
              )}
              <Link
                to={
                  path === "models"
                    ? "/models"
                    : `/console${path ? `/${path}` : ""}`
                }
                onClick={() => setMobileNavOpen(false)}
                className={`xy-nav-link ${section === path || (path === "" && section === "console") ? "is-active" : ""}`}
                aria-current={
                  section === path || (path === "" && section === "console")
                    ? "page"
                    : undefined
                }
              >
                <Icon size={18} />
                <span>{path === "settings" ? settingsLabel : t(key)}</span>
                {path === "keys" && keysNearQuota.length > 0 && (
                  <span className="xy-nav-badge">{keysNearQuota.length}</span>
                )}
              </Link>
            </div>
          ))}
          {canReadFinance(role) && <Link to="/finance" className="xy-nav-link" onClick={()=>setMobileNavOpen(false)}><CircleDollarSign size={18}/><span>{c("财务概览","Finance overview","نظرة مالية")}</span></Link>}
          {isSystemAdmin && (
            <>
              <span className="xy-nav-label xy-nav-divider">
                {c("系统设置", "SYSTEM SETTINGS", "إعدادات النظام")}
              </span>
              {systemNavigation.map(({ path, label, Icon }) => (
                <Link
                  key={path}
                  to={`/console/${path}`}
                  onClick={() => setMobileNavOpen(false)}
                  className={`xy-nav-link ${section === path ? "is-active" : ""}`}
                  aria-current={section === path ? "page" : undefined}
                >
                  <Icon size={18} />
                  <span>{label}</span>
                </Link>
              ))}
            </>
          )}
        </nav>
        <div className="xy-sidebar-bottom">
          <Link to="/docs" className="xy-help-link">
            <BookOpen size={17} />
            {t("navDocs")}
            <ArrowUpRight size={14} />
          </Link>
          <div className="xy-sidebar-tip">
            <div className="xy-tip-symbol">
              <Sparkles size={19} />
            </div>
            <strong>
              {c(
                "从一个好想法开始",
                "Build your next big idea",
                "ابدأ فكرتك القادمة",
              )}
            </strong>
            <p>
              {c(
                "接入前沿模型，让创意更快实现。",
                "Everything you need to ship with AI.",
                "كل ما تحتاجه لبناء تطبيقك الذكي.",
              )}
            </p>
            <Link to="/models">
              {c("探索模型", "Explore models", "استكشف النماذج")}
              <ArrowRight size={15} />
            </Link>
          </div>
          <Link className="xy-profile" to="/console/settings">
            <span className="xy-profile-avatar" aria-hidden="true">
              {profileInitials}
            </span>
            <div>
              <strong>{profileName}</strong>
              <small>{settingsLabel}</small>
            </div>
            <MoreHorizontal size={17} />
          </Link>
        </div>
      </aside>
      <div className="xy-workspace-body">
        <header className="xy-topbar">
          <div className="xy-breadcrumb">
            <button
              className="xy-menu-toggle"
              aria-label={c("打开导航", "Open navigation", "افتح التنقل")}
              aria-expanded={mobileNavOpen}
              onClick={() => setMobileNavOpen(true)}
            >
              <Menu size={20} />
            </button>
            <span>{c("工作台", "Console", "لوحة العمل")}</span>
            <ChevronRight size={14} className="directional" />
            <strong>
              {isRechargePage
                ? c("账户充值", "Add credits", "إضافة رصيد")
                : section === "settings"
                  ? settingsLabel
                  : systemSection?.label || t(activeSection.key)}
            </strong>
          </div>
          <div className="xy-topbar-actions">
            <ProductSearch />
            <Link className="xy-system-link" to="/status">
              <span
                className={`xy-status-dot ${
                  (() => {
                    const reqs = metricsQuery.data?.requests ?? [];
                    const total = reqs.reduce((s, r) => s + r.count, 0);
                    const errs = reqs
                      .filter((r) => r.class !== "success" && r.class !== "client_error")
                      .reduce((s, r) => s + r.count, 0);
                    return total > 0 && errs / total > 0.01 ? "is-warning" : "";
                  })()
                }`}
              />
              {c("系统状态", "System status", "حالة النظام")}
            </Link>
            <Preferences />
            <Link
              to="/console/settings"
              className="xy-header-avatar"
              aria-label={`${profileName} · ${settingsLabel}`}
            >
              {profileInitials}
            </Link>
          </div>
        </header>
        {(isLowBalance || isZeroBalance) && (
          <div
            className={`xy-balance-alert ${isZeroBalance ? "is-empty" : ""}`}
          >
            <AlertTriangle size={17} />
            <span>
              {t(isZeroBalance ? "balanceDepletedWarning" : "balanceWarning")}
            </span>
            {!isRechargePage && (
              <button onClick={() => openRecharge()}>
                {t("recharge")}
                <ArrowRight size={14} />
              </button>
            )}
          </div>
        )}
        <main className="xy-main">
          {systemSection && !isSystemAdmin && (
            <section className="xy-card" style={{ padding: 28 }}>
              <ShieldCheck size={28} />
              <h1>
                {c(
                  "需要系统管理员权限",
                  "System administrator access required",
                  "يلزم إذن مسؤول النظام",
                )}
              </h1>
              <p>
                {c(
                  "当前为普通用户演示账户，无法查看或修改系统定价与分组。可在右下角演示控制中切换身份进行验证。",
                  "The customer demo account cannot view or edit system pricing or groups. Use the demo control to test another role.",
                  "لا يمكن للمستخدم عرض أو تعديل الأسعار والمجموعات. يمكن تغيير الهوية من أداة العرض.",
                )}
              </p>
              <Button asChild variant="secondary">
                <Link to="/console">
                  {c("返回概览", "Back to overview", "العودة للنظرة العامة")}
                </Link>
              </Button>
            </section>
          )}
          {isSystemAdmin && section === "model-pricing" && (
            <StandardModels role={role} />
          )}
          {isSystemAdmin && section === "channels" && <Channels role={role} />}
          {isSystemAdmin && section === "service-groups" && (
            <ServiceGroups role={role} />
          )}
          {section === "connect" && (
            <AccessCenter
              key={location.key}
              plaintextKeys={plaintextKeys}
              revealedKeys={revealedKeyIds}
              onReveal={verifyAndReveal}
            />
          )}
          {section === "service-quality" && <ServiceQuality />}
          {section === "smart-routing" && <SmartRouting />}
          {(section === "" || section === "console") && (
            <div className="xy-overview">
              <div className="xy-page-heading">
                <div>
                  <div className="xy-eyebrow">
                    {c(
                      "一切尽在掌握",
                      "YOUR WORKSPACE, AT A GLANCE",
                      "مساحة عملك بنظرة واحدة",
                    )}
                  </div>
                  <h1>{c("工作台概览", "Overview", "نظرة عامة")}</h1>
                  <p>
                    {c(
                      "欢迎回来，来看看你的模型今天表现如何。",
                      "Welcome back. Here’s how your models are performing.",
                      "مرحبًا بعودتك. إليك أداء نماذجك.",
                    )}
                  </p>
                </div>
                <div className="xy-heading-actions">
                  <button
                    className="xy-secondary"
                    onClick={() => navigate("/console/connect")}
                  >
                    <BookOpen size={16} />
                    {c("接入指南", "Quickstart", "دليل البدء")}
                  </button>
                  <button
                    className="xy-primary"
                    onClick={() => setCreateOpen(true)}
                  >
                    <Plus size={17} />
                    {t("pCreateKey")}
                  </button>
                </div>
              </div>
{!onboardingComplete && (
                <section className="xy-onboarding">
                  <div>
                    <span className="xy-onboarding-icon">
                      <Sparkles size={20} />
                    </span>
                    <div>
                      <h2>{t("onboardingTitle")}</h2>
                      <p>
                        {c(
                          "完成以下步骤，开启第一次模型调用。",
                          "A few simple steps to your first model request.",
                          "خطوات بسيطة لطلبك الأول.",
                        )}
                      </p>
                    </div>
                    <button onClick={() => setOnboardingDismissed(true)}>
                      {t("dismissOnboarding")}
                    </button>
                  </div>
                  <div className="xy-onboarding-steps">
                    {[
                      {
                        title: c(
                          "创建或复用密钥",
                          "Create or reuse a key",
                          "إنشاء مفتاح أو إعادة استخدامه",
                        ),
                        Icon: KeyRound,
                        action: () => setCreateOpen(true),
                      },
                      {
                        title: c(
                          "选择工具并获取配置",
                          "Configure your tool",
                          "إعداد الأداة",
                        ),
                        Icon: BookOpen,
                        action: () => navigate("/console/connect"),
                      },
                      {
                        title: c(
                          "补充预付费余额",
                          "Add prepaid balance",
                          "إضافة رصيد مسبق",
                        ),
                        Icon: CreditCard,
                        action: () => openRecharge(),
                      },
                      {
                        title: c(
                          "查看调用与费用",
                          "View calls and costs",
                          "عرض الطلبات والتكلفة",
                        ),
                        Icon: Zap,
                        action: () => navigate("/console/requests"),
                      },
                    ].map(({ title, Icon, action }, index) => (
                      <button key={title} onClick={action}>
                        <span>0{index + 1}</span>
                        <Icon size={16} />
                        <strong>{title}</strong>
                        <ArrowUpRight size={15} />
                      </button>
                    ))}
                  </div>
                </section>
              )}
              <div className="overview-actionable">
                {backendLogs.length === 0 && !logsQuery.isLoading && (
                  <span>
                    {c("暂无调用记录", "No requests yet", "لا توجد طلبات بعد")}
                  </span>
                )}
                {/* Failed-request alerts: the backend Log has no
                    status field, so we show a general link when logs
                    exist. The keys-near-quota alerts come from real
                    backend key data. */}
                {keysNearQuota.map((k) => (
                  <Link key={k.id} to="/console/keys">
                    {k.name} ·{" "}
                    {keyQuotaSummary(k, quotaAt).exhausted
                      ? c("额度已用尽", "Quota exhausted", "الحصة مستنفدة")
                      : c(
                          "额度即将耗尽",
                          "Quota nearly exhausted",
                          "الحصة قاربت النفاد",
                        )}{" "}
                    →
                  </Link>
                ))}
              </div>
              <div className="xy-metrics">
                {[
                  {
                    label: c("请求总量", "Total requests", "إجمالي الطلبات"),
                    value: summary.count.toLocaleString(),
                    note: c(
                      `近 ${days} 天累计调用`,
                      `Requests in the last ${days} days`,
                      `الطلبات خلال ${days} يومًا`,
                    ),
                    Icon: Activity,
                    series: chartActivity.map((item) => item.requests),
                    color: "blue",
                  },
                  {
                    label: c("累计消费", "Total spend", "إجمالي الإنفاق"),
                    value: formatMoney(summary.cost, "total"),
                    note: c(
                      "按实际用量计费",
                      "Pay only for what you use",
                      "ادفع مقابل استخدامك فقط",
                    ),
                    Icon: Wallet,
                    series: chartActivity.map((item) => item.cost),
                    color: "purple",
                  },
                  {
                    label: c("请求成功率", "Success rate", "معدل النجاح"),
                    value: summary.count ? `${summary.success}%` : "—",
                    note: c(
                      "所选周期内成功请求占比",
                      "Successful requests this period",
                      "الطلبات الناجحة في هذه الفترة",
                    ),
                    Icon: CircleCheck,
                    series: chartActivity.map((item) =>
                      item.requests > 0 ? 100 : 0,
                    ),
                    color: "green",
                  },
                  {
                    label: c("有效密钥", "Active keys", "المفاتيح النشطة"),
                    value: String(activeKeyCount).padStart(2, "0"),
                    note: c(
                      `共 ${keys.length} 个密钥`,
                      `${keys.length} keys in your account`,
                      `${keys.length} مفاتيح في مساحة عملك`,
                    ),
                    Icon: KeyRound,
                    series: chartActivity.map(() => activeKeyCount),
                    color: "amber",
                  },
                ].map(({ label, value, note, Icon, series, color }) => (
                  <section className={`xy-metric-card is-${color}`} key={label}>
                    <div className="xy-metric-label">
                      <span>{label}</span>
                      <Icon size={18} />
                    </div>
                    <div className="xy-metric-middle">
                      <strong>{value}</strong>
                      <svg
                        viewBox="0 0 90 34"
                        className="xy-sparkline"
                        aria-hidden="true"
                      >
                        <polyline
                          points={series
                            .map(
                              (v, index) =>
                                `${(index / (series.length - 1 || 1)) * 88 + 1},${30 - ((v - Math.min(...series)) / (Math.max(...series) - Math.min(...series) || 1)) * 24}`,
                            )
                            .join(" ")}
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="2"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                        />
                      </svg>
                    </div>
                    <div className="xy-metric-note">
                      <span className="xy-tiny-dot" />
                      {note}
                    </div>
                  </section>
                ))}
              </div>
              <div className="xy-dashboard-grid">
                <div className="xy-dashboard-primary">
                  <section className="xy-card xy-chart-card">
                    <div className="xy-card-heading">
                      <div>
                        <h2>
                          {c(
                            "用量趋势",
                            "Usage overview",
                            "نظرة على الاستخدام",
                          )}
                        </h2>
                        <p>
                          {c(
                            "让每一次调用，都清晰可见",
                            "A clearer picture of every request",
                            "صورة أوضح لكل طلب",
                          )}
                        </p>
                      </div>
                      <div
                        className="xy-period-tabs"
                        aria-label={c(
                          "统计周期",
                          "Time period",
                          "الفترة الزمنية",
                        )}
                      >
                        {(["7", "30"] as const).map((day) => (
                          <button
                            key={day}
                            className={days === day ? "is-active" : ""}
                            onClick={() => setDays(day)}
                          >
                            {c(`近 ${day} 天`, `${day} days`, `${day} يومًا`)}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div className="xy-chart-summary">
                      <strong>{overviewMetric}</strong>
                      <div className="xy-chart-legend">
                        <span />
                        {chartMetric === "requests"
                          ? c("调用请求", "Requests", "الطلبات")
                          : chartMetric === "cost"
                            ? c("消费金额", "Spend", "الإنفاق")
                            : c(
                                "平均延迟",
                                "Average latency",
                                "متوسط الاستجابة",
                              )}
                      </div>
                    </div>
                    <div className="xy-chart-controls">
                      {(
                        [
                          {
                            id: "requests",
                            label: c("请求数", "Requests", "الطلبات"),
                          },
                          { id: "cost", label: c("消费", "Spend", "الإنفاق") },
                          {
                            id: "latency",
                            label: c("延迟", "Latency", "الاستجابة"),
                          },
                        ] as const
                      ).map((metric) => (
                        <button
                          key={metric.id}
                          className={
                            chartMetric === metric.id ? "is-active" : ""
                          }
                          onClick={() => setChartMetric(metric.id)}
                        >
                          {metric.label}
                        </button>
                      ))}
                    </div>
                    <div className="xy-chart">
                      <ResponsiveContainer width="100%" height="100%">
                        <AreaChart
                          data={chartActivity}
                          margin={{ top: 18, right: 8, bottom: 0, left: -18 }}
                        >
                          <defs>
                            <linearGradient
                              id="xyUsageGradient"
                              x1="0"
                              y1="0"
                              x2="0"
                              y2="1"
                            >
                              <stop
                                offset="0%"
                                stopColor="var(--brand)"
                                stopOpacity={0.18}
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
                            strokeDasharray="4 5"
                            stroke="var(--border)"
                          />
                          <XAxis
                            dataKey="date"
                            stroke="var(--muted)"
                            fontSize={11}
                            axisLine={false}
                            tickLine={false}
                            tickMargin={14}
                            minTickGap={24}
                          />
                          <YAxis
                            stroke="var(--muted)"
                            fontSize={11}
                            axisLine={false}
                            tickLine={false}
                            tickMargin={10}
                          />
                          <ChartTooltip
                            contentStyle={{
                              background: "var(--surface)",
                              border: "1px solid var(--border)",
                              borderRadius: 10,
                              fontSize: 12,
                              boxShadow: "0 8px 32px #16264a12",
                            }}
                            formatter={(value: number) => [
                              chartMetric === "cost"
                                ? `$${value.toFixed(4)}`
                                : value.toLocaleString(),
                              chartMetric === "requests"
                                ? c("请求数", "Requests", "الطلبات")
                                : chartMetric === "cost"
                                  ? c("消费", "Spend", "الإنفاق")
                                  : c(
                                      "延迟（毫秒）",
                                      "Latency (ms)",
                                      "الاستجابة",
                                    ),
                            ]}
                          />
                          <Area
                            isAnimationActive={false}
                            type="monotone"
                            dataKey={chartMetric}
                            stroke="var(--brand)"
                            strokeWidth={2.6}
                            fill="url(#xyUsageGradient)"
                            activeDot={{
                              r: 5,
                              stroke: "var(--surface)",
                              strokeWidth: 3,
                            }}
                          />
                        </AreaChart>
                      </ResponsiveContainer>
                    </div>
                    <div className="xy-model-breakdown">
                      {modelBreakdown.map(([vendor, count], index) => (
                        <div key={vendor}>
                          <span>
                            <i
                              style={{
                                background: [
                                  "#2e68fa",
                                  "#8062e7",
                                  "#48bb98",
                                  "#f3ae5b",
                                  "#df7e9a",
                                ][index],
                              }}
                            />
                            {vendor}
                          </span>
                          <strong>
                            {modelShares[index]}
                            <small>%</small>
                          </strong>
                          <div className="xy-share-track">
                            <i
                              style={{
                                width: `${(count / (periodRows.length || 1)) * 100}%`,
                                background: [
                                  "#2e68fa",
                                  "#8062e7",
                                  "#48bb98",
                                  "#f3ae5b",
                                  "#df7e9a",
                                ][index],
                              }}
                            />
                          </div>
                        </div>
                      ))}
                    </div>
                  </section>
                  <section className="xy-card xy-recent-card">
                    <div className="xy-card-heading">
                      <div>
                        <h2>
                          {c("最近调用", "Recent requests", "الطلبات الأخيرة")}
                        </h2>
                        <p>
                          {c(
                            "追踪请求状态与每一笔消费",
                            "Track status and cost, down to the request",
                            "تتبع الحالة والتكلفة لكل طلب",
                          )}
                        </p>
                      </div>
                      <Link to="/console/requests" className="xy-text-link">
                        {c("查看全部", "View all", "عرض الكل")}
                        <ArrowUpRight size={15} />
                      </Link>
                    </div>
                    <div className="xy-table-scroll">
                      <table>
                        <thead>
                          <tr>
                            <th>{c("模型", "Model", "النموذج")}</th>
                            <th>
                              {c("时间（迪拜）", "Time · Dubai", "الوقت · دبي")}
                            </th>
                            <th>{c("状态", "Status", "الحالة")}</th>
                            <th>{c("费用", "Cost", "التكلفة")}</th>
                            <th>
                              <span className="sr-only">
                                {c("查看详情", "View details", "عرض التفاصيل")}
                              </span>
                            </th>
                          </tr>
                        </thead>
                        <tbody>
                          {backendLogs.map((log, index) => (
                            <tr key={log.id}>
                              <td>
                                <div className="xy-table-model">
                                  <span
                                    className={`xy-model-icon is-model-${index % 3}`}
                                  >
                                    {log.model.charAt(0).toUpperCase()}
                                  </span>
                                  <div>
                                    <strong>{log.model}</strong>
                                    <small>{log.token_name}</small>
                                  </div>
                                </div>
                              </td>
                              <td>
                                {new Date(log.created_at * 1000).toLocaleTimeString(
                                  "en-GB",
                                  { timeZone: "Asia/Dubai", hour12: false },
                                )}
                              </td>
                              <td>
                                {/* Backend logs only contain billed
                                    (successful) requests. Failed requests
                                    are not logged, so failed={false} is
                                    always correct here. */}
                                <ResultPill failed={false} />
                              </td>
                              <td className="xy-cost">
                                {formatMoney(quotaToNanoUsd(log.quota), "call")}
                              </td>
                              <td>
                                <Link
                                  className="xy-row-detail"
                                  to="/console/requests"
                                  aria-label={c("查看调用详情", "View request details", "عرض تفاصيل الطلب")}
                                >
                                  <ArrowUpRight size={16} />
                                </Link>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                      {!backendLogs.length && !logsQuery.isLoading && (
                        <p className="xy-empty">
                          {c(
                            "暂无调用，前往调试台开始第一次请求。",
                            "No requests yet. Configure your preferred tool to get started.",
                            "لا توجد طلبات بعد. جرّب طلبك الأول.",
                          )}
                        </p>
                      )}
                    </div>
                  </section>
                </div>
                <aside className="xy-dashboard-secondary">
                  <section className="xy-wallet-card">
                    <div className="xy-wallet-top">
                      <span>
                        <Wallet size={17} />
                        {t("availableBalance")}
                      </span>
                      <span className="xy-wallet-tag">
                        {c("预付费", "PREPAID", "مدفوع مسبقًا")}
                      </span>
                    </div>
                    <strong tabIndex={balanceKnown ? 0 : undefined} style={{ maxWidth: "100%", overflowX: "auto", whiteSpace: "nowrap" }}>{balanceKnown ? formatMoney(balance) : "—"}</strong>
                    <p>
                      {c(
                        "按量付费 · 随时充值",
                        "Pay as you go. Top up anytime.",
                        "ادفع حسب الاستخدام. اشحن في أي وقت.",
                      )}
                    </p>
                    <div className="xy-wallet-art" aria-hidden="true">
                      <i />
                      <i />
                      <i />
                    </div>
                    <button onClick={() => openRecharge()}>
                      <Plus size={17} />
                      {c("为账户充值", "Add funds", "إضافة رصيد")}
                      <ArrowUpRight size={16} />
                    </button>
                  </section>
                  <section className="xy-card xy-health-card">
                    <div className="xy-card-heading">
                      <h2>{c("服务状态", "Service health", "حالة الخدمة")}</h2>
                      <Link
                        to="/status"
                        aria-label={c(
                          "查看服务状态",
                          "View service status",
                          "عرض حالة الخدمة",
                        )}
                      >
                        <ArrowUpRight size={17} />
                      </Link>
                    </div>
                    {(() => {
                      const reqs = metricsQuery.data?.requests ?? [];
                      const total = reqs.reduce((s, r) => s + r.count, 0);
                      const errors = reqs
                        .filter(
                          (r) =>
                            r.class !== "success" &&
                            r.class !== "client_error",
                        )
                        .reduce((s, r) => s + r.count, 0);
                      const hasIssue = total > 0 && errors / total > 0.01;
                      return (
                        <>
                          <div
                            className={`xy-health-summary ${hasIssue ? "has-issue" : ""}`}
                          >
                            <span>
                              <ShieldCheck size={23} />
                            </span>
                            <div>
                              <strong>
                                {!total
                                  ? c(
                                      "暂无请求",
                                      "No requests yet",
                                      "لا طلبات بعد",
                                    )
                                  : hasIssue
                                    ? c(
                                        "部分上游异常",
                                        "Upstream degraded",
                                        "تدهور خدمة المزود",
                                      )
                                    : c(
                                        "运行正常",
                                        "Operational",
                                        "جميع الأنظمة تعمل",
                                      )}
                              </strong>
                              <small>
                                {total > 0
                                  ? c(
                                      `${total} 次请求 · ${errors} 错误`,
                                      `${total} requests · ${errors} errors`,
                                      `${total} طلبات · ${errors} أخطاء`,
                                    )
                                  : c(
                                      "等待第一次调用",
                                      "Awaiting first request",
                                      "بانتظار أول طلب",
                                    )}
                              </small>
                            </div>
                          </div>
                          <div className="xy-health-bars" aria-hidden="true">
                            {Array.from({ length: 36 }, (_, index) => (
                              <i
                                key={index}
                                className={
                                  hasIssue && index > 30 ? "has-issue" : ""
                                }
                              />
                            ))}
                          </div>
                          <div className="xy-health-caption">
                            <span>
                              {c("平台状态", "Platform status", "حالة المنصة")}
                            </span>
                            <strong>
                              {!total
                                ? c(
                                    "尚无请求，待验证",
                                    "No requests · unverified",
                                    "لا طلبات · غير متحقق",
                                  )
                                : hasIssue
                                  ? c(
                                      "检测到上游异常",
                                      "Upstream issues detected",
                                      "تم رصد مشكلات",
                                    )
                                  : c(
                                      "查看调用结果",
                                      "See request results",
                                      "راجع نتائج الطلبات",
                                    )}
                            </strong>
                          </div>
                          <div className="xy-health-divider" />
                          <div className="xy-health-line">
                            <span>
                              {c("网关区域", "Gateway region", "منطقة البوابة")}
                            </span>
                            <strong>
                              {c("迪拜", "Dubai", "دبي")}
                              <span className="xy-status-dot" />
                            </strong>
                          </div>
                          <div className="xy-health-line">
                            <span>
                              {c(
                                "活跃密钥",
                                "Active keys",
                                "المفاتيح النشطة",
                              )}
                            </span>
                            <strong>{keys.length}</strong>
                          </div>
                        </>
                      );
                    })()}
                  </section>
                  <section className="xy-card xy-shortcuts">
                    <h2>
                      {c("下一步，创造更多", "Keep building", "واصل البناء")}
                    </h2>
                    <button onClick={() => navigate("/console/playground")}>
                      <span className="xy-shortcut-icon">
                        <Terminal size={18} />
                      </span>
                      <span>
                        <strong>
                          {c(
                            "在线调试台",
                            "Open playground",
                            "افتح ساحة التجربة",
                          )}
                        </strong>
                        <small>
                          {c(
                            "即刻尝试你的模型",
                            "Try a model in seconds",
                            "جرّب نموذجًا في ثوانٍ",
                          )}
                        </small>
                      </span>
                      <ChevronRight size={16} className="directional" />
                    </button>
                    <button onClick={() => navigate("/console/connect")}>
                      <span className="xy-shortcut-icon is-purple">
                        <BookOpen size={18} />
                      </span>
                      <span>
                        <strong>
                          {c("快速接入", "Integration guide", "دليل التكامل")}
                        </strong>
                        <small>
                          {c(
                            "只需几行代码即可开始",
                            "Get started in a few lines",
                            "ابدأ ببضعة أسطر",
                          )}
                        </small>
                      </span>
                      <ChevronRight size={16} className="directional" />
                    </button>
                  </section>
                </aside>
              </div>
              {keysNearQuota.length > 0 && (
                <div className="xy-quota-note">
                  <AlertTriangle size={16} />
                  <span>
                    {c(
                      "部分密钥已接近或达到消费额度。",
                      "Some keys are near or at their spending limit.",
                      "بعض المفاتيح قريبة من حد الإنفاق أو بلغت الحد.",
                    )}
                  </span>
                  <Link to="/console/keys">
                    {c("查看密钥", "Review keys", "مراجعة المفاتيح")}
                    <ArrowRight size={14} />
                  </Link>
                </div>
              )}
              <footer className="xy-dashboard-footer">
                <span>
                  © 2026{" "}
                  {c(
                    "MyAPIs · 让智能触手可及",
                    "MyAPIs · Intelligence, within reach",
                    "MyAPIs · الذكاء بين يديك",
                  )}
                </span>
                <span>
                  {c("演示数据", "Demo data", "بيانات تجريبية")}
                  {isSystemAdmin && (
                    <>
                      <i />{" "}
                      <Link to="/console/model-pricing">
                        {c("系统设置", "System settings", "إعدادات النظام")}
                      </Link>
                    </>
                  )}
                </span>
              </footer>
            </div>
          )}

          {/* SECTION: KEYS */}
          {section === "keys" &&
            (tokensLoading ? (
              <div className="key-list-empty">
                <KeyRound size={28} />
                <h2>
                  {c("加载中…", "Loading…", "جارٍ التحميل…")}
                </h2>
              </div>
            ) : tokensError ? (
              <div className="key-list-empty">
                <KeyRound size={28} />
                <h2>
                  {c("无法加载密钥", "Failed to load keys", "تعذّر تحميل المفاتيح")}
                </h2>
                <p>
                  {c(
                    "请检查网络连接或重新登录后刷新。",
                    "Check your connection or sign in again.",
                    "تحقق من الاتصال أو سجّل الدخول مجددًا.",
                  )}
                </p>
              </div>
            ) : (
              <KeyList
                keys={keys}
                revealedKeyIds={revealedKeyIds}
                visibleKeyIds={visibleKeyIds}
                now={keyDisplayNow}
                onCreate={() => setCreateOpen(true)}
                onDetails={(key) => setUsageKeyId(key.id)}
                onEdit={setEditingKey}
                onConnect={(key) => {
                  chooseKey(key.id);
                  navigate(`/console/connect?key=${encodeURIComponent(key.id)}`);
                }}
                onToggle={(key) =>
                  updateTokenMut.mutate({
                    id: Number(key.id),
                    status: key.active ? 2 : 1,
                  })
                }
                onRegenerate={(key) =>
                  setKeyConfirmation({ action: "regenerate", key })
                }
                onDelete={(key) => setKeyConfirmation({ action: "delete", key })}
                onSecret={requestKeySecret}
                onHide={(id) =>
                  setVisibleKeyIds((current) => ({ ...current, [id]: false }))
                }
              />
            ))}

          {/* SECTION: PLAYGROUND */}
          {section === "playground" && (
            <div>
              <div style={{ marginBottom: "var(--space-6)" }}>
                <h1
                  style={{
                    fontSize: "var(--text-xl)",
                    fontWeight: "var(--weight-bold)",
                    margin: 0,
                  }}
                >
                  {t("pPlayground")}
                </h1>
                <p
                  style={{
                    color: "var(--muted)",
                    fontSize: "var(--text-sm)",
                    margin: "4px 0 0 0",
                  }}
                >
                  {t("pPlaygroundHint")}
                </p>
              </div>

              <Playground
                plaintextKeys={plaintextKeys}
                onRelayComplete={() => {
                  void refreshRequests();
                  void walletQuery.refetch();
                }}
                key={location.key}
                initialModel={initialCreateModel}
                onRecord={setSelected}
                onCreate={() => setCreateOpen(true)}
              />
            </div>
          )}

          {/* SECTION: USAGE REPORTS */}
          {section === "usage" && (
            <div>
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  marginBottom: "var(--space-6)",
                }}
              >
                <div>
                  <h1
                    style={{
                      fontSize: "var(--text-xl)",
                      fontWeight: "var(--weight-bold)",
                      margin: 0,
                    }}
                  >
                    {t("usageReportsTitle")}
                  </h1>
                  <p
                    style={{
                      color: "var(--muted)",
                      fontSize: "var(--text-sm)",
                      margin: "4px 0 0 0",
                    }}
                  >
                    {c(
                      "按模型、服务等级与时间查看用量和消费。",
                      "Explore usage and spend by model, service tier and time.",
                      "استكشف الاستخدام والإنفاق حسب النموذج والمستوى والوقت.",
                    )}
                  </p>
                </div>
                <Button
                  size="small"
                  variant="secondary"
                  onClick={() => setExportModalOpen(true)}
                >
                  <Download size={14} style={{ marginInlineEnd: 4 }} />
                  {t("createExport")}
                </Button>
              </div>

              {/* Aggregated dimension breakdown */}
              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
                  gap: "var(--space-4)",
                  marginBottom: "var(--space-6)",
                }}
              >
                <div
                  style={{
                    background: "var(--surface)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-lg)",
                    padding: "var(--space-4)",
                  }}
                >
                  <div
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                      marginBottom: 4,
                    }}
                  >
                    {t("aggregateCalls")}
                  </div>
                  <div
                    className="mono"
                    style={{ fontSize: "var(--text-2xl)", fontWeight: 700 }}
                  >
                    {summary.count.toLocaleString()}
                  </div>
                </div>
                <div
                  style={{
                    background: "var(--surface)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-lg)",
                    padding: "var(--space-4)",
                  }}
                >
                  <div
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                      marginBottom: 4,
                    }}
                  >
                    {t("aggregateCost")}
                  </div>
                  <div
                    className="mono"
                    style={{
                      fontSize: "var(--text-2xl)",
                      fontWeight: 700,
                      color: "var(--brand)",
                    }}
                  >
                    {formatMoney(summary.cost, "total")}
                  </div>
                </div>
                <div
                  style={{
                    background: "var(--surface)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-lg)",
                    padding: "var(--space-4)",
                  }}
                >
                  <div
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                      marginBottom: 4,
                    }}
                  >
                    {t("successRate")}
                  </div>
                  <div
                    className="mono"
                    style={{
                      fontSize: "var(--text-2xl)",
                      fontWeight: 700,
                      color: "var(--success)",
                    }}
                  >
                    {summary.count ? `${summary.success}%` : "—"}
                  </div>
                </div>
                <div
                  style={{
                    background: "var(--surface)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-lg)",
                    padding: "var(--space-4)",
                  }}
                >
                  <div
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                      marginBottom: 4,
                    }}
                  >
                    {c("平均延迟", "Avg latency", "متوسط الاستجابة")}
                  </div>
                  <div
                    className="mono"
                    style={{ fontSize: "var(--text-2xl)", fontWeight: 700 }}
                  >
                    {periodRows.length
                      ? `${Math.round(periodRows.reduce((total, row) => total + row.duration_ms, 0) / periodRows.length)}ms`
                      : "—"}
                  </div>
                </div>
              </div>

              {/* Grouped Table by Model */}
              <div
                style={{
                  background: "var(--surface)",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius-lg)",
                  overflow: "hidden",
                }}
              >
                <div
                  style={{
                    padding: "var(--space-4)",
                    borderBottom: "1px solid var(--border)",
                    background: "var(--subtle)",
                  }}
                >
                  <h3
                    style={{
                      margin: 0,
                      fontSize: "var(--text-base)",
                      fontWeight: 600,
                    }}
                  >
                    {t("breakdownByModel")}
                  </h3>
                </div>
                <table
                  style={{
                    width: "100%",
                    borderCollapse: "collapse",
                    fontSize: "var(--text-xs)",
                  }}
                >
                  <thead>
                    <tr
                      style={{
                        borderBottom: "1px solid var(--border)",
                        background: "var(--subtle)",
                      }}
                    >
                      <th style={{ padding: "10px 14px", textAlign: "start" }}>
                        Model
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "start" }}>
                        Vendor
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "end" }}>
                        Calls
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "end" }}>
                        Success
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "end" }}>
                        Total Spend (4 dec)
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {modelRows.map((m) => (
                      <tr
                        key={m.model}
                        style={{ borderBottom: "1px solid var(--border)" }}
                      >
                        <td style={{ padding: "12px 14px", fontWeight: 600 }}>
                          {m.model}
                        </td>
                        <td
                          style={{
                            padding: "12px 14px",
                            color: "var(--secondary)",
                          }}
                        >
                          {m.vendor}
                        </td>
                        <td
                          className="mono"
                          style={{ padding: "12px 14px", textAlign: "end" }}
                        >
                          {m.count.toLocaleString()}
                        </td>
                        <td
                          className="mono"
                          style={{
                            padding: "12px 14px",
                            textAlign: "end",
                            color: "var(--success)",
                          }}
                        >
                          {m.success === "—" ? "—" : `${m.success}%`}
                        </td>
                        <td
                          className="mono"
                          style={{
                            padding: "12px 14px",
                            textAlign: "end",
                            fontWeight: 600,
                          }}
                        >
                          {formatMoney(m.cost, "total")}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* SECTION: REQUESTS */}
          {section === "requests" && (
            <RequestHistory
              requests={backendRequests as any}
              onSelect={setSelected}
              onExport={() => {
                setExportType("requests");
                setExportModalOpen(true);
              }}
            />
          )}

          {/* SECTION: BILLING & RECHARGE */}
          {section === "billing" && !isRechargePage && (
            <BillingCenter onRecharge={openRecharge} />
          )}
          {isRechargePage && (
            <RechargeFlow
              orderId={rechargeOrderId}
              onOrderChange={(orderId) => {
                const params = new URLSearchParams(location.search);
                if (orderId) params.set("order", orderId);
                else params.delete("order");
                navigate(
                  { pathname: location.pathname, search: params.toString() },
                  { replace: true, state: location.state },
                );
              }}
              onExit={() => navigate(rechargeReturnTo, { replace: true })}
              onViewRecords={(orderId) =>
                navigate("/console/billing?tab=orders", {
                  state: { billingOrder: orderId },
                })
              }
            />
          )}

          {/* SECTION: EXPORTS */}
          {section === "exports" && (
            <div>
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  marginBottom: "var(--space-6)",
                }}
              >
                <div>
                  <h1
                    style={{
                      fontSize: "var(--text-xl)",
                      fontWeight: "var(--weight-bold)",
                      margin: 0,
                    }}
                  >
                    {t("exportsTitle")}
                  </h1>
                  <p
                    style={{
                      color: "var(--muted)",
                      fontSize: "var(--text-sm)",
                      margin: "4px 0 0 0",
                    }}
                  >
                    {t("fileRetentionNote")}
                  </p>
                </div>
                <Button onClick={() => setExportModalOpen(true)}>
                  <Plus size={15} style={{ marginInlineEnd: 4 }} />
                  {t("createExport")}
                </Button>
              </div>

              <div
                style={{
                  background: "var(--surface)",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius-lg)",
                  overflow: "hidden",
                }}
              >
                <table
                  style={{
                    width: "100%",
                    borderCollapse: "collapse",
                    fontSize: "var(--text-xs)",
                  }}
                >
                  <thead>
                    <tr
                      style={{
                        background: "var(--subtle)",
                        borderBottom: "1px solid var(--border)",
                      }}
                    >
                      <th style={{ padding: "10px 14px", textAlign: "start" }}>
                        Export ID
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "start" }}>
                        Type
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "start" }}>
                        Date Range
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "start" }}>
                        Created
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "start" }}>
                        Status
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "end" }}>
                        Download
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {backendExports.map((exp) => (
                      <tr
                        key={exp.id}
                        style={{ borderBottom: "1px solid var(--border)" }}
                      >
                        <td className="mono" style={{ padding: "12px 14px" }}>
                          {exp.id}
                        </td>
                        <td
                          style={{
                            padding: "12px 14px",
                            fontWeight: 500,
                            textTransform: "capitalize",
                          }}
                        >
                          {exp.type}
                        </td>
                        <td
                          className="mono"
                          style={{
                            padding: "12px 14px",
                            color: "var(--muted)",
                          }}
                        >
                          {exp.range_days}d
                        </td>
                        <td className="mono" style={{ padding: "12px 14px" }}>
                          {new Date(exp.created_at * 1000).toLocaleDateString()}
                        </td>
                        <td style={{ padding: "12px 14px" }}>
                          <span
                            style={{
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: "var(--success-soft)",
                              color: "var(--success)",
                              fontWeight: 600,
                              fontSize: 11,
                            }}
                          >
                            {exp.status.toUpperCase()}
                          </span>
                        </td>
                        <td style={{ padding: "12px 14px", textAlign: "end" }}>
                          <Button
                            size="small"
                            variant="secondary"
                            onClick={() => downloadExportFile(exp)}
                          >
                            <Download
                              size={13}
                              style={{ marginInlineEnd: 4 }}
                            />
                            {t("downloadCsv")}
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* SECTION: ACCOUNT CENTER */}
          {section === "settings" && (
            <AccountCenter onDeactivate={() => setDeactivateOpen(true)} />
          )}
        </main>
      </div>

      {/* Password Re-Verification Modal for Revealing Key (KEY-4, Decision #68) */}
      <Modal
        size="compact"
        initialFocus={authInput}
        open={!!authRevealKeyId}
        preventOutsideClose={authPending}
        onOpenChange={(v) => {
          if (!v && !authPendingRef.current) {
            setAuthRevealKeyId(null);
            setAuthPassword("");
            setAuthError("");
          }
        }}
        title={t("reRevealTitle")}
        description={t("enterPasswordToReveal")}
      >
        <form
          onSubmit={handleRevealSubmit}
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-4)",
          }}
        >
          {authError && (
            <div
              id="key-auth-error"
              role="alert"
              style={{
                padding: "var(--space-3)",
                background: "var(--danger-soft)",
                color: "var(--danger)",
                borderRadius: "var(--radius-md)",
                fontSize: "var(--text-xs)",
              }}
            >
              {authError}
            </div>
          )}
          <div>
            <label
              htmlFor="key-auth-password"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {t("password")}
            </label>
            <Input
              id="key-auth-password"
              ref={authInput}
              autoComplete="current-password"
              type="password"
              placeholder={c(
                "输入当前账号密码",
                "Enter your current account password",
                "أدخل كلمة مرور حسابك الحالية",
              )}
              value={authPassword}
              disabled={authPending}
              aria-invalid={Boolean(authError)}
              aria-describedby={authError ? "key-auth-error" : undefined}
              onChange={(e) => setAuthPassword(e.target.value)}
              required
            />
          </div>
          <Button type="submit" variant="primary" disabled={authPending} aria-busy={authPending}>
            {t("unlockKey")}
          </Button>
        </form>
      </Modal>

      {/* Key details retain authorization and quota context outside the list. */}
      <Drawer
        open={!!usageKey}
        onOpenChange={(open) => {
          if (!open) setUsageKeyId(null);
        }}
        title={
          usageKey?.name
            ? `${usageKey.name} · ${c("密钥详情", "Key details", "تفاصيل المفتاح")}`
            : c("密钥详情", "Key details", "تفاصيل المفتاح")
        }
        description={c(
          "查看凭证状态、消费额度与授权范围。",
          "Review credential status, spending limits and access scope.",
          "راجع حالة بيانات الاعتماد وحدود الإنفاق ونطاق الصلاحيات.",
        )}
      >
        {usageKey && (
          <KeyDetails
            item={usageKey}
            now={keyDisplayNow}
            onEdit={(key) => {
              setEditingKey(key);
              setUsageKeyId(null);
            }}
          />
        )}
      </Drawer>

      {/* Create Export Modal with 31-day enforcement (PRD UI-3.6) */}
      <Modal
        size="default"
        open={exportModalOpen}
        onOpenChange={setExportModalOpen}
        title={t("createExport")}
        description={
          exportType === "ledger"
            ? c(
                "流水导出按迪拜时区日期单独选择范围，不沿用页面筛选。文件保留 7 天。",
                "Choose an independent Dubai-time date range for the ledger. Page filters do not apply. Files remain available for 7 days.",
                "اختر نطاق تواريخ مستقلًا بتوقيت دبي للسجل المالي. لا تُطبق مرشحات الصفحة. تبقى الملفات متاحة لمدة 7 أيام.",
              )
            : c(
                "导出按协调世界时日期单独选择范围，不沿用调用明细的时间、状态或搜索筛选。文件保留 7 天。",
                "Choose an independent UTC date range. Request-list date, status and search filters do not apply. Files remain available for 7 days.",
                "اختر نطاق تواريخ مستقلًا بتوقيت UTC. لا تُطبق مرشحات التاريخ والحالة والبحث في قائمة الطلبات. تبقى الملفات متاحة لمدة 7 أيام.",
              )
        }
      >
        <form
          onSubmit={handleExportSubmit}
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-4)",
          }}
        >
          {exportError && (
            <div
              style={{
                padding: "var(--space-3)",
                background: "var(--danger-soft)",
                color: "var(--danger)",
                borderRadius: "var(--radius-md)",
                fontSize: "var(--text-xs)",
              }}
            >
              {exportError}
            </div>
          )}

          <div>
            <label
              htmlFor="export-task-type"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {t("exportType")}
            </label>
            <select
              id="export-task-type"
              value={exportType}
              onChange={(e) => setExportType(e.target.value as any)}
              className="select-trigger"
              style={{ width: "100%" }}
            >
              <option value="requests">{t("requestsCsv")}</option>
              <option value="ledger">{t("ledgerCsv")}</option>
              <option value="usage">{t("usageCsv")}</option>
            </select>
          </div>

          <div>
            <label
              htmlFor="export-task-days"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {c("时间跨度（天）", "Date range (days)", "الفترة (أيام)")}
            </label>
            <Input
              id="export-task-days"
              type="number"
              value={exportDays}
              onChange={(e) => {
                const val = parseInt(e.target.value) || 1;
                setExportDays(val);
                if (val > 31) setExportError(t("max31DaysWarning"));
                else setExportError("");
              }}
              min="1"
              max="31"
              step="1"
            />
            <span style={{ fontSize: 11, color: "var(--muted)" }}>
              {c(
                "每份文件最多包含 31 天的数据",
                "Up to 31 days per export file",
                "حتى 31 يومًا لكل ملف",
              )}
            </span>
          </div>

          <Button type="submit" variant="primary" disabled={createExportMut.isPending}>
            {c("生成 CSV（表格文件）", "Generate CSV export", "إنشاء ملف CSV")}
          </Button>
        </form>
      </Modal>

      <Modal
        open={deactivateOpen}
        onOpenChange={(open) => {
          setDeactivateOpen(open);
          if (!open) setAcknowledgeDust(false);
        }}
        size="compact"
        preventOutsideClose
        title={c("确认注销账户", "Close this account", "تأكيد إغلاق الحساب")}
        description={c(
          "注销后无法继续使用此账户，所有密钥将失效。",
          "Closing the account ends access and disables all its keys.",
          "يؤدي إغلاق الحساب إلى إنهاء الوصول وتعطيل جميع مفاتيحه.",
        )}
        footer={
          <>
            <Button
              onClick={() => {
                setDeactivateOpen(false);
                setAcknowledgeDust(false);
              }}
            >
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              disabled={
                !balanceKnown ||
                BigInt(balance) < 0n ||
                BigInt(balance) >= 10000000n ||
                (BigInt(balance) > 0n && !acknowledgeDust)
              }
              onClick={async () => {
                if (
                  !balanceKnown ||
                  BigInt(balance) < 0n ||
                  BigInt(balance) >= 10000000n ||
                  (BigInt(balance) > 0n && !acknowledgeDust)
                )
                  return;
                await deactivateAccount();
                setDeactivateOpen(false);
                setAcknowledgeDust(false);
                navigate("/");
              }}
            >
              {c("确认注销", "Close account", "إغلاق الحساب")}
            </Button>
          </>
        }
      >
        <div className="key-task-close-account">
          <div className="key-task-account-balance">
            <span>{c("当前余额", "Current balance", "الرصيد الحالي")}</span>
            <strong dir="ltr">{balanceKnown ? formatMoney(balance, "detail") : "—"}</strong>
          </div>
          {!balanceKnown ? (
            <p role="alert">{c("无法确认钱包余额，请刷新后再处理注销。", "Wallet balance is unconfirmed. Refresh before closing the account.", "رصيد المحفظة غير مؤكد. حدّث قبل إغلاق الحساب.")}</p>
          ) : BigInt(balance) < 0n ? (
            <p className="key-task-danger" role="status">
              {c(
                "账户仍有未结清金额，请先结清余额，再联系客服处理注销。",
                "An outstanding balance remains. Settle it and contact support before closing the account.",
                "يوجد مبلغ مستحق. سدده وتواصل مع الدعم قبل إغلاق الحساب.",
              )}
            </p>
          ) : BigInt(balance) >= 10000000n ? (
            <p className="key-task-danger" role="status">
              {t("contactSupportNotice")}
            </p>
          ) : BigInt(balance) > 0n ? (
            <label className="key-task-dust-ack">
              <input
                type="checkbox"
                checked={acknowledgeDust}
                onChange={(event) => setAcknowledgeDust(event.target.checked)}
              />
              <span>
                {c(
                  "我确认放弃以上不足 0.01 美元的剩余余额；注销时将清零并记录流水。",
                  "I agree to forfeit the remaining balance below $0.01. Closing the account will record this write-off.",
                  "أوافق على التنازل عن الرصيد المتبقي الأقل من 0.01 دولار. سيُسجل تصفيره عند الإغلاق.",
                )}
              </span>
            </label>
          ) : (
            <p>
              {c(
                "余额为零，可以继续注销。此操作无法撤销。",
                "The balance is zero. You can close the account; this cannot be undone.",
                "الرصيد صفر. يمكنك إغلاق الحساب ولا يمكن التراجع عن ذلك.",
              )}
            </p>
          )}
        </div>
      </Modal>

      <ConfirmDialog
        open={keyConfirmation !== null}
        onOpenChange={(open) => {
          if (!open) setKeyConfirmation(null);
        }}
        title={
          keyConfirmation?.action === "delete"
            ? c("删除这把密钥？", "Delete this key?", "حذف هذا المفتاح؟")
            : c("重置这把密钥？", "Regenerate this key?", "تجديد هذا المفتاح؟")
        }
        description={
          keyConfirmation?.action === "delete"
            ? c(
                `删除“${keyConfirmation.key.name}”后，使用它的应用将无法继续调用。此操作无法撤销。`,
                `Deleting “${keyConfirmation.key.name}” stops calls from apps using it. This cannot be undone.`,
                `حذف «${keyConfirmation.key.name}» يوقف طلبات التطبيقات التي تستخدمه. لا يمكن التراجع عن ذلك.`,
              )
            : c(
                `重置“${keyConfirmation?.key.name ?? ""}”将立即使旧密钥失效。你需要将新密钥更新到使用它的应用。`,
                `Regenerating “${keyConfirmation?.key.name ?? ""}” immediately invalidates the old key. Update your apps with the replacement.`,
                `تجديد «${keyConfirmation?.key.name ?? ""}» يُبطل المفتاح القديم فورًا. حدّث التطبيقات بالمفتاح الجديد.`,
              )
        }
        confirmLabel={
          keyConfirmation?.action === "delete"
            ? c("删除密钥", "Delete key", "حذف المفتاح")
            : c("重置密钥", "Regenerate key", "تجديد المفتاح")
        }
        onConfirm={() => {
          if (!keyConfirmation) return;
          if (keyConfirmation.action === "delete") {
            deleteTokenMut.mutate(Number(keyConfirmation.key.id));
          } else {
            regenerateTokenMut.mutate(Number(keyConfirmation.key.id), {
              onSuccess: (token) => {
                const id = String(token.id);
                setPlaintextKeys((s) => ({ ...s, [id]: token.key }));
                setRegeneratedKey({
                  name: keyConfirmation.key.name,
                  plaintext: token.key,
                });
              },
            });
          }
          setKeyConfirmation(null);
        }}
      />

      {editingKey && (
        <EditKey
          key={editingKey.id}
          item={editingKey}
          onClose={() => setEditingKey(null)}
        />
      )}

      {/* One-time plaintext key dialog after regenerate (B10). The old
          key is invalid; this is the only chance to copy the new one. */}
      <Modal
        open={regeneratedKey !== null}
        onOpenChange={(open) => {
          if (!open) setRegeneratedKey(null);
        }}
        size="wide"
        title={c("密钥已重置", "Key regenerated", "تم تجديد المفتاح")}
        description={c(
          "旧密钥已立即失效，请将新密钥更新到使用它的应用。此为新密钥的唯一一次明文展示。",
          "The old key is invalid. Update your apps with the new key below — this is the only time it will be shown.",
          "المفتاح القديم غير صالح. حدّث تطبيقاتك بالمفتاح الجديد؛ هذا العرض الوحيد له.",
        )}
        footer={
          <Button
            variant="primary"
            onClick={() => setRegeneratedKey(null)}
          >
            {c("我已保存", "I've saved it", "حفظته")}
          </Button>
        }
      >
        {regeneratedKey && (
          <div className="key-success">
            <h3>{regeneratedKey.name}</h3>
            <div className="created-secret">
              <code dir="ltr">{regeneratedKey.plaintext}</code>
              <Button
                onClick={() =>
                  copyText(
                    regeneratedKey.plaintext,
                    t("copied"),
                    t("copyFailed"),
                  )
                }
              >
                <Copy size={14} />
                {t("copy")}
              </Button>
            </div>
          </div>
        )}
      </Modal>
      {/* Drawer and Modal Components */}
      <RequestDetails
        record={selected}
        onClose={() => {
          setSelected(null);
          if (section === "requests") {
            const params = new URLSearchParams(location.search);
            if (params.has("trace")) {
              params.delete("trace");
              navigate(
                { pathname: location.pathname, search: params.toString() },
                { replace: true },
              );
            }
          }
        }}
        onRetry={() => {
          if (selected?.metered_demo) {
            sessionStorage.setItem(
              "veyra-metered-retry-v1",
              JSON.stringify({
                modelId: selected.requested_model,
                keyId: selected.key_id,
                usage: selected.metered_usage,
                attributes: selected.metered_attributes,
                at: selected.pricing_snapshot?.quotedAt,
              }),
            );
            setSelected(null);
            navigate(
              `/console/model-pricing?model=${encodeURIComponent(selected.requested_model ?? selected.model)}`,
            );
            return;
          }
          if (selected?.request) {
            const r = selected.request;
            sessionStorage.setItem(
              "veyra-request-draft-v2",
              JSON.stringify({
                keyId: selected.key_id,
                model: r.model,
                protocol: r.protocol,
                prompt: r.messages
                  .filter((m) => m.role === "user")
                  .map((m) => m.content)
                  .join("\n"),
                system: r.messages
                  .filter((m) => m.role === "system")
                  .map((m) => m.content)
                  .join("\n"),
                temperature: r.temperature,
                maxTokens: r.max_tokens,
                capability: r.capability,
                chainId: r.chainId,
              }),
            );
          }
          const params = new URLSearchParams({
            key: selected?.key_id || "",
            model: selected?.requested_model || selected?.model || "gpt-4.1",
            protocol: selected?.protocol || "OpenAI",
          });
          setSelected(null);
          navigate(`/console/playground?${params.toString()}`);
        }}
      />

      <CreateKey
        key={location.key}
        open={createOpen}
        setOpen={changeCreateOpen}
        initialModel={initialCreateModel}
        initialGroup={initialCreateGroup}
        onCreated={(keyId, plaintext) => {
          setPlaintextKeys((s) => ({ ...s, [keyId]: plaintext }));
        }}
      />

      <Quickstart open={guideOpen} onOpenChange={setGuideOpen} />
    </div>
  );
}
