import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Layers3, Plus, Search } from "lucide-react";
import { Button } from "../components/ui/button";
import { PricingHistory } from "./admin-operations";
import { Drawer, Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  useServiceGroupCatalog,
  useUpsertServiceGroup,
  useDeleteServiceGroup,
} from "../lib/api/service-group";
import {
  configuredGroupRatio,
  validGroupDefinitionID,
  validGroupMultiplier,
  parseGroupAccountIDs,
  type ManagedServiceGroup,
} from "../lib/service-group-config";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import { ServiceGroupTargets } from "./service-group-targets";
import "../components/catalog-management.css";
import "./service-groups.css";

type Draft = {
  original: ManagedServiceGroup | null;
  id: string;
  locked: boolean;
  name: string;
  description: string;
  multiplier: string;
  enabled: boolean;
  visibility: "common" | "custom";
  accountText: string;
  replaceAccounts: boolean;
  optionVersion: number;
  ratios: Readonly<Record<string, number>>;
  remove: boolean;
};

export function ServiceGroups({ role: _role }: { role: number }) {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role);
  const authenticated = useSystemAccess((s) => s.session.accountId !== null);
  const allowed = authenticated && isAdmin(role);
  const query = useServiceGroupCatalog(allowed);
  const upsert = useUpsertServiceGroup();
  const remove = useDeleteServiceGroup();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [history, setHistory] = useState<string | null>(null);
  const [review, setReview] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  if (!allowed)
    return (
      <p role="alert">
        {text(
          ...translatedMessage(
            "an_actual_administrator_session_is_required_73d66b6397",
          ),
        )}
      </p>
    );
  const data = query.isError ? undefined : query.data;
  const groups = new Map(data?.groups.map((g) => [g.group_id, g]));
  const ids = [
    ...new Set([...groups.keys(), ...Object.keys(data?.group_ratios ?? {})]),
  ]
    .sort()
    .filter((id) =>
      `${id} ${groups.get(id)?.name ?? ""}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    );
  const pages = Math.max(1, Math.ceil(ids.length / 20));
  const currentPage = Math.min(page, pages - 1);
  const pending = upsert.isPending || remove.isPending;
  const versionWritable =
    !!data && data.option_version < Number.MAX_SAFE_INTEGER;
  const current = draft && groups.get(draft.id.trim());
  const conflict =
    !!draft &&
    (!data ||
      data.option_version !== draft.optionVersion ||
      (draft.original
        ? current?.id !== draft.original.id ||
          current?.revision !== draft.original.revision
        : !!current));
  const draftAccounts =
    draft?.visibility === "custom"
      ? draft.original?.visibility === "custom" && !draft.replaceAccounts
        ? draft.original.account_ids
        : parseGroupAccountIDs(draft.accountText)
      : [];
  const valid =
    !!draft &&
    (draft.remove ||
      (validGroupDefinitionID(draft.id.trim()) &&
        draft.name.trim().length > 0 &&
        draft.name.trim().length <= 128 &&
        draft.description.length <= 512 &&
        validGroupMultiplier(draft.multiplier) &&
        draftAccounts !== null));
  const begin = (id = "", deleting = false) => {
    if (!data) return;
    const original = groups.get(id) ?? null;
    const ratio = configuredGroupRatio(data, id);
    upsert.reset();
    remove.reset();
    setError("");
    setNotice("");
    setReview(deleting);
    setDraft({
      original,
      id,
      locked: !!id,
      name: original?.name ?? id,
      description: original?.description ?? "",
      multiplier:
        ratio !== undefined && ratio >= 0 && ratio <= 100
          ? String(ratio)
          : id
            ? ""
            : "1",
      enabled: original?.enabled ?? true,
      visibility: original?.visibility ?? "common",
      accountText: original?.account_ids.join("\n") ?? "",
      replaceAccounts: false,
      optionVersion: data.option_version,
      ratios: data.group_ratios,
      remove: deleting,
    });
  };
  const close = () => {
    if (!pending) {
      setDraft(null);
      setError("");
    }
  };
  const save = async () => {
    if (!draft || !data || !review || !valid || conflict || !versionWritable)
      return;
    const identity = {
      expected_group_id: draft.original?.id ?? 0,
      expected_revision: draft.original?.revision ?? 0,
      expected_option_version: draft.optionVersion,
    };
    try {
      if (draft.remove) {
        const result = await remove.mutateAsync({
          group_id: draft.id,
          ...identity,
        });
        if (result != null) throw new Error("Invalid deletion acknowledgement");
      } else {
        const result = await upsert.mutateAsync({
          group_id: draft.id.trim(),
          name: draft.name.trim(),
          description: draft.description.trim(),
          enabled: draft.enabled,
          multiplier: draft.multiplier.trim(),
          visibility: draft.visibility,
          account_ids: draftAccounts ?? [],
          ...identity,
        });
        if (
          result.group_id !== draft.id.trim() ||
          result.revision !== identity.expected_revision + 1 ||
          (draft.original && result.id !== draft.original.id)
        )
          throw new Error("Invalid group update acknowledgement");
      }
      setDraft(null);
      setNotice(
        draft.remove
          ? text(
              ...translatedMessage(
                "group_definition_and_its_ratio_were_deleted_0c58126cf8",
              ),
            )
          : text(
              ...translatedMessage(
                "group_configuration_saved_the_response_proces_c6300fa48f",
              ),
            ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              ...translatedMessage(
                "the_group_or_configuration_version_changed_th_15b0d0a1bf",
              ),
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                ...translatedMessage(
                  "the_server_rejected_this_configuration_the_dr_17f822f73d",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_save_or_reload_outcome_is_unconfirmed_ref_39ca5a925a",
                ),
              ),
      );
    }
  };
  const oldRatio =
    draft && Object.prototype.hasOwnProperty.call(draft.ratios, draft.id.trim())
      ? draft.ratios[draft.id.trim()]
      : undefined;
  const unknown = text(...translatedMessage("unconfigured_02a7666991"));
  return (
    <div className="catalog-management service-group-management">
      <header className="cm-heading">
        <div>
          <span className="pricing-eyebrow">
            <Layers3 size={16} />
            {text(...translatedMessage("system_settings_966a9e1755"))}
          </span>
          <h1>{text(...translatedMessage("service_groups_0b4d78d515"))}</h1>
          <p>
            {text(
              ...translatedMessage(
                "manage_database_group_definitions_and_ratios__5ca0806347",
              ),
            )}
          </p>
        </div>
        <div className="cm-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text(...translatedMessage("refresh_groups_fd60cbc9f7"))}
          </Button>
          <Button
            variant="primary"
            disabled={!versionWritable}
            onClick={() => begin()}
          >
            <Plus size={16} />
            {text(...translatedMessage("add_group_definition_2094e39272"))}
          </Button>
        </div>
      </header>
      <div className="cm-callout">
        <p>
          {text(
            ...translatedMessage(
              "this_view_shows_database_configuration_final__3cdc35163a",
            ),
          )}
        </p>
        <Link to="/admin?tab=resources">
          {text(...translatedMessage("view_configuration_sync_e89e18fe01"))}
        </Link>
      </div>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_group_configuration_4fa6d32826"))}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "cannot_verify_server_configuration_version_gu_02a9aa7812",
            ),
          )}
        </p>
      )}
      {data && (
        <p className="sgm-versions">
          <span>
            {text(...translatedMessage("configuration_version_1191a660ca"))}:{" "}
            <bdi>v{data.option_version}</bdi>
          </span>
          <span>
            {text(
              ...translatedMessage(
                "response_process_loaded_version_220a8c977b",
              ),
            )}
            : <bdi>v{data.loaded_option_version}</bdi>
          </span>
        </p>
      )}
      {notice && (
        <p className="pricing-notice" role="status">
          {notice}
        </p>
      )}
      <div className="cm-toolbar">
        <label className="cm-search">
          <Search size={17} />
          <input
            aria-label={text(
              ...translatedMessage("search_group_definitions_db1498b73c"),
            )}
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(0);
            }}
            placeholder={text(
              ...translatedMessage("search_names_or_identifiers_9dc5dcb475"),
            )}
          />
        </label>
        <span>
          {ids.length} {text(...translatedMessage("entries_b7200e3364"))}
        </span>
      </div>
      {data && data.groups.length === 0 && (
        <p>
          {text(
            ...translatedMessage(
              "no_database_group_definitions_existing_ratio__a1f08c83de",
            ),
          )}
        </p>
      )}
      <div
        className="cm-table"
        role="region"
        tabIndex={0}
        aria-label={text(
          ...translatedMessage(
            "group_configuration_table_horizontally_scroll_39aff114c3",
          ),
        )}
      >
        <table>
          <thead>
            <tr>
              <th>{text(...translatedMessage("group_528d77dde5"))}</th>
              <th>
                {text(...translatedMessage("definition_state_38341af71f"))}
              </th>
              <th>
                {text(...translatedMessage("sale_multiplier_fc00917682"))}
              </th>

              <th>{text(...translatedMessage("actions_d83f4ed14a"))}</th>
            </tr>
          </thead>
          <tbody>
            {ids.slice(currentPage * 20, currentPage * 20 + 20).map((id) => {
              const group = groups.get(id);
              const ratio = data ? configuredGroupRatio(data, id) : undefined;
              const editable =
                versionWritable &&
                validGroupDefinitionID(id) &&
                (!group || group.revision < Number.MAX_SAFE_INTEGER);
              return (
                <tr key={id} data-config-group={id}>
                  <td>
                    <strong>{group?.name ?? id}</strong>
                    <code dir="ltr">{id}</code>
                    {group?.description && <small>{group.description}</small>}
                  </td>
                  <td>
                    {group
                      ? group.enabled
                        ? text(...translatedMessage("enabled_f028916a96"))
                        : text(...translatedMessage("disabled_0049e8e608"))
                      : text(
                          ...translatedMessage(
                            "ratio_only_no_definition_1869d3ba62",
                          ),
                        )}
                    {group?.visibility === "custom" && (
                      <small>
                        {text(
                          ...translatedMessage(
                            "custom_account_scope_1d24b5701c",
                          ),
                        )}
                      </small>
                    )}
                  </td>
                  <td>
                    <bdi>{ratio === undefined ? unknown : ratio}</bdi>
                    {group &&
                      ratio !== undefined &&
                      Number(group.multiplier) !== ratio && (
                        <small>
                          {text(
                            ...translatedMessage(
                              "stored_definition_4a0212ae33",
                            ),
                          )}
                          : {group.multiplier} ·{" "}
                          {text(
                            ...translatedMessage("aligned_on_save_d007ce55fc"),
                          )}
                        </small>
                      )}
                  </td>
                  <td>
                    <div className="cm-actions">
                      <Button onClick={() => setHistory(id)}>
                        {text(
                          ...translatedMessage("multiplier_history_d1836371bc"),
                        )}
                      </Button>
                      <Button disabled={!editable} onClick={() => begin(id)}>
                        {group
                          ? text(
                              ...translatedMessage(
                                "edit_definition_1bcd613a31",
                              ),
                            )
                          : text(
                              ...translatedMessage("register_group_3a91186601"),
                            )}
                      </Button>
                      {group && (
                        <Button
                          disabled={!editable}
                          onClick={() => begin(id, true)}
                        >
                          {text(
                            ...translatedMessage(
                              "delete_definition_d0cc3d2cb1",
                            ),
                          )}
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {data && (
        <div className="cm-actions">
          <Button
            disabled={currentPage === 0}
            onClick={() => setPage(currentPage - 1)}
          >
            {text(...translatedMessage("previous_2672646f39"))}
          </Button>
          <span>
            {currentPage + 1} / {pages}
          </span>
          <Button
            disabled={currentPage >= pages - 1}
            onClick={() => setPage(currentPage + 1)}
          >
            {text(...translatedMessage("next_c0fd39244f"))}
          </Button>
        </div>
      )}
      <ServiceGroupTargets />
      <Drawer
        open={!!history}
        onOpenChange={(v) => {
          if (!v) setHistory(null);
        }}
        title={text(...translatedMessage("multiplier_history_d1836371bc"))}
        description={history ?? ""}
      >
        {history && <PricingHistory kind="service-group" id={history} />}
      </Drawer>
      <Modal
        className="service-group-dialog"
        open={!!draft}
        onOpenChange={(open) => !open && close()}
        title={
          draft?.remove
            ? text(...translatedMessage("delete_group_definition_c22c2b0316"))
            : text(...translatedMessage("edit_group_definition_474a05934f"))
        }
        description={text(
          ...translatedMessage(
            "saving_requires_matching_group_identity_metad_d937d9133b",
          ),
        )}
        footer={
          <>
            <Button disabled={pending} onClick={close}>
              {text(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            {review ? (
              <Button
                variant="primary"
                disabled={pending || conflict || !valid || !versionWritable}
                onClick={() => void save()}
              >
                {text(...translatedMessage("confirm_group_changes_b237850451"))}
              </Button>
            ) : (
              <Button
                variant="primary"
                disabled={!valid || conflict || !versionWritable}
                onClick={() => setReview(true)}
              >
                {text(...translatedMessage("review_group_changes_0c1a80773f"))}
              </Button>
            )}
          </>
        }
      >
        {draft && (
          <>
            <p>
              <bdi>{draft.id || "—"}</bdi> ·{" "}
              {text(...translatedMessage("metadata_cfa777c3b7"))}: v
              {draft.original?.revision ?? 0} ·{" "}
              {text(...translatedMessage("configuration_2034cf5638"))}: v
              {draft.optionVersion}
            </p>
            {review ? (
              <div className="sgm-review">
                <h3>
                  {draft.remove
                    ? text(
                        ...translatedMessage(
                          "delete_definition_and_its_ratio_5283f418cf",
                        ),
                      )
                    : draft.name.trim()}
                </h3>
                <p>
                  {text(...translatedMessage("option_ratio_change_dc4a4548b2"))}
                  :{" "}
                  <bdi>
                    {oldRatio ?? unknown} →{" "}
                    {draft.remove || !draft.enabled
                      ? unknown
                      : draft.multiplier.trim()}
                  </bdi>
                </p>
                <p>
                  {draft.remove || !draft.enabled
                    ? text(
                        ...translatedMessage(
                          "after_removing_the_ratio_this_group_no_longer_b1f55d6e04",
                        ),
                      )
                    : draft.visibility === "custom"
                      ? text(
                          ...translatedMessage(
                            "saving_publishes_this_ratio_and_the_account_s_576b3f5c4d",
                          ),
                        )
                      : text(
                          ...translatedMessage(
                            "common_account_scope_saving_publishes_this_ra_1e5a32fe96",
                          ),
                        )}
                </p>
                {!draft.remove && draft.visibility === "custom" && (
                  <div className="sgm-legacy">
                    <strong>
                      {text(
                        ...translatedMessage("confirm_account_list_7c2a6c5484"),
                      )}
                    </strong>
                    <p
                      dir="ltr"
                      role="region"
                      tabIndex={0}
                      aria-label={text(
                        ...translatedMessage(
                          "account_identifiers_to_save_257dd6d12b",
                        ),
                      )}
                    >
                      {draftAccounts?.join(", ")}
                    </p>
                  </div>
                )}
                {!draft.remove && (
                  <Button
                    disabled={pending}
                    onClick={() => {
                      setReview(false);
                      setError("");
                    }}
                  >
                    {text(...translatedMessage("back_to_edit_8e464c075a"))}
                  </Button>
                )}
              </div>
            ) : (
              <div className="sgm-fields">
                {draft.visibility !== "custom" && (
                  <p className="cm-wide">
                    {text(
                      ...translatedMessage(
                        "common_scope_is_open_to_all_accounts_subject__0e694a5d43",
                      ),
                    )}
                  </p>
                )}
                <label>
                  {text(...translatedMessage("group_identifier_24068b0a25"))}
                  <input
                    dir="ltr"
                    readOnly={draft.locked}
                    maxLength={64}
                    value={draft.id}
                    onChange={(e) => setDraft({ ...draft, id: e.target.value })}
                  />
                </label>
                <label>
                  {text(...translatedMessage("group_name_f6efd7fd39"))}
                  <input
                    maxLength={128}
                    value={draft.name}
                    onChange={(e) =>
                      setDraft({ ...draft, name: e.target.value })
                    }
                  />
                </label>
                <label>
                  {text(...translatedMessage("group_ratio_c88644f2fa"))}
                  <input
                    dir="ltr"
                    inputMode="decimal"
                    aria-label={text(
                      ...translatedMessage("group_ratio_c88644f2fa"),
                    )}
                    aria-describedby="group-ratio-help"
                    maxLength={32}
                    value={draft.multiplier}
                    onChange={(e) =>
                      setDraft({ ...draft, multiplier: e.target.value })
                    }
                  />
                  <small id="group-ratio-help">
                    {text(
                      ...translatedMessage(
                        "0_100_with_at_most_four_decimal_places_0_make_4357e2b5fe",
                      ),
                    )}
                  </small>
                </label>
                <label className="cm-check">
                  <input
                    type="checkbox"
                    checked={draft.enabled}
                    onChange={(e) =>
                      setDraft({ ...draft, enabled: e.target.checked })
                    }
                  />
                  {text(...translatedMessage("enable_group_ratio_d4c9629636"))}
                </label>
                <label className="cm-wide">
                  {text(...translatedMessage("group_description_17ae505524"))}
                  <textarea
                    maxLength={512}
                    value={draft.description}
                    onChange={(e) =>
                      setDraft({ ...draft, description: e.target.value })
                    }
                  />
                </label>
                <label className="cm-wide">
                  {text(...translatedMessage("account_scope_fdf75b281e"))}
                  <select
                    aria-label={text(
                      ...translatedMessage("account_scope_fdf75b281e"),
                    )}
                    value={draft.visibility}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        visibility: e.target.value as "common" | "custom",
                      })
                    }
                  >
                    <option value="common">
                      {text(
                        ...translatedMessage("common_all_accounts_347848fbee"),
                      )}
                    </option>
                    <option value="custom">
                      {text(
                        ...translatedMessage(
                          "custom_specified_server_user_ids_68339730fa",
                        ),
                      )}
                    </option>
                  </select>
                </label>
                {draft.visibility === "custom" &&
                  draft.original?.visibility === "custom" && (
                    <label className="cm-wide cm-check">
                      <input
                        type="checkbox"
                        checked={draft.replaceAccounts}
                        onChange={(e) =>
                          setDraft({
                            ...draft,
                            replaceAccounts: e.target.checked,
                          })
                        }
                      />
                      {text(
                        ...translatedMessage("replace_account_list_40f63eb6d4"),
                      )}
                    </label>
                  )}
                {draft.visibility === "custom" &&
                  (draft.original?.visibility !== "custom" ||
                    draft.replaceAccounts) && (
                    <label className="cm-wide">
                      {text(...translatedMessage("server_user_ids_b6ca9632e1"))}
                      <textarea
                        dir="ltr"
                        aria-label={text(
                          ...translatedMessage("server_user_ids_b6ca9632e1"),
                        )}
                        maxLength={4096}
                        value={draft.accountText}
                        onChange={(e) =>
                          setDraft({ ...draft, accountText: e.target.value })
                        }
                        aria-describedby="group-account-help"
                      />
                      <small id="group-account-help">
                        {text(
                          ...translatedMessage(
                            "enter_1_100_distinct_positive_integer_ids_sep_2ac166ceae",
                          ),
                        )}
                      </small>
                      {draft.accountText.trim() && !draftAccounts && (
                        <small role="alert">
                          {text(
                            ...translatedMessage(
                              "invalid_list_format_or_size_use_server_user_i_7b54ff2c4c",
                            ),
                          )}
                        </small>
                      )}
                    </label>
                  )}
                {draft.visibility === "custom" &&
                  draft.original?.visibility === "custom" &&
                  !draft.replaceAccounts && (
                    <div className="cm-wide sgm-legacy">
                      <strong>
                        {text(
                          ...translatedMessage(
                            "retained_custom_metadata_874ecd0d9f",
                          ),
                        )}
                      </strong>
                      <p
                        dir="ltr"
                        tabIndex={0}
                        role="region"
                        aria-label={text(
                          ...translatedMessage(
                            "retained_account_identifiers_9ba8719a8f",
                          ),
                        )}
                      >
                        {draft.original.account_ids.join(", ")}
                      </p>
                      <small>
                        {text(
                          ...translatedMessage(
                            "this_edit_retains_the_stored_account_list_the_e6b278f6ca",
                          ),
                        )}
                      </small>
                    </div>
                  )}
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  ...translatedMessage(
                    "the_group_or_configuration_version_changed_or_e233ae243b",
                  ),
                )}
              </p>
            )}
            {error && <p role="alert">{error}</p>}
          </>
        )}
      </Modal>
    </div>
  );
}
