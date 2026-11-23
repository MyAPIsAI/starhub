import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import {
  previewResourcePublication,
  usePublicationContext,
} from "../lib/api/resource-publication";
import { ApiError } from "../lib/api/http";
import type { PublicationPreview } from "../lib/resource-publication";
import { PublicationCommit } from "./resource-publication-commit";
import {
  PairTable,
  IssueList,
  usePublicationIssueLabel,
} from "./resource-publication-review";
import { useSettingsText } from "./settings-text";

export function CurrentChannelBindingDialog({
  open,
  onOpenChange,
  onUseSnapshot,
  onCommitted,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onUseSnapshot: () => void;
  onCommitted: (review: PublicationPreview) => void;
}) {
  const { text } = useSettingsText();
  const issueLabel = usePublicationIssueLabel();
  const query = usePublicationContext(open);
  const context = query.error ? undefined : query.data;
  const [sourceID, setSourceID] = useState("");
  const [targetID, setTargetID] = useState("");
  const [models, setModels] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [preview, setPreview] = useState<PublicationPreview>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const active = useRef<AbortController>();
  const afterClose = useRef<() => void>();
  const result = useRef<HTMLDivElement>(null);
  const source = context?.channels.find((ch) => ch.id === sourceID);
  const target = context?.channels.find((ch) => ch.id === targetID);
  const availableModels = source?.models ?? [];
  const visibleModels = availableModels.filter((model) =>
    model.toLowerCase().includes(search.trim().toLowerCase()),
  );
  const validSelection =
    source?.status === 1 &&
    target?.status === 1 &&
    models.length > 0 &&
    models.every((model) => availableModels.includes(model));
  const stale = Boolean(
    preview &&
    (!context ||
      preview.resource_revision !== context.resource_revision ||
      preview.channel_revision !== context.channel_revision ||
      preview.option_version !== context.option_version),
  );
  const reset = () => {
    active.current?.abort();
    active.current = undefined;
    setBusy(false);
    setPreview(undefined);
    setError("");
  };
  useEffect(() => () => active.current?.abort(), []);
  useEffect(() => {
    if (!open) return;
    active.current?.abort();
    active.current = undefined;
    setBusy(false);
    setPreview(undefined);
    setError("");
    void query.refetch();
  }, [open, query.refetch]);
  useEffect(() => {
    if (preview || error) result.current?.focus();
  }, [preview, error]);
  const close = () => {
    reset();
    onOpenChange(false);
  };
  const run = async () => {
    if (!context || !validSelection || query.isFetching) return;
    reset();
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    try {
      const review = await previewResourcePublication(
        context,
        [
          {
            source_channel_id: `channel:${sourceID}`,
            gateway_channel_id: targetID,
            source_models: [...models],
          },
        ],
        controller.signal,
      );
      if (active.current === controller) setPreview(review);
    } catch (cause) {
      if (active.current === controller && !controller.signal.aborted) {
        setError(
          cause instanceof ApiError && cause.status === 409
            ? text(
                "渠道或配置已变化，请刷新后重新预览。",
                "Channels or configuration changed. Refresh and preview again.",
                "تغيرت القنوات أو الإعدادات. حدّث وأعد المعاينة.",
              )
            : text(
                "无法生成预览，请刷新渠道数据后重试。",
                "Could not generate a preview. Refresh channel data and retry.",
                "تعذر إنشاء المعاينة. حدّث بيانات القنوات وأعد المحاولة.",
              ),
        );
      }
    } finally {
      if (active.current === controller) {
        active.current = undefined;
        setBusy(false);
      }
    }
  };
  return (
    <Modal
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      onAfterClose={() => {
        const action = afterClose.current;
        afterClose.current = undefined;
        action?.();
      }}
      size="wide"
      className="rp-create-dialog"
      preventOutsideClose
      title={text("新建持久绑定", "New persistent binding", "إنشاء ربط دائم")}
      description={text(
        "直接选择当前渠道及已录入模型，无需导入资源快照。",
        "Choose current channels and configured models without importing a resource snapshot.",
        "اختر القنوات الحالية والنماذج المسجلة دون استيراد لقطة موارد.",
      )}
      footer={
        <>
          <Button onClick={close}>
            {text(
              "关闭（不发布）",
              "Close without publishing",
              "إغلاق دون نشر",
            )}
          </Button>
          <Button
            variant={preview?.ready && !stale ? "secondary" : "primary"}
            disabled={!context || !validSelection || busy || query.isFetching}
            onClick={() => void run()}
          >
            {busy
              ? text(
                  "正在生成预览…",
                  "Generating preview…",
                  "جارٍ إنشاء المعاينة…",
                )
              : text(
                  "生成服务器预览",
                  "Generate server preview",
                  "إنشاء معاينة الخادم",
                )}
          </Button>
          <PublicationCommit
            publishLabel={text("发布绑定", "Publish binding", "نشر الربط")}
            showReceipt={false}
            review={preview && !stale ? preview : undefined}
            disabled={
              stale ||
              busy ||
              query.isFetching ||
              context?.publish_available !== true
            }
            renderRoutes={(review) => (
              <PairTable
                title={text(
                  "确认发布组合",
                  "Confirmed publication pairs",
                  "أزواج النشر المؤكدة",
                )}
                rows={review.proposed_routes}
                showConstraints
              />
            )}
            onCommitted={(review) => {
              afterClose.current = () => onCommitted(review);
              setSourceID("");
              setTargetID("");
              setModels([]);
              setSearch("");
              close();
            }}
          />
        </>
      }
    >
      <div className="rp-direct-source">
        <p className="rp-result-status" role="status">
          {text(
            "选择渠道和模型后，先生成预览，再点击“发布绑定”并确认，才会写入下方列表。关闭窗口只保留选择，不会创建绑定。",
            "Choose a channel and models, generate a preview, then publish and confirm to add the binding to the list. Closing retains your selections without creating a binding.",
            "اختر قناة ونماذج، ثم أنشئ معاينة وانشر وأكّد لإضافة الربط إلى القائمة. يحتفظ الإغلاق باختياراتك دون إنشاء ربط.",
          )}
        </p>
        <Button
          disabled={query.isFetching}
          onClick={() => {
            reset();
            void query.refetch();
          }}
        >
          {text("刷新渠道数据", "Refresh channel data", "تحديث بيانات القنوات")}
        </Button>
        {query.isFetching && (
          <p role="status">
            {text(
              "正在读取渠道数据…",
              "Loading channel data…",
              "جارٍ تحميل بيانات القنوات…",
            )}
          </p>
        )}
        {query.error && (
          <p role="alert">
            {text(
              "无法读取渠道数据，请刷新重试。",
              "Could not load channel data. Refresh to retry.",
              "تعذر تحميل بيانات القنوات. حدّث للمحاولة مجددًا.",
            )}
          </p>
        )}
        {context && context.channels.length === 0 && (
          <p role="status">
            {text(
              "尚未录入渠道，请先在渠道管理中新增渠道及模型。",
              "No channels are configured. Add a channel and its models first.",
              "لا توجد قنوات مسجلة. أضف قناة ونماذجها أولًا.",
            )}
          </p>
        )}
        {context && context.channels.length > 0 && (
          <>
            <label>
              {text("来源渠道", "Source channel", "قناة المصدر")}
              <select
                aria-label={text("来源渠道", "Source channel", "قناة المصدر")}
                value={sourceID}
                onChange={(e) => {
                  reset();
                  if (!targetID || targetID === sourceID)
                    setTargetID(e.target.value);
                  setSourceID(e.target.value);
                  setModels([]);
                  setSearch("");
                }}
              >
                <option value="">
                  {text("请选择渠道", "Select a channel", "اختر قناة")}
                </option>
                {context.channels.map((ch) => (
                  <option key={ch.id} value={ch.id} disabled={ch.status !== 1}>
                    {ch.name} · {ch.id}
                    {ch.status !== 1
                      ? ` · ${text("已停用", "Disabled", "معطلة")}`
                      : ""}
                  </option>
                ))}
              </select>
            </label>
            {source && availableModels.length === 0 && (
              <p role="status">
                {text(
                  "该渠道尚未录入模型，请先配置模型。",
                  "This channel has no configured models. Add models first.",
                  "لم تُسجل نماذج لهذه القناة. أضف النماذج أولًا.",
                )}
              </p>
            )}
            {source && availableModels.length > 0 && (
              <fieldset className="rp-model-picker">
                <legend>
                  {text(
                    "选择模型资源",
                    "Select model resources",
                    "اختيار موارد النماذج",
                  )}
                </legend>
                <label>
                  {text("搜索模型", "Search models", "البحث عن النماذج")}
                  <input
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                  />
                </label>
                <div className="rp-actions">
                  <Button
                    disabled={visibleModels.length === 0}
                    onClick={() => {
                      reset();
                      setModels([...new Set([...models, ...visibleModels])]);
                    }}
                  >
                    {text(
                      "选择搜索结果",
                      "Select search results",
                      "تحديد نتائج البحث",
                    )}
                  </Button>
                  <Button
                    disabled={models.length === 0}
                    onClick={() => {
                      reset();
                      setModels([]);
                    }}
                  >
                    {text("清空选择", "Clear selection", "مسح الاختيار")}
                  </Button>
                  <span>
                    {text(
                      `已选 ${models.length} 个模型`,
                      `${models.length} models selected`,
                      `تم اختيار ${models.length} نموذج`,
                    )}
                  </span>
                </div>
                <div className="rp-model-options">
                  {visibleModels.map((model) => (
                    <label key={model} className="rp-model-choice">
                      <input
                        type="checkbox"
                        checked={models.includes(model)}
                        onChange={(e) => {
                          reset();
                          setModels((prior) =>
                            e.target.checked
                              ? [...prior, model]
                              : prior.filter((item) => item !== model),
                          );
                        }}
                      />
                      <code dir="ltr">{model}</code>
                    </label>
                  ))}
                  {visibleModels.length === 0 && (
                    <p>
                      {text(
                        "没有匹配的模型。",
                        "No matching models.",
                        "لا توجد نماذج مطابقة.",
                      )}
                    </p>
                  )}
                </div>
              </fieldset>
            )}
            <label>
              {text("目标渠道", "Gateway channel", "قناة البوابة")}
              <select
                aria-label={text("目标渠道", "Gateway channel", "قناة البوابة")}
                value={targetID}
                onChange={(e) => {
                  reset();
                  setTargetID(e.target.value);
                }}
              >
                <option value="">
                  {text("请选择渠道", "Select a channel", "اختر قناة")}
                </option>
                {context.channels.map((ch) => (
                  <option key={ch.id} value={ch.id} disabled={ch.status !== 1}>
                    {ch.name} · {ch.id}
                    {ch.status !== 1
                      ? ` · ${text("已停用", "Disabled", "معطلة")}`
                      : ""}
                  </option>
                ))}
              </select>
            </label>
            <p>
              {text(
                "目标渠道默认使用来源渠道，也可以改选其他渠道。",
                "The gateway defaults to the source channel. You can select a different channel.",
                "تستخدم البوابة قناة المصدر افتراضيًا. يمكنك اختيار قناة أخرى.",
              )}
            </p>
            <p>
              {text(
                "分组沿用来源渠道现有的启用组合。发布会替换目标渠道的路由授权，请核对预览中的模型和分组。",
                "Groups use the source channel's existing enabled pairs. Publishing replaces the gateway channel's routing grants; review the models and groups below.",
                "تُستخدم مجموعات المصدر المفعلة. يستبدل النشر صلاحيات توجيه قناة البوابة؛ راجع النماذج والمجموعات في المعاينة.",
              )}
            </p>
            {models.some((model) => !availableModels.includes(model)) && (
              <p role="alert">
                {text(
                  "已选模型发生变化，请重新选择模型。",
                  "Selected models changed. Select models again.",
                  "تغيرت النماذج المحددة. أعد اختيار النماذج.",
                )}
              </p>
            )}
          </>
        )}
        <details>
          <summary>
            {text(
              "其他来源：资源快照",
              "Other source: resource snapshot",
              "مصدر آخر: لقطة الموارد",
            )}
          </summary>
          <Button
            onClick={() => {
              afterClose.current = onUseSnapshot;
              close();
            }}
          >
            {text(
              "使用资源快照流程",
              "Use the resource snapshot workflow",
              "استخدام مسار لقطة الموارد",
            )}
          </Button>
        </details>
        <div ref={result} tabIndex={-1}>
          {error && <p role="alert">{error}</p>}
          {stale && (
            <p role="alert">
              {text(
                "预览已过期，请刷新并重新生成。",
                "The preview is stale. Refresh and generate it again.",
                "انتهت صلاحية المعاينة. حدّث وأنشئها مجددًا.",
              )}
            </p>
          )}
          {preview && !stale && (
            <div className="rp-result">
              <p role="status">
                {preview.ready
                  ? text(
                      "预览校验通过，尚未发布。请点击底部“发布绑定”，确认后才会显示在已发布列表。",
                      "Preview checks passed. Click Publish binding in the footer and confirm to add it to the published list.",
                      "اجتازت المعاينة الفحوص. انقر نشر الربط أسفل النافذة ثم أكّد لإضافته إلى القائمة المنشورة.",
                    )
                  : text(
                      "存在阻塞项，尚未发布。",
                      "Blocking issues remain. Nothing has been published.",
                      "توجد مشكلات مانعة. لم يُنشر أي شيء.",
                    )}
              </p>
              <PairTable
                title={text("拟发布组合", "Proposed pairs", "الأزواج المقترحة")}
                rows={preview.proposed_routes}
                showConstraints
              />
              <PairTable
                title={text(
                  "当前能力组合",
                  "Current ability pairs",
                  "أزواج القدرات الحالية",
                )}
                rows={preview.existing_pairs}
                existing
              />
              {preview.issues.length > 0 && (
                <IssueList
                  key={preview.review_digest}
                  issues={preview.issues}
                  label={issueLabel}
                />
              )}
            </div>
          )}
        </div>
        {context && context.publish_available !== true && (
          <p>
            {text(
              "当前服务器仅支持预览，尚未开放发布。",
              "This server allows previews only; publication is unavailable.",
              "يتيح هذا الخادم المعاينة فقط؛ النشر غير متاح.",
            )}
          </p>
        )}
      </div>
    </Modal>
  );
}
