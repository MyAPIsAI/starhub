import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  useServerTokenTimePricing,
  useSaveServerTokenTimePricing,
} from "../lib/api/time-token-pricing";
import { isAdmin, useSessionQueryIdentity } from "../lib/system-access";
import { validTimeModelId } from "../lib/server-time-pricing";
import {
  emptyTokenTimeWindow,
  replaceTokenTimeSchedule,
  tokenPriceKeys,
  tokenTimeScheduleError,
  type ServerTokenTimePricing,
  type TokenPriceKey,
  type TokenTimeSchedule,
  type TokenTimeWindow,
} from "../lib/server-time-token-pricing";
import { useSettingsText } from "./settings-text";
import "./server-time-pricing.css";
import "./server-time-token-pricing.css";

type Draft = {
  base: ServerTokenTimePricing;
  id: string;
  locked: boolean;
  remove: boolean;
  schedule: TokenTimeSchedule;
};

export function ServerTokenTimePricingEditor() {
  const scope = useSessionQueryIdentity();
  return <TokenTimeEditor key={JSON.stringify(scope)} />;
}

function TokenTimeEditor() {
  const { text, lang } = useSettingsText(),
    scope = useSessionQueryIdentity();
  const allowed = scope[1] !== null && isAdmin(scope[3]);
  const query = useServerTokenTimePricing(allowed),
    mutation = useSaveServerTokenTimePricing();
  const [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [attempted, setAttempted] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [search, setSearch] = useState(""),
    [page, setPage] = useState(0);
  if (!allowed) return null;
  const data = query.isError ? undefined : query.data;
  const writable = !!data && data.version < Number.MAX_SAFE_INTEGER;
  const ids = data
    ? Object.keys(data.rules)
        .sort()
        .filter((id) => id.toLowerCase().includes(search.trim().toLowerCase()))
    : [];
  const pages = Math.max(1, Math.ceil(ids.length / 20)),
    currentPage = Math.min(page, pages - 1);
  const conflict = !!draft && (!data || draft.base.version !== data.version);
  const issue =
    draft && !draft.remove ? tokenTimeScheduleError(draft.schedule) : null;
  const valid =
    !!draft &&
    (draft.remove ||
      (validTimeModelId(draft.id) &&
        !["myapis-auto", "veyra-auto"].includes(draft.id) &&
        !issue &&
        (draft.locked || !Object.hasOwn(draft.base.rules, draft.id))));
  const prices: Record<TokenPriceKey, string> = {
    input_per_m: text(...translatedMessage("text_input_ec3230d3db")),
    output_per_m: text(...translatedMessage("text_output_1e35e96b9d")),
    cache_read_per_m: text(...translatedMessage("cache_read_45022ee2b3")),
    cache_write_per_m: text(...translatedMessage("cache_write_bb46a7ab03")),
    audio_input_per_m: text(...translatedMessage("audio_input_1ed2703c7e")),
    audio_output_per_m: text(...translatedMessage("audio_output_f7c8200fa1")),
  };
  const weekdays = Array.from({ length: 7 }, (_, day) =>
    new Intl.DateTimeFormat(regionalLocale(lang), {
      weekday: "long",
      timeZone: "UTC",
    }).format(new Date(Date.UTC(2026, 0, 4 + day))),
  );
  const begin = (id = "", remove = false) => {
    if (!data) return;
    mutation.reset();
    setAttempted(false);
    setError("");
    setNotice("");
    setReview(remove);
    setDraft({
      base: data,
      id,
      locked: !!id,
      remove,
      schedule: Object.hasOwn(data.rules, id)
        ? structuredClone(data.rules[id])
        : { timezone: "UTC", windows: [emptyTokenTimeWindow()] },
    });
  };
  const updateWindow = (index: number, changes: Partial<TokenTimeWindow>) => {
    if (draft)
      setDraft({
        ...draft,
        schedule: {
          ...draft.schedule,
          windows: draft.schedule.windows.map((window, i) =>
            i === index ? { ...window, ...changes } : window,
          ),
        },
      });
  };
  const save = async () => {
    if (
      !draft ||
      !review ||
      !valid ||
      conflict ||
      !writable ||
      mutation.isPending ||
      attempted
    )
      return;
    let rules;
    try {
      rules = replaceTokenTimeSchedule(
        draft.base,
        draft.id,
        draft.remove ? null : draft.schedule,
      );
    } catch {
      setError(
        text(
          ...translatedMessage(
            "the_draft_exceeds_limits_or_contains_invalid__bbfb007a01",
          ),
        ),
      );
      return;
    }
    setAttempted(true);
    setError("");
    try {
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        rules,
      });
      setDraft(null);
      setNotice(
        text(
          ...translatedMessage(
            "independent_prices_saved_at_v_response_proces_16f65a0bce",
            { p0: result.version, p1: result.loaded_version },
          ),
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              ...translatedMessage(
                "configuration_changed_the_newer_version_was_n_cb544b59e2",
              ),
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                ...translatedMessage(
                  "the_server_rejected_the_rules_check_timezone__5d28be47c8",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_submission_outcome_is_unconfirmed_the_dra_55a8fa1ac3",
                ),
              ),
      );
    }
  };
  const priceReview = (window: TokenTimeWindow) => (
    <div className="token-window-prices">
      {tokenPriceKeys.map((key) => (
        <p key={key}>
          <span>{prices[key]}</span>
          <bdi>${window.prices[key]}</bdi>
        </p>
      ))}
    </div>
  );
  return (
    <section
      className="server-time-pricing server-time-token-pricing"
      aria-label={text(
        ...translatedMessage("server_independent_window_prices_3e99534292"),
      )}
    >
      <header>
        <div>
          <h3>
            {text(
              ...translatedMessage(
                "server_independent_window_prices_3e99534292",
              ),
            )}
          </h3>
          <p>
            {text(
              ...translatedMessage(
                "set_all_six_token_prices_per_window_in_usd_pe_22702fa0a0",
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
            {text(
              ...translatedMessage("refresh_independent_prices_ecf6f83371"),
            )}
          </Button>
          <Button
            disabled={
              !writable || (!!data && Object.keys(data.rules).length >= 1024)
            }
            onClick={() => begin()}
          >
            {text(
              ...translatedMessage("configure_independent_prices_03ee586732"),
            )}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_independent_prices_ce26d71458"))}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "cannot_verify_server_independent_prices_billi_1d2a401fb1",
            ),
          )}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {data && (
        <>
          <p>
            {text(
              ...translatedMessage(
                "database_response_process_version_2e855ba12b",
              ),
            )}
            :{" "}
            <bdi>
              v{data.version} / v{data.loaded_version}
            </bdi>
          </p>
          <label className="stp-search">
            {text(
              ...translatedMessage(
                "search_independent_price_models_f0d1899f30",
              ),
            )}
            <input
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
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
                "independent_price_schedule_table_horizontally_cfbfe21fc8",
              ),
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>{text(...translatedMessage("model_id_2e00e41239"))}</th>
                  <th>{text(...translatedMessage("timezone_589797f8d4"))}</th>
                  <th>{text(...translatedMessage("windows_cb0e7bb70d"))}</th>
                  <th>{text(...translatedMessage("action_2a8e78b55a"))}</th>
                </tr>
              </thead>
              <tbody>
                {ids
                  .slice(currentPage * 20, (currentPage + 1) * 20)
                  .map((id) => (
                    <tr key={id} data-token-time-model={id}>
                      <td>
                        <bdi>{id}</bdi>
                      </td>
                      <td>
                        <bdi>{data.rules[id].timezone}</bdi>
                      </td>
                      <td>{data.rules[id].windows.length}</td>
                      <td>
                        <div className="stp-actions">
                          <Button
                            size="small"
                            disabled={!writable}
                            onClick={() => begin(id)}
                          >
                            {text(
                              ...translatedMessage(
                                "edit_independent_prices_f265903bbe",
                              ),
                            )}
                          </Button>
                          <Button
                            size="small"
                            disabled={!writable}
                            onClick={() => begin(id, true)}
                          >
                            {text(
                              ...translatedMessage(
                                "delete_independent_schedule_7d1b56466d",
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
          {!ids.length && (
            <p>
              {text(
                ...translatedMessage(
                  "no_matching_independent_server_price_schedule_6b7fd107fe",
                ),
              )}
            </p>
          )}
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
      <Modal
        className="server-pricing-dialog server-time-dialog server-token-time-dialog"
        open={!!draft}
        preventOutsideClose
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDraft(null);
        }}
        title={
          draft?.remove
            ? text(
                ...translatedMessage("delete_independent_schedule_7d1b56466d"),
              )
            : text(
                ...translatedMessage(
                  "change_independent_window_prices_1aeed4f6dc",
                ),
              )
        }
        description={text(
          ...translatedMessage(
            "save_all_independent_windows_for_the_selected_0236233e61",
          ),
        )}
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
              disabled={
                !valid ||
                !writable ||
                conflict ||
                mutation.isPending ||
                attempted
              }
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(
                    ...translatedMessage(
                      "confirm_independent_price_changes_5cfe79db24",
                    ),
                  )
                : text(
                    ...translatedMessage(
                      "review_independent_price_changes_07d6b2746c",
                    ),
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
                  {draft.remove
                    ? text(
                        ...translatedMessage(
                          "after_deletion_future_requests_use_base_token_6bab9a163d",
                        ),
                      )
                    : text(
                        ...translatedMessage(
                          "save_these_complete_independent_window_prices_dcbaf4253e",
                        ),
                      )}
                </p>
                <p>
                  <bdi>{draft.schedule.timezone}</bdi> ·{" "}
                  {draft.schedule.windows.length}
                </p>
                {draft.schedule.windows.map((window, index) => (
                  <div className="time-window-review" key={index}>
                    <bdi>{window.id}</bdi>
                    <p>{window.label}</p>
                    <p>{window.days.map((day) => weekdays[day]).join(" · ")}</p>
                    <bdi>
                      {window.start} → {window.end}
                    </bdi>
                    {priceReview(window)}
                  </div>
                ))}
                <p>
                  {text(
                    ...translatedMessage(
                      "overnight_windows_belong_to_their_start_weekd_f4867c383b",
                    ),
                  )}
                </p>
                {!draft.remove && (
                  <Button
                    disabled={mutation.isPending || attempted}
                    onClick={() => setReview(false)}
                  >
                    {text(...translatedMessage("back_to_edit_8e464c075a"))}
                  </Button>
                )}
              </div>
            ) : (
              <div className="stp-fields">
                <label>
                  {text(
                    ...translatedMessage(
                      "independent_price_model_id_7f6b49865c",
                    ),
                  )}
                  <input
                    dir="ltr"
                    readOnly={draft.locked}
                    maxLength={128}
                    value={draft.id}
                    onChange={(event) =>
                      setDraft({ ...draft, id: event.target.value })
                    }
                  />
                </label>
                <label>
                  {text(
                    ...translatedMessage("iana_timezone_or_utc_441e4d2fe7"),
                  )}
                  <input
                    dir="ltr"
                    maxLength={64}
                    value={draft.schedule.timezone}
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        schedule: {
                          ...draft.schedule,
                          timezone: event.target.value,
                        },
                      })
                    }
                  />
                </label>
                {draft.schedule.windows.map((window, index) => (
                  <fieldset
                    className="time-window-fields"
                    key={index}
                    aria-label={text(
                      ...translatedMessage(
                        "independent_price_window_3c94b15e59",
                        { p0: index + 1 },
                      ),
                    )}
                  >
                    <legend>
                      {text(
                        ...translatedMessage("window_ae6bb2941a", {
                          p0: index + 1,
                        }),
                      )}
                    </legend>
                    <label>
                      {text(...translatedMessage("window_id_d73926f6d2"))}
                      <input
                        dir="ltr"
                        maxLength={64}
                        value={window.id}
                        onChange={(event) =>
                          updateWindow(index, { id: event.target.value })
                        }
                      />
                    </label>
                    <label>
                      {text(...translatedMessage("window_label_d2547826c7"))}
                      <input
                        maxLength={128}
                        value={window.label}
                        onChange={(event) =>
                          updateWindow(index, { label: event.target.value })
                        }
                      />
                    </label>
                    <div className="time-window-days">
                      {weekdays.map((day, number) => (
                        <label key={number}>
                          <input
                            type="checkbox"
                            checked={window.days.includes(number)}
                            onChange={(event) =>
                              updateWindow(index, {
                                days: event.target.checked
                                  ? [...window.days, number].sort()
                                  : window.days.filter((day) => day !== number),
                              })
                            }
                          />
                          {day}
                        </label>
                      ))}
                    </div>
                    <div className="token-window-times">
                      <label>
                        {text(...translatedMessage("start_hh_mm_186a4d8d50"))}
                        <input
                          dir="ltr"
                          maxLength={5}
                          value={window.start}
                          onChange={(event) =>
                            updateWindow(index, { start: event.target.value })
                          }
                        />
                      </label>
                      <label>
                        {text(
                          ...translatedMessage(
                            "end_hh_mm_24_00_allowed_02828fe9c3",
                          ),
                        )}
                        <input
                          dir="ltr"
                          maxLength={5}
                          value={window.end}
                          onChange={(event) =>
                            updateWindow(index, { end: event.target.value })
                          }
                        />
                      </label>
                    </div>
                    <div className="token-window-inputs">
                      {tokenPriceKeys.map((key) => (
                        <label key={key}>
                          {prices[key]} ·{" "}
                          {text(
                            ...translatedMessage(
                              "usd_million_tokens_385fb62e60",
                            ),
                          )}
                          <input
                            dir="ltr"
                            inputMode="decimal"
                            maxLength={17}
                            value={window.prices[key]}
                            onChange={(event) =>
                              updateWindow(index, {
                                prices: {
                                  ...window.prices,
                                  [key]: event.target.value,
                                },
                              })
                            }
                          />
                        </label>
                      ))}
                    </div>
                    <Button
                      onClick={() =>
                        setDraft({
                          ...draft,
                          schedule: {
                            ...draft.schedule,
                            windows: draft.schedule.windows.filter(
                              (_, i) => i !== index,
                            ),
                          },
                        })
                      }
                    >
                      {text(
                        ...translatedMessage("remove_this_window_3ef5184de9"),
                      )}
                    </Button>
                  </fieldset>
                ))}
                <Button
                  disabled={draft.schedule.windows.length >= 32}
                  onClick={() =>
                    setDraft({
                      ...draft,
                      schedule: {
                        ...draft.schedule,
                        windows: [
                          ...draft.schedule.windows,
                          emptyTokenTimeWindow(),
                        ],
                      },
                    })
                  }
                >
                  {text(...translatedMessage("add_window_971b01314f"))}
                </Button>
                <small>
                  {text(
                    ...translatedMessage(
                      "at_most_32_non_overlapping_weekly_windows_per_aab9ee4e6c",
                    ),
                  )}
                </small>
                {issue && (
                  <p role="alert">
                    {issue === "prices"
                      ? text(
                          ...translatedMessage(
                            "enter_all_six_valid_prices_17f3a4364d",
                          ),
                        )
                      : issue === "overlap"
                        ? text(
                            ...translatedMessage(
                              "windows_overlap_check_weekdays_and_overnight__f8c190e8ed",
                            ),
                          )
                        : text(
                            ...translatedMessage(
                              "check_timezone_unique_ids_labels_weekdays_and_84fab1b7c6",
                            ),
                          )}
                  </p>
                )}
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  ...translatedMessage(
                    "configuration_changed_or_could_not_be_read_re_236ae35825",
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
