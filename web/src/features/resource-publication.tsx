import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import {
  previewResourcePublication,
  usePublicationContext,
} from "../lib/api/resource-publication";
import type { ResourceCatalog } from "../lib/resource-catalog";
import type {
  PublicationPair,
  PublicationPreview,
} from "../lib/resource-publication";
import { ApiError } from "../lib/api/http";
import {
  isAdmin,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import { PublicationCommit } from "./resource-publication-commit";
import { ResourceBindingList } from "./resource-binding-state";
import "./resource-publication.css";

export function ResourcePublication(props: {
  catalog: ResourceCatalog | undefined;
  refreshCatalog: () => Promise<unknown>;
}) {
  const identity = useSessionQueryIdentity();
  return <PublicationPanel key={JSON.stringify(identity)} {...props} />;
}

function PublicationPanel({
  catalog,
  refreshCatalog,
}: {
  catalog: ResourceCatalog | undefined;
  refreshCatalog: () => Promise<unknown>;
}) {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role),
    account = useSystemAccess((s) => s.session.accountId);
  const enabled = account !== null && isAdmin(role);
  const query = usePublicationContext(enabled);
  const [selected, setSelected] = useState(new Map<string, string>());
  const [preview, setPreview] = useState<PublicationPreview>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const active = useRef<AbortController>();
  useEffect(() => () => active.current?.abort(), []);
  if (!enabled) return null;
  const context = query.error ? undefined : query.data;
  const coherent = Boolean(
    catalog && context && catalog.revision === context.resource_revision,
  );
  const stale = Boolean(
    preview &&
    (!coherent ||
      !context ||
      preview.resource_revision !== context.resource_revision ||
      preview.channel_revision !== context.channel_revision ||
      preview.option_version !== context.option_version),
  );
  const sources = (catalog?.snapshot?.channels ?? []).filter((ch) =>
    [ch.id, ch.name].some((value) =>
      value.toLowerCase().includes(search.trim().toLowerCase()),
    ),
  );
  const pages = Math.max(1, Math.ceil(sources.length / 20)),
    current = Math.min(page, pages - 1);
  const reset = () => {
    active.current?.abort();
    active.current = undefined;
    setBusy(false);
    setPreview(undefined);
    setError("");
  };
  const run = async () => {
    if (!coherent || !context || !catalog || selected.size === 0) return;
    reset();
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    try {
      const sourceIDs = new Set(
        catalog.snapshot?.channels.map((ch) => ch.id) ?? [],
      );
      const bindings = [...selected]
        .filter(([source]) => sourceIDs.has(source))
        .map(([source_channel_id, gateway_channel_id]) => ({
          source_channel_id,
          gateway_channel_id,
        }));
      const result = await previewResourcePublication(
        context,
        bindings,
        controller.signal,
      );
      if (active.current === controller) setPreview(result);
    } catch (cause) {
      if (active.current === controller && !controller.signal.aborted)
        setError(
          cause instanceof ApiError && cause.status === 409
            ? text(
                ...translatedMessage(
                  "resources_channels_or_configuration_changed_r_a38dc0bfb0",
                ),
              )
            : text(
                ...translatedMessage(
                  "server_preview_is_unavailable_refresh_and_che_1f276181e5",
                ),
              ),
        );
    } finally {
      if (active.current === controller) {
        setBusy(false);
        active.current = undefined;
      }
    }
  };
  const issueLabel = (code: string) => {
    switch (code) {
      case "unknown_source_channel":
        return text(
          ...translatedMessage("source_channel_no_longer_exists_e37305772c"),
        );
      case "unknown_gateway_channel":
        return text(
          ...translatedMessage("gateway_channel_no_longer_exists_1bef1ecc74"),
        );
      case "unbound_source_channel":
        return text(
          ...translatedMessage(
            "select_a_gateway_channel_explicitly_fd2bc15132",
          ),
        );
      case "unavailable_resource_or_channel":
        return text(
          ...translatedMessage(
            "source_resource_or_gateway_channel_is_unavail_b645e8cecf",
          ),
        );
      case "model_alias_not_supported":
        return text(
          ...translatedMessage(
            "aliases_are_unsupported_all_three_model_ids_m_b244dd8077",
          ),
        );
      case "gateway_model_id_not_supported":
        return text(
          ...translatedMessage("model_id_exceeds_gateway_limits_f023d4d26a"),
        );
      case "protocol_or_capability_not_supported":
        return text(
          ...translatedMessage(
            "this_preview_supports_openai_type_text_code_r_f4cf7b8f77",
          ),
        );
      case "group_unavailable_or_unpriced":
        return text(
          ...translatedMessage(
            "group_is_unavailable_deleted_or_has_no_config_1c21cc3b9d",
          ),
        );
      case "no_supported_pairs":
        return text(
          ...translatedMessage("no_supported_model_and_group_pairs_807cbf610c"),
        );
      case "incompatible_resource_constraints":
        return text(
          ...translatedMessage(
            "sources_for_this_pair_declare_incompatible_ca_00660dcecb",
          ),
        );
      case "gateway_bound_to_other_source":
        return text(
          ...translatedMessage(
            "this_gateway_belongs_to_another_source_review_f9f02d2439",
          ),
        );
      case "source_bound_to_other_gateway":
        return text(
          ...translatedMessage(
            "this_source_is_bound_to_another_gateway_revie_7a8c687aa1",
          ),
        );
      case "binding_without_supported_pairs":
        return text(
          ...translatedMessage(
            "this_binding_has_no_supported_pairs_enable_va_582d0d3055",
          ),
        );
      case "publication_revision_exhausted":
        return text(
          ...translatedMessage(
            "publication_revisions_reached_the_safe_limit__f2ed19bb75",
          ),
        );
      default:
        return text(
          ...translatedMessage(
            "the_server_reported_a_blocking_issue_check_so_9f2fe57633",
          ),
        );
    }
  };
  return (
    <section
      id="gateway-publication"
      className="resource-publication"
      aria-label={text(
        ...translatedMessage("gateway_publication_preview_86ab27029b"),
      )}
    >
      <header>
        <div>
          <h2>
            {text(
              ...translatedMessage("gateway_publication_preview_86ab27029b"),
            )}
          </h2>
          <p>
            {text(
              ...translatedMessage(
                "map_inventory_sources_to_real_channels_then_r_1b3482100d",
              ),
            )}
          </p>
        </div>
        <Button
          disabled={busy || query.isFetching}
          onClick={() => {
            reset();
            void Promise.allSettled([query.refetch(), refreshCatalog()]);
          }}
        >
          {text(...translatedMessage("refresh_publication_context_a8afd9f762"))}
        </Button>
      </header>
      <aside>
        {text(
          ...translatedMessage(
            "generate_a_server_preview_first_only_explicit_ff0dc136f6",
          ),
        )}
      </aside>
      <p className="rp-profile">
        {text(
          ...translatedMessage(
            "resource_declarations_control_protocols_and_e_29171829ff",
          ),
        )}{" "}
        <code dir="ltr">/v1/chat/completions</code>
        {" · "}
        <code dir="ltr">/v1/messages</code>
        {". "}
        {text(
          ...translatedMessage(
            "tools_vision_responses_native_gemini_and_imag_270ce455cb",
          ),
        )}
      </p>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_real_channels_454111d899"))}
        </p>
      )}
      {(!catalog || query.error || (!query.isLoading && !context)) && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "server_publication_context_is_unavailable_pre_9735a5c575",
            ),
          )}
        </p>
      )}
      {context && catalog && (
        <>
          <p className="rp-versions">
            {text(...translatedMessage("resources_dd59219f36"))} v
            {context.resource_revision} ·{" "}
            {text(...translatedMessage("channels_40b918ebad"))} v
            {context.channel_revision} ·{" "}
            {text(...translatedMessage("configuration_2034cf5638"))} v
            {context.option_version}
          </p>
          {!coherent && (
            <p role="alert">
              {text(
                ...translatedMessage(
                  "inventory_revisions_differ_refresh_both_inven_d68c6775c9",
                ),
              )}
            </p>
          )}
          <label className="rp-search">
            {text(...translatedMessage("search_source_channels_580724a9d4"))}
            <input
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(0);
              }}
            />
          </label>
          <div
            className="rp-table"
            tabIndex={0}
            role="region"
            aria-label={text(
              ...translatedMessage("source_to_gateway_bindings_77591e6d89"),
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>
                    {text(...translatedMessage("source_channel_e6d8b886ae"))}
                  </th>
                  <th>
                    {text(
                      ...translatedMessage(
                        "gateway_channel_explicit_selection_a00053456d",
                      ),
                    )}
                  </th>
                </tr>
              </thead>
              <tbody>
                {sources
                  .slice(current * 20, (current + 1) * 20)
                  .map((source) => (
                    <tr key={source.id} data-publication-source={source.id}>
                      <td>
                        <strong>{source.name}</strong>
                        <code>{source.id}</code>
                      </td>
                      <td>
                        <select
                          aria-label={text(
                            ...translatedMessage("gateway_channel_47da408931", {
                              p0: source.id,
                            }),
                          )}
                          value={selected.get(source.id) ?? ""}
                          onChange={(e) => {
                            reset();
                            const next = new Map(selected);
                            if (e.target.value)
                              next.set(source.id, e.target.value);
                            else next.delete(source.id);
                            setSelected(next);
                          }}
                        >
                          <option value="">
                            {text(
                              ...translatedMessage(
                                "select_never_bound_automatically_6be2086699",
                              ),
                            )}
                          </option>
                          {context.channels.map((ch) => (
                            <option
                              key={ch.id}
                              value={ch.id}
                              disabled={[...selected].some(
                                ([s, id]) => s !== source.id && id === ch.id,
                              )}
                            >
                              {ch.name ||
                                text(
                                  ...translatedMessage(
                                    "unnamed_channel_a5aec61ea8",
                                  ),
                                )}{" "}
                              · {ch.id}
                              {ch.status !== 1
                                ? ` · ${text(...translatedMessage("disabled_2e15d27fba"))}`
                                : ""}
                            </option>
                          ))}
                        </select>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          {sources.length === 0 && (
            <p>
              {text(
                ...translatedMessage("no_matching_source_channels_8b2245b2b0"),
              )}
            </p>
          )}
          <div className="rp-actions">
            <Button
              disabled={current === 0}
              onClick={() => setPage(current - 1)}
            >
              {text(...translatedMessage("previous_2672646f39"))}
            </Button>
            <span>
              {current + 1} / {pages}
            </span>
            <Button
              disabled={current + 1 >= pages}
              onClick={() => setPage(current + 1)}
            >
              {text(...translatedMessage("next_c0fd39244f"))}
            </Button>
            <Button
              disabled={
                !coherent || busy || query.isFetching || selected.size === 0
              }
              onClick={() => void run()}
            >
              {busy
                ? text(...translatedMessage("generating_preview_20450fab38"))
                : text(
                    ...translatedMessage("generate_server_preview_5f52b840fb"),
                  )}
            </Button>
          </div>
        </>
      )}
      {error && <p role="alert">{error}</p>}
      {preview && context && catalog && !stale && (
        <div className="rp-result">
          <p role="status" className="rp-result-status">
            {preview.ready
              ? text(
                  ...translatedMessage(
                    "preview_checks_passed_nothing_has_been_publis_17071ec58d",
                  ),
                )
              : text(
                  ...translatedMessage(
                    "blocking_issues_remain_nothing_has_been_publi_427ab9f3ac",
                  ),
                )}
          </p>
          <PairTable
            key={`proposed-${preview.review_digest}`}
            title={text(...translatedMessage("proposed_pairs_91c410b11d"))}
            rows={preview.proposed_routes}
            showConstraints
          />
          <PairTable
            key={`existing-${preview.review_digest}`}
            title={text(
              ...translatedMessage("current_ability_pairs_39c462f95d"),
            )}
            rows={preview.existing_pairs}
            existing
          />
          {preview.issues.length > 0 && (
            <IssueList
              key={JSON.stringify(preview.issues)}
              issues={preview.issues}
              label={issueLabel}
            />
          )}
        </div>
      )}
      {stale && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "preview_dependencies_changed_previous_results_44e3806162",
            ),
          )}
        </p>
      )}
      {context && context.publish_available !== true && (
        <p>
          {text(
            ...translatedMessage(
              "this_server_provides_previews_only_publicatio_c5b0e5af39",
            ),
          )}
        </p>
      )}
      <PublicationCommit
        review={preview && !stale ? preview : undefined}
        disabled={
          !coherent ||
          stale ||
          busy ||
          query.isFetching ||
          context?.publish_available !== true
        }
        onCommitted={() => {
          reset();
          void Promise.allSettled([query.refetch(), refreshCatalog()]);
        }}
        renderRoutes={(review) => (
          <PairTable
            key={`confirmed-${review.review_digest}`}
            title={text(
              ...translatedMessage("confirmed_publication_pairs_787fbdc710"),
            )}
            rows={review.proposed_routes}
            showConstraints
          />
        )}
      />
      <ResourceBindingList enabled={enabled} />
    </section>
  );
}

function PairTable({
  title,
  rows,
  existing = false,
  showConstraints = false,
}: {
  title: string;
  rows: (PublicationPair & {
    enabled?: boolean;
    capabilities?: string[];
    upstream_model?: string;
    protocols?: string[];
    context_length?: number;
  })[];
  existing?: boolean;
  showConstraints?: boolean;
}) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(0);
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const current = Math.min(page, pages - 1);
  return (
    <section className="rp-pairs" data-proposed={showConstraints || undefined}>
      <h3>
        {title} · {rows.length}
      </h3>
      <div className="rp-table" role="region" tabIndex={0} aria-label={title}>
        <table>
          <thead>
            <tr>
              <th>{text(...translatedMessage("gateway_id_dbd0090ddd"))}</th>
              <th>{text(...translatedMessage("model_9c2e5b5065"))}</th>
              <th>{text(...translatedMessage("group_528d77dde5"))}</th>
              {showConstraints && (
                <>
                  <th>
                    {text(
                      ...translatedMessage("shared_capabilities_5093ca00de"),
                    )}
                  </th>
                  <th>
                    {text(...translatedMessage("context_limit_7479560329"))}
                  </th>
                </>
              )}
              {existing && (
                <th>
                  {text(...translatedMessage("ability_state_7fd03af902"))}
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {rows.slice(current * 20, (current + 1) * 20).map((row) => (
              <tr
                key={JSON.stringify([
                  row.gateway_channel_id,
                  row.model,
                  row.group,
                ])}
              >
                <td>
                  <code>{row.gateway_channel_id}</code>
                </td>
                <td>
                  <code>{row.model}</code>
                  {row.upstream_model && (
                    <div>
                      <code dir="ltr">
                        → {row.upstream_model} · {row.protocols?.join(" / ")}
                      </code>
                    </div>
                  )}
                </td>
                <td>
                  <code>{row.group}</code>
                </td>
                {showConstraints && (
                  <>
                    <td>
                      {row.capabilities
                        ?.map((capability) =>
                          capability === "code"
                            ? text(...translatedMessage("code_5d3b561425"))
                            : capability === "text"
                              ? text(...translatedMessage("text_0c246fb7e5"))
                              : capability,
                        )
                        .join(" · ")}
                    </td>
                    <td>
                      {row.context_length ? (
                        <code>{row.context_length.toLocaleString()}</code>
                      ) : (
                        text(...translatedMessage("unknown_a3cf284f50"))
                      )}
                    </td>
                  </>
                )}
                {existing && (
                  <td>
                    {row.enabled
                      ? text(...translatedMessage("enabled_d03bf0e513"))
                      : text(...translatedMessage("disabled_2e15d27fba"))}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {rows.length === 0 && (
        <p>{text(...translatedMessage("no_pairs_073d411f62"))}</p>
      )}
      {pages > 1 && (
        <div className="rp-actions">
          <Button disabled={current === 0} onClick={() => setPage(current - 1)}>
            {text(...translatedMessage("previous_2672646f39"))}
          </Button>
          <span>
            {current + 1} / {pages}
          </span>
          <Button
            disabled={current + 1 >= pages}
            onClick={() => setPage(current + 1)}
          >
            {text(...translatedMessage("next_c0fd39244f"))}
          </Button>
        </div>
      )}
    </section>
  );
}

function IssueList({
  issues,
  label,
}: {
  issues: PublicationPreview["issues"];
  label: (code: string) => string;
}) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(0);
  const pages = Math.ceil(issues.length / 20);
  return (
    <section
      aria-label={text(...translatedMessage("publication_blockers_a6af78d04c"))}
    >
      <h3>
        {text(...translatedMessage("publication_blockers_a6af78d04c"))} ·{" "}
        {issues.length}
      </h3>
      <ul className="rp-issues">
        {issues.slice(page * 20, (page + 1) * 20).map((issue, i) => (
          <li key={i}>
            <strong>{label(issue.code)}</strong>
            {(issue.resource_id || issue.source_channel_id || issue.group) && (
              <code>
                {[issue.resource_id, issue.source_channel_id, issue.group]
                  .filter(Boolean)
                  .join(" · ")}
              </code>
            )}
          </li>
        ))}
      </ul>
      {pages > 1 && (
        <div className="rp-actions">
          <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
            {text(...translatedMessage("previous_2672646f39"))}
          </Button>
          <span>
            {page + 1} / {pages}
          </span>
          <Button
            disabled={page + 1 >= pages}
            onClick={() => setPage(page + 1)}
          >
            {text(...translatedMessage("next_c0fd39244f"))}
          </Button>
        </div>
      )}
    </section>
  );
}
