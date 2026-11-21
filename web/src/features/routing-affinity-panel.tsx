import type {
  useRoutingAffinityStats,
  useClearRoutingAffinityCache,
} from "../lib/api/routing-affinity";
import { useSettingsText } from "./settings-text";
import "./resource-publication.css";

const eventLabels: Record<string, [string, string, string]> = {
  conditions_unmatched: [
    "条件未匹配",
    "Conditions not matched",
    "لم تتطابق الشروط",
  ],
  affinity_disabled: [
    "总开关关闭，规则未执行",
    "Rules skipped: affinity disabled",
    "تُخطيت القواعد: التقارب معطل",
  ],
  missing_token: [
    "请求缺少有效 Token，规则未执行",
    "Rules skipped: no valid token",
    "تُخطيت القواعد: لا يوجد رمز صالح",
  ],
  hash_mode_bypass: [
    "哈希模式跳过会话规则",
    "Session rules skipped in hash mode",
    "تُخطيت قواعد الجلسة في وضع التجزئة",
  ],
  missing_source: ["缺少会话标识", "Missing session key", "معرّف الجلسة مفقود"],
  invalid_value: [
    "会话值不合规",
    "Invalid session value",
    "قيمة جلسة غير صالحة",
  ],
  rule_off: ["匹配关闭规则", "Matched an off rule", "تطابق قاعدة متوقفة"],
  cold_start: ["规则冷启动", "Rule cold starts", "بدايات القواعد الباردة"],
  rule_hit: ["规则首选命中", "Rule preference hits", "إصابات تفضيل القاعدة"],
  legacy_hit: [
    "旧 Token 亲和命中",
    "Legacy token preference hits",
    "إصابات تفضيل الرمز القديم",
  ],
  write_ok: [
    "成功写入或续期",
    "Bindings written or renewed",
    "روابط مكتوبة أو مجددة",
  ],
  write_revision: [
    "规则版本变化未写入",
    "Writes skipped: rules changed",
    "كتابات متخطاة لتغيّر القواعد",
  ],
  write_clear: [
    "缓存清空后未写入",
    "Writes skipped: cache cleared",
    "كتابات متخطاة لمسح الذاكرة",
  ],
  write_raced: [
    "条目已变化未写入",
    "Writes skipped: entry changed",
    "كتابات متخطاة لتغيّر الرابط",
  ],
  write_full: [
    "容量已满未写入",
    "Writes skipped: capacity reached",
    "كتابات متخطاة لبلوغ السعة",
  ],
};

/** The binding section hosts global HTTP configuration and replica-local cache controls. */
export function RoutingAffinityPanel({
  query,
  clear,
  onEditRule,
}: {
  query: ReturnType<typeof useRoutingAffinityStats>;
  clear: ReturnType<typeof useClearRoutingAffinityCache>;
  onEditRule: (name: string) => void;
}) {
  const { text } = useSettingsText();
  const stats = query.error ? undefined : query.data;
  const ruleEntries = stats
    ? Object.entries(stats.by_rule_name).filter(
        ([name, count]) => name !== "" || count > 0,
      )
    : [];
  return (
    <>
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
      {stats?.instance && stats.observed_at && (
        <p className="rp-versions">
          {text("统计副本", "Statistics replica", "نسخة الإحصاءات")}:{" "}
          <bdi>{stats.instance}</bdi> ·{" "}
          {text("最近成功读取", "Last successful read", "آخر قراءة ناجحة")}:{" "}
          <time dateTime={new Date(stats.observed_at).toISOString()}>
            {new Date(stats.observed_at).toLocaleTimeString()}
          </time>
        </p>
      )}
      {stats?.events && (
        <details>
          <summary>
            {text(
              "亲和诊断（本副本启动以来）",
              "Affinity diagnostics (since replica start)",
              "تشخيص التقارب منذ بدء النسخة",
            )}
          </summary>
          <p>
            {text(
              "以下为所有规则的累计事件，不是当前绑定数。可将请求响应头 X-StarHub-Affinity-Instance 与统计副本对照。",
              "These cumulative events cover all rules; they are not live binding counts. Compare the response header X-StarHub-Affinity-Instance with the statistics replica.",
              "هذه أحداث تراكمية لجميع القواعد وليست عدد الروابط الحالية. قارن ترويسة الاستجابة X-StarHub-Affinity-Instance بنسخة الإحصاءات.",
            )}
          </p>
          <ul>
            {Object.entries(eventLabels).map(([key, label]) => (
              <li key={key}>
                {text(...label)}: {stats.events?.[key] ?? "—"}
              </li>
            ))}
          </ul>
        </details>
      )}
      {(stats?.events?.missing_source ?? 0) > 0 && (
        <p role="note">
          {text(
            "有请求满足规则的模型/路径条件，但没有提供配置的会话来源。测试台会自动发送稳定的 X-Session-Id；CLI/SDK 请在同一会话的连续请求中重复发送相同的非空值，或将规则来源改为客户端实际提供的稳定字段。不要使用每次请求变化的 request_id/trace_id。",
            "Some requests matched a rule's model/path conditions but omitted its configured session source. The playground sends a stable X-Session-Id automatically. For CLI/SDK clients, repeat the same non-empty value across requests in one session, or change the rule to a stable field the client actually sends. Do not use a per-request request_id/trace_id.",
            "طابقت بعض الطلبات شروط النموذج/المسار لكنها لم ترسل مصدر الجلسة المحدد. ترسل ساحة الاختبار X-Session-Id ثابتًا تلقائيًا. في عملاء CLI/SDK، أعد استخدام القيمة غير الفارغة نفسها عبر طلبات الجلسة الواحدة، أو غيّر المصدر إلى حقل ثابت يرسله العميل فعليًا. لا تستخدم request_id/trace_id متغيرًا لكل طلب.",
          )}
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
                    <tr key={name}>
                      <td>
                        {name ? (
                          <button
                            type="button"
                            className="rp-rule-edit"
                            aria-haspopup="dialog"
                            aria-label={text(
                              `编辑规则：${name}`,
                              `Edit rule: ${name}`,
                              `تحرير القاعدة: ${name}`,
                            )}
                            onClick={() => onEditRule(name)}
                          >
                            {name}
                          </button>
                        ) : (
                          text("未归属", "Unattributed", "غير منسوبة")
                        )}
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
