import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { apiGet } from "../lib/api/http";
import { useStaffText } from "../lib/api/staff";
import { useSessionQueryIdentity } from "../lib/system-access";
import { Button } from "../components/ui/button";

export type SnapshotStatus = {
  sequence: number;
  published_at: number | null;
  age_seconds: number | null;
  status: "fresh" | "expired" | "missing" | "unknown" | "clock_skew";
};
type MonitorHistory = {
  mismatches: {
    id: string;
    resource_id: string;
    snapshot_sequence: number;
    snapshot_price: number;
    event_price: number;
    event_version: number;
    at: number;
    resolved_at: number;
  }[];
  mismatch_total: number;
  incidents: {
    event_id: string;
    type: string;
    resource_id: string;
    gateway_id: string;
    occurred_at: number;
    delivered_at: number;
    attempts: number;
    delivery_status: string;
  }[];
  incident_total: number;
  events_configured: boolean;
  snapshot_configured: boolean;
  reports_configured: boolean;
  feeds: {
    kind: string;
    error_code: string;
    last_success_at: number;
    initialized: boolean;
  }[];
};

export function ResourceMonitorPanels({
  snapshot,
  receivedAt,
  failed,
  pending,
  selectResource,
  syncStatus,
  syncDisabled,
  onForceSync,
  children,
}: {
  snapshot?: SnapshotStatus;
  receivedAt: number;
  failed: boolean;
  pending: boolean;
  selectResource: (id: string) => void;
  syncStatus?: {
    last_aligned_at: number;
    failures: { id: number; at: number; code: string }[];
  };
  syncDisabled: boolean;
  onForceSync: () => void;
  children: ReactNode;
}) {
  const t = useStaffText(),
    identity = useSessionQueryIdentity();
  const [clock, setClock] = useState(Date.now());
  const [mismatchPage, setMismatchPage] = useState(1),
    [incidentPage, setIncidentPage] = useState(1);
  useEffect(() => {
    const timer = window.setInterval(() => setClock(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const history = useQuery({
    queryKey: [
      "staff-resource-monitoring",
      ...identity,
      mismatchPage,
      incidentPage,
    ],
    queryFn: () =>
      apiGet<MonitorHistory>(
        `/api/admin/resource-monitoring?mismatch_page=${mismatchPage}&incident_page=${incidentPage}`,
      ),
    refetchInterval: 5000,
  });
  const rows = history.isError ? undefined : history.data;
  const age =
    snapshot?.age_seconds == null
      ? null
      : snapshot.age_seconds +
        Math.max(0, Math.floor((clock - receivedAt) / 1000));
  const snapshotState = !snapshot
    ? "unknown"
    : snapshot.status === "fresh" || snapshot.status === "expired"
      ? age !== null && age > 180
        ? "expired"
        : "fresh"
      : snapshot.status;
  const date = (value?: number | null) =>
    value
      ? new Intl.DateTimeFormat(document.documentElement.lang || "en", {
          dateStyle: "short",
          timeStyle: "medium",
          timeZone: "UTC",
        }).format(new Date(value * 1000))
      : "—";
  const error = t(
    ...translatedMessage("unable_to_load_current_status_retry_d200806886"),
  );
  const ageText =
    age === null || age < 0
      ? "—"
      : t(
          ...translatedMessage("m_s_02cb837575", {
            p0: Math.floor(age / 60),
            p1: age % 60,
          }),
        );
  const statusText: Record<string, string> = {
    fresh: t(...translatedMessage("fresh_c05526e604")),
    expired: t(...translatedMessage("snapshot_expired_9820d583bf")),
    missing: t(...translatedMessage("no_snapshot_06d49e90e4")),
    unknown: t(...translatedMessage("unknown_3eb118e4ec")),
    clock_skew: t(...translatedMessage("invalid_publication_time_31067a7044")),
    pending: t(...translatedMessage("pending_fdc1587362")),
    retrying: t(...translatedMessage("retrying_89f804445e")),
    delivered: t(...translatedMessage("reported_23028fa12d")),
    not_configured: t(
      ...translatedMessage("awaiting_report_configuration_d0cf5e6747"),
    ),
  };
  const failureLabels: Record<string, string> = {
    source_unreachable: t(
      ...translatedMessage("resource_platform_unreachable_6a0272d30e"),
    ),
    source_rejected: t(
      ...translatedMessage("source_rejected_the_request_0c4725400b"),
    ),
    invalid_snapshot: t(
      ...translatedMessage("resource_validation_failed_ec65090949"),
    ),
    invalid_payload: t(
      ...translatedMessage("invalid_response_format_or_size_3f4b26609a"),
    ),
    version_conflict: t(
      ...translatedMessage("version_rollback_or_content_conflict_93dba0a33b"),
    ),
    checksum_mismatch: t(
      ...translatedMessage("catalog_checksum_mismatch_5d7315f458"),
    ),
    apply_failed: t(
      ...translatedMessage("apply_failed_published_data_retained_0e039a8f8d"),
    ),
  };
  const timestamp = (value: number) => (
    <time dateTime={value ? new Date(value * 1000).toISOString() : undefined}>
      {date(value)}
      {value ? " UTC" : ""}
    </time>
  );
  const paginate = (
    page: number,
    total: number,
    change: (value: number) => void,
  ) => (
    <div className="resource-status-pagination">
      <span>
        {t(...translatedMessage("total_301e42d384"))} {total}
      </span>
      <Button
        size="small"
        disabled={page <= 1}
        onClick={() => change(page - 1)}
      >
        {t(...translatedMessage("previous_2672646f39"))}
      </Button>
      <span>
        {page} / {Math.max(1, Math.ceil(total / 20))}
      </span>
      <Button
        size="small"
        disabled={page * 20 >= total}
        onClick={() => change(page + 1)}
      >
        {t(...translatedMessage("next_c0fd39244f"))}
      </Button>
    </div>
  );
  return (
    <>
      <section
        className="resource-status-panel resource-snapshot-panel"
        aria-labelledby="resource-snapshot-title"
      >
        <header className="resource-status-heading">
          <h2 id="resource-snapshot-title">
            {t(...translatedMessage("snapshot_status_1aefbafe24"))}
          </h2>
          {!failed && !pending && (
            <span
              className="resource-status-badge"
              data-state={
                snapshotState === "expired"
                  ? "alert"
                  : snapshotState === "fresh"
                    ? "normal"
                    : "unknown"
              }
            >
              {statusText[snapshotState]}
            </span>
          )}
        </header>
        {failed ? (
          <p role="alert">{error}</p>
        ) : pending ? (
          <p role="status">{t(...translatedMessage("loading_4c20385375"))}</p>
        ) : (
          <>
            <dl className="resource-status-summary">
              <div>
                <dt>
                  {t(...translatedMessage("snapshot_sequence_804a2514e7"))}
                </dt>
                <dd>{snapshot?.sequence || "—"}</dd>
              </div>
              <div>
                <dt>
                  {t(...translatedMessage("published_at_utc_9d25567e6d"))}
                </dt>
                <dd>{date(snapshot?.published_at)}</dd>
              </div>
              <div>
                <dt>{t(...translatedMessage("snapshot_age_7b6036babc"))}</dt>
                <dd
                  data-testid="snapshot-age"
                  data-snapshot-state={snapshotState}
                >
                  {ageText}
                </dd>
              </div>
            </dl>
            <p>
              {t(
                ...translatedMessage(
                  "snapshots_older_than_3_minutes_turn_red_a_fre_bfda8f50f6",
                ),
              )}
            </p>
            {snapshotState === "clock_skew" && (
              <p role="alert">
                {t(
                  ...translatedMessage(
                    "the_source_publication_time_is_ahead_of_the_s_a390c1b041",
                  ),
                )}
              </p>
            )}
          </>
        )}
        {rows && !rows.snapshot_configured && (
          <p className="resource-status-footnote">
            {t(
              ...translatedMessage(
                "the_minute_by_minute_snapshot_source_is_not_c_19a04c9610",
              ),
            )}
          </p>
        )}
      </section>
      {children}
      <div className="resource-monitor-grid">
        <section
          className="resource-status-panel resource-monitor-history"
          aria-labelledby="resource-failures-title"
        >
          <header>
            <h2 id="resource-failures-title">
              {t(...translatedMessage("sync_failure_history_title"))}
            </h2>
            <p>{t(...translatedMessage("sync_failure_history_description"))}</p>
          </header>
          {failed ? (
            <p role="alert">{error}</p>
          ) : pending ? (
            <p role="status">{t(...translatedMessage("loading_4c20385375"))}</p>
          ) : (
            syncStatus &&
            (syncStatus.failures.length ? (
              <ul
                className="resource-monitor-list"
                data-history="sync"
                tabIndex={0}
                aria-labelledby="resource-failures-title"
              >
                {syncStatus.failures.map((row) => (
                  <li key={row.id}>
                    <div className="resource-monitor-entry-heading">
                      {timestamp(row.at)}
                      {syncStatus.last_aligned_at > row.at && (
                        <span
                          className="resource-status-badge"
                          data-state="normal"
                        >
                          {t(
                            ...translatedMessage("sync_aligned_since_failure"),
                          )}
                        </span>
                      )}
                    </div>
                    <p>
                      {failureLabels[row.code] ??
                        t(
                          ...translatedMessage(
                            "synchronization_failed_6c3ef2b844",
                          ),
                        )}
                    </p>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="resource-monitor-empty">
                {t(...translatedMessage("no_failure_records_7cbc91c430"))}
              </p>
            ))
          )}
        </section>
        <section
          className="resource-status-panel resource-monitor-history"
          aria-labelledby="resource-mismatch-title"
        >
          <header>
            <h2 id="resource-mismatch-title">
              {t(...translatedMessage("price_mismatch_history_title"))}
            </h2>
            <p>
              {t(...translatedMessage("price_mismatch_history_description"))}
            </p>
          </header>
          <div className="resource-monitor-toolbar">
            <Button
              size="small"
              data-testid="history-force-resource-sync"
              disabled={syncDisabled}
              onClick={onForceSync}
            >
              {t(...translatedMessage("force_full_sync_fafc970d6d"))}
            </Button>
            <Button
              size="small"
              variant="ghost"
              disabled={history.isFetching}
              onClick={() => void history.refetch()}
            >
              {t(...translatedMessage("refresh_history_e25eee27af"))}
            </Button>
          </div>
          {history.isPending && (
            <p role="status">{t(...translatedMessage("loading_4c20385375"))}</p>
          )}
          {history.isError && <p role="alert">{error}</p>}
          {rows && (
            <>
              {!rows.events_configured && (
                <p className="resource-monitor-notice">
                  {t(
                    ...translatedMessage(
                      "the_event_source_is_not_configured_price_cons_b3b4ef0f1d",
                    ),
                  )}
                </p>
              )}
              {rows.feeds
                .filter((feed) => feed.error_code)
                .map((feed) => (
                  <p
                    key={feed.kind}
                    role="alert"
                    className="resource-monitor-notice"
                  >
                    {feed.kind === "events"
                      ? t(
                          ...translatedMessage(
                            "event_loading_failed_the_last_valid_data_is_r_e55829cf47",
                          ),
                        )
                      : t(
                          ...translatedMessage(
                            "snapshot_loading_failed_the_last_valid_snapsh_ec3bf6b24b",
                          ),
                        )}{" "}
                    {t(...translatedMessage("last_success_2d928f74f8"))}:{" "}
                    {timestamp(feed.last_success_at)}
                  </p>
                ))}
              {rows.mismatches.length ? (
                <ul
                  className="resource-monitor-list"
                  data-history="mismatches"
                  tabIndex={0}
                  aria-labelledby="resource-mismatch-title"
                >
                  {rows.mismatches.map((row) => (
                    <li key={row.id}>
                      <div className="resource-monitor-entry-heading">
                        <span
                          className="resource-status-badge"
                          data-state={row.resolved_at ? "normal" : "alert"}
                        >
                          {row.resolved_at
                            ? t(...translatedMessage("resolved_4b05490b80"))
                            : t(
                                ...translatedMessage(
                                  "awaiting_alignment_5343aa8dc3",
                                ),
                              )}
                        </span>
                        {timestamp(row.at)}
                      </div>
                      <code className="resource-monitor-identifier" dir="ltr">
                        {row.resource_id}
                      </code>
                      <p className="resource-monitor-prices">
                        <span>
                          {t(...translatedMessage("snapshot_p_93e887b871"))}{" "}
                          <bdi dir="ltr">P{row.snapshot_price}</bdi>
                        </span>
                        <span>
                          {t(...translatedMessage("event_p_e8ab98d7ff"))}{" "}
                          <bdi dir="ltr">P{row.event_price}</bdi>
                        </span>
                      </p>
                      <ResourceHistoryLinks
                        id={row.resource_id}
                        selectResource={selectResource}
                      />
                      <details className="resource-monitor-details">
                        <summary>
                          {t(...translatedMessage("monitor_record_details"))}
                        </summary>
                        <dl>
                          <div>
                            <dt>
                              {t(
                                ...translatedMessage("snapshot_seq_9eec99d0d4"),
                              )}
                            </dt>
                            <dd>{row.snapshot_sequence}</dd>
                          </div>
                          <div>
                            <dt>
                              {t(
                                ...translatedMessage(
                                  "event_version_ed1542f446",
                                ),
                              )}
                            </dt>
                            <dd>{row.event_version}</dd>
                          </div>
                          {row.resolved_at > 0 && (
                            <div>
                              <dt>
                                {t(...translatedMessage("monitor_resolved_at"))}
                              </dt>
                              <dd>{timestamp(row.resolved_at)}</dd>
                            </div>
                          )}
                        </dl>
                      </details>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="resource-monitor-empty">
                  {t(
                    ...translatedMessage(
                      "no_recorded_price_mismatches_b710b9f493",
                    ),
                  )}
                </p>
              )}
              {rows.mismatch_total > 20 &&
                paginate(mismatchPage, rows.mismatch_total, setMismatchPage)}
            </>
          )}
        </section>
        <section
          className="resource-status-panel resource-monitor-history"
          aria-labelledby="resource-incident-title"
        >
          <header>
            <h2 id="resource-incident-title">
              {t(...translatedMessage("incident_reports_a701f20f79"))}
            </h2>
            <p>{t(...translatedMessage("incident_history_description"))}</p>
          </header>
          {history.isPending && (
            <p role="status">{t(...translatedMessage("loading_4c20385375"))}</p>
          )}
          {history.isError && <p role="alert">{error}</p>}
          {rows && (
            <>
              {!rows.reports_configured && (
                <p className="resource-monitor-notice">
                  {t(
                    ...translatedMessage(
                      "the_report_endpoint_is_not_configured_events__850ed8a730",
                    ),
                  )}
                </p>
              )}
              {rows.incidents.length ? (
                <ul
                  className="resource-monitor-list"
                  data-history="incidents"
                  tabIndex={0}
                  aria-labelledby="resource-incident-title"
                >
                  {rows.incidents.map((row) => (
                    <li key={row.event_id}>
                      <div className="resource-monitor-entry-heading">
                        <strong>
                          {row.type === "half_open_failed" ? (
                            t(
                              ...translatedMessage(
                                "five_failed_half_open_probes_4d808ab8db",
                              ),
                            )
                          ) : row.type === "ewma_breach_twice" ? (
                            t(
                              ...translatedMessage(
                                "two_consecutive_ewma_breaches_e524b82efb",
                              ),
                            )
                          ) : (
                            <code dir="ltr">{row.type}</code>
                          )}
                        </strong>
                        {timestamp(row.occurred_at)}
                      </div>
                      <p className="resource-monitor-identifiers">
                        <code dir="ltr">{row.resource_id}</code>
                        <span aria-hidden="true"> · </span>
                        <code dir="ltr">{row.gateway_id}</code>
                      </p>
                      <div className="resource-monitor-delivery">
                        <span>
                          {t(
                            ...translatedMessage("delivery_status_96d9f75827"),
                          )}
                        </span>
                        <span
                          className="resource-status-badge"
                          data-state={
                            row.delivery_status === "delivered"
                              ? "normal"
                              : row.delivery_status === "retrying"
                                ? "retrying"
                                : "unknown"
                          }
                        >
                          {statusText[row.delivery_status] ??
                            statusText.unknown}
                        </span>
                      </div>
                      <ResourceHistoryLinks
                        id={row.resource_id}
                        selectResource={selectResource}
                      />
                      <details className="resource-monitor-details">
                        <summary>
                          {t(...translatedMessage("monitor_record_details"))}
                        </summary>
                        <dl>
                          <div>
                            <dt>
                              {t(...translatedMessage("event_id_27eff7c057"))}
                            </dt>
                            <dd>
                              <code dir="ltr">{row.event_id}</code>
                            </dd>
                          </div>
                          <div>
                            <dt>
                              {t(...translatedMessage("event_type_8f67e63113"))}
                            </dt>
                            <dd>
                              <code dir="ltr">{row.type}</code>
                            </dd>
                          </div>
                          <div>
                            <dt>
                              {t(...translatedMessage("attempts_4321915685"))}
                            </dt>
                            <dd>{row.attempts}</dd>
                          </div>
                          <div>
                            <dt>
                              {t(
                                ...translatedMessage(
                                  "reported_at_utc_44015dcecb",
                                ),
                              )}
                            </dt>
                            <dd>{timestamp(row.delivered_at)}</dd>
                          </div>
                        </dl>
                      </details>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="resource-monitor-empty">
                  {t(...translatedMessage("no_incident_reports_3df6b6cb8a"))}
                </p>
              )}
              {rows.incident_total > 20 &&
                paginate(incidentPage, rows.incident_total, setIncidentPage)}
            </>
          )}
        </section>
      </div>
    </>
  );
}

function ResourceHistoryLinks({
  id,
  selectResource,
}: {
  id: string;
  selectResource: (id: string) => void;
}) {
  const t = useStaffText(),
    identity = useSessionQueryIdentity();
  // Subscribe to the existing detail lookup without fetching once per history row.
  const detail = useQuery({
    queryKey: ["staff-resource-detail", ...identity, id],
    queryFn: () =>
      apiGet<{ items: { id: string }[] }>(
        `/api/admin/resource-instances?${new URLSearchParams({ resource_id: id })}`,
      ),
    enabled: false,
  });
  const missing = detail.isSuccess && detail.data.items.length === 0;
  return (
    <div className="resource-monitor-links">
      <button
        className="resource-status-link"
        data-action="history-resource-detail"
        onClick={() => selectResource(id)}
      >
        {t(...translatedMessage("view_resource_a4db550ee1"))}
      </button>
      <Link
        className="resource-status-link"
        to={`/admin?${new URLSearchParams({ tab: "calls", resource_id: id })}`}
      >
        {t(...translatedMessage("related_calls_fc23d01e81"))}
      </Link>
      {missing && (
        <span className="resource-monitor-missing">
          {t(
            ...translatedMessage(
              "resource_is_no_longer_in_the_current_snapshot_cfb013249e",
            ),
          )}
        </span>
      )}
    </div>
  );
}
