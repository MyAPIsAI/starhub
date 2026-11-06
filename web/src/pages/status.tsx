import { localizedMessage } from "../i18n/messages";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { CheckCircle2, AlertTriangle, Radio } from "lucide-react";
import { PublicHeader } from "../components/product-shell";
import { catalogModels } from "../config/models";
import { useProductPreview } from "../lib/product-preview";
import { usePreferences } from "../lib/preferences";
import { usePublishedAnnouncements } from "../lib/api/announcements";
import { PublicAnnouncements } from "../features/public-announcements";
import { useServiceQuality } from "../lib/api/service-quality";

export default function StatusPage() {
  const { t } = useTranslation();
  const lang = usePreferences((s) => s.language);
  const announcementQuery = usePublishedAnnouncements();
  const incidents = announcementQuery.isError
    ? []
    : (announcementQuery.data?.items ?? []);
  const upstreamOutage = useProductPreview((s) => s.demoMode.upstreamOutage);
  // Fetch real service-quality metrics (last 7 days). Degrades to
  // demo defaults when the endpoint is unavailable or has no data.
  const sqQuery = useServiceQuality(168);

  // Build a model → quality metric lookup from the backend.
  const metricByModel = useMemo(() => {
    const map = new Map<
      string,
      { successRate: number; avgDurationMs: number }
    >();
    for (const m of sqQuery.data?.by_model ?? []) {
      map.set(m.model, {
        successRate: m.success_rate,
        avgDurationMs: m.avg_duration_ms,
      });
    }
    return map;
  }, [sqQuery.data]);

  // Overall health from backend metrics.
  const overall = sqQuery.data?.overall;
  const hasRealData = (overall?.total_requests ?? 0) > 0;
  const overallSuccessRate = overall?.success_rate ?? 1;
  const isBackendDegraded = hasRealData && overallSuccessRate < 0.99;

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
    isBackendDegraded ||
    upstreamOutage ||
    incidents.some((inc) => inc.status !== "resolved");

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
        <PublicAnnouncements query={announcementQuery} />
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
            {hasRealData
              ? localizedMessage("live_data_dubai_utc_4_567be1ee66", lang)
              : localizedMessage("no_request_data_yet_e3608cbfc3", lang)}
          </div>
        </div>

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
                  ({models.length} {localizedMessage("models_77eff90879", lang)}
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
                  const metric = metricByModel.get(model.id);
                  const hasModelData = metric && metric.successRate > 0;
                  const isDegraded = hasModelData
                    ? metric!.successRate < 0.99
                    : upstreamOutage && model.vendor === "Anthropic";
                  const bars = getDailyBars(isDegraded);

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
                                color: isDegraded
                                  ? "var(--warning)"
                                  : "var(--text)",
                              }}
                            >
                              {hasModelData
                                ? (metric!.successRate * 100).toFixed(2) + "%"
                                : upstreamOutage && model.vendor === "Anthropic"
                                  ? "97.2%"
                                  : "—"}
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
                              {hasModelData && metric!.avgDurationMs > 0
                                ? metric!.avgDurationMs + "ms"
                                : "—"}
                            </div>
                          </div>

                          <span
                            style={{
                              display: "inline-flex",
                              alignItems: "center",
                              gap: 6,
                              fontSize: "var(--text-xs)",
                              fontWeight: "var(--weight-semibold)",
                              color: isDegraded
                                ? "var(--warning)"
                                : "var(--success)",
                            }}
                          >
                            <span
                              style={{
                                width: 8,
                                height: 8,
                                borderRadius: "50%",
                                background: isDegraded
                                  ? "var(--warning)"
                                  : "var(--success)",
                              }}
                            />
                            {isDegraded ? t("degraded") : t("operational")}
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
                            {localizedMessage("90_days_ago_a926d1b97a", lang)}
                          </span>
                          <span>
                            {t("uptime90Days")}:{" "}
                            {hasRealData
                              ? (overallSuccessRate * 100).toFixed(2) + "%"
                              : "—"}
                          </span>
                          <span>
                            {localizedMessage("today_3a255d525f", lang)}
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
