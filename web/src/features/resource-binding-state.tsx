import { useEffect, useRef, useState } from "react";
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

export function ResourceBindingList({ enabled }: { enabled: boolean }) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(1);
  const query = useResourceBindingState(enabled, page);
  const state = query.error ? undefined : query.data;
  const { hash } = useLocation();
  const section = useRef<HTMLElement>(null);
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
          {text(
            "持久网关绑定",
            "Persistent gateway bindings",
            "روابط البوابة الدائمة",
          )}
        </h3>
        <Button
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          {text("刷新绑定状态", "Refresh binding state", "تحديث حالة الروابط")}
        </Button>
      </header>
      <p>
        {text(
          "已存组合是当前保留的路线数；路由条件通过数还核对渠道启用、能力、分组身份及已配置倍率。它不代表具体用户授权、余额、请求上下文或供应商健康检查通过。撤销后仍保留来源绑定，已释放的渠道仍保留防止旧权限恢复的记录。",
          "Stored pairs count retained routes. Route-eligible pairs also pass channel, ability, group identity and configured ratio checks. They do not prove user authorization, balance, request context or provider health. Revoked bindings retain source ownership; released channels retain a marker that prevents legacy grants from returning.",
          "تحصي الأزواج المخزنة المسارات المحتفظ بها. تجتاز الأزواج المؤهلة أيضًا فحوص القناة والقدرة وهوية المجموعة والمعامل المحدد. لا تثبت صلاحية المستخدم أو الرصيد أو سياق الطلب أو سلامة المزود. تحتفظ الروابط الملغاة بملكية المصدر، وتحتفظ القنوات المحررة بعلامة تمنع عودة الصلاحيات القديمة.",
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
                  "已提交释放绑定",
                  "Binding release committed",
                  "اعتُمد تحرير الرابط",
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
              "这是历史提交事实；当前状态请以服务器最新成功读取结果为准。不会自动重新发布或恢复旧权限。",
              "This proves a historical commit. Verify current state with a successful server read. It does not republish or restore legacy grants automatically.",
              "يثبت هذا اعتمادًا تاريخيًا. تحقق من الحالة الحالية بقراءة ناجحة من الخادم. لا يحدث نشر أو استعادة للصلاحيات القديمة تلقائيًا.",
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
                            "已释放，保留保护记录",
                            "Released; protective marker retained",
                            "محررة؛ العلامة الوقائية محفوظة",
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
                            disabled={!changesEnabled}
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
                        "—"
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
                "尚无持久绑定。请先显式预览并确认发布。",
                "No persistent bindings yet. Review explicit selections and confirm publication first.",
                "لا توجد روابط دائمة بعد. راجع الاختيارات الصريحة وأكد النشر أولًا.",
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
        }}
      />
    </section>
  );
}
