import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import {
  getBindingChangeReceipt,
  refreshPublicationQueries,
  useChangeResourceBinding,
} from "../lib/api/resource-publication";
import { ApiError } from "../lib/api/http";
import {
  bindingReceiptMatchesReview,
  type BindingChangeReceipt,
  type BindingChangeReview,
} from "../lib/resource-binding-change";
import { useSettingsText } from "./settings-text";

export function ResourceBindingChangeDialog({
  review,
  onClose,
  onCommitted,
}: {
  review: BindingChangeReview | undefined;
  onClose: () => void;
  onCommitted: (receipt: BindingChangeReceipt) => void;
}) {
  const { text } = useSettingsText();
  const mutation = useChangeResourceBinding();
  const client = useQueryClient();
  const [attempted, setAttempted] = useState(false);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true),
    pending = useRef(false),
    errorElement = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    if (error) errorElement.current?.focus();
  }, [error]);
  const confirm = async () => {
    if (!review || pending.current || attempted) return;
    pending.current = true;
    setAttempted(true);
    setError("");
    try {
      const result = await mutation.mutateAsync(review);
      if (mounted.current) onCommitted(result);
    } catch (cause) {
      if (mounted.current)
        setError(
          cause instanceof ApiError && cause.status === 409
            ? text(
                ...translatedMessage(
                  "the_binding_or_versions_changed_close_and_ref_b963a6ed0a",
                ),
              )
            : cause instanceof ApiError && cause.status === 400
              ? text(
                  ...translatedMessage(
                    "the_server_rejected_this_action_close_check_t_65adc1286e",
                  ),
                )
              : text(
                  ...translatedMessage(
                    "the_action_outcome_is_unconfirmed_check_its_r_4d394a79e5",
                  ),
                ),
        );
    } finally {
      pending.current = false;
    }
  };
  const check = async () => {
    if (!review || pending.current) return;
    pending.current = true;
    setChecking(true);
    setError("");
    try {
      const receipt = await getBindingChangeReceipt(review.request_id);
      if (!bindingReceiptMatchesReview(receipt, review))
        throw Error("Binding receipt does not match review");
      await refreshPublicationQueries(client);
      if (mounted.current) onCommitted(receipt);
    } catch (cause) {
      if (mounted.current)
        setError(
          cause instanceof ApiError && cause.status === 404
            ? text(
                ...translatedMessage(
                  "no_action_record_was_found_yet_this_does_not__5e164cc023",
                ),
              )
            : text(
                ...translatedMessage(
                  "the_action_record_could_not_be_verified_check_162686485c",
                ),
              ),
        );
    } finally {
      pending.current = false;
      if (mounted.current) setChecking(false);
    }
  };
  return (
    <Modal
      open={Boolean(review)}
      onOpenChange={(open) => {
        if (!open && !pending.current) onClose();
      }}
      title={
        review?.operation === "release"
          ? text(
              ...translatedMessage("confirm_source_binding_release_8732662172"),
            )
          : text(
              ...translatedMessage(
                "confirm_managed_route_revocation_514159cfde",
              ),
            )
      }
      description={text(
        ...translatedMessage(
          "check_the_source_target_and_captured_versions_153f658686",
        ),
      )}
      className="rp-publish-dialog"
      preventOutsideClose
      footer={
        <>
          <Button disabled={mutation.isPending || checking} onClick={onClose}>
            {text(...translatedMessage("close_6bf477426b"))}
          </Button>
          {attempted && (
            <Button
              loading={checking}
              disabled={mutation.isPending}
              onClick={() => void check()}
            >
              {text(...translatedMessage("check_action_record_43ad8024cc"))}
            </Button>
          )}
          <Button
            variant="danger"
            loading={mutation.isPending}
            disabled={attempted || checking}
            onClick={() => void confirm()}
          >
            {review?.operation === "release"
              ? text(...translatedMessage("confirm_binding_release_e8dcdb5b95"))
              : text(
                  ...translatedMessage("confirm_route_revocation_b07278bf3f"),
                )}
          </Button>
        </>
      }
    >
      {review && (
        <div className="rp-publish-content">
          {error && (
            <p role="alert" tabIndex={-1} ref={errorElement}>
              {error}
            </p>
          )}
          <p>
            <strong>
              {review.row.gateway_name ||
                text(
                  ...translatedMessage("deleted_or_unnamed_gateway_75592ec3f3"),
                )}
            </strong>
            <br />
            <code dir="ltr">
              {review.row.source_channel_id} → {review.row.gateway_channel_id}
            </code>
          </p>
          <p>
            {review.operation === "release"
              ? text(
                  ...translatedMessage(
                    "clear_all_managed_routes_and_abilities_on_thi_55b16a3ed1",
                  ),
                )
              : text(
                  ...translatedMessage(
                    "clear_all_managed_routes_and_abilities_on_thi_49e416f3a1",
                  ),
                )}
          </p>
          <p>
            {text(...translatedMessage("stored_pairs_b8a06ed7f7"))}:{" "}
            {review.row.stored_pair_count} → 0 ·{" "}
            {text(...translatedMessage("last_publication_record_59e2458727"))}:{" "}
            <code dir="ltr">{review.row.publication_id}</code>
          </p>
          <p>
            {text(
              ...translatedMessage(
                "already_admitted_calls_and_history_are_unchan_d77d4c0830",
              ),
            )}
          </p>
          <p className="rp-versions">
            {text(...translatedMessage("resources_dd59219f36"))} v
            {review.resource_revision} ·{" "}
            {text(...translatedMessage("channels_40b918ebad"))} v
            {review.channel_revision}
          </p>
          <p>
            {text(...translatedMessage("request_id_d598531376"))}:{" "}
            <code dir="ltr">{review.request_id}</code>
          </p>
        </div>
      )}
    </Modal>
  );
}
