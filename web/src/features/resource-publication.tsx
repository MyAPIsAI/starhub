import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import {
  previewResourcePublication,
  usePublicationContext,
} from "../lib/api/resource-publication";
import type { ResourceCatalog } from "../lib/resource-catalog";
import type { PublicationPreview } from "../lib/resource-publication";
import { ApiError } from "../lib/api/http";
import {
  isAdmin,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import { PublicationCommit } from "./resource-publication-commit";
import { ResourceBindingList } from "./resource-binding-state";
import { PlatformChannelList } from "./platform-channel-list";
import {
  PairTable,
  IssueList,
  usePublicationIssueLabel,
} from "./resource-publication-review";
import { CurrentChannelBindingDialog } from "./current-channel-binding-dialog";
import "./resource-publication.css";

type PublicationProps = {
  openImport: () => void;
  openPlacement: (id: string) => void;
  catalog: ResourceCatalog | undefined;
  refreshCatalog: () => Promise<unknown>;
};
export function ResourcePublication(props: PublicationProps) {
  const identity = useSessionQueryIdentity();
  return <PublicationPanel key={JSON.stringify(identity)} {...props} />;
}

function PublicationPanel({
  catalog,
  refreshCatalog,
  openImport,
  openPlacement,
}: PublicationProps) {
  const { text } = useSettingsText();
  const issueLabel = usePublicationIssueLabel();
  const role = useSystemAccess((s) => s.role),
    account = useSystemAccess((s) => s.session.accountId);
  const enabled = account !== null && isAdmin(role);
  const query = usePublicationContext(enabled);
  const [selected, setSelected] = useState(new Map<string, string>());
  const [preview, setPreview] = useState<PublicationPreview>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const active = useRef<AbortController>();
  const [creating, setCreating] = useState(false);
  const [sourceId, setSourceId] = useState("");
  const [currentCreating, setCurrentCreating] = useState(false);
  const [focusTarget, setFocusTarget] = useState("");
  const mappingTable = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (focusTarget) {
      mappingTable.current
        ?.querySelector<HTMLSelectElement>(
          `[data-publication-source="${CSS.escape(focusTarget)}"] select`,
        )
        ?.focus();
      setFocusTarget("");
    }
  }, [focusTarget]);
  useEffect(() => {
    if (!creating) return;
    document.getElementById("binding-creation-guide")?.focus();
  }, [creating]);
  const [published, setPublished] = useState<{ id: string }>();
  useEffect(() => () => active.current?.abort(), []);
  if (!enabled) return null;
  const context = query.error ? undefined : query.data;
  const coherent = Boolean(
    catalog && context && catalog.revision === context.resource_revision,
  );
  const stale = Boolean(
    preview &&
    (!coherent ||
      !context ||
      preview.resource_revision !== context.resource_revision ||
      preview.channel_revision !== context.channel_revision ||
      preview.option_version !== context.option_version),
  );
  const sources = (catalog?.snapshot?.channels ?? []).filter((ch) =>
    [ch.id, ch.name].some((value) =>
      value.toLowerCase().includes(search.trim().toLowerCase()),
    ),
  );
  const pages = Math.max(1, Math.ceil(sources.length / 20)),
    current = Math.min(page, pages - 1);
  const sourceInstances =
    catalog?.snapshot?.instances.filter((i) => i.channelId === sourceId) ?? [];
  const guide = !creating ? null : !catalog || !context || !coherent ? (
    <p role="status">
      {text(
        "发布上下文不可用，请刷新发布上下文。",
        "Refresh the publication context before continuing.",
        "حدّث سياق النشر للمتابعة.",
      )}{" "}
      <Button
        onClick={() =>
          void Promise.allSettled([query.refetch(), refreshCatalog()])
        }
      >
        {text(
          "刷新发布上下文",
          "Refresh publication context",
          "تحديث سياق النشر",
        )}
      </Button>
    </p>
  ) : !catalog.snapshot?.channels.length ? (
    <div>
      <p>
        {text(
          "快照来源尚未导入。可以导入资源快照，或点击“新建持久绑定”使用当前渠道模型。",
          "You can bind models from current channels directly. Import a resource snapshot only when you need an external inventory.",
          "لم تُستورد لقطة بعد. استورد لقطة أو أنشئ ربطًا دائمًا باستخدام نماذج القنوات الحالية.",
        )}
      </p>
      <Button onClick={openImport}>
        {text(
          "导入资源快照",
          "Import optional resource snapshot",
          "استيراد لقطة الموارد",
        )}
      </Button>
    </div>
  ) : (
    <div>
      {context.publish_available !== true && (
        <p>
          {text(
            "当前服务器未开放发布，仅可预览。",
            "This server allows previews only.",
            "يتيح هذا الخادم المعاينة فقط.",
          )}
        </p>
      )}
      <label>
        {text("选择要绑定的来源", "Source to bind", "مصدر الربط")}{" "}
        <select value={sourceId} onChange={(e) => setSourceId(e.target.value)}>
          <option value="">—</option>
          {catalog.snapshot.channels.map((ch) => (
            <option key={ch.id} value={ch.id}>
              {ch.name} · {ch.id}
            </option>
          ))}
        </select>
      </label>
      {sourceId && (
        <>
          {!sourceInstances.length && (
            <p>
              {text(
                "来源没有资源，请更新快照。",
                "This source has no resources. Update the snapshot.",
                "لا موارد لهذا المصدر. حدّث اللقطة.",
              )}{" "}
              <Button onClick={openImport}>
                {text(
                  "导入资源快照",
                  "Import resource snapshot",
                  "استيراد لقطة الموارد",
                )}
              </Button>
            </p>
          )}
          {sourceInstances.map((i) => {
            const p = catalog.placements[i.id];
            return (
              <p key={i.id}>
                <bdi>
                  {i.upstreamModel} · {i.id}
                </bdi>{" "}
                —{" "}
                {p?.enabled && p.modelId && p.groupIds.length
                  ? `${p.modelId} / ${p.groupIds.join(", ")}`
                  : text(
                      "尚未启用有效的模型和分组投放",
                      "An enabled model and group placement is missing",
                      "إعداد نشر فعّال للنموذج والمجموعة مفقود",
                    )}{" "}
                <Button onClick={() => openPlacement(i.id)}>
                  {text("配置投放", "Configure placement", "إعداد النشر")}
                </Button>
              </p>
            );
          })}
          {sourceInstances.some((i) => {
            const p = catalog.placements[i.id];
            return p?.enabled && p.modelId && p.groupIds.length;
          }) && (
            <Button
              onClick={() => {
                setSearch("");
                setPage(
                  Math.floor(
                    catalog.snapshot!.channels.findIndex(
                      (ch) => ch.id === sourceId,
                    ) / 20,
                  ),
                );
                setFocusTarget(sourceId);
              }}
            >
              {text(
                "选择真实渠道并预览",
                "Select gateway and preview",
                "اختيار البوابة ومعاينتها",
              )}
            </Button>
          )}
        </>
      )}
    </div>
  );
  const reset = () => {
    active.current?.abort();
    active.current = undefined;
    setBusy(false);
    setPreview(undefined);
    setError("");
  };
  const run = async () => {
    if (!coherent || !context || !catalog || selected.size === 0) return;
    reset();
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    try {
      const sourceIDs = new Set(
        catalog.snapshot?.channels.map((ch) => ch.id) ?? [],
      );
      const bindings = [...selected]
        .filter(([source]) => sourceIDs.has(source))
        .map(([source_channel_id, gateway_channel_id]) => ({
          source_channel_id,
          gateway_channel_id,
        }));
      const result = await previewResourcePublication(
        context,
        bindings,
        controller.signal,
      );
      if (active.current === controller) setPreview(result);
    } catch (cause) {
      if (active.current === controller && !controller.signal.aborted)
        setError(
          cause instanceof ApiError && cause.status === 409
            ? text(
                "资源、渠道或配置已变化，请刷新后重新生成预览。",
                "Resources, channels or configuration changed. Refresh and generate a new preview.",
                "تغيرت الموارد أو القنوات أو الإعدادات. حدّث وأنشئ معاينة جديدة.",
              )
            : text(
                "无法生成服务器预览。请刷新核对绑定，选择会保留且不会自动重试。",
                "Server preview is unavailable. Refresh and check the retained selections; requests are not retried automatically.",
                "معاينة الخادم غير متاحة. حدّث وتحقق من الاختيارات المحفوظة؛ لا تُعاد المحاولة تلقائيًا.",
              ),
        );
    } finally {
      if (active.current === controller) {
        setBusy(false);
        active.current = undefined;
      }
    }
  };
  return (
    <section
      id="gateway-publication"
      className="resource-publication"
      aria-label={text(
        "网关发布预览",
        "Gateway publication preview",
        "معاينة نشر البوابة",
      )}
    >
      <header>
        <div>
          <h2>
            {text(
              "网关发布预览",
              "Gateway publication preview",
              "معاينة نشر البوابة",
            )}
          </h2>
          <p>
            {text(
              "将清单来源显式对应到真实渠道，再核对准确的模型与分组组合。",
              "Map inventory sources to real channels, then review exact model and group pairs.",
              "اربط مصادر المخزون بالقنوات الفعلية، ثم راجع أزواج النماذج والمجموعات بدقة.",
            )}
          </p>
        </div>
        <Button
          disabled={busy || query.isFetching}
          onClick={() => {
            reset();
            void Promise.allSettled([query.refetch(), refreshCatalog()]);
          }}
        >
          {text(
            "刷新发布上下文",
            "Refresh publication context",
            "تحديث سياق النشر",
          )}
        </Button>
      </header>
      <aside>
        {text(
          "先生成服务器预览；只有明确确认发布后，才会保存绑定并替换所选渠道的路由授权。公开模型与上游模型分别确认，按实际协议和能力逐条授权。上游凭证仍在真实渠道中单独管理。",
          "Generate a server preview first. Only explicit publication confirmation saves bindings and replaces routing grants on selected channels. Review public and upstream model IDs separately; each route grants only its declared protocol and capabilities. Manage upstream credentials separately in real channels.",
          "أنشئ معاينة الخادم أولًا. لا تُحفظ الروابط ولا تُستبدل صلاحيات التوجيه للقنوات المحددة إلا بعد تأكيد النشر صراحةً. راجع معرّفات النموذج العام والأصلي والبروتوكول والقدرات لكل مسار. تُدار بيانات الاعتماد في القنوات الفعلية بشكل منفصل.",
        )}
      </aside>
      <p className="rp-profile">
        {text(
          "按资源声明选择协议与端点，文本对话包括：",
          "Resource declarations control protocols and endpoints. Text chat includes:",
          "تحدد الموارد البروتوكولات ونقاط النهاية. تشمل المحادثة النصية:",
        )}{" "}
        <code dir="ltr">/v1/chat/completions</code>
        {" · "}
        <code dir="ltr">/v1/messages</code>
        {". "}
        {text(
          "工具、视觉、Responses、原生 Gemini 与图像/音视频等端点须逐项声明能力，并匹配渠道协议。上下文按请求估算核对，未登记保持未知；声明不代表真实供应商验收。",
          "Tools, vision, Responses, native Gemini and image/audio/video endpoints require explicit capabilities and a matching provider protocol. Context checks use estimates; missing limits remain unknown. Declarations do not establish provider acceptance.",
          "تتطلب الأدوات والوسائط وResponses وGemini قدرات معلنة وبروتوكولًا مطابقًا. السياق تقديري؛ ولا تثبت التصريحات قبول المزوّد.",
        )}
      </p>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取真实渠道…",
            "Loading real channels…",
            "جارٍ تحميل القنوات الفعلية…",
          )}
        </p>
      )}
      {(!catalog || query.error || (!query.isLoading && !context)) && (
        <p role="alert">
          {text(
            "服务器发布上下文不可用，已隐藏旧预览。请刷新。",
            "Server publication context is unavailable. Previous previews are hidden. Refresh to retry.",
            "سياق نشر الخادم غير متاح. أُخفيت المعاينات السابقة. حدّث للمحاولة.",
          )}
        </p>
      )}
      {context && catalog && (
        <>
          <p className="rp-versions">
            {text("资源", "Resources", "الموارد")} v{context.resource_revision}{" "}
            · {text("渠道", "Channels", "القنوات")} v{context.channel_revision}{" "}
            · {text("配置", "Configuration", "الإعدادات")} v
            {context.option_version}
          </p>
          {!coherent && (
            <p role="alert">
              {text(
                "资源目录版本已变化，请一起刷新目录和发布上下文。",
                "Inventory revisions differ. Refresh both inventory and publication context.",
                "تختلف إصدارات المخزون. حدّث المخزون وسياق النشر معًا.",
              )}
            </p>
          )}
        </>
      )}
      <PlatformChannelList />
      {context && catalog && Boolean(catalog.snapshot?.channels.length) && (
        <>
          <label className="rp-search">
            {text(
              "搜索来源渠道",
              "Search source channels",
              "البحث في قنوات المصدر",
            )}
            <input
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(0);
              }}
            />
          </label>
          <div
            ref={mappingTable}
            className="rp-table"
            tabIndex={0}
            role="region"
            aria-label={text(
              "来源与真实渠道绑定",
              "Source to gateway bindings",
              "روابط المصدر والبوابة",
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>{text("来源渠道", "Source channel", "قناة المصدر")}</th>
                  <th>
                    {text(
                      "真实渠道（显式选择）",
                      "Gateway channel (explicit selection)",
                      "قناة البوابة (اختيار صريح)",
                    )}
                  </th>
                </tr>
              </thead>
              <tbody>
                {sources
                  .slice(current * 20, (current + 1) * 20)
                  .map((source) => (
                    <tr key={source.id} data-publication-source={source.id}>
                      <td>
                        <strong>{source.name}</strong>
                        <code>{source.id}</code>
                      </td>
                      <td>
                        <select
                          aria-label={text(
                            `真实渠道：${source.id}`,
                            `Gateway channel: ${source.id}`,
                            `قناة البوابة: ${source.id}`,
                          )}
                          value={selected.get(source.id) ?? ""}
                          onChange={(e) => {
                            reset();
                            const next = new Map(selected);
                            if (e.target.value)
                              next.set(source.id, e.target.value);
                            else next.delete(source.id);
                            setSelected(next);
                          }}
                        >
                          <option value="">
                            {text(
                              "请选择，不自动绑定",
                              "Select; never bound automatically",
                              "اختر؛ لا يوجد ربط تلقائي",
                            )}
                          </option>
                          {context.channels.map((ch) => (
                            <option
                              key={ch.id}
                              value={ch.id}
                              disabled={[...selected].some(
                                ([s, id]) => s !== source.id && id === ch.id,
                              )}
                            >
                              {ch.name ||
                                text(
                                  "未命名渠道",
                                  "Unnamed channel",
                                  "قناة بلا اسم",
                                )}{" "}
                              · {ch.id}
                              {ch.status !== 1
                                ? ` · ${text("已停用", "Disabled", "معطلة")}`
                                : ""}
                            </option>
                          ))}
                        </select>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          {sources.length === 0 && (
            <p>
              {text(
                "没有匹配的来源渠道。",
                "No matching source channels.",
                "لا توجد قنوات مصدر مطابقة.",
              )}
            </p>
          )}
          <div className="rp-actions">
            <Button
              disabled={current === 0}
              onClick={() => setPage(current - 1)}
            >
              {text("上一页", "Previous", "السابق")}
            </Button>
            <span>
              {current + 1} / {pages}
            </span>
            <Button
              disabled={current + 1 >= pages}
              onClick={() => setPage(current + 1)}
            >
              {text("下一页", "Next", "التالي")}
            </Button>
            <Button
              disabled={
                !coherent || busy || query.isFetching || selected.size === 0
              }
              onClick={() => void run()}
            >
              {busy
                ? text(
                    "生成预览中…",
                    "Generating preview…",
                    "جارٍ إنشاء المعاينة…",
                  )
                : text(
                    "生成服务器预览",
                    "Generate server preview",
                    "إنشاء معاينة الخادم",
                  )}
            </Button>
          </div>
        </>
      )}
      {error && <p role="alert">{error}</p>}
      {preview && context && catalog && !stale && (
        <div className="rp-result">
          <p role="status" className="rp-result-status">
            {preview.ready
              ? text(
                  "预览校验通过，尚未发布。",
                  "Preview checks passed. Nothing has been published.",
                  "اجتازت المعاينة الفحوص. لم يُنشر أي شيء.",
                )
              : text(
                  "存在阻塞项，尚未发布。",
                  "Blocking issues remain. Nothing has been published.",
                  "توجد مشكلات مانعة. لم يُنشر أي شيء.",
                )}
          </p>
          <PairTable
            key={`proposed-${preview.review_digest}`}
            title={text("拟发布组合", "Proposed pairs", "الأزواج المقترحة")}
            rows={preview.proposed_routes}
            showConstraints
          />
          <PairTable
            key={`existing-${preview.review_digest}`}
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
              key={JSON.stringify(preview.issues)}
              issues={preview.issues}
              label={issueLabel}
            />
          )}
        </div>
      )}
      {stale && (
        <p role="alert">
          {text(
            "预览依赖已变化，旧结果已隐藏。请重新生成。",
            "Preview dependencies changed. Previous results are hidden. Generate a new preview.",
            "تغيرت متطلبات المعاينة. أُخفيت النتائج السابقة. أنشئ معاينة جديدة.",
          )}
        </p>
      )}
      {context && context.publish_available !== true && (
        <p>
          {text(
            "此服务器仅提供预览，未开放发布。",
            "This server provides previews only; publication is unavailable.",
            "يوفر هذا الخادم المعاينة فقط؛ النشر غير متاح.",
          )}
        </p>
      )}
      <PublicationCommit
        review={preview && !stale ? preview : undefined}
        disabled={
          !coherent ||
          stale ||
          busy ||
          query.isFetching ||
          context?.publish_available !== true
        }
        onCommitted={(review) => {
          const first = review.bindings[0];
          if (first) setPublished({ id: first.gateway_channel_id });
          setCreating(false);
          reset();
          void Promise.allSettled([query.refetch(), refreshCatalog()]);
        }}
        renderRoutes={(review) => (
          <PairTable
            key={`confirmed-${review.review_digest}`}
            title={text(
              "确认发布组合",
              "Confirmed publication pairs",
              "أزواج النشر المؤكدة",
            )}
            rows={review.proposed_routes}
            showConstraints
          />
        )}
      />
      <CurrentChannelBindingDialog
        open={currentCreating}
        onOpenChange={setCurrentCreating}
        onUseSnapshot={() => setCreating(true)}
        onCommitted={(review) => {
          setPublished({ id: review.bindings[0].gateway_channel_id });
          reset();
          void Promise.allSettled([query.refetch(), refreshCatalog()]);
        }}
      />
      <ResourceBindingList
        enabled={enabled}
        channels={context?.channels}
        guide={guide}
        onCreate={() => {
          setCreating(false);
          reset();
          setCurrentCreating(true);
        }}
        published={published}
      />
    </section>
  );
}
