import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { useSaveServerPricing } from "../lib/api/pricing-options";
import { ApiError } from "../lib/api/http";
import {
  replaceServerDetailRatios,
  validServerRatio,
  type ServerPricing,
} from "../lib/server-pricing";
import { useSettingsText } from "./settings-text";

type Draft = { base: ServerPricing; cache: string; audio: string };
export function ServerDetailPricing({
  data,
}: {
  data: ServerPricing | undefined;
}) {
  const { text } = useSettingsText(),
    mutation = useSaveServerPricing();
  const [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const valid =
    !!draft &&
    [draft.cache, draft.audio].every(
      (value) => validServerRatio(value) && Number(value) <= 1000,
    );
  const conflict = !!draft && (!data || data.version !== draft.base.version);
  const writable = !!data && data.version <= Number.MAX_SAFE_INTEGER - 2;
  const cacheLabel = text(
    ...translatedMessage("cached_input_ratio_712cb0daf3"),
  );
  const audioLabel = text(...translatedMessage("audio_token_ratio_eae3784800"));
  const save = async () => {
    if (!draft || !review || !valid || conflict || !writable) return;
    try {
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        options: replaceServerDetailRatios(draft.cache, draft.audio),
      });
      if (
        result.version < draft.base.version ||
        result.version > draft.base.version + 2
      )
        throw Error("Unexpected pricing revision");
      setDraft(null);
      setNotice(
        text(
          ...translatedMessage(
            "global_ratios_saved_at_v_response_process_loa_ba878523f4",
            { p0: result.version, p1: result.loaded_version },
          ),
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              ...translatedMessage(
                "configuration_changed_the_newer_version_was_n_cdbaf9fd55",
              ),
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                ...translatedMessage(
                  "the_server_rejected_these_ratios_the_draft_is_2514c0fd00",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_save_or_reload_result_is_unconfirmed_refr_52e6c431c9",
                ),
              ),
      );
    }
  };
  return (
    <div className="stp-detail-ratios">
      {data && (
        <>
          <div className="stp-actions">
            <span>
              {cacheLabel}: <bdi>{data.cache}×</bdi> · {audioLabel}:{" "}
              <bdi>{data.audio}×</bdi>
            </span>
            <Button
              disabled={!writable}
              onClick={() => {
                setError("");
                setNotice("");
                setReview(false);
                mutation.reset();
                setDraft({
                  base: data,
                  cache: String(data.cache),
                  audio: String(data.audio),
                });
              }}
            >
              {text(
                ...translatedMessage("edit_global_detail_ratios_d0048695d9"),
              )}
            </Button>
          </div>
          <p>
            {text(
              ...translatedMessage(
                "global_multipliers_for_cached_input_and_audio_e12e8eee1f",
              ),
            )}
          </p>
        </>
      )}
      {notice && <p role="status">{notice}</p>}
      <Modal
        className="server-pricing-dialog"
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDraft(null);
        }}
        preventOutsideClose
        title={text(
          ...translatedMessage("change_global_detail_ratios_c3fb05e76f"),
        )}
        description={text(
          ...translatedMessage(
            "affects_future_requests_for_all_models_using__a3c0a36f4a",
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
              disabled={!valid || conflict || !writable || mutation.isPending}
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(...translatedMessage("confirm_global_ratios_59f5c8c089"))
                : text(
                    ...translatedMessage("review_global_changes_4508de6e7b"),
                  )}
            </Button>
          </>
        }
      >
        {draft && (
          <>
            <p>v{draft.base.version}</p>
            {review ? (
              <div className="stp-review">
                <p>
                  {cacheLabel}:{" "}
                  <bdi>
                    {draft.base.cache} → {draft.cache}
                  </bdi>
                </p>
                <p>
                  {audioLabel}:{" "}
                  <bdi>
                    {draft.base.audio} → {draft.audio}
                  </bdi>
                </p>
                <p>
                  {text(
                    ...translatedMessage(
                      "both_ratios_are_saved_atomically_against_one__a72b2a044b",
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
                  {cacheLabel}
                  <input
                    dir="ltr"
                    inputMode="decimal"
                    maxLength={32}
                    value={draft.cache}
                    onChange={(e) =>
                      setDraft({ ...draft, cache: e.target.value })
                    }
                  />
                </label>
                <label>
                  {audioLabel}
                  <input
                    dir="ltr"
                    inputMode="decimal"
                    maxLength={32}
                    value={draft.audio}
                    onChange={(e) =>
                      setDraft({ ...draft, audio: e.target.value })
                    }
                  />
                </label>
                <small>
                  {text(
                    ...translatedMessage(
                      "both_values_must_be_finite_nonnegative_number_807c899fb5",
                    ),
                  )}
                </small>
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  ...translatedMessage(
                    "configuration_changed_or_could_not_be_read_re_5f7e9ceca7",
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
