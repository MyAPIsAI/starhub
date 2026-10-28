import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import {
  previewResourcePublication,
  usePublicationContext,
} from "../lib/api/resource-publication";
import type { ResourceCatalog } from "../lib/resource-catalog";
import type {
  PublicationPair,
  PublicationPreview,
} from "../lib/resource-publication";
import { ApiError } from "../lib/api/http";
import {
  isAdmin,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import { PublicationCommit } from "./resource-publication-commit";
import { ResourceBindingList } from "./resource-binding-state";
import "./resource-publication.css";

export function ResourcePublication(props: {
  catalog: ResourceCatalog | undefined;
  refreshCatalog: () => Promise<unknown>;
}) {
  const identity = useSessionQueryIdentity();
  return <PublicationPanel key={JSON.stringify(identity)} {...props} />;
}

function PublicationPanel({
  catalog,
  refreshCatalog,
}: {
  catalog: ResourceCatalog | undefined;
  refreshCatalog: () => Promise<unknown>;
}) {
  const { text } = useSettingsText();
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
  const issueLabel = (code: string) => {
    switch (code) {
      case "unknown_source_channel":
        return text(
          "来源渠道已不存在",
          "Source channel no longer exists",
          "قناة المصدر لم تعد موجودة",
        );
      case "unknown_gateway_channel":
        return text(
          "真实渠道已不存在",
          "Gateway channel no longer exists",
          "قناة البوابة لم تعد موجودة",
        );
      case "unbound_source_channel":
        return text(
          "请显式选择真实渠道",
          "Select a gateway channel explicitly",
          "اختر قناة البوابة صراحةً",
        );
      case "unavailable_resource_or_channel":
        return text(
          "来源、资源或真实渠道不可用",
          "Source, resource or gateway channel is unavailable",
          "المصدر أو المورد أو قناة البوابة غير متاحة",
        );
      case "model_alias_not_supported":
        return text(
          "暂不支持模型别名，三个模型标识须一致",
          "Aliases are unsupported; all three model IDs must match",
          "الأسماء البديلة غير مدعومة؛ يجب تطابق معرّفات النماذج الثلاثة",
        );
      case "gateway_model_id_not_supported":
        return text(
          "模型标识超出网关支持范围",
          "Model ID exceeds gateway limits",
          "معرّف النموذج يتجاوز حدود البوابة",
        );
      case "protocol_or_capability_not_supported":
        return text(
          "首期仅支持 OpenAI 类型文本/代码资源",
          "This preview supports OpenAI-type text/code resources only",
          "تدعم هذه المعاينة موارد النص والبرمجة من نوع OpenAI فقط",
        );
      case "group_unavailable_or_unpriced":
        return text(
          "分组不可用、已删除或未配置倍率",
          "Group is unavailable, deleted or has no configured ratio",
          "المجموعة غير متاحة أو محذوفة أو بلا معامل محدد",
        );
      case "no_supported_pairs":
        return text(
          "没有可支持的模型与分组组合",
          "No supported model and group pairs",
          "لا توجد أزواج مدعومة من النماذج والمجموعات",
        );
      case "incompatible_resource_constraints":
        return text(
          "同一组合的来源能力互不兼容，请统一清单或拆分真实渠道。",
          "Sources for this pair declare incompatible capabilities. Align inventory or use separate gateway channels.",
          "قدرات مصادر هذا الزوج غير متوافقة. وحّد المخزون أو استخدم قنوات بوابة منفصلة.",
        );
      case "gateway_bound_to_other_source":
        return text(
          "真实渠道仍绑定其他来源，请先核对并解除旧绑定。",
          "This gateway belongs to another source. Review and release its existing binding first.",
          "ترتبط قناة البوابة بمصدر آخر. راجع الرابط الحالي وأزله أولًا.",
        );
      case "source_bound_to_other_gateway":
        return text(
          "来源仍绑定另一个真实渠道，请先核对并解除旧绑定。",
          "This source is bound to another gateway. Review and release its existing binding first.",
          "يرتبط هذا المصدر بقناة بوابة أخرى. راجع الرابط الحالي وأزله أولًا.",
        );
      case "binding_without_supported_pairs":
        return text(
          "该绑定没有可发布组合，请启用有效投放或取消该选择。",
          "This binding has no supported pairs. Enable valid placements or remove this selection.",
          "لا يحتوي هذا الرابط على أزواج مدعومة. فعّل إعدادات نشر صالحة أو أزل هذا الاختيار.",
        );
      case "publication_revision_exhausted":
        return text(
          "发布版本已达到安全上限，请联系管理员处理。",
          "Publication revisions reached the safe limit. Contact an administrator.",
          "بلغت إصدارات النشر الحد الآمن. تواصل مع المسؤول.",
        );
      default:
        return text(
          "服务器返回了阻塞原因，请核对来源配置",
          "The server reported a blocking issue; check source configuration",
          "أبلغ الخادم عن مشكلة مانعة؛ تحقق من إعدادات المصدر",
        );
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
          "先生成服务器预览；只有明确确认发布后，才会保存绑定并替换所选渠道的路由授权。来源模型、标准模型和投放模型须同名。上游凭证仍在真实渠道中单独管理。",
          "Generate a server preview first. Only explicit publication confirmation saves bindings and replaces routing grants on selected channels. Source, canonical and placement model IDs must match. Manage upstream credentials separately in real channels.",
          "أنشئ معاينة الخادم أولًا. لا تُحفظ الروابط ولا تُستبدل صلاحيات التوجيه للقنوات المحددة إلا بعد تأكيد النشر صراحةً. يجب تطابق معرّفات المصدر والنموذج القياسي والنشر. تُدار بيانات الاعتماد في القنوات الفعلية بشكل منفصل.",
        )}
      </aside>
      <p className="rp-profile">
        {text(
          "首期支持 OpenAI/Anthropic 基础文本对话：",
          "This initial profile supports basic OpenAI/Anthropic text chat:",
          "يدعم النطاق الأولي المحادثة النصية الأساسية عبر OpenAI/Anthropic:",
        )}{" "}
        <code dir="ltr">/v1/chat/completions</code>
        {" · "}
        <code dir="ltr">/v1/messages</code>
        {". "}
        {text(
          "工具、图片、音视频及其他端点暂不支持；已登记的上下文限制按请求估算核对，未登记保持未知。",
          "Tools, images, audio/video and other endpoints are unsupported. Registered context limits are checked against request estimates; missing limits remain unknown.",
          "الأدوات والصور والصوت والفيديو ونقاط النهاية الأخرى غير مدعومة. تُراجع حدود السياق المسجلة مقابل تقديرات الطلب؛ تبقى الحدود غير المسجلة غير معروفة.",
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
        onCommitted={() => {
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
      <ResourceBindingList enabled={enabled} />
    </section>
  );
}

function PairTable({
  title,
  rows,
  existing = false,
  showConstraints = false,
}: {
  title: string;
  rows: (PublicationPair & {
    enabled?: boolean;
    capabilities?: string[];
    context_length?: number;
  })[];
  existing?: boolean;
  showConstraints?: boolean;
}) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(0);
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const current = Math.min(page, pages - 1);
  return (
    <section className="rp-pairs" data-proposed={showConstraints || undefined}>
      <h3>
        {title} · {rows.length}
      </h3>
      <div className="rp-table" role="region" tabIndex={0} aria-label={title}>
        <table>
          <thead>
            <tr>
              <th>{text("真实渠道标识", "Gateway ID", "معرّف البوابة")}</th>
              <th>{text("模型", "Model", "النموذج")}</th>
              <th>{text("分组", "Group", "المجموعة")}</th>
              {showConstraints && (
                <>
                  <th>
                    {text(
                      "共同能力",
                      "Shared capabilities",
                      "القدرات المشتركة",
                    )}
                  </th>
                  <th>{text("上下文限制", "Context limit", "حد السياق")}</th>
                </>
              )}
              {existing && (
                <th>{text("能力状态", "Ability state", "حالة القدرة")}</th>
              )}
            </tr>
          </thead>
          <tbody>
            {rows.slice(current * 20, (current + 1) * 20).map((row) => (
              <tr
                key={JSON.stringify([
                  row.gateway_channel_id,
                  row.model,
                  row.group,
                ])}
              >
                <td>
                  <code>{row.gateway_channel_id}</code>
                </td>
                <td>
                  <code>{row.model}</code>
                </td>
                <td>
                  <code>{row.group}</code>
                </td>
                {showConstraints && (
                  <>
                    <td>
                      {row.capabilities
                        ?.map((capability) =>
                          capability === "code"
                            ? text("代码", "Code", "برمجة")
                            : text("文本", "Text", "نص"),
                        )
                        .join(" · ")}
                    </td>
                    <td>
                      {row.context_length ? (
                        <code>{row.context_length.toLocaleString()}</code>
                      ) : (
                        text("未知", "Unknown", "غير معروف")
                      )}
                    </td>
                  </>
                )}
                {existing && (
                  <td>
                    {row.enabled
                      ? text("已启用", "Enabled", "مفعلة")
                      : text("已停用", "Disabled", "معطلة")}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {rows.length === 0 && (
        <p>{text("没有组合", "No pairs", "لا توجد أزواج")}</p>
      )}
      {pages > 1 && (
        <div className="rp-actions">
          <Button disabled={current === 0} onClick={() => setPage(current - 1)}>
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
        </div>
      )}
    </section>
  );
}

function IssueList({
  issues,
  label,
}: {
  issues: PublicationPreview["issues"];
  label: (code: string) => string;
}) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(0);
  const pages = Math.ceil(issues.length / 20);
  return (
    <section
      aria-label={text("发布阻塞原因", "Publication blockers", "عوائق النشر")}
    >
      <h3>
        {text("发布阻塞原因", "Publication blockers", "عوائق النشر")} ·{" "}
        {issues.length}
      </h3>
      <ul className="rp-issues">
        {issues.slice(page * 20, (page + 1) * 20).map((issue, i) => (
          <li key={i}>
            <strong>{label(issue.code)}</strong>
            {(issue.resource_id || issue.source_channel_id || issue.group) && (
              <code>
                {[issue.resource_id, issue.source_channel_id, issue.group]
                  .filter(Boolean)
                  .join(" · ")}
              </code>
            )}
          </li>
        ))}
      </ul>
      {pages > 1 && (
        <div className="rp-actions">
          <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
            {text("上一页", "Previous", "السابق")}
          </Button>
          <span>
            {page + 1} / {pages}
          </span>
          <Button
            disabled={page + 1 >= pages}
            onClick={() => setPage(page + 1)}
          >
            {text("下一页", "Next", "التالي")}
          </Button>
        </div>
      )}
    </section>
  );
}
