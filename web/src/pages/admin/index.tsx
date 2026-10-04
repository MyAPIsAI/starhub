import { accountDisplayName } from "../../lib/account-identity";
import { useEffect, useState } from "react";
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
} from "lucide-react";
import {
  ProductBrand,
  Preferences,
  PreviewNote,
} from "../../components/product-shell";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/primitives";
import { Modal } from "../../components/ui/overlay";
import { useProductPreview } from "../../lib/product-preview";
import { formatMoney } from "../../lib/money";
import { ModelPricingAdmin } from "../../features/model-pricing-admin";

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
  const [tab, setTab] = useState<
    "customers" | "models" | "analytics" | "resources" | "incidents" | "audit"
  >(() =>
    new URLSearchParams(location.search).get("tab") === "models"
      ? "models"
      : "customers",
  );
  useEffect(() => {
    if (new URLSearchParams(location.search).get("tab") === "models")
      setTab("models");
  }, [location.search]);

  const user = useProductPreview((s) => s.user);
  const balance = useProductPreview((s) => s.balance);
  const adminReviews = useProductPreview((s) => s.adminReviews);
  const adminCreateTopUp = useProductPreview((s) => s.adminCreateTopUp);
  const adminUpdateConcurrency = useProductPreview(
    (s) => s.adminUpdateConcurrency,
  );
  const adminToggleAccountStatus = useProductPreview(
    (s) => s.adminToggleAccountStatus,
  );
  const incidents = useProductPreview((s) => s.incidents);
  const adminPublishIncident = useProductPreview((s) => s.adminPublishIncident);

  // Top-up modal state
  const [topUpOpen, setTopUpOpen] = useState(false);
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
    const amt = parseFloat(topUpAmount);
    if (isNaN(amt) || amt === 0) return;

    const operator =
      role === "finance" ? "财务管理员（当前账号）" : "超级管理员（当前账号）";
    adminCreateTopUp({
      amount_usd: amt,
      type: topUpType,
      reason: topUpReason,
      operator,
    });

    // #105：单人操作，提交即入账，不再有金额阈值与待复核分支。
    setTopUpNotice(
      `已为 ${user.enterpriseName} 入账 $${amt.toFixed(2)}（操作人：${operator}）。`,
    );

    setTopUpOpen(false);
  };

  const handleForceSync = () => {
    setSyncing(true);
    setTimeout(() => {
      setSyncing(false);
      setSyncedMessage(t("syncConfirmed"));
      setTimeout(() => setSyncedMessage(""), 4000);
    }, 800);
  };

  const handlePublishIncidentSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!incTitle || !incContent) return;
    adminPublishIncident({
      id: `inc_${Date.now()}`,
      title: { en: incTitle, zh: incTitle, ar: incTitle },
      impactedVendor: incVendor,
      status: "investigating",
      updatedAt: "Just now",
      content: { en: incContent, zh: incContent, ar: incContent },
    });
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
            { id: "customers", label: t("adminCustomers"), Icon: Users },
            { id: "models", label: t("adminModels"), Icon: Cpu },
            { id: "analytics", label: t("adminAnalytics"), Icon: BarChart3 },
            { id: "resources", label: t("adminResources"), Icon: RefreshCw },
            { id: "incidents", label: t("adminIncidents"), Icon: Radio },
            { id: "audit", label: t("adminAudit"), Icon: History },
          ].map(({ id, label, Icon }) => {
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
                <Button variant="primary" onClick={() => setTopUpOpen(true)}>
                  <Plus size={15} style={{ marginInlineEnd: 4 }} />
                  {t("manualTopUp")}
                </Button>
              )}
            </div>

            {/* Customer Overview Card */}
            <div
              style={{
                background: "var(--surface)",
                border: "1px solid var(--border)",
                borderRadius: "var(--radius-lg)",
                padding: "var(--space-5)",
                marginBottom: "var(--space-6)",
              }}
            >
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "flex-start",
                  flexWrap: "wrap",
                  gap: "var(--space-4)",
                }}
              >
                <div>
                  <div
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: "var(--space-2)",
                      marginBottom: 4,
                    }}
                  >
                    <h2
                      style={{
                        fontSize: "var(--text-lg)",
                        fontWeight: 600,
                        margin: 0,
                      }}
                    >
                      {accountDisplayName(user)}
                    </h2>
                    <span
                      style={{
                        padding: "2px 8px",
                        borderRadius: "var(--radius-sm)",
                        fontSize: 11,
                        fontWeight: 600,
                        background:
                          user.accountStatus === "normal"
                            ? "var(--success-soft)"
                            : "var(--danger-soft)",
                        color:
                          user.accountStatus === "normal"
                            ? "var(--success)"
                            : "var(--danger)",
                      }}
                    >
                      {user.accountStatus.toUpperCase()}
                    </span>
                  </div>
                  <div
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                    }}
                  >
                    {user.email} ·{" "}
                    {text(
                      "注册于 2026-09-01 · 迪拜时区",
                      "Registered 2026-09-01 · Dubai time",
                      "مسجل في 2026-09-01 · توقيت دبي",
                    )}
                  </div>
                </div>

                <div style={{ display: "flex", gap: "var(--space-3)" }}>
                  <Button
                    size="small"
                    variant="secondary"
                    onClick={() => {
                      const newLim = prompt(
                        "Set account concurrency limit (default 50):",
                        user.concurrencyLimit.toString(),
                      );
                      if (newLim)
                        adminUpdateConcurrency(parseInt(newLim) || 50);
                    }}
                  >
                    {t("concurrencyLimit")}: <b>{user.concurrencyLimit}</b>
                  </Button>

                  <Button
                    size="small"
                    variant={
                      user.accountStatus === "normal" ? "danger" : "primary"
                    }
                    onClick={adminToggleAccountStatus}
                  >
                    {user.accountStatus === "normal"
                      ? t("disableAccount")
                      : t("enableAccount")}
                  </Button>
                </div>
              </div>

              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))",
                  gap: "var(--space-4)",
                  marginTop: "var(--space-4)",
                  paddingTop: "var(--space-4)",
                  borderTop: "1px solid var(--border)",
                }}
              >
                <div>
                  <div
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                    }}
                  >
                    {text(
                      "当前预付余额",
                      "Current prepaid balance",
                      "الرصيد الحالي",
                    )}
                  </div>
                  <div
                    className="mono"
                    style={{
                      fontSize: "var(--text-xl)",
                      fontWeight: "var(--weight-bold)",
                      color: "var(--brand)",
                    }}
                  >
                    {formatMoney(balance)}
                  </div>
                </div>
                <div>
                  <div
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                    }}
                  >
                    {text(
                      "本月累计用量",
                      "Month-to-date usage",
                      "استخدام الشهر الحالي",
                    )}
                  </div>
                  <div
                    className="mono"
                    style={{ fontSize: "var(--text-xl)", fontWeight: 600 }}
                  >
                    $14.2850
                  </div>
                </div>
                <div>
                  <div
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                    }}
                  >
                    {text(
                      "使用中的团队密钥",
                      "Active team keys",
                      "مفاتيح الفريق النشطة",
                    )}
                  </div>
                  <div
                    className="mono"
                    style={{ fontSize: "var(--text-xl)", fontWeight: 600 }}
                  >
                    3
                  </div>
                </div>
              </div>
            </div>

            {/* Pending Top-up Reviews Table */}
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
                  {t("topUpReviews")} ({adminReviews.length})
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
                      {text("单号", "Record ID", "معرّف السجل")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("账号", "Account", "الحساب")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("金额", "Amount", "المبلغ")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("类型与原因", "Type & reason", "النوع والسبب")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("操作人", "Operator", "المشغّل")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {text("操作时间", "Time", "الوقت")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      {t("status")}
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "end" }}>
                      {text("操作", "Actions", "الإجراءات")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {adminReviews.map((rev) => {
                    return (
                      <tr
                        key={rev.id}
                        style={{ borderBottom: "1px solid var(--border)" }}
                      >
                        <td className="mono" style={{ padding: "12px 14px" }}>
                          {rev.id}
                        </td>
                        <td style={{ padding: "12px 14px", fontWeight: 500 }}>
                          {rev.enterpriseName}
                        </td>
                        <td
                          className="mono"
                          style={{
                            padding: "12px 14px",
                            fontWeight: 600,
                            color: "var(--brand)",
                          }}
                        >
                          ${rev.amount_usd}
                        </td>
                        <td style={{ padding: "12px 14px" }}>
                          <span
                            style={{
                              fontSize: 11,
                              padding: "2px 6px",
                              background: "var(--subtle)",
                              borderRadius: 3,
                              marginInlineEnd: 6,
                            }}
                          >
                            {rev.type}
                          </span>
                          <span
                            style={{
                              color: "var(--muted)",
                              fontSize: "var(--text-xs)",
                            }}
                          >
                            {rev.reason}
                          </span>
                        </td>
                        <td
                          style={{
                            padding: "12px 14px",
                            fontSize: "var(--text-xs)",
                            color: "var(--muted)",
                          }}
                        >
                          {rev.operator}
                        </td>
                        <td
                          className="mono"
                          style={{
                            padding: "12px 14px",
                            fontSize: "var(--text-xs)",
                            color: "var(--muted)",
                          }}
                        >
                          {new Date(rev.created_at).toLocaleString()}
                        </td>
                        <td style={{ padding: "12px 14px" }}>
                          {/* #105：单人提交即入账，只有「已入账」一种状态。 */}
                          <span
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              fontWeight: 600,
                              background: "var(--success-soft)",
                              color: "var(--success)",
                            }}
                          >
                            {text("已入账", "CREDITED", "تم الإيداع")}
                          </span>
                        </td>
                        <td style={{ padding: "12px 14px", textAlign: "end" }}>
                          <span
                            style={{
                              fontSize: "var(--text-xs)",
                              color: "var(--muted)",
                            }}
                          >
                            {text(
                              "已记入审计日志",
                              "Audit log written",
                              "تم تسجيله في سجل التدقيق",
                            )}
                          </span>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </section>
          </div>
        )}

        {/* Keep drafts mounted when switching admin sections. */}
        <div hidden={tab !== "models"}>
          <ModelPricingAdmin role={role} />
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
                    "近 30 天平台充值",
                    "30-day platform top-ups",
                    "شحن المنصة خلال 30 يومًا",
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
                  $42,850.00
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
                  {text(
                    "累计调用次数",
                    "Total requests served",
                    "إجمالي الطلبات",
                  )}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-2xl)",
                    fontWeight: "var(--weight-bold)",
                  }}
                >
                  1,248,910
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
                  {text("活跃调用账号", "Active accounts", "الحسابات النشطة")}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-2xl)",
                    fontWeight: "var(--weight-bold)",
                  }}
                >
                  184
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
                  {text(
                    "路由成功率（演示）",
                    "Routing success (demo)",
                    "نجاح التوجيه (تجريبي)",
                  )}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-2xl)",
                    fontWeight: "var(--weight-bold)",
                    color: "var(--success)",
                  }}
                >
                  99.98%
                </div>
              </div>
            </div>
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

            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
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
                  style={{ fontSize: "var(--text-xs)", color: "var(--muted)" }}
                >
                  {t("snapshotSeq")}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-lg)",
                    fontWeight: 600,
                    marginTop: 4,
                  }}
                >
                  seq_20260918_144200
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
                  style={{ fontSize: "var(--text-xs)", color: "var(--muted)" }}
                >
                  {t("snapshotAge")}
                </div>
                <div
                  className="mono"
                  style={{
                    fontSize: "var(--text-lg)",
                    fontWeight: 600,
                    color: "var(--success)",
                    marginTop: 4,
                  }}
                >
                  {text(
                    "42 秒 · 正常",
                    "42 seconds · Healthy",
                    "42 ثانية · سليم",
                  )}
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
                  style={{ fontSize: "var(--text-xs)", color: "var(--muted)" }}
                >
                  {text("网关实例", "Gateway instances", "مثيلات البوابة")}
                </div>
                <div
                  style={{
                    fontSize: "var(--text-sm)",
                    fontWeight: 600,
                    color: "var(--success)",
                    marginTop: 4,
                  }}
                >
                  {text(
                    "4 / 4 个实例已同步",
                    "4 / 4 instances in sync",
                    "4 / 4 مثيلات متزامنة",
                  )}
                </div>
              </div>
            </div>
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
                      Time (UTC+4)
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      Operator
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      Role
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      Action
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      Object
                    </th>
                    <th style={{ padding: "10px 14px", textAlign: "start" }}>
                      Audit Summary
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {[
                    {
                      time: "2026-09-18 14:41",
                      op: "Alex Chen",
                      role: "Finance",
                      action: "Top-up Created",
                      obj: "rev_801",
                      desc: "Initiated $2,500 manual top-up (pending review)",
                    },
                    {
                      time: "2026-09-18 13:20",
                      op: "Admin User",
                      role: "Super Admin",
                      action: "Key Viewed",
                      obj: "pk_2",
                      desc: "Password re-verified to view plaintext key",
                    },
                    {
                      time: "2026-09-17 16:22",
                      op: "Michael Vance",
                      role: "Super Admin",
                      action: "Top-up Approved",
                      obj: "rev_802",
                      desc: "Approved $500 compensation top-up",
                    },
                    {
                      time: "2026-09-17 10:15",
                      op: "Sara Lin",
                      role: "Ops",
                      action: "Concurrency Adjusted",
                      obj: "Account",
                      desc: "Limit updated 50 -> 100",
                    },
                  ].map((row, idx) => (
                    <tr
                      key={idx}
                      style={{ borderBottom: "1px solid var(--border)" }}
                    >
                      <td
                        className="mono"
                        style={{ padding: "10px 14px", fontSize: 12 }}
                      >
                        {row.time}
                      </td>
                      <td style={{ padding: "10px 14px", fontWeight: 500 }}>
                        {row.op}
                      </td>
                      <td
                        style={{
                          padding: "10px 14px",
                          fontSize: 12,
                          color: "var(--muted)",
                        }}
                      >
                        {row.role}
                      </td>
                      <td style={{ padding: "10px 14px", fontWeight: 500 }}>
                        {row.action}
                      </td>
                      <td
                        className="mono"
                        style={{ padding: "10px 14px", fontSize: 12 }}
                      >
                        {row.obj}
                      </td>
                      <td
                        style={{
                          padding: "10px 14px",
                          color: "var(--secondary)",
                        }}
                      >
                        {row.desc}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
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
            <Button variant="primary" type="submit" form="admin-top-up-form">
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
    </div>
  );
}
