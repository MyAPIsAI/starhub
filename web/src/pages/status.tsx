import { PublicHeader } from "../components/product-shell";
import { Button } from "../components/ui/button";
import { IncidentList } from "../features/incident-list";
import { useSettingsText } from "../features/settings-text";
import { usePublicIncidents } from "../lib/api/incident";
import {
  usePublicStatus,
  type PublicServiceState,
  type PublicStatusModel,
  type PublicStatusMetric,
} from "../lib/api/status";
import "./status.css";

const rate = (value: number | null) =>
  value === null ? "—" : (value * 100).toFixed(2) + "%";
const sampleState = (metric: PublicStatusMetric): PublicServiceState =>
  metric.total_requests === 0
    ? "unknown"
    : metric.success_count === 0
      ? "unavailable"
      : metric.failed_count > 0
        ? "degraded"
        : "operational";

export default function StatusPage() {
  const { text, lang } = useSettingsText();
  const query = usePublicStatus();
  const notices = usePublicIncidents();
  const view = query.isError ? undefined : query.data;
  const incidentImpact =
    !notices.isError &&
    notices.data?.some(
      (notice) =>
        notice.status !== "resolved" &&
        ["warning", "critical", "maintenance"].includes(notice.severity),
    );
  let state: PublicServiceState = incidentImpact
    ? "degraded"
    : (view?.state ?? "unknown");
  const unconfirmed =
    query.isPending || query.isError || notices.isPending || notices.isError;
  if (unconfirmed && state === "operational") state = "unknown";
  const labels: Record<PublicServiceState, string> = {
    operational: text(
      "近期请求正常",
      "Recent requests successful",
      "الطلبات الأخيرة ناجحة",
    ),
    degraded: text(
      "部分服务受影响",
      "Some services affected",
      "بعض الخدمات متأثرة",
    ),
    unavailable: text(
      "当前资源不可用",
      "Resources currently unavailable",
      "الموارد غير متاحة حاليًا",
    ),
    unknown: text(
      "暂无近期请求数据",
      "No recent request data",
      "لا توجد بيانات طلبات حديثة",
    ),
  };
  const groups = new Map<string, PublicStatusModel[]>();
  for (const model of view?.models ?? []) {
    const vendor =
      model.vendor === "unknown"
        ? text("其他", "Other", "أخرى")
        : model.vendor_display || model.vendor;
    groups.set(vendor, [...(groups.get(vendor) ?? []), model]);
  }
  const updated = view
    ? new Intl.DateTimeFormat(lang, {
        dateStyle: "medium",
        timeStyle: "medium",
        timeZone: "UTC",
      }).format(new Date(view.generated_at * 1000)) + " UTC"
    : null;
  const dailyLabel = text(
    "每日请求数据",
    "Daily request data",
    "بيانات الطلبات اليومية",
  );
  const noSamples = text("暂无样本", "No samples", "لا توجد عينات");
  return (
    <div
      className="brand-home public-refined status-refined"
      style={{ minHeight: "100vh" }}
    >
      <PublicHeader />
      <main className="status-content public-status-content">
        <section
          className="public-status-banner"
          data-status-state={state}
          aria-busy={query.isFetching}
        >
          <h1>
            {query.isPending
              ? text(
                  "正在读取服务状态…",
                  "Loading service status…",
                  "جارٍ تحميل حالة الخدمة…",
                )
              : unconfirmed && state === "unknown"
                ? text(
                    "服务状态待确认",
                    "Service status unconfirmed",
                    "حالة الخدمة غير مؤكدة",
                  )
                : view?.models.length === 0 && !incidentImpact
                  ? text(
                      "暂无公开模型",
                      "No public models",
                      "لا توجد نماذج عامة",
                    )
                  : labels[state]}
          </h1>
          <p>
            {text(
              "根据实际请求与当前公开资源统计，每分钟更新。请求成功率不代表连续在线率。",
              "Based on recorded requests and current public resources, refreshed every minute. Request success rate is not continuous uptime.",
              "استنادًا إلى الطلبات المسجلة والموارد العامة الحالية، مع تحديث كل دقيقة. معدل نجاح الطلبات لا يمثل التوفر المستمر.",
            )}
          </p>
          {updated && (
            <p>
              {text("更新于", "Updated", "آخر تحديث")}{" "}
              <time
                dateTime={new Date(view!.generated_at * 1000).toISOString()}
                dir="ltr"
              >
                {updated}
              </time>
            </p>
          )}
        </section>

        <section aria-labelledby="public-status-notices">
          <h2 id="public-status-notices">
            {text("事件公告", "Service notices", "إشعارات الخدمة")}
          </h2>
          <IncidentList
            incidents={notices.data}
            loading={notices.isPending}
            error={notices.isError}
            refreshing={notices.isFetching}
            onRetry={() => void notices.refetch()}
            publicView
          />
        </section>

        <section
          aria-label={text(
            "模型服务状态",
            "Model service status",
            "حالة خدمة النماذج",
          )}
          data-status-models
        >
          {query.isPending ? (
            <p role="status">
              {text(
                "正在读取模型指标…",
                "Loading model metrics…",
                "جارٍ تحميل مقاييس النماذج…",
              )}
            </p>
          ) : query.isError ? (
            <div className="public-status-feedback" role="alert">
              <p>
                {text(
                  "服务指标加载失败，当前状态无法确认，请重试。",
                  "Service metrics could not be loaded. Current status is unconfirmed. Please retry.",
                  "تعذر تحميل مقاييس الخدمة. الحالة الحالية غير مؤكدة. أعد المحاولة.",
                )}
              </p>
              <Button
                disabled={query.isFetching}
                onClick={() => void query.refetch()}
              >
                {text("重试", "Retry", "إعادة المحاولة")}
              </Button>
            </div>
          ) : !view?.models.length ? (
            <p role="status">
              {text(
                "暂无已配置的公开模型。",
                "No public models are configured.",
                "لم يتم إعداد نماذج عامة.",
              )}
            </p>
          ) : (
            [...groups].map(([vendor, models]) => (
              <section key={vendor}>
                <h2>
                  {vendor} ({models.length})
                </h2>
                {models.map((model) => (
                  <article
                    className="public-status-card"
                    key={model.id}
                    data-status-model={model.id}
                  >
                    <div className="public-status-model-heading">
                      <h3 dir="auto">{model.id}</h3>
                      <span
                        className="public-status-badge"
                        data-state={model.state}
                      >
                        {model.state === "unknown"
                          ? noSamples
                          : labels[model.state]}
                      </span>
                    </div>
                    <dl className="public-status-stats">
                      <div>
                        <dt>
                          {text(
                            "近 24 小时请求成功率",
                            "24h request success rate",
                            "معدل نجاح الطلبات خلال 24 ساعة",
                          )}
                        </dt>
                        <dd data-metric="recent-rate">
                          {rate(model.recent.success_rate)}
                        </dd>
                      </div>
                      <div>
                        <dt>
                          {text(
                            "近 24 小时平均响应耗时",
                            "24h average response time",
                            "متوسط وقت الاستجابة خلال 24 ساعة",
                          )}
                        </dt>
                        <dd data-metric="duration">
                          {model.recent.avg_duration_ms === null
                            ? "—"
                            : model.recent.avg_duration_ms + " ms"}
                        </dd>
                      </div>
                      <div>
                        <dt>
                          {text(
                            "近 90 天请求成功率",
                            "90-day request success rate",
                            "معدل نجاح الطلبات خلال 90 يومًا",
                          )}
                        </dt>
                        <dd data-metric="history-rate">
                          {rate(model.history.success_rate)}
                        </dd>
                      </div>
                    </dl>
                    <div className="public-status-bars" aria-hidden="true">
                      {model.days.map((day) => (
                        <span
                          key={day.date}
                          data-day-state={sampleState(day)}
                          title={
                            day.date +
                            " UTC: " +
                            (day.total_requests
                              ? rate(day.success_rate)
                              : noSamples)
                          }
                        />
                      ))}
                    </div>
                    <div className="public-status-dates">
                      <time dateTime={view.history_start}>
                        {view.history_start}
                      </time>
                      <span>UTC</span>
                      <time dateTime={model.days.at(-1)?.date}>
                        {model.days.at(-1)?.date}
                      </time>
                    </div>
                    <details>
                      <summary>
                        {text(
                          "查看每日数据",
                          "View daily data",
                          "عرض البيانات اليومية",
                        )}
                      </summary>
                      <div
                        className="public-status-history"
                        tabIndex={0}
                        role="region"
                        aria-label={dailyLabel}
                      >
                        <table>
                          <thead>
                            <tr>
                              <th>
                                {text(
                                  "日期（UTC）",
                                  "Date (UTC)",
                                  "التاريخ (UTC)",
                                )}
                              </th>
                              <th>{text("请求数", "Requests", "الطلبات")}</th>
                              <th>
                                {text("成功率", "Success rate", "معدل النجاح")}
                              </th>
                            </tr>
                          </thead>
                          <tbody>
                            {model.days.map((day) => (
                              <tr key={day.date}>
                                <td>
                                  <time dateTime={day.date} dir="ltr">
                                    {day.date}
                                  </time>
                                </td>
                                <td>{day.total_requests}</td>
                                <td>
                                  {day.total_requests
                                    ? rate(day.success_rate)
                                    : noSamples}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </details>
                  </article>
                ))}
              </section>
            ))
          )}
        </section>
      </main>
    </div>
  );
}
