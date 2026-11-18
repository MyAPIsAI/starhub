import { useState } from "react";
import { Button } from "../components/ui/button";
import { copyText } from "./code-example";
import { useSettingsText } from "./settings-text";
import "./image-probe-preview.css";

export function imagePreviewURL(raw?: string): string | undefined {
  if (!raw || raw.length > 8192) return;
  try {
    const url = new URL(raw);
    if (
      ["https:", "http:"].includes(url.protocol) &&
      url.hostname &&
      !url.username &&
      !url.password
    )
      return raw;
  } catch {
    /* Invalid preview links must not become navigation targets. */
  }
}

export function ImageProbePreview({ url }: { url?: string }) {
  const { text } = useSettingsText();
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const safeURL = imagePreviewURL(url);
  if (!safeURL)
    return (
      <p className="image-probe-note" role="status">
        {text(
          "图片已生成，但此记录未保存可用的预览地址，可能是旧记录或仅返回图片数据的响应。",
          "The image was generated, but no usable preview URL was saved. Older records or responses containing only image data cannot be previewed here.",
          "تم توليد الصورة لكن لم يُحفظ رابط معاينة صالح. لا يمكن معاينة السجلات القديمة أو الاستجابات التي تتضمن بيانات الصورة فقط هنا.",
        )}
      </p>
    );
  return (
    <div className="image-probe-preview">
      {failed ? (
        <div role="alert" className="image-probe-note">
          <p>
            {text(
              "图片已生成，但预览加载失败，链接可能已失效。",
              "The image was generated, but the preview could not load. The link may have expired.",
              "تم توليد الصورة لكن تعذر تحميل المعاينة. ربما انتهت صلاحية الرابط.",
            )}
          </p>
          <Button
            size="small"
            onClick={() => {
              setFailed(false);
              setAttempt((value) => value + 1);
            }}
          >
            {text("重新加载预览", "Reload preview", "إعادة تحميل المعاينة")}
          </Button>
        </div>
      ) : (
        <img
          key={attempt}
          src={safeURL}
          alt={text(
            "测试生成的图片",
            "Generated test image",
            "صورة الاختبار المولدة",
          )}
          referrerPolicy="no-referrer"
          onError={() => setFailed(true)}
        />
      )}
      <div className="image-probe-links">
        <Button size="small" asChild>
          <a href={safeURL} target="_blank" rel="noopener noreferrer">
            {text("打开原图", "Open original", "فتح الصورة الأصلية")}
          </a>
        </Button>
        <Button
          size="small"
          onClick={() =>
            void copyText(
              safeURL,
              text("图片链接已复制", "Image link copied", "تم نسخ رابط الصورة"),
              text(
                "复制失败，请打开原图后复制地址。",
                "Copy failed. Open the original and copy its address.",
                "فشل النسخ. افتح الصورة الأصلية وانسخ عنوانها.",
              ),
            )
          }
        >
          {text("复制链接", "Copy link", "نسخ الرابط")}
        </Button>
      </div>
      <p className="image-probe-note">
        {text(
          "链接有时效，请及时保存图片。重新查看或加载预览不会再次生成图片。",
          "The link is temporary; save the image promptly. Viewing or reloading the preview does not generate another image.",
          "الرابط مؤقت؛ احفظ الصورة سريعًا. عرض المعاينة أو إعادة تحميلها لا يولد صورة أخرى.",
        )}
      </p>
    </div>
  );
}
