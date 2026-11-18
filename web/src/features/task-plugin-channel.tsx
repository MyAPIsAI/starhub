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
              "尚无插件版本，请添加 OpenAI 视频插件，或上传适配上游的可信插件。",
              "No plugin versions yet. Add the OpenAI video plugin or upload a trusted adapter for your upstream.",
              "لا توجد إصدارات. أضف إضافة فيديو OpenAI أو ارفع إضافة موثوقة لمزودك.",
            )
          : text(
              "插件必须适配上游视频接口。使用单个上游密钥；有未完成任务时不可更换密钥或地址。",
              "The plugin must match the upstream video API. Use one key; credentials and URL are locked while tasks are pending.",
              "يجب أن تتوافق الإضافة مع واجهة الفيديو. استخدم مفتاحاً واحداً؛ تُقفل بيانات الاعتماد والعنوان أثناء المهام المعلقة.",
            )}
      </span>
      <span>
        {text(
          "内置插件仅适用于 OpenAI 视频接口（sora-2 / sora-2-pro），其他上游请上传对应插件。",
          "The built-in plugin supports the OpenAI video API (sora-2 / sora-2-pro). Upload a matching plugin for other upstreams.",
          "تدعم الإضافة المدمجة واجهة فيديو OpenAI (sora-2 / sora-2-pro). ارفع إضافة متوافقة للمزودين الآخرين.",
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
