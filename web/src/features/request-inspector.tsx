import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  Copy,
  Plus,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Drawer } from "../components/ui/overlay";
import { TierBadge } from "../components/tier-badge";
import { formatMoney } from "../lib/money";
import { useProductPreview, type PreviewRequest } from "../lib/product-preview";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useLedger, adaptLedgerEntry } from "../lib/api/billing";
import { copyText } from "./code-example";
import "../components/demo-scenarios.css";
import { PricingSnapshot } from "../components/pricing-snapshot";
import { useSystemAccess, isAdmin } from "../lib/system-access";

const errors: Record<string, [string, string, string]> = {
  insufficient_balance: [
    "账户余额不足。本次请求未执行，请先完成模拟充值后重试。",
    "The balance is insufficient. This request was not run. Complete a simulated top-up, then retry.",
    "الرصيد غير كافٍ. لم ينفذ الطلب. أضف رصيداً محاكياً ثم أعد المحاولة.",
  ],
  key_quota_exceeded: [
    "本密钥剩余额度不足以承担本次请求的最大预估费用。降低最大输出词元、调整额度或选择另一把兼容密钥后重试。",
    "This request’s maximum estimated cost exceeds the key’s remaining budget. Reduce max output tokens, adjust the limit or use another compatible key.",
    "تتجاوز التكلفة القصوى المتوقعة الميزانية المتبقية للمفتاح. قلل حد الإخراج أو عدّل الميزانية أو اختر مفتاحاً آخر.",
  ],
  model_unauthorized: [
    "本密钥未授权此模型。选择已授权模型，或在密钥管理中明确修改白名单。",
    "This model is not authorized for the key. Select an authorized model or explicitly update its allowlist.",
    "النموذج غير مصرح لهذا المفتاح. اختر نموذجاً مصرحاً أو عدّل القائمة صراحةً.",
  ],
  protocol_incompatible: [
    "请求协议与模型不兼容。返回接入中心，选择适配目标工具的模型与地址。",
    "The protocol is incompatible with this model. Select a matching model and endpoint in the connection center.",
    "البروتوكول غير متوافق مع النموذج. اختر نموذجاً وعنواناً متوافقين في مركز الاتصال.",
  ],
  capability_incompatible: [
    "该模型或智能路由候选不支持本次请求能力。保留输入，调整模型或请求参数后重试。",
    "No authorized model supports the requested capability. Keep your input and adjust the model or parameters.",
    "لا يدعم نموذج مصرح قدرات الطلب. احتفظ بالمدخلات وعدّل النموذج أو المعلمات.",
  ],
  tool_chain_incompatible: [
    "工具调用链已固定到兼容模型，当前请求与该上下文不匹配。请恢复原模型或开始新的调用链。",
    "This tool chain is pinned to a compatible model. Restore that model or start a new chain.",
    "سلسلة الأدوات مرتبطة بنموذج متوافق. استعده أو ابدأ سلسلة جديدة.",
  ],
  unknown_model: [
    "请求的模型标识不存在。平台未替换模型，请检查标识后重试。",
    "The requested model ID does not exist. No substitute was used; check the ID.",
    "معرّف النموذج غير موجود. لم يُستخدم بديل؛ تحقق من المعرّف.",
  ],
  model_unavailable: [
    "此模型已下架或未向当前服务分组开放。请选择当前可用模型。",
    "This model is offline or unavailable in this service group. Choose an available model.",
    "النموذج غير متاح أو غير مفتوح لهذه المجموعة. اختر نموذجاً متاحاً.",
  ],
  ip_unauthorized: [
    "当前模拟来源地址不在密钥允许范围内。请在密钥管理中检查来源地址白名单。",
    "The simulated source address is outside the key's allowed range. Check its IP allowlist in key management.",
    "عنوان المصدر المحاكى خارج النطاق المسموح للمفتاح. راجع قائمة العناوين في إدارة المفاتيح.",
  ],
  key_disabled: [
    "密钥已停用。启用密钥或选择其他兼容密钥后重试。",
    "This key is disabled. Enable it or use another compatible key.",
    "المفتاح معطل. فعّله أو استخدم مفتاحاً متوافقاً آخر.",
  ],
  key_expired: [
    "密钥已过期。更新有效期或选择其他兼容密钥后重试。",
    "This key has expired. Update its validity or use another compatible key.",
    "انتهت صلاحية المفتاح. حدّث الصلاحية أو اختر مفتاحاً آخر.",
  ],
  key_not_found: [
    "密钥不存在，请选择一把有效密钥。",
    "The key was not found. Select a valid key.",
    "المفتاح غير موجود. اختر مفتاحاً صالحاً.",
  ],
  upstream_unavailable: [
    "模拟上游故障，请求未完成。关闭演示故障后可用原输入重试，平台不会静默改用其他模型。",
    "The simulated upstream failed. Restore the demo upstream and retry with your original input; no model was silently substituted.",
    "فشل المزوّد المحاكى. استعد حالته وأعد المحاولة بالمدخلات الأصلية؛ لم يُستبدل النموذج تلقائياً.",
  ],
  account_unavailable: [
    "当前账户状态不允许调用。恢复演示账户后重试。",
    "The account cannot make requests in its current state. Restore the demo account before retrying.",
    "حالة الحساب تمنع الطلبات. استعد الحساب التجريبي قبل إعادة المحاولة.",
  ],
  invalid_request: [
    "请求参数无效。保留输入并修正标记的参数后重试。",
    "Request parameters are invalid. Preserve your input and correct the parameters.",
    "معلمات الطلب غير صالحة. احتفظ بالمدخلات وصحح المعلمات.",
  ],
  pricing_invalid: [
    "计价规则或所需用量无效，本次未计费。保留输入，检查计费单位、表达式变量与费率后重试。",
    "The pricing rule or required usage is invalid. No charge was made. Retain the input and correct the billing unit, variables or rates.",
    "قاعدة التسعير أو الاستخدام المطلوب غير صالح. لم تحتسب رسوم. احتفظ بالمدخلات وصحح الوحدة أو المتغيرات أو الأسعار.",
  ],
  stream_interrupted: [
    "模拟输出中断，请检查已记录的用量与计费金额后重试。",
    "The simulated stream was interrupted. Check recorded usage and charges before retrying.",
    "انقطع الإخراج المحاكى. تحقق من الاستخدام والتكلفة المسجلين قبل إعادة المحاولة.",
  ],
};

export default function RequestInspector({
  record,
  onClose,
  onRetry,
}: {
  record: PreviewRequest | null;
  onClose: () => void;
  onRetry: () => void;
}) {
  const { i18n, t } = useTranslation();
  const lang = i18n.language.startsWith("zh")
    ? "zh"
    : i18n.language.startsWith("ar")
      ? "ar"
      : "en";
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const { keys } = useBackendKeys();
  const ledgerQuery = useLedger();
  const ledger = (ledgerQuery.data ?? []).map(adaptLedgerEntry) as any[];
  const setOutage = useProductPreview((s) => s.demoSetUpstreamOutage);
  const role = useSystemAccess((s) => s.role);
  // M6 §3.5：详情抽屉不向客户显示渠道或资源实例。仅运营视角（非 customer）可见内部来源标识。
  const isOps = isAdmin(role);
  const failed = record?.status !== "success";
  const error = record?.error_code ? errors[record.error_code] : undefined;
  const reason = error
    ? error[lang === "zh" ? 0 : lang === "ar" ? 2 : 1]
    : text(
        record?.error_message ?? "请检查请求配置后重试。",
        "Review the request configuration, then retry.",
        "راجع إعداد الطلب ثم أعد المحاولة.",
      );
  const key = keys.find((item) => item.id === record?.key_id);
  const entry = ledger.find((item) => item.trace_id === record?.trace_id);
  const requestFix = [
    "unknown_model",
    "model_unavailable",
    "capability_incompatible",
    "tool_chain_incompatible",
    "invalid_request",
    "pricing_invalid",
  ].includes(record?.error_code ?? "");
  const actionPath =
    record?.error_code === "insufficient_balance"
      ? "/console/billing/recharge"
      : record?.error_code === "protocol_incompatible"
        ? "/console/connect"
        : "/console/keys";
  const actionLabel =
    record?.error_code === "insufficient_balance"
      ? text("前往模拟充值", "Simulated top-up", "إضافة رصيد محاكى")
      : record?.error_code === "protocol_incompatible"
        ? text("检查工具配置", "Check configuration", "تحقق من الإعداد")
        : text(
            "管理密钥权限与额度",
            "Manage key access & limits",
            "إدارة صلاحيات المفتاح وحدوده",
          );
  return (
    <Drawer
      open={!!record}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={text("调用明细", "Request details", "تفاصيل الطلب")}
      description={text(
        "模拟调用 · 一笔业务记录对应一次计费",
        "Simulated call · one business record, one charge",
        "طلب محاكى · سجل واحد ورسوم واحدة",
      )}
      footer={
        <>
          <Button
            onClick={() =>
              record &&
              copyText(
                JSON.stringify(
                  {
                    trace_id: record.trace_id,
                    requested_model: record.requested_model ?? record.model,
                    actual_model:
                      record.actual_model ??
                      (record.status === "success" ? record.model : null),
                    key_id: record.key_id,
                    status: record.status,
                    error_code: record.error_code,
                    billing_status: record.billing_status,
                    cost_nano: record.cost_nano,
                    pricing_revision: record.pricing_revision,
                    input_rate_usd: record.input_rate_usd,
                    output_rate_usd: record.output_rate_usd,
                    pricing_snapshot: record.pricing_snapshot,
                    metered_usage: record.metered_usage,
                    ledger_id: entry?.id,
                  },
                  null,
                  2,
                ),
                t("copied"),
                t("copyFailed"),
              )
            }
          >
            <Copy size={14} />
            {text("复制诊断信息", "Copy diagnostics", "نسخ التشخيص")}
          </Button>
          <Button
            variant="primary"
            onClick={onRetry}
            disabled={record?.metered_demo && !isAdmin(role)}
          >
            {text(
              failed ? "保留输入并重试" : "复用请求验证",
              failed ? "Retry with input" : "Reuse request",
              failed ? "إعادة المحاولة بالمدخلات" : "إعادة استخدام الطلب",
            )}
            <ArrowRight size={14} />
          </Button>
        </>
      }
    >
      {record && (
        <>
          <div className={`request-outcome ${failed ? "is-failed" : ""}`}>
            {failed ? <CircleAlert size={20} /> : <CheckCircle2 size={20} />}
            <h3>
              {failed
                ? text(
                    "模拟请求未完成",
                    "Simulated request incomplete",
                    "الطلب المحاكى غير مكتمل",
                  )
                : text(
                    "模拟请求已完成",
                    "Simulated request completed",
                    "اكتمل الطلب المحاكى",
                  )}
            </h3>
            <p>
              {failed
                ? reason
                : text(
                    "结果由本地演示生成，可用于核对接入、权限与计费流程。",
                    "This local demo result verifies configuration, access and billing flows.",
                    "النتيجة محاكاة محلية للتحقق من الاتصال والصلاحيات والفوترة.",
                  )}
            </p>
            {failed && (
              <div className="request-inspector-actions">
                {record.error_code === "upstream_unavailable" ? (
                  <Button
                    onClick={() => {
                      setOutage(false);
                      onRetry();
                    }}
                  >
                    {text(
                      "恢复模拟上游并重试",
                      "Restore upstream & retry",
                      "استعادة المزوّد وإعادة المحاولة",
                    )}
                  </Button>
                ) : requestFix ? (
                  <Button onClick={onRetry}>
                    {text(
                      "保留输入并修改请求",
                      "Edit request with input",
                      "تعديل الطلب مع المدخلات",
                    )}
                  </Button>
                ) : (
                  <Button asChild>
                    <Link to={actionPath} onClick={onClose}>
                      {actionLabel}
                    </Link>
                  </Button>
                )}
              </div>
            )}
          </div>
          <div className="request-charge">
            <span>
              {BigInt(record.cost_nano) > 0n
                ? text("已计费", "Charged", "تمت المحاسبة")
                : record.status === "success"
                  ? text(
                      "已结算 · 零费用",
                      "Settled · zero cost",
                      "تمت التسوية · دون تكلفة",
                    )
                  : text(
                      "未计费 · 余额与额度未扣减",
                      "Not charged · balance and quota unchanged",
                      "دون رسوم · الرصيد والحد لم يتغيرا",
                    )}
            </span>
            <strong className="money">
              {formatMoney(record.cost_nano, "call")}
            </strong>
          </div>
          <dl className="product-detail-list">
            <div>
              <dt>{text("业务记录", "Business record", "سجل العملية")}</dt>
              <dd className="mono" dir="ltr">
                {record.trace_id}
              </dd>
            </div>
            <div>
              <dt>{text("请求模型", "Requested model", "النموذج المطلوب")}</dt>
              <dd className="mono" dir="ltr">
                {record.requested_model ?? record.model}
              </dd>
            </div>
            <div>
              <dt>{text("实际模型", "Actual model", "النموذج الفعلي")}</dt>
              <dd className="mono" dir="ltr">
                {record.actual_model ??
                  (record.status === "success" ? record.model : "—")}
              </dd>
            </div>
            <div>
              <dt>{text("使用密钥", "Key", "المفتاح")}</dt>
              <dd>
                {key?.name ??
                  record.key_id ??
                  text("历史样例", "Historical sample", "عينة سابقة")}
              </dd>
            </div>
            <div>
              <dt>{text("服务分组", "Service group", "مجموعة الخدمة")}</dt>
              <dd>{record.tier ? <TierBadge id={record.tier} /> : "—"}</dd>
            </div>
            {record.pricing_snapshot ? (
              <PricingSnapshot snapshot={record.pricing_snapshot} />
            ) : record.metered_demo ? (
              <div>
                <dt>
                  {text(
                    "模拟用量（未结算）",
                    "Simulated usage (unsettled)",
                    "الاستخدام المحاكى غير المسوى",
                  )}
                </dt>
                <dd>
                  <code style={{ overflowWrap: "anywhere" }}>
                    {JSON.stringify(record.metered_usage)}
                  </code>
                  <small className="request-rate-note">
                    {record.error_message}
                  </small>
                </dd>
              </div>
            ) : (
              <>
                <div>
                  <dt>
                    {text(
                      "输入 / 输出词元",
                      "Input / output tokens",
                      "رموز الإدخال / الإخراج",
                    )}
                  </dt>
                  <dd>
                    {record.input_tokens.toLocaleString()} /{" "}
                    {record.output_tokens.toLocaleString()}
                  </dd>
                </div>
                <div>
                  <dt>
                    {text(
                      "调用时输入 / 输出单价",
                      "Input / output rate at request",
                      "سعر الإدخال / الإخراج وقت الطلب",
                    )}
                  </dt>
                  <dd>
                    {record.input_rate_usd !== undefined &&
                    record.output_rate_usd !== undefined
                      ? `$${record.input_rate_usd} / $${record.output_rate_usd}`
                      : record.input_price_nano !== undefined &&
                          record.output_price_nano !== undefined
                        ? `${formatMoney(record.input_price_nano, "detail")} / ${formatMoney(record.output_price_nano, "detail")}`
                        : text("未记录", "Not recorded", "غير مسجل")}
                    <small className="request-rate-note">
                      {text(
                        "美元 / 百万词元，已包含分组倍率",
                        "USD / 1M tokens, group multiplier included",
                        "دولار / مليون رمز، شاملاً معامل المجموعة",
                      )}
                    </small>
                  </dd>
                </div>
              </>
            )}
            <div>
              <dt>{text("费率快照", "Rate snapshot", "لقطة السعر")}</dt>
              <dd className="mono" dir="ltr">
                {record.pricing_revision ??
                  text(
                    "历史样例费率",
                    "Historical sample rates",
                    "أسعار العينة السابقة",
                  )}
              </dd>
            </div>
            <div>
              <dt>{text("关联账本流水", "Ledger entry", "قيد السجل")}</dt>
              <dd>
                {entry ? (
                  <Link
                    to={`/console/billing?trace=${encodeURIComponent(record.trace_id)}`}
                    onClick={onClose}
                  >
                    {entry.id}
                  </Link>
                ) : (
                  text("无扣款流水", "No debit entry", "لا يوجد قيد خصم")
                )}
              </dd>
            </div>
            <div>
              <dt>{text("时间", "Time", "الوقت")}</dt>
              <dd>
                {new Date(record.time).toLocaleString(
                  lang === "zh" ? "zh-CN" : lang === "ar" ? "ar" : "en-GB",
                )}
              </dd>
            </div>
          </dl>
          <p className="request-inspector-note">
            {text(
              "费率与金额在调用时保存。运营后续改价不会重算此记录；智能路由不会把请求模型和实际模型拆成两笔收费。",
              "Rates and amounts were saved at request time. Later pricing changes do not recalculate this record; smart routing does not create duplicate charges.",
              "حُفظت الأسعار والمبالغ وقت الطلب. تغيير السعر لاحقاً لا يعيد حساب هذا السجل ولا يضاعف التوجيه الذكي الرسوم.",
            )}
          </p>
          <details className="technical-details">
            <summary>
              {text("高级诊断", "Advanced diagnostics", "التشخيص المتقدم")}
              <Plus size={15} />
            </summary>
            <dl className="product-detail-list">
              {isOps && record.resource_id && (
                <>
                  <div>
                    <dt>
                      {text(
                        "资源实例 / 渠道",
                        "Resource / channel",
                        "المورد / القناة",
                      )}
                    </dt>
                    <dd>
                      <code>
                        {record.resource_id} / {record.channel_id}
                      </code>
                    </dd>
                  </div>
                  <div>
                    <dt>
                      {text(
                        "来源快照 / 上游模型",
                        "Snapshot / upstream model",
                        "اللقطة / نموذج المصدر",
                      )}
                    </dt>
                    <dd>
                      <code>
                        {record.resource_snapshot_id} / {record.upstream_model}
                      </code>
                    </dd>
                  </div>
                </>
              )}
              <div>
                <dt>{text("错误代码", "Error code", "رمز الخطأ")}</dt>
                <dd className="mono">{record.error_code ?? "—"}</dd>
              </div>
              <div>
                <dt>
                  {text(
                    "耗时 / 首字延迟",
                    "Duration / first token",
                    "المدة / أول رمز",
                  )}
                </dt>
                <dd>
                  {record.duration_ms} / {record.ttft_ms}{" "}
                  {text("毫秒", "ms", "مللي ثانية")}
                </dd>
              </div>
              <div>
                <dt>{text("精确费用", "Exact cost", "التكلفة الدقيقة")}</dt>
                <dd>{formatMoney(record.cost_nano, "detail")}</dd>
              </div>
              <div>
                <dt>{text("来源", "Source", "المصدر")}</dt>
                <dd>
                  {record.metered_demo
                    ? text(
                        "管理员计费模拟（未生成媒体）",
                        "Admin billing simulation (no media generated)",
                        "محاكاة فوترة المسؤول دون إنشاء وسائط",
                      )
                    : record.source === "playground"
                      ? text(
                          "调试台模拟",
                          "Playground simulation",
                          "محاكاة مساحة التجربة",
                        )
                      : text(
                          "模拟客户端请求",
                          "Simulated client request",
                          "طلب عميل محاكى",
                        )}
                </dd>
              </div>
            </dl>
            {record.request && (
              <pre className="request-inspector-code" dir="ltr">
                {JSON.stringify(record.request, null, 2)}
              </pre>
            )}
          </details>
        </>
      )}
    </Drawer>
  );
}
