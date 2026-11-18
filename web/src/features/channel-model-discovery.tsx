import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import {
  fetchChannelModels,
  type ChannelModelsInput,
} from "../lib/api/channel";
import { ApiError } from "../lib/api/http";
import {
  TASK_PLUGIN_CHANNEL_TYPE,
  type TaskPluginVersion,
} from "../lib/api/task-plugin";
import { useSessionQueryIdentity } from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import "./channel-model-discovery.css";

const errors: Record<string, [string, string, string]> = {
  invalid_models_base_url: [
    "请填写有效的 HTTP 或 HTTPS 上游根地址，不包含查询参数或凭证。",
    "Enter a valid HTTP or HTTPS upstream root URL without query parameters or credentials.",
    "أدخل عنوان جذر HTTP أو HTTPS صالحًا دون معاملات أو بيانات اعتماد.",
  ],
  models_base_url_contains_v1: [
    "请去掉上游地址末尾的 /v1；系统会自动追加接口路径。",
    "Remove the trailing /v1 from the upstream URL; endpoint paths are added automatically.",
    "احذف /v1 من نهاية العنوان؛ تُضاف مسارات الواجهة تلقائيًا.",
  ],
  models_changed_connection_requires_key: [
    "上游地址或类型已改变，请输入用于新配置的密钥后重新获取。",
    "The upstream URL or type changed. Enter a key for the new connection and fetch again.",
    "تغيّر العنوان أو النوع. أدخل مفتاح الاتصال الجديد وأعد الجلب.",
  ],
  models_key_required: [
    "请填写密钥；已有渠道需要至少一个启用的密钥。",
    "Enter a key. Saved channels need at least one enabled key.",
    "أدخل مفتاحًا. تحتاج القناة المحفوظة إلى مفتاح مُفعّل واحد على الأقل.",
  ],
  models_channel_unavailable: [
    "渠道已不存在或凭证无法读取，请刷新后重试。",
    "The channel or its credential is unavailable. Refresh and retry.",
    "القناة أو بيانات اعتمادها غير متاحة. حدّث وأعد المحاولة.",
  ],
  models_channel_type_unsupported: [
    "当前支持 OpenAI 兼容渠道和视频任务插件获取模型，其他类型请手动填写。",
    "Discovery supports OpenAI-compatible and video task plugin channels. Enter models manually for other types.",
    "الجلب متاح لقنوات OpenAI وإضافات الفيديو. أدخل النماذج يدويًا للأنواع الأخرى.",
  ],
  models_plugin_unavailable: [
    "请选择可用的插件版本后重试。",
    "Select an available plugin version and retry.",
    "اختر إصدار إضافة متاحًا وأعد المحاولة.",
  ],
  upstream_models_authentication_failed: [
    "上游拒绝认证，请检查密钥及其模型列表权限。",
    "Upstream authentication failed. Check the key and its model-list permission.",
    "رفض المزود المصادقة. تحقق من المفتاح وصلاحية قائمة النماذج.",
  ],
  upstream_models_not_found: [
    "上游模型目录不存在，请检查渠道类型和地址，或手动填写模型。",
    "The upstream model catalog was not found. Check the channel type and URL, or enter models manually.",
    "لم يُعثر على دليل النماذج. تحقق من نوع القناة والعنوان أو أدخل النماذج يدويًا.",
  ],
  upstream_models_rate_limited: [
    "上游请求过于频繁，请稍后重试。",
    "Upstream rate limit reached. Retry later.",
    "تم بلوغ حد طلبات المزود. أعد المحاولة لاحقًا.",
  ],
  upstream_models_timeout: [
    "获取上游模型超时，请重试。",
    "Fetching upstream models timed out. Retry.",
    "انتهت مهلة جلب النماذج. أعد المحاولة.",
  ],
  upstream_models_network_error: [
    "无法连接上游，请检查地址和网络后重试。",
    "Cannot reach upstream. Check the URL and network, then retry.",
    "تعذر الاتصال بالمزود. تحقق من العنوان والشبكة ثم أعد المحاولة.",
  ],
  upstream_models_redirect_refused: [
    "上游返回了重定向，请直接填写正确的上游根地址。",
    "Upstream redirected the request. Enter the correct root URL directly.",
    "أعاد المزود توجيه الطلب. أدخل عنوان الجذر الصحيح مباشرةً.",
  ],
  upstream_models_too_large: [
    "上游列表超过大小限制，请手动填写需要的模型。",
    "The upstream list exceeds size limits. Enter the required models manually.",
    "تجاوزت القائمة حد الحجم. أدخل النماذج المطلوبة يدويًا.",
  ],
  upstream_models_pagination_unsupported: [
    "上游返回分页列表，当前无法获取完整目录，请手动填写模型。",
    "Upstream returned a paginated list. Enter models manually until pagination is supported.",
    "أعاد المزود قائمة مقسمة لصفحات. أدخل النماذج يدويًا حاليًا.",
  ],
  upstream_models_invalid_id: [
    "上游返回了无法保存的模型标识，请手动填写。",
    "Upstream returned a model ID that cannot be saved. Enter models manually.",
    "أعاد المزود معرّف نموذج لا يمكن حفظه. أدخل النماذج يدويًا.",
  ],
  upstream_models_invalid_response: [
    "上游返回的模型列表格式不正确，请重试或手动填写。",
    "Upstream returned an invalid model list. Retry or enter models manually.",
    "أعاد المزود قائمة نماذج غير صالحة. أعد المحاولة أو أدخلها يدويًا.",
  ],
};

const domains: Record<string, [string, string, string]> = {
  LLM: ["文本生成", "Text generation", "توليد النصوص"],
  VLM: ["多模态理解", "Multimodal understanding", "فهم متعدد الوسائط"],
  ImageGeneration: ["图片生成", "Image generation", "توليد الصور"],
  VideoGeneration: ["视频生成", "Video generation", "توليد الفيديو"],
  "3DGeneration": ["三维生成", "3D generation", "توليد ثلاثي الأبعاد"],
  Audio: ["音频", "Audio", "الصوت"],
  Embedding: ["向量化", "Embeddings", "التضمينات"],
};

export function ChannelModelDiscovery({
  connection,
  models,
  onChange,
  disabled,
  plugin,
  capabilities = {},
}: {
  connection: ChannelModelsInput;
  models: string;
  onChange: (models: string, capabilities?: Record<string, string>) => void;
  disabled: boolean;
  plugin?: TaskPluginVersion;
  capabilities?: Record<string, string>;
}) {
  const { text } = useSettingsText();
  const identity = JSON.stringify(useSessionQueryIdentity());
  const signature = JSON.stringify([identity, connection, disabled]);
  const latest = useRef(signature);
  latest.current = signature;
  const active = useRef<AbortController>();
  const sequence = useRef(0);
  const [state, setState] = useState<{
    status: "idle" | "loading" | "success" | "error" | "stale";
    signature?: string;
    models: string[];
    capabilities?: Record<string, string>;
    error?: string;
  }>({ status: "idle", models: [] });
  const [selected, setSelected] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    sequence.current++;
    active.current?.abort();
    setState((previous) =>
      previous.status === "idle" ? previous : { status: "stale", models: [] },
    );
    setSelected([]);
    setSearch("");
    setNotice("");
    return () => {
      sequence.current++;
      active.current?.abort();
    };
  }, [signature]);

  const isPlugin = connection.type === TASK_PLUGIN_CHANNEL_TYPE;
  const supported = connection.type === 1 || connection.type === 32 || isPlugin;
  let declaredModels: string[] = [];
  if (isPlugin && plugin) {
    try {
      const metadata = JSON.parse(plugin.meta_json);
      if (Array.isArray(metadata.models))
        declaredModels = metadata.models.filter(
          (value: unknown): value is string => typeof value === "string",
        );
    } catch {
      // Invalid stored metadata must not populate the channel form.
    }
  }
  const coherent = state.signature === signature;
  const loading = coherent && state.status === "loading";
  const success = coherent && state.status === "success";
  const current = [
    ...new Set(
      models
        .split(",")
        .map((model) => model.trim())
        .filter(Boolean),
    ),
  ];
  const fetched = new Set(state.models);
  const unlisted = current.filter((model) => !fetched.has(model));
  const filtered = state.models.filter((model) =>
    model.toLowerCase().includes(search.trim().toLowerCase()),
  );

  const fetchModels = async () => {
    if (!supported || disabled || loading) return;
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    const request = ++sequence.current;
    setState({ status: "loading", signature, models: [] });
    setSelected([]);
    setNotice("");
    try {
      const catalog = await fetchChannelModels(connection, controller.signal);
      const found = catalog.map((entry) => entry.id);
      if (
        request !== sequence.current ||
        controller.signal.aborted ||
        latest.current !== signature
      )
        return;
      setState({
        status: "success",
        signature,
        models: found,
        capabilities: Object.fromEntries(
          catalog
            .filter((entry) => entry.domain)
            .map((entry) => [entry.id, entry.domain!]),
        ),
      });
      setSelected(current.filter((model) => found.includes(model)));
    } catch (error) {
      if (
        request !== sequence.current ||
        controller.signal.aborted ||
        latest.current !== signature
      )
        return;
      setState({
        status: "error",
        signature,
        models: [],
        error:
          error instanceof ApiError ? error.message : "upstream_models_failed",
      });
    }
  };
  const failure = state.error && errors[state.error];
  const applySelection = (replace = false) => {
    const ids = [...new Set([...(replace ? [] : current), ...selected])];
    const hints = { ...capabilities, ...state.capabilities };
    onChange(
      ids.join(","),
      Object.fromEntries(
        ids.filter((id) => hints[id]).map((id) => [id, hints[id]]),
      ),
    );
    setNotice(
      text(
        "所选模型已填入表单，请保存渠道后生效。",
        "Selected models filled into the form. Save the channel to apply.",
        "أُضيفت النماذج إلى النموذج. احفظ القناة للتطبيق.",
      ),
    );
  };

  return (
    <div className="channel-model-discovery">
      <div className="channel-model-heading">
        <label htmlFor="ch-models">
          {text(
            "模型（逗号分隔）",
            "Models (comma-separated)",
            "النماذج (مفصولة بفواصل)",
          )}
        </label>
        {supported && (
          <Button
            size="small"
            type="button"
            loading={loading}
            disabled={disabled || (isPlugin && !connection.task_plugin_id)}
            onClick={() => void fetchModels()}
          >
            {loading
              ? text("获取中…", "Fetching…", "جارٍ الجلب…")
              : text(
                  "获取上游模型",
                  "Fetch upstream models",
                  "جلب نماذج المزود",
                )}
          </Button>
        )}
      </div>
      <Input
        id="ch-models"
        value={models}
        onChange={(event) => onChange(event.target.value)}
        required
        placeholder="gpt-4.1,claude-sonnet-4-20250514"
      />
      {connection.type === 32 && (
        <p className="channel-model-hint">
          {text(
            "请使用目录中的准确版本 ID 或方舟 Endpoint ID。模型短名不会自动转换；可用所选模型替换旧短名。",
            "Use an exact catalog version ID or Ark Endpoint ID. Short names are not converted automatically; replace old names with selected models.",
            "استخدم معرّف الإصدار الدقيق أو معرّف نقطة Ark. لا تُحوّل الأسماء المختصرة تلقائيًا؛ استبدلها بالنماذج المحددة.",
          )}
        </p>
      )}
      {isPlugin && (
        <>
          <p className="channel-model-hint">
            {text(
              "目录路径可由插件指定，目前读取 OpenAI 格式目录，且仅显示插件支持的模型。上游不提供兼容目录时，可使用插件声明模型或手动填写；声明不代表上游实测可用。",
              "Plugins can specify the catalog path. Discovery reads OpenAI-format catalogs and lists supported models only. If no compatible catalog exists, use declared models or enter them manually. Declarations do not verify upstream availability.",
              "يمكن للإضافة تحديد مسار الدليل. يُقرأ دليل بصيغة OpenAI وتظهر النماذج المدعومة فقط. إن لم يتوفر دليل متوافق، استخدم نماذج الإضافة أو أدخلها يدويًا. التصريح لا يؤكد توفرها لدى المزود.",
            )}
          </p>
          <Button
            size="small"
            disabled={disabled || !declaredModels.length}
            onClick={() => onChange(declaredModels.join(","))}
          >
            {text(
              "使用插件声明模型",
              "Use plugin models",
              "استخدام نماذج الإضافة",
            )}
          </Button>
        </>
      )}
      {!supported && (
        <p className="channel-model-hint">
          {text(
            "当前类型请手动填写模型；获取功能支持 OpenAI 兼容渠道和视频任务插件。",
            "Enter models manually for this type. Discovery supports OpenAI-compatible and video task plugin channels.",
            "أدخل النماذج يدويًا لهذا النوع. الجلب متاح لقنوات OpenAI وإضافات الفيديو.",
          )}
        </p>
      )}
      <div aria-live="polite">
        {loading && (
          <p role="status">
            {text(
              "正在读取上游模型目录…",
              "Reading the upstream model catalog…",
              "جارٍ قراءة دليل نماذج المزود…",
            )}
          </p>
        )}
        {coherent && state.status === "error" && (
          <p role="alert" className="channel-model-error">
            {failure
              ? text(...failure)
              : text(
                  "无法获取模型，请重试。现有模型保持不变。",
                  "Cannot fetch models. Retry; existing models are preserved.",
                  "تعذر جلب النماذج. أعد المحاولة؛ النماذج الحالية محفوظة.",
                )}
          </p>
        )}
        {(state.status === "stale" ||
          (!coherent && state.status !== "idle")) && (
          <p role="status">
            {text(
              "连接配置已变化，请重新获取模型。",
              "Connection settings changed. Fetch models again.",
              "تغيّرت إعدادات الاتصال. أعد جلب النماذج.",
            )}
          </p>
        )}
        {notice && <p role="status">{notice}</p>}
      </div>
      {success && (
        <section
          className="channel-model-results"
          aria-label={text(
            "上游模型选择",
            "Upstream model selection",
            "اختيار نماذج المزود",
          )}
        >
          <p>
            {text(
              `获取到 ${state.models.length} 个模型，已选 ${selected.length} 个。`,
              `Fetched ${state.models.length} models; ${selected.length} selected.`,
              `تم جلب ${state.models.length} نموذجًا؛ المحدد ${selected.length}.`,
            )}
          </p>
          <p className="channel-model-hint">
            {text(
              "目录不代表实测可用。只填入表单，保存后生效；已有模型会保留。",
              "Catalog entries are not tested. Fill the form, then save to apply. Existing models are retained.",
              "عناصر الدليل غير مختبرة. املأ النموذج ثم احفظ للتطبيق. تُحفظ النماذج الحالية.",
            )}
          </p>
          {state.models.length === 0 ? (
            <p>
              {text(
                "上游未返回模型，可重试或手动填写。",
                "Upstream returned no models. Retry or enter models manually.",
                "لم يُرجع المزود نماذج. أعد المحاولة أو أدخلها يدويًا.",
              )}
            </p>
          ) : (
            <>
              <Input
                type="search"
                placeholder={text(
                  "搜索模型…",
                  "Search models…",
                  "ابحث عن نموذج…",
                )}
                aria-label={text(
                  "搜索上游模型",
                  "Search upstream models",
                  "البحث في نماذج المزود",
                )}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                onKeyDown={(event) => {
                  if (
                    event.key === "Enter" &&
                    !event.nativeEvent.isComposing &&
                    event.keyCode !== 229
                  )
                    event.preventDefault();
                }}
              />
              <div className="channel-model-actions">
                <Button
                  size="small"
                  onClick={() =>
                    setSelected([...new Set([...selected, ...filtered])])
                  }
                  disabled={disabled || filtered.length === 0}
                >
                  {text(
                    "选择搜索结果",
                    "Select search results",
                    "اختيار نتائج البحث",
                  )}
                </Button>
                <Button
                  size="small"
                  onClick={() => setSelected([])}
                  disabled={disabled || !selected.length}
                >
                  {text("取消勾选", "Deselect all", "إلغاء تحديد الكل")}
                </Button>
              </div>
              <div className="channel-model-list">
                {filtered.length === 0 && (
                  <p>
                    {text(
                      "没有匹配的模型。",
                      "No matching models.",
                      "لا توجد نماذج مطابقة.",
                    )}
                  </p>
                )}
                {filtered.map((model) => (
                  <label key={model} className="channel-model-option">
                    <input
                      type="checkbox"
                      checked={selected.includes(model)}
                      disabled={disabled}
                      onChange={(event) =>
                        setSelected((previous) =>
                          event.target.checked
                            ? [...previous, model]
                            : previous.filter((item) => item !== model),
                        )
                      }
                    />
                    <span dir="auto">
                      {model}
                      {domains[state.capabilities?.[model] ?? ""]
                        ? ` · ${text(...domains[state.capabilities![model]])}`
                        : ""}
                    </span>
                  </label>
                ))}
              </div>
              <Button
                size="small"
                disabled={disabled || !selected.length}
                onClick={() => applySelection()}
              >
                {text(
                  "添加所选模型",
                  "Add selected models",
                  "إضافة النماذج المحددة",
                )}
              </Button>
              {connection.type === 32 && (
                <Button
                  size="small"
                  disabled={disabled || !selected.length}
                  onClick={() => applySelection(true)}
                >
                  {text(
                    "仅使用所选模型",
                    "Use selected models only",
                    "استخدام النماذج المحددة فقط",
                  )}
                </Button>
              )}
            </>
          )}
          {unlisted.length > 0 && (
            <p className="channel-model-hint">
              {text(
                "现有但未在目录中返回的模型仍保留：",
                "Existing models absent from this catalog are retained:",
                "تُحفظ النماذج الحالية غير الواردة في الدليل:",
              )}{" "}
              <span dir="auto">{unlisted.join(", ")}</span>
            </p>
          )}
        </section>
      )}
    </div>
  );
}
