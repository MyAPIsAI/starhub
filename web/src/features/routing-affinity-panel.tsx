import { Button } from "../components/ui/button";
import {
  useRoutingAffinityStats,
  useClearRoutingAffinityCache,
} from "../lib/api/routing-affinity";
import { useSettingsText } from "./settings-text";
import "./resource-publication.css";
import { AffinityEditorActions } from "./routing-affinity-editor";

/** The binding section hosts global HTTP configuration and replica-local cache controls. */
export function RoutingAffinityPanel({ enabled }: { enabled: boolean }) {
  const { text } = useSettingsText();
  const query = useRoutingAffinityStats(enabled);
  const clear = useClearRoutingAffinityCache();
  if (!enabled) return null;
  const stats = query.error ? undefined : query.data;
  const ruleEntries = stats
    ? Object.entries(stats.by_rule_name).filter(
        ([name, count]) => name !== "" || count > 0,
      )
    : [];
  return (
    <>
      <div className="rp-actions">
        {stats && (
          <p className="rp-versions">
            {text(
              "全局亲和配置",
              "Global affinity configuration",
              "إعدادات التقارب العامة",
            )}{" "}
            ·{" "}
            {stats.config
              ? stats.config.enabled
                ? text("已启用", "Enabled", "مفعّل")
                : text("未启用", "Disabled", "غير مفعّل")
              : "—"}
            {" · "}
            {text("规则数", "Rules", "قواعد")}: {stats.rules} ·{" "}
            {text(
              "本副本粘滞条目",
              "Pinned entries on this replica",
              "روابط هذه النسخة",
            )}
            : {stats.total}
          </p>
        )}
        <AffinityEditorActions />
        <Button
          disabled={
            query.isFetching || clear.isPending || (stats?.total ?? 0) === 0
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
      </div>
      {stats?.config && (
        <p className="rp-versions">
          {text("模式", "Mode", "الوضع")}:{" "}
          {stats.config.mode === "hash"
            ? text("哈希分配", "Hash", "التجزئة")
            : text("会话粘滞", "Sticky", "تثبيت الجلسة")}{" "}
          ·{" "}
          {text(
            "默认有效期（秒）",
            "Default TTL (seconds)",
            "مدة الصلاحية الافتراضية (ثوانٍ)",
          )}
          : {stats.config.ttl_seconds} ·{" "}
          {text(
            "失败冷却（秒）",
            "Failure cooldown (seconds)",
            "مهلة الفشل (ثوانٍ)",
          )}
          : {stats.config.cooldown_seconds} ·{" "}
          {text(
            "提示词前缀亲和",
            "Prompt-prefix affinity",
            "تقارب بادئة الطلب",
          )}
          :{" "}
          {stats.config.prefix_cache
            ? text("开", "On", "مفعّل")
            : text("关", "Off", "غير مفعّل")}
        </p>
      )}
      <p>
        {text(
          "此处统计当前服务副本的全部 HTTP 请求亲和规则缓存。清空将移除本副本所有会话的渠道偏好；已发布的路由授权和其他副本不受影响。",
          "These are all HTTP rule-affinity entries on this replica. Clearing them removes channel preferences for all sessions here. Published routing grants and other replicas are unaffected.",
          "تشمل الإحصاءات جميع روابط قواعد طلبات HTTP في هذه النسخة. يزيل المسح تفضيلات القنوات لجميع الجلسات هنا دون تغيير صلاحيات التوجيه المنشورة أو النسخ الأخرى.",
        )}
      </p>
      {query.isLoading && (
        <p role="status">
          {text("正在读取缓存…", "Loading cache…", "جارٍ تحميل الذاكرة…")}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            "缓存读取失败，请刷新重试。",
            "Cache read failed. Refresh to retry.",
            "تعذرت قراءة الذاكرة. حدّث للمحاولة.",
          )}
        </p>
      )}
      {stats && (
        <>
          {ruleEntries.length > 0 && (
            <div className="rp-table" role="region" tabIndex={0}>
              <table>
                <thead>
                  <tr>
                    <th>{text("规则", "Rule", "قاعدة")}</th>
                    <th>{text("粘滞条目", "Entries", "إدخالات")}</th>
                  </tr>
                </thead>
                <tbody>
                  {ruleEntries.map(([name, count]) => (
                    <tr key={name || "_unattributed"}>
                      <td>
                        {name || text("未归属", "Unattributed", "غير منسوبة")}
                      </td>
                      <td>
                        <code>{count}</code>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {stats.total === 0 && (
            <p>
              {text(
                "当前无粘滞条目。",
                "No pinned entries.",
                "لا توجد إدخالات مثبتة.",
              )}
            </p>
          )}
        </>
      )}
      {clear.data && (
        <p role="status" className="rp-result-status">
          {text("已清除", "Cleared", "تم المسح")} {clear.data.removed}{" "}
          {text("条粘滞", "entries", "إدخال")}
        </p>
      )}
      {clear.isError && (
        <p role="alert">
          {text(
            "未能确认清空结果，请核对最新缓存统计后再试。",
            "The clear was not confirmed. Check the latest cache counts before retrying.",
            "لم يتأكد المسح. تحقّق من أحدث إحصاءات الذاكرة قبل إعادة المحاولة.",
          )}
        </p>
      )}
    </>
  );
}
