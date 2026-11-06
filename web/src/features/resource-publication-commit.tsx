import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import {
  getPublicationReceipt,
  usePublishResourceReview,
} from "../lib/api/resource-publication";
import { ApiError } from "../lib/api/http";
import type { PublicationPreview } from "../lib/resource-publication";
import {
  receiptMatchesReview,
  type PublicationAcknowledgement,
} from "../lib/resource-publication-commit";
import { useSettingsText } from "./settings-text";

export function PublicationCommit({
  review,
  disabled,
  renderRoutes,
  onCommitted,
}: {
  review: PublicationPreview | undefined;
  disabled: boolean;
  renderRoutes: (review: PublicationPreview) => ReactNode;
  onCommitted: () => void;
}) {
  const { text } = useSettingsText();
  const publish = usePublishResourceReview();
  const [captured, setCaptured] = useState<PublicationPreview>();
  const [attempted, setAttempted] = useState(false);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const [ack, setAck] = useState<PublicationAcknowledgement>();
  const [historical, setHistorical] = useState(false);
  const mounted = useRef(true);
  const inFlight = useRef(false);
  const errorElement = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (error) errorElement.current?.focus();
  }, [error]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const pending = publish.isPending || checking;
  const confirm = async () => {
    if (!captured || attempted || inFlight.current || disabled) return;
    inFlight.current = true;
    setAttempted(true);
    setError("");
    try {
      const result = await publish.mutateAsync(captured);
      if (!mounted.current) return;
      setAck(result);
      setCaptured(undefined);
      onCommitted();
    } catch (cause) {
      if (!mounted.current) return;
      setError(
        cause instanceof ApiError && cause.status === 409
          ? text(
              ...translatedMessage(
                "this_review_is_stale_close_refresh_and_genera_2807f5ec40",
              ),
            )
          : cause instanceof ApiError && cause.status === 400
            ? text(
                ...translatedMessage(
                  "the_server_rejected_this_review_close_check_c_5fe5a7a804",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_submission_outcome_is_unconfirmed_check_i_f6fa918f0f",
                ),
              ),
      );
    } finally {
      inFlight.current = false;
    }
  };
  const check = async () => {
    if (!captured || inFlight.current) return;
    inFlight.current = true;
    setChecking(true);
    setError("");
    try {
      const receipt = await getPublicationReceipt(captured.review_digest);
      if (!receiptMatchesReview(receipt, captured))
        throw Error("Receipt does not match review");
      if (!mounted.current) return;
      setAck(receipt);
      setHistorical(true);
      setCaptured(undefined);
      onCommitted();
    } catch (cause) {
      if (mounted.current)
        setError(
          cause instanceof ApiError && cause.status === 404
            ? text(
                ...translatedMessage(
                  "no_submission_record_was_found_yet_close_refr_b63c224422",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_submission_record_could_not_be_verified_c_7e20ce5de9",
                ),
              ),
        );
    } finally {
      inFlight.current = false;
      if (mounted.current) setChecking(false);
    }
  };
  return (
    <>
      {ack && (
        <div className="rp-result-status" role="status">
          <strong>
            {historical
              ? text(
                  ...translatedMessage(
                    "historical_submission_verified_b2624aada8",
                  ),
                )
              : text(
                  ...translatedMessage(
                    "publication_committed_by_the_server_1944f41a27",
                  ),
                )}
          </strong>
          <p>
            {text(...translatedMessage("submission_record_0c4c5312f8"))}{" "}
            <code dir="ltr">{ack.publication_id}</code> · {ack.binding_count}{" "}
            {text(...translatedMessage("bindings_4aa0f48b55"))} ·{" "}
            {ack.route_count} {text(...translatedMessage("pairs_16421ff278"))}
          </p>
          <p>
            {text(...translatedMessage("resources_dd59219f36"))} v
            {ack.resource_revision} ·{" "}
            {text(...translatedMessage("channels_40b918ebad"))} v
            {ack.channel_revision} ·{" "}
            {text(...translatedMessage("configuration_2034cf5638"))} v
            {ack.option_version}
          </p>
          <p>
            {text(
              ...translatedMessage(
                "this_receipt_proves_a_historical_commit_refre_135c02b7a3",
              ),
            )}
          </p>
        </div>
      )}
      {review?.ready && (
        <Button
          variant="primary"
          disabled={disabled || review.publish_available !== true || pending}
          onClick={() => {
            setCaptured(review);
            setAttempted(false);
            setError("");
            setAck(undefined);
            setHistorical(false);
          }}
        >
          {text(...translatedMessage("publish_this_review_bb78390619"))}
        </Button>
      )}
      <Modal
        open={Boolean(captured)}
        onOpenChange={(open) => {
          if (!open && !inFlight.current && !pending) setCaptured(undefined);
        }}
        size="wide"
        className="rp-publish-dialog"
        preventOutsideClose
        title={text(
          ...translatedMessage(
            "confirm_route_replacement_for_selected_channe_d218882823",
          ),
        )}
        description={text(
          ...translatedMessage(
            "only_explicit_confirmation_submits_this_serve_291abbd781",
          ),
        )}
        footer={
          <>
            <Button disabled={pending} onClick={() => setCaptured(undefined)}>
              {text(...translatedMessage("close_6bf477426b"))}
            </Button>
            {attempted && (
              <Button
                loading={checking}
                disabled={publish.isPending}
                onClick={() => void check()}
              >
                {text(
                  ...translatedMessage("check_submission_record_a6065b3dfb"),
                )}
              </Button>
            )}
            <Button
              variant="primary"
              loading={publish.isPending}
              disabled={disabled || attempted || checking}
              onClick={() => void confirm()}
            >
              {text(...translatedMessage("confirm_publication_5d7b49f8ab"))}
            </Button>
          </>
        }
      >
        {captured && (
          <div className="rp-publish-content">
            {error && (
              <p role="alert" tabIndex={-1} ref={errorElement}>
                {error}
              </p>
            )}
            <p>
              {text(
                ...translatedMessage(
                  "this_replaces_all_model_group_grants_on_selec_a5bd2ec261",
                ),
              )}
            </p>
            <p>
              {text(...translatedMessage("current_ability_pairs_39c462f95d"))}:{" "}
              {captured.existing_pairs.length} →{" "}
              {text(...translatedMessage("proposed_pairs_91c410b11d"))}:{" "}
              {captured.proposed_routes.length}
            </p>
            <ul>
              {captured.bindings.map((binding) => (
                <li key={binding.source_channel_id}>
                  <code dir="ltr">
                    {binding.source_channel_id} → {binding.gateway_channel_id}
                  </code>
                </li>
              ))}
            </ul>
            <p>
              {text(
                ...translatedMessage(
                  "only_the_listed_model_group_pairs_provider_mo_8078deb1c8",
                ),
              )}
            </p>
            <p>
              {text(
                ...translatedMessage(
                  "this_affects_future_requests_already_admitted_aa66960a5a",
                ),
              )}
            </p>
            <p className="rp-versions">
              {text(...translatedMessage("resources_dd59219f36"))} v
              {captured.resource_revision} ·{" "}
              {text(...translatedMessage("channels_40b918ebad"))} v
              {captured.channel_revision} ·{" "}
              {text(...translatedMessage("configuration_2034cf5638"))} v
              {captured.option_version}
            </p>
            {renderRoutes(captured)}
          </div>
        )}
      </Modal>
    </>
  );
}
