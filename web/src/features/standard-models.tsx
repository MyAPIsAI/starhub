import { message as translatedMessage } from "../i18n/messages";
import { lazy, Suspense, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import {
  usePlazaModelMeta,
  useUpsertModelMeta,
  useDeleteModelMeta,
  type ModelMeta,
} from "../lib/api/plaza-meta";
import { emptyModelMetadata } from "../lib/model-metadata";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import { ApiError } from "../lib/api/http";
import { useSettingsText } from "./settings-text";
import "../components/catalog-management.css";
import "./standard-models.css";

const DemoModelPricing = lazy(() =>
  import("./model-pricing-admin").then((module) => ({
    default: module.ModelPricingAdmin,
  })),
);

type Draft = {
  original: ModelMeta | null;
  row: ModelMeta;
  remove: boolean;
  revision: number;
};
export function StandardModels({ role: _role }: { role: number }) {
  const { text } = useSettingsText(),
    role = useSystemAccess((s) => s.role),
    authenticated = useSystemAccess((s) => s.session.accountId !== null);
  const allowed = authenticated && isAdmin(role),
    [params, setParams] = useSearchParams(),
    demo = params.get("demo") === "1",
    selected = params.get("model");
  const query = usePlazaModelMeta(allowed && !demo),
    upsert = useUpsertModelMeta(),
    remove = useDeleteModelMeta();
  const [search, setSearch] = useState(""),
    [page, setPage] = useState(0),
    [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const selectModel = (id?: string) =>
    setParams((previous) => {
      const next = new URLSearchParams(previous);
      if (id) next.set("model", id);
      else next.delete("model");
      return next;
    });
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
  if (demo)
    return (
      <>
        <p className="pricing-policy">
          <Link to="/console/model-pricing">
            {text(
              ...translatedMessage("back_to_server_model_catalog_28ccc1fc3e"),
            )}
          </Link>
        </p>
        <Suspense
          fallback={
            <p role="status">
              {text(
                ...translatedMessage("loading_browser_demo_editor_3ce5730370"),
              )}
            </p>
          }
        >
          <DemoModelPricing role={role} modelId={selected ?? undefined} />
        </Suspense>
      </>
    );
  const data = query.isError ? undefined : query.data;
  const rows = data?.models ?? [],
    current = rows.find((row) => row.model === selected),
    filtered = rows.filter((row) =>
      `${row.model} ${row.vendor} ${row.vendor_display}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    );
  const pages = Math.max(1, Math.ceil(filtered.length / 20)),
    currentPage = Math.min(page, pages - 1),
    readable = !!data && query.isSuccess && !query.isError,
    pending = upsert.isPending || remove.isPending;
  const writable =
    !!data && readable && data.revision < Number.MAX_SAFE_INTEGER;
  const conflict = !!draft && (!data || draft.revision !== data.revision);
  const valid =
    !!draft &&
    (draft.remove ||
      (draft.row.model === draft.row.model.trim() &&
        draft.row.model.length > 0 &&
        draft.row.model.length <= 128 &&
        !/[\u0000-\u001f\u007f]/.test(draft.row.model) &&
        [draft.row.context_length, draft.row.max_output].every(
          (v) => Number.isSafeInteger(v) && v >= 0,
        ) &&
        Number.isSafeInteger(draft.row.sort_weight) &&
        (!!draft.original ||
          !rows.some((row) => row.model === draft.row.model))));
  const begin = (row: ModelMeta | null = null, deleting = false) => {
    if (!data || !writable) return;
    upsert.reset();
    remove.reset();
    setError("");
    setNotice("");
    setReview(deleting);
    setDraft({
      original: row,
      row: row ? structuredClone(row) : emptyModelMetadata(),
      remove: deleting,
      revision: data.revision,
    });
  };
  const patch = (value: Partial<ModelMeta>) =>
    setDraft((previous) =>
      previous ? { ...previous, row: { ...previous.row, ...value } } : previous,
    );
  const save = async () => {
    if (!draft || !review || !valid || !writable || conflict) return;
    try {
      if (draft.remove) {
        const result = await remove.mutateAsync({
          model: draft.row.model,
          expected_catalog_revision: draft.revision,
        });
        if (
          result.model !== draft.row.model ||
          result.revision < draft.revision ||
          result.revision > draft.revision + 1
        )
          throw Error("Invalid metadata deletion acknowledgement");
        selectModel();
      } else {
        const result = await upsert.mutateAsync({
          meta: draft.row,
          expected_catalog_revision: draft.revision,
        });
        if (
          result.meta.model !== draft.row.model ||
          result.revision !== draft.revision + 1
        )
          throw Error("Invalid metadata save acknowledgement");
        selectModel(result.meta.model);
      }
      setDraft(null);
      setNotice(
        text(
          ...translatedMessage(
            "server_model_metadata_saved_check_billing_rat_349215424b",
          ),
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              ...translatedMessage(
                "the_model_catalog_changed_and_was_not_overwri_12c005046b",
              ),
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                ...translatedMessage(
                  "the_server_rejected_this_metadata_the_draft_i_e9737802a7",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_save_result_is_unconfirmed_and_the_draft__a1c1c4c1bc",
                ),
              ),
      );
    }
  };
  const fields: [
    keyof Pick<
      ModelMeta,
      "vendor" | "vendor_display" | "tags" | "compatible_groups"
    >,
    string,
  ][] = [
    ["vendor", text(...translatedMessage("vendor_id_c044bad499"))],
    [
      "vendor_display",
      text(...translatedMessage("vendor_display_name_73a4170683")),
    ],
    [
      "tags",
      text(...translatedMessage("capability_tags_comma_separated_a2344bf02d")),
    ],
    [
      "compatible_groups",
      text(...translatedMessage("compatible_group_metadata_c7f3364fcc")),
    ],
  ];
  const reference = (row: ModelMeta) => (
    <dl className="sm-references">
      {(
        [
          [
            "official_input_per_m",
            text(
              ...translatedMessage("official_reference_input_usd_m_243caf060c"),
            ),
          ],
          [
            "official_output_per_m",
            text(
              ...translatedMessage(
                "official_reference_output_usd_m_deb7488cd5",
              ),
            ),
          ],
          [
            "official_cache_read_per_m",
            text(
              ...translatedMessage(
                "official_cache_read_reference_usd_m_5a0f269317",
              ),
            ),
          ],
          [
            "official_cache_write_per_m",
            text(
              ...translatedMessage(
                "official_cache_write_reference_usd_m_70dce01b7e",
              ),
            ),
          ],
          [
            "official_per_request",
            text(
              ...translatedMessage(
                "official_per_request_reference_usd_0de7386b05",
              ),
            ),
          ],
        ] as const
      ).map(([key, label]) => (
        <div key={key}>
          <dt>{label}</dt>
          <dd>
            <bdi>{row[key] ?? "—"}</bdi>
          </dd>
        </div>
      ))}
    </dl>
  );
  return (
    <div className="catalog-management server-model-metadata">
      <header className="cm-heading">
        <div>
          <h1>
            {text(...translatedMessage("standard_model_metadata_77cddae1a1"))}
          </h1>
          <p>
            {text(
              ...translatedMessage(
                "only_server_registered_model_identities_refer_0c2dca68e5",
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
            {text(...translatedMessage("refresh_models_df0567c446"))}
          </Button>
          <Button
            variant="primary"
            disabled={!writable}
            onClick={() => begin()}
          >
            {text(...translatedMessage("register_model_metadata_d3820a9b66"))}
          </Button>
        </div>
      </header>
      <div className="cm-actions">
        <Link to="/admin?tab=resources">
          {text(
            ...translatedMessage(
              "server_billing_prices_and_configuration_sync_9de4cd2bbf",
            ),
          )}
        </Link>
        <Link to="/console/channels">
          {text(
            ...translatedMessage("channels_and_resource_inventory_cdd220a067"),
          )}
        </Link>
        <Link to="/console/model-pricing?demo=1">
          {text(
            ...translatedMessage("browser_demo_pricing_separate_e43351a479"),
          )}
        </Link>
      </div>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_server_models_403dded6f4"))}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "cannot_verify_server_model_metadata_or_revisi_3fe7f57450",
            ),
          )}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {data && (
        <p>
          {text(...translatedMessage("model_catalog_version_3cd090b87b"))}:{" "}
          <bdi>v{data.revision}</bdi>
        </p>
      )}
      {readable &&
        (selected ? (
          <section className="pricing-panel">
            <div className="cm-actions">
              <Button onClick={() => selectModel()}>
                {text(
                  ...translatedMessage("back_to_server_model_list_915d43f6b8"),
                )}
              </Button>
              {current && (
                <>
                  <Button disabled={!writable} onClick={() => begin(current)}>
                    {text(...translatedMessage("edit_metadata_ac32bce57d"))}
                  </Button>
                  <Button
                    disabled={!writable}
                    onClick={() => begin(current, true)}
                  >
                    {text(...translatedMessage("delete_metadata_9730180311"))}
                  </Button>
                </>
              )}
            </div>
            {current ? (
              <>
                <h2>
                  <bdi>{current.model}</bdi>
                </h2>
                <p>{current.vendor_display || current.vendor || "—"}</p>
                <p>{current.description || "—"}</p>
                <p>
                  {text(...translatedMessage("capability_tags_380f56f1e9"))}:{" "}
                  <bdi>{current.tags || "—"}</bdi>
                </p>
                <p>
                  {text(
                    ...translatedMessage(
                      "context_max_output_0_means_unknown_cda3079b5a",
                    ),
                  )}
                  :{" "}
                  <bdi>
                    {current.context_length} / {current.max_output}
                  </bdi>
                </p>
                {reference(current)}
              </>
            ) : (
              <p>
                {text(
                  ...translatedMessage(
                    "no_metadata_is_registered_for_this_model_on_t_226269f2a6",
                  ),
                )}
              </p>
            )}
          </section>
        ) : (
          <>
            <label className="cm-search">
              {text(...translatedMessage("search_server_models_344924f205"))}
              <input
                aria-label={text(
                  ...translatedMessage("search_server_models_344924f205"),
                )}
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setPage(0);
                }}
              />
            </label>
            <div
              className="cm-table"
              tabIndex={0}
              role="region"
              aria-label={text(
                ...translatedMessage(
                  "server_model_table_horizontally_scrollable_82d2506bf8",
                ),
              )}
            >
              <table>
                <thead>
                  <tr>
                    <th>{text(...translatedMessage("model_id_2e00e41239"))}</th>
                    <th>{text(...translatedMessage("vendor_a4625f4b1f"))}</th>
                    <th>
                      {text(
                        ...translatedMessage("capability_metadata_04a94730b6"),
                      )}
                    </th>
                    <th>
                      {text(
                        ...translatedMessage("context_max_output_8dbeb2fcd6"),
                      )}
                    </th>
                    <th>{text(...translatedMessage("action_2a8e78b55a"))}</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered
                    .slice(currentPage * 20, (currentPage + 1) * 20)
                    .map((row) => (
                      <tr key={row.model} data-server-model={row.model}>
                        <td>
                          <button
                            className="cm-text-button"
                            onClick={() => selectModel(row.model)}
                          >
                            <bdi>{row.model}</bdi>
                          </button>
                        </td>
                        <td>{row.vendor_display || row.vendor || "—"}</td>
                        <td>
                          <bdi>{row.tags || "—"}</bdi>
                        </td>
                        <td>
                          <bdi>
                            {row.context_length || "—"} /{" "}
                            {row.max_output || "—"}
                          </bdi>
                        </td>
                        <td>
                          <div className="cm-actions">
                            <Button
                              size="small"
                              disabled={!writable}
                              onClick={() => begin(row)}
                            >
                              {text(
                                ...translatedMessage(
                                  "edit_metadata_ac32bce57d",
                                ),
                              )}
                            </Button>
                            <Button
                              size="small"
                              disabled={!writable}
                              onClick={() => begin(row, true)}
                            >
                              {text(
                                ...translatedMessage(
                                  "delete_metadata_9730180311",
                                ),
                              )}
                            </Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
            {!rows.length && (
              <p>
                {text(
                  ...translatedMessage(
                    "no_model_metadata_is_registered_on_the_server_444d88fc5c",
                  ),
                )}
              </p>
            )}
            <div className="cm-actions">
              <Button
                disabled={currentPage === 0}
                onClick={() => setPage(currentPage - 1)}
              >
                {text(...translatedMessage("previous_2672646f39"))}
              </Button>
              <span>
                {currentPage + 1} / {pages} · {filtered.length}
              </span>
              <Button
                disabled={currentPage + 1 >= pages}
                onClick={() => setPage(currentPage + 1)}
              >
                {text(...translatedMessage("next_c0fd39244f"))}
              </Button>
            </div>
          </>
        ))}
      <Modal
        className="server-model-dialog"
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !pending) setDraft(null);
        }}
        preventOutsideClose
        title={
          draft?.remove
            ? text(
                ...translatedMessage("delete_server_model_metadata_f073d8d3c2"),
              )
            : text(
                ...translatedMessage("edit_server_model_metadata_80ffedb872"),
              )
        }
        description={text(
          ...translatedMessage(
            "capability_tags_and_context_constraints_affec_c648a39a09",
          ),
        )}
        footer={
          <>
            <Button disabled={pending} onClick={() => setDraft(null)}>
              {text(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            <Button
              variant="primary"
              disabled={!valid || !writable || conflict || pending}
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(
                    ...translatedMessage("confirm_server_metadata_e43b9c5e1e"),
                  )
                : text(
                    ...translatedMessage("review_metadata_changes_6136d177e3"),
                  )}
            </Button>
          </>
        }
      >
        {draft && (
          <p>
            {text(...translatedMessage("draft_catalog_version_62e8cf517e"))}:{" "}
            <bdi>v{draft.revision}</bdi>
          </p>
        )}
        {draft &&
          (review ? (
            <div className="sm-review">
              <h3>
                <bdi>{draft.row.model}</bdi>
              </h3>
              {draft.remove ? (
                <p>
                  {text(
                    ...translatedMessage(
                      "deletes_this_model_metadata_only_channels_key_5e00410100",
                    ),
                  )}
                </p>
              ) : (
                <>
                  <p>{draft.row.vendor_display || draft.row.vendor || "—"}</p>
                  <p>
                    <bdi>{draft.row.tags || "—"}</bdi>
                  </p>
                  <p>
                    {text(
                      ...translatedMessage("context_max_output_8dbeb2fcd6"),
                    )}
                    :{" "}
                    <bdi>
                      {draft.row.context_length} / {draft.row.max_output}
                    </bdi>
                  </p>
                  <p>{draft.row.description || "—"}</p>
                  <p>
                    {text(
                      ...translatedMessage(
                        "compatible_groups_sort_weight_965904ed96",
                      ),
                    )}
                    :{" "}
                    <bdi>
                      {draft.row.compatible_groups || "—"} /{" "}
                      {draft.row.sort_weight}
                    </bdi>
                  </p>
                  <p>
                    {text(
                      ...translatedMessage(
                        "existing_official_references_score_and_other__b7d911d893",
                      ),
                    )}
                  </p>
                  <Button
                    disabled={pending}
                    onClick={() => {
                      setReview(false);
                      setError("");
                    }}
                  >
                    {text(...translatedMessage("back_to_edit_8e464c075a"))}
                  </Button>
                </>
              )}
            </div>
          ) : (
            <div className="cm-form">
              <label className="cm-wide">
                {text(...translatedMessage("server_model_id_a9c8feb9d9"))}
                <input
                  dir="ltr"
                  readOnly={!!draft.original}
                  maxLength={128}
                  value={draft.row.model}
                  onChange={(e) => patch({ model: e.target.value })}
                />
              </label>
              {fields.map(([key, label]) => (
                <label key={key}>
                  {label}
                  <input
                    value={draft.row[key]}
                    maxLength={
                      key === "vendor"
                        ? 64
                        : key === "vendor_display"
                          ? 128
                          : 256
                    }
                    onChange={(e) => patch({ [key]: e.target.value })}
                  />
                </label>
              ))}
              {(
                [
                  [
                    "context_length",
                    text(
                      ...translatedMessage(
                        "context_limit_0_means_unknown_6ce0e82c21",
                      ),
                    ),
                  ],
                  [
                    "max_output",
                    text(
                      ...translatedMessage(
                        "max_output_0_means_unknown_795f5133c0",
                      ),
                    ),
                  ],
                  [
                    "sort_weight",
                    text(...translatedMessage("sort_weight_8d8cdaea21")),
                  ],
                ] as const
              ).map(([key, label]) => (
                <label key={key}>
                  {label}
                  <input
                    type="number"
                    step="1"
                    value={draft.row[key]}
                    onChange={(e) => patch({ [key]: Number(e.target.value) })}
                  />
                </label>
              ))}
              <label className="cm-wide">
                {text(...translatedMessage("model_description_c9b2428dd0"))}
                <textarea
                  aria-label={text(
                    ...translatedMessage("model_description_c9b2428dd0"),
                  )}
                  maxLength={8192}
                  value={draft.row.description}
                  onChange={(e) => patch({ description: e.target.value })}
                />
              </label>
              <div className="cm-wide">{reference(draft.row)}</div>
            </div>
          ))}
        {conflict && draft && (
          <p role="alert">
            {text(
              ...translatedMessage(
                "the_catalog_changed_or_cannot_be_read_refresh_a669c6dfd0",
              ),
            )}
          </p>
        )}
        {!readable && draft && (
          <p role="alert">
            {text(
              ...translatedMessage(
                "server_data_cannot_be_read_saving_is_paused_5946f79db4",
              ),
            )}
          </p>
        )}
        {error && <p role="alert">{error}</p>}
      </Modal>
    </div>
  );
}
