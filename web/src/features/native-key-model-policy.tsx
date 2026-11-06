import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, Search, Sparkles } from "lucide-react";
import { SMART_MODEL_ID } from "../lib/smart-model";
import type { TokenChoices } from "../lib/api/token-choices";
import "./native-key-model-policy.css";

export type NativeModelScope = "snapshot" | "custom";
export type NativePolicyError = "models" | "candidates" | null;

export function NativeKeyModelPolicy({
  choices,
  group,
  available,
  selected,
  scope,
  onScopeChange,
  onModelsChange,
  locked,
  existing,
  legacyUnrestricted,
  smartEnabled,
  onSmartChange,
  smartAvailable,
  smartSelected,
  onCandidatesChange,
  error,
  validationAttempt,
}: {
  choices: TokenChoices["choices"];
  group: string;
  available: string[];
  selected: string[];
  scope: NativeModelScope;
  onScopeChange: (scope: NativeModelScope) => void;
  onModelsChange: (models: string[]) => void;
  locked: boolean;
  existing: boolean;
  legacyUnrestricted: boolean;
  smartEnabled: boolean;
  onSmartChange: (enabled: boolean) => void;
  smartAvailable: string[];
  smartSelected: string[];
  onCandidatesChange: (models: string[] | null) => void;
  error: NativePolicyError;
  validationAttempt: number;
}) {
  const { i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [candidatesOpen, setCandidatesOpen] = useState(false);
  const candidateField = useRef<HTMLFieldSetElement>(null);
  const modelField = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!error) return;
    setOpen(true);
    if (error === "candidates") setCandidatesOpen(true);
    const frame = requestAnimationFrame(() =>
      (error === "candidates"
        ? candidateField.current
        : modelField.current
      )?.focus(),
    );
    return () => cancelAnimationFrame(frame);
  }, [error, validationAttempt]);

  const ids = [
    ...new Set([
      ...(locked ? [] : choices.map((choice) => choice.model)),
      ...selected,
    ]),
  ];
  const rows = ids
    .map((id) => {
      const info =
        choices.find(
          (choice) => choice.group === group && choice.model === id,
        ) ?? choices.find((choice) => choice.model === id);
      const vendor =
        info?.vendor && info.vendor !== "unknown"
          ? info.vendor_display || info.vendor
          : c(...translatedMessage("other_f6ed33d295"));
      return {
        id,
        info,
        vendor,
        available: available.includes(id),
        retained: selected.includes(id),
      };
    })
    .filter((row) =>
      `${row.id} ${row.vendor}`
        .toLocaleLowerCase()
        .includes(search.trim().toLocaleLowerCase()),
    );
  const vendors = [...new Set(rows.map((row) => row.vendor))].sort((a, b) =>
    a.localeCompare(b),
  );
  const capability = (
    tags: string[] | undefined,
    smart: boolean | undefined,
  ) => {
    if (tags?.includes("video"))
      return c(...translatedMessage("video_85e7b59814"));
    if (tags?.includes("image"))
      return c(...translatedMessage("images_d073ede5bb"));
    if (tags?.some((tag) => ["audio", "tts", "stt"].includes(tag)))
      return c(...translatedMessage("audio_c62be0ebb2"));
    if (tags?.some((tag) => ["embedding", "embeddings"].includes(tag)))
      return c(...translatedMessage("embeddings_33397a5e10"));
    if (tags?.includes("rerank"))
      return c(...translatedMessage("reranking_685334e184"));
    return smart ? c(...translatedMessage("text_chat_8bfb0015d8")) : "—";
  };
  return (
    <section className="native-key-model-policy">
      <div className="native-key-policy-heading">
        <div>
          <h3 id="native-key-policy-title">
            {c(...translatedMessage("model_permissions_68da5e952e"))}
          </h3>
          <p>
            {c(
              ...translatedMessage(
                "control_which_models_this_key_can_call_explic_c48b92d14c",
              ),
            )}
          </p>
        </div>
        <button
          type="button"
          className="native-key-policy-disclosure"
          aria-expanded={open}
          aria-controls="native-key-policy-body"
          onClick={() => setOpen((value) => !value)}
        >
          {open
            ? c(...translatedMessage("collapse_e8d986c7b2"))
            : c(...translatedMessage("expand_c77fe8dab8"))}
          <ChevronDown size={14} aria-hidden="true" />
        </button>
      </div>
      <div
        id="native-key-policy-body"
        className="native-key-policy-body"
        hidden={!open}
      >
        <div
          className="native-key-model-scope segmented"
          role="group"
          aria-label={c(
            ...translatedMessage("model_authorization_scope_9c96cfc823"),
          )}
        >
          <button
            id="native-key-scope-snapshot"
            type="button"
            aria-pressed={scope === "snapshot"}
            disabled={locked}
            onClick={() => onScopeChange("snapshot")}
          >
            {c(...translatedMessage("all_models_at_save_d762055489"))}
          </button>
          <button
            id="native-key-scope-custom"
            type="button"
            aria-pressed={scope === "custom"}
            disabled={locked}
            onClick={() => onScopeChange("custom")}
          >
            {c(...translatedMessage("select_models_6c1f578d62"))}
          </button>
        </div>
        {locked && (
          <p className="native-key-policy-hint" role="status">
            {c(
              ...translatedMessage(
                "existing_automatic_groups_and_model_permissio_4a1721d222",
              ),
            )}
          </p>
        )}
        {scope === "snapshot" ? (
          <div
            ref={modelField}
            tabIndex={-1}
            className="native-key-policy-hint"
          >
            <p>
              {c(
                ...translatedMessage(
                  "authorize_the_current_group_s_available_model_7da6554441",
                ),
              )}
            </p>
            <span>
              {c(
                ...translatedMessage("models_in_this_snapshot_7168c114b4", {
                  p0: selected.length,
                }),
              )}
            </span>
            {existing && (
              <button
                type="button"
                className="native-key-policy-link"
                disabled={locked}
                onClick={() => onScopeChange("snapshot")}
              >
                {c(...translatedMessage("update_snapshot_8e218ee594"))}
              </button>
            )}
          </div>
        ) : (
          <div
            ref={modelField}
            tabIndex={-1}
            className="native-key-model-picker"
          >
            <div className="native-key-model-toolbar">
              <label className="native-key-model-search">
                <Search size={15} aria-hidden="true" />
                <input
                  id="native-key-model-search"
                  type="search"
                  value={search}
                  aria-label={c(
                    ...translatedMessage("search_models_13e4a8df03"),
                  )}
                  placeholder={c(
                    ...translatedMessage("search_models_13e4a8df03"),
                  )}
                  onChange={(event) => setSearch(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") event.preventDefault();
                  }}
                />
              </label>
              <span className="native-key-model-count">
                {c(
                  ...translatedMessage("selected_20fd78cb1c", {
                    p0: selected.length,
                  }),
                )}
              </span>
            </div>
            <div
              className="native-key-model-list"
              role="group"
              aria-label={c(
                ...translatedMessage("available_models_7875383e3b"),
              )}
            >
              {vendors.map((vendor) => (
                <div key={vendor} role="group" aria-label={vendor}>
                  <div className="native-key-model-vendor">{vendor}</div>
                  {rows
                    .filter((row) => row.vendor === vendor)
                    .map((row) => (
                      <label
                        key={row.id}
                        className={`native-key-model-row${!row.available && !locked ? " is-unavailable" : ""}`}
                      >
                        <input
                          type="checkbox"
                          aria-label={row.id}
                          checked={row.retained}
                          disabled={locked || (!row.available && !row.retained)}
                          onChange={(event) =>
                            onModelsChange(
                              event.target.checked
                                ? [...selected, row.id]
                                : selected.filter((id) => id !== row.id),
                            )
                          }
                        />
                        <code dir="ltr">{row.id}</code>
                        <span>
                          {locked
                            ? c(
                                ...translatedMessage(
                                  "retained_permission_20a3fd935d",
                                ),
                              )
                            : row.available
                              ? capability(
                                  row.info?.tags,
                                  row.info?.smart_eligible,
                                )
                              : row.info
                                ? c(
                                    ...translatedMessage(
                                      "unavailable_in_40102a9859",
                                      { p0: group },
                                    ),
                                  )
                                : c(
                                    ...translatedMessage(
                                      "unavailable_410033df61",
                                    ),
                                  )}
                        </span>
                      </label>
                    ))}
                </div>
              ))}
              {!rows.length && (
                <p className="native-key-model-empty">
                  {c(...translatedMessage("no_matching_models_dcc6cc1d60"))}
                </p>
              )}
            </div>
          </div>
        )}
        {legacyUnrestricted && (
          <p className="native-key-policy-hint">
            {c(
              ...translatedMessage(
                "editing_other_fields_preserves_the_existing_p_acde7c8d7d",
              ),
            )}
          </p>
        )}
        {error === "models" && (
          <p className="native-key-policy-error" role="alert">
            {c(
              ...translatedMessage(
                "select_at_least_one_available_model_in_this_g_ca33c115b9",
              ),
            )}
          </p>
        )}
        <div
          className={`native-key-smart-card${smartEnabled ? " is-enabled" : ""}`}
        >
          <div className="native-key-smart-heading">
            <span className="native-key-smart-icon" aria-hidden="true">
              <Sparkles size={18} />
            </span>
            <div className="native-key-smart-title">
              <span>{c(...translatedMessage("smart_entry_95929b20ea"))}</span>
              <code dir="ltr">{SMART_MODEL_ID}</code>
            </div>
            <button
              id="native-key-smart-access"
              type="button"
              role="switch"
              aria-checked={smartEnabled}
              aria-label={c(
                ...translatedMessage("allow_smart_routing_d717c151a5"),
              )}
              disabled={locked}
              className="native-key-smart-switch"
              onClick={() => onSmartChange(!smartEnabled)}
            >
              <span />
            </button>
          </div>
          <div className="native-key-smart-content">
            {smartEnabled ? (
              <p>
                {c(...translatedMessage("set_the_request_model_to_d469cad192"))}
                <code dir="ltr">{SMART_MODEL_ID}</code>
                {c(
                  ...translatedMessage(
                    "to_choose_within_the_authorized_scope_by_comb_1b2ed76ad5",
                  ),
                )}
              </p>
            ) : (
              <p>
                {c(
                  ...translatedMessage(
                    "smart_entry_is_off_this_key_can_still_call_ex_174a261310",
                  ),
                )}
              </p>
            )}
            {smartEnabled && (
              <>
                <div className="native-key-smart-details">
                  <p>
                    {c(...translatedMessage("strategy_999a2df0fc"))}
                    <strong>
                      {c(...translatedMessage("lowest_cost_6008c03066"))}
                    </strong>
                  </p>
                  <div className="native-key-candidate-summary">
                    <span>
                      {smartSelected.length === smartAvailable.length
                        ? c(
                            ...translatedMessage(
                              "candidates_all_authorized_text_models_7f323a6817",
                              { p0: smartSelected.length },
                            ),
                          )
                        : c(
                            ...translatedMessage(
                              "candidates_selected_text_models_def5a77907",
                              { p0: smartSelected.length },
                            ),
                          )}
                    </span>
                    <button
                      id="native-key-candidates-toggle"
                      type="button"
                      className="native-key-policy-link"
                      aria-expanded={candidatesOpen}
                      aria-controls="native-key-candidate-fields"
                      onClick={() => setCandidatesOpen((value) => !value)}
                    >
                      {candidatesOpen
                        ? c(...translatedMessage("hide_candidates_f29b0780f6"))
                        : c(
                            ...translatedMessage(
                              "narrow_candidates_952a6aef9a",
                            ),
                          )}
                    </button>
                  </div>
                </div>
                <div id="native-key-candidate-fields" hidden={!candidatesOpen}>
                  <fieldset
                    ref={candidateField}
                    tabIndex={-1}
                    className="native-key-candidate-list"
                  >
                    <legend>
                      {c(
                        ...translatedMessage(
                          "smart_candidate_models_7b2e6299b6",
                        ),
                      )}
                    </legend>
                    <button
                      type="button"
                      className="native-key-policy-link"
                      disabled={locked || smartAvailable.length === 0}
                      onClick={() => onCandidatesChange(null)}
                    >
                      {c(
                        ...translatedMessage(
                          "use_all_authorized_text_models_7f84e59c3f",
                        ),
                      )}
                    </button>
                    {smartAvailable.map((id) => (
                      <label key={id}>
                        <input
                          type="checkbox"
                          aria-label={id}
                          checked={smartSelected.includes(id)}
                          disabled={locked}
                          onChange={(event) =>
                            onCandidatesChange(
                              event.target.checked
                                ? [...smartSelected, id]
                                : smartSelected.filter((model) => model !== id),
                            )
                          }
                        />
                        <code dir="ltr">{id}</code>
                      </label>
                    ))}
                    {!smartAvailable.length && (
                      <p role="status">
                        {c(
                          ...translatedMessage(
                            "no_authorized_text_candidates_are_available_d91d7d1eb5",
                          ),
                        )}
                      </p>
                    )}
                  </fieldset>
                </div>
                {error === "candidates" && (
                  <p className="native-key-policy-error" role="alert">
                    {c(
                      ...translatedMessage(
                        "select_at_least_one_authorized_text_model_for_d429825043",
                      ),
                    )}
                  </p>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
