import { message as translatedMessage } from "../i18n/messages";
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
            ...translatedMessage("copy_model_id_01095e7880", { p0: id }),
          )}
          title={text(...translatedMessage("copy_model_id_03ddd2ee81"))}
        >
          {status === "copied" ? <Check size={13} /> : <Copy size={13} />}
        </button>
        <span className="model-id-copy-feedback" role="status">
          {status === "copied"
            ? text(...translatedMessage("copied_e6bf924fa2"))
            : ""}
        </span>
      </div>
      {status === "error" && (
        <small role="alert">
          {text(
            ...translatedMessage(
              "copy_failed_select_the_id_to_copy_manually_or_bbb8d2b49b",
            ),
          )}
        </small>
      )}
    </div>
  );
}
