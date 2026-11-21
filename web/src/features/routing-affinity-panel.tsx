import { Button } from "../components/ui/button";
import {
  useRoutingAffinityStats,
  useClearRoutingAffinityCache,
} from "../lib/api/routing-affinity";
import { useSettingsText } from "./settings-text";
import "./resource-publication.css";

/** RoutingAffinityPanel shows the live rule-affinity cache state and a
 *  clear button. It is embedded under the persistent gateway bindings
 *  section so an operator can see how many sessions are pinned and evict
 *  them when a bound channel is rotated. Rules themselves are configured
 *  through the routing_affinity_rules option in the option manager. */
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
    <section
      className="rp-result"
      aria-label={text(
        "网关亲和性缓存",
        "Gateway affinity cache",
        "ذاكرة التقارب",
      )}
    >
      <header>
        <h3>
          {text(
            "网关亲和性缓存",
            "Gateway affinity cache",
            "ذاكرة التقارب",
          )}
        </h3>
        <Button
          disabled={query.isFetching || clear.isPending || (stats?.total ?? 0) === 0}
          onClick={() => clear.mutate()}
        >
          {clear.isPending
            ? text("清空中…", "Clearing…", "جارٍ المسح…")
            : text("清空亲和缓存", "Clear affinity cache", "مسح ذاكرة التقارب")}
        </Button>
      </header>
      <p>
        {text(
          "展示当前副本上被规则粘滞的会话数。缓存为进程内状态，重启即失；多副本不共享。撤销绑定或停用渠道后请手动清空，避免旧粘滞在 TTL 内残留。",
          "Shows sessions pinned by rules on this replica. The cache is process-local and resets on restart; replicas do not share it. After revoking a binding or disabling a channel, clear manually so a stale pin does not survive its TTL.",
          "يعرض الجلسات المثبتة بقواعد على هذه النسخة. الذاكرة محلية للعملية وتُفرّغ عند إعادة التشغيل؛ النسخ لا تتشاركها. بعد إلغاء رابط أو تعطيل قناة، أمسح يدويًا كي لا تبقى التثبيتات القديمة ضمن مدة البقاء.",
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
          <p className="rp-versions">
            {stats.enabled
              ? text("已启用", "Enabled", "مفعّل")
              : text("未启用", "Disabled", "غير مفعّل")}{" "}
            · {text("规则数", "Rules", "قواعد")}: {stats.rules} ·{" "}
            {text("粘滞条目", "Pinned entries", "إدخالات مثبتة")}: {stats.total}
          </p>
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
    </section>
  );
}
