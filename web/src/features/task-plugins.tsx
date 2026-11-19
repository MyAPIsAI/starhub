import { useRef, useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import {
  useTaskPlugins,
  useUploadTaskPlugin,
  useToggleTaskPlugin,
  useTestTaskPlugin,
  type TaskPluginVersion,
} from "../lib/api/task-plugin";
import { useSettingsText } from "./settings-text";
import "./task-plugins.css";
import { useSessionQueryIdentity } from "../lib/system-access";
import { ArkVideoIntegration } from "./ark-video-integration";

export function TaskPlugins() {
  const identity = useSessionQueryIdentity();
  return <TaskPluginsView key={JSON.stringify(identity)} />;
}

function TaskPluginsView() {
  const { text } = useSettingsText();
  const versions = useTaskPlugins();
  const upload = useUploadTaskPlugin();
  const toggle = useToggleTaskPlugin();
  const test = useTestTaskPlugin();
  const fileRef = useRef<HTMLInputElement>(null);
  const [selected, setSelected] = useState<TaskPluginVersion | null>(null);
  const [hook, setHook] = useState("prepareSubmit");
  const [args, setArgs] = useState(
    '[{"model":"sora-2","request":{"model":"sora-2","prompt":"A cat","seconds":4,"n":1}}]',
  );
  const [error, setError] = useState("");
  return (
    <section className="task-plugins">
      <h1>
        {text("视频任务插件", "Video task plugins", "إضافات مهام الفيديو")}
      </h1>
      <p>
        {text(
          "豆包视频可直接配置火山方舟渠道。自定义视频接口可上传可信插件，再为渠道绑定固定版本；禁用插件后已有任务继续查询。",
          "Configure a Volcengine Ark channel for Doubao video. For custom video APIs, upload a trusted plugin and bind a fixed version to a channel. Existing tasks continue polling when a plugin is disabled.",
          "أعدّ قناة Volcengine Ark لفيديو Doubao. للواجهات المخصصة، ارفع إضافة موثوقة واربط إصدارًا ثابتًا بالقناة. تستمر متابعة المهام الحالية عند تعطيل الإضافة.",
        )}
      </p>
      <ArkVideoIntegration />
      <label>
        {text(
          "插件文件（JavaScript，最大 256 KB）",
          "Plugin file (JavaScript, up to 256 KB)",
          "ملف الإضافة (JavaScript، حتى 256 كيلوبايت)",
        )}
        <input
          ref={fileRef}
          type="file"
          accept=".js"
          disabled={upload.isPending}
          onChange={async (event) => {
            const file = event.target.files?.[0];
            if (!file) return;
            setError("");
            if (file.size > 262144) {
              setError(
                text(
                  "文件超过大小限制",
                  "File is too large",
                  "الملف كبير جداً",
                ),
              );
              return;
            }
            try {
              await upload.mutateAsync(await file.text());
              if (fileRef.current) fileRef.current.value = "";
            } catch (e) {
              setError(e instanceof Error ? e.message : String(e));
            }
          }}
        />
      </label>
      {upload.isPending && (
        <p role="status">
          {text(
            "正在校验并上传…",
            "Validating and uploading…",
            "جارٍ التحقق والرفع…",
          )}
        </p>
      )}
      {(error || versions.error || toggle.error) && (
        <p role="alert">
          {error || versions.error?.message || toggle.error?.message}
        </p>
      )}
      {versions.isPending && (
        <p role="status">{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>
      )}
      {versions.isError && (
        <Button onClick={() => void versions.refetch()}>
          {text("重试", "Retry", "إعادة المحاولة")}
        </Button>
      )}
      {versions.data?.length === 0 && (
        <p>
          {text(
            "尚未上传自定义插件；豆包视频可直接配置上方渠道。",
            "No custom plugins uploaded. Configure Doubao video using the channel above.",
            "لم تُرفع إضافات مخصصة. أعدّ فيديو Doubao عبر القناة أعلاه.",
          )}
        </p>
      )}
      <ul>
        {versions.data?.map((version) => (
          <li key={version.id}>
            <div>
              <strong>{version.key}</strong> <code>{version.version}</code>
              <span>
                {version.enabled
                  ? text("已启用", "Enabled", "مفعّل")
                  : text("已禁用", "Disabled", "معطّل")}
              </span>
            </div>
            <div className="task-plugin-actions">
              <Button
                disabled={toggle.isPending}
                onClick={() =>
                  toggle.mutate({ id: version.id, enabled: !version.enabled })
                }
              >
                {version.enabled
                  ? text("禁用", "Disable", "تعطيل")
                  : text("启用", "Enable", "تفعيل")}
              </Button>
              <Button
                onClick={() => {
                  test.reset();
                  setError("");
                  setSelected(version);
                }}
              >
                {text("解析测试", "Test transforms", "اختبار التحويل")}
              </Button>
            </div>
          </li>
        ))}
      </ul>
      <Modal
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
        title={text(
          "插件解析测试",
          "Plugin transform test",
          "اختبار تحويل الإضافة",
        )}
        description={text(
          "仅执行数据转换，不发送上游请求，也不扣费。请使用不含密钥的样例数据。",
          "Runs data transforms without upstream requests or charges. Use sample data without credentials.",
          "يختبر التحويل دون طلبات خارجية أو رسوم. استخدم بيانات تجريبية بلا مفاتيح.",
        )}
        footer={
          <Button
            disabled={test.isPending}
            onClick={() => {
              setError("");
              test.reset();
              try {
                const values: unknown = JSON.parse(args);
                if (!Array.isArray(values))
                  throw new Error(
                    text(
                      "参数必须是数组",
                      "Arguments must be an array",
                      "يجب أن تكون المعاملات مصفوفة",
                    ),
                  );
                if (selected)
                  test.mutate({ id: selected.id, hook, args: values });
              } catch (e) {
                setError(e instanceof Error ? e.message : String(e));
              }
            }}
          >
            {text("运行测试", "Run test", "تشغيل الاختبار")}
          </Button>
        }
      >
        <div className="task-plugins">
          <label>
            {text("转换步骤", "Transform", "التحويل")}
            <select
              value={hook}
              onChange={(e) => {
                setHook(e.target.value);
                test.reset();
              }}
            >
              {["prepareSubmit", "parseSubmit", "buildQuery", "parseQuery"].map(
                (name) => (
                  <option key={name}>{name}</option>
                ),
              )}
            </select>
          </label>
          <label>
            {text(
              "参数数组（JSON）",
              "Arguments (JSON array)",
              "المعاملات (مصفوفة JSON)",
            )}
            <textarea
              rows={8}
              dir="ltr"
              value={args}
              onChange={(e) => setArgs(e.target.value)}
            />
          </label>
          {(error || test.error) && (
            <p role="alert">{error || test.error?.message}</p>
          )}
          {test.isSuccess && (
            <pre dir="ltr" tabIndex={0}>
              {JSON.stringify(test.data, null, 2)}
            </pre>
          )}
        </div>
      </Modal>
    </section>
  );
}
