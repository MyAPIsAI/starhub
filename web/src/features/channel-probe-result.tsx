import type { ChannelTestResult } from "../lib/api/admin";
import { CheckCircle2, CircleAlert } from "lucide-react";
import { useSettingsText } from "./settings-text";
import "./channel-probe-result.css";

const errors: Record<string, [string, string, string]> = {
  upstream_model_not_found: [
    "配置的模型 ID 不在上游目录中。请获取准确版本并替换旧短名；Endpoint ID 需单独验证生成。",
    "The configured model ID is not in the upstream catalog. Fetch an exact version and replace the short name; Endpoint IDs need a separate inference test.",
    "معرّف النموذج غير موجود في دليل المزود. اجلب الإصدار الدقيق واستبدل الاسم المختصر؛ تحتاج نقاط النهاية إلى اختبار توليد منفصل.",
  ],
  models_key_required: [
    "没有可用密钥，请检查渠道凭证。",
    "No enabled key is available. Check channel credentials.",
    "لا يوجد مفتاح متاح. تحقق من بيانات اعتماد القناة.",
  ],
  upstream_models_authentication_failed: [
    "上游拒绝目录认证，请检查密钥及目录权限。",
    "Upstream catalog authentication failed. Check the key and catalog permission.",
    "فشلت مصادقة الدليل. تحقق من المفتاح وصلاحية الدليل.",
  ],
  upstream_models_not_found: [
    "上游目录接口不存在，请检查渠道类型和地址。",
    "The upstream catalog endpoint was not found. Check the channel type and URL.",
    "واجهة دليل المزود غير موجودة. تحقق من نوع القناة والعنوان.",
  ],
  upstream_models_network_error: [
    "无法连接上游目录，请检查地址和网络。",
    "Cannot reach the upstream catalog. Check the URL and network.",
    "تعذر الاتصال بدليل المزود. تحقق من العنوان والشبكة.",
  ],
  upstream_models_timeout: [
    "获取上游目录超时，请稍后重试。",
    "The upstream catalog request timed out. Retry later.",
    "انتهت مهلة طلب الدليل. أعد المحاولة لاحقًا.",
  ],
  upstream_models_rate_limited: [
    "上游目录请求被限流，请稍后重试。",
    "The upstream catalog rate limit was reached. Retry later.",
    "تم بلوغ حد طلبات الدليل. أعد المحاولة لاحقًا.",
  ],
  upstream_models_invalid_response: [
    "上游返回的目录格式无效。",
    "The upstream catalog response is invalid.",
    "استجابة دليل المزود غير صالحة.",
  ],
};

export function ChannelProbeResult({
  result,
  compact = false,
}: {
  result: ChannelTestResult;
  compact?: boolean;
}) {
  const { text } = useSettingsText();
  // Recognize responses from a backend that has not loaded diagnostic codes yet.
  const code =
    result.code ||
    (result.message.startsWith("Model ID not in catalog;")
      ? "upstream_model_not_found"
      : result.message);
  const failure = errors[code];
  let label = result.ok
    ? text("成功", "OK", "نجح")
    : text("失败", "Failed", "فشل");
  if (result.scope === "catalog")
    label = result.ok
      ? text(
          "目录检查通过（未验证生成）",
          "Catalog accessible (generation not tested)",
          "الدليل متاح (لم يُختبر التوليد)",
        )
      : text("目录检查失败", "Catalog check failed", "فشل فحص الدليل");
  if (result.scope === "image")
    label = result.ok
      ? text("图片生成成功", "Image generated", "تم توليد الصورة")
      : text("图片生成失败", "Image generation failed", "فشل توليد الصورة");
  if (result.scope === "local" && result.ok)
    label = text(
      "插件本地校验通过",
      "Plugin validated locally",
      "تم التحقق من الإضافة محليًا",
    );
  if (compact && result.scope === "catalog" && result.ok)
    label = text("目录可访问", "Catalog accessible", "الدليل متاح");
  const model = result.model && (
    <span className="channel-probe-model">
      {text("被测模型：", "Tested model: ", "النموذج المختبر: ")}
      <bdi>{result.model}</bdi>
    </span>
  );
  return (
    <div
      className={`channel-probe-result${compact ? " channel-probe-result--compact" : ""}`}
      role={result.ok ? "status" : "alert"}
      data-ok={result.ok}
    >
      <span className="channel-probe-state">
        {result.ok ? (
          <CheckCircle2 size={13} aria-hidden="true" />
        ) : (
          <CircleAlert size={13} aria-hidden="true" />
        )}
        {label}
      </span>
      <span className="channel-probe-latency">{` · ${result.elapsed_ms}ms`}</span>
      {compact ? (
        <details className="channel-probe-details">
          <summary>{text("详情", "Details", "التفاصيل")}</summary>
          {model}
          {result.scope === "catalog" && (
            <span className="channel-probe-model">
              {text(
                "目录检查未验证实际生成。",
                "Catalog checks do not verify inference.",
                "فحص الدليل لا يتحقق من التوليد الفعلي.",
              )}
            </span>
          )}
        </details>
      ) : (
        model
      )}
      {!result.ok && (
        <span className="channel-probe-error">
          {failure
            ? text(...failure)
            : result.scope === "catalog"
              ? text(
                  "无法验证上游目录，请检查连接和模型配置。",
                  "The upstream catalog could not be verified. Check the connection and model configuration.",
                  "تعذر التحقق من الدليل. تحقق من الاتصال وإعدادات النموذج.",
                )
              : result.message}
        </span>
      )}
    </div>
  );
}
