import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  useServerPricing,
  useSaveServerPricing,
} from "../lib/api/pricing-options";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import {
  ownPrice,
  serverPricingMode,
  validServerRatio,
  validServerRequestPrice,
  validServerExpression,
  replaceServerModelPricing,
  type ServerPricing,
} from "../lib/server-pricing";
import { useSettingsText } from "./settings-text";
import "./server-token-pricing.css";
import { ServerDetailPricing } from "./server-detail-pricing";
import { ServerTimePricingEditor } from "./server-time-pricing";
import { ServerTokenTimePricingEditor } from "./server-time-token-pricing";

type Draft = {
  base: ServerPricing;
  id: string;
  locked: boolean;
  input: string;
  completion: string;
  kind: ReturnType<typeof serverPricingMode>;
  request: string;
  expression: string;
};
export function ServerTokenPricing() {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role),
    authenticated = useSystemAccess((s) => s.session.accountId !== null);
  const allowed = authenticated && isAdmin(role),
    query = useServerPricing(allowed),
    mutation = useSaveServerPricing();
  const [search, setSearch] = useState(""),
    [page, setPage] = useState(0),
    [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  if (!allowed) return null;
  const data = query.isError ? undefined : query.data;
  const ids = data
    ? [
        ...new Set([
          ...Object.keys(data.input),
          ...Object.keys(data.completion),
          ...Object.keys(data.perRequest),
          ...Object.keys(data.expressions),
        ]),
      ]
        .sort()
        .filter((id) => id.toLowerCase().includes(search.trim().toLowerCase()))
    : [];
  const pages = Math.max(1, Math.ceil(ids.length / 20)),
    currentPage = Math.min(page, pages - 1);
  const writable = !!data && data.version <= Number.MAX_SAFE_INTEGER - 4;
  const conflict = !!draft && (!data || data.version !== draft.base.version);
  const valid =
    !!draft &&
    draft.id === draft.id.trim() &&
    draft.id.length > 0 &&
    draft.id.length <= 128 &&
    !/[\u0000-\u001f\u007f]/.test(draft.id) &&
    !["myapis-auto", "veyra-auto"].includes(draft.id) &&
    (draft.locked ||
      ![
        draft.base.input,
        draft.base.completion,
        draft.base.perRequest,
        draft.base.expressions,
      ].some(
        (values) => ownPrice<string | number>(values, draft.id) !== undefined,
      )) &&
    (draft.kind === "expression"
      ? validServerExpression(draft.expression)
      : draft.kind === "per_request"
        ? validServerRequestPrice(draft.request)
        : validServerRatio(draft.input) && validServerRatio(draft.completion));
  const mode = (state: ServerPricing, id: string) =>
    ({
      per_token: text(...translatedMessage("per_token_7f44ff66c4")),
      per_request: text(...translatedMessage("per_request_63e05e97c4")),
      expression: text(...translatedMessage("expression_396d42fc66")),
    })[serverPricingMode(state, id)];
  const begin = (id = "") => {
    if (!data) return;
    mutation.reset();
    setError("");
    setNotice("");
    setReview(false);
    setDraft({
      base: data,
      id,
      locked: !!id,
      input: ownPrice(data.input, id)?.toString() ?? "",
      completion: ownPrice(data.completion, id)?.toString() ?? "",
      kind: serverPricingMode(data, id),
      request: ownPrice(data.perRequest, id)?.toString() ?? "",
      expression: ownPrice(data.expressions, id) ?? "",
    });
  };
  const save = async () => {
    if (!draft || !review || !valid || conflict || !writable) return;
    try {
      const options = replaceServerModelPricing(
        draft.base,
        draft.id,
        draft.kind === "expression"
          ? { mode: draft.kind, expression: draft.expression }
          : draft.kind === "per_request"
            ? { mode: draft.kind, price: draft.request }
            : {
                mode: draft.kind,
                input: draft.input,
                completion: draft.completion,
              },
      );
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        options,
      });
      if (
        result.version < draft.base.version ||
        result.version > draft.base.version + options.length
      )
        throw Error("Unexpected pricing revision");
      setDraft(null);
      setNotice(
        text(
          ...translatedMessage(
            "server_pricing_saved_at_v_response_process_lo_fd72fcb20f",
            { p0: result.version, p1: result.loaded_version },
          ),
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              ...translatedMessage(
                "server_configuration_changed_the_new_version__8751096670",
              ),
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                ...translatedMessage(
                  "the_server_rejected_this_configuration_the_dr_9f416eb0a7",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_save_or_reload_result_is_unconfirmed_refr_0684cbce2d",
                ),
              ),
      );
    }
  };
  return (
    <section className="server-token-pricing">
      <header>
        <div>
          <h2>
            {text(...translatedMessage("server_model_pricing_308a70163d"))}
          </h2>
          <p>
            {text(
              ...translatedMessage(
                "read_and_edit_actual_server_billing_configura_52ad6eed9e",
              ),
            )}
          </p>
        </div>
        <div className="stp-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text(...translatedMessage("refresh_pricing_db20dc83dd"))}
          </Button>
          <Button disabled={!writable} onClick={() => begin()}>
            {text(...translatedMessage("configure_model_pricing_4045f367bb"))}
          </Button>
        </div>
      </header>
      <p>
        {text(
          ...translatedMessage(
            "configure_edit_or_explicitly_switch_token_per_6e38dfdc5b",
          ),
        )}
      </p>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_server_pricing_8443ffc4cd"))}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "cannot_verify_the_server_snapshot_or_atomic_v_1145daed6d",
            ),
          )}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      <ServerDetailPricing data={data} />
      {data && (
        <>
          <p className="stp-versions">
            {text(...translatedMessage("database_version_7331c145d8"))}:{" "}
            <bdi>v{data.version}</bdi> ·{" "}
            {text(
              ...translatedMessage(
                "response_process_loaded_version_220a8c977b",
              ),
            )}
            : <bdi>v{data.loaded_version}</bdi> ·{" "}
            <Link to="/admin?tab=resources">
              {text(...translatedMessage("configuration_sync_82e05cc2ab"))}
            </Link>
          </p>
          <label className="stp-search">
            {text(...translatedMessage("search_configured_models_3507a557d4"))}
            <input
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(0);
              }}
            />
          </label>
          <div
            className="stp-table"
            tabIndex={0}
            role="region"
            aria-label={text(
              ...translatedMessage(
                "server_pricing_table_horizontally_scrollable_10f81abc56",
              ),
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>{text(...translatedMessage("model_id_2e00e41239"))}</th>
                  <th>
                    {text(...translatedMessage("pricing_mode_aa6a8d8f59"))}
                  </th>
                  <th>
                    {text(...translatedMessage("input_base_usd_m_44e6c38185"))}
                  </th>
                  <th>
                    {text(
                      ...translatedMessage("output_input_ratio_b3c82f0166"),
                    )}
                  </th>
                  <th>
                    {text(
                      ...translatedMessage(
                        "per_request_usd_request_f7d390b205",
                      ),
                    )}
                  </th>
                  <th>{text(...translatedMessage("action_2a8e78b55a"))}</th>
                </tr>
              </thead>
              <tbody>
                {ids
                  .slice(currentPage * 20, (currentPage + 1) * 20)
                  .map((id) => (
                    <tr key={id} data-server-priced-model={id}>
                      <td>
                        <bdi>{id}</bdi>
                      </td>
                      <td>{mode(data, id)}</td>
                      <td>
                        <bdi>
                          {serverPricingMode(data, id) === "per_token"
                            ? (ownPrice(data.input, id) ?? "—")
                            : "—"}
                        </bdi>
                      </td>
                      <td>
                        <bdi>
                          {serverPricingMode(data, id) === "per_token"
                            ? (ownPrice(data.completion, id) ?? "—")
                            : "—"}
                        </bdi>
                      </td>
                      <td>
                        <bdi>
                          {serverPricingMode(data, id) === "per_request"
                            ? ownPrice(data.perRequest, id)
                            : "—"}
                        </bdi>
                      </td>
                      <td>
                        <Button
                          size="small"
                          disabled={!writable}
                          onClick={() => begin(id)}
                        >
                          {serverPricingMode(data, id) === "expression"
                            ? text(
                                ...translatedMessage(
                                  "edit_billing_expression_9a565eebac",
                                ),
                              )
                            : serverPricingMode(data, id) === "per_request"
                              ? text(
                                  ...translatedMessage(
                                    "edit_request_price_14540a818a",
                                  ),
                                )
                              : text(
                                  ...translatedMessage(
                                    "edit_server_ratios_33d8381f40",
                                  ),
                                )}
                        </Button>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          <div className="stp-actions">
            <Button
              disabled={currentPage === 0}
              onClick={() => setPage(currentPage - 1)}
            >
              {text(...translatedMessage("previous_2672646f39"))}
            </Button>
            <span>
              {currentPage + 1} / {pages} · {ids.length}
            </span>
            <Button
              disabled={currentPage + 1 >= pages}
              onClick={() => setPage(currentPage + 1)}
            >
              {text(...translatedMessage("next_c0fd39244f"))}
            </Button>
          </div>
        </>
      )}
      <ServerTimePricingEditor />
      <ServerTokenTimePricingEditor />
      <Modal
        className="server-pricing-dialog"
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDraft(null);
        }}
        preventOutsideClose
        title={
          draft?.kind === "expression"
            ? text(
                ...translatedMessage(
                  "change_actual_billing_expression_a73da43aff",
                ),
              )
            : draft?.kind === "per_request"
              ? text(
                  ...translatedMessage(
                    "change_actual_request_price_9c68d608dd",
                  ),
                )
              : text(
                  ...translatedMessage(
                    "change_actual_server_ratios_1459c829d8",
                  ),
                )
        }
        description={
          draft?.kind === "expression"
            ? text(
                ...translatedMessage(
                  "the_expression_result_is_a_base_usd_amount_gr_642781fd18",
                ),
              )
            : draft?.kind === "per_request"
              ? text(
                  ...translatedMessage(
                    "per_request_prices_use_usd_request_mode_and_p_dec25e00dc",
                  ),
                )
              : text(
                  ...translatedMessage(
                    "input_and_output_ratios_are_saved_together_ag_431503f790",
                  ),
                )
        }
        footer={
          <>
            <Button
              disabled={mutation.isPending}
              onClick={() => setDraft(null)}
            >
              {text(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            <Button
              variant="primary"
              disabled={!valid || conflict || !writable || mutation.isPending}
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(
                    ...translatedMessage("confirm_server_pricing_5765113e19"),
                  )
                : text(
                    ...translatedMessage("review_server_changes_d9a0f4c6cc"),
                  )}
            </Button>
          </>
        }
      >
        {draft && (
          <>
            <p>
              <bdi>{draft.id || "—"}</bdi> · v{draft.base.version}
            </p>
            {review ? (
              <div className="stp-review">
                <p>
                  {text(...translatedMessage("pricing_mode_aa6a8d8f59"))}:{" "}
                  {draft.locked
                    ? mode(draft.base, draft.id)
                    : text(
                        ...translatedMessage("unconfigured_23a581fe9b"),
                      )}{" "}
                  →{" "}
                  {
                    {
                      per_token: text(
                        ...translatedMessage("per_token_7f44ff66c4"),
                      ),
                      per_request: text(
                        ...translatedMessage("per_request_63e05e97c4"),
                      ),
                      expression: text(
                        ...translatedMessage("expression_396d42fc66"),
                      ),
                    }[draft.kind]
                  }
                </p>
                {draft.kind === "expression" ? (
                  <>
                    <p>
                      {text(
                        ...translatedMessage("previous_expression_23f92e7324"),
                      )}
                    </p>
                    <code className="stp-expression">
                      {ownPrice(draft.base.expressions, draft.id)}
                    </code>
                    <p>
                      {text(
                        ...translatedMessage(
                          "new_expression_base_usd_e223547213",
                        ),
                      )}
                    </p>
                    <code className="stp-expression">{draft.expression}</code>
                  </>
                ) : draft.kind === "per_request" ? (
                  <p>
                    {text(
                      ...translatedMessage(
                        "per_request_usd_request_f7d390b205",
                      ),
                    )}
                    :{" "}
                    <bdi>
                      {ownPrice(draft.base.perRequest, draft.id)} →{" "}
                      {draft.request}
                    </bdi>
                  </p>
                ) : (
                  <>
                    <p>
                      {text(...translatedMessage("input_base_7c6f0894d7"))}:{" "}
                      <bdi>
                        {ownPrice(draft.base.input, draft.id) ?? "—"} →{" "}
                        {draft.input}
                      </bdi>
                    </p>
                    <p>
                      {text(
                        ...translatedMessage("output_input_ratio_b3c82f0166"),
                      )}
                      :{" "}
                      <bdi>
                        {ownPrice(draft.base.completion, draft.id) ?? "—"} →{" "}
                        {draft.completion}
                      </bdi>
                    </p>
                  </>
                )}
                <p>
                  {text(
                    ...translatedMessage(
                      "this_changes_the_selected_model_s_real_server_ff36c86a30",
                    ),
                  )}
                </p>
                <p>
                  {text(
                    ...translatedMessage(
                      "switching_to_per_token_removes_this_model_s_e_c1027f7f4e",
                    ),
                  )}
                </p>
                <Button
                  disabled={mutation.isPending}
                  onClick={() => {
                    setReview(false);
                    setError("");
                  }}
                >
                  {text(...translatedMessage("back_to_edit_8e464c075a"))}
                </Button>
              </div>
            ) : (
              <div className="stp-fields">
                <label>
                  {text(...translatedMessage("server_model_id_a9c8feb9d9"))}
                  <input
                    dir="ltr"
                    readOnly={draft.locked}
                    maxLength={128}
                    value={draft.id}
                    onChange={(e) => setDraft({ ...draft, id: e.target.value })}
                  />
                </label>
                <label>
                  {text(...translatedMessage("pricing_mode_aa6a8d8f59"))}
                  <select
                    aria-label={text(
                      ...translatedMessage("pricing_mode_aa6a8d8f59"),
                    )}
                    value={draft.kind}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        kind: e.target.value as Draft["kind"],
                      })
                    }
                  >
                    <option value="per_token">
                      {text(...translatedMessage("per_token_7f44ff66c4"))}
                    </option>
                    <option value="per_request">
                      {text(...translatedMessage("per_request_63e05e97c4"))}
                    </option>
                    <option value="expression">
                      {text(...translatedMessage("expression_396d42fc66"))}
                    </option>
                  </select>
                </label>
                {draft.kind === "expression" ? (
                  <>
                    <label>
                      {text(
                        ...translatedMessage(
                          "billing_expression_base_usd_a387449ba0",
                        ),
                      )}
                      <textarea
                        dir="ltr"
                        rows={5}
                        maxLength={512}
                        aria-label={text(
                          ...translatedMessage(
                            "billing_expression_base_usd_a387449ba0",
                          ),
                        )}
                        value={draft.expression}
                        onChange={(e) =>
                          setDraft({ ...draft, expression: e.target.value })
                        }
                      />
                    </label>
                    <small>
                      {text(
                        ...translatedMessage(
                          "use_a_nonempty_expression_of_at_most_512_utf__d7e16c7dea",
                        ),
                      )}
                    </small>
                  </>
                ) : draft.kind === "per_request" ? (
                  <>
                    <label>
                      {text(
                        ...translatedMessage(
                          "per_request_usd_request_f7d390b205",
                        ),
                      )}
                      <input
                        dir="ltr"
                        inputMode="decimal"
                        maxLength={32}
                        value={draft.request}
                        onChange={(e) =>
                          setDraft({ ...draft, request: e.target.value })
                        }
                      />
                    </label>
                    <small>
                      {text(
                        ...translatedMessage(
                          "price_must_be_greater_than_0_and_at_most_1000_525b787ceb",
                        ),
                      )}
                    </small>
                  </>
                ) : (
                  <>
                    <label>
                      {text(
                        ...translatedMessage(
                          "input_base_usd_million_tokens_928d66f340",
                        ),
                      )}
                      <input
                        dir="ltr"
                        inputMode="decimal"
                        maxLength={32}
                        value={draft.input}
                        onChange={(e) =>
                          setDraft({ ...draft, input: e.target.value })
                        }
                      />
                    </label>
                    <label>
                      {text(
                        ...translatedMessage("output_input_ratio_b3c82f0166"),
                      )}
                      <input
                        dir="ltr"
                        inputMode="decimal"
                        maxLength={32}
                        value={draft.completion}
                        onChange={(e) =>
                          setDraft({ ...draft, completion: e.target.value })
                        }
                      />
                    </label>
                    <small>
                      {text(
                        ...translatedMessage(
                          "both_values_must_be_finite_numbers_from_0_to__277e1c4460",
                        ),
                      )}
                    </small>
                  </>
                )}
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  ...translatedMessage(
                    "configuration_changed_or_cannot_be_read_refre_d4edd25f0d",
                  ),
                )}
              </p>
            )}
            {error && <p role="alert">{error}</p>}
          </>
        )}
      </Modal>
    </section>
  );
}
