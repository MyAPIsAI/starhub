import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import type { AdminChannel } from "../lib/api/admin";
import { useSettingsText } from "./settings-text";

export function ArkImageProbe({
  channel,
  disabled,
  onProbe,
}: {
  channel: AdminChannel;
  disabled: boolean;
  onProbe: (model: string) => void;
}) {
  const { text } = useSettingsText();
  const [open, setOpen] = useState(false);
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
        disabled={disabled}
        onClick={() => {
          setSelected(candidates.length === 1 ? candidates[0] : "");
          setOpen(true);
        }}
      >
        {label}
      </Button>
      <Modal
        open={open}
        onOpenChange={setOpen}
        title={label}
        description={text(
          "将向所选模型请求一张测试图片，可能产生上游费用。",
          "Request one test image from the selected model. Upstream charges may apply.",
          "سيُطلب إنشاء صورة اختبار واحدة من النموذج المحدد. قد تُطبق رسوم المزود.",
        )}
        footer={
          <>
            <Button
              onClick={() => setOpen(false)}
              style={{ flexShrink: 0, whiteSpace: "nowrap" }}
            >
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            <Button
              variant="primary"
              disabled={disabled || !candidates.includes(selected)}
              onClick={() => {
                if (disabled || !candidates.includes(selected)) return;
                setOpen(false);
                onProbe(selected);
              }}
            >
              {text(
                "生成测试图片（上游计费）",
                "Generate test image (upstream charges)",
                "توليد صورة الاختبار (رسوم المزود)",
              )}
            </Button>
          </>
        }
      >
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
      </Modal>
    </>
  );
}
