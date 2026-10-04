import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import { usePreferences } from "../lib/preferences";
import "./model-id-copy.css";

/** Copy the exact client-facing identifier, never the localized display name. */
export function ModelIdCopy({ id }: { id: string }) {
  const language = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [status, setStatus] = useState<"idle" | "copied" | "error">("idle");
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const generation = useRef(0);
  useEffect(() => {
    generation.current += 1;
    setStatus("idle");
    return () => {
      generation.current += 1;
      clearTimeout(timer.current);
    };
  }, [id]);
  const copy = async () => {
    const attempt = ++generation.current;
    clearTimeout(timer.current);
    setStatus("idle");
    try {
      await navigator.clipboard.writeText(id);
      if (attempt !== generation.current) return;
      setStatus("copied");
      timer.current = setTimeout(() => {
        if (attempt === generation.current) setStatus("idle");
      }, 2500);
    } catch {
      if (attempt === generation.current) setStatus("error");
    }
  };
  return (
    <div
      className={`model-id-copy${status === "error" ? " has-copy-error" : ""}`}
    >
      <div className="model-id-copy-line">
        <code dir="ltr">{id}</code>
        <button
          type="button"
          data-copy-model-id={id}
          onClick={copy}
          aria-label={text(
            `复制模型标识 ${id}`,
            `Copy model ID ${id}`,
            `نسخ معرف النموذج ${id}`,
          )}
          title={text("复制模型标识", "Copy model ID", "نسخ معرف النموذج")}
        >
          {status === "copied" ? <Check size={13} /> : <Copy size={13} />}
        </button>
        <span className="model-id-copy-feedback" role="status">
          {status === "copied" ? text("已复制", "Copied", "تم النسخ") : ""}
        </span>
      </div>
      {status === "error" && (
        <small role="alert">
          {text(
            "复制失败，请选中标识手动复制，或重试。",
            "Copy failed. Select the ID to copy manually, or retry.",
            "تعذر النسخ. حدد المعرف لنسخه يدويًا أو أعد المحاولة.",
          )}
        </small>
      )}
    </div>
  );
}
