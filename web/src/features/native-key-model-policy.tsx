import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, Search, Sparkles } from "lucide-react";
import { SMART_MODEL_ID } from "../lib/smart-model";
import type { TokenChoices } from "../lib/api/token-choices";
import "./native-key-model-policy.css";

export type NativeModelScope = "snapshot" | "custom";
export type NativePolicyError = "models" | "candidates" | null;

export function NativeKeyModelPolicy({
  choices,
  group,
  available,
  selected,
  scope,
  onScopeChange,
  onModelsChange,
  locked,
  existing,
  legacyUnrestricted,
  smartEnabled,
  onSmartChange,
  smartAvailable,
  smartSelected,
  onCandidatesChange,
  error,
  validationAttempt,
}: {
  choices: TokenChoices["choices"];
  group: string;
  available: string[];
  selected: string[];
  scope: NativeModelScope;
  onScopeChange: (scope: NativeModelScope) => void;
  onModelsChange: (models: string[]) => void;
  locked: boolean;
  existing: boolean;
  legacyUnrestricted: boolean;
  smartEnabled: boolean;
  onSmartChange: (enabled: boolean) => void;
  smartAvailable: string[];
  smartSelected: string[];
  onCandidatesChange: (models: string[] | null) => void;
  error: NativePolicyError;
  validationAttempt: number;
}) {
  const { i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [candidatesOpen, setCandidatesOpen] = useState(false);
  const candidateField = useRef<HTMLFieldSetElement>(null);
  const modelField = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!error) return;
    setOpen(true);
    if (error === "candidates") setCandidatesOpen(true);
    const frame = requestAnimationFrame(() =>
      (error === "candidates"
        ? candidateField.current
        : modelField.current
      )?.focus(),
    );
    return () => cancelAnimationFrame(frame);
  }, [error, validationAttempt]);

  const ids = [
    ...new Set([
      ...(locked ? [] : choices.map((choice) => choice.model)),
      ...selected,
    ]),
  ];
  const rows = ids
    .map((id) => {
      const info =
        choices.find(
          (choice) => choice.group === group && choice.model === id,
        ) ?? choices.find((choice) => choice.model === id);
      const vendor =
        info?.vendor && info.vendor !== "unknown"
          ? info.vendor_display || info.vendor
          : c("其他", "Other", "أخرى");
      return {
        id,
        info,
        vendor,
        available: available.includes(id),
        retained: selected.includes(id),
      };
    })
    .filter((row) =>
      `${row.id} ${row.vendor}`
        .toLocaleLowerCase()
        .includes(search.trim().toLocaleLowerCase()),
    );
  const vendors = [...new Set(rows.map((row) => row.vendor))].sort((a, b) =>
    a.localeCompare(b),
  );
  const capability = (
    tags: string[] | undefined,
    smart: boolean | undefined,
  ) => {
    if (tags?.includes("video")) return c("视频生成", "Video", "فيديو");
    if (tags?.includes("image")) return c("图像生成", "Images", "صور");
    if (tags?.some((tag) => ["audio", "tts", "stt"].includes(tag)))
      return c("音频", "Audio", "صوت");
    if (tags?.some((tag) => ["embedding", "embeddings"].includes(tag)))
      return c("向量嵌入", "Embeddings", "تضمين");
    if (tags?.includes("rerank"))
      return c("重排序", "Reranking", "إعادة ترتيب");
    return smart ? c("文本对话", "Text chat", "محادثة نصية") : "—";
  };
  return (
    <section className="native-key-model-policy">
      <div className="native-key-policy-heading">
        <div>
          <h3 id="native-key-policy-title">
            {c("模型权限", "Model permissions", "أذونات النماذج")}
          </h3>
          <p>
            {c(
              "控制密钥可调用的模型，明确指定模型时不会被自动替换。",
              "Control which models this key can call. Explicitly requested models are never substituted.",
              "تحكم في النماذج التي يمكن للمفتاح استخدامها. لا يُستبدل النموذج المحدد صراحةً.",
            )}
          </p>
        </div>
        <button
          type="button"
          className="native-key-policy-disclosure"
          aria-expanded={open}
          aria-controls="native-key-policy-body"
          onClick={() => setOpen((value) => !value)}
        >
          {open ? c("收起", "Collapse", "طي") : c("展开", "Expand", "توسيع")}
          <ChevronDown size={14} aria-hidden="true" />
        </button>
      </div>
      <div
        id="native-key-policy-body"
        className="native-key-policy-body"
        hidden={!open}
      >
        <div
          className="native-key-model-scope segmented"
          role="group"
          aria-label={c(
            "授权模型范围",
            "Model authorization scope",
            "نطاق أذونات النماذج",
          )}
        >
          <button
            id="native-key-scope-snapshot"
            type="button"
            aria-pressed={scope === "snapshot"}
            disabled={locked}
            onClick={() => onScopeChange("snapshot")}
          >
            {c(
              "保存时的全部模型",
              "All models at save",
              "كل النماذج عند الحفظ",
            )}
          </button>
          <button
            id="native-key-scope-custom"
            type="button"
            aria-pressed={scope === "custom"}
            disabled={locked}
            onClick={() => onScopeChange("custom")}
          >
            {c("指定模型", "Select models", "تحديد النماذج")}
          </button>
        </div>
        {locked && (
          <p className="native-key-policy-hint" role="status">
            {c(
              "保留现有自动分组及模型授权。可直接修改名称、额度和 IP 限制；选择具体分组才会更改路由授权。",
              "Existing automatic groups and model permissions are preserved. Edit the name, limit or IP restrictions directly; selecting a specific group changes routing permissions.",
              "تُحفظ المجموعات التلقائية وأذونات النماذج الحالية. يمكنك تعديل الاسم أو الحد أو قيود IP؛ اختيار مجموعة محددة يغيّر أذونات التوجيه.",
            )}
          </p>
        )}
        {scope === "snapshot" ? (
          <div
            ref={modelField}
            tabIndex={-1}
            className="native-key-policy-hint"
          >
            <p>
              {c(
                "保存时授权当前分组的全部可用模型，并保留本次授权快照。此后新增模型仍需明确更新授权。",
                "Authorize the current group's available models as an explicit snapshot. Future models require an explicit permission update.",
                "امنح صلاحية النماذج المتاحة في المجموعة الحالية كلقطة محددة. النماذج الجديدة تتطلب تحديثًا صريحًا للأذونات.",
              )}
            </p>
            <span>
              {c(
                `本次授权 ${selected.length} 个模型`,
                `${selected.length} models in this snapshot`,
                `${selected.length} نماذج في هذه اللقطة`,
              )}
            </span>
            {existing && (
              <button
                type="button"
                className="native-key-policy-link"
                disabled={locked}
                onClick={() => onScopeChange("snapshot")}
              >
                {c("更新授权快照", "Update snapshot", "تحديث اللقطة")}
              </button>
            )}
          </div>
        ) : (
          <div
            ref={modelField}
            tabIndex={-1}
            className="native-key-model-picker"
          >
            <div className="native-key-model-toolbar">
              <label className="native-key-model-search">
                <Search size={15} aria-hidden="true" />
                <input
                  id="native-key-model-search"
                  type="search"
                  value={search}
                  aria-label={c("搜索模型", "Search models", "البحث عن نموذج")}
                  placeholder={c("搜索模型", "Search models", "البحث عن نموذج")}
                  onChange={(event) => setSearch(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") event.preventDefault();
                  }}
                />
              </label>
              <span className="native-key-model-count">
                {c(
                  `已选 ${selected.length}`,
                  `${selected.length} selected`,
                  `${selected.length} محدد`,
                )}
              </span>
            </div>
            <div
              className="native-key-model-list"
              role="group"
              aria-label={c("可选模型", "Available models", "النماذج المتاحة")}
            >
              {vendors.map((vendor) => (
                <div key={vendor} role="group" aria-label={vendor}>
                  <div className="native-key-model-vendor">{vendor}</div>
                  {rows
                    .filter((row) => row.vendor === vendor)
                    .map((row) => (
                      <label
                        key={row.id}
                        className={`native-key-model-row${!row.available && !locked ? " is-unavailable" : ""}`}
                      >
                        <input
                          type="checkbox"
                          aria-label={row.id}
                          checked={row.retained}
                          disabled={locked || (!row.available && !row.retained)}
                          onChange={(event) =>
                            onModelsChange(
                              event.target.checked
                                ? [...selected, row.id]
                                : selected.filter((id) => id !== row.id),
                            )
                          }
                        />
                        <code dir="ltr">{row.id}</code>
                        <span>
                          {locked
                            ? c(
                                "保留现有授权",
                                "Retained permission",
                                "صلاحية محفوظة",
                              )
                            : row.available
                              ? capability(
                                  row.info?.tags,
                                  row.info?.smart_eligible,
                                )
                              : row.info
                                ? c(
                                    `${group} 暂不可用`,
                                    `Unavailable in ${group}`,
                                    `غير متاح في ${group}`,
                                  )
                                : c("已下架", "Unavailable", "غير متاح")}
                        </span>
                      </label>
                    ))}
                </div>
              ))}
              {!rows.length && (
                <p className="native-key-model-empty">
                  {c(
                    "没有匹配的模型。",
                    "No matching models.",
                    "لا توجد نماذج مطابقة.",
                  )}
                </p>
              )}
            </div>
          </div>
        )}
        {legacyUnrestricted && (
          <p className="native-key-policy-hint">
            {c(
              "仅编辑其他字段会保留既有授权。主动选择范围或模型后，才按新列表保存权限。",
              "Editing other fields preserves the existing policy. Permissions change only after explicitly choosing a scope or models.",
              "تعديل الحقول الأخرى يحفظ الأذونات الحالية. تتغير فقط بعد اختيار نطاق أو نماذج صراحةً.",
            )}
          </p>
        )}
        {error === "models" && (
          <p className="native-key-policy-error" role="alert">
            {c(
              "请选择至少一个当前分组可用的授权模型。",
              "Select at least one available model in this group.",
              "اختر نموذجًا متاحًا واحدًا على الأقل في هذه المجموعة.",
            )}
          </p>
        )}
        <div
          className={`native-key-smart-card${smartEnabled ? " is-enabled" : ""}`}
        >
          <div className="native-key-smart-heading">
            <span className="native-key-smart-icon" aria-hidden="true">
              <Sparkles size={18} />
            </span>
            <div className="native-key-smart-title">
              <span>{c("智能入口", "Smart entry", "المدخل الذكي")}</span>
              <code dir="ltr">{SMART_MODEL_ID}</code>
            </div>
            <button
              id="native-key-smart-access"
              type="button"
              role="switch"
              aria-checked={smartEnabled}
              aria-label={c(
                "允许智能路由入口",
                "Allow smart routing",
                "السماح بالتوجيه الذكي",
              )}
              disabled={locked}
              className="native-key-smart-switch"
              onClick={() => onSmartChange(!smartEnabled)}
            >
              <span />
            </button>
          </div>
          <div className="native-key-smart-content">
            {smartEnabled ? (
              <p>
                {c(
                  "请求中 model（模型字段）填 ",
                  "Set the request model to ",
                  "اضبط حقل model في الطلب على ",
                )}
                <code dir="ltr">{SMART_MODEL_ID}</code>
                {c(
                  " 时，在授权范围内按输入与输出单价之和选模，再按实际用量计费。同一把密钥仍可直接指定具体模型。",
                  " to choose within the authorized scope by combined input and output rates, then bill actual usage. This key can also call specific models directly.",
                  " للاختيار ضمن النطاق المصرح حسب مجموع أسعار الإدخال والإخراج، ثم احتساب الاستخدام الفعلي. يمكن للمفتاح أيضًا طلب نموذج محدد مباشرةً.",
                )}
              </p>
            ) : (
              <p>
                {c(
                  "智能入口已关闭。仍可在授权范围内直接指定模型调用。",
                  "Smart entry is off. This key can still call explicitly authorized models directly.",
                  "المدخل الذكي معطل. لا يزال بإمكان المفتاح طلب النماذج المصرح بها مباشرةً.",
                )}
              </p>
            )}
            {smartEnabled && (
              <>
                <div className="native-key-smart-details">
                  <p>
                    {c("策略：", "Strategy: ", "الاستراتيجية: ")}
                    <strong>{c("最省", "Lowest cost", "الأقل تكلفة")}</strong>
                  </p>
                  <div className="native-key-candidate-summary">
                    <span>
                      {smartSelected.length === smartAvailable.length
                        ? c(
                            `候选模型：授权范围内全部文本模型（${smartSelected.length} 个）`,
                            `Candidates: all authorized text models (${smartSelected.length})`,
                            `المرشحون: كل النماذج النصية المصرح بها (${smartSelected.length})`,
                          )
                        : c(
                            `候选模型：已选择 ${smartSelected.length} 个文本模型`,
                            `Candidates: ${smartSelected.length} selected text models`,
                            `المرشحون: ${smartSelected.length} نماذج نصية محددة`,
                          )}
                    </span>
                    <button
                      id="native-key-candidates-toggle"
                      type="button"
                      className="native-key-policy-link"
                      aria-expanded={candidatesOpen}
                      aria-controls="native-key-candidate-fields"
                      onClick={() => setCandidatesOpen((value) => !value)}
                    >
                      {candidatesOpen
                        ? c("收起候选", "Hide candidates", "إخفاء المرشحين")
                        : c("收窄候选", "Narrow candidates", "تضييق المرشحين")}
                    </button>
                  </div>
                </div>
                <div id="native-key-candidate-fields" hidden={!candidatesOpen}>
                  <fieldset
                    ref={candidateField}
                    tabIndex={-1}
                    className="native-key-candidate-list"
                  >
                    <legend>
                      {c(
                        "智能候选模型",
                        "Smart candidate models",
                        "النماذج المرشحة للتوجيه الذكي",
                      )}
                    </legend>
                    <button
                      type="button"
                      className="native-key-policy-link"
                      disabled={locked || smartAvailable.length === 0}
                      onClick={() => onCandidatesChange(null)}
                    >
                      {c(
                        "使用全部授权文本模型",
                        "Use all authorized text models",
                        "استخدام كل النماذج النصية المصرح بها",
                      )}
                    </button>
                    {smartAvailable.map((id) => (
                      <label key={id}>
                        <input
                          type="checkbox"
                          aria-label={id}
                          checked={smartSelected.includes(id)}
                          disabled={locked}
                          onChange={(event) =>
                            onCandidatesChange(
                              event.target.checked
                                ? [...smartSelected, id]
                                : smartSelected.filter((model) => model !== id),
                            )
                          }
                        />
                        <code dir="ltr">{id}</code>
                      </label>
                    ))}
                    {!smartAvailable.length && (
                      <p role="status">
                        {c(
                          "当前没有已授权的文本候选模型。",
                          "No authorized text candidates are available.",
                          "لا تتوفر نماذج نصية مرشحة مصرح بها.",
                        )}
                      </p>
                    )}
                  </fieldset>
                </div>
                {error === "candidates" && (
                  <p className="native-key-policy-error" role="alert">
                    {c(
                      "开启智能入口时须选择至少一个已授权的文本模型。",
                      "Select at least one authorized text model for smart routing.",
                      "اختر نموذجًا نصيًا مصرحًا به على الأقل للتوجيه الذكي.",
                    )}
                  </p>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
