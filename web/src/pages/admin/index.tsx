import { message as translatedMessage } from "../../i18n/messages";
import { AdminAnnouncements } from "../../features/admin-announcements";
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
import { useSystemAccess, ROLE_ROOT } from "../../lib/system-access";
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
  const testChannelMut = useTestChannel();
  const createChannelMut = useCreateChannel();
  const updateChannelMut = useUpdateChannel();
  const deleteChannelMut = useDeleteChannel();
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

  return (
    <StaffWorkspace active={tab}>
      <div className="admin-content">
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
                    ...translatedMessage(
                      "view_upstream_channels_and_shared_configurati_ebc80df852",
                    ),
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
                      {text(...translatedMessage("channels_dc6282e0ba"))} (
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
                      {text(...translatedMessage("new_channel_70800bf37c"))}
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
                      {text(...translatedMessage("name_e639662a6f"))}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text(...translatedMessage("models_098275fbd2"))}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {t("status")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text(...translatedMessage("success_rate_b8e8689749"))}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text(...translatedMessage("avg_ttft_ms_d960312604"))}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>
                      {text(...translatedMessage("test_2f53109312"))}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>
                      {text(...translatedMessage("manage_e3761d66d9"))}
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
                        {text(...translatedMessage("loading_4c20385375"))}
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
                              ? text(...translatedMessage("enabled_bc0e24c0da"))
                              : text(
                                  ...translatedMessage("disabled_d13f2c5fc4"),
                                )}
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
                                                ...translatedMessage(
                                                  "test_failed_f4a1bca81d",
                                                ),
                                              ),
                                      })),
                                  },
                                );
                              }}
                            >
                              {testChannelMut.isPending &&
                              testChannelMut.variables?.id === ch.id
                                ? text(
                                    ...translatedMessage("testing_b1cdbb3652"),
                                  )
                                : text(...translatedMessage("test_43ee6472a6"))}
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
                                    ? `${text(...translatedMessage("ok_be3a283cbb"))} · ${(testResults[ch.id] as ChannelTestResult).elapsed_ms}ms`
                                    : text(
                                        ...translatedMessage(
                                          "failed_5da76a883a",
                                        ),
                                      )}
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
                                            ...translatedMessage(
                                              "query_failed_952b68b314",
                                            ),
                                          ),
                                    );
                                    setUpstreamLoading(false);
                                  },
                                });
                              }}
                            >
                              {upstreamLoading &&
                              upstreamMut.variables === ch.id
                                ? text(
                                    ...translatedMessage("querying_92c9a85457"),
                                  )
                                : text(
                                    ...translatedMessage(
                                      "reconcile_04a7e2f385",
                                    ),
                                  )}
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
                                  status: ch.status,
                                  priority: ch.priority,
                                  weight: ch.weight,
                                });
                                setChannelModalOpen(true);
                              }}
                            >
                              {text(...translatedMessage("edit_6224671192"))}
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
                                      ...translatedMessage(
                                        "delete_this_channel_6d60a34ae2",
                                      ),
                                    ),
                                  )
                                )
                                  return;
                                deleteChannelMut.mutate(ch.id);
                              }}
                            >
                              {text(...translatedMessage("delete_0a459cc2a8"))}
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
                    {text(...translatedMessage("close_6bf477426b"))}
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
                        ...translatedMessage(
                          "upstream_reconciliation_f0316575cd",
                        ),
                      )}
                    </h3>
                    <Button
                      size="small"
                      variant="secondary"
                      onClick={() => setUpstreamModal(null)}
                    >
                      {text(...translatedMessage("close_6bf477426b"))}
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
                        ...translatedMessage(
                          "upstream_does_not_support_usage_queries_may_n_e52818571b",
                        ),
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
                            {text(
                              ...translatedMessage("local_used_dcad0c7a11"),
                            )}
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
                            {text(
                              ...translatedMessage("upstream_used_78c322d155"),
                            )}
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
                            {text(...translatedMessage("diff_2bd3633c01"))}
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
                              {text(...translatedMessage("used_9178f3966b"))}
                            </th>
                            <th
                              style={{ textAlign: "end", padding: "8px 12px" }}
                            >
                              {text(
                                ...translatedMessage("available_00e89c4586"),
                              )}
                            </th>
                            <th
                              style={{ textAlign: "end", padding: "8px 12px" }}
                            >
                              {text(...translatedMessage("expires_af2035b496"))}
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
                                      text(
                                        ...translatedMessage("n_a_6fcb32bb1f"),
                                      )}
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
                                    ? text(
                                        ...translatedMessage(
                                          "unlimited_868a96a574",
                                        ),
                                      )
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
                          ...translatedMessage(
                            "the_diff_may_stem_from_different_multipliers__2a4461be4f",
                          ),
                        )}
                      </p>
                    </>
                  )}
                </div>
              </div>
            )}
          </div>
        )}

        {tab === "incidents" && <AdminAnnouncements />}

        {tab === "audit" && <AdminAudit />}

        {tab === "finance" && (
          <>
            <AdminReconciliation />
            <details className="adm-card">
              <summary>
                {text(
                  ...translatedMessage(
                    "financial_summary_and_orders_02c9410d90",
                  ),
                )}
              </summary>
              <FinanceOverview />
            </details>
          </>
        )}
      </div>

      {/* Channel create/edit modal */}
      <Modal
        size="default"
        preventOutsideClose
        open={channelModalOpen}
        onOpenChange={setChannelModalOpen}
        title={
          editingChannel
            ? text(...translatedMessage("edit_channel_33053e2138"))
            : text(...translatedMessage("new_channel_70800bf37c"))
        }
        description={text(
          ...translatedMessage(
            "configure_channel_name_key_models_and_group_8a86fc902c",
          ),
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
                ? text(...translatedMessage("save_6774e725d1"))
                : text(...translatedMessage("create_6d0538dedd"))}
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
          <div className="admin-channel-type-field">
            <label htmlFor="ch-type">
              {text(...translatedMessage("channel_type_11743331db"))}
            </label>
            <select
              id="ch-type"
              value={channelForm.type ?? 1}
              onChange={(e) =>
                setChannelForm((f) => ({
                  ...f,
                  type: Number(e.target.value),
                }))
              }
            >
              <option value={1}>OpenAI</option>
              <option value={14}>Claude</option>
              <option value={15}>AWS Bedrock</option>
              <option value={25}>Gemini</option>
            </select>
          </div>
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
              {text(...translatedMessage("name_e639662a6f"))}
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
              {text(...translatedMessage("api_key_74afeeaf64"))}
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
                  ? text(...translatedMessage("leave_empty_to_keep_521458eb6b"))
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
              {text(...translatedMessage("models_comma_separated_368c1f6199"))}
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
              {text(...translatedMessage("group_528d77dde5"))}
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
              {text(...translatedMessage("base_url_optional_976f4a52dc"))}
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
    </StaffWorkspace>
  );
}
