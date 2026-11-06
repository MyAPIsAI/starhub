import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
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
  children,
}: {
  snapshot?: SnapshotStatus;
  receivedAt: number;
  failed: boolean;
  pending: boolean;
  selectResource: (id: string) => void;
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
  const resource = (id: string) => (
    <button className="resource-status-link" onClick={() => selectResource(id)}>
      <code dir="ltr">{id}</code>
    </button>
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
      <section
        className="resource-status-panel resource-monitor-history"
        aria-labelledby="resource-mismatch-title"
      >
        <header className="resource-status-heading">
          <div>
            <h2 id="resource-mismatch-title">
              {t(...translatedMessage("price_tier_mismatch_alerts_13aafc19c7"))}
            </h2>
            <p>
              {t(
                ...translatedMessage(
                  "compare_snapshot_and_event_price_tiers_events_b05b142d76",
                ),
              )}
            </p>
          </div>
          <Button
            size="small"
            disabled={history.isFetching}
            onClick={() => void history.refetch()}
          >
            {t(...translatedMessage("refresh_history_e25eee27af"))}
          </Button>
        </header>
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
                  {date(feed.last_success_at)} UTC
                </p>
              ))}
            <div
              className="resource-monitor-table"
              role="region"
              tabIndex={0}
              aria-label={t(
                ...translatedMessage(
                  "price_mismatch_history_horizontally_scrollabl_0bed193285",
                ),
              )}
            >
              <table>
                <thead>
                  <tr>
                    {[
                      t(...translatedMessage("time_utc_f0c3e232cd")),
                      t(...translatedMessage("resource_1a3d8d4288")),
                      t(...translatedMessage("snapshot_seq_9eec99d0d4")),
                      t(...translatedMessage("snapshot_p_93e887b871")),
                      t(...translatedMessage("event_p_e8ab98d7ff")),
                      t(...translatedMessage("event_version_ed1542f446")),
                      t(...translatedMessage("status_dda39dcd63")),
                    ].map((label) => (
                      <th key={label} scope="col">
                        {label}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.mismatches.map((row) => (
                    <tr key={row.id}>
                      <td>{date(row.at)}</td>
                      <td>{resource(row.resource_id)}</td>
                      <td>{row.snapshot_sequence}</td>
                      <td>P{row.snapshot_price}</td>
                      <td>P{row.event_price}</td>
                      <td>{row.event_version}</td>
                      <td>
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
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!rows.mismatches.length && (
              <p>
                {t(
                  ...translatedMessage(
                    "no_recorded_price_mismatches_b710b9f493",
                  ),
                )}
              </p>
            )}
            {paginate(mismatchPage, rows.mismatch_total, setMismatchPage)}
          </>
        )}
      </section>
      <section
        className="resource-status-panel resource-monitor-history"
        aria-labelledby="resource-incident-title"
      >
        <h2 id="resource-incident-title">
          {t(...translatedMessage("incident_reports_a701f20f79"))}
        </h2>
        <p>
          {t(
            ...translatedMessage(
              "gateway_incidents_and_their_delivery_results__bfe652790b",
            ),
          )}
        </p>
        {history.isError && <p role="alert">{error}</p>}
        {history.isPending && (
          <p role="status">{t(...translatedMessage("loading_4c20385375"))}</p>
        )}
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
            <div
              className="resource-monitor-table"
              role="region"
              tabIndex={0}
              aria-label={t(
                ...translatedMessage(
                  "incident_history_horizontally_scrollable_2cb57114d9",
                ),
              )}
            >
              <table>
                <thead>
                  <tr>
                    {[
                      t(...translatedMessage("occurred_at_utc_ef0bf696db")),
                      t(...translatedMessage("event_type_8f67e63113")),
                      t(...translatedMessage("resource_1a3d8d4288")),
                      t(...translatedMessage("gateway_07318ec341")),
                      t(...translatedMessage("delivery_status_96d9f75827")),
                      t(...translatedMessage("reported_at_utc_44015dcecb")),
                    ].map((label) => (
                      <th key={label} scope="col">
                        {label}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.incidents.map((row) => (
                    <tr key={row.event_id}>
                      <td>{date(row.occurred_at)}</td>
                      <td>
                        <span>
                          {row.type === "half_open_failed"
                            ? t(
                                ...translatedMessage(
                                  "five_failed_half_open_probes_4d808ab8db",
                                ),
                              )
                            : t(
                                ...translatedMessage(
                                  "two_consecutive_ewma_breaches_e524b82efb",
                                ),
                              )}
                        </span>
                        <small>
                          <code>{row.type}</code>
                        </small>
                        <small title={row.event_id}>
                          {t(...translatedMessage("event_id_27eff7c057"))}:{" "}
                          <code>{row.event_id}</code>
                        </small>
                      </td>
                      <td>{resource(row.resource_id)}</td>
                      <td>
                        <code dir="ltr">{row.gateway_id}</code>
                      </td>
                      <td>
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
                          {statusText[row.delivery_status] ?? "—"}
                        </span>
                        <small>
                          {t(...translatedMessage("attempts_4321915685"))}:{" "}
                          {row.attempts}
                        </small>
                      </td>
                      <td>{date(row.delivered_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!rows.incidents.length && (
              <p>{t(...translatedMessage("no_incident_reports_3df6b6cb8a"))}</p>
            )}
            {paginate(incidentPage, rows.incident_total, setIncidentPage)}
          </>
        )}
      </section>
    </>
  );
}
