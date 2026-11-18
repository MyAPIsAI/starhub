import { useState } from "react";
import { ImagePlus, Eye } from "lucide-react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import type { AdminChannel, ChannelTestResult } from "../lib/api/admin";
import { ChannelProbeResult } from "./channel-probe-result";
import { ImageProbePreview, imagePreviewURL } from "./image-probe-preview";
import { useSettingsText } from "./settings-text";

export function ArkImageProbe({
  channel,
  disabled,
  pending,
  result,
  onProbe,
}: {
  channel: AdminChannel;
  disabled: boolean;
  pending: boolean;
  result?: ChannelTestResult;
  onProbe: (model: string) => void;
}) {
  const { text } = useSettingsText();
  const [open, setOpen] = useState(false);
  const [viewOnly, setViewOnly] = useState(false);
  const [selected, setSelected] = useState("");
  const candidates = [
    ...new Set(
      channel.models
        .split(",")
        .map((id) => id.trim())
        .filter(Boolean),
    ),
  ].filter(
    (id) =>
      !channel.model_capabilities?.[id] ||
      channel.model_capabilities[id] === "ImageGeneration",
  );
  if (channel.type !== 32 || candidates.length === 0) return null;
  const label = text(
    "图片生成测试（上游计费）",
    "Test image generation (upstream charges)",
    "اختبار توليد الصور (رسوم المزود)",
  );
  return (
    <>
      <Button
        size="small"
        className="channel-probe-generate"
        aria-label={label}
        disabled={disabled}
        onClick={() => {
          setSelected(candidates.length === 1 ? candidates[0] : "");
          setViewOnly(false);
          setOpen(true);
        }}
      >
        <ImagePlus size={14} aria-hidden="true" />
        {pending
          ? text("生成中…", "Generating…", "جارٍ التوليد…")
          : text("生成图片", "Generate image", "توليد صورة")}
        <span className="channel-probe-fee">
          {text("计费", "Paid", "مدفوع")}
        </span>
      </Button>
      {result && (
        <Button
          size="small"
          variant="ghost"
          className="channel-probe-view"
          onClick={() => {
            setViewOnly(true);
            setOpen(true);
          }}
        >
          <Eye size={14} aria-hidden="true" />
          {result.ok && imagePreviewURL(result.image_url)
            ? text("查看图片", "View image", "عرض الصورة")
            : text("查看结果", "View result", "عرض النتيجة")}
        </Button>
      )}
      <Modal
        open={open}
        onOpenChange={setOpen}
        title={
          viewOnly
            ? text("图片测试结果", "Image test result", "نتيجة اختبار الصورة")
            : label
        }
        description={
          viewOnly
            ? text(
                "查看本次图片测试的状态、模型和生成结果。",
                "Review the image test status, model and generated result.",
                "راجع حالة اختبار الصورة والنموذج والنتيجة المولدة.",
              )
            : text(
                "将向所选模型请求一张测试图片，可能产生上游费用。",
                "Request one test image from the selected model. Upstream charges may apply.",
                "سيُطلب إنشاء صورة اختبار واحدة من النموذج المحدد. قد تُطبق رسوم المزود.",
              )
        }
        footer={
          <>
            <Button
              onClick={() => setOpen(false)}
              style={{ flexShrink: 0, whiteSpace: "nowrap" }}
            >
              {viewOnly
                ? text("关闭", "Close", "إغلاق")
                : text("取消", "Cancel", "إلغاء")}
            </Button>
            {!viewOnly && (
              <Button
                variant="primary"
                disabled={disabled || !candidates.includes(selected)}
                onClick={() => {
                  if (disabled || !candidates.includes(selected)) return;
                  setViewOnly(true);
                  onProbe(selected);
                }}
              >
                {text(
                  "生成测试图片（上游计费）",
                  "Generate test image (upstream charges)",
                  "توليد صورة الاختبار (رسوم المزود)",
                )}
              </Button>
            )}
          </>
        }
      >
        {viewOnly ? (
          <>
            {pending && (
              <p role="status">
                {text(
                  "正在生成测试图片…",
                  "Generating test image…",
                  "جارٍ توليد صورة الاختبار…",
                )}
              </p>
            )}
            {!pending && result && (
              <>
                <ChannelProbeResult result={result} />
                {result.ok && (
                  <ImageProbePreview
                    key={result.image_url}
                    url={result.image_url}
                  />
                )}
              </>
            )}
          </>
        ) : (
          <>
            <label htmlFor={`ark-image-model-${channel.id}`}>
              {text(
                "被测图片模型",
                "Image model to test",
                "نموذج الصور المراد اختباره",
              )}
            </label>
            <select
              id={`ark-image-model-${channel.id}`}
              value={selected}
              onChange={(event) => setSelected(event.target.value)}
              className="select-trigger"
              style={{ width: "100%", marginTop: 8 }}
            >
              <option value="">
                {text("请选择模型", "Select a model", "اختر نموذجًا")}
              </option>
              {candidates.map((id) => (
                <option key={id} value={id}>
                  {id}
                </option>
              ))}
            </select>
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 12 }}>
              {text(
                "手工配置的模型或 Endpoint ID（推理接入点标识）需确认支持图片生成。",
                "Confirm that manually configured models or Endpoint IDs support image generation.",
                "تأكد من أن النماذج أو نقاط النهاية المُدخلة يدويًا تدعم توليد الصور.",
              )}
            </p>
          </>
        )}
      </Modal>
    </>
  );
}
