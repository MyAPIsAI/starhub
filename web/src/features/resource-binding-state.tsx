import { useEffect, useRef, useState, type ReactNode } from "react";
import type { PublicationChannel } from "../lib/resource-publication";
import { useLocation } from "react-router-dom";
import { Button } from "../components/ui/button";
import { useResourceBindingState } from "../lib/api/resource-publication";
import { useSettingsText } from "./settings-text";
import {
  prepareBindingChange,
  type BindingChangeReceipt,
  type BindingChangeReview,
} from "../lib/resource-binding-change";
import type { ResourceBindingStatus } from "../lib/resource-binding-state";
import { ResourceBindingChangeDialog } from "./resource-binding-change";
import { RoutingAffinityPanel } from "./routing-affinity-panel";
import {
  AffinityEditor,
  AffinityEditorActions,
  type AffinityEditorTarget,
} from "./routing-affinity-editor";
import {
  useRoutingAffinityStats,
  useClearRoutingAffinityCache,
} from "../lib/api/routing-affinity";

export function ResourceBindingList({
  enabled,
  channels,
  guide,
  onCreate,
  published,
}: {
  enabled: boolean;
  channels?: PublicationChannel[];
  guide: ReactNode;
  onCreate: () => void;
  published?: { id: string };
}) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(1);
  const query = useResourceBindingState(enabled, page);
  const affinity = useRoutingAffinityStats(enabled);
  const clear = useClearRoutingAffinityCache();
  const [affinitySaved, setAffinitySaved] = useState(false);
  const [editor, setEditor] = useState<AffinityEditorTarget>();
  const openEditor = (target: AffinityEditorTarget) => {
    setAffinitySaved(false);
    setEditor(target);
  };
  const state = query.error ? undefined : query.data;
  const { hash } = useLocation();
  const section = useRef<HTMLElement>(null);
  const [refreshedPublication, setRefreshedPublication] =
    useState<typeof published>();
  const searchPages = useRef<{ id: string; lo: number; hi: number } | null>(
    null,
  );
  useEffect(() => {
    let active = true;
    searchPages.current = published
      ? { id: published.id, lo: 1, hi: Infinity }
      : null;
    if (published)
      void query.refetch().then(() => {
        if (active) setRefreshedPublication(published);
      });
    return () => {
      active = false;
    };
  }, [published, query.refetch]);
  useEffect(() => {
    const target = searchPages.current;
    if (
      !target ||
      refreshedPublication !== published ||
      !state ||
      query.isFetching ||
      state.page !== page
    )
      return;
    const row = section.current?.querySelector<HTMLElement>(
      `[data-binding-gateway="${CSS.escape(target.id)}"]`,
    );
    if (row) {
      row.focus();
      searchPages.current = null;
      return;
    }
    target.hi = Math.min(target.hi, Math.ceil(state.total / state.page_size));
    const last = state.bindings.at(-1);
    if (last && BigInt(last.gateway_channel_id) < BigInt(target.id))
      target.lo = Math.max(target.lo, page + 1);
    else target.hi = Math.min(target.hi, page - 1);
    if (target.lo <= target.hi)
      setPage(Math.floor((target.lo + target.hi) / 2));
    else searchPages.current = null;
  }, [published, refreshedPublication, state, query.isFetching, page]);
  useEffect(() => {
    if (hash !== "#gateway-bindings" || !enabled) return;
    const frame = requestAnimationFrame(() => {
      section.current?.scrollIntoView({ block: "start" });
      section.current?.focus({ preventScroll: true });
    });
    return () => cancelAnimationFrame(frame);
  }, [hash, enabled, query.isLoading]);
  const [review, setReview] = useState<BindingChangeReview>();
  const [receipt, setReceipt] = useState<BindingChangeReceipt>();
  const [actionError, setActionError] = useState("");
  const resultElement = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (receipt) resultElement.current?.focus();
  }, [receipt]);
  const changesEnabled = Boolean(
    state?.binding_changes_available === true &&
    !query.isFetching &&
    state.resource_revision < Number.MAX_SAFE_INTEGER &&
    state.channel_revision < Number.MAX_SAFE_INTEGER,
  );
  const choose = (
    row: ResourceBindingStatus,
    operation: BindingChangeReview["operation"],
  ) => {
    if (!state || !changesEnabled) return;
    try {
      const key = Array.from(
        crypto.getRandomValues(new Uint8Array(16)),
        (byte) => byte.toString(16).padStart(2, "0"),
      ).join("");
      setReview(prepareBindingChange(state, row, operation, key));
      setReceipt(undefined);
      setActionError("");
    } catch {
      setActionError(
        text(
          "无法准备操作，请刷新后重试。",
          "The action could not be prepared. Refresh and try again.",
          "تعذر إعداد الإجراء. حدّث وحاول مجددًا.",
        ),
      );
    }
  };
  return (
    <section
      className="rp-result"
      id="gateway-bindings"
      ref={section}
      tabIndex={-1}
      aria-label={text(
        "持久网关绑定",
        "Persistent gateway bindings",
        "روابط البوابة الدائمة",
      )}
    >
      <header>
        <h3>
          {text("HTTP 会话亲和", "HTTP session affinity", "تقارب جلسات HTTP")}
        </h3>
        <div className="rp-binding-toolbar">
          {enabled && (
            <>
              <AffinityEditorActions onOpen={openEditor} />
              <Button
                disabled={
                  affinity.isFetching ||
                  affinity.isError ||
                  clear.isPending ||
                  !affinity.data?.total
                }
                onClick={() => clear.mutate()}
              >
                {clear.isPending
                  ? text("清空中…", "Clearing…", "جارٍ المسح…")
                  : text(
                      "清空本副本规则缓存",
                      "Clear all rule bindings on this replica",
                      "مسح جميع روابط القواعد في هذه النسخة",
                    )}
              </Button>
            </>
          )}
          <Button
            disabled={query.isFetching || affinity.isFetching}
            onClick={() =>
              void Promise.allSettled([
                query.refetch(),
                ...(enabled ? [affinity.refetch()] : []),
              ])
            }
          >
            {text(
              "刷新绑定状态",
              "Refresh binding state",
              "تحديث حالة الروابط",
            )}
          </Button>
        </div>
      </header>
      {enabled && affinitySaved && (
        <p role="status">
          {text(
            "亲和配置已保存。",
            "Affinity configuration saved.",
            "حُفظت إعدادات التقارب.",
          )}
        </p>
      )}
      {enabled && (
        <RoutingAffinityPanel
          channels={channels}
          query={affinity}
          clear={clear}
          onEditRule={(ruleName) => openEditor({ kind: "rules", ruleName })}
        />
      )}
      {enabled && editor && (
        <AffinityEditor
          target={editor}
          close={() => setEditor(undefined)}
          saved={() => {
            setEditor(undefined);
            setAffinitySaved(true);
          }}
        />
      )}
      <header>
        <h3>
          {text(
            "已发布的资源路由绑定",
            "Published resource route bindings",
            "روابط مسارات الموارد المنشورة",
          )}
        </h3>
        <Button aria-haspopup="dialog" onClick={onCreate}>
          {text("新建持久绑定", "New persistent binding", "إنشاء ربط دائم")}
        </Button>
      </header>
      {guide && (
        <div id="binding-creation-guide" tabIndex={-1}>
          {guide}
        </div>
      )}
      {published && (
        <p className="rp-result-status" role="status">
          {text(
            `服务器已确认发布渠道 ${published.id} 的绑定。下表显示重新读取的绑定状态；若读取失败，请点击“刷新绑定状态”。`,
            `The server confirmed publication for channel ${published.id}. The table reloads the binding state; if loading fails, click Refresh binding state.`,
            `أكد الخادم نشر ربط القناة ${published.id}. يعيد الجدول تحميل حالة الروابط؛ إذا فشل التحميل، انقر تحديث حالة الروابط.`,
          )}
        </p>
      )}
      <p>
        {text(
          "已存组合是当前保留的托管路线数；路由条件通过数不代表用户授权、余额或上游健康检查通过。撤销会停止托管路由并保留来源；释放会按渠道当前模型和分组配置恢复普通调用，并从此表移除。旧版已释放记录可点击“恢复普通调用”。",
          "Stored pairs count managed routes; route eligibility does not prove user authorization, balance or upstream health. Revoke stops managed routing and retains ownership. Release restores ordinary routing from the current channel configuration and removes the entry. Previously released entries can use Restore ordinary routing.",
          "تحصي الأزواج المخزنة المسارات المُدارة ولا تثبت صلاحية المستخدم أو الرصيد أو سلامة المزود. يوقف الإلغاء التوجيه المُدار مع حفظ المصدر. يعيد التحرير التوجيه العادي حسب إعدادات القناة الحالية ويحذف السجل. يمكن استعادة التوجيه العادي للسجلات المحررة سابقًا.",
        )}
      </p>
      {actionError && <p role="alert">{actionError}</p>}
      {receipt && (
        <div
          className="rp-result-status"
          role="status"
          ref={resultElement}
          tabIndex={-1}
        >
          <strong>
            {text(
              "已核对绑定操作记录",
              "Binding action record verified",
              "تم التحقق من سجل إجراء الرابط",
            )}
          </strong>
          <p>
            {receipt.operation === "release"
              ? text(
                  "已解除托管，恢复按渠道配置进行普通路由",
                  "Managed ownership removed; ordinary routing restored from channel configuration",
                  "أزيل الربط المُدار واستُعيد التوجيه العادي حسب إعدادات القناة",
                )
              : text(
                  "已提交撤销路由",
                  "Route revocation committed",
                  "اعتُمد إلغاء المسارات",
                )}{" "}
            · <code dir="ltr">{receipt.change_id}</code>
          </p>
          <p>
            <code dir="ltr">
              {receipt.source_channel_id} → {receipt.gateway_channel_id}
            </code>{" "}
            · {text("资源", "Resources", "الموارد")} v
            {receipt.resource_revision} · {text("渠道", "Channels", "القنوات")}{" "}
            v{receipt.channel_revision}
          </p>
          <p>
            {text(
              "这是历史提交事实；当前状态请以服务器最新读取结果为准。普通调用仍受渠道启用状态、用户和密钥授权等条件约束。",
              "This proves a historical commit. Check current server state. Ordinary calls still require an enabled channel and valid user and token authorization.",
              "يثبت هذا اعتمادًا تاريخيًا. تحقق من حالة الخادم الحالية. تتطلب الاستدعاءات العادية قناة مفعلة وصلاحيات مستخدم ومفتاح سارية.",
            )}
          </p>
        </div>
      )}
      {query.isLoading && (
        <p role="status">
          {text("正在读取绑定…", "Loading bindings…", "جارٍ تحميل الروابط…")}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            "绑定状态读取失败，旧结果已隐藏。请刷新核对。",
            "Binding state could not be read. Old results are hidden. Refresh to verify.",
            "تعذرت قراءة حالة الروابط. أُخفيت النتائج القديمة. حدّث للتحقق.",
          )}
        </p>
      )}
      {state && (
        <>
          {state.binding_changes_available !== true && (
            <p>
              {text(
                "此服务器未开放绑定变更，仅可查看状态。",
                "This server has not enabled binding changes. State is read-only.",
                "لم يُفعّل هذا الخادم تغييرات الروابط. الحالة للقراءة فقط.",
              )}
            </p>
          )}
          {state.binding_changes_available === true &&
            state.release_restores_ordinary_routing !== true && (
              <p role="status">
                {text(
                  "当前服务器尚不支持释放后恢复普通调用，请更新后端后再操作。",
                  "This server does not support restoring ordinary routing on release. Update the backend first.",
                  "لا يدعم هذا الخادم استعادة التوجيه العادي عند التحرير. حدّث الخادم أولًا.",
                )}
              </p>
            )}
          <p className="rp-versions">
            {text("资源", "Resources", "الموارد")} v{state.resource_revision} ·{" "}
            {text("渠道", "Channels", "القنوات")} v{state.channel_revision} ·{" "}
            {text("配置", "Configuration", "الإعدادات")} v{state.option_version}
          </p>
          <div
            className="rp-table"
            tabIndex={0}
            role="region"
            aria-label={text(
              "绑定状态表",
              "Binding state table",
              "جدول حالة الروابط",
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>
                    {text(
                      "来源 / 真实渠道",
                      "Source / gateway",
                      "المصدر / البوابة",
                    )}
                  </th>
                  <th>
                    {text("配置状态", "Configured state", "حالة الإعداد")}
                  </th>
                  <th>
                    {text(
                      "已存 / 路由条件通过",
                      "Stored / route eligible",
                      "مخزنة / مؤهلة للتوجيه",
                    )}
                  </th>
                  <th>
                    {text(
                      "最近发布记录",
                      "Last publication record",
                      "آخر سجل نشر",
                    )}
                  </th>
                  <th>{text("操作", "Actions", "الإجراءات")}</th>
                </tr>
              </thead>
              <tbody>
                {state.bindings.map((row) => (
                  <tr
                    key={row.gateway_channel_id}
                    data-binding-gateway={row.gateway_channel_id}
                    tabIndex={-1}
                  >
                    <td>
                      <strong>
                        {row.gateway_name ||
                          text(
                            "渠道不存在或未命名",
                            "Missing or unnamed gateway",
                            "بوابة مفقودة أو بلا اسم",
                          )}
                      </strong>
                      <code dir="ltr">
                        {row.source_channel_id || "—"} →{" "}
                        {row.gateway_channel_id}
                      </code>
                      {!row.gateway_present && (
                        <span>
                          {text(
                            "真实渠道已删除",
                            "Gateway deleted",
                            "حُذفت البوابة",
                          )}
                        </span>
                      )}
                      {row.source_channel_id && !row.source_present && (
                        <span>
                          {text(
                            "来源已不在清单",
                            "Source absent from inventory",
                            "المصدر غير موجود في المخزون",
                          )}
                        </span>
                      )}
                    </td>
                    <td>
                      {!row.source_channel_id
                        ? text(
                            "旧版已释放，普通调用尚未恢复",
                            "Previously released; ordinary routing not restored",
                            "محررة سابقًا؛ لم يُستعد التوجيه العادي",
                          )
                        : row.configured_active
                          ? text(
                              "配置标记启用",
                              "Configuration marked active",
                              "الإعداد محدد كنشط",
                            )
                          : text(
                              "已撤销，来源仍保留",
                              "Revoked; source retained",
                              "ملغاة؛ المصدر محفوظ",
                            )}
                      {row.profile !== "openai_text_exact_v1" &&
                        row.profile !== "exact_resource_policy_v2" && (
                          <p>
                            {text(
                              "策略未识别，未开放路由",
                              "Unrecognized profile; routing unavailable",
                              "نطاق غير معروف؛ التوجيه غير متاح",
                            )}
                          </p>
                        )}
                    </td>
                    <td>
                      <code dir="ltr">
                        {row.stored_pair_count} /{" "}
                        {row.route_eligible_pair_count}
                      </code>
                    </td>
                    <td>
                      <code dir="ltr">
                        {row.publication_id === "0" ? "—" : row.publication_id}
                      </code>
                    </td>
                    <td>
                      {row.source_channel_id ? (
                        <div className="rp-binding-actions">
                          <Button
                            variant="danger"
                            aria-label={text(
                              `撤销路线：${row.gateway_channel_id}`,
                              `Revoke routes: ${row.gateway_channel_id}`,
                              `إلغاء المسارات: ${row.gateway_channel_id}`,
                            )}
                            disabled={
                              !changesEnabled ||
                              (!row.configured_active &&
                                row.stored_pair_count === 0)
                            }
                            onClick={() => choose(row, "revoke")}
                          >
                            {text(
                              "撤销路线",
                              "Revoke routes",
                              "إلغاء المسارات",
                            )}
                          </Button>
                          <Button
                            aria-label={text(
                              `释放绑定：${row.gateway_channel_id}`,
                              `Release binding: ${row.gateway_channel_id}`,
                              `تحرير الرابط: ${row.gateway_channel_id}`,
                            )}
                            disabled={
                              !changesEnabled ||
                              state.release_restores_ordinary_routing !== true
                            }
                            onClick={() => choose(row, "release")}
                          >
                            {text(
                              "释放绑定",
                              "Release binding",
                              "تحرير الرابط",
                            )}
                          </Button>
                        </div>
                      ) : (
                        <Button
                          disabled={
                            !changesEnabled ||
                            state.release_restores_ordinary_routing !== true
                          }
                          onClick={() => choose(row, "release")}
                        >
                          {text(
                            "恢复普通调用",
                            "Restore ordinary routing",
                            "استعادة التوجيه العادي",
                          )}
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {state.total === 0 && (
            <p>
              {text(
                "尚未发布资源路由；不影响上方 HTTP 会话亲和。",
                "No resource routes published yet. HTTP session affinity is independent.",
                "لم تُنشر مسارات موارد بعد. تقارب جلسات HTTP مستقل.",
              )}
            </p>
          )}
          <div className="rp-actions">
            <Button
              disabled={page === 1 || query.isFetching}
              onClick={() => setPage(page - 1)}
            >
              {text("上一页", "Previous", "السابق")}
            </Button>
            <span>
              {page} / {Math.max(1, Math.ceil(state.total / state.page_size))} ·{" "}
              {state.total}
            </span>
            <Button
              disabled={
                page * state.page_size >= state.total || query.isFetching
              }
              onClick={() => setPage(page + 1)}
            >
              {text("下一页", "Next", "التالي")}
            </Button>
          </div>
        </>
      )}
      <ResourceBindingChangeDialog
        key={review?.request_id ?? "none"}
        review={review}
        onClose={() => setReview(undefined)}
        onCommitted={(value) => {
          setReceipt(value);
          setReview(undefined);
          if (value.operation === "release") setPage(1);
        }}
      />
    </section>
  );
}
