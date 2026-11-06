import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Search } from "lucide-react";
import {
  ResourceMonitorPanels,
  type SnapshotStatus,
} from "./resource-monitor-panels";
import { apiGet, apiPost } from "../lib/api/http";
import { useStaffText } from "../lib/api/staff";
import { useSessionQueryIdentity } from "../lib/system-access";
import { Button } from "../components/ui/button";
import { Drawer, Modal } from "../components/ui/overlay";
import "./admin-resource-status.css";

type SyncStatus = {
  configured: boolean;
  status: string;
  mode: string;
  applied_version: number;
  remote_version: number | null;
  version_gap: number | null;
  checksum: string;
  sequence: number;
  snapshot?: SnapshotStatus;
  last_aligned_at: number;
  last_attempt_at: number;
  observed_at: number;
  failed_cycles: number;
  failures: { id: number; at: number; code: string }[];
};
type ResourceStatus = {
  id: string;
  vendor: string;
  canonicalModel: string;
  upstreamModel?: string;
  channelId?: string;
  protocols?: string[];
  contextLength?: number;
  region?: string;
  capabilities: string[];
  price_tier?: number;
  quality_level?: string;
  state?: string;
  status: string;
  trend?: string;
  score?: number;
  tiers: number[];
  calls_5m: number;
  success_rate: number | null;
  gateways: {
    gateway_id: string;
    state: string;
    last_seen: number;
    consecutive_failures: number;
  }[];
};
type ResourcePage = {
  items: ResourceStatus[];
  total: number;
  vendors: string[];
  models: string[];
  observed_at: number;
};
const gatewayPriority: Record<string, number> = {
  open: 0,
  degraded: 1,
  unknown: 2,
  normal: 3,
};

export function AdminResourceStatus() {
  const t = useStaffText(),
    identity = useSessionQueryIdentity();
  const queryClient = useQueryClient();
  const [confirm, setConfirm] = useState(false),
    [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [filters, setFilters] = useState({
    q: "",
    model: "",
    vendor: "",
    status: "",
    tier: "",
  });
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setFilters((previous) => ({ ...previous, q: search.trim() }));
      setPage(1);
    }, 250);
    return () => window.clearTimeout(timer);
  }, [search]);
  const sync = useQuery({
    queryKey: ["staff-sync", ...identity],
    queryFn: async () => {
      const value = await apiGet<SyncStatus>("/api/admin/resource-sync");
      if (
        !value ||
        !Array.isArray(value.failures) ||
        typeof value.status !== "string"
      ) {
        throw new Error("Resource monitoring response unavailable");
      }
      return value;
    },
    refetchInterval: 5000,
  });
  const params = new URLSearchParams({ ...filters, page: String(page) });
  const resources = useQuery({
    queryKey: ["staff-resource-status", ...identity, params.toString()],
    queryFn: () =>
      apiGet<ResourcePage>(`/api/admin/resource-instances?${params}`),
    refetchInterval: 5000,
  });
  const detail = useQuery({
    queryKey: ["staff-resource-detail", ...identity, selected],
    queryFn: () =>
      apiGet<ResourcePage>(
        `/api/admin/resource-instances?${new URLSearchParams({ resource_id: selected! })}`,
      ),
    enabled: selected !== null,
    refetchInterval: 5000,
  });
  const mutation = useMutation({
    mutationFn: () =>
      apiPost<SyncStatus>("/api/admin/resource-sync", { confirm: true }),
    onSuccess: async () => {
      setConfirm(false);
      await Promise.all([
        sync.refetch(),
        resources.refetch(),
        queryClient.invalidateQueries({
          queryKey: ["staff-resource-monitoring", ...identity],
        }),
      ]);
    },
  });
  const data = sync.isError ? undefined : sync.data,
    rows = resources.isError ? undefined : resources.data;
  const selectedResource = detail.isError ? undefined : detail.data?.items[0];
  const locale = document.documentElement.lang || "en";
  const number = (value: number) => new Intl.NumberFormat(locale).format(value);
  const success = (resource: ResourceStatus) =>
    resource.calls_5m === 0 || resource.success_rate === null
      ? "—"
      : new Intl.NumberFormat(locale, {
          style: "percent",
          minimumFractionDigits: 2,
          maximumFractionDigits: 2,
        }).format(resource.success_rate);
  const callsLink = (id: string) =>
    `/admin?${new URLSearchParams({ tab: "calls", resource_id: id })}`;
  const date = (value?: number) =>
    value
      ? new Intl.DateTimeFormat(document.documentElement.lang || "en", {
          dateStyle: "short",
          timeStyle: "medium",
          timeZone: "UTC",
        }).format(new Date(value * 1000))
      : "—";
  const labels: Record<string, string> = {
    aligned: t(...translatedMessage("aligned_34081ba30a")),
    alert: t(...translatedMessage("synchronization_alert_3ca8b38159")),
    retrying: t(...translatedMessage("retry_pending_cdaf0daeb3")),
    syncing: t(...translatedMessage("syncing_6eef08df6a")),
    awaiting_sync: t(
      ...translatedMessage("awaiting_first_alignment_1f2f80d241"),
    ),
    not_configured: t(...translatedMessage("source_not_configured_bb0e45a4fd")),
    normal: t(...translatedMessage("normal_bdde31bb16")),
    open: t(...translatedMessage("circuit_open_3193ca9bb8")),
    degraded: t(...translatedMessage("deprioritized_01a56dc787")),
    unknown: t(...translatedMessage("unknown_3eb118e4ec")),
    hot: t(...translatedMessage("hot_ff01b75c6d")),
    cold: t(...translatedMessage("cold_61ab0b1e50")),
    bad: t(...translatedMessage("bad_97ef14e567")),
    disabled: t(...translatedMessage("disabled_7233008bdb")),
    available: t(...translatedMessage("available_d26b22e8da")),
    unavailable: t(...translatedMessage("unavailable_0b91d6e139")),
    retired: t(...translatedMessage("retired_bd6ea0fdcd")),
    stable: t(...translatedMessage("stable_9b4af0ebaa")),
    abnormal: t(...translatedMessage("abnormal_gateways_dbc1cf436c")),
    text: t(...translatedMessage("text_0c246fb7e5")),
    code: t(...translatedMessage("code_08cde54c7e")),
    image: t(...translatedMessage("image_beebfb3fcd")),
    audio: t(...translatedMessage("audio_c62be0ebb2")),
    video: t(...translatedMessage("video_e2fb3c780b")),
    embedding: t(...translatedMessage("embedding_379afcef40")),
  };
  const syncDisabled =
    !data?.configured || mutation.isPending || data.status === "syncing";
  const requestFullSync = () => {
    mutation.reset();
    setConfirm(true);
  };
  const error = t(
    ...translatedMessage("unable_to_load_current_status_retry_d200806886"),
  );
  return (
    <div className="resource-status-page">
      <header className="resource-status-heading">
        <div>
          <h1>
            {t(...translatedMessage("sync_and_routing_status_b65492fd1f"))}
          </h1>
          <p>
            {t(
              ...translatedMessage(
                "resource_data_is_read_only_make_changes_on_th_e37ac0c337",
              ),
            )}
          </p>
        </div>
        <div className="resource-status-actions">
          <Button
            variant="secondary"
            disabled={sync.isFetching || resources.isFetching}
            onClick={() =>
              void Promise.all([
                sync.refetch(),
                resources.refetch(),
                queryClient.invalidateQueries({
                  queryKey: ["staff-resource-monitoring", ...identity],
                }),
              ])
            }
          >
            {t(...translatedMessage("refresh_9c83c3600a"))}
          </Button>
          <Button
            data-testid="force-resource-sync"
            disabled={syncDisabled}
            onClick={requestFullSync}
          >
            {t(...translatedMessage("force_full_sync_fafc970d6d"))}
          </Button>
        </div>
      </header>
      <section
        className="resource-status-panel"
        aria-label={t(
          ...translatedMessage("synchronization_status_c07f61e6db"),
        )}
      >
        {sync.isPending && (
          <p role="status">{t(...translatedMessage("loading_4c20385375"))}</p>
        )}
        {sync.isError && <p role="alert">{error}</p>}
        {data && (
          <>
            <div className="resource-status-heading">
              <h2>
                {t(...translatedMessage("resource_synchronization_76e5fe1666"))}
              </h2>
              <span
                className="resource-status-badge"
                data-state={data.status}
                role="status"
              >
                {labels[data.status] ?? labels.unknown}
              </span>
            </div>
            <dl className="resource-status-summary">
              <div>
                <dt>{t(...translatedMessage("applied_version_35d9f65658"))}</dt>
                <dd>{data.applied_version || "—"}</dd>
              </div>
              <div>
                <dt>
                  {t(
                    ...translatedMessage(
                      "last_observed_upstream_version_4880d19f33",
                    ),
                  )}
                </dt>
                <dd>{data.remote_version ?? "—"}</dd>
              </div>
              <div>
                <dt>{t(...translatedMessage("version_gap_501e891281"))}</dt>
                <dd>{data.version_gap ?? "—"}</dd>
              </div>
              <div>
                <dt>{t(...translatedMessage("failed_cycles_f751ba2820"))}</dt>
                <dd>{data.failed_cycles}</dd>
              </div>
              <div>
                <dt>
                  {t(...translatedMessage("last_alignment_utc_4dfb9bc6a0"))}
                </dt>
                <dd>{date(data.last_aligned_at)}</dd>
              </div>
              <div>
                <dt>
                  {t(...translatedMessage("last_attempt_utc_5472ee2d67"))}
                </dt>
                <dd>{date(data.last_attempt_at)}</dd>
              </div>
            </dl>
            <div className="resource-status-checksum">
              <span>
                {t(...translatedMessage("catalog_checksum_19b83181a5"))}
              </span>
              <code dir="ltr">{data.checksum || "—"}</code>
            </div>
            <p>
              {t(
                ...translatedMessage(
                  "automatic_alignment_every_5_minutes_two_misse_1ac2713245",
                ),
              )}
            </p>
            {data.mode === "snapshot_compatibility" && (
              <p className="resource-status-note">
                {t(
                  ...translatedMessage(
                    "this_source_provides_a_snapshot_sequence_an_i_f07bc7dd7b",
                  ),
                )}
              </p>
            )}
            {data.status === "alert" && (
              <p className="resource-status-danger" role="alert">
                {t(
                  ...translatedMessage(
                    "synchronization_is_out_of_date_the_last_valid_9d7f4e3ba1",
                  ),
                )}
              </p>
            )}
          </>
        )}
        {mutation.isSuccess && !confirm && data?.status === "aligned" && (
          <p role="status">
            {t(
              ...translatedMessage(
                "full_synchronization_completed_versions_are_a_5cc7cf2faf",
              ),
            )}
          </p>
        )}
      </section>
      <ResourceMonitorPanels
        snapshot={data?.snapshot}
        receivedAt={sync.dataUpdatedAt}
        failed={sync.isError}
        pending={sync.isPending}
        selectResource={setSelected}
        syncStatus={data}
        syncDisabled={syncDisabled}
        onForceSync={requestFullSync}
      >
        <section className="resource-status-panel resource-status-inventory">
          <header className="resource-status-heading">
            <div>
              <h2>
                {t(...translatedMessage("resource_instance_status_793824758c"))}
              </h2>
              <p>
                {t(
                  ...translatedMessage(
                    "local_gateway_state_circuit_open_deprioritize_55fb0c54e3",
                  ),
                )}
              </p>
            </div>
            <label className="resource-status-search">
              <Search size={16} aria-hidden="true" />
              <input
                type="search"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                aria-label={t(
                  ...translatedMessage(
                    "search_resource_id_or_model_ef414805a6",
                  ),
                )}
                placeholder={t(
                  ...translatedMessage("resource_id_model_e701c724ee"),
                )}
              />
            </label>
          </header>
          <div className="resource-status-filters">
            {(["model", "vendor", "status", "tier"] as const).map((key) => (
              <label key={key}>
                {
                  {
                    model: t(...translatedMessage("model_9c2e5b5065")),
                    vendor: t(...translatedMessage("vendor_a4625f4b1f")),
                    status: t(...translatedMessage("state_c67e5a7ccf")),
                    tier: t(
                      ...translatedMessage("serviceable_tier_31d2874eeb"),
                    ),
                  }[key]
                }
                <select
                  data-filter={key}
                  value={filters[key]}
                  onChange={(e) => {
                    setFilters((previous) => ({
                      ...previous,
                      [key]: e.target.value,
                    }));
                    setPage(1);
                  }}
                >
                  <option value="">
                    {t(...translatedMessage("all_ac49939df0"))}
                  </option>
                  {(key === "model"
                    ? (rows?.models ?? [])
                    : key === "vendor"
                      ? (rows?.vendors ?? [])
                      : key === "tier"
                        ? ["4", "6", "8", "10"]
                        : [
                            "abnormal",
                            "open",
                            "degraded",
                            "unknown",
                            "hot",
                            "cold",
                            "bad",
                            "disabled",
                            "available",
                            "unavailable",
                            "retired",
                          ]
                  ).map((v) => (
                    <option key={v} value={v}>
                      {labels[v] ?? v}
                    </option>
                  ))}
                </select>
              </label>
            ))}
          </div>
          {resources.isPending && (
            <p role="status">{t(...translatedMessage("loading_4c20385375"))}</p>
          )}
          {resources.isError && <p role="alert">{error}</p>}
          {rows && (
            <>
              <div
                className="resource-status-table"
                role="region"
                aria-label={t(
                  ...translatedMessage(
                    "resource_status_table_horizontally_scrollable_f7dfdda12f",
                  ),
                )}
                tabIndex={0}
              >
                <table>
                  <thead>
                    <tr>
                      {[
                        t(...translatedMessage("resource_id_5338f8bf31")),
                        t(
                          ...translatedMessage(
                            "model_vendor_capability_2cfdb2c7f0",
                          ),
                        ),
                        t(...translatedMessage("tier_p_quality_q_6ea1642d3e")),
                        t(...translatedMessage("lifecycle_d2d13fde51")),
                        t(...translatedMessage("trend_3f835d14c8")),
                        t(...translatedMessage("routing_score_a9f0e6a00f")),
                        t(...translatedMessage("5_minute_attempts_c56f6fadc4")),
                        t(...translatedMessage("success_rate_b8e8689749")),
                        t(
                          ...translatedMessage(
                            "local_gateway_state_f97b66ae94",
                          ),
                        ),
                        t(...translatedMessage("actions_d83f4ed14a")),
                      ].map((h, index) => (
                        <th
                          key={h}
                          scope="col"
                          className={
                            index >= 5 && index <= 7
                              ? "resource-status-numeric"
                              : undefined
                          }
                        >
                          {h}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rows.items.map((r) => (
                      <tr key={r.id}>
                        <td>
                          <button
                            className="resource-status-link resource-status-id"
                            onClick={() => setSelected(r.id)}
                            title={r.id}
                          >
                            <code dir="ltr">{r.id}</code>
                          </button>
                        </td>
                        <td>
                          <span
                            className="resource-status-model"
                            title={r.canonicalModel}
                          >
                            <bdi>{r.canonicalModel || "—"}</bdi>
                          </span>
                          <small className="resource-status-meta">
                            <bdi>{r.vendor || "—"}</bdi> ·{" "}
                            {r.capabilities
                              .map((c) => labels[c] ?? c)
                              .join(" / ") || "—"}
                          </small>
                        </td>
                        <td>
                          <bdi className="resource-status-tier">
                            {r.price_tier === undefined
                              ? "—"
                              : `P${r.price_tier}`}{" "}
                            · {r.quality_level || "—"}
                          </bdi>
                        </td>
                        <td>
                          <span
                            className="resource-status-badge"
                            data-state={r.state}
                          >
                            {labels[r.state || ""] ?? "—"}
                          </span>
                        </td>
                        <td>
                          <span
                            className="resource-status-trend"
                            data-trend={r.trend}
                          >
                            {r.trend === "degraded"
                              ? t(...translatedMessage("declining_c22ff97529"))
                              : (labels[r.trend || ""] ?? "—")}
                          </span>
                        </td>
                        <td className="resource-status-numeric">
                          {r.score === undefined ? "—" : number(r.score)}
                        </td>
                        <td className="resource-status-numeric resource-status-count">
                          {number(r.calls_5m)}
                        </td>
                        <td className="resource-status-numeric">
                          {success(r)}
                        </td>
                        <td>
                          <div className="resource-status-gateways">
                            {[...r.gateways]
                              .sort(
                                (a, b) =>
                                  (gatewayPriority[a.state] ?? 2) -
                                  (gatewayPriority[b.state] ?? 2),
                              )
                              .slice(0, 3)
                              .map((g) => (
                                <button
                                  className="resource-status-badge resource-status-gateway"
                                  key={g.gateway_id}
                                  data-state={g.state}
                                  onClick={() => setSelected(r.id)}
                                  title={`${g.gateway_id} · ${labels[g.state] ?? labels.unknown} · ${date(g.last_seen)} UTC`}
                                >
                                  <code dir="ltr">{g.gateway_id}</code>
                                  <span>
                                    {g.state === "unknown"
                                      ? t(
                                          ...translatedMessage(
                                            "unknown_a3cf284f50",
                                          ),
                                        )
                                      : (labels[g.state] ?? labels.unknown)}
                                  </span>
                                </button>
                              ))}
                            {r.gateways.length > 3 && (
                              <button
                                className="resource-status-link"
                                onClick={() => setSelected(r.id)}
                                aria-label={t(
                                  ...translatedMessage(
                                    "view_all_gateways_5d72240ea1",
                                    { p0: r.gateways.length },
                                  ),
                                )}
                              >
                                +{r.gateways.length - 3}
                              </button>
                            )}
                            {!r.gateways.length && (
                              <span
                                className="resource-status-badge"
                                data-state="unknown"
                              >
                                {labels.unknown}
                              </span>
                            )}
                          </div>
                        </td>
                        <td>
                          <div className="resource-status-row-actions">
                            <Button
                              size="small"
                              onClick={() => setSelected(r.id)}
                              data-action="resource-detail"
                            >
                              {t(
                                ...translatedMessage(
                                  "view_resource_a4db550ee1",
                                ),
                              )}
                            </Button>
                            <Link
                              className="resource-status-link"
                              to={callsLink(r.id)}
                            >
                              {t(
                                ...translatedMessage(
                                  "related_calls_fc23d01e81",
                                ),
                              )}
                            </Link>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {!rows.items.length && (
                <p>
                  {t(...translatedMessage("no_matching_resources_bab7f4f774"))}
                </p>
              )}
              <div className="resource-status-pagination">
                <span>
                  {t(...translatedMessage("total_301e42d384"))} {rows.total}
                </span>
                <Button
                  variant="secondary"
                  disabled={page === 1}
                  onClick={() => setPage(page - 1)}
                >
                  {t(...translatedMessage("previous_2672646f39"))}
                </Button>
                <span>
                  {page} / {Math.max(1, Math.ceil(rows.total / 20))}
                </span>
                <Button
                  variant="secondary"
                  disabled={page * 20 >= rows.total}
                  onClick={() => setPage(page + 1)}
                >
                  {t(...translatedMessage("next_c0fd39244f"))}
                </Button>
              </div>
              <small>
                {t(...translatedMessage("observed_at_utc_e65a39a4f0"))}:{" "}
                {date(rows.observed_at)}
              </small>
            </>
          )}
          <p className="resource-status-footnote">
            {t(
              ...translatedMessage(
                "last_5_minutes_of_platform_attempts_including_bd1c6462f9",
              ),
            )}
          </p>
        </section>
      </ResourceMonitorPanels>
      <Drawer
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
        title={t(...translatedMessage("resource_instance_details_f20bfcbb7d"))}
        description={t(
          ...translatedMessage(
            "resource_data_is_read_only_make_changes_on_th_b9bfd1b125",
          ),
        )}
        footer={
          selected && (
            <Button asChild>
              <Link to={callsLink(selected)}>
                {t(...translatedMessage("related_calls_fc23d01e81"))}
              </Link>
            </Button>
          )
        }
      >
        <div className="resource-status-detail">
          {detail.isPending && (
            <p role="status">{t(...translatedMessage("loading_4c20385375"))}</p>
          )}
          {detail.isError && <p role="alert">{error}</p>}
          {detail.isSuccess && !selectedResource && (
            <p role="status">
              {t(
                ...translatedMessage(
                  "resource_is_no_longer_in_the_current_snapshot_cfb013249e",
                ),
              )}
            </p>
          )}
          {selectedResource && (
            <>
              <dl>
                {[
                  [
                    t(...translatedMessage("resource_id_5338f8bf31")),
                    selectedResource.id,
                  ],
                  [
                    t(...translatedMessage("vendor_a4625f4b1f")),
                    selectedResource.vendor,
                  ],
                  [
                    t(...translatedMessage("canonical_model_dfe90bcc65")),
                    selectedResource.canonicalModel,
                  ],
                  [
                    t(...translatedMessage("upstream_model_e70b5369dc")),
                    selectedResource.upstreamModel,
                  ],
                  [
                    t(...translatedMessage("capabilities_23e0d2f417")),
                    selectedResource.capabilities
                      .map((v) => labels[v] ?? v)
                      .join(" / "),
                  ],
                  [
                    t(
                      ...translatedMessage("price_tier_p_quality_q_d436aa61b8"),
                    ),
                    `${selectedResource.price_tier ?? "—"} / ${selectedResource.quality_level || "—"}`,
                  ],
                  [
                    t(...translatedMessage("serviceable_tiers_0bea3c4168")),
                    selectedResource.tiers.join(" / "),
                  ],
                  [
                    t(...translatedMessage("source_availability_8992813ef8")),
                    labels[selectedResource.status] ?? selectedResource.status,
                  ],
                  [
                    t(...translatedMessage("lifecycle_d2d13fde51")),
                    labels[selectedResource.state || ""],
                  ],
                  [
                    t(...translatedMessage("trend_3f835d14c8")),
                    selectedResource.trend === "degraded"
                      ? t(...translatedMessage("declining_c22ff97529"))
                      : labels[selectedResource.trend || ""],
                  ],
                  [
                    t(...translatedMessage("routing_score_a9f0e6a00f")),
                    selectedResource.score === undefined
                      ? "—"
                      : number(selectedResource.score),
                  ],
                  [
                    t(
                      ...translatedMessage(
                        "5_minute_attempts_success_rate_f2a3f50a47",
                      ),
                    ),
                    `${number(selectedResource.calls_5m)} / ${success(selectedResource)}`,
                  ],
                ].map(([name, value]) => (
                  <div key={name}>
                    <dt>{name}</dt>
                    <dd>
                      <bdi>{value || "—"}</bdi>
                    </dd>
                  </div>
                ))}
              </dl>
              <h3>
                {t(...translatedMessage("local_state_per_gateway_21b66b391e"))}
              </h3>
              <ul className="resource-status-gateway-details">
                {selectedResource.gateways.map((g) => (
                  <li key={g.gateway_id}>
                    <div>
                      <code dir="ltr">{g.gateway_id}</code>
                      <span
                        className="resource-status-badge"
                        data-state={g.state}
                      >
                        {labels[g.state] ?? labels.unknown}
                      </span>
                    </div>
                    <p>
                      {t(...translatedMessage("last_heartbeat_utc_cde07fbf28"))}
                      : {date(g.last_seen)}
                    </p>
                    <p>
                      {t(
                        ...translatedMessage("consecutive_failures_5ec642844f"),
                      )}
                      :{" "}
                      {g.state === "unknown"
                        ? "—"
                        : number(g.consecutive_failures)}
                    </p>
                  </li>
                ))}
              </ul>
              {!selectedResource.gateways.length && <p>{labels.unknown}</p>}
              <small>
                {t(...translatedMessage("observed_at_utc_e65a39a4f0"))}:{" "}
                {date(detail.data?.observed_at)}
              </small>
            </>
          )}
        </div>
      </Drawer>
      <Modal
        open={confirm}
        onOpenChange={(open) => {
          if (!mutation.isPending) setConfirm(open);
        }}
        title={t(
          ...translatedMessage("confirm_full_synchronization_f20957fd07"),
        )}
        description={t(
          ...translatedMessage(
            "fetch_the_complete_catalog_and_snapshot_from__d52cc27c91",
          ),
        )}
      >
        {mutation.isError && (
          <p role="alert">
            {t(
              ...translatedMessage(
                "synchronization_did_not_complete_published_da_53144ab0cf",
              ),
            )}
          </p>
        )}
        <div className="resource-status-actions">
          <Button
            variant="secondary"
            disabled={mutation.isPending}
            onClick={() => setConfirm(false)}
          >
            {t(...translatedMessage("cancel_0828e4ce08"))}
          </Button>
          <Button
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending
              ? t(...translatedMessage("syncing_4c97cd0b2e"))
              : t(...translatedMessage("confirm_sync_870a69a6a3"))}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
