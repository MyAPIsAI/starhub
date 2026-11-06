import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import { Button } from "../components/ui/button";
import { useResourceBindingState } from "../lib/api/resource-publication";
import { useSettingsText } from "./settings-text";
import {
  prepareBindingChange,
  type BindingChangeReceipt,
  type BindingChangeReview,
} from "../lib/resource-binding-change";
import type { ResourceBindingStatus } from "../lib/resource-binding-state";
import { ResourceBindingChangeDialog } from "./resource-binding-change";

export function ResourceBindingList({ enabled }: { enabled: boolean }) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(1);
  const query = useResourceBindingState(enabled, page);
  const state = query.error ? undefined : query.data;
  const { hash } = useLocation();
  const section = useRef<HTMLElement>(null);
  useEffect(() => {
    if (hash !== "#gateway-bindings" || !enabled) return;
    const frame = requestAnimationFrame(() => {
      section.current?.scrollIntoView({ block: "start" });
      section.current?.focus({ preventScroll: true });
    });
    return () => cancelAnimationFrame(frame);
  }, [hash, enabled, query.isLoading]);
  const [review, setReview] = useState<BindingChangeReview>();
  const [receipt, setReceipt] = useState<BindingChangeReceipt>();
  const [actionError, setActionError] = useState("");
  const resultElement = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (receipt) resultElement.current?.focus();
  }, [receipt]);
  const changesEnabled = Boolean(
    state?.binding_changes_available === true &&
    !query.isFetching &&
    state.resource_revision < Number.MAX_SAFE_INTEGER &&
    state.channel_revision < Number.MAX_SAFE_INTEGER,
  );
  const choose = (
    row: ResourceBindingStatus,
    operation: BindingChangeReview["operation"],
  ) => {
    if (!state || !changesEnabled) return;
    try {
      const key = Array.from(
        crypto.getRandomValues(new Uint8Array(16)),
        (byte) => byte.toString(16).padStart(2, "0"),
      ).join("");
      setReview(prepareBindingChange(state, row, operation, key));
      setReceipt(undefined);
      setActionError("");
    } catch {
      setActionError(
        text(
          ...translatedMessage(
            "the_action_could_not_be_prepared_refresh_and__29238fe6b7",
          ),
        ),
      );
    }
  };
  return (
    <section
      className="rp-result"
      id="gateway-bindings"
      ref={section}
      tabIndex={-1}
      aria-label={text(
        ...translatedMessage("persistent_gateway_bindings_a07357cc40"),
      )}
    >
      <header>
        <h3>
          {text(...translatedMessage("persistent_gateway_bindings_a07357cc40"))}
        </h3>
        <Button
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          {text(...translatedMessage("refresh_binding_state_aa78b3a732"))}
        </Button>
      </header>
      <p>
        {text(
          ...translatedMessage(
            "stored_pairs_count_retained_routes_route_elig_054347aa8f",
          ),
        )}
      </p>
      {actionError && <p role="alert">{actionError}</p>}
      {receipt && (
        <div
          className="rp-result-status"
          role="status"
          ref={resultElement}
          tabIndex={-1}
        >
          <strong>
            {text(
              ...translatedMessage("binding_action_record_verified_b06bf70153"),
            )}
          </strong>
          <p>
            {receipt.operation === "release"
              ? text(
                  ...translatedMessage("binding_release_committed_ac8e82da09"),
                )
              : text(
                  ...translatedMessage("route_revocation_committed_8c7c7909f9"),
                )}{" "}
            · <code dir="ltr">{receipt.change_id}</code>
          </p>
          <p>
            <code dir="ltr">
              {receipt.source_channel_id} → {receipt.gateway_channel_id}
            </code>{" "}
            · {text(...translatedMessage("resources_dd59219f36"))} v
            {receipt.resource_revision} ·{" "}
            {text(...translatedMessage("channels_40b918ebad"))} v
            {receipt.channel_revision}
          </p>
          <p>
            {text(
              ...translatedMessage(
                "this_proves_a_historical_commit_verify_curren_ad92a7f440",
              ),
            )}
          </p>
        </div>
      )}
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_bindings_20257bf604"))}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "binding_state_could_not_be_read_old_results_a_3d492936db",
            ),
          )}
        </p>
      )}
      {state && (
        <>
          {state.binding_changes_available !== true && (
            <p>
              {text(
                ...translatedMessage(
                  "this_server_has_not_enabled_binding_changes_s_0649f5c245",
                ),
              )}
            </p>
          )}
          <p className="rp-versions">
            {text(...translatedMessage("resources_dd59219f36"))} v
            {state.resource_revision} ·{" "}
            {text(...translatedMessage("channels_40b918ebad"))} v
            {state.channel_revision} ·{" "}
            {text(...translatedMessage("configuration_2034cf5638"))} v
            {state.option_version}
          </p>
          <div
            className="rp-table"
            tabIndex={0}
            role="region"
            aria-label={text(
              ...translatedMessage("binding_state_table_b6d76cd562"),
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>
                    {text(...translatedMessage("source_gateway_228821385d"))}
                  </th>
                  <th>
                    {text(...translatedMessage("configured_state_9bc9aab62a"))}
                  </th>
                  <th>
                    {text(
                      ...translatedMessage("stored_route_eligible_f55197f3bd"),
                    )}
                  </th>
                  <th>
                    {text(
                      ...translatedMessage(
                        "last_publication_record_59e2458727",
                      ),
                    )}
                  </th>
                  <th>{text(...translatedMessage("actions_d83f4ed14a"))}</th>
                </tr>
              </thead>
              <tbody>
                {state.bindings.map((row) => (
                  <tr
                    key={row.gateway_channel_id}
                    data-binding-gateway={row.gateway_channel_id}
                  >
                    <td>
                      <strong>
                        {row.gateway_name ||
                          text(
                            ...translatedMessage(
                              "missing_or_unnamed_gateway_a75d36e662",
                            ),
                          )}
                      </strong>
                      <code dir="ltr">
                        {row.source_channel_id || "—"} →{" "}
                        {row.gateway_channel_id}
                      </code>
                      {!row.gateway_present && (
                        <span>
                          {text(
                            ...translatedMessage("gateway_deleted_d6da530156"),
                          )}
                        </span>
                      )}
                      {row.source_channel_id && !row.source_present && (
                        <span>
                          {text(
                            ...translatedMessage(
                              "source_absent_from_inventory_2c82eba76a",
                            ),
                          )}
                        </span>
                      )}
                    </td>
                    <td>
                      {!row.source_channel_id
                        ? text(
                            ...translatedMessage(
                              "released_protective_marker_retained_fa1605b2a1",
                            ),
                          )
                        : row.configured_active
                          ? text(
                              ...translatedMessage(
                                "configuration_marked_active_d5af76d4cc",
                              ),
                            )
                          : text(
                              ...translatedMessage(
                                "revoked_source_retained_84f5d11b73",
                              ),
                            )}
                      {row.profile !== "openai_text_exact_v1" &&
                        row.profile !== "exact_resource_policy_v2" && (
                          <p>
                            {text(
                              ...translatedMessage(
                                "unrecognized_profile_routing_unavailable_1b159fe996",
                              ),
                            )}
                          </p>
                        )}
                    </td>
                    <td>
                      <code dir="ltr">
                        {row.stored_pair_count} /{" "}
                        {row.route_eligible_pair_count}
                      </code>
                    </td>
                    <td>
                      <code dir="ltr">
                        {row.publication_id === "0" ? "—" : row.publication_id}
                      </code>
                    </td>
                    <td>
                      {row.source_channel_id ? (
                        <div className="rp-binding-actions">
                          <Button
                            variant="danger"
                            aria-label={text(
                              ...translatedMessage("revoke_routes_db8670c658", {
                                p0: row.gateway_channel_id,
                              }),
                            )}
                            disabled={
                              !changesEnabled ||
                              (!row.configured_active &&
                                row.stored_pair_count === 0)
                            }
                            onClick={() => choose(row, "revoke")}
                          >
                            {text(
                              ...translatedMessage("revoke_routes_8b230db210"),
                            )}
                          </Button>
                          <Button
                            aria-label={text(
                              ...translatedMessage(
                                "release_binding_6405218f13",
                                { p0: row.gateway_channel_id },
                              ),
                            )}
                            disabled={!changesEnabled}
                            onClick={() => choose(row, "release")}
                          >
                            {text(
                              ...translatedMessage(
                                "release_binding_073dde5344",
                              ),
                            )}
                          </Button>
                        </div>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {state.total === 0 && (
            <p>
              {text(
                ...translatedMessage(
                  "no_persistent_bindings_yet_review_explicit_se_2a8ca9115b",
                ),
              )}
            </p>
          )}
          <div className="rp-actions">
            <Button
              disabled={page === 1 || query.isFetching}
              onClick={() => setPage(page - 1)}
            >
              {text(...translatedMessage("previous_2672646f39"))}
            </Button>
            <span>
              {page} / {Math.max(1, Math.ceil(state.total / state.page_size))} ·{" "}
              {state.total}
            </span>
            <Button
              disabled={
                page * state.page_size >= state.total || query.isFetching
              }
              onClick={() => setPage(page + 1)}
            >
              {text(...translatedMessage("next_c0fd39244f"))}
            </Button>
          </div>
        </>
      )}
      <ResourceBindingChangeDialog
        key={review?.request_id ?? "none"}
        review={review}
        onClose={() => setReview(undefined)}
        onCommitted={(value) => {
          setReceipt(value);
          setReview(undefined);
        }}
      />
    </section>
  );
}
