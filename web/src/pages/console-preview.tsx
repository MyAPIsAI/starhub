import { ResponsiveTable } from "../components/responsive-table";
import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
  Plus,
  Terminal,
  Wallet,
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
import ExportHistory from "../features/export-history";
import { validateBillingRange, billingDate } from "../lib/billing-export";
import { writtenLanguage } from "../lib/localize";
import AccessCenter from "../features/access-center";
import { isSmartModelId } from "../lib/smart-model";
import RequestDetails from "../features/request-inspector";
import RequestHistoryPage from "../features/request-history-page";
import UsageReports from "../features/usage-reports";
import ServiceQuality from "../features/service-quality";
import SmartRouting from "../features/resource-routing";
import { StandardModels } from "../features/standard-models";
import { Channels } from "../features/channels";
import { ServiceGroups } from "../features/service-groups";
import {
  useSystemAccess,
  isAdmin,
  canReadFinance,
  canManagePlatform,
  sessionUsername,
  sessionQueryIdentity,
  useSessionQueryIdentity,
} from "../lib/system-access";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useTokenChoices } from "../lib/api/token-choices";
import {
  useDeleteToken,
  useRegenerateToken,
  useUpdateToken,
  verifyTokenPassword,
  revealTokenSecret,
  readOwnedKeyReceipt,
} from "../lib/api/tokens";
import { useWallet } from "../lib/api/billing";
import { useCreateExport } from "../lib/api/exports";
import { ApiError, apiGet } from "../lib/api/http";
import AccountDeactivation from "../features/account-deactivation";
import {
  useUserUsage,
  adaptLogRow,
  useLogs,
  useBackendRequests,
  quotaToNanoUsd,
  type LogRow,
} from "../lib/api/usage";
import { useAdminMetrics } from "../lib/api/admin";

import {
  WorkspaceLinks,
  workspaceNavigation,
} from "../features/workspace-navigation";

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
  const queryClient = useQueryClient();
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
  const platformAdmin = canManagePlatform({ role, session });
  const workspaceRoleLabel = platformAdmin
    ? c(...translatedMessage("platform_administrator_489c68ad11"))
    : c(...translatedMessage("customer_daa1d3a318"));
  const sessionScope = sessionQueryIdentity({ session, role });
  const isCurrentSession = () =>
    JSON.stringify(sessionScope) ===
    JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()));
  const settingsLabel = c(...translatedMessage("account_center_60e51179ff"));
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
  useTokenChoices();
  const [keyActionError, setKeyActionError] = useState("");
  const [keyOperation, setKeyOperation] = useState("");
  const keyActionPending = useRef(false);
  const keyWriteFailed = (error: unknown) => {
    if (!isCurrentSession()) return;
    setKeyActionError(
      error instanceof ApiError && error.status === 409
        ? c(
            ...translatedMessage(
              "key_or_configuration_changed_reload_before_co_d94e79d745",
            ),
          )
        : c(
            ...translatedMessage(
              "the_operation_is_unconfirmed_reload_keys_or_c_63f3b6a989",
            ),
          ),
    );
    setKeyOperation((error as { operationId?: string })?.operationId ?? "");
  };
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
  const balance = balanceKnown
    ? (walletQuery.data!.api_balance_nano ?? walletQuery.data!.balance_nano)
    : "0";
  const requests = useProductPreview((s) => s.requests);
  // Exports come from the backend /api/export endpoints.
  const createExportMut = useCreateExport();
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
  // Reauthentication unlocks this owner's encrypted key for five minutes.
  const revealGrants = useRef<Record<string, string>>({});
  const [revealedKeyIds, setRevealedKeyIds] = useState<Record<string, number>>(
    {},
  );
  const verifyAndReveal = async (
    keyId: string,
    password: string,
  ): Promise<boolean> => {
    if (tokensError || !keys.some((key) => key.id === keyId)) return false;
    try {
      const unlocked = await verifyTokenPassword(
        Number(keyId),
        password,
        sessionScope,
      );
      if (!isCurrentSession()) return false;
      revealGrants.current[keyId] = unlocked.grant;
      setPlaintextKeys((previous) => ({ ...previous, [keyId]: unlocked.key }));
      setRevealedKeyIds((state) => ({
        ...state,
        [keyId]: Math.min(unlocked.expires_at * 1000, Date.now() + 300000),
      }));
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

  const [chartMetric, setChartMetric] = useState<
    "requests" | "cost" | "latency"
  >("requests");
  const [selected, setSelected] = useState<PreviewRequest | null>(null);
  const previousTrace = useRef<string | null>(null);
  const linkedTrace = new URLSearchParams(location.search).get("trace");
  const linkedRequest = useQuery({
    queryKey: ["request-detail", sessionScope, linkedTrace],
    enabled: section === "requests" && !!linkedTrace,
    retry: false,
    queryFn: async ({ signal }) => {
      const row = await apiGet<LogRow>(
        `/api/log/${encodeURIComponent(linkedTrace!)}`,
        signal,
      );
      if (!isCurrentSession()) throw Error("Request account changed");
      return adaptLogRow(row) as PreviewRequest;
    },
  });
  useEffect(() => {
    if (linkedTrace && section === "requests") {
      setSelected(linkedRequest.data ?? null);
    } else if (previousTrace.current) {
      setSelected(null);
    }
    previousTrace.current = section === "requests" ? linkedTrace : null;
  }, [linkedTrace, section, linkedRequest.data]);
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
  const [exportStart, setExportStart] = useState(() =>
    billingDate(new Date(Date.now() - 6 * 86400000).toISOString()),
  );
  const [exportEnd, setExportEnd] = useState(() =>
    billingDate(new Date().toISOString()),
  );
  const [exportSelection, setExportSelection] = useState<{
    status?: string;
    query?: string;
    key_id?: number;
    model?: string;
    trace_id?: string;
  }>({});
  const [exportError, setExportError] = useState("");

  // Deactivate modal
  const [deactivateOpen, setDeactivateOpen] = useState(false);

  // Overview metrics use only the authenticated backend usage summary.
  const usageItems = usageQuery.data?.items ?? [];
  // Recent backend rows supply the optional latency display.
  const periodRows = useMemo(
    () =>
      periodRequests(
        backendRequests as PreviewRequest[],
        days === "7" ? 7 : 30,
      ),
    [backendRequests, days],
  );
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
    // No backend records are available.
    return [];
  }, [usageItems, requests, days]);

  const summary = useMemo(() => {
    if (usageItems.length) {
      const totalRequests = usageItems.reduce((sum, i) => sum + i.requests, 0);
      const totalQuota = usageItems.reduce((sum, i) => sum + i.quota, 0);
      const totalSuccess = usageItems.reduce(
        (sum, i) => sum + (i.success_count ?? 0),
        0,
      );
      return {
        count: totalRequests,
        cost: quotaToNanoUsd(totalQuota),
        // Zero successful requests must remain a zero success rate.
        success:
          totalRequests > 0
            ? Number(((totalSuccess / totalRequests) * 100).toFixed(2))
            : 0,
      };
    }
    // No backend records are available.
    return { count: 0, cost: "0", success: 0 };
  }, [usageItems, requests, days]);

  const isLowBalance = balanceKnown && BigInt(balance) < 5000000000n; // < $5.00
  const isZeroBalance = balanceKnown && BigInt(balance) <= 0n;

  const requestKeySecret = async (key: PreviewKey, intent: KeySecretAction) => {
    if (authPendingRef.current || !isCurrentSession()) return;
    if (revealedKeyIds[key.id] > Date.now() && revealGrants.current[key.id]) {
      authPendingRef.current = true;
      try {
        const unlocked = await revealTokenSecret(
          Number(key.id),
          revealGrants.current[key.id],
          sessionScope,
        );
        if (!isCurrentSession()) return;
        setPlaintextKeys((previous) => ({
          ...previous,
          [key.id]: unlocked.key,
        }));
        if (intent === "view")
          setVisibleKeyIds((visible) => ({ ...visible, [key.id]: true }));
        else void copyText(unlocked.key, t("keyCopied"), t("copyFailed"));
        return;
      } catch {
        if (!isCurrentSession()) return;
        delete revealGrants.current[key.id];
        setRevealedKeyIds((previous) => ({ ...previous, [key.id]: 0 }));
      } finally {
        authPendingRef.current = false;
      }
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
      const unlocked = await verifyTokenPassword(
        Number(authRevealKeyId),
        authPassword,
        sessionScope,
      );
      if (!isCurrentSession()) return;
      revealGrants.current[authRevealKeyId] = unlocked.grant;
      setPlaintextKeys((previous) => ({
        ...previous,
        [authRevealKeyId]: unlocked.key,
      }));
      setRevealedKeyIds((s) => ({
        ...s,
        [authRevealKeyId]: Math.min(
          unlocked.expires_at * 1000,
          Date.now() + 300000,
        ),
      }));
      if (authKeyIntent === "view")
        setVisibleKeyIds((visible) => ({
          ...visible,
          [authRevealKeyId]: true,
        }));
      else {
        void copyText(unlocked.key, t("keyCopied"), t("copyFailed"));
      }
      setAuthRevealKeyId(null);
      setAuthPassword("");
      setAuthError("");
    } catch (error) {
      setAuthError(
        error instanceof ApiError && error.status === 403
          ? t("invalidPassword")
          : error instanceof ApiError && error.status === 429
            ? c(
                ...translatedMessage(
                  "too_many_verification_attempts_try_again_late_270bbf7ad9",
                ),
              )
            : c(
                ...translatedMessage(
                  "verification_could_not_be_confirmed_retry_or__5a9e29d2b2",
                ),
              ),
      );
    } finally {
      authPendingRef.current = false;
      setAuthPending(false);
    }
  };

  const handleExportSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!validateBillingRange(exportStart, exportEnd)) {
      setExportError(t("max31DaysWarning"));
      return;
    }
    createExportMut.mutate(
      {
        type: exportType,
        start_date: exportStart,
        end_date: exportEnd,
        language: writtenLanguage(i18n.language as any),
        ...exportSelection,
      },
      {
        onSuccess: () => {
          setExportModalOpen(false);
          setExportError("");
          navigate("/console/exports");
        },
        onError: (err) => {
          setExportError(err instanceof Error ? err.message : String(err));
        },
      },
    );
  };

  const activeSection =
    workspaceNavigation.find((item) => item.path === section) ||
    workspaceNavigation[0];
  const systemNavigation = [
    {
      path: "model-pricing",
      label: c(...translatedMessage("standard_models_d7f00fd7f0")),
      Icon: CircleDollarSign,
    },
    {
      path: "channels",
      label: c(...translatedMessage("channels_eee07fd951")),
      Icon: Network,
    },
    {
      path: "service-groups",
      label: c(...translatedMessage("service_groups_0b4d78d515")),
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
  // Usage reports load their own owner-scoped aggregate query.
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
            <strong>{c(...translatedMessage("console_1d5ce558be"))}</strong>
            <small>{workspaceRoleLabel}</small>
          </div>
        </div>
        <nav
          ref={sidebarNav}
          className="xy-sidebar-nav"
          aria-label={t("pNavigation")}
        >
          <WorkspaceLinks
            section={section}
            keysNearQuota={keysNearQuota.length}
            onNavigate={() => setMobileNavOpen(false)}
          />
          {canReadFinance(role) && (
            <Link
              to="/finance"
              className="xy-nav-link"
              onClick={() => setMobileNavOpen(false)}
            >
              <CircleDollarSign size={18} />
              <span>
                {c(...translatedMessage("finance_overview_f47c7aaf02"))}
              </span>
            </Link>
          )}
          {platformAdmin && (
            <>
              <span className="xy-nav-label xy-nav-divider">
                {c(...translatedMessage("platform_management_4b7a0c4faa"))}
              </span>
              <Link
                to="/admin"
                className="xy-nav-link"
                data-platform-management
                onClick={() => setMobileNavOpen(false)}
              >
                <Settings size={18} />
                <span>
                  {c(...translatedMessage("platform_management_4b7a0c4faa"))}
                </span>
                <span className="xy-nav-badge">
                  {c(...translatedMessage("administrator_5ccdea960a"))}
                </span>
              </Link>
            </>
          )}
          {isSystemAdmin && (
            <>
              <span className="xy-nav-label xy-nav-divider">
                {c(...translatedMessage("system_settings_4c9b4bdb1e"))}
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
              {c(...translatedMessage("build_your_next_big_idea_1a3ad28025"))}
            </strong>
            <p>
              {c(
                ...translatedMessage(
                  "everything_you_need_to_ship_with_ai_7dd8cafdfa",
                ),
              )}
            </p>
            <Link to="/models">
              {c(...translatedMessage("explore_models_6d7b3cc60c"))}
              <ArrowRight size={15} />
            </Link>
          </div>
          <Link className="xy-profile" to="/console/settings">
            <span className="xy-profile-avatar" aria-hidden="true">
              {profileInitials}
            </span>
            <div>
              <strong>{profileName}</strong>
              <small>{workspaceRoleLabel}</small>
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
              aria-label={c(...translatedMessage("open_navigation_77b8073ea6"))}
              aria-expanded={mobileNavOpen}
              onClick={() => setMobileNavOpen(true)}
            >
              <Menu size={20} />
            </button>
            <span>{c(...translatedMessage("console_1d5ce558be"))}</span>
            <ChevronRight size={14} className="directional" />
            <strong>
              {isRechargePage
                ? c(...translatedMessage("add_credits_7952fdf706"))
                : section === "settings"
                  ? settingsLabel
                  : systemSection?.label || t(activeSection.key)}
            </strong>
          </div>
          <div className="xy-topbar-actions">
            <ProductSearch />
            <Link className="xy-system-link" to="/status">
              <span
                className={`xy-status-dot ${(() => {
                  const reqs = metricsQuery.data?.requests ?? [];
                  const total = reqs.reduce((s, r) => s + r.count, 0);
                  const errs = reqs
                    .filter(
                      (r) =>
                        r.class !== "success" && r.class !== "client_error",
                    )
                    .reduce((s, r) => s + r.count, 0);
                  return total > 0 && errs / total > 0.01 ? "is-warning" : "";
                })()}`}
              />
              {c(...translatedMessage("system_status_cca6c51b95"))}
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
                  ...translatedMessage(
                    "system_administrator_access_required_f8aef00eba",
                  ),
                )}
              </h1>
              <p>
                {c(
                  ...translatedMessage(
                    "the_customer_demo_account_cannot_view_or_edit_83494f30cc",
                  ),
                )}
              </p>
              <Button asChild variant="secondary">
                <Link to="/console">
                  {c(...translatedMessage("back_to_overview_730e411a05"))}
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
                      ...translatedMessage(
                        "your_workspace_at_a_glance_435ab92061",
                      ),
                    )}
                  </div>
                  <h1>{c(...translatedMessage("overview_3f81a09f70"))}</h1>
                  <p>
                    {c(
                      ...translatedMessage(
                        "welcome_back_here_s_how_your_models_are_perfo_4b106d25c3",
                      ),
                    )}
                  </p>
                </div>
                <div className="xy-heading-actions">
                  <button
                    className="xy-secondary"
                    onClick={() => navigate("/console/connect")}
                  >
                    <BookOpen size={16} />
                    {c(...translatedMessage("quickstart_2785ea5326"))}
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
                          ...translatedMessage(
                            "a_few_simple_steps_to_your_first_model_reques_704ddda2d8",
                          ),
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
                          ...translatedMessage(
                            "create_or_reuse_a_key_69f5ae8ce2",
                          ),
                        ),
                        Icon: KeyRound,
                        action: () => setCreateOpen(true),
                      },
                      {
                        title: c(
                          ...translatedMessage(
                            "configure_your_tool_f83dd14509",
                          ),
                        ),
                        Icon: BookOpen,
                        action: () => navigate("/console/connect"),
                      },
                      {
                        title: c(
                          ...translatedMessage(
                            "add_prepaid_balance_3d9c5b11a8",
                          ),
                        ),
                        Icon: CreditCard,
                        action: () => openRecharge(),
                      },
                      {
                        title: c(
                          ...translatedMessage(
                            "view_calls_and_costs_09586d4f9d",
                          ),
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
                    {c(...translatedMessage("no_requests_yet_091d5c50e7"))}
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
                      ? c(...translatedMessage("quota_exhausted_41294054cf"))
                      : c(
                          ...translatedMessage(
                            "quota_nearly_exhausted_59ed17805f",
                          ),
                        )}{" "}
                    →
                  </Link>
                ))}
              </div>
              <div className="xy-metrics">
                {[
                  {
                    label: c(...translatedMessage("total_requests_0013914751")),
                    value: summary.count.toLocaleString(),
                    note: c(
                      ...translatedMessage(
                        "requests_in_the_last_days_eddf7e7ad8",
                        { p0: days },
                      ),
                    ),
                    Icon: Activity,
                    series: chartActivity.map((item) => item.requests),
                    color: "blue",
                  },
                  {
                    label: c(...translatedMessage("total_spend_fa745f7b25")),
                    value: formatMoney(summary.cost, "total"),
                    note: c(
                      ...translatedMessage(
                        "pay_only_for_what_you_use_1efdd9571b",
                      ),
                    ),
                    Icon: Wallet,
                    series: chartActivity.map((item) => item.cost),
                    color: "purple",
                  },
                  {
                    label: c(...translatedMessage("success_rate_f2722e5f55")),
                    value: summary.count ? `${summary.success}%` : "—",
                    note: c(
                      ...translatedMessage(
                        "successful_requests_this_period_e01e66f03a",
                      ),
                    ),
                    Icon: CircleCheck,
                    series: chartActivity.map((item) =>
                      item.requests > 0 ? 100 : 0,
                    ),
                    color: "green",
                  },
                  {
                    label: c(...translatedMessage("active_keys_cb353d19d0")),
                    value: String(activeKeyCount).padStart(2, "0"),
                    note: c(
                      ...translatedMessage("keys_in_your_account_f48ee66268", {
                        p0: keys.length,
                      }),
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
                          {c(...translatedMessage("usage_overview_0d9957d956"))}
                        </h2>
                        <p>
                          {c(
                            ...translatedMessage(
                              "a_clearer_picture_of_every_request_65dcf75b17",
                            ),
                          )}
                        </p>
                      </div>
                      <div
                        className="xy-period-tabs"
                        aria-label={c(
                          ...translatedMessage("time_period_f38f25c2ff"),
                        )}
                      >
                        {(["7", "30"] as const).map((day) => (
                          <button
                            key={day}
                            className={days === day ? "is-active" : ""}
                            onClick={() => setDays(day)}
                          >
                            {c(
                              ...translatedMessage("days_aaef666749", {
                                p0: day,
                              }),
                            )}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div className="xy-chart-summary">
                      <strong>{overviewMetric}</strong>
                      <div className="xy-chart-legend">
                        <span />
                        {chartMetric === "requests"
                          ? c(...translatedMessage("requests_01bf71f145"))
                          : chartMetric === "cost"
                            ? c(...translatedMessage("spend_ce2b22ebd6"))
                            : c(
                                ...translatedMessage(
                                  "average_latency_c010c3241b",
                                ),
                              )}
                      </div>
                    </div>
                    <div className="xy-chart-controls">
                      {(
                        [
                          {
                            id: "requests",
                            label: c(
                              ...translatedMessage("requests_fe89af9421"),
                            ),
                          },
                          {
                            id: "cost",
                            label: c(...translatedMessage("spend_d3c8580068")),
                          },
                          {
                            id: "latency",
                            label: c(
                              ...translatedMessage("latency_0b764a38e9"),
                            ),
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
                                ? c(...translatedMessage("requests_fe89af9421"))
                                : chartMetric === "cost"
                                  ? c(...translatedMessage("spend_d3c8580068"))
                                  : c(
                                      ...translatedMessage(
                                        "latency_ms_7766a3ece9",
                                      ),
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
                          {c(
                            ...translatedMessage("recent_requests_328e086b9f"),
                          )}
                        </h2>
                        <p>
                          {c(
                            ...translatedMessage(
                              "track_status_and_cost_down_to_the_request_95d5a60d2f",
                            ),
                          )}
                        </p>
                      </div>
                      <Link to="/console/requests" className="xy-text-link">
                        {c(...translatedMessage("view_all_990b081362"))}
                        <ArrowUpRight size={15} />
                      </Link>
                    </div>
                    <div className="xy-table-scroll">
                      <ResponsiveTable>
                        <thead>
                          <tr>
                            <th>
                              {c(...translatedMessage("model_9c2e5b5065"))}
                            </th>
                            <th>
                              {c(...translatedMessage("time_dubai_e0a70fcde3"))}
                            </th>
                            <th>
                              {c(...translatedMessage("status_dda39dcd63"))}
                            </th>
                            <th>
                              {c(...translatedMessage("cost_d118b228e2"))}
                            </th>
                            <th>
                              <span className="sr-only">
                                {c(
                                  ...translatedMessage(
                                    "view_details_8c9210e282",
                                  ),
                                )}
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
                                {new Date(
                                  log.created_at * 1000,
                                ).toLocaleTimeString("en-GB", {
                                  timeZone: "Asia/Dubai",
                                  hour12: false,
                                })}
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
                                  aria-label={c(
                                    ...translatedMessage(
                                      "view_request_details_c18f0afee3",
                                    ),
                                  )}
                                >
                                  <ArrowUpRight size={16} />
                                </Link>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </ResponsiveTable>
                      {!backendLogs.length && !logsQuery.isLoading && (
                        <p className="xy-empty">
                          {c(
                            ...translatedMessage(
                              "no_requests_yet_configure_your_preferred_tool_68e7e2a095",
                            ),
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
                        {c(...translatedMessage("prepaid_f3ae72a6f3"))}
                      </span>
                    </div>
                    <strong
                      tabIndex={balanceKnown ? 0 : undefined}
                      style={{
                        maxWidth: "100%",
                        overflowX: "auto",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {balanceKnown ? formatMoney(balance) : "—"}
                    </strong>
                    <p>
                      {c(
                        ...translatedMessage(
                          "pay_as_you_go_top_up_anytime_72b470da3a",
                        ),
                      )}
                    </p>
                    <div className="xy-wallet-art" aria-hidden="true">
                      <i />
                      <i />
                      <i />
                    </div>
                    <button onClick={() => openRecharge()}>
                      <Plus size={17} />
                      {c(...translatedMessage("add_funds_f196c2cc2b"))}
                      <ArrowUpRight size={16} />
                    </button>
                  </section>
                  <section className="xy-card xy-health-card">
                    <div className="xy-card-heading">
                      <h2>
                        {c(...translatedMessage("service_health_6d7da75c52"))}
                      </h2>
                      <Link
                        to="/status"
                        aria-label={c(
                          ...translatedMessage(
                            "view_service_status_12eb01ae5b",
                          ),
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
                            r.class !== "success" && r.class !== "client_error",
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
                                      ...translatedMessage(
                                        "no_requests_yet_806c7066f6",
                                      ),
                                    )
                                  : hasIssue
                                    ? c(
                                        ...translatedMessage(
                                          "upstream_degraded_d3bd34cd79",
                                        ),
                                      )
                                    : c(
                                        ...translatedMessage(
                                          "operational_910edf7393",
                                        ),
                                      )}
                              </strong>
                              <small>
                                {total > 0
                                  ? c(
                                      ...translatedMessage(
                                        "requests_errors_0fde61764c",
                                        { p0: total, p1: errors },
                                      ),
                                    )
                                  : c(
                                      ...translatedMessage(
                                        "awaiting_first_request_ed061b0e23",
                                      ),
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
                              {c(
                                ...translatedMessage(
                                  "platform_status_c618d713b5",
                                ),
                              )}
                            </span>
                            <strong>
                              {!total
                                ? c(
                                    ...translatedMessage(
                                      "no_requests_unverified_334f31ff47",
                                    ),
                                  )
                                : hasIssue
                                  ? c(
                                      ...translatedMessage(
                                        "upstream_issues_detected_1d3f4b7e0a",
                                      ),
                                    )
                                  : c(
                                      ...translatedMessage(
                                        "see_request_results_521b301187",
                                      ),
                                    )}
                            </strong>
                          </div>
                          <div className="xy-health-divider" />
                          <div className="xy-health-line">
                            <span>
                              {c(
                                ...translatedMessage(
                                  "gateway_region_79a6056067",
                                ),
                              )}
                            </span>
                            <strong>
                              {c(...translatedMessage("dubai_a52ed72a44"))}
                              <span className="xy-status-dot" />
                            </strong>
                          </div>
                          <div className="xy-health-line">
                            <span>
                              {c(
                                ...translatedMessage("active_keys_867ef24d2f"),
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
                      {c(...translatedMessage("keep_building_9b93f3454a"))}
                    </h2>
                    <button onClick={() => navigate("/console/playground")}>
                      <span className="xy-shortcut-icon">
                        <Terminal size={18} />
                      </span>
                      <span>
                        <strong>
                          {c(
                            ...translatedMessage("open_playground_53efa9ebc7"),
                          )}
                        </strong>
                        <small>
                          {c(
                            ...translatedMessage(
                              "try_a_model_in_seconds_5edf41b92b",
                            ),
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
                          {c(
                            ...translatedMessage(
                              "integration_guide_7a843f9768",
                            ),
                          )}
                        </strong>
                        <small>
                          {c(
                            ...translatedMessage(
                              "get_started_in_a_few_lines_ac3b28ce1d",
                            ),
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
                      ...translatedMessage(
                        "some_keys_are_near_or_at_their_spending_limit_053952af4c",
                      ),
                    )}
                  </span>
                  <Link to="/console/keys">
                    {c(...translatedMessage("review_keys_7fd0b44d67"))}
                    <ArrowRight size={14} />
                  </Link>
                </div>
              )}
              <footer className="xy-dashboard-footer">
                <span>
                  © 2026{" "}
                  {c(
                    ...translatedMessage(
                      "myapis_intelligence_within_reach_d27831d1ff",
                    ),
                  )}
                </span>
                <span>
                  {c(...translatedMessage("demo_data_c6e167f020"))}
                  {isSystemAdmin && (
                    <>
                      <i />{" "}
                      <Link to="/console/model-pricing">
                        {c(...translatedMessage("system_settings_966a9e1755"))}
                      </Link>
                    </>
                  )}
                </span>
              </footer>
            </div>
          )}

          {section === "keys" && keyActionError && (
            <div role="alert" className="key-list-empty">
              <p>{keyActionError}</p>
              <Button onClick={() => window.location.reload()}>
                {c(...translatedMessage("reload_keys_87b3e090af"))}
              </Button>
              {keyOperation && (
                <Button
                  onClick={async () => {
                    try {
                      const receipt = await readOwnedKeyReceipt(
                        keyOperation,
                        sessionScope,
                      );
                      if (isCurrentSession())
                        setKeyActionError(
                          c(
                            ...translatedMessage(
                              "operation_confirmed_for_key_plaintext_is_not__251d552505",
                              { p0: receipt.token_id },
                            ),
                          ),
                        );
                    } catch {
                      if (isCurrentSession())
                        setKeyActionError(
                          c(
                            ...translatedMessage(
                              "no_confirmed_receipt_found_this_does_not_prov_d2a47b4b4c",
                            ),
                          ),
                        );
                    }
                  }}
                >
                  {c(
                    ...translatedMessage("check_operation_receipt_7c28e8096d"),
                  )}
                </Button>
              )}
            </div>
          )}
          {/* SECTION: KEYS */}
          {section === "keys" &&
            (tokensLoading ? (
              <div className="key-list-empty">
                <KeyRound size={28} />
                <h2>{c(...translatedMessage("loading_4c20385375"))}</h2>
              </div>
            ) : tokensError ? (
              <div className="key-list-empty">
                <KeyRound size={28} />
                <h2>
                  {c(...translatedMessage("failed_to_load_keys_6034ad4a82"))}
                </h2>
                <p>
                  {c(
                    ...translatedMessage(
                      "check_your_connection_or_sign_in_again_15220283f2",
                    ),
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
                  navigate(
                    `/console/connect?key=${encodeURIComponent(key.id)}`,
                  );
                }}
                onToggle={(key) => {
                  if (keyActionPending.current) return;
                  keyActionPending.current = true;
                  setKeyActionError("");
                  updateTokenMut.mutate(
                    {
                      id: Number(key.id),
                      status: key.active ? 2 : 1,
                      expected_revision: key.managementRevision,
                    },
                    {
                      onError: keyWriteFailed,
                      onSettled: () => {
                        keyActionPending.current = false;
                      },
                    },
                  );
                }}
                onRegenerate={(key) =>
                  setKeyConfirmation({ action: "regenerate", key })
                }
                onDelete={(key) =>
                  setKeyConfirmation({ action: "delete", key })
                }
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
                onRelayComplete={() => {
                  void refreshRequests();
                  void walletQuery.refetch();
                  void queryClient.invalidateQueries({ queryKey: ["tokens"] });
                }}
                key={location.key}
                initialModel={initialCreateModel}
                onRecord={setSelected}
                onCreate={() => setCreateOpen(true)}
              />
            </div>
          )}

          {/* SECTION: USAGE REPORTS */}
          {section === "usage" && <UsageReports />}

          {/* SECTION: REQUESTS */}
          {section === "requests" && (
            <RequestHistoryPage
              timezone={walletQuery.data?.timezone}
              onSelect={setSelected}
              onExport={(selection) => {
                if (selection) {
                  setExportStart(selection.start_date);
                  setExportEnd(selection.end_date);
                  setExportSelection({
                    status: selection.status,
                    query: selection.query,
                    key_id: selection.key_id,
                    model: selection.model,
                    trace_id: selection.trace_id,
                  });
                }
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
            <ExportHistory
              onCreate={() => {
                setExportSelection({});
                setExportStart(
                  billingDate(
                    new Date(Date.now() - 6 * 86400000).toISOString(),
                    walletQuery.data?.timezone,
                  ),
                );
                setExportEnd(
                  billingDate(
                    new Date().toISOString(),
                    walletQuery.data?.timezone,
                  ),
                );
                setExportModalOpen(true);
              }}
            />
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
                ...translatedMessage(
                  "enter_your_current_account_password_330a4bccdf",
                ),
              )}
              value={authPassword}
              disabled={authPending}
              aria-invalid={Boolean(authError)}
              aria-describedby={authError ? "key-auth-error" : undefined}
              onChange={(e) => setAuthPassword(e.target.value)}
              required
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={authPending}
            aria-busy={authPending}
          >
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
            ? `${usageKey.name} · ${c(...translatedMessage("key_details_85c8df0bff"))}`
            : c(...translatedMessage("key_details_85c8df0bff"))
        }
        description={c(
          ...translatedMessage(
            "review_credential_status_spending_limits_and__2bffc8530a",
          ),
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
        description={c(
          ...translatedMessage(
            "choose_up_to_31_calendar_days_in_current_filt_c618dea4b8",
            { p0: walletQuery.data?.timezone ?? "Asia/Dubai" },
          ),
        )}
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
              onChange={(e) => {
                setExportType(
                  e.target.value as "requests" | "ledger" | "usage",
                );
                setExportSelection({});
              }}
              className="select-trigger"
              style={{ width: "100%" }}
            >
              <option value="requests">{t("requestsCsv")}</option>
              <option value="ledger">{t("ledgerCsv")}</option>
              <option value="usage">{t("usageCsv")}</option>
            </select>
          </div>

          <div className="verify-field-row">
            <div>
              <label htmlFor="export-task-start">
                {c(...translatedMessage("start_date_761834f95f"))}
              </label>
              <Input
                id="export-task-start"
                type="date"
                required
                value={exportStart}
                onChange={(e) => setExportStart(e.target.value)}
              />
            </div>
            <div>
              <label htmlFor="export-task-end">
                {c(...translatedMessage("end_date_ddebce9446"))}
              </label>
              <Input
                id="export-task-end"
                type="date"
                required
                value={exportEnd}
                onChange={(e) => setExportEnd(e.target.value)}
              />
            </div>
          </div>

          <Button
            type="submit"
            variant="primary"
            disabled={createExportMut.isPending}
          >
            {c(...translatedMessage("generate_csv_export_115628914c"))}
          </Button>
        </form>
      </Modal>

      {deactivateOpen && (
        <AccountDeactivation onClose={() => setDeactivateOpen(false)} />
      )}

      <ConfirmDialog
        open={keyConfirmation !== null}
        onOpenChange={(open) => {
          if (!open) setKeyConfirmation(null);
        }}
        title={
          keyConfirmation?.action === "delete"
            ? c(...translatedMessage("delete_this_key_0b6049e022"))
            : c(...translatedMessage("regenerate_this_key_d136f6cefe"))
        }
        description={
          keyConfirmation?.action === "delete"
            ? c(
                ...translatedMessage(
                  "deleting_stops_calls_from_apps_using_it_this__34a9e07812",
                  { p0: keyConfirmation.key.name },
                ),
              )
            : c(
                ...translatedMessage(
                  "regenerating_immediately_invalidates_the_old__c7c4fe0054",
                  { p0: keyConfirmation?.key.name ?? "" },
                ),
              )
        }
        confirmLabel={
          keyConfirmation?.action === "delete"
            ? c(...translatedMessage("delete_key_90a1ff06c7"))
            : c(...translatedMessage("regenerate_key_230088f779"))
        }
        onConfirm={() => {
          if (!keyConfirmation || keyActionPending.current) return;
          keyActionPending.current = true;
          setKeyActionError("");
          if (keyConfirmation.action === "delete") {
            deleteTokenMut.mutate(
              {
                id: Number(keyConfirmation.key.id),
                expected_revision: keyConfirmation.key.managementRevision,
              },
              {
                onError: keyWriteFailed,
                onSettled: () => {
                  keyActionPending.current = false;
                },
              },
            );
          } else {
            regenerateTokenMut.mutate(
              {
                id: Number(keyConfirmation.key.id),
                expected_revision: keyConfirmation.key.managementRevision,
              },
              {
                onError: keyWriteFailed,
                onSettled: () => {
                  keyActionPending.current = false;
                },
                onSuccess: (token) => {
                  const id = String(token.id);
                  setPlaintextKeys((s) => ({ ...s, [id]: token.key }));
                  setRegeneratedKey({
                    name: keyConfirmation.key.name,
                    plaintext: token.key,
                  });
                },
              },
            );
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
        title={c(...translatedMessage("key_regenerated_e1e7e715c2"))}
        description={c(
          ...translatedMessage(
            "the_old_key_is_invalid_update_your_apps_with__a160572072",
          ),
        )}
        footer={
          <Button variant="primary" onClick={() => setRegeneratedKey(null)}>
            {c(...translatedMessage("i_ve_saved_it_c12a651b84"))}
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
        timezone={walletQuery.data?.timezone}
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
