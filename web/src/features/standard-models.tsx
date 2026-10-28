import { lazy, Suspense, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import {
  usePlazaModelMeta,
  useUpsertModelMeta,
  useDeleteModelMeta,
  type ModelMeta,
} from "../lib/api/plaza-meta";
import { emptyModelMetadata } from "../lib/model-metadata";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import { ApiError } from "../lib/api/http";
import { useSettingsText } from "./settings-text";
import "../components/catalog-management.css";
import "./standard-models.css";

const DemoModelPricing = lazy(() =>
  import("./model-pricing-admin").then((module) => ({
    default: module.ModelPricingAdmin,
  })),
);

type Draft = {
  original: ModelMeta | null;
  row: ModelMeta;
  remove: boolean;
  revision: number;
};
export function StandardModels({ role: _role }: { role: number }) {
  const { text } = useSettingsText(),
    role = useSystemAccess((s) => s.role),
    authenticated = useSystemAccess((s) => s.session.accountId !== null);
  const allowed = authenticated && isAdmin(role),
    [params, setParams] = useSearchParams(),
    demo = params.get("demo") === "1",
    selected = params.get("model");
  const query = usePlazaModelMeta(allowed && !demo),
    upsert = useUpsertModelMeta(),
    remove = useDeleteModelMeta();
  const [search, setSearch] = useState(""),
    [page, setPage] = useState(0),
    [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const selectModel = (id?: string) =>
    setParams((previous) => {
      const next = new URLSearchParams(previous);
      if (id) next.set("model", id);
      else next.delete("model");
      return next;
    });
  if (!allowed)
    return (
      <p role="alert">
        {text(
          "需要实际管理员会话。",
          "An actual administrator session is required.",
          "يلزم تسجيل دخول مسؤول فعلي.",
        )}
      </p>
    );
  if (demo)
    return (
      <>
        <p className="pricing-policy">
          <Link to="/console/model-pricing">
            {text(
              "返回服务器模型目录",
              "Back to server model catalog",
              "العودة إلى فهرس نماذج الخادم",
            )}
          </Link>
        </p>
        <Suspense
          fallback={
            <p role="status">
              {text(
                "正在加载浏览器演示编辑器…",
                "Loading browser Demo editor…",
                "جارٍ تحميل المحرر التجريبي للمتصفح…",
              )}
            </p>
          }
        >
          <DemoModelPricing role={role} modelId={selected ?? undefined} />
        </Suspense>
      </>
    );
  const data = query.isError ? undefined : query.data;
  const rows = data?.models ?? [],
    current = rows.find((row) => row.model === selected),
    filtered = rows.filter((row) =>
      `${row.model} ${row.vendor} ${row.vendor_display}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    );
  const pages = Math.max(1, Math.ceil(filtered.length / 20)),
    currentPage = Math.min(page, pages - 1),
    readable = !!data && query.isSuccess && !query.isError,
    pending = upsert.isPending || remove.isPending;
  const writable =
    !!data && readable && data.revision < Number.MAX_SAFE_INTEGER;
  const conflict = !!draft && (!data || draft.revision !== data.revision);
  const valid =
    !!draft &&
    (draft.remove ||
      (draft.row.model === draft.row.model.trim() &&
        draft.row.model.length > 0 &&
        draft.row.model.length <= 128 &&
        !/[\u0000-\u001f\u007f]/.test(draft.row.model) &&
        [draft.row.context_length, draft.row.max_output].every(
          (v) => Number.isSafeInteger(v) && v >= 0,
        ) &&
        Number.isSafeInteger(draft.row.sort_weight) &&
        (!!draft.original ||
          !rows.some((row) => row.model === draft.row.model))));
  const begin = (row: ModelMeta | null = null, deleting = false) => {
    if (!data || !writable) return;
    upsert.reset();
    remove.reset();
    setError("");
    setNotice("");
    setReview(deleting);
    setDraft({
      original: row,
      row: row ? structuredClone(row) : emptyModelMetadata(),
      remove: deleting,
      revision: data.revision,
    });
  };
  const patch = (value: Partial<ModelMeta>) =>
    setDraft((previous) =>
      previous ? { ...previous, row: { ...previous.row, ...value } } : previous,
    );
  const save = async () => {
    if (!draft || !review || !valid || !writable || conflict) return;
    try {
      if (draft.remove) {
        const result = await remove.mutateAsync({
          model: draft.row.model,
          expected_catalog_revision: draft.revision,
        });
        if (
          result.model !== draft.row.model ||
          result.revision < draft.revision ||
          result.revision > draft.revision + 1
        )
          throw Error("Invalid metadata deletion acknowledgement");
        selectModel();
      } else {
        const result = await upsert.mutateAsync({
          meta: draft.row,
          expected_catalog_revision: draft.revision,
        });
        if (
          result.meta.model !== draft.row.model ||
          result.revision !== draft.revision + 1
        )
          throw Error("Invalid metadata save acknowledgement");
        selectModel(result.meta.model);
      }
      setDraft(null);
      setNotice(
        text(
          "服务器模型元数据已保存。计费倍率和渠道权限请在各自页面核对。",
          "Server model metadata saved. Check billing ratios and channel permissions on their respective pages.",
          "حُفظت بيانات نموذج الخادم. تحقق من معاملات الفوترة وصلاحيات القنوات في صفحاتها.",
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              "模型目录已变化，本次未覆盖。请刷新并重新打开编辑。",
              "The model catalog changed and was not overwritten. Refresh and reopen the editor.",
              "تغيّر فهرس النماذج ولم يُستبدل. حدّث وأعد فتح المحرر.",
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                "服务器拒绝此元数据，草稿已保留，请检查字段。",
                "The server rejected this metadata; the draft is retained. Check the fields.",
                "رفض الخادم هذه البيانات واحتُفظ بالمسودة. تحقق من الحقول.",
              )
            : text(
                "无法确认保存结果，草稿已保留。请刷新核对服务器数据后再操作。",
                "The save result is unconfirmed and the draft is retained. Refresh and check server data before trying again.",
                "تعذر تأكيد الحفظ واحتُفظ بالمسودة. حدّث وتحقق من بيانات الخادم قبل المحاولة مجددًا.",
              ),
      );
    }
  };
  const fields: [
    keyof Pick<
      ModelMeta,
      "vendor" | "vendor_display" | "tags" | "compatible_groups"
    >,
    string,
  ][] = [
    ["vendor", text("厂商标识", "Vendor ID", "معرّف المورّد")],
    [
      "vendor_display",
      text("厂商显示名称", "Vendor display name", "اسم المورّد المعروض"),
    ],
    [
      "tags",
      text(
        "能力标签（逗号分隔）",
        "Capability tags (comma separated)",
        "وسوم القدرات (مفصولة بفواصل)",
      ),
    ],
    [
      "compatible_groups",
      text(
        "兼容分组元数据",
        "Compatible group metadata",
        "بيانات المجموعات المتوافقة",
      ),
    ],
  ];
  const reference = (row: ModelMeta) => (
    <dl className="sm-references">
      {(
        [
          [
            "official_input_per_m",
            text(
              "官方参考输入 USD/百万",
              "Official reference input USD/M",
              "إدخال مرجعي رسمي دولار/مليون",
            ),
          ],
          [
            "official_output_per_m",
            text(
              "官方参考输出 USD/百万",
              "Official reference output USD/M",
              "إخراج مرجعي رسمي دولار/مليون",
            ),
          ],
          [
            "official_cache_read_per_m",
            text(
              "官方缓存读取参考 USD/百万",
              "Official cache-read reference USD/M",
              "مرجع قراءة الذاكرة دولار/مليون",
            ),
          ],
          [
            "official_cache_write_per_m",
            text(
              "官方缓存写入参考 USD/百万",
              "Official cache-write reference USD/M",
              "مرجع كتابة الذاكرة دولار/مليون",
            ),
          ],
          [
            "official_per_request",
            text(
              "官方按次参考 USD",
              "Official per-request reference USD",
              "مرجع رسمي لكل طلب بالدولار",
            ),
          ],
        ] as const
      ).map(([key, label]) => (
        <div key={key}>
          <dt>{label}</dt>
          <dd>
            <bdi>{row[key] ?? "—"}</bdi>
          </dd>
        </div>
      ))}
    </dl>
  );
  return (
    <div className="catalog-management server-model-metadata">
      <header className="cm-heading">
        <div>
          <h1>
            {text(
              "标准模型元数据",
              "Standard model metadata",
              "بيانات النماذج القياسية",
            )}
          </h1>
          <p>
            {text(
              "这里只展示服务器登记的模型身份、参考数据和能力/上下文约束；空库和错误不回退演示模型。计费价格、渠道与密钥权限独立管理。",
              "Only server-registered model identities, reference data and capability/context constraints appear here. Empty storage and errors do not fall back to Demo models. Billing prices, channels and key permissions are managed separately.",
              "تُعرض هنا هويات النماذج وبياناتها المرجعية وقيود القدرات والسياق المسجلة في الخادم فقط. لا تُستبدل البيانات الفارغة أو الأخطاء بنماذج تجريبية. تُدار أسعار الفوترة والقنوات وصلاحيات المفاتيح منفصلة.",
            )}
          </p>
        </div>
        <div className="cm-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text("刷新模型", "Refresh models", "تحديث النماذج")}
          </Button>
          <Button
            variant="primary"
            disabled={!writable}
            onClick={() => begin()}
          >
            {text(
              "登记模型元数据",
              "Register model metadata",
              "تسجيل بيانات نموذج",
            )}
          </Button>
        </div>
      </header>
      <div className="cm-actions">
        <Link to="/admin?tab=resources">
          {text(
            "服务器计费定价与配置同步",
            "Server billing prices and configuration sync",
            "أسعار فوترة الخادم ومزامنة الإعدادات",
          )}
        </Link>
        <Link to="/console/channels">
          {text(
            "渠道与资源清单",
            "Channels and resource inventory",
            "القنوات وقائمة الموارد",
          )}
        </Link>
        <Link to="/console/model-pricing?demo=1">
          {text(
            "浏览器演示定价（独立）",
            "Browser Demo pricing (separate)",
            "تسعير المتصفح التجريبي (مستقل)",
          )}
        </Link>
      </div>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取服务器模型…",
            "Loading server models…",
            "جارٍ تحميل نماذج الخادم…",
          )}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "无法确认服务器模型元数据或版本保护能力，已隐藏缓存并暂停编辑。",
            "Cannot verify server model metadata or revision-guard support. Cached data is hidden and editing is paused.",
            "تعذر التحقق من بيانات نماذج الخادم أو حماية الإصدار. أُخفيت البيانات السابقة وأُوقف التحرير.",
          )}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {data && (
        <p>
          {text("模型目录版本", "Model catalog version", "إصدار فهرس النماذج")}:{" "}
          <bdi>v{data.revision}</bdi>
        </p>
      )}
      {readable &&
        (selected ? (
          <section className="pricing-panel">
            <div className="cm-actions">
              <Button onClick={() => selectModel()}>
                {text(
                  "返回服务器模型列表",
                  "Back to server model list",
                  "العودة إلى قائمة نماذج الخادم",
                )}
              </Button>
              {current && (
                <>
                  <Button disabled={!writable} onClick={() => begin(current)}>
                    {text("编辑元数据", "Edit metadata", "تحرير البيانات")}
                  </Button>
                  <Button
                    disabled={!writable}
                    onClick={() => begin(current, true)}
                  >
                    {text("删除元数据", "Delete metadata", "حذف البيانات")}
                  </Button>
                </>
              )}
            </div>
            {current ? (
              <>
                <h2>
                  <bdi>{current.model}</bdi>
                </h2>
                <p>{current.vendor_display || current.vendor || "—"}</p>
                <p>{current.description || "—"}</p>
                <p>
                  {text("能力标签", "Capability tags", "وسوم القدرات")}:{" "}
                  <bdi>{current.tags || "—"}</bdi>
                </p>
                <p>
                  {text(
                    "上下文 / 最大输出（0 表示未知）",
                    "Context / max output (0 means unknown)",
                    "السياق / أقصى إخراج (الصفر يعني غير معروف)",
                  )}
                  :{" "}
                  <bdi>
                    {current.context_length} / {current.max_output}
                  </bdi>
                </p>
                {reference(current)}
              </>
            ) : (
              <p>
                {text(
                  "服务器没有此模型的登记元数据。",
                  "No metadata is registered for this model on the server.",
                  "لم تُسجل بيانات هذا النموذج في الخادم.",
                )}
              </p>
            )}
          </section>
        ) : (
          <>
            <label className="cm-search">
              {text(
                "搜索服务器模型",
                "Search server models",
                "البحث في نماذج الخادم",
              )}
              <input
                aria-label={text(
                  "搜索服务器模型",
                  "Search server models",
                  "البحث في نماذج الخادم",
                )}
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setPage(0);
                }}
              />
            </label>
            <div
              className="cm-table"
              tabIndex={0}
              role="region"
              aria-label={text(
                "服务器模型表，可横向滚动",
                "Server model table, horizontally scrollable",
                "جدول نماذج الخادم، قابل للتمرير أفقيًا",
              )}
            >
              <table>
                <thead>
                  <tr>
                    <th>{text("模型标识", "Model ID", "معرّف النموذج")}</th>
                    <th>{text("厂商", "Vendor", "المورّد")}</th>
                    <th>
                      {text(
                        "能力元数据",
                        "Capability metadata",
                        "بيانات القدرات",
                      )}
                    </th>
                    <th>
                      {text(
                        "上下文 / 最大输出",
                        "Context / max output",
                        "السياق / أقصى إخراج",
                      )}
                    </th>
                    <th>{text("操作", "Action", "الإجراء")}</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered
                    .slice(currentPage * 20, (currentPage + 1) * 20)
                    .map((row) => (
                      <tr key={row.model} data-server-model={row.model}>
                        <td>
                          <button
                            className="cm-text-button"
                            onClick={() => selectModel(row.model)}
                          >
                            <bdi>{row.model}</bdi>
                          </button>
                        </td>
                        <td>{row.vendor_display || row.vendor || "—"}</td>
                        <td>
                          <bdi>{row.tags || "—"}</bdi>
                        </td>
                        <td>
                          <bdi>
                            {row.context_length || "—"} /{" "}
                            {row.max_output || "—"}
                          </bdi>
                        </td>
                        <td>
                          <div className="cm-actions">
                            <Button
                              size="small"
                              disabled={!writable}
                              onClick={() => begin(row)}
                            >
                              {text(
                                "编辑元数据",
                                "Edit metadata",
                                "تحرير البيانات",
                              )}
                            </Button>
                            <Button
                              size="small"
                              disabled={!writable}
                              onClick={() => begin(row, true)}
                            >
                              {text(
                                "删除元数据",
                                "Delete metadata",
                                "حذف البيانات",
                              )}
                            </Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
            {!rows.length && (
              <p>
                {text(
                  "服务器尚未登记模型元数据；不会自动导入演示目录。",
                  "No model metadata is registered on the server; the Demo catalog is not imported automatically.",
                  "لم تُسجل بيانات نماذج في الخادم ولا يُستورد الفهرس التجريبي تلقائيًا.",
                )}
              </p>
            )}
            <div className="cm-actions">
              <Button
                disabled={currentPage === 0}
                onClick={() => setPage(currentPage - 1)}
              >
                {text("上一页", "Previous", "السابق")}
              </Button>
              <span>
                {currentPage + 1} / {pages} · {filtered.length}
              </span>
              <Button
                disabled={currentPage + 1 >= pages}
                onClick={() => setPage(currentPage + 1)}
              >
                {text("下一页", "Next", "التالي")}
              </Button>
            </div>
          </>
        ))}
      <Modal
        className="server-model-dialog"
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !pending) setDraft(null);
        }}
        preventOutsideClose
        title={
          draft?.remove
            ? text(
                "删除服务器模型元数据",
                "Delete server model metadata",
                "حذف بيانات نموذج الخادم",
              )
            : text(
                "编辑服务器模型元数据",
                "Edit server model metadata",
                "تحرير بيانات نموذج الخادم",
              )
        }
        description={text(
          "能力标签和上下文约束会影响智能候选筛选。此处不修改计费选项，不授予调用权限。",
          "Capability tags and context constraints affect smart-routing candidates. These edits do not change billing options or grant calling rights.",
          "تؤثر وسوم القدرات وقيود السياق في مرشحي التوجيه الذكي. لا تغيّر هذه التعديلات خيارات الفوترة ولا تمنح حق الاستدعاء.",
        )}
        footer={
          <>
            <Button disabled={pending} onClick={() => setDraft(null)}>
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            <Button
              variant="primary"
              disabled={!valid || !writable || conflict || pending}
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(
                    "确认服务器元数据",
                    "Confirm server metadata",
                    "تأكيد بيانات الخادم",
                  )
                : text(
                    "预览元数据变更",
                    "Review metadata changes",
                    "مراجعة تغييرات البيانات",
                  )}
            </Button>
          </>
        }
      >
        {draft && (
          <p>
            {text(
              "草稿目录版本",
              "Draft catalog version",
              "إصدار فهرس المسودة",
            )}
            : <bdi>v{draft.revision}</bdi>
          </p>
        )}
        {draft &&
          (review ? (
            <div className="sm-review">
              <h3>
                <bdi>{draft.row.model}</bdi>
              </h3>
              {draft.remove ? (
                <p>
                  {text(
                    "仅删除这条模型元数据；不删除渠道、密钥、历史记录或计费价格。缺少能力/上下文元数据可能使部分智能候选被拒绝。",
                    "Deletes this model metadata only. Channels, keys, historical records and billing prices remain. Missing capability/context metadata can exclude some smart-routing candidates.",
                    "يحذف بيانات هذا النموذج فقط دون القنوات أو المفاتيح أو السجلات أو أسعار الفوترة. قد يستبعد غياب بيانات القدرات أو السياق بعض مرشحي التوجيه الذكي.",
                  )}
                </p>
              ) : (
                <>
                  <p>{draft.row.vendor_display || draft.row.vendor || "—"}</p>
                  <p>
                    <bdi>{draft.row.tags || "—"}</bdi>
                  </p>
                  <p>
                    {text(
                      "上下文 / 最大输出",
                      "Context / max output",
                      "السياق / أقصى إخراج",
                    )}
                    :{" "}
                    <bdi>
                      {draft.row.context_length} / {draft.row.max_output}
                    </bdi>
                  </p>
                  <p>{draft.row.description || "—"}</p>
                  <p>
                    {text(
                      "兼容分组 / 排序权重",
                      "Compatible groups / sort weight",
                      "المجموعات المتوافقة / وزن الترتيب",
                    )}
                    :{" "}
                    <bdi>
                      {draft.row.compatible_groups || "—"} /{" "}
                      {draft.row.sort_weight}
                    </bdi>
                  </p>
                  <p>
                    {text(
                      "现有官方参考价格、评分及其他未编辑元数据原样保留；未登记值保持未知。",
                      "Existing official references, score and other unedited metadata are retained. Unregistered values remain unknown.",
                      "تُحفظ المراجع الرسمية والتقييم والبيانات غير المعدلة، وتبقى القيم غير المسجلة غير معروفة.",
                    )}
                  </p>
                  <Button
                    disabled={pending}
                    onClick={() => {
                      setReview(false);
                      setError("");
                    }}
                  >
                    {text("返回编辑", "Back to edit", "العودة للتحرير")}
                  </Button>
                </>
              )}
            </div>
          ) : (
            <div className="cm-form">
              <label className="cm-wide">
                {text(
                  "服务器模型标识",
                  "Server model ID",
                  "معرّف نموذج الخادم",
                )}
                <input
                  dir="ltr"
                  readOnly={!!draft.original}
                  maxLength={128}
                  value={draft.row.model}
                  onChange={(e) => patch({ model: e.target.value })}
                />
              </label>
              {fields.map(([key, label]) => (
                <label key={key}>
                  {label}
                  <input
                    value={draft.row[key]}
                    maxLength={
                      key === "vendor"
                        ? 64
                        : key === "vendor_display"
                          ? 128
                          : 256
                    }
                    onChange={(e) => patch({ [key]: e.target.value })}
                  />
                </label>
              ))}
              {(
                [
                  [
                    "context_length",
                    text(
                      "上下文上限（0 表示未知）",
                      "Context limit (0 means unknown)",
                      "حد السياق (الصفر يعني غير معروف)",
                    ),
                  ],
                  [
                    "max_output",
                    text(
                      "最大输出（0 表示未知）",
                      "Max output (0 means unknown)",
                      "أقصى إخراج (الصفر يعني غير معروف)",
                    ),
                  ],
                  [
                    "sort_weight",
                    text("排序权重", "Sort weight", "وزن الترتيب"),
                  ],
                ] as const
              ).map(([key, label]) => (
                <label key={key}>
                  {label}
                  <input
                    type="number"
                    step="1"
                    value={draft.row[key]}
                    onChange={(e) => patch({ [key]: Number(e.target.value) })}
                  />
                </label>
              ))}
              <label className="cm-wide">
                {text("模型说明", "Model description", "وصف النموذج")}
                <textarea
                  aria-label={text(
                    "模型说明",
                    "Model description",
                    "وصف النموذج",
                  )}
                  maxLength={8192}
                  value={draft.row.description}
                  onChange={(e) => patch({ description: e.target.value })}
                />
              </label>
              <div className="cm-wide">{reference(draft.row)}</div>
            </div>
          ))}
        {conflict && draft && (
          <p role="alert">
            {text(
              "目录版本已变化或读取不可用，请刷新并重新打开编辑。",
              "The catalog changed or cannot be read. Refresh and reopen the editor.",
              "تغيّر الفهرس أو تعذرت قراءته. حدّث وأعد فتح المحرر.",
            )}
          </p>
        )}
        {!readable && draft && (
          <p role="alert">
            {text(
              "服务器读取不可用，已暂停保存。",
              "Server data cannot be read; saving is paused.",
              "تعذرت قراءة بيانات الخادم؛ أُوقف الحفظ.",
            )}
          </p>
        )}
        {error && <p role="alert">{error}</p>}
      </Modal>
    </div>
  );
}
