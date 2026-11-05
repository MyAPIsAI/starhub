import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { StaffWorkspace } from "../../components/staff-workspace";
import {
  staffNavigationFor,
  defaultStaffTab,
  type StaffTab,
} from "../../features/staff-navigation";
import { ServiceGroups } from "../../features/service-groups";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/primitives";
import { Modal } from "../../components/ui/overlay";
import { useSystemAccess, isAdmin, ROLE_ROOT } from "../../lib/system-access";
import { FinanceOverview } from "../../features/finance-overview";
import { OperatingDashboard } from "../../features/operating-dashboard";
import { AdminCustomers } from "../../features/admin-customers";
import { AdminModels } from "../../features/admin-models";
import { AdminRegionProfile } from "../../features/admin-profile";
import {
  AdminAudit,
  AdminReconciliation,
  AdminSystem,
} from "../../features/admin-operations";
import { ConfigurationSync } from "../../features/configuration-sync";
import { ChannelRoutingNotice } from "../../features/channel-routing-notice";
import {
  StaffAccounts,
  StaffSync,
  StaffCalls,
} from "../../features/staff-management";
import {
  quotaToUsd,
  useAdminMetrics,
  useAdminChannels,
  useTestChannel,
  useUpstreamUsage,
  type ChannelTestResult,
  type UpstreamUsageResult,
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
import { TaskPlugins } from "../../features/task-plugins";
import {
  useTaskPlugins,
  TASK_PLUGIN_CHANNEL_TYPE,
} from "../../lib/api/task-plugin";
import { ApiError } from "../../lib/api/http";

export default function AdminPortal() {
  const { t, i18n } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const lang = i18n.language.startsWith("zh")
    ? "zh"
    : i18n.language.startsWith("ar")
      ? "ar"
      : "en";
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;

  const authRole = useSystemAccess((s) => s.role);
  const isRoot = authRole === ROLE_ROOT;
  const validTabs = staffNavigationFor(authRole).map((item) => item.id);
  const requestedTab = searchParams.get("tab") as StaffTab | null;
  const tab: StaffTab =
    requestedTab && validTabs.includes(requestedTab)
      ? requestedTab
      : defaultStaffTab(authRole);
  useEffect(() => {
    const requested = searchParams.get("tab");
    if (requested && !validTabs.includes(requested as StaffTab)) {
      const next = new URLSearchParams(searchParams);
      next.delete("tab");
      setSearchParams(next, { replace: true });
    }
  }, [searchParams, authRole]);
  // Backend API hooks.
  const channelsQuery = useAdminChannels(tab === "resources");
  const metricsQuery = useAdminMetrics(tab === "resources");
  const pluginsQuery = useTaskPlugins(
    isRoot && (tab === "plugins" || tab === "resources"),
  );
  const testChannelMut = useTestChannel();
  const createChannelMut = useCreateChannel();
  const updateChannelMut = useUpdateChannel();
  const deleteChannelMut = useDeleteChannel();
  const redemptionsQuery = useRedemptions(isRoot && tab === "redemption");
  const createRedemptionMut = useCreateRedemption();
  const deleteRedemptionMut = useDeleteRedemption();

  // Channel test result per channel id (cleared on next test).
  const [testResults, setTestResults] = useState<
    Record<number, ChannelTestResult | string>
  >({});

  // Upstream usage reconciliation modal state.
  const upstreamMut = useUpstreamUsage();
  const [upstreamModal, setUpstreamModal] =
    useState<UpstreamUsageResult | null>(null);
  const [upstreamLoading, setUpstreamLoading] = useState(false);
  const [upstreamError, setUpstreamError] = useState<string | null>(null);

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

  // Incident modal state
  const [incidentOpen, setIncidentOpen] = useState(false);
  const [incTitle, setIncTitle] = useState("");
  const [incVendor, setIncVendor] = useState("OpenAI");
  const [incContent, setIncContent] = useState("");

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
    <StaffWorkspace active={tab}>
      <div className="admin-content">
        {tab === "plugins" && <TaskPlugins />}
        {tab === "customers" && <AdminCustomers />}
        {tab === "staff" && isRoot && <StaffAccounts />}
        {tab === "sync" && (
          <>
            <StaffSync />
            {isRoot && <ConfigurationSync />}
          </>
        )}
        {tab === "regions" && isRoot && <AdminRegionProfile />}
        {tab === "groups" && <ServiceGroups role={authRole} />}
        {tab === "calls" && <StaffCalls />}
        {tab === "models" && <AdminModels />}
        {tab === "system" && <AdminSystem />}

        {tab === "analytics" && <OperatingDashboard />}

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
                    "查看上游渠道及共享配置数据库的加载和心跳状态。",
                    "View upstream channels and shared-configuration load and heartbeat status.",
                    "عرض القنوات وحالة تحميل الإعدادات المشتركة والنبضات.",
                  )}
                </p>
              </div>
            </div>

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
                  <div
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                    }}
                  >
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
                      {text(
                        "平均 TTFT (ms)",
                        "Avg TTFT (ms)",
                        "متوسط TTFT (ملث)",
                      )}
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
                    const stat = metricsQuery.data?.channel_stats?.find(
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
                        <td className="mono" style={{ padding: "10px 14px" }}>
                          {stat
                            ? (stat.success_rate * 100).toFixed(1) + "%"
                            : "—"}
                        </td>
                        <td className="mono" style={{ padding: "10px 14px" }}>
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
                                    color: (
                                      testResults[ch.id] as ChannelTestResult
                                    ).ok
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
                              disabled={
                                upstreamLoading &&
                                upstreamModal?.channel_id === ch.id
                              }
                              onClick={() => {
                                setUpstreamModal(null);
                                setUpstreamError(null);
                                setUpstreamLoading(true);
                                upstreamMut.mutate(ch.id, {
                                  onSuccess: (res) => {
                                    setUpstreamModal(res);
                                    setUpstreamLoading(false);
                                  },
                                  onError: (err) => {
                                    setUpstreamError(
                                      err instanceof ApiError
                                        ? err.message
                                        : text(
                                            "查询失败",
                                            "Query failed",
                                            "فشل الاستعلام",
                                          ),
                                    );
                                    setUpstreamLoading(false);
                                  },
                                });
                              }}
                            >
                              {upstreamLoading &&
                              upstreamMut.variables === ch.id
                                ? text("查询中…", "Querying…", "استعلام…")
                                : text("上游对账", "Reconcile", "مطابقة")}
                            </Button>
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
                                  task_plugin_id: ch.task_plugin_id ?? 0,
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
                                if (
                                  !confirm(
                                    text(
                                      "确认删除此渠道？",
                                      "Delete this channel?",
                                      "حذف هذه القناة؟",
                                    ),
                                  )
                                )
                                  return;
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

            {/* Upstream usage reconciliation modal */}
            {upstreamError && (
              <div
                style={{
                  position: "fixed",
                  inset: 0,
                  background: "rgba(0,0,0,0.4)",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  zIndex: 1000,
                }}
                onClick={() => setUpstreamError(null)}
              >
                <div
                  onClick={(e) => e.stopPropagation()}
                  style={{
                    background: "var(--bg)",
                    borderRadius: 12,
                    padding: 24,
                    maxWidth: 480,
                    width: "90%",
                    boxShadow: "0 8px 32px rgba(0,0,0,0.2)",
                  }}
                >
                  <p style={{ color: "var(--danger)", margin: "0 0 16px 0" }}>
                    {upstreamError}
                  </p>
                  <Button
                    size="small"
                    variant="secondary"
                    onClick={() => setUpstreamError(null)}
                  >
                    {text("关闭", "Close", "إغلاق")}
                  </Button>
                </div>
              </div>
            )}
            {upstreamModal && (
              <div
                style={{
                  position: "fixed",
                  inset: 0,
                  background: "rgba(0,0,0,0.4)",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  zIndex: 1000,
                }}
                onClick={() => setUpstreamModal(null)}
              >
                <div
                  onClick={(e) => e.stopPropagation()}
                  style={{
                    background: "var(--bg)",
                    borderRadius: 12,
                    padding: 24,
                    maxWidth: 680,
                    width: "90%",
                    maxHeight: "80vh",
                    overflow: "auto",
                    boxShadow: "0 8px 32px rgba(0,0,0,0.2)",
                  }}
                >
                  <div
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      marginBottom: 16,
                    }}
                  >
                    <h3 style={{ margin: 0, fontSize: "var(--text-lg)" }}>
                      {text(
                        "上游对账",
                        "Upstream Reconciliation",
                        "مطابقة المنبع",
                      )}
                    </h3>
                    <Button
                      size="small"
                      variant="secondary"
                      onClick={() => setUpstreamModal(null)}
                    >
                      {text("关闭", "Close", "إغلاق")}
                    </Button>
                  </div>
                  <p
                    style={{
                      color: "var(--muted)",
                      fontSize: "var(--text-sm)",
                      margin: "0 0 16px 0",
                    }}
                  >
                    {upstreamModal.channel_name} (ID: {upstreamModal.channel_id}
                    )
                  </p>
                  {!upstreamModal.supported ? (
                    <p style={{ color: "var(--muted)" }}>
                      {text(
                        "上游不支持用量查询（可能不是 new-api 实例）",
                        "Upstream does not support usage queries (may not be a new-api instance)",
                        "المنبع لا يدعم استعلامات الاستخدام",
                      )}
                    </p>
                  ) : (
                    <>
                      <div
                        style={{
                          display: "flex",
                          gap: 24,
                          marginBottom: 16,
                          flexWrap: "wrap",
                        }}
                      >
                        <div>
                          <div style={{ fontSize: 11, color: "var(--muted)" }}>
                            {text("本地累计", "Local Used", "المحلي")}
                          </div>
                          <div
                            className="mono"
                            style={{ fontSize: "var(--text-lg)" }}
                          >
                            {quotaToUsd(upstreamModal.local_used_quota)}
                          </div>
                        </div>
                        <div>
                          <div style={{ fontSize: 11, color: "var(--muted)" }}>
                            {text("上游累计", "Upstream Used", "المنبع")}
                          </div>
                          <div
                            className="mono"
                            style={{ fontSize: "var(--text-lg)" }}
                          >
                            {quotaToUsd(upstreamModal.upstream_total_used)}
                          </div>
                        </div>
                        <div>
                          <div style={{ fontSize: 11, color: "var(--muted)" }}>
                            {text("差异", "Diff", "الفرق")}
                          </div>
                          <div
                            className="mono"
                            style={{
                              fontSize: "var(--text-lg)",
                              color:
                                Math.abs(upstreamModal.diff) > 100000
                                  ? "var(--warning, #e0a030)"
                                  : "var(--success)",
                            }}
                          >
                            {upstreamModal.diff >= 0 ? "+" : ""}
                            {quotaToUsd(upstreamModal.diff)}
                          </div>
                        </div>
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
                            style={{ borderBottom: "1px solid var(--border)" }}
                          >
                            <th
                              style={{
                                textAlign: "start",
                                padding: "8px 12px",
                              }}
                            >
                              Key
                            </th>
                            <th
                              style={{ textAlign: "end", padding: "8px 12px" }}
                            >
                              {text("已用", "Used", "مستخدم")}
                            </th>
                            <th
                              style={{ textAlign: "end", padding: "8px 12px" }}
                            >
                              {text("剩余", "Available", "متاح")}
                            </th>
                            <th
                              style={{ textAlign: "end", padding: "8px 12px" }}
                            >
                              {text("过期", "Expires", "تنتهي")}
                            </th>
                          </tr>
                        </thead>
                        <tbody>
                          {upstreamModal.keys.map((k) => (
                            <tr
                              key={k.key_index}
                              style={{
                                borderBottom: "1px solid var(--border)",
                              }}
                            >
                              <td
                                className="mono"
                                style={{
                                  padding: "8px 12px",
                                  fontSize: "var(--text-xs)",
                                }}
                              >
                                {k.key_masked}
                                {!k.available && (
                                  <span
                                    style={{
                                      marginLeft: 8,
                                      fontSize: 11,
                                      color: "var(--danger)",
                                    }}
                                  >
                                    {k.error ||
                                      text("不可用", "N/A", "غير متاح")}
                                  </span>
                                )}
                              </td>
                              <td
                                className="mono"
                                style={{
                                  padding: "8px 12px",
                                  textAlign: "end",
                                }}
                              >
                                {k.available ? quotaToUsd(k.total_used) : "—"}
                              </td>
                              <td
                                className="mono"
                                style={{
                                  padding: "8px 12px",
                                  textAlign: "end",
                                  color:
                                    k.available &&
                                    !k.unlimited &&
                                    k.total_available < 100000
                                      ? "var(--danger)"
                                      : "inherit",
                                }}
                              >
                                {k.available
                                  ? k.unlimited
                                    ? text("无限", "Unlimited", "غير محدود")
                                    : quotaToUsd(k.total_available)
                                  : "—"}
                              </td>
                              <td
                                className="mono"
                                style={{
                                  padding: "8px 12px",
                                  textAlign: "end",
                                }}
                              >
                                {k.available && k.expires_at > 0
                                  ? new Date(
                                      k.expires_at * 1000,
                                    ).toLocaleDateString()
                                  : "—"}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                      <p
                        style={{
                          fontSize: 11,
                          color: "var(--muted)",
                          marginTop: 16,
                        }}
                      >
                        {text(
                          "差异可能来自倍率不同、精度差异或退款同步延迟，不一定代表计费错误。",
                          "The diff may stem from different multipliers, rounding, or refund sync delays, and does not necessarily indicate a billing error.",
                          "قد ينبع الفرق من معدلات مختلفة أو تأخر المزامنة ولا يعني بالضرورة خطأ في الفوترة.",
                        )}
                      </p>
                    </>
                  )}
                </div>
              </div>
            )}
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
                    "当前为页面内演示；发布不会更新公开服务状态页。",
                    "Local page demonstration; publishing does not update the public status page.",
                    "عرض محلي داخل الصفحة؛ لا يحدّث النشر صفحة حالة الخدمة العامة.",
                  )}
                </p>
              </div>

              {isAdmin(authRole) && (
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

        {tab === "audit" && <AdminAudit />}

        {tab === "finance" && (
          <>
            <AdminReconciliation />
            <details className="adm-card">
              <summary>
                {text(
                  "账务汇总与订单",
                  "Financial summary and orders",
                  "الملخص المالي والطلبات",
                )}
              </summary>
              <FinanceOverview />
            </details>
          </>
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
                <h1
                  style={{
                    fontSize: "var(--text-xl)",
                    fontWeight: "var(--weight-bold)",
                    margin: 0,
                  }}
                >
                  {text("兑换码管理", "Redemption codes", "أكواد الاسترداد")}
                </h1>
                <p
                  style={{
                    color: "var(--muted)",
                    fontSize: "var(--text-sm)",
                    margin: "4px 0 0 0",
                  }}
                >
                  {text(
                    "创建、查看和删除预付费兑换码。",
                    "Create, view and delete prepaid redemption codes.",
                    "إنشاء وعرض وحذف أكواد الاسترداد المدفوعة مسبقاً.",
                  )}
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
                {redeemNotice}
              </div>
            )}
            <section
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
                      borderBottom: "1px solid var(--border)",
                      background: "var(--subtle)",
                    }}
                  >
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("码", "Code", "الرمز")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("面值", "Value", "القيمة")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {t("status")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("使用人", "Used by", "استُخدم من")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("创建时间", "Created", "أُنشئ")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>
                      {text("操作", "Actions", "الإجراءات")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {redemptionsQuery.isLoading && (
                    <tr>
                      <td
                        colSpan={6}
                        style={{
                          padding: 24,
                          textAlign: "center",
                          color: "var(--muted)",
                        }}
                      >
                        {text("加载中…", "Loading…", "جارٍ التحميل…")}
                      </td>
                    </tr>
                  )}
                  {redemptionsQuery.data?.map((rd: Redemption) => (
                    <tr
                      key={rd.id}
                      style={{ borderBottom: "1px solid var(--border)" }}
                    >
                      <td
                        className="mono"
                        style={{ padding: "10px 14px", fontWeight: 600 }}
                      >
                        {rd.code}
                      </td>
                      <td
                        className="mono"
                        style={{ padding: "10px 14px", color: "var(--brand)" }}
                      >
                        ${(rd.quota / 1e9).toFixed(2)}
                      </td>
                      <td style={{ padding: "10px 14px" }}>
                        <span
                          style={{
                            fontSize: 11,
                            padding: "2px 8px",
                            borderRadius: 4,
                            fontWeight: 600,
                            background:
                              rd.status === 1
                                ? "var(--success-soft)"
                                : "var(--subtle)",
                            color:
                              rd.status === 1
                                ? "var(--success)"
                                : "var(--muted)",
                          }}
                        >
                          {rd.status === 1
                            ? text("未使用", "Unused", "غير مستخدم")
                            : text("已使用", "Used", "مستخدم")}
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
                        {rd.used_by ? `#${rd.used_by}` : "—"}
                      </td>
                      <td
                        className="mono"
                        style={{
                          padding: "10px 14px",
                          fontSize: 12,
                          color: "var(--muted)",
                        }}
                      >
                        {new Date(rd.created_at * 1000).toLocaleString()}
                      </td>
                      <td style={{ padding: "10px 14px", textAlign: "end" }}>
                        {rd.status === 1 && (
                          <Button
                            size="small"
                            variant="danger"
                            disabled={
                              deleteRedemptionMut.isPending &&
                              deleteRedemptionMut.variables === rd.id
                            }
                            onClick={() => {
                              if (
                                !confirm(
                                  text(
                                    "确认删除此兑换码？",
                                    "Delete this code?",
                                    "حذف هذا الرمز؟",
                                  ),
                                )
                              )
                                return;
                              deleteRedemptionMut.mutate(rd.id);
                            }}
                          >
                            {text("删除", "Delete", "حذف")}
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                  {redemptionsQuery.data &&
                    redemptionsQuery.data.length === 0 && (
                      <tr>
                        <td
                          colSpan={6}
                          style={{
                            padding: 24,
                            textAlign: "center",
                            color: "var(--muted)",
                          }}
                        >
                          {text("暂无兑换码", "No codes", "لا أكواد")}
                        </td>
                      </tr>
                    )}
                </tbody>
              </table>
            </section>
          </div>
        )}
      </div>

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
        title={
          editingChannel
            ? text("编辑渠道", "Edit channel", "تعديل القناة")
            : text("新建渠道", "New channel", "قناة جديدة")
        }
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
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-4)",
          }}
        >
          <ChannelRoutingNotice
            onOpenBindings={() => setChannelModalOpen(false)}
          />
          <div className="task-plugin-channel-field">
            <label htmlFor="ch-type">
              {text("渠道类型", "Channel type", "نوع القناة")}
            </label>
            <select
              id="ch-type"
              value={channelForm.type ?? 1}
              onChange={(e) =>
                setChannelForm((f) => ({
                  ...f,
                  type: Number(e.target.value),
                  task_plugin_id: 0,
                }))
              }
            >
              <option value={1}>OpenAI</option>
              <option value={14}>Claude</option>
              <option value={15}>AWS Bedrock</option>
              <option value={25}>Gemini</option>
              <option value={TASK_PLUGIN_CHANNEL_TYPE}>
                {text(
                  "视频任务插件",
                  "Video task plugin",
                  "إضافة مهام الفيديو",
                )}
              </option>
            </select>
          </div>
          {channelForm.type === TASK_PLUGIN_CHANNEL_TYPE && (
            <div className="task-plugin-channel-field">
              <label htmlFor="ch-plugin">
                {text("插件版本", "Plugin version", "إصدار الإضافة")}
              </label>
              <select
                id="ch-plugin"
                required
                value={channelForm.task_plugin_id || ""}
                onChange={(e) =>
                  setChannelForm((f) => ({
                    ...f,
                    task_plugin_id: Number(e.target.value),
                  }))
                }
              >
                <option value="">
                  {text("选择版本", "Select a version", "اختر إصداراً")}
                </option>
                {pluginsQuery.data?.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.key} {p.version}
                    {p.enabled
                      ? ""
                      : text("（已禁用）", " (disabled)", " (معطل)")}
                  </option>
                ))}
              </select>
              {pluginsQuery.isError && (
                <span role="alert">{pluginsQuery.error.message}</span>
              )}
              <span>
                {text(
                  "使用单个上游密钥；有未完成任务时不可更换密钥或地址。",
                  "Use one upstream key. Credentials and base URL are locked while tasks are pending.",
                  "استخدم مفتاحاً واحداً. لا يمكن تغيير المفتاح أو العنوان أثناء وجود مهام معلقة.",
                )}
              </span>
            </div>
          )}

          <div>
            <label
              htmlFor="ch-name"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text("名称", "Name", "الاسم")}
            </label>
            <Input
              id="ch-name"
              value={channelForm.name}
              onChange={(e) =>
                setChannelForm((f) => ({ ...f, name: e.target.value }))
              }
              required
            />
          </div>
          <div>
            <label
              htmlFor="ch-key"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text("密钥", "API Key", "المفتاح")}
            </label>
            <Input
              id="ch-key"
              type="password"
              value={channelForm.key}
              onChange={(e) =>
                setChannelForm((f) => ({ ...f, key: e.target.value }))
              }
              placeholder={
                editingChannel
                  ? text(
                      "留空则不修改",
                      "Leave empty to keep",
                      "اترك فارغًا للإبقاء",
                    )
                  : ""
              }
              required={!editingChannel}
            />
          </div>
          <div>
            <label
              htmlFor="ch-models"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text(
                "模型（逗号分隔）",
                "Models (comma-separated)",
                "النماذج (مفصولة بفواصل)",
              )}
            </label>
            <Input
              id="ch-models"
              value={channelForm.models}
              onChange={(e) =>
                setChannelForm((f) => ({ ...f, models: e.target.value }))
              }
              placeholder="gpt-4.1,claude-sonnet-4-20250514"
              required
            />
          </div>
          <div>
            <label
              htmlFor="ch-group"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text("分组", "Group", "المجموعة")}
            </label>
            <Input
              id="ch-group"
              value={channelForm.group}
              onChange={(e) =>
                setChannelForm((f) => ({ ...f, group: e.target.value }))
              }
            />
          </div>
          <div>
            <label
              htmlFor="ch-base-url"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
              {text(
                "Base URL（可选）",
                "Base URL (optional)",
                "عنوان أساسي (اختياري)",
              )}
            </label>
            <Input
              id="ch-base-url"
              value={channelForm.base_url}
              onChange={(e) =>
                setChannelForm((f) => ({ ...f, base_url: e.target.value }))
              }
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
        title={text(
          "创建兑换码",
          "Create redemption codes",
          "إنشاء أكواد استرداد",
        )}
        description={text(
          "指定面值和数量，系统将生成随机兑换码。",
          "Specify value and count; the system will generate random codes.",
          "حدد القيمة والعدد؛ سيقوم النظام بإنشاء أكواد عشوائية.",
        )}
        footer={
          <>
            <Button onClick={() => setRedeemModalOpen(false)}>
              {t("cancel")}
            </Button>
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
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-4)",
          }}
        >
          <div>
            <label
              htmlFor="redeem-quota"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
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
            <label
              htmlFor="redeem-count"
              style={{
                display: "block",
                fontSize: "var(--text-xs)",
                fontWeight: 600,
                marginBottom: 4,
              }}
            >
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
    </StaffWorkspace>
  );
}
