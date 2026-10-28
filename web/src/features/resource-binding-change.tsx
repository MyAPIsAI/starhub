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
                "绑定或版本已变化。关闭后刷新，核对新状态再操作；本次草稿已保留。",
                "The binding or versions changed. Close and refresh before reviewing a new action; this draft is retained.",
                "تغير الرابط أو إصداراته. أغلق وحدّث قبل مراجعة إجراء جديد؛ تُحفظ المسودة الحالية.",
              )
            : cause instanceof ApiError && cause.status === 400
              ? text(
                  "服务器拒绝此操作。关闭后核对当前绑定，再重新确认。",
                  "The server rejected this action. Close, check the current binding and review again.",
                  "رفض الخادم هذا الإجراء. أغلق وتحقق من الرابط الحالي وراجعه مجددًا.",
                )
              : text(
                  "尚未确认操作结果。请核对操作记录，暂勿重复提交。",
                  "The action outcome is unconfirmed. Check its record before submitting again.",
                  "لم تُؤكد نتيجة الإجراء. تحقق من سجله قبل الإرسال مجددًا.",
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
                "暂未找到操作记录。这不证明之前的请求一定失败；请稍后核对或关闭后刷新当前状态。",
                "No action record was found yet. This does not prove the earlier request failed; check later or close and refresh current state.",
                "لم يُعثر على سجل للإجراء بعد. هذا لا يثبت فشل الطلب السابق؛ تحقق لاحقًا أو أغلق وحدّث الحالة.",
              )
            : text(
                "无法核对操作记录，请稍后再查或刷新当前状态。",
                "The action record could not be verified. Check again later or refresh current state.",
                "تعذر التحقق من سجل الإجراء. أعد التحقق لاحقًا أو حدّث الحالة الحالية.",
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
              "确认释放来源绑定",
              "Confirm source binding release",
              "تأكيد تحرير رابط المصدر",
            )
          : text(
              "确认撤销托管路由",
              "Confirm managed route revocation",
              "تأكيد إلغاء المسارات المُدارة",
            )
      }
      description={text(
        "请核对来源、目标和冻结版本后确认。",
        "Check the source, target and captured versions before confirming.",
        "تحقق من المصدر والهدف والإصدارات المسجلة قبل التأكيد.",
      )}
      className="rp-publish-dialog"
      preventOutsideClose
      footer={
        <>
          <Button disabled={mutation.isPending || checking} onClick={onClose}>
            {text("关闭", "Close", "إغلاق")}
          </Button>
          {attempted && (
            <Button
              loading={checking}
              disabled={mutation.isPending}
              onClick={() => void check()}
            >
              {text(
                "核对操作记录",
                "Check action record",
                "تحقق من سجل الإجراء",
              )}
            </Button>
          )}
          <Button
            variant="danger"
            loading={mutation.isPending}
            disabled={attempted || checking}
            onClick={() => void confirm()}
          >
            {review?.operation === "release"
              ? text(
                  "确认释放绑定",
                  "Confirm binding release",
                  "تأكيد تحرير الرابط",
                )
              : text(
                  "确认撤销路由",
                  "Confirm route revocation",
                  "تأكيد إلغاء المسارات",
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
                  "已删除或未命名渠道",
                  "Deleted or unnamed gateway",
                  "بوابة محذوفة أو بلا اسم",
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
                  "清除该渠道的全部托管路线及能力，并释放来源归属，允许之后显式绑定其他渠道。目标渠道保留保护记录，不恢复旧的模型/分组权限。",
                  "Clear all managed routes and abilities on this channel and release source ownership for a later explicit binding elsewhere. The target retains a protective marker; legacy model/group grants do not return.",
                  "يمسح هذا جميع المسارات والقدرات المُدارة للقناة ويحرر ملكية المصدر لربط صريح لاحق في موضع آخر. يحتفظ الهدف بعلامة وقائية ولا تعود صلاحيات النماذج والمجموعات القديمة.",
                )
              : text(
                  "清除该渠道的全部托管路线及能力；保留来源归属。后续请求将不能使用这些路线，恢复须重新预览并明确发布。",
                  "Clear all managed routes and abilities on this channel while retaining source ownership. Future requests cannot use these routes; restoration requires a new review and explicit publication.",
                  "يمسح هذا جميع المسارات والقدرات المُدارة للقناة مع الاحتفاظ بملكية المصدر. لا يمكن للطلبات اللاحقة استخدام هذه المسارات؛ تتطلب الاستعادة مراجعة ونشرًا صريحًا جديدين.",
                )}
          </p>
          <p>
            {text("已存组合", "Stored pairs", "الأزواج المخزنة")}:{" "}
            {review.row.stored_pair_count} → 0 ·{" "}
            {text("最近发布记录", "Last publication record", "آخر سجل نشر")}:{" "}
            <code dir="ltr">{review.row.publication_id}</code>
          </p>
          <p>
            {text(
              "已准入调用及历史记录不回溯修改；凭证、密钥授权范围、价格及资金不变。收据只证明历史提交，当前状态应刷新核对。",
              "Already admitted calls and history are unchanged. Credentials, token scopes, prices and funds remain unchanged. A receipt proves a historical commit; refresh to verify current state.",
              "لا تتغير الطلبات المقبولة مسبقًا أو السجلات. تبقى بيانات الاعتماد ونطاقات المفاتيح والأسعار والأموال كما هي. يثبت الإيصال اعتمادًا تاريخيًا؛ حدّث للتحقق من الحالة الحالية.",
            )}
          </p>
          <p className="rp-versions">
            {text("资源", "Resources", "الموارد")} v{review.resource_revision} ·{" "}
            {text("渠道", "Channels", "القنوات")} v{review.channel_revision}
          </p>
          <p>
            {text("请求标识", "Request ID", "معرّف الطلب")}:{" "}
            <code dir="ltr">{review.request_id}</code>
          </p>
        </div>
      )}
    </Modal>
  );
}
