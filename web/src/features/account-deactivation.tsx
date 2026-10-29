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
            "未能确认注销结果。请查询本次收据；未查到收据不代表操作失败。",
            "Account closure is unconfirmed. Check this operation's receipt; a missing receipt does not prove failure.",
            "إغلاق الحساب غير مؤكد. تحقق من إيصال العملية؛ عدم العثور عليه لا يثبت فشلها.",
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
      title={c("确认注销账户", "Close this account", "تأكيد إغلاق الحساب")}
      description={c(
        "确认后将停用账号及所有密钥，并撤销登录会话。已受理的请求仍按原计费流程结算。",
        "Confirmation disables the account and all keys and revokes login sessions. Already admitted requests retain their existing settlement flow.",
        "يؤدي التأكيد إلى تعطيل الحساب والمفاتيح وإلغاء جلسات الدخول. تُسوّى الطلبات المقبولة سابقًا وفق مسارها الحالي.",
      )}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            {c("取消", "Cancel", "إلغاء")}
          </Button>
          {uncertain ? (
            <Button disabled={busy} onClick={() => void submit(true)}>
              {c(
                "查询本次注销收据",
                "Check closure receipt",
                "التحقق من إيصال الإغلاق",
              )}
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
              {c("确认注销", "Close account", "إغلاق الحساب")}
            </Button>
          )}
        </>
      }
    >
      <div className="key-task-close-account">
        <div className="key-task-account-balance">
          <span>{c("当前余额", "Current balance", "الرصيد الحالي")}</span>
          <strong dir="ltr">
            {state ? formatMoney(state.wallet_balance_nano, "detail") : "—"}
          </strong>
        </div>
        {query.isPending && (
          <p role="status">
            {c(
              "正在读取注销条件…",
              "Loading closure conditions…",
              "جارٍ تحميل شروط الإغلاق…",
            )}
          </p>
        )}
        {(!state || !state.guarded_deactivation) && !query.isPending && (
          <p role="alert">
            {c(
              "无法确认受保护的注销能力，请重新读取。",
              "Guarded account closure is unavailable. Reload the preview.",
              "إغلاق الحساب المحمي غير متاح. أعد تحميل المعاينة.",
            )}
          </p>
        )}
        {state?.blocked_reasons.map((reason) => (
          <p role="status" key={reason}>
            {reason === "pending_recharges"
              ? c(
                  "仍有未完成的充值订单，请先处理订单。",
                  "Resolve unfinished top-up orders first.",
                  "عالج طلبات الشحن غير المكتملة أولاً.",
                )
              : reason === "last_active_root"
                ? c(
                    "不能注销最后一个有效超级管理员。",
                    "The last active Root account cannot be closed.",
                    "لا يمكن إغلاق آخر حساب مسؤول أعلى نشط.",
                  )
                : c(
                    "余额尚不满足注销条件，请先处理余额。",
                    "Resolve the wallet balance before closing the account.",
                    "عالج رصيد المحفظة قبل إغلاق الحساب.",
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
                "我确认放弃以上不足 0.01 美元的余额；注销时将清零并记录流水。",
                "I agree to forfeit the balance below $0.01. Closure records the exact write-off.",
                "أوافق على التنازل عن الرصيد الأقل من 0.01 دولار مع تسجيل المبلغ الدقيق.",
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
            {c(
              "重新读取注销条件",
              "Reload closure conditions",
              "إعادة تحميل شروط الإغلاق",
            )}
          </Button>
        )}
      </div>
    </Modal>
  );
}
