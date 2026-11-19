import { Button } from "../components/ui/button";
import type { ServiceIncident } from "../lib/api/incident";
import { useSettingsText } from "./settings-text";
import "./incident-list.css";

export function IncidentList({
  incidents = [],
  loading,
  error,
  refreshing,
  onRetry,
  publicView = false,
}: {
  incidents?: ServiceIncident[];
  loading: boolean;
  error: boolean;
  refreshing: boolean;
  onRetry: () => void;
  publicView?: boolean;
}) {
  const { text, lang } = useSettingsText();
  const Heading = publicView ? "h3" : "h2";
  const labels: Record<string, string> = {
    investigating: text("调查中", "Investigating", "قيد التحقيق"),
    identified: text("已定位", "Identified", "تم تحديد السبب"),
    monitoring: text("监测中", "Monitoring", "قيد المراقبة"),
    resolved: text("已解决", "Resolved", "تم الحل"),
  };
  return (
    <div className="incident-list" aria-busy={loading || refreshing}>
      {loading ? (
        <p role="status">
          {text("正在读取公告…", "Loading notices…", "جارٍ تحميل الإشعارات…")}
        </p>
      ) : error ? (
        <div role="alert" className="incident-error">
          <p>
            {text(
              "公告加载失败，请重试。",
              "Notices could not be loaded. Please retry.",
              "تعذر تحميل الإشعارات. أعد المحاولة.",
            )}
          </p>
          <Button disabled={refreshing} onClick={onRetry}>
            {text("重试", "Retry", "إعادة المحاولة")}
          </Button>
        </div>
      ) : !incidents.length ? (
        <p role="status">
          {text("暂无公告。", "No notices.", "لا توجد إشعارات.")}
        </p>
      ) : (
        incidents.map((incident) => {
          const date = new Date(incident.updated_at * 1000);
          const validDate = Number.isFinite(date.getTime());
          return (
            <article key={incident.id} data-incident-id={incident.id}>
              <header className="incident-heading">
                <Heading className="incident-title">{incident.title}</Heading>
                <span
                  className="incident-status"
                  data-incident-status={incident.status}
                >
                  {labels[incident.status] ?? incident.status}
                </span>
              </header>
              <p className="incident-description">{incident.description}</p>
              <div className="incident-meta">
                <span>
                  {text("受影响供应商", "Impacted vendor", "المزوّد المتأثر")}:{" "}
                  {incident.component || "—"}
                </span>
                <span>
                  {text("更新于", "Updated", "آخر تحديث")}{" "}
                  <time
                    dateTime={validDate ? date.toISOString() : undefined}
                    dir="ltr"
                  >
                    {validDate
                      ? `${new Intl.DateTimeFormat(lang, {
                          dateStyle: "short",
                          timeStyle: "medium",
                          timeZone: "UTC",
                        }).format(date)} UTC`
                      : "—"}
                  </time>
                </span>
              </div>
            </article>
          );
        })
      )}
    </div>
  );
}
