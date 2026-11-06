import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { useResourceCatalog, useResourceMutation } from "../lib/api/resource";
import {
  ROLE_ADMIN,
  ROLE_ROOT,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import {
  parseResourceSnapshot,
  resourceImportDiff,
} from "../lib/resource-catalog";
import type { ResourceSnapshot } from "../config/models";
import { useSettingsText } from "./settings-text";
import "./resource-inventory.css";
import { ResourcePublication } from "./resource-publication";

type Draft =
  | {
      kind: "import";
      revision: number;
      json: string;
      snapshot?: ResourceSnapshot;
    }
  | {
      kind: "placement";
      revision: number;
      id: string;
      model: string;
      groups: string;
      enabled: boolean;
    };

export function ResourceInventory() {
  const identity = useSessionQueryIdentity();
  return <ResourceInventoryView key={JSON.stringify(identity)} />;
}

function ResourceInventoryView() {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role);
  const query = useResourceCatalog();
  const mutation = useResourceMutation();
  const [search, setSearch] = useState("");
  const [channel, setChannel] = useState("");
  const [region, setRegion] = useState("");
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(0);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [review, setReview] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  if (role !== ROLE_ADMIN && role !== ROLE_ROOT) return null;
  const data = query.error ? undefined : query.data;
  const snapshot = data?.snapshot;
  const rows = (snapshot?.instances ?? []).filter(
    (row) =>
      (!channel || row.channelId === channel) &&
      (!region || JSON.stringify(row.region) === region) &&
      (!status || row.status === status) &&
      [
        row.id,
        row.upstreamModel,
        row.canonicalModel,
        data?.placements[row.id]?.modelId ?? "",
      ].some((value) =>
        value.toLowerCase().includes(search.trim().toLowerCase()),
      ),
  );
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const currentPage = Math.min(page, pages - 1);
  const conflict = Boolean(draft && data && draft.revision !== data.revision);
  const diff =
    draft?.kind === "import" && draft.snapshot
      ? resourceImportDiff(snapshot ?? null, draft.snapshot)
      : null;
  const unknown = text(...translatedMessage("unknown_region_e682d37883"));
  const statusLabel = (value: string) =>
    value === "available"
      ? text(...translatedMessage("available_d26b22e8da"))
      : value === "unavailable"
        ? text(...translatedMessage("unavailable_0b91d6e139"))
        : text(...translatedMessage("retired_75ce51762f"));
  const begin = (next: Draft) => {
    mutation.reset();
    setDraft(next);
    setReview(false);
    setError("");
    setNotice("");
  };
  const close = () => {
    if (!mutation.isPending) {
      setDraft(null);
      setError("");
      mutation.reset();
    }
  };
  const preview = () => {
    if (!draft || !data) return;
    try {
      if (draft.kind === "import") {
        const next = parseResourceSnapshot(JSON.parse(draft.json));
        if (next.sequence <= (snapshot?.sequence ?? 0))
          throw new Error("sequence");
        setDraft({ ...draft, snapshot: next });
      } else {
        if (
          draft.enabled &&
          (!draft.model.trim() ||
            !draft.groups.split(",").some((group) => group.trim()))
        )
          throw new Error("placement");
      }
      setError("");
      setReview(true);
    } catch {
      setError(
        text(
          ...translatedMessage(
            "check_fields_snapshot_sequence_must_increase__9bf6428d9d",
          ),
        ),
      );
    }
  };
  const save = async () => {
    if (!draft || !review || conflict || !data) return;
    try {
      const result = await mutation.mutateAsync(
        draft.kind === "import"
          ? { expected_revision: draft.revision, snapshot: draft.snapshot! }
          : {
              expected_revision: draft.revision,
              id: draft.id,
              placement: {
                modelId: draft.model.trim() || null,
                groupIds: draft.groups
                  .split(",")
                  .map((group) => group.trim())
                  .filter(Boolean),
                enabled: draft.enabled,
              },
            },
      );
      setNotice(
        text(
          ...translatedMessage("inventory_saved_as_v_ac7fd69268", {
            p0: result.revision,
          }),
        ),
      );
      setDraft(null);
      setError("");
    } catch {
      setError(
        text(
          ...translatedMessage(
            "save_outcome_is_unconfirmed_refresh_to_check__02941a883e",
          ),
        ),
      );
    }
  };
  return (
    <div className="resource-inventory">
      <header>
        <div>
          <h1>
            {text(
              ...translatedMessage(
                "channel_inventory_and_placements_24502175ca",
              ),
            )}
          </h1>
          <p>
            {text(
              ...translatedMessage(
                "store_supplier_inventory_regions_and_placemen_f67910aa4e",
              ),
            )}
          </p>
        </div>
        <div className="ri-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text(...translatedMessage("refresh_9c83c3600a"))}
          </Button>
          <Button
            disabled={!data}
            onClick={() =>
              data &&
              begin({
                kind: "import",
                revision: data.revision,
                json: JSON.stringify(
                  snapshot
                    ? {
                        ...snapshot,
                        sequence: snapshot.sequence + 1,
                        publishedAt: new Date().toISOString(),
                      }
                    : {
                        id: "inventory-1",
                        sequence: 1,
                        publishedAt: new Date().toISOString(),
                        source: "",
                        channels: [],
                        instances: [],
                      },
                  null,
                  2,
                ),
              })
            }
          >
            {text(...translatedMessage("import_snapshot_9fbcc55332"))}
          </Button>
          <Button asChild>
            <a href="#gateway-publication">
              {text(
                ...translatedMessage("gateway_publication_preview_86ab27029b"),
              )}
            </a>
          </Button>
          <Button asChild>
            <Link to="/admin?tab=resources#regions">
              {text(...translatedMessage("region_directory_8a612c18f4"))}
            </Link>
          </Button>
        </div>
      </header>
      <aside className="ri-boundary">
        {data?.affects_gateway_routing
          ? text(
              ...translatedMessage(
                "this_inventory_has_managed_gateway_channels_p_4c57025dd3",
              ),
            )
          : text(
              ...translatedMessage(
                "this_inventory_does_not_yet_change_gateway_ro_75817ecefa",
              ),
            )}{" "}
        <Link to="/admin?tab=resources">
          {text(...translatedMessage("open_live_channels_337c9e60b0"))}
        </Link>
      </aside>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_inventory_44cc0550f1"))}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "server_inventory_is_unavailable_refresh_to_re_59a49a4611",
            ),
          )}
        </p>
      )}
      {notice && (
        <p role="status" className="ri-notice">
          {notice}
        </p>
      )}
      {data && (
        <>
          <p className="ri-summary">
            <strong>v{data.revision}</strong> ·{" "}
            {snapshot?.id ??
              text(
                ...translatedMessage("no_snapshot_imported_b5dfbbb8db"),
              )}{" "}
            · {snapshot?.instances.length ?? 0}{" "}
            {text(...translatedMessage("resources_a6c40e4594"))}
          </p>
          {!snapshot ? (
            <p className="ri-empty">
              {text(
                ...translatedMessage(
                  "the_server_has_no_inventory_snapshot_import_y_a9b6d1d99c",
                ),
              )}
            </p>
          ) : (
            <>
              <p className="ri-source">
                {text(...translatedMessage("source_7986b56c96"))}:{" "}
                {snapshot.source || "—"} ·{" "}
                {text(...translatedMessage("source_publication_5f70b4c1ad"))}:{" "}
                <time dateTime={snapshot.publishedAt}>
                  {snapshot.publishedAt}
                </time>
              </p>
              <div className="ri-filters">
                <label>
                  {text(
                    ...translatedMessage("search_model_or_resource_fd5ae6d66c"),
                  )}
                  <input
                    value={search}
                    onChange={(e) => {
                      setSearch(e.target.value);
                      setPage(0);
                    }}
                  />
                </label>
                <label>
                  {text(...translatedMessage("channel_1d199d8e98"))}
                  <select
                    aria-label={text(
                      ...translatedMessage("channel_1d199d8e98"),
                    )}
                    value={channel}
                    onChange={(e) => {
                      setChannel(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">
                      {text(...translatedMessage("all_channels_4c3a0b8907"))}
                    </option>
                    {snapshot.channels.map((row) => (
                      <option key={row.id} value={row.id}>
                        {row.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {text(...translatedMessage("region_288ebd1a0e"))}
                  <select
                    aria-label={text(...translatedMessage("region_288ebd1a0e"))}
                    value={region}
                    onChange={(e) => {
                      setRegion(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">
                      {text(...translatedMessage("all_regions_b23e02a5a7"))}
                    </option>
                    {[...new Set(snapshot.instances.map((row) => row.region))]
                      .sort()
                      .map((value) => (
                        <option key={value} value={JSON.stringify(value)}>
                          {data.regions[value]?.name ?? (value || unknown)}
                        </option>
                      ))}
                  </select>
                </label>
                <label>
                  {text(...translatedMessage("source_state_2c32c66bce"))}
                  <select
                    aria-label={text(
                      ...translatedMessage("source_state_2c32c66bce"),
                    )}
                    value={status}
                    onChange={(e) => {
                      setStatus(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">
                      {text(...translatedMessage("all_states_4e405d0900"))}
                    </option>
                    {["available", "unavailable", "retired"].map((value) => (
                      <option key={value} value={value}>
                        {statusLabel(value)}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <div
                className="ri-table"
                role="region"
                aria-label={text(
                  ...translatedMessage("server_inventory_1c50c7fcd1"),
                )}
                tabIndex={0}
              >
                <table>
                  <thead>
                    <tr>
                      {[
                        text(
                          ...translatedMessage(
                            "resource_upstream_model_32a17c0242",
                          ),
                        ),
                        text(
                          ...translatedMessage(
                            "region_source_state_beed1dabd3",
                          ),
                        ),
                        text(
                          ...translatedMessage("stored_placement_1f413af87e"),
                        ),
                        text(...translatedMessage("actions_d83f4ed14a")),
                      ].map((label) => (
                        <th key={label}>{label}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rows
                      .slice(currentPage * 20, currentPage * 20 + 20)
                      .map((row) => {
                        const placement = data.placements[row.id];
                        return (
                          <tr key={row.id} data-resource-id={row.id}>
                            <td>
                              <strong>{row.id}</strong>
                              <code>{row.upstreamModel}</code>
                              <small>
                                {row.protocols.join(" / ")} ·{" "}
                                {row.contextLength.toLocaleString()}
                              </small>
                            </td>
                            <td>
                              <span>
                                {data.regions[row.region]?.name ??
                                  (row.region || unknown)}
                              </span>
                              <span>{statusLabel(row.status)}</span>
                              <small>
                                {row.latencyMs} ms ·{" "}
                                {text(
                                  ...translatedMessage(
                                    "source_value_28f99d7a83",
                                  ),
                                )}
                              </small>
                            </td>
                            <td>
                              <span>
                                {placement?.modelId ??
                                  text(
                                    ...translatedMessage("unmapped_6d198b7550"),
                                  )}
                              </span>
                              <small>
                                {placement?.groupIds.join(", ") || "—"}
                              </small>
                              <small>
                                {placement?.enabled
                                  ? text(
                                      ...translatedMessage(
                                        "inventory_placement_enabled_2f6535a3fe",
                                      ),
                                    )
                                  : text(
                                      ...translatedMessage(
                                        "inventory_placement_disabled_b469fd0882",
                                      ),
                                    )}
                              </small>
                            </td>
                            <td>
                              <Button
                                onClick={() =>
                                  begin({
                                    kind: "placement",
                                    revision: data.revision,
                                    id: row.id,
                                    model: placement?.modelId ?? "",
                                    groups:
                                      placement?.groupIds.join(", ") ?? "",
                                    enabled: placement?.enabled ?? false,
                                  })
                                }
                              >
                                {text(
                                  ...translatedMessage(
                                    "edit_placement_ffca10c028",
                                  ),
                                )}
                              </Button>
                            </td>
                          </tr>
                        );
                      })}
                  </tbody>
                </table>
              </div>
              {!rows.length && (
                <p className="ri-empty">
                  {text(
                    ...translatedMessage("no_matching_resources_941a8e0fb3"),
                  )}
                </p>
              )}
              <nav
                className="ri-pagination"
                aria-label={text(
                  ...translatedMessage("inventory_pagination_6c84359706"),
                )}
              >
                <Button
                  disabled={currentPage === 0}
                  onClick={() => setPage(currentPage - 1)}
                >
                  {text(...translatedMessage("previous_2672646f39"))}
                </Button>
                <span>
                  {currentPage + 1} / {pages} · {rows.length}
                </span>
                <Button
                  disabled={currentPage >= pages - 1}
                  onClick={() => setPage(currentPage + 1)}
                >
                  {text(...translatedMessage("next_c0fd39244f"))}
                </Button>
              </nav>
            </>
          )}
        </>
      )}
      <ResourcePublication
        catalog={data}
        refreshCatalog={() => query.refetch()}
      />
      <Modal
        open={Boolean(draft)}
        onOpenChange={(open) => !open && close()}
        preventOutsideClose
        size="wide"
        title={
          review
            ? text(...translatedMessage("confirm_inventory_save_1f7d7b761f"))
            : draft?.kind === "placement"
              ? text(
                  ...translatedMessage("edit_inventory_placement_09ffb0ccee"),
                )
              : text(
                  ...translatedMessage("import_inventory_snapshot_01201cb36a"),
                )
        }
        description={
          data?.affects_gateway_routing
            ? text(
                ...translatedMessage(
                  "saving_resource_or_placement_changes_may_revo_347e3d6cca",
                ),
              )
            : text(
                ...translatedMessage(
                  "save_to_server_inventory_live_gateway_routing_e9d9d1ab16",
                ),
              )
        }
        footer={
          <>
            <Button disabled={mutation.isPending} onClick={close}>
              {text(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            {review && (
              <Button
                disabled={mutation.isPending}
                onClick={() => setReview(false)}
              >
                {text(...translatedMessage("back_to_editing_85b057f793"))}
              </Button>
            )}
            <Button
              disabled={mutation.isPending || conflict || !data}
              onClick={() => (review ? void save() : preview())}
            >
              {mutation.isPending
                ? text(...translatedMessage("saving_b89854a86e"))
                : review
                  ? text(...translatedMessage("confirm_save_f90ef9ab0e"))
                  : text(...translatedMessage("review_changes_0594a17c6d"))}
            </Button>
          </>
        }
      >
        <div className="ri-editor">
          {draft && (
            <p>
              {text(...translatedMessage("base_revision_40fdfc70dc"))} v
              {draft.revision}
            </p>
          )}
          {conflict && (
            <p role="alert">
              {text(
                ...translatedMessage(
                  "the_server_revision_changed_cancel_and_reopen_0e6c67d1e2",
                ),
              )}
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          {draft?.kind === "import" &&
            (!review ? (
              <label>
                JSON
                <textarea
                  dir="ltr"
                  aria-label="Snapshot JSON"
                  value={draft.json}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      json: e.target.value,
                      snapshot: undefined,
                    })
                  }
                />
              </label>
            ) : (
              <>
                <p>
                  {draft.snapshot?.id} ·{" "}
                  {text(...translatedMessage("source_sequence_fb090bde31"))}{" "}
                  {draft.snapshot?.sequence}
                </p>
                <p>
                  {text(
                    ...translatedMessage("added_changed_removed_a7ad9e8c66", {
                      p0: diff?.added,
                      p1: diff?.changed,
                      p2: diff?.removed,
                    }),
                  )}
                </p>
                <p>
                  {text(
                    ...translatedMessage(
                      "new_resources_are_unplaced_channel_model_iden_3f86144634",
                    ),
                  )}
                </p>
              </>
            ))}
          {draft?.kind === "placement" && (
            <>
              <p>
                <code>{draft.id}</code>
              </p>
              {review ? (
                <dl>
                  <dt>
                    {text(...translatedMessage("standard_model_18b603bd24"))}
                  </dt>
                  <dd>{draft.model.trim() || "—"}</dd>
                  <dt>
                    {text(...translatedMessage("group_identifiers_9e0a470944"))}
                  </dt>
                  <dd>{draft.groups || "—"}</dd>
                  <dt>
                    {text(...translatedMessage("inventory_state_9fa79f44c1"))}
                  </dt>
                  <dd>
                    {draft.enabled
                      ? text(...translatedMessage("enabled_d4fda6d86a"))
                      : text(...translatedMessage("disabled_ab66c5caec"))}
                  </dd>
                </dl>
              ) : (
                <>
                  <label>
                    {text(
                      ...translatedMessage(
                        "standard_model_identifier_453d74296f",
                      ),
                    )}
                    <input
                      value={draft.model}
                      onChange={(e) =>
                        setDraft({ ...draft, model: e.target.value })
                      }
                    />
                  </label>
                  <label>
                    {text(
                      ...translatedMessage(
                        "group_identifiers_comma_separated_ef11199aba",
                      ),
                    )}
                    <input
                      dir="ltr"
                      value={draft.groups}
                      onChange={(e) =>
                        setDraft({ ...draft, groups: e.target.value })
                      }
                    />
                  </label>
                  <label className="ri-checkbox">
                    <input
                      type="checkbox"
                      checked={draft.enabled}
                      onChange={(e) =>
                        setDraft({ ...draft, enabled: e.target.checked })
                      }
                    />
                    {text(
                      ...translatedMessage(
                        "enable_inventory_placement_26a3d2514e",
                      ),
                    )}
                  </label>
                </>
              )}
            </>
          )}
        </div>
      </Modal>
    </div>
  );
}
