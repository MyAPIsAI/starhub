import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Users,
  Cpu,
  BarChart3,
  RefreshCw,
  Radio,
  History,
  Plus,
  CircleDollarSign,
} from "lucide-react";
import {
  ProductBrand,
  Preferences,
  PreviewNote,
} from "../../components/product-shell";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/primitives";
import { Modal } from "../../components/ui/overlay";
import { ModelPricingAdmin } from "../../features/model-pricing-admin";
import { useSystemAccess } from "../../lib/system-access";
import {
  useAdminUsers,
  useUpdateUser,
  useAdminAudit,
  useAdminMetrics,
  useAdminChannels,
  useTestChannel,
  userIsActive,
  userStatusText,
  quotaToUsd,
  type AdminUser,
  type ChannelTestResult,
} from "../../lib/api/admin";
import {
  useCreateChannel,
  useUpdateChannel,
  useDeleteChannel,
  type ChannelInput,
} from "../../lib/api/channel";
import {
  useRedemptions,
  useCreateRedemption,
  useDeleteRedemption,
  type Redemption,
} from "../../lib/api/redemption";
import { ApiError } from "../../lib/api/http";

export default function AdminPortal() {
  const { t, i18n } = useTranslation();
  const location = useLocation();
  const lang = i18n.language.startsWith("zh")
    ? "zh"
    : i18n.language.startsWith("ar")
      ? "ar"
      : "en";
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;

  const [role, setRole] = useState<"superAdmin" | "finance" | "ops" | "tech">(
    "superAdmin",
  );
  const authRole = useSystemAccess((s) => s.role);
  const [tab, setTab] = useState<
    | "customers"
    | "models"
    | "analytics"
    | "resources"
    | "incidents"
    | "audit"
    | "redemption"
    | "finance"
  >(() =>
    new URLSearchParams(location.search).get("tab") === "models"
      ? "models"
      : "customers",
  );
  useEffect(() => {
    if (new URLSearchParams(location.search).get("tab") === "models")
      setTab("models");
  }, [location.search]);

  // Backend API hooks.
  const usersQuery = useAdminUsers();
  const updateUserMut = useUpdateUser();
  const auditQuery = useAdminAudit(1, 20);
  const metricsQuery = useAdminMetrics();
  const channelsQuery = useAdminChannels();
  const testChannelMut = useTestChannel();
  const createChannelMut = useCreateChannel();
  const updateChannelMut = useUpdateChannel();
  const deleteChannelMut = useDeleteChannel();
  const redemptionsQuery = useRedemptions();
  const createRedemptionMut = useCreateRedemption();
  const deleteRedemptionMut = useDeleteRedemption();
  const queryClient = useQueryClient();

  // Channel test result per channel id (cleared on next test).
  const [testResults, setTestResults] = useState<
    Record<number, ChannelTestResult | string>
  >({});

  // Channel create/edit modal state.
  const [channelModalOpen, setChannelModalOpen] = useState(false);
  const [editingChannel, setEditingChannel] = useState<number | null>(null);
  const [channelForm, setChannelForm] = useState<ChannelInput>({
    name: "",
    key: "",
    models: "",
    group: "default",
    base_url: "",
    key_mode: "",
    type: 1,
    status: 1,
    priority: 0,
    weight: 1,
  });

  // Redemption create modal state.
  const [redeemModalOpen, setRedeemModalOpen] = useState(false);
  const [redeemQuota, setRedeemQuota] = useState("10");
  const [redeemCount, setRedeemCount] = useState("1");
  const [redeemNotice, setRedeemNotice] = useState("");

  // Incidents have no backend endpoint — keep as local state.
  const [incidents, setIncidents] = useState<
    {
      id: string;
      title: { en: string; zh: string; ar: string };
      impactedVendor: string;
      status: string;
      updatedAt: string;
      content: { en: string; zh: string; ar: string };
    }[]
  >([]);

  // Top-up modal state — targets a specific user.
  const [topUpOpen, setTopUpOpen] = useState(false);
  const [topUpTarget, setTopUpTarget] = useState<AdminUser | null>(null);
  const [detailUser, setDetailUser] = useState<AdminUser | null>(null);
  const [topUpAmount, setTopUpAmount] = useState("1200");
  const [topUpType, setTopUpType] = useState<
    "gift" | "compensation" | "offline_payment" | "clawback"
  >("offline_payment");
  const [topUpReason, setTopUpReason] = useState(
    "Account payment reference #XY-2026-108",
  );
  const [topUpNotice, setTopUpNotice] = useState("");

  // Force sync state
  const [syncing, setSyncing] = useState(false);
  const [syncedMessage, setSyncedMessage] = useState("");

  // Incident modal state
  const [incidentOpen, setIncidentOpen] = useState(false);
  const [incTitle, setIncTitle] = useState("");
  const [incVendor, setIncVendor] = useState("OpenAI");
  const [incContent, setIncContent] = useState("");

  const handleTopUpSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!topUpTarget) return;
    const amt = parseFloat(topUpAmount);
    if (isNaN(amt) || amt === 0) return;

    const currentQuota = topUpTarget.quota;
    const deltaNano = Math.round(amt * 1e9);
    const newQuota =
      topUpType === "clawback"
        ? Math.max(0, currentQuota + deltaNano)
        : currentQuota + deltaNano;

    setTopUpNotice("");
    updateUserMut.mutate(
      {
        id: topUpTarget.id,
        quota: newQuota,
        expected_quota: currentQuota,
      },
      {
        onSuccess: () => {
          setTopUpNotice(
            text(
              `已为 ${topUpTarget.username} ${topUpType === "clawback" ? "扣除" : "入账"} $${Math.abs(amt).toFixed(2)}。`,
              `Credited $${Math.abs(amt).toFixed(2)} to ${topUpTarget.username}.`,
              `تم إيداع $${Math.abs(amt).toFixed(2)} في حساب ${topUpTarget.username}.`,
            ),
          );
          setTopUpOpen(false);
        },
        onError: (err) => {
          const msg =
            err instanceof ApiError
              ? err.message
              : text("操作失败", "Operation failed", "فشلت العملية");
          setTopUpNotice(msg);
        },
      },
    );
  };

  const handleToggleStatus = (user: AdminUser) => {
    const nextStatus = userIsActive(user.status) ? 2 : 1;
    updateUserMut.mutate({ id: user.id, status: nextStatus });
  };

  const handleForceSync = () => {
    setSyncing(true);
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ["admin-metrics"] }),
      queryClient.invalidateQueries({ queryKey: ["admin-channels"] }),
    ]).then(() => {
      setSyncing(false);
      setSyncedMessage(t("syncConfirmed"));
      setTimeout(() => setSyncedMessage(""), 4000);
    });
  };

  const handlePublishIncidentSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!incTitle || !incContent) return;
    setIncidents((prev) => [
      {
        id: `inc_${Date.now()}`,
        title: { en: incTitle, zh: incTitle, ar: incTitle },
        impactedVendor: incVendor,
        status: "investigating",
        updatedAt: text("刚刚", "Just now", "الآن"),
        content: { en: incContent, zh: incContent, ar: incContent },
      },
      ...prev,
    ]);
    setIncidentOpen(false);
    setIncTitle("");
    setIncContent("");
  };

  return (
    <div
      className="brand-home public-refined admin-refined"
      style={{
        minHeight: "100vh",
        display: "flex",
        flexDirection: "column",
        background: "var(--neutral-2)",
      }}
    >
      <PreviewNote />

      {/* Admin Darker Professional Header */}
      <header
        className="admin-header"
        style={{
          background: "var(--surface)",
          color: "var(--text)",
          padding: "var(--space-3) var(--space-6)",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          borderBottom: "1px solid var(--border)",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: "var(--space-4)",
          }}
        >
          <ProductBrand />
          <span
            style={{
              fontSize: "var(--text-xs)",
              background: "var(--brand)",
              color: "#fff",
              padding: "2px 8px",
              borderRadius: "var(--radius-sm)",
              fontWeight: 600,
            }}
          >
            {t("adminPortal")}
          </span>
        </div>

        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: "var(--space-4)",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "var(--space-2)",
              fontSize: "var(--text-xs)",
            }}
          >
            <span style={{ color: "var(--muted)" }}>{t("switchRole")}:</span>
            <select
              aria-label={t("switchRole")}
              value={role}
              onChange={(e) => setRole(e.target.value as any)}
              className="select-trigger"
              style={{
                height: 28,
                background: "var(--surface)",
                color: "var(--text)",
                border: "1px solid var(--border)",
                fontSize: 11,
                borderRadius: "var(--radius-sm)",
              }}
            >
              <option value="superAdmin">{t("roleSuperAdmin")}</option>
              <option value="finance">{t("roleFinance")}</option>
              <option value="ops">{t("roleOps")}</option>
              <option value="tech">{t("roleTech")}</option>
            </select>
          </div>

          <Preferences />

          <Button
            asChild
            size="small"
            variant="secondary"
            style={{ height: 28, fontSize: 12 }}
          >
            <Link to="/console">{t("pConsole")}</Link>
          </Button>
        </div>
      </header>

      {/* Admin Navigation Tabs */}
      <div
        className="admin-tabbar"
        style={{
          background: "var(--surface)",
          borderBottom: "1px solid var(--border)",
          padding: "0 var(--space-6)",
        }}
      >
        <div
          style={{
            display: "flex",
            gap: "var(--space-4)",
            maxWidth: 1200,
            margin: "0 auto",
            overflowX: "auto",
          }}
        >
          {[
            { id: "customers", label: t("adminCustomers"), Icon: Users, roles: ["superAdmin", "finance", "ops"] },
            { id: "models", label: t("adminModels"), Icon: Cpu, roles: ["superAdmin", "tech"] },
            { id: "finance", label: text("财务", "Finance", "المالية"), Icon: CircleDollarSign, roles: ["superAdmin", "finance"] },
            { id: "analytics", label: t("adminAnalytics"), Icon: BarChart3, roles: ["superAdmin", "finance", "tech"] },
            { id: "resources", label: t("adminResources"), Icon: RefreshCw, roles: ["superAdmin", "tech"] },
            { id: "incidents", label: t("adminIncidents"), Icon: Radio, roles: ["superAdmin", "ops"] },
            { id: "audit", label: t("adminAudit"), Icon: History, roles: ["superAdmin", "finance", "ops", "tech"] },
            { id: "redemption", label: text("兑换码", "Redemption", "استرداد"), Icon: CircleDollarSign, roles: ["superAdmin"] },
          ].filter(({ roles }) => roles.includes(role)).map(({ id, label, Icon }) => {
            const active = tab === id;
            return (
              <button
                key={id}
                onClick={() => setTab(id as any)}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  padding: "12px 4px",
                  background: "transparent",
                  border: "none",
                  borderBottom: active
                    ? "2px solid var(--brand)"
                    : "2px solid transparent",
                  color: active ? "var(--brand)" : "var(--secondary)",
                  fontWeight: active ? 600 : 500,
                  fontSize: "var(--text-sm)",
                  cursor: "pointer",
                }}
              >
                <Icon size={15} />
                <span>{label}</span>
              </button>
            );
          })}
        </div>
      </div>

      <main
        className="admin-content"
        style={{
          maxWidth: 1200,
          width: "100%",
          margin: "0 auto",
          padding: "var(--space-6)",
          flex: 1,
        }}
      >
        {topUpNotice && (
          <div
            style={{
              padding: "var(--space-3) var(--space-4)",
              background: "var(--warning-soft)",
              border: "1px solid var(--warning)",
              borderRadius: "var(--radius-md)",
              color: "var(--warning)",
              fontSize: "var(--text-sm)",
              marginBottom: "var(--space-6)",
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
            }}
          >
            <span>{topUpNotice}</span>
            <button
              onClick={() => setTopUpNotice("")}
              style={{
                background: "none",
                border: "none",
                cursor: "pointer",
                color: "inherit",
                fontWeight: "bold",
              }}
            >
              ×
            </button>
          </div>
        )}

        {/* Tab 1: Customers */}
        {tab === "customers" && (
          <div>
            <div
              className="admin-section-heading"
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
                  {t("adminCustomers")}
                </h1>
                <p
                  style={{
                    color: "var(--muted)",
                    fontSize: "var(--text-sm)",
                    margin: "4px 0 0 0",
                  }}
                >
                  {text(
                    "管理账号、额度、并发限制与手动加款。",
                    "Manage accounts, quotas, concurrency, and manual top-ups.",
                    "إدارة الحسابات والحصص والتزامن والشحن اليدوي.",
                  )}
                </p>
              </div>

              {(role === "finance" || role === "superAdmin") && (
                <Button
                  variant="primary"
                  onClick={() => {
                    const first = usersQuery.data?.[0];
                    if (first) {
                      setTopUpTarget(first);
                      setTopUpAmount("");
                      setTopUpReason("");
                      setTopUpOpen(true);
                    }
                  }}
                >
                  <Plus size={15} style={{ marginInlineEnd: 4 }} />
                  {t("manualTopUp")}
                </Button>
              )}
            </div>

            {/* Backend user list table */}
            <section
              style={{
                background: "var(--surface)",
                border: "1px solid var(--border)",
                borderRadius: "var(--radius-lg)",
                overflowX: "auto",
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
                  {text("用户列表", "User list", "قائمة المستخدمين")} (
                  {usersQuery.data?.length ?? 0})
                </h3>
              </div>
              <table
                style={{
                  width: "100%",
                  borderCollapse: "collapse",
                  fontSize: "var(--text-sm)",
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
                      {text("用户名", "Username", "اسم المستخدم")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("邮箱", "Email", "البريد")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {t("status")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("余额", "Balance", "الرصيد")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("已用", "Used", "المستخدم")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("分组", "Group", "المجموعة")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>
                      {text("操作", "Actions", "الإجراءات")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {usersQuery.isLoading && (
                    <tr>
                      <td
                        colSpan={7}
                        style={{
                          padding: "24px",
                          textAlign: "center",
                          color: "var(--muted)",
                        }}
                      >
                        {text("加载中…", "Loading…", "جارٍ التحميل…")}
                      </td>
                    </tr>
                  )}
                  {usersQuery.isError && (
                    <tr>
                      <td
                        colSpan={7}
                        style={{
                          padding: "24px",
                          textAlign: "center",
                          color: "var(--danger)",
                        }}
                      >
                        {text(
                          "加载失败",
                          "Failed to load",
                          "فشل التحميل",
                        )}
                      </td>
                    </tr>
                  )}
                  {usersQuery.data?.map((u) => (
                    <tr
                      key={u.id}
                      style={{ borderBottom: "1px solid var(--border)" }}
                    >
                      <td
                        style={{
                          padding: "10px 14px",
                          fontWeight: 500,
                        }}
                      >
                        {u.username}
                      </td>
                      <td
                        style={{
                          padding: "10px 14px",
                          fontSize: "var(--text-xs)",
                          color: "var(--muted)",
                        }}
                      >
                        {u.email || "—"}
                      </td>
                      <td style={{ padding: "10px 14px" }}>
                        <span
                          style={{
                            fontSize: 11,
                            padding: "2px 8px",
                            borderRadius: 4,
                            fontWeight: 600,
                            background: userIsActive(u.status)
                              ? "var(--success-soft)"
                              : "var(--danger-soft)",
                            color: userIsActive(u.status)
                              ? "var(--success)"
                              : "var(--danger)",
                          }}
                        >
                          {userStatusText(u.status)}
                        </span>
                      </td>
                      <td
                        className="mono"
                        style={{
                          padding: "10px 14px",
                          fontWeight: 600,
                          color: "var(--brand)",
                        }}
                      >
                        ${quotaToUsd(u.quota)}
                      </td>
                      <td
                        className="mono"
                        style={{
                          padding: "10px 14px",
                          fontSize: "var(--text-xs)",
                          color: "var(--muted)",
                        }}
                      >
                        ${quotaToUsd(u.used_quota)}
                      </td>
                      <td style={{ padding: "10px 14px" }}>
                        <span
                          style={{
                            fontSize: 11,
                            padding: "2px 6px",
                            background: "var(--subtle)",
                            borderRadius: 3,
                          }}
                        >
                          {u.group || "default"}
                        </span>
                      </td>
                      <td style={{ padding: "10px 14px", textAlign: "end" }}>
                        <div
                          style={{
                            display: "flex",
                            gap: 4,
                            justifyContent: "flex-end",
                          }}
                        >
                          <Button
                            size="small"
                            variant="secondary"
                            onClick={() => {
                              setTopUpTarget(u);
                              setTopUpAmount("");
                              setTopUpReason("");
                              setTopUpOpen(true);
                            }}
                          >
                            <Plus size={12} style={{ marginInlineEnd: 2 }} />
                            {text("加款", "Top-up", "شحن")}
                          </Button>
                          <Button
                            size="small"
                            variant={
                              userIsActive(u.status) ? "danger" : "primary"
                            }
                            onClick={() => handleToggleStatus(u)}
                            disabled={
                              updateUserMut.isPending &&
                              updateUserMut.variables?.id === u.id
                            }
                          >
                            {userIsActive(u.status)
                              ? t("disableAccount")
                              : t("enableAccount")}
                          </Button>
                          <Button
                            size="small"
                            variant="secondary"
                            onClick={() => {
                              setDetailUser(u);
                            }}
                          >
                            {text("详情", "Details", "التفاصيل")}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          </div>
        )}

        {/* Keep drafts mounted when switching admin sections. */}
        <div hidden={tab !== "models"}>
          <ModelPricingAdmin role={authRole} />
        </div>

        {/* Tab 3: Analytics */}
        {tab === "analytics" && (
          <div>
            <h1
              style={{
                fontSize: "var(--text-xl)",
                fontWeight: "var(--weight-bold)",
                marginBottom: "var(--space-4)",
              }}
            >
              {t("adminAnalytics")}
            </h1>
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
                  {text(
                    "总请求数",
                    "Total requests",
                    "إجمالي الطلبات",
                  )}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-2xl)",
                    fontWeight: "var(--weight-bold)",
                    color: "var(--brand)",
                  }}
                >
                  {(metricsQuery.data?.requests.reduce(
                    (sum, r) => sum + r.count,
                    0,
                  ) ?? 0).toLocaleString()}
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
                  {text("活跃用户", "Active users", "المستخدمون النشطون")}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-2xl)",
                    fontWeight: "var(--weight-bold)",
                  }}
                >
                  {(usersQuery.data?.filter((u) =>
                    userIsActive(u.status),
                  ).length ?? 0).toLocaleString()}
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
                  {text("渠道数", "Channels", "القنوات")}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-2xl)",
                    fontWeight: "var(--weight-bold)",
                  }}
                >
                  {channelsQuery.data?.length ?? 0}
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
                  {text("请求成功率", "Request success rate", "معدل نجاح الطلبات")}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-2xl)",
                    fontWeight: "var(--weight-bold)",
                    color: "var(--success)",
                  }}
                >
                  {(() => {
                    const reqs = metricsQuery.data?.requests ?? [];
                    const total = reqs.reduce((s, r) => s + r.count, 0);
                    const success =
                      reqs.find((r) => r.class === "success")?.count ?? 0;
                    return total > 0
                      ? ((success / total) * 100).toFixed(2) + "%"
                      : "—";
                  })()}
                </div>
              </div>
            </div>

            {/* Request breakdown by class */}
            {metricsQuery.data && metricsQuery.data.requests.length > 0 && (
              <section
                style={{
                  background: "var(--surface)",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius-lg)",
                  overflowX: "auto",
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
                    {text(
                      "请求分类统计",
                      "Request breakdown by class",
                      "تصنيف الطلبات حسب الفئة",
                    )}
                  </h3>
                </div>
                <table
                  style={{
                    width: "100%",
                    borderCollapse: "collapse",
                    fontSize: "var(--text-sm)",
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
                        {text("分类", "Class", "الفئة")}
                      </th>
                      <th style={{ padding: "10px 14px", textAlign: "start" }}>
                        {text("次数", "Count", "العدد")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {metricsQuery.data.requests.map((r) => (
                      <tr
                        key={r.class}
                        style={{ borderBottom: "1px solid var(--border)" }}
                      >
                        <td
                          style={{
                            padding: "10px 14px",
                            fontWeight: 500,
                          }}
                        >
                          {r.class}
                        </td>
                        <td
                          className="mono"
                          style={{ padding: "10px 14px" }}
                        >
                          {r.count.toLocaleString()}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>
            )}
          </div>
        )}

        {/* Tab 4: Resources & Sync */}
        {tab === "resources" && (
          <div>
            <div
              className="admin-section-heading"
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
                  {t("adminResources")}
                </h1>
                <p
                  style={{
                    color: "var(--muted)",
                    fontSize: "var(--text-sm)",
                    margin: "4px 0 0 0",
                  }}
                >
                  {text(
                    "查看资源快照同步情况与网关运行状态。",
                    "View resource snapshot synchronization and gateway health.",
                    "عرض مزامنة لقطات الموارد وحالة البوابة.",
                  )}
                </p>
              </div>

              {(role === "tech" || role === "superAdmin") && (
                <Button
                  variant="primary"
                  onClick={handleForceSync}
                  disabled={syncing}
                >
                  <RefreshCw
                    size={14}
                    className={syncing ? "spinning" : ""}
                    style={{ marginInlineEnd: 6 }}
                  />
                  {t("forceSync")}
                </Button>
              )}
            </div>

            {syncedMessage && (
              <div
                style={{
                  padding: "var(--space-3) var(--space-4)",
                  background: "var(--success-soft)",
                  color: "var(--success)",
                  borderRadius: "var(--radius-md)",
                  marginBottom: "var(--space-4)",
                  fontSize: "var(--text-sm)",
                }}
              >
                {syncedMessage}
              </div>
            )}

            {/* Channel list from backend */}
            <section
              style={{
                background: "var(--surface)",
                border: "1px solid var(--border)",
                borderRadius: "var(--radius-lg)",
                overflowX: "auto",
                marginBottom: "var(--space-6)",
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
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                    <span>
                      {text("渠道列表", "Channels", "القنوات")} (
                      {channelsQuery.data?.length ?? 0})
                    </span>
                    <Button
                      size="small"
                      variant="primary"
                      onClick={() => {
                        setEditingChannel(null);
                        setChannelForm({
                          name: "",
                          key: "",
                          models: "",
                          group: "default",
                          base_url: "",
                          key_mode: "",
                          type: 1,
                          status: 1,
                          priority: 0,
                          weight: 1,
                        });
                        setChannelModalOpen(true);
                      }}
                    >
                      <Plus size={14} style={{ marginInlineEnd: 4 }} />
                      {text("新建渠道", "New channel", "قناة جديدة")}
                    </Button>
                  </div>
                </h3>
              </div>
              <table
                style={{
                  width: "100%",
                  borderCollapse: "collapse",
                  fontSize: "var(--text-sm)",
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
                      {text("名称", "Name", "الاسم")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("模型", "Models", "النماذج")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {t("status")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("成功率", "Success rate", "معدل النجاح")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("平均 TTFT (ms)", "Avg TTFT (ms)", "متوسط TTFT (ملث)")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>
                      {text("测试", "Test", "اختبار")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>
                      {text("管理", "Manage", "إدارة")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {channelsQuery.isLoading && (
                    <tr>
                      <td
                        colSpan={7}
                        style={{
                          padding: "24px",
                          textAlign: "center",
                          color: "var(--muted)",
                        }}
                      >
                        {text("加载中…", "Loading…", "جارٍ التحميل…")}
                      </td>
                    </tr>
                  )}
                  {channelsQuery.data?.map((ch) => {
                    const stat = metricsQuery.data?.channel_stats.find(
                      (s) => s.channel_id === ch.id,
                    );
                    return (
                      <tr
                        key={ch.id}
                        style={{ borderBottom: "1px solid var(--border)" }}
                      >
                        <td
                          style={{
                            padding: "10px 14px",
                            fontWeight: 500,
                          }}
                        >
                          {ch.name}
                        </td>
                        <td
                          style={{
                            padding: "10px 14px",
                            fontSize: "var(--text-xs)",
                            color: "var(--muted)",
                            maxWidth: 200,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {ch.models}
                        </td>
                        <td style={{ padding: "10px 14px" }}>
                          <span
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              fontWeight: 600,
                              background:
                                ch.status === 1
                                  ? "var(--success-soft)"
                                  : "var(--danger-soft)",
                              color:
                                ch.status === 1
                                  ? "var(--success)"
                                  : "var(--danger)",
                            }}
                          >
                            {ch.status === 1
                              ? text("启用", "Enabled", "مُفعّل")
                              : text("禁用", "Disabled", "مُعطّل")}
                          </span>
                        </td>
                        <td
                          className="mono"
                          style={{ padding: "10px 14px" }}
                        >
                          {stat
                            ? (stat.success_rate * 100).toFixed(1) + "%"
                            : "—"}
                        </td>
                        <td
                          className="mono"
                          style={{ padding: "10px 14px" }}
                        >
                          {stat ? stat.avg_ttft_ms.toFixed(0) : "—"}
                        </td>
                        <td style={{ padding: "10px 14px", textAlign: "end" }}>
                          <div
                            style={{
                              display: "flex",
                              flexDirection: "column",
                              gap: 4,
                              alignItems: "flex-end",
                            }}
                          >
                            <Button
                              size="small"
                              variant="secondary"
                              disabled={
                                testChannelMut.isPending &&
                                testChannelMut.variables?.id === ch.id
                              }
                              onClick={() => {
                                setTestResults((s) => {
                                  const next = { ...s };
                                  delete next[ch.id];
                                  return next;
                                });
                                testChannelMut.mutate(
                                  { id: ch.id },
                                  {
                                    onSuccess: (res) =>
                                      setTestResults((s) => ({
                                        ...s,
                                        [ch.id]: res,
                                      })),
                                    onError: (err) =>
                                      setTestResults((s) => ({
                                        ...s,
                                        [ch.id]:
                                          err instanceof ApiError
                                            ? err.message
                                            : text(
                                                "测试失败",
                                                "Test failed",
                                                "فشل الاختبار",
                                              ),
                                      })),
                                  },
                                );
                              }}
                            >
                              {testChannelMut.isPending &&
                              testChannelMut.variables?.id === ch.id
                                ? text("测试中…", "Testing…", "اختبار…")
                                : text("测试连通", "Test", "اختبار")}
                            </Button>
                            {testResults[ch.id] &&
                              (typeof testResults[ch.id] === "string" ? (
                                <span
                                  style={{
                                    fontSize: 11,
                                    color: "var(--danger)",
                                  }}
                                >
                                  {testResults[ch.id] as string}
                                </span>
                              ) : (
                                <span
                                  style={{
                                    fontSize: 11,
                                    color: (testResults[ch.id] as ChannelTestResult).ok
                                      ? "var(--success)"
                                      : "var(--danger)",
                                  }}
                                >
                                  {(testResults[ch.id] as ChannelTestResult).ok
                                    ? `${text("成功", "OK", "نجح")} · ${(testResults[ch.id] as ChannelTestResult).elapsed_ms}ms`
                                    : text("失败", "Failed", "فشل")}
                                </span>
                              ))}
                          </div>
                        </td>
                        <td style={{ padding: "10px 14px", textAlign: "end" }}>
                          <div style={{ display: "flex", gap: 4, justifyContent: "flex-end" }}>
                            <Button
                              size="small"
                              variant="secondary"
                              onClick={() => {
                                setEditingChannel(ch.id);
                                setChannelForm({
                                  name: ch.name,
                                  key: "",
                                  models: ch.models,
                                  group: ch.group || "default",
                                  base_url: ch.base_url,
                                  key_mode: ch.key_mode,
                                  type: ch.type,
                                  status: ch.status,
                                  priority: ch.priority,
                                  weight: ch.weight,
                                });
                                setChannelModalOpen(true);
                              }}
                            >
                              {text("编辑", "Edit", "تعديل")}
                            </Button>
                            <Button
                              size="small"
                              variant="danger"
                              disabled={
                                deleteChannelMut.isPending &&
                                deleteChannelMut.variables === ch.id
                              }
                              onClick={() => {
                                if (!confirm(text("确认删除此渠道？", "Delete this channel?", "حذف هذه القناة؟"))) return;
                                deleteChannelMut.mutate(ch.id);
                              }}
                            >
                              {text("删除", "Delete", "حذف")}
                            </Button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </section>
          </div>
        )}

        {/* Tab 5: Incidents */}
        {tab === "incidents" && (
          <div>
            <div
              className="admin-section-heading"
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
                  {t("adminIncidents")}
                </h1>
                <p
                  style={{
                    color: "var(--muted)",
                    fontSize: "var(--text-sm)",
                    margin: "4px 0 0 0",
                  }}
                >
                  {text(
                    "管理展示在公开服务状态页的事件公告。",
                    "Manage incident notices on the public status page.",
                    "إدارة إشعارات الحوادث في صفحة الحالة العامة.",
                  )}
                </p>
              </div>

              {(role === "ops" || role === "superAdmin") && (
                <Button variant="primary" onClick={() => setIncidentOpen(true)}>
                  <Plus size={15} style={{ marginInlineEnd: 4 }} />
                  {t("publishIncident")}
                </Button>
              )}
            </div>

            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: "var(--space-3)",
              }}
            >
              {incidents.map((inc) => (
                <div
                  key={inc.id}
                  style={{
                    background: "var(--surface)",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-lg)",
                    padding: "var(--space-4)",
                  }}
                >
                  <div
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      marginBottom: 6,
                    }}
                  >
                    <h3
                      style={{
                        margin: 0,
                        fontSize: "var(--text-base)",
                        fontWeight: 600,
                      }}
                    >
                      {inc.title[lang] || inc.title.en}
                    </h3>
                    <span
                      style={{
                        fontSize: 11,
                        padding: "2px 8px",
                        background: "var(--subtle)",
                        borderRadius: 4,
                        textTransform: "capitalize",
                      }}
                    >
                      {inc.status}
                    </span>
                  </div>
                  <p
                    style={{
                      margin: "0 0 8px 0",
                      color: "var(--muted)",
                      fontSize: "var(--text-sm)",
                    }}
                  >
                    {inc.content[lang] || inc.content.en}
                  </p>
                  <div
                    className="mono"
                    style={{ fontSize: 11, color: "var(--muted)" }}
                  >
                    Vendor: {inc.impactedVendor} · {inc.updatedAt}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Tab 6: Audit Logs */}
        {tab === "audit" && (
          <div>
            <h1
              style={{
                fontSize: "var(--text-xl)",
                fontWeight: "var(--weight-bold)",
                marginBottom: "var(--space-4)",
              }}
            >
              {t("adminAudit")}
            </h1>
            <div
              style={{
                background: "var(--surface)",
                border: "1px solid var(--border)",
                borderRadius: "var(--radius-lg)",
                overflowX: "auto",
              }}
            >
              <table
                style={{
                  width: "100%",
                  borderCollapse: "collapse",
                  fontSize: "var(--text-sm)",
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
                      {text("时间", "Time", "الوقت")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("用户", "User", "المستخدم")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("方法", "Method", "الأسلوب")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("路径", "Path", "المسار")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("状态码", "Status", "الحالة")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("IP", "IP", "IP")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {auditQuery.isLoading && (
                    <tr>
                      <td
                        colSpan={6}
                        style={{
                          padding: "24px",
                          textAlign: "center",
                          color: "var(--muted)",
                        }}
                      >
                        {text("加载中…", "Loading…", "جارٍ التحميل…")}
                      </td>
                    </tr>
                  )}
                  {auditQuery.data?.items.map((log) => (
                    <tr
                      key={log.id}
                      style={{ borderBottom: "1px solid var(--border)" }}
                    >
                      <td
                        className="mono"
                        style={{ padding: "10px 14px", fontSize: 12 }}
                      >
                        {new Date(
                          log.created_at * 1000,
                        ).toLocaleString()}
                      </td>
                      <td
                        style={{ padding: "10px 14px", fontWeight: 500 }}
                      >
                        {log.username || `#${log.user_id}`}
                      </td>
                      <td
                        style={{
                          padding: "10px 14px",
                          fontSize: 12,
                        }}
                      >
                        {log.method}
                      </td>
                      <td
                        className="mono"
                        style={{
                          padding: "10px 14px",
                          fontSize: 12,
                          color: "var(--secondary)",
                        }}
                      >
                        {log.path}
                      </td>
                      <td style={{ padding: "10px 14px" }}>
                        <span
                          style={{
                            fontSize: 11,
                            padding: "2px 8px",
                            borderRadius: 4,
                            fontWeight: 600,
                            background:
                              log.status >= 200 && log.status < 300
                                ? "var(--success-soft)"
                                : "var(--danger-soft)",
                            color:
                              log.status >= 200 && log.status < 300
                                ? "var(--success)"
                                : "var(--danger)",
                          }}
                        >
                          {log.status || "—"}
                        </span>
                      </td>
                      <td
                        className="mono"
                        style={{
                          padding: "10px 14px",
                          fontSize: 12,
                          color: "var(--muted)",
                        }}
                      >
                        {log.ip || "—"}
                      </td>
                    </tr>
                  ))}
                  {auditQuery.data && auditQuery.data.items.length === 0 && (
                    <tr>
                      <td
                        colSpan={6}
                        style={{
                          padding: "24px",
                          textAlign: "center",
                          color: "var(--muted)",
                        }}
                      >
                        {text("暂无记录", "No records", "لا سجلات")}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
            {auditQuery.data && auditQuery.data.total > 20 && (
              <p
                style={{
                  textAlign: "center",
                  color: "var(--muted)",
                  fontSize: "var(--text-xs)",
                  marginTop: "var(--space-3)",
                }}
              >
                {text(
                  `显示前 20 条（共 ${auditQuery.data.total} 条）`,
                  `Showing 20 of ${auditQuery.data.total}`,
                  `عرض 20 من ${auditQuery.data.total}`,
                )}
              </p>
            )}
          </div>
        )}

        {/* Tab: Finance — billing overview for finance role */}
        {tab === "finance" && (
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
                <h1 style={{ fontSize: "var(--text-xl)", fontWeight: "var(--weight-bold)", margin: 0 }}>
                  {text("财务概览", "Finance overview", "نظرة مالية")}
                </h1>
                <p style={{ color: "var(--muted)", fontSize: "var(--text-sm)", margin: "4px 0 0 0" }}>
                  {text(
                    "查看平台充值、消费与流水汇总。",
                    "View platform top-ups, consumption and ledger summary.",
                    "عرض شحن المنصة والاستهلاك وسجل الأستاذ.",
                  )}
                </p>
              </div>
              <Button
                variant="primary"
                onClick={() => {
                  const first = usersQuery.data?.[0];
                  if (first) {
                    setTopUpTarget(first);
                    setTopUpAmount("");
                    setTopUpReason("");
                    setTopUpOpen(true);
                  }
                }}
              >
                <Plus size={15} style={{ marginInlineEnd: 4 }} />
                {t("manualTopUp")}
              </Button>
            </div>
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
                <div style={{ fontSize: "var(--text-xs)", color: "var(--muted)", marginBottom: 4 }}>
                  {text("活跃用户", "Active users", "المستخدمون النشطون")}
                </div>
                <div className="mono" style={{ fontSize: "var(--text-2xl)", fontWeight: "var(--weight-bold)" }}>
                  {(usersQuery.data?.filter((u: AdminUser) => userIsActive(u.status)).length ?? 0).toLocaleString()}
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
                <div style={{ fontSize: "var(--text-xs)", color: "var(--muted)", marginBottom: 4 }}>
                  {text("总用户", "Total users", "إجمالي المستخدمين")}
                </div>
                <div className="mono" style={{ fontSize: "var(--text-2xl)", fontWeight: "var(--weight-bold)" }}>
                  {(usersQuery.data?.length ?? 0).toLocaleString()}
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
                <div style={{ fontSize: "var(--text-xs)", color: "var(--muted)", marginBottom: 4 }}>
                  {text("兑换码", "Redemption codes", "أكواد الاسترداد")}
                </div>
                <div className="mono" style={{ fontSize: "var(--text-2xl)", fontWeight: "var(--weight-bold)" }}>
                  {(redemptionsQuery.data?.length ?? 0).toLocaleString()}
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
                <div style={{ fontSize: "var(--text-xs)", color: "var(--muted)", marginBottom: 4 }}>
                  {text("渠道数", "Channels", "القنوات")}
                </div>
                <div className="mono" style={{ fontSize: "var(--text-2xl)", fontWeight: "var(--weight-bold)" }}>
                  {(channelsQuery.data?.length ?? 0).toLocaleString()}
                </div>
              </div>
            </div>
            <section
              style={{
                background: "var(--surface)",
                border: "1px solid var(--border)",
                borderRadius: "var(--radius-lg)",
                overflowX: "auto",
              }}
            >
              <div
                style={{
                  padding: "var(--space-4)",
                  borderBottom: "1px solid var(--border)",
                  background: "var(--subtle)",
                }}
              >
                <h3 style={{ margin: 0, fontSize: "var(--text-base)", fontWeight: 600 }}>
                  {text("用户额度汇总", "User quota summary", "ملخص حصص المستخدمين")}
                </h3>
              </div>
              <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "var(--text-sm)" }}>
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--border)", background: "var(--subtle)" }}>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>{text("用户名", "Username", "اسم المستخدم")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>{t("status")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>{text("余额", "Balance", "الرصيد")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>{text("已用", "Used", "المستخدم")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>{text("分组", "Group", "المجموعة")}</th>
                  </tr>
                </thead>
                <tbody>
                  {usersQuery.data?.map((u: AdminUser) => (
                    <tr key={u.id} style={{ borderBottom: "1px solid var(--border)" }}>
                      <td style={{ padding: "10px 14px", fontWeight: 500 }}>{u.username}</td>
                      <td style={{ padding: "10px 14px" }}>
                        <span
                          style={{
                            fontSize: 11,
                            padding: "2px 8px",
                            borderRadius: 4,
                            fontWeight: 600,
                            background: userIsActive(u.status) ? "var(--success-soft)" : "var(--danger-soft)",
                            color: userIsActive(u.status) ? "var(--success)" : "var(--danger)",
                          }}
                        >
                          {userStatusText(u.status)}
                        </span>
                      </td>
                      <td className="mono" style={{ padding: "10px 14px", textAlign: "end", color: "var(--brand)", fontWeight: 600 }}>
                        ${quotaToUsd(u.quota)}
                      </td>
                      <td className="mono" style={{ padding: "10px 14px", textAlign: "end", fontSize: "var(--text-xs)", color: "var(--muted)" }}>
                        ${quotaToUsd(u.used_quota)}
                      </td>
                      <td style={{ padding: "10px 14px" }}>
                        <span style={{ fontSize: 11, padding: "2px 6px", background: "var(--subtle)", borderRadius: 3 }}>
                          {u.group || "default"}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          </div>
        )}

        {/* Tab: Redemption codes */}
        {tab === "redemption" && (
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
                <h1 style={{ fontSize: "var(--text-xl)", fontWeight: "var(--weight-bold)", margin: 0 }}>
                  {text("兑换码管理", "Redemption codes", "أكواد الاسترداد")}
                </h1>
                <p style={{ color: "var(--muted)", fontSize: "var(--text-sm)", margin: "4px 0 0 0" }}>
                  {text("创建、查看和删除预付费兑换码。", "Create, view and delete prepaid redemption codes.", "إنشاء وعرض وحذف أكواد الاسترداد المدفوعة مسبقاً.")}
                </p>
              </div>
              <Button
                variant="primary"
                onClick={() => {
                  setRedeemQuota("10");
                  setRedeemCount("1");
                  setRedeemNotice("");
                  setRedeemModalOpen(true);
                }}
              >
                <Plus size={15} style={{ marginInlineEnd: 4 }} />
                {text("创建兑换码", "Create codes", "إنشاء أكواد")}
              </Button>
            </div>
            {redeemNotice && (
              <div style={{
                padding: "var(--space-3) var(--space-4)",
                background: "var(--success-soft)",
                color: "var(--success)",
                borderRadius: "var(--radius-md)",
                marginBottom: "var(--space-4)",
                fontSize: "var(--text-sm)",
              }}>
                {redeemNotice}
              </div>
            )}
            <section style={{
              background: "var(--surface)",
              border: "1px solid var(--border)",
              borderRadius: "var(--radius-lg)",
              overflowX: "auto",
            }}>
              <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "var(--text-sm)" }}>
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--border)", background: "var(--subtle)" }}>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>{text("码", "Code", "الرمز")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>{text("面值", "Value", "القيمة")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>{t("status")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>{text("使用人", "Used by", "استُخدم من")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>{text("创建时间", "Created", "أُنشئ")}</th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>{text("操作", "Actions", "الإجراءات")}</th>
                  </tr>
                </thead>
                <tbody>
                  {redemptionsQuery.isLoading && (
                    <tr><td colSpan={6} style={{ padding: 24, textAlign: "center", color: "var(--muted)" }}>
                      {text("加载中…", "Loading…", "جارٍ التحميل…")}
                    </td></tr>
                  )}
                  {redemptionsQuery.data?.map((rd: Redemption) => (
                    <tr key={rd.id} style={{ borderBottom: "1px solid var(--border)" }}>
                      <td className="mono" style={{ padding: "10px 14px", fontWeight: 600 }}>{rd.code}</td>
                      <td className="mono" style={{ padding: "10px 14px", color: "var(--brand)" }}>
                        ${(rd.quota / 1e9).toFixed(2)}
                      </td>
                      <td style={{ padding: "10px 14px" }}>
                        <span style={{
                          fontSize: 11, padding: "2px 8px", borderRadius: 4, fontWeight: 600,
                          background: rd.status === 1 ? "var(--success-soft)" : "var(--subtle)",
                          color: rd.status === 1 ? "var(--success)" : "var(--muted)",
                        }}>
                          {rd.status === 1 ? text("未使用", "Unused", "غير مستخدم") : text("已使用", "Used", "مستخدم")}
                        </span>
                      </td>
                      <td className="mono" style={{ padding: "10px 14px", fontSize: 12, color: "var(--muted)" }}>
                        {rd.used_by ? `#${rd.used_by}` : "—"}
                      </td>
                      <td className="mono" style={{ padding: "10px 14px", fontSize: 12, color: "var(--muted)" }}>
                        {new Date(rd.created_at * 1000).toLocaleString()}
                      </td>
                      <td style={{ padding: "10px 14px", textAlign: "end" }}>
                        {rd.status === 1 && (
                          <Button
                            size="small"
                            variant="danger"
                            disabled={deleteRedemptionMut.isPending && deleteRedemptionMut.variables === rd.id}
                            onClick={() => {
                              if (!confirm(text("确认删除此兑换码？", "Delete this code?", "حذف هذا الرمز؟"))) return;
                              deleteRedemptionMut.mutate(rd.id);
                            }}
                          >
                            {text("删除", "Delete", "حذف")}
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                  {redemptionsQuery.data && redemptionsQuery.data.length === 0 && (
                    <tr><td colSpan={6} style={{ padding: 24, textAlign: "center", color: "var(--muted)" }}>
                      {text("暂无兑换码", "No codes", "لا أكواد")}
                    </td></tr>
                  )}
                </tbody>
              </table>
            </section>
          </div>
        )}
      </main>

      {/* Manual Top-up Modal */}
      <Modal
        size="default"
        preventOutsideClose
        open={topUpOpen}
        onOpenChange={setTopUpOpen}
        title={t("manualTopUp")}
        description={text(
          "加款提交即入账，操作人、金额、类型与原因全部写入审计日志。",
          "The credit is applied immediately; operator, amount, type and reason are written to the audit log.",
          "يُحتسب الرصيد فور الإرسال، ويُسجَّل المشغّل والمبلغ والنوع والسبب في سجل التدقيق.",
        )}
        footer={
          <>
            <Button onClick={() => setTopUpOpen(false)}>{t("cancel")}</Button>
            <Button
              variant="primary"
              type="submit"
              form="admin-top-up-form"
              disabled={updateUserMut.isPending}
            >
              {text("提交充值", "Submit top-up", "إرسال الشحن")}
            </Button>
          </>
        }
      >
        <form
          id="admin-top-up-form"
          onSubmit={handleTopUpSubmit}
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-4)",
          }}
        >
          {topUpTarget && (
            <div
              style={{
                padding: "var(--space-3) var(--space-4)",
                background: "var(--subtle)",
                borderRadius: "var(--radius-md)",
                fontSize: "var(--text-sm)",
              }}
            >
              <strong>{topUpTarget.username}</strong>
              <span
                style={{
                  marginInlineStart: 8,
                  color: "var(--muted)",
                  fontSize: "var(--text-xs)",
                }}
              >
                {text("当前余额", "Current balance", "الرصيد الحالي")}: $
                {quotaToUsd(topUpTarget.quota)}
              </span>
            </div>
          )}
          <div>
            <label
              htmlFor="admin-top-up-amount"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {t("topUpAmount")}
            </label>
            <Input
              id="admin-top-up-amount"
              type="number"
              value={topUpAmount}
              onChange={(e) => setTopUpAmount(e.target.value)}
              placeholder={text(
                "例如 500 或 2500",
                "e.g. 500 or 2500",
                "مثل 500 أو 2500",
              )}
              required
            />
          </div>

          <div>
            <label
              htmlFor="admin-top-up-type"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text("交易类型", "Transaction type", "نوع المعاملة")}
            </label>
            <select
              id="admin-top-up-type"
              value={topUpType}
              onChange={(e) => setTopUpType(e.target.value as any)}
              className="select-trigger"
              style={{ width: "100%" }}
            >
              <option value="offline_payment">
                {text(
                  "线下银行付款 / 转账",
                  "Offline bank payment / wire transfer",
                  "دفع مصرفي دون اتصال / تحويل",
                )}
              </option>
              <option value="gift">
                {text(
                  "活动奖励 / 赠送额度",
                  "Incentive / free credit",
                  "مكافأة / رصيد مجاني",
                )}
              </option>
              <option value="compensation">
                {text(
                  "服务异常补偿",
                  "Service incident compensation",
                  "تعويض حادث الخدمة",
                )}
              </option>
              <option value="clawback">
                {text(
                  "拒付追回（负数）",
                  "Chargeback clawback (negative)",
                  "استرداد رد المبلغ (سالب)",
                )}
              </option>
            </select>
          </div>

          <div>
            <label
              htmlFor="admin-top-up-reason"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {t("topUpReason")}
            </label>
            <Input
              id="admin-top-up-reason"
              type="text"
              value={topUpReason}
              onChange={(e) => setTopUpReason(e.target.value)}
              placeholder={text(
                "填写具体原因及合同或凭证编号",
                "Detailed justification and contract or reference number",
                "سبب مفصل ورقم العقد أو المرجع",
              )}
              required
            />
          </div>
        </form>
      </Modal>

      {/* Incident Modal */}
      <Modal
        size="default"
        preventOutsideClose
        open={incidentOpen}
        onOpenChange={setIncidentOpen}
        title={t("publishIncident")}
        description={text(
          "向公开服务状态页发布服务异常公告。",
          "Publish service incident notices to the status page.",
          "انشر إشعارات حوادث الخدمة في صفحة الحالة.",
        )}
        footer={
          <>
            <Button onClick={() => setIncidentOpen(false)}>
              {t("cancel")}
            </Button>
            <Button variant="primary" type="submit" form="admin-incident-form">
              {text(
                "发布事件公告",
                "Publish incident announcement",
                "نشر إشعار الحادث",
              )}
            </Button>
          </>
        }
      >
        <form
          id="admin-incident-form"
          onSubmit={handlePublishIncidentSubmit}
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-4)",
          }}
        >
          <div>
            <label
              htmlFor="admin-incident-title"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text("公告标题", "Notice title", "عنوان الإشعار")}
            </label>
            <Input
              id="admin-incident-title"
              type="text"
              value={incTitle}
              onChange={(e) => setIncTitle(e.target.value)}
              placeholder={text(
                "例如：部分模型响应延迟增加",
                "e.g. Increased response latency for some models",
                "مثل: زيادة زمن استجابة بعض النماذج",
              )}
              required
            />
          </div>

          <div>
            <label
              htmlFor="admin-incident-vendor"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text("受影响供应商", "Impacted vendor", "المزوّد المتأثر")}
            </label>
            <select
              id="admin-incident-vendor"
              value={incVendor}
              onChange={(e) => setIncVendor(e.target.value)}
              className="select-trigger"
              style={{ width: "100%" }}
            >
              <option value="OpenAI">OpenAI</option>
              <option value="Anthropic">Anthropic</option>
              <option value="Google">Google</option>
              <option value="DeepSeek">DeepSeek</option>
              <option value="Qwen">Qwen</option>
            </select>
          </div>

          <div>
            <label
              htmlFor="admin-incident-content"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text(
                "事件说明与处理建议",
                "Description & workaround",
                "الوصف والحل البديل",
              )}
            </label>
            <textarea
              id="admin-incident-content"
              value={incContent}
              onChange={(e) => setIncContent(e.target.value)}
              placeholder={text(
                "说明影响范围、当前状态与处理建议",
                "Describe the impact, current status, and workaround",
                "صف التأثير والحالة الحالية والحل البديل",
              )}
              className="input"
              rows={4}
              required
            />
          </div>
        </form>
      </Modal>

      {/* Channel create/edit modal */}
      <Modal
        size="default"
        preventOutsideClose
        open={channelModalOpen}
        onOpenChange={setChannelModalOpen}
        title={editingChannel
          ? text("编辑渠道", "Edit channel", "تعديل القناة")
          : text("新建渠道", "New channel", "قناة جديدة")}
        description={text(
          "配置渠道名称、密钥、模型和分组。",
          "Configure channel name, key, models and group.",
          "تكوين اسم القناة والمفتاح والنماذج والمجموعة.",
        )}
        footer={
          <>
            <Button onClick={() => setChannelModalOpen(false)}>
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="admin-channel-form"
              disabled={
                createChannelMut.isPending || updateChannelMut.isPending
              }
            >
              {editingChannel
                ? text("保存", "Save", "حفظ")
                : text("创建", "Create", "إنشاء")}
            </Button>
          </>
        }
      >
        <form
          id="admin-channel-form"
          onSubmit={(e) => {
            e.preventDefault();
            if (editingChannel) {
              updateChannelMut.mutate(
                { ...channelForm, id: editingChannel },
                { onSuccess: () => setChannelModalOpen(false) },
              );
            } else {
              createChannelMut.mutate(channelForm, {
                onSuccess: () => setChannelModalOpen(false),
              });
            }
          }}
          style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}
        >
          <div>
            <label htmlFor="ch-name" style={{ display: "block", fontSize: "var(--text-xs)", fontWeight: 600, marginBottom: 4 }}>
              {text("名称", "Name", "الاسم")}
            </label>
            <Input
              id="ch-name"
              value={channelForm.name}
              onChange={(e) => setChannelForm((f) => ({ ...f, name: e.target.value }))}
              required
            />
          </div>
          <div>
            <label htmlFor="ch-key" style={{ display: "block", fontSize: "var(--text-xs)", fontWeight: 600, marginBottom: 4 }}>
              {text("密钥", "API Key", "المفتاح")}
            </label>
            <Input
              id="ch-key"
              type="password"
              value={channelForm.key}
              onChange={(e) => setChannelForm((f) => ({ ...f, key: e.target.value }))}
              placeholder={editingChannel ? text("留空则不修改", "Leave empty to keep", "اترك فارغًا للإبقاء") : ""}
              required={!editingChannel}
            />
          </div>
          <div>
            <label htmlFor="ch-models" style={{ display: "block", fontSize: "var(--text-xs)", fontWeight: 600, marginBottom: 4 }}>
              {text("模型（逗号分隔）", "Models (comma-separated)", "النماذج (مفصولة بفواصل)")}
            </label>
            <Input
              id="ch-models"
              value={channelForm.models}
              onChange={(e) => setChannelForm((f) => ({ ...f, models: e.target.value }))}
              placeholder="gpt-4.1,claude-sonnet-4-20250514"
              required
            />
          </div>
          <div>
            <label htmlFor="ch-group" style={{ display: "block", fontSize: "var(--text-xs)", fontWeight: 600, marginBottom: 4 }}>
              {text("分组", "Group", "المجموعة")}
            </label>
            <Input
              id="ch-group"
              value={channelForm.group}
              onChange={(e) => setChannelForm((f) => ({ ...f, group: e.target.value }))}
            />
          </div>
          <div>
            <label htmlFor="ch-base-url" style={{ display: "block", fontSize: "var(--text-xs)", fontWeight: 600, marginBottom: 4 }}>
              {text("Base URL（可选）", "Base URL (optional)", "عنوان أساسي (اختياري)")}
            </label>
            <Input
              id="ch-base-url"
              value={channelForm.base_url}
              onChange={(e) => setChannelForm((f) => ({ ...f, base_url: e.target.value }))}
              placeholder="https://api.openai.com/v1"
            />
          </div>
        </form>
      </Modal>

      {/* Redemption create modal */}
      <Modal
        size="default"
        preventOutsideClose
        open={redeemModalOpen}
        onOpenChange={setRedeemModalOpen}
        title={text("创建兑换码", "Create redemption codes", "إنشاء أكواد استرداد")}
        description={text(
          "指定面值和数量，系统将生成随机兑换码。",
          "Specify value and count; the system will generate random codes.",
          "حدد القيمة والعدد؛ سيقوم النظام بإنشاء أكواد عشوائية.",
        )}
        footer={
          <>
            <Button onClick={() => setRedeemModalOpen(false)}>{t("cancel")}</Button>
            <Button
              variant="primary"
              type="submit"
              form="admin-redeem-form"
              disabled={createRedemptionMut.isPending}
            >
              {text("创建", "Create", "إنشاء")}
            </Button>
          </>
        }
      >
        <form
          id="admin-redeem-form"
          onSubmit={(e) => {
            e.preventDefault();
            const usd = parseFloat(redeemQuota);
            if (isNaN(usd) || usd <= 0) return;
            const count = parseInt(redeemCount) || 1;
            createRedemptionMut.mutate(
              { quota: Math.round(usd * 1e9), count },
              {
                onSuccess: (result) => {
                  setRedeemNotice(
                    text(
                      `已创建 ${result.count} 个兑换码：${result.codes.join(", ")}`,
                      `Created ${result.count} code(s): ${result.codes.join(", ")}`,
                      `تم إنشاء ${result.count} رمز: ${result.codes.join(", ")}`,
                    ),
                  );
                  setRedeemModalOpen(false);
                },
                onError: (err) => {
                  setRedeemNotice(
                    err instanceof ApiError
                      ? err.message
                      : text("创建失败", "Failed", "فشل"),
                  );
                },
              },
            );
          }}
          style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}
        >
          <div>
            <label htmlFor="redeem-quota" style={{ display: "block", fontSize: "var(--text-xs)", fontWeight: 600, marginBottom: 4 }}>
              {text("面值 (USD)", "Value (USD)", "القيمة (دولار)")}
            </label>
            <Input
              id="redeem-quota"
              type="number"
              value={redeemQuota}
              onChange={(e) => setRedeemQuota(e.target.value)}
              placeholder="10"
              required
            />
          </div>
          <div>
            <label htmlFor="redeem-count" style={{ display: "block", fontSize: "var(--text-xs)", fontWeight: 600, marginBottom: 4 }}>
              {text("数量", "Count", "العدد")}
            </label>
            <Input
              id="redeem-count"
              type="number"
              value={redeemCount}
              onChange={(e) => setRedeemCount(e.target.value)}
              placeholder="1"
              min="1"
              max="100"
              required
            />
          </div>
        </form>
      </Modal>

      {/* Account detail modal */}
      <Modal
        size="default"
        open={!!detailUser}
        onOpenChange={(open) => { if (!open) setDetailUser(null); }}
        title={text("账户详情", "Account details", "تفاصيل الحساب")}
        description={detailUser ? detailUser.username : ""}
        footer={
          <>
            <Button onClick={() => {
              if (detailUser) {
                setTopUpTarget(detailUser);
                setTopUpAmount("");
                setTopUpReason("");
                setTopUpOpen(true);
                setDetailUser(null);
              }
            }}>
              {t("manualTopUp")}
            </Button>
            <Button variant={detailUser && userIsActive(detailUser.status) ? "danger" : "primary"}
              onClick={() => {
                if (detailUser) {
                  handleToggleStatus(detailUser);
                  setDetailUser(null);
                }
              }}
            >
              {detailUser && userIsActive(detailUser.status) ? t("disableAccount") : t("enableAccount")}
            </Button>
            <Button variant="secondary" onClick={() => setDetailUser(null)}>
              {text("关闭", "Close", "إغلاق")}
            </Button>
          </>
        }
      >
        {detailUser && (
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "var(--space-3)" }}>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("用户名", "Username", "اسم المستخدم")}</small>
                <div style={{ fontWeight: 600 }}>{detailUser.username}</div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("邮箱", "Email", "البريد")}</small>
                <div>{detailUser.email || "—"}</div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("角色", "Role", "الدور")}</small>
                <div>
                  <span style={{ fontSize: 11, padding: "2px 8px", borderRadius: 4, fontWeight: 600, background: "var(--subtle)" }}>
                    {detailUser.role >= 100 ? "Root" : detailUser.role >= 10 ? "Admin" : "User"}
                  </span>
                </div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{t("status")}</small>
                <div>
                  <span style={{ fontSize: 11, padding: "2px 8px", borderRadius: 4, fontWeight: 600, background: userIsActive(detailUser.status) ? "var(--success-soft)" : "var(--danger-soft)", color: userIsActive(detailUser.status) ? "var(--success)" : "var(--danger)" }}>
                    {userStatusText(detailUser.status)}
                  </span>
                </div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("余额", "Balance", "الرصيد")}</small>
                <div className="mono" style={{ fontWeight: 600, color: "var(--brand)" }}>${quotaToUsd(detailUser.quota)}</div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("已用", "Used", "المستخدم")}</small>
                <div className="mono" style={{ color: "var(--muted)" }}>${quotaToUsd(detailUser.used_quota)}</div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("分组", "Group", "المجموعة")}</small>
                <div>{detailUser.group || "default"}</div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("邀请码", "Affiliate code", "رمز الإحالة")}</small>
                <div className="mono">{detailUser.aff_code || "—"}</div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("邀请人", "Inviter", "الداعي")}</small>
                <div>{detailUser.inviter_id ? `#${detailUser.inviter_id}` : "—"}</div>
              </div>
              <div>
                <small style={{ color: "var(--muted)" }}>{text("注册时间", "Registered", "مسجل")}</small>
                <div className="mono" style={{ fontSize: "var(--text-xs)" }}>{new Date(detailUser.created_at * 1000).toLocaleString()}</div>
              </div>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}
