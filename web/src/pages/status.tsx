import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { CheckCircle2, AlertTriangle, Radio } from "lucide-react";
import { PublicHeader } from "../components/product-shell";
import { catalogModels } from "../config/models";
import { useProductPreview } from "../lib/product-preview";
import { usePreferences } from "../lib/preferences";

export default function StatusPage() {
  const { t } = useTranslation();
  const lang = usePreferences((s) => s.language);
  const incidents = useProductPreview((s) => s.incidents);
  const upstreamOutage = useProductPreview((s) => s.demoMode.upstreamOutage);

  // Group models by vendor
  const vendorGroups = useMemo(() => {
    const groups: Record<string, typeof catalogModels> = {};
    for (const m of catalogModels) {
      if (!groups[m.vendor]) groups[m.vendor] = [];
      groups[m.vendor].push(m);
    }
    return groups;
  }, []);

  // Generate 90-day data points for a model
  const getDailyBars = (isImpacted: boolean) => {
    return Array.from({ length: 90 }, (_, i) => {
      if (i === 89 && isImpacted) {
        return { status: "degraded", rate: "97.2%", day: `Day ${i + 1}` };
      }
      if (i % 23 === 0) {
        return { status: "minor", rate: "99.4%", day: `Day ${i + 1}` };
      }
      if (i < 5) {
        return { status: "insufficient", rate: "N/A", day: `Day ${i + 1}` };
      }
      return { status: "healthy", rate: "99.98%", day: `Day ${i + 1}` };
    });
  };

  const isGlobalDegraded =
    upstreamOutage || incidents.some((inc) => inc.status !== "resolved");

  return (
    <div
      className="brand-home public-refined status-refined"
      style={{ minHeight: "100vh", display: "flex", flexDirection: "column" }}
    >
      <PublicHeader />

      <main
        className="status-content"
        style={{
          padding: "var(--space-8) var(--space-6)",
          maxWidth: 1100,
          margin: "0 auto",
          width: "100%",
          boxSizing: "border-box",
        }}
      >
        {/* Top Health Banner */}
        <div
          style={{
            padding: "var(--space-6)",
            borderRadius: "var(--radius-lg)",
            background: isGlobalDegraded
              ? "var(--warning-soft)"
              : "var(--success-soft)",
            border: `1px solid ${isGlobalDegraded ? "var(--warning)" : "var(--success)"}`,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            marginBottom: "var(--space-8)",
            flexWrap: "wrap",
            gap: "var(--space-4)",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "var(--space-3)",
            }}
          >
            {isGlobalDegraded ? (
              <AlertTriangle size={28} style={{ color: "var(--warning)" }} />
            ) : (
              <CheckCircle2 size={28} style={{ color: "var(--success)" }} />
            )}
            <div>
              <h1
                style={{
                  fontSize: "var(--text-xl)",
                  fontWeight: "var(--weight-bold)",
                  margin: "0 0 2px 0",
                }}
              >
                {isGlobalDegraded ? t("someDegraded") : t("allOperational")}
              </h1>
              <p
                style={{
                  margin: 0,
                  fontSize: "var(--text-sm)",
                  color: "var(--secondary)",
                }}
              >
                {t("statusSubtitle")}
              </p>
            </div>
          </div>

          <div
            className="mono"
            style={{
              fontSize: "var(--text-xs)",
              color: "var(--muted)",
              display: "flex",
              alignItems: "center",
              gap: 6,
            }}
          >
            <Radio size={12} style={{ color: "var(--brand)" }} />
            {lang === "zh"
              ? "演示数据 · 迪拜时区（UTC+4）"
              : lang === "ar"
                ? "بيانات تجريبية · دبي (UTC+4)"
                : "Demo data · Dubai (UTC+4)"}
          </div>
        </div>

        {/* Active Incidents Announcement Section */}
        {incidents.length > 0 && (
          <section style={{ marginBottom: "var(--space-8)" }}>
            <h2
              style={{
                fontSize: "var(--text-lg)",
                fontWeight: "var(--weight-bold)",
                marginBottom: "var(--space-3)",
              }}
            >
              {t("activeIncidents")}
            </h2>
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: "var(--space-3)",
              }}
            >
              {incidents.map((incident) => (
                <div
                  key={incident.id}
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
                      alignItems: "flex-start",
                      marginBottom: "var(--space-2)",
                    }}
                  >
                    <div
                      style={{
                        fontWeight: "var(--weight-semibold)",
                        fontSize: "var(--text-base)",
                      }}
                    >
                      {incident.title[lang] || incident.title.en}
                    </div>
                    <span
                      style={{
                        fontSize: "var(--text-xs)",
                        padding: "2px 8px",
                        borderRadius: "var(--radius-sm)",
                        background:
                          incident.status === "resolved"
                            ? "var(--success-soft)"
                            : "var(--warning-soft)",
                        color:
                          incident.status === "resolved"
                            ? "var(--success)"
                            : "var(--warning)",
                        fontWeight: "var(--weight-medium)",
                        textTransform: "capitalize",
                      }}
                    >
                      {lang === "zh"
                        ? (
                            {
                              resolved: "已解决",
                              investigating: "调查中",
                              identified: "已定位",
                              monitoring: "监测中",
                            } as Record<string, string>
                          )[incident.status] || incident.status
                        : incident.status}
                    </span>
                  </div>
                  <p
                    style={{
                      fontSize: "var(--text-sm)",
                      color: "var(--muted)",
                      margin: "0 0 var(--space-2) 0",
                      lineHeight: "var(--leading-base)",
                    }}
                  >
                    {incident.content[lang] || incident.content.en}
                  </p>
                  <div
                    className="mono"
                    style={{
                      fontSize: "var(--text-xs)",
                      color: "var(--muted)",
                    }}
                  >
                    {lang === "zh"
                      ? "更新于"
                      : lang === "ar"
                        ? "تحديث"
                        : "Updated"}{" "}
                    {incident.updatedAt}
                  </div>
                </div>
              ))}
            </div>
          </section>
        )}

        {/* Grouped Vendor Models List */}
        <section>
          {Object.entries(vendorGroups).map(([vendor, models]) => (
            <div key={vendor} style={{ marginBottom: "var(--space-8)" }}>
              <h2
                style={{
                  fontSize: "var(--text-lg)",
                  fontWeight: "var(--weight-bold)",
                  marginBottom: "var(--space-4)",
                  display: "flex",
                  alignItems: "center",
                  gap: "var(--space-2)",
                }}
              >
                <span>{vendor}</span>
                <span
                  style={{
                    fontSize: "var(--text-xs)",
                    color: "var(--muted)",
                    fontWeight: "normal",
                  }}
                >
                  ({models.length}{" "}
                  {lang === "zh"
                    ? "个模型"
                    : lang === "ar"
                      ? "نموذج"
                      : "models"}
                  )
                </span>
              </h2>

              <div
                style={{
                  display: "flex",
                  flexDirection: "column",
                  gap: "var(--space-4)",
                }}
              >
                {models.map((model) => {
                  const isAnthropicOutage =
                    upstreamOutage && model.vendor === "Anthropic";
                  const bars = getDailyBars(isAnthropicOutage);

                  return (
                    <div
                      key={model.id}
                      style={{
                        background: "var(--surface)",
                        border: "1px solid var(--border)",
                        borderRadius: "var(--radius-lg)",
                        padding: "var(--space-5)",
                      }}
                    >
                      <div
                        style={{
                          display: "flex",
                          justifyContent: "space-between",
                          alignItems: "center",
                          marginBottom: "var(--space-4)",
                          flexWrap: "wrap",
                          gap: "var(--space-2)",
                        }}
                      >
                        <div>
                          <div
                            style={{
                              display: "flex",
                              alignItems: "center",
                              gap: "var(--space-2)",
                            }}
                          >
                            <h3
                              style={{
                                fontSize: "var(--text-base)",
                                fontWeight: "var(--weight-semibold)",
                                margin: 0,
                              }}
                            >
                              {model.name}
                            </h3>
                            <span
                              className="mono"
                              style={{
                                fontSize: "var(--text-xs)",
                                color: "var(--muted)",
                              }}
                            >
                              {model.id}
                            </span>
                          </div>
                        </div>

                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: "var(--space-4)",
                          }}
                        >
                          <div style={{ textAlign: "end" }}>
                            <div
                              style={{
                                fontSize: "var(--text-xs)",
                                color: "var(--muted)",
                              }}
                            >
                              {t("pastDaySuccess")}
                            </div>
                            <div
                              className="mono"
                              style={{
                                fontSize: "var(--text-sm)",
                                fontWeight: "var(--weight-bold)",
                                color: isAnthropicOutage
                                  ? "var(--warning)"
                                  : "var(--text)",
                              }}
                            >
                              {isAnthropicOutage ? "97.2%" : "99.98%"}
                            </div>
                          </div>

                          <div style={{ textAlign: "end" }}>
                            <div
                              style={{
                                fontSize: "var(--text-xs)",
                                color: "var(--muted)",
                              }}
                            >
                              {t("ttftLatency")}
                            </div>
                            <div
                              className="mono"
                              style={{
                                fontSize: "var(--text-sm)",
                                fontWeight: "var(--weight-semibold)",
                              }}
                            >
                              {isAnthropicOutage ? "1,240ms" : "320ms"}
                            </div>
                          </div>

                          <span
                            style={{
                              display: "inline-flex",
                              alignItems: "center",
                              gap: 6,
                              fontSize: "var(--text-xs)",
                              fontWeight: "var(--weight-semibold)",
                              color: isAnthropicOutage
                                ? "var(--warning)"
                                : "var(--success)",
                            }}
                          >
                            <span
                              style={{
                                width: 8,
                                height: 8,
                                borderRadius: "50%",
                                background: isAnthropicOutage
                                  ? "var(--warning)"
                                  : "var(--success)",
                              }}
                            />
                            {isAnthropicOutage
                              ? t("degraded")
                              : t("operational")}
                          </span>
                        </div>
                      </div>

                      {/* 90-Day Uptime Bars */}
                      <div>
                        <div
                          style={{
                            display: "flex",
                            gap: 2,
                            height: 28,
                            alignItems: "stretch",
                            marginBottom: 6,
                          }}
                        >
                          {bars.map((bar, idx) => (
                            <div
                              key={idx}
                              title={`${bar.day}: ${bar.rate}`}
                              style={{
                                flex: 1,
                                background:
                                  bar.status === "healthy"
                                    ? "var(--success)"
                                    : bar.status === "minor"
                                      ? "#138570"
                                      : bar.status === "degraded"
                                        ? "var(--warning)"
                                        : "var(--neutral-5)",
                                borderRadius: 1,
                                opacity:
                                  bar.status === "insufficient" ? 0.35 : 1,
                              }}
                            />
                          ))}
                        </div>
                        <div
                          style={{
                            display: "flex",
                            justifyContent: "space-between",
                            fontSize: "var(--text-xs)",
                            color: "var(--muted)",
                          }}
                        >
                          <span>
                            {lang === "zh"
                              ? "90 天前"
                              : lang === "ar"
                                ? "قبل 90 يومًا"
                                : "90 days ago"}
                          </span>
                          <span>{t("uptime90Days")}: 99.95%</span>
                          <span>
                            {lang === "zh"
                              ? "今天"
                              : lang === "ar"
                                ? "اليوم"
                                : "Today"}
                          </span>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </section>
      </main>
    </div>
  );
}
