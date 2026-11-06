import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  useServerTimePricing,
  useSaveServerTimePricing,
} from "../lib/api/time-pricing";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import {
  replaceServerTimeSchedule,
  timeScheduleDraft,
  timeScheduleError,
  validTimeModelId,
  type ServerTimePricing,
  type TimeScheduleDraft,
  type TimeWindowDraft,
  type TimeScheduleError,
} from "../lib/server-time-pricing";
import { useSettingsText } from "./settings-text";
import "./server-time-pricing.css";

type Draft = {
  base: ServerTimePricing;
  id: string;
  locked: boolean;
  remove: boolean;
  schedule: TimeScheduleDraft;
};
export function ServerTimePricingEditor() {
  const { text } = useSettingsText(),
    role = useSystemAccess((state) => state.role),
    authenticated = useSystemAccess(
      (state) => state.session.accountId !== null,
    );
  const allowed = authenticated && isAdmin(role),
    query = useServerTimePricing(allowed),
    mutation = useSaveServerTimePricing();
  const [search, setSearch] = useState(""),
    [page, setPage] = useState(0),
    [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  if (!allowed) return null;
  const data = query.isError ? undefined : query.data,
    writable = !!data && data.version < Number.MAX_SAFE_INTEGER;
  const ids = data
    ? Object.keys(data.rules)
        .sort()
        .filter((id) => id.toLowerCase().includes(search.trim().toLowerCase()))
    : [];
  const pages = Math.max(1, Math.ceil(ids.length / 20)),
    currentPage = Math.min(page, pages - 1);
  const conflict = !!draft && (!data || data.version !== draft.base.version);
  const issue =
    draft && !draft.remove ? timeScheduleError(draft.schedule) : null;
  const valid =
    !!draft &&
    (draft.remove ||
      (validTimeModelId(draft.id) &&
        !["myapis-auto", "veyra-auto"].includes(draft.id) &&
        !issue &&
        (draft.locked || !Object.hasOwn(draft.base.rules, draft.id))));
  const weekdays = [
    text(...translatedMessage("sunday_82937de721")),
    text(...translatedMessage("monday_7c39b82322")),
    text(...translatedMessage("tuesday_49b912ebc9")),
    text(...translatedMessage("wednesday_30e13d177b")),
    text(...translatedMessage("thursday_499610b34d")),
    text(...translatedMessage("friday_4023e1b4cd")),
    text(...translatedMessage("saturday_75bbed5f03")),
  ];
  const issueText: Record<TimeScheduleError, string> = {
    timezone: text(
      ...translatedMessage(
        "enter_an_explicit_iana_timezone_or_utc_local__fdda7768ff",
      ),
    ),
    limit: text(
      ...translatedMessage("each_model_supports_at_most_32_windows_459e6711ab"),
    ),
    id: text(
      ...translatedMessage(
        "window_ids_must_be_unique_using_1_64_ascii_le_bba3856c3c",
      ),
    ),
    label: text(
      ...translatedMessage(
        "window_labels_must_be_nonempty_at_most_128_ut_584c062da5",
      ),
    ),
    days: text(
      ...translatedMessage("select_at_least_one_weekday_per_window_404fa35fdc"),
    ),
    time: text(
      ...translatedMessage(
        "use_hh_mm_with_24_00_allowed_only_as_an_end_s_a63dda204d",
      ),
    ),
    multiplier: text(
      ...translatedMessage(
        "multiplier_must_be_greater_than_0_and_at_most_fdab9eba54",
      ),
    ),
    overlap: text(
      ...translatedMessage(
        "windows_overlap_check_weekdays_and_overnight__838187eb34",
      ),
    ),
  };
  const begin = (id = "", remove = false) => {
    if (!data) return;
    mutation.reset();
    setError("");
    setNotice("");
    setReview(remove);
    setDraft({
      base: data,
      id,
      locked: !!id,
      remove,
      schedule: timeScheduleDraft(
        Object.hasOwn(data.rules, id) ? data.rules[id] : undefined,
      ),
    });
  };
  const updateWindow = (index: number, changes: Partial<TimeWindowDraft>) => {
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
    if (!draft || !review || !valid || conflict || !writable) return;
    try {
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        rules: replaceServerTimeSchedule(
          draft.base,
          draft.id,
          draft.remove ? null : draft.schedule,
        ),
      });
      if (
        result.version < draft.base.version ||
        result.version > draft.base.version + 1
      )
        throw Error("Unexpected time-pricing revision");
      setDraft(null);
      setNotice(
        text(
          ...translatedMessage(
            "time_pricing_saved_at_v_response_process_load_5568185e02",
            { p0: result.version, p1: result.loaded_version },
          ),
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              ...translatedMessage(
                "server_configuration_changed_the_newer_versio_40699aa766",
              ),
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                ...translatedMessage(
                  "the_server_rejected_the_timezone_or_schedule__5e2bf605f6",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_save_or_reload_result_is_unconfirmed_refr_7247813b2a",
                ),
              ),
      );
    }
  };
  return (
    <section className="server-time-pricing">
      <header>
        <div>
          <h3>
            {text(
              ...translatedMessage("server_time_based_multipliers_1db7dcb25f"),
            )}
          </h3>
          <p>
            {text(
              ...translatedMessage(
                "actual_weekly_windows_apply_to_all_three_pric_2a15724f82",
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
            {text(...translatedMessage("refresh_schedules_f165f05804"))}
          </Button>
          <Button
            disabled={
              !writable || (!!data && Object.keys(data.rules).length >= 1024)
            }
            onClick={() => begin()}
          >
            {text(...translatedMessage("configure_time_schedule_6ec512e131"))}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_server_schedules_c8c565953f"))}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "cannot_verify_actual_schedules_billing_activa_bae65a9995",
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
            {text(...translatedMessage("search_scheduled_models_2fba1c79c6"))}
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
                "time_schedule_table_horizontally_scrollable_153b9e55b3",
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
                    <tr key={id} data-time-priced-model={id}>
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
                              ...translatedMessage("edit_schedule_f7b360bc8b"),
                            )}
                          </Button>
                          <Button
                            size="small"
                            disabled={!writable}
                            onClick={() => begin(id, true)}
                          >
                            {text(
                              ...translatedMessage(
                                "delete_time_schedule_e5f8f0e767",
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
                  "no_matching_server_schedules_demo_rules_are_n_e6ae057b83",
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
        className="server-pricing-dialog server-time-dialog"
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDraft(null);
        }}
        preventOutsideClose
        title={
          draft?.remove
            ? text(
                ...translatedMessage("delete_actual_time_schedule_4fb03ab595"),
              )
            : text(
                ...translatedMessage("change_actual_time_schedule_9c76a82ca5"),
              )
        }
        description={text(
          ...translatedMessage(
            "all_windows_for_the_selected_model_are_saved__fd62a1845a",
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
              disabled={!valid || !writable || conflict || mutation.isPending}
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(...translatedMessage("confirm_time_changes_ac098577f5"))
                : text(...translatedMessage("review_time_changes_5bdebbc2df"))}
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
                          "delete_this_model_s_schedule_future_requests__0e66691eaf",
                        ),
                      )
                    : text(
                        ...translatedMessage(
                          "save_this_model_s_complete_schedule_1a675f0ebd",
                        ),
                      )}
                </p>
                <p>
                  <bdi>{draft.schedule.timezone}</bdi> ·{" "}
                  {draft.schedule.windows.length}
                </p>
                {draft.schedule.windows.map((window) => (
                  <div className="time-window-review" key={window.id}>
                    <bdi>{window.id}</bdi>
                    <p>{window.label}</p>
                    <p>{window.days.map((day) => weekdays[day]).join(" · ")}</p>
                    <p>
                      <bdi>
                        {window.start} → {window.end} · {window.multiplier}×
                      </bdi>
                    </p>
                  </div>
                ))}
                <p>
                  {text(
                    ...translatedMessage(
                      "overnight_windows_belong_to_their_start_weekd_c3953ae946",
                    ),
                  )}
                </p>
                {!draft.remove && (
                  <Button
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
              <div className="stp-fields">
                <label>
                  {text(...translatedMessage("scheduled_model_id_97c646c482"))}
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
                      ...translatedMessage("window_ae6bb2941a", {
                        p0: index + 1,
                      }),
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
                      {[1, 2, 3, 4, 5, 6, 0].map((day) => (
                        <label key={day}>
                          <input
                            type="checkbox"
                            checked={window.days.includes(day)}
                            onChange={(event) =>
                              updateWindow(index, {
                                days: event.target.checked
                                  ? [...window.days, day]
                                  : window.days.filter(
                                      (selected) => selected !== day,
                                    ),
                              })
                            }
                          />
                          {weekdays[day]}
                        </label>
                      ))}
                    </div>
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
                          "end_hh_mm_24_00_allowed_60f391517d",
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
                    <label>
                      {text(
                        ...translatedMessage("window_multiplier_b309f0fc1c"),
                      )}
                      <input
                        dir="ltr"
                        inputMode="decimal"
                        maxLength={32}
                        value={window.multiplier}
                        onChange={(event) =>
                          updateWindow(index, {
                            multiplier: event.target.value,
                          })
                        }
                      />
                    </label>
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
                          {
                            id: "",
                            label: "",
                            days: [],
                            start: "00:00",
                            end: "24:00",
                            multiplier: "1",
                          },
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
                      "at_most_32_non_overlapping_weekly_windows_per_4dc8b73c07",
                    ),
                  )}
                </small>
                {issue && <p role="alert">{issueText[issue]}</p>}
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  ...translatedMessage(
                    "configuration_changed_or_could_not_be_read_re_0bc0410fe4",
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
