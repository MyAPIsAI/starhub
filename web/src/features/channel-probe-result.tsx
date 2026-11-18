import type { ChannelTestResult } from "../lib/api/admin";
import { CheckCircle2, CircleAlert } from "lucide-react";
import { useSettingsText } from "./settings-text";
import "./channel-probe-result.css";

const errors: Record<string, [string, string, string]> = {
  image_probe_usage_unavailable: [
    "本地图片测试用量记录失败，请检查数据库；请先核对上游执行情况再重试。",
    "The local image test usage could not be recorded. Check the database and upstream execution before retrying.",
    "تعذر تسجيل استخدام اختبار الصور محليًا. تحقق من قاعدة البيانات وتنفيذ المزود قبل إعادة المحاولة.",
  ],
  upstream_video_authentication_failed: [
    "上游返回 401：视频接口认证失败，请更新有效的方舟 API Key。",
    "Upstream returned 401: video API authentication failed. Update the Ark API key.",
    "أعاد المزود 401: فشلت مصادقة واجهة الفيديو. حدّث مفتاح Ark.",
  ],
  upstream_video_permission_denied: [
    "上游返回 403：无权访问视频任务接口，请检查账号权限。",
    "Upstream returned 403: video task access denied. Check account permissions.",
    "أعاد المزود 403: الوصول لمهام الفيديو مرفوض. تحقق من صلاحيات الحساب.",
  ],
  upstream_video_not_found: [
    "上游未提供方舟视频任务接口，请检查地址和渠道类型。",
    "The upstream does not provide the Ark video task API. Check the URL and channel type.",
    "لا يوفر المزود واجهة مهام Ark. تحقق من العنوان ونوع القناة.",
  ],
  upstream_video_network_error: [
    "无法连接视频任务接口，请检查地址和网络。",
    "Cannot connect to the video task API. Check the URL and network.",
    "تعذر الاتصال بواجهة الفيديو. تحقق من العنوان والشبكة.",
  ],
  upstream_video_invalid_response: [
    "上游视频任务列表格式不正确，尚未验证生成。",
    "Invalid video task list; generation remains untested.",
    "قائمة مهام الفيديو غير صالحة؛ لم يُختبر التوليد.",
  ],
  image_probe_model_not_supported: [
    "所选模型不是图片模型，请使用对应的任务 API。",
    "The selected model is not an image model. Use the appropriate task API.",
    "النموذج المحدد ليس نموذج صور. استخدم واجهة المهام المناسبة.",
  ],
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
    "上游返回 401：目录认证失败，请检查 API Key 是否正确、有效且属于当前上游。",
    "Upstream returned 401: catalog authentication failed. Check that the API key is valid and belongs to this upstream.",
    "أعاد المزود 401: فشلت مصادقة الدليل. تحقق من صلاحية المفتاح وانتمائه للمزود.",
  ],
  upstream_models_permission_denied: [
    "上游返回 403：目录访问受限，请检查项目权限及上游访问策略。",
    "Upstream returned 403: catalog access denied. Check project permissions and upstream access policies.",
    "أعاد المزود 403: الوصول للدليل مرفوض. تحقق من صلاحيات المشروع وسياسات الوصول.",
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
      : text(
          "目录检查失败（生成未验证）",
          "Catalog check failed (generation not tested)",
          "فشل فحص الدليل (لم يُختبر التوليد)",
        );
  if (result.scope === "image")
    label = result.ok
      ? text("图片生成成功", "Image generated", "تم توليد الصورة")
      : text("图片生成失败", "Image generation failed", "فشل توليد الصورة");
  if (result.scope === "video_api")
    label = result.ok
      ? text(
          "视频接口连通（生成未验证）",
          "Video API accessible (generation not tested)",
          "واجهة الفيديو متاحة (لم يُختبر التوليد)",
        )
      : text(
          "视频接口连接失败",
          "Video API connection failed",
          "فشل الاتصال بواجهة الفيديو",
        );
  if (result.scope === "local" && result.ok)
    label = text(
      "插件本地校验通过",
      "Plugin validated locally",
      "تم التحقق من الإضافة محليًا",
    );
  if (compact && result.scope === "catalog" && result.ok)
    label = text(
      "目录可访问（生成未验证）",
      "Catalog accessible (generation not tested)",
      "الدليل متاح (لم يُختبر التوليد)",
    );
  const model = result.model && (
    <span className="channel-probe-model">
      {result.scope === "catalog"
        ? text("目录查询模型：", "Catalog model: ", "نموذج الدليل: ")
        : text("被测模型：", "Tested model: ", "النموذج المختبر: ")}
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
                result.code === "ark_video_catalog_only"
                  ? "视频目录可访问，尚未生成视频；实际生成请通过任务 API 提交。"
                  : "目录检查未验证实际生成。",
                result.code === "ark_video_catalog_only"
                  ? "Video catalog accessible; no video was generated. Submit generation through the task API."
                  : "Catalog checks do not verify inference.",
                result.code === "ark_video_catalog_only"
                  ? "دليل الفيديو متاح ولم يُولد فيديو. أرسل التوليد عبر واجهة المهام."
                  : "فحص الدليل لا يتحقق من التوليد الفعلي.",
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
      {!result.ok && result.diagnostic?.request_id && (
        <span className="channel-probe-model">
          {text("请求 ID：", "Request ID: ", "معرّف الطلب: ")}
          <bdi>{result.diagnostic.request_id}</bdi>
        </span>
      )}
    </div>
  );
}
