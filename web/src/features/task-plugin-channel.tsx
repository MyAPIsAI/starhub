import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import { useTaskPlugins, useUploadTaskPlugin } from "../lib/api/task-plugin";
import { useSettingsText } from "./settings-text";

export function TaskPluginChannel({
  value,
  onChange,
  disabled,
  onBusyChange,
}: {
  value: number;
  onChange: (id: number) => void;
  disabled: boolean;
  onBusyChange: (busy: boolean) => void;
}) {
  const { text } = useSettingsText();
  const versions = useTaskPlugins();
  const upload = useUploadTaskPlugin();
  const [error, setError] = useState("");
  const [working, setWorking] = useState(false);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      onBusyChange(false);
    };
  }, [onBusyChange]);
  const busy = disabled || working;
  const save = async (loadSource: () => Promise<string>) => {
    setError("");
    setWorking(true);
    onBusyChange(true);
    try {
      const plugin = await upload.mutateAsync(await loadSource());
      await versions.refetch();
      if (!plugin.enabled)
        throw new Error(
          text(
            "该插件版本已禁用，请先在插件管理中启用，或选择其他版本。",
            "This plugin version is disabled. Enable it in plugin management or select another version.",
            "إصدار الإضافة معطّل. فعّله من إدارة الإضافات أو اختر إصدارًا آخر.",
          ),
        );
      if (mounted.current) onChange(plugin.id);
    } catch (failure) {
      if (mounted.current)
        setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      if (mounted.current) {
        setWorking(false);
        onBusyChange(false);
      }
    }
  };

  return (
    <div className="task-plugin-channel-field">
      <label htmlFor="ch-plugin">
        {text("插件版本", "Plugin version", "إصدار الإضافة")}
      </label>
      <select
        id="ch-plugin"
        required
        value={value || ""}
        disabled={
          busy ||
          versions.isPending ||
          versions.isError ||
          !versions.data?.length
        }
        aria-describedby="ch-plugin-help"
        onChange={(event) => onChange(Number(event.target.value))}
      >
        <option value="">
          {versions.isPending
            ? text("加载中…", "Loading…", "جارٍ التحميل…")
            : text("选择版本", "Select a version", "اختر إصداراً")}
        </option>
        {versions.data?.map((plugin) => (
          <option key={plugin.id} value={plugin.id} disabled={!plugin.enabled}>
            {plugin.key} {plugin.version}
            {plugin.enabled ? "" : text("（已禁用）", " (disabled)", " (معطل)")}
          </option>
        ))}
      </select>
      <span id="ch-plugin-help" role="status">
        {versions.isSuccess && versions.data.length === 0
          ? text(
              "尚无插件版本。OpenAI 视频可添加示例插件；豆包视频请选择火山方舟（豆包）渠道。",
              "No plugin versions yet. Add the sample plugin for OpenAI video; select Volcengine Ark (Doubao) for Doubao video.",
              "لا توجد إصدارات. أضف الإضافة النموذجية لفيديو OpenAI؛ واختر قناة Volcengine Ark (Doubao) لفيديو Doubao.",
            )
          : text(
              "插件必须适配上游视频接口。使用单个上游密钥；有未完成任务时不可更换密钥或地址。",
              "The plugin must match the upstream video API. Use one key; credentials and URL are locked while tasks are pending.",
              "يجب أن تتوافق الإضافة مع واجهة الفيديو. استخدم مفتاحاً واحداً؛ تُقفل بيانات الاعتماد والعنوان أثناء المهام المعلقة.",
            )}
      </span>
      <span>
        {text(
          "OpenAI 示例插件适配 sora-2 / sora-2-pro。豆包视频请选择“火山方舟（豆包）”渠道，无需上传插件；其他自定义接口需匹配的可信插件。",
          "The OpenAI sample supports sora-2 / sora-2-pro. For Doubao video, select Volcengine Ark (Doubao) without uploading a plugin. Other custom APIs need a matching trusted plugin.",
          "تدعم إضافة OpenAI النموذجية sora-2 / sora-2-pro. لفيديو Doubao اختر Volcengine Ark (Doubao) دون رفع إضافة. تحتاج الواجهات المخصصة الأخرى إلى إضافة موثوقة متوافقة.",
        )}
      </span>
      <div className="task-plugin-actions">
        <Button
          size="small"
          disabled={busy}
          loading={working}
          onClick={() =>
            void save(
              async () =>
                (await import("../../../plugins/tasks/openai-video.js?raw"))
                  .default,
            )
          }
        >
          {text(
            "添加 OpenAI 视频插件",
            "Add OpenAI video plugin",
            "إضافة فيديو OpenAI",
          )}
        </Button>
        <Button
          size="small"
          disabled={busy || versions.isFetching}
          onClick={() => void versions.refetch()}
        >
          {text("刷新版本", "Refresh versions", "تحديث الإصدارات")}
        </Button>
      </div>
      <label htmlFor="ch-plugin-file">
        {text(
          "上传可信插件（JavaScript，最大 256 KB）",
          "Upload trusted plugin (JavaScript, up to 256 KB)",
          "رفع إضافة موثوقة (JavaScript، حتى 256 كيلوبايت)",
        )}
      </label>
      <input
        id="ch-plugin-file"
        type="file"
        accept=".js"
        disabled={busy}
        onChange={async (event) => {
          const input = event.currentTarget;
          const file = input.files?.[0];
          if (!file) return;
          await save(async () => {
            if (file.size > 262144)
              throw new Error(
                text(
                  "文件超过大小限制",
                  "File is too large",
                  "الملف كبير جداً",
                ),
              );
            return file.text();
          });
          input.value = "";
        }}
      />
      {(error || versions.isError) && (
        <span role="alert">{error || versions.error?.message}</span>
      )}
    </div>
  );
}
