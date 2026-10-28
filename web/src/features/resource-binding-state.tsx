import { useState } from "react";
import { Button } from "../components/ui/button";
import { useResourceBindingState } from "../lib/api/resource-publication";
import { useSettingsText } from "./settings-text";

export function ResourceBindingList({ enabled }: { enabled: boolean }) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(1);
  const query = useResourceBindingState(enabled, page);
  const state = query.error ? undefined : query.data;
  return (
    <section
      className="rp-result"
      id="gateway-bindings"
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
                      {row.profile !== "openai_text_exact_v1" && (
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
    </section>
  );
}
