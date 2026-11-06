import { message as translatedMessage } from "../i18n/messages";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "../components/ui/overlay";
import { Button } from "../components/ui/button";
import {
  useCloseAccount,
  useDeactivationState,
  type CloseInput,
} from "../lib/api/deactivation";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { formatMoney } from "../lib/money";
export default function AccountDeactivation({
  onClose,
}: {
  onClose: () => void;
}) {
  const { i18n } = useTranslation(),
    scope = useSessionQueryIdentity(),
    query = useDeactivationState(),
    close = useCloseAccount(),
    recover = useCloseAccount(true);
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [ack, setAck] = useState(false),
    [error, setError] = useState(""),
    [uncertain, setUncertain] = useState(false),
    [busy, setBusy] = useState(false),
    pending = useRef(false),
    intent = useRef<CloseInput | null>(null);
  const state = query.isError ? undefined : query.data;
  const current = () =>
    JSON.stringify(scope) ===
    JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()));
  async function submit(readback = false) {
    if (pending.current || !current()) return;
    if (!readback) {
      if (
        !state ||
        !state.guarded_deactivation ||
        state.blocked_reasons.length ||
        uncertain
      )
        return;
      intent.current = {
        schema: "deactivation_action_v1",
        expected_user_id: state.user_id,
        account_revision: state.account_revision,
        operation_id: crypto.randomUUID().replaceAll("-", ""),
        acknowledge_dust: ack,
        expected_writeoff_nano: state.wallet_balance_nano,
      };
    }
    if (!intent.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const receipt = await (readback ? recover : close).mutateAsync(
        intent.current,
      );
      if (current() && receipt.closed === true) window.location.assign("/");
    } catch {
      if (current()) {
        setUncertain(true);
        setError(
          c(
            ...translatedMessage(
              "account_closure_is_unconfirmed_check_this_ope_f64c29057b",
            ),
          ),
        );
      }
    } finally {
      pending.current = false;
      if (current()) setBusy(false);
    }
  }
  return (
    <Modal
      open
      onOpenChange={(open) => {
        if (!open && !pending.current) onClose();
      }}
      size="compact"
      preventOutsideClose
      title={c(...translatedMessage("close_this_account_a702568233"))}
      description={c(
        ...translatedMessage(
          "confirmation_disables_the_account_and_all_key_712235fdbe",
        ),
      )}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            {c(...translatedMessage("cancel_0828e4ce08"))}
          </Button>
          {uncertain ? (
            <Button disabled={busy} onClick={() => void submit(true)}>
              {c(...translatedMessage("check_closure_receipt_f22673f5bd"))}
            </Button>
          ) : (
            <Button
              variant="danger"
              disabled={
                busy ||
                !state?.guarded_deactivation ||
                query.isError ||
                !!state?.blocked_reasons.length ||
                (state?.requires_dust_acknowledgement && !ack)
              }
              onClick={() => void submit()}
            >
              {c(...translatedMessage("close_account_962bef43df"))}
            </Button>
          )}
        </>
      }
    >
      <div className="key-task-close-account">
        <div className="key-task-account-balance">
          <span>{c(...translatedMessage("current_balance_567932fc55"))}</span>
          <strong dir="ltr">
            {state ? formatMoney(state.wallet_balance_nano, "detail") : "—"}
          </strong>
        </div>
        {query.isPending && (
          <p role="status">
            {c(...translatedMessage("loading_closure_conditions_c561b57c4f"))}
          </p>
        )}
        {(!state || !state.guarded_deactivation) && !query.isPending && (
          <p role="alert">
            {c(
              ...translatedMessage(
                "guarded_account_closure_is_unavailable_reload_6c19d63169",
              ),
            )}
          </p>
        )}
        {state?.blocked_reasons.map((reason) => (
          <p role="status" key={reason}>
            {reason === "pending_recharges"
              ? c(
                  ...translatedMessage(
                    "resolve_unfinished_top_up_orders_first_c437931596",
                  ),
                )
              : reason === "last_active_root"
                ? c(
                    ...translatedMessage(
                      "the_last_active_root_account_cannot_be_closed_57382b2a14",
                    ),
                  )
                : c(
                    ...translatedMessage(
                      "resolve_the_wallet_balance_before_closing_the_96c531d194",
                    ),
                  )}
          </p>
        ))}
        {state?.requires_dust_acknowledgement && (
          <label className="key-task-dust-ack">
            <input
              type="checkbox"
              checked={ack}
              disabled={busy || uncertain}
              onChange={(e) => setAck(e.target.checked)}
            />
            <span>
              {c(
                ...translatedMessage(
                  "i_agree_to_forfeit_the_balance_below_0_01_clo_2d806bfede",
                ),
              )}
            </span>
          </label>
        )}
        {error && <p role="alert">{error}</p>}
        {!uncertain && (
          <Button
            disabled={busy || query.isFetching}
            onClick={() => {
              setAck(false);
              void query.refetch();
            }}
          >
            {c(...translatedMessage("reload_closure_conditions_68e0cb3115"))}
          </Button>
        )}
      </div>
    </Modal>
  );
}
