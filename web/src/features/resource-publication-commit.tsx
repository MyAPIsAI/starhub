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
              "预览已过期。关闭后刷新并重新生成；原选择已保留。",
              "This review is stale. Close, refresh and generate again; selections are retained.",
              "انتهت صلاحية المعاينة. أغلق وحدّث وأنشئها مجددًا؛ تُحفظ الاختيارات.",
            )
          : cause instanceof ApiError && cause.status === 400
            ? text(
                "服务器拒绝了这份预览。关闭后核对配置并重新生成。",
                "The server rejected this review. Close, check configuration and generate again.",
                "رفض الخادم هذه المعاينة. أغلق وراجع الإعدادات وأنشئها مجددًا.",
              )
            : text(
                "尚未确认提交结果。请核对提交记录，不要重复发送。",
                "The submission outcome is unconfirmed. Check its record before any new publication.",
                "لم تُؤكد نتيجة الإرسال. تحقق من السجل قبل أي نشر جديد.",
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
                "暂未找到这份预览的提交记录。关闭后刷新并重新核对状态；这不证明之前的请求一定失败。",
                "No submission record was found yet. Close, refresh and review current state; this does not prove the earlier request failed.",
                "لم يُعثر على سجل إرسال بعد. أغلق وحدّث وراجع الحالة؛ هذا لا يثبت فشل الطلب السابق.",
              )
            : text(
                "无法核对提交记录。请稍后再次查询或刷新当前状态。",
                "The submission record could not be verified. Check again later or refresh current state.",
                "تعذر التحقق من سجل الإرسال. أعد التحقق لاحقًا أو حدّث الحالة الحالية.",
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
                  "已核对历史提交",
                  "Historical submission verified",
                  "تم التحقق من الإرسال التاريخي",
                )
              : text(
                  "服务器已提交发布",
                  "Publication committed by the server",
                  "اعتمد الخادم النشر",
                )}
          </strong>
          <p>
            {text("提交记录", "Submission record", "سجل الإرسال")}{" "}
            <code dir="ltr">{ack.publication_id}</code> · {ack.binding_count}{" "}
            {text("绑定", "bindings", "روابط")} · {ack.route_count}{" "}
            {text("组合", "pairs", "أزواج")}
          </p>
          <p>
            {text("资源", "Resources", "الموارد")} v{ack.resource_revision} ·{" "}
            {text("渠道", "Channels", "القنوات")} v{ack.channel_revision} ·{" "}
            {text("配置", "Configuration", "الإعدادات")} v{ack.option_version}
          </p>
          <p>
            {text(
              "收据仅证明这次历史提交。请刷新核对当前绑定和可用性；不代表上游或真实客户端验收。",
              "This receipt proves a historical commit. Refresh to check current bindings and availability; it does not verify an upstream or real client.",
              "يثبت الإيصال اعتمادًا تاريخيًا. حدّث للتحقق من الروابط والتوافر الحاليين؛ لا يثبت اختبار مزود أو عميل فعلي.",
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
          {text("发布这份预览", "Publish this review", "نشر هذه المعاينة")}
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
          "确认替换所选渠道的路由",
          "Confirm route replacement for selected channels",
          "تأكيد استبدال مسارات القنوات المحددة",
        )}
        description={text(
          "仅在明确确认后提交这份服务器预览。",
          "Only explicit confirmation submits this server review.",
          "لا تُرسل معاينة الخادم إلا بعد التأكيد الصريح.",
        )}
        footer={
          <>
            <Button disabled={pending} onClick={() => setCaptured(undefined)}>
              {text("关闭", "Close", "إغلاق")}
            </Button>
            {attempted && (
              <Button
                loading={checking}
                disabled={publish.isPending}
                onClick={() => void check()}
              >
                {text(
                  "核对提交记录",
                  "Check submission record",
                  "تحقق من سجل الإرسال",
                )}
              </Button>
            )}
            <Button
              variant="primary"
              loading={publish.isPending}
              disabled={disabled || attempted || checking}
              onClick={() => void confirm()}
            >
              {text("确认发布", "Confirm publication", "تأكيد النشر")}
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
                "将替换所选真实渠道的全部模型/分组授权，不保留未列出的旧组合。",
                "This replaces all model/group grants on selected gateway channels. Unlisted existing pairs will be removed.",
                "يستبدل هذا جميع صلاحيات النماذج والمجموعات للقنوات المحددة. ستُزال الأزواج الحالية غير المدرجة.",
              )}
            </p>
            <p>
              {text(
                "当前能力组合",
                "Current ability pairs",
                "أزواج القدرات الحالية",
              )}
              : {captured.existing_pairs.length} →{" "}
              {text("拟发布组合", "Proposed pairs", "الأزواج المقترحة")}:{" "}
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
                "仅支持基础 Chat/Anthropic 文本，按所列能力与上下文约束接入。工具、多模态及其他端点不在本次发布范围。",
                "Only basic Chat/Anthropic text is supported under the listed capability and context constraints. Tools, multimodal requests and other endpoints are outside this publication profile.",
                "يُدعم النص الأساسي عبر Chat/Anthropic فقط ضمن قيود القدرات والسياق المدرجة. الأدوات والطلبات متعددة الوسائط ونقاط النهاية الأخرى خارج هذا النطاق.",
              )}
            </p>
            <p>
              {text(
                "只影响后续请求；已准入调用及历史记录保持原口径。密钥授权范围、凭证、价格和资金不由此修改。",
                "This affects future requests. Already admitted calls and historical records retain their existing terms. Token scopes, credentials, prices and funds are unchanged.",
                "يؤثر هذا في الطلبات اللاحقة. تحتفظ الطلبات المقبولة والسجلات التاريخية بشروطها. لا تتغير نطاقات المفاتيح أو بيانات الاعتماد أو الأسعار أو الأموال.",
              )}
            </p>
            <p className="rp-versions">
              {text("资源", "Resources", "الموارد")} v
              {captured.resource_revision} ·{" "}
              {text("渠道", "Channels", "القنوات")} v{captured.channel_revision}{" "}
              · {text("配置", "Configuration", "الإعدادات")} v
              {captured.option_version}
            </p>
            {renderRoutes(captured)}
          </div>
        )}
      </Modal>
    </>
  );
}
