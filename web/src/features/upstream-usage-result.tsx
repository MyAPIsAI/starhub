import type { UpstreamUsageResult } from "../lib/api/admin";
import { formatMoney, quotaMoneyNano } from "../lib/money";
import { useSettingsText } from "./settings-text";
import "./upstream-usage-result.css";

export function UpstreamUsageResultView({
  result,
}: {
  result: UpstreamUsageResult;
}) {
  const { text, lang } = useSettingsText();
  const labels = {
    local: text("本地累计", "Local used", "الاستخدام المحلي"),
    upstream: text("上游累计", "Upstream used", "استخدام المزود"),
    diff: text("差异", "Difference", "الفرق"),
    key: text("密钥", "Key", "المفتاح"),
    used: text("已用", "Used", "المستخدم"),
    available: text("剩余", "Available", "المتاح"),
    expires: text("过期时间", "Expires", "انتهاء الصلاحية"),
  };
  const money = (quota: number, exact?: string, detail = false) =>
    formatMoney(quotaMoneyNano(exact, quota), detail ? "detail" : "total");
  const amounts = [
    {
      label: labels.local,
      quota: result.local_used_quota,
      exact: result.local_used_quota_nano,
    },
    ...(result.supported
      ? [
          {
            label: labels.upstream,
            quota: result.upstream_total_used,
            exact: result.upstream_total_used_nano,
          },
          { label: labels.diff, quota: result.diff, exact: result.diff_nano },
        ]
      : []),
  ];
  return (
    <div className="upstream-usage" dir={lang === "ar" ? "rtl" : "ltr"}>
      <div className="usage-summary">
        {amounts.map((amount) => (
          <div key={amount.label}>
            <span>{amount.label} (USD)</span>
            <strong className="mono" dir="ltr">
              {money(amount.quota, amount.exact)}
            </strong>
          </div>
        ))}
      </div>
      <details className="usage-exact">
        <summary>
          {text("查看精确金额", "View exact amounts", "عرض المبالغ الدقيقة")}
        </summary>
        <dl>
          {amounts.map((amount) => (
            <div key={amount.label}>
              <dt>{amount.label} (USD)</dt>
              <dd className="mono" dir="ltr">
                {money(amount.quota, amount.exact, true)}
              </dd>
            </div>
          ))}
        </dl>
      </details>
      {!result.supported ? (
        <p>
          {result.code === "ark_usage_not_supported"
            ? text(
                "尚未接入方舟官方用量查询，请到火山引擎控制台查看账单。此处仅展示本地消费；模型测试与用量查询是独立能力。",
                "Ark usage queries are not integrated. View bills in the Volcengine console. Only local usage is shown here; model tests and usage queries are separate capabilities.",
                "استعلام استخدام Ark غير مدمج. راجع الفواتير في لوحة Volcengine. يعرض هنا الاستخدام المحلي فقط؛ اختبار النماذج واستعلام الاستخدام قدرتان منفصلتان.",
              )
            : text(
                "上游不支持用量查询（可能不是 new-api 实例）",
                "Upstream does not support usage queries (may not be a new-api instance)",
                "المنبع لا يدعم استعلامات الاستخدام",
              )}
        </p>
      ) : (
        <>
          <table className="usage-keys">
            <caption>
              {text(
                "各密钥用量（USD）",
                "Per-key usage (USD)",
                "الاستخدام لكل مفتاح (USD)",
              )}
            </caption>
            <thead>
              <tr>
                {[
                  labels.key,
                  labels.used,
                  labels.available,
                  labels.expires,
                ].map((label) => (
                  <th key={label} scope="col">
                    {label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.keys.map((key) => (
                <tr key={key.key_index}>
                  <td data-label={labels.key}>
                    <span className="mono" dir="ltr">
                      #{key.key_index + 1} {key.key_masked}
                    </span>
                    {!key.available && (
                      <p>
                        {key.error ||
                          text("查询失败", "Query failed", "فشل الاستعلام")}
                      </p>
                    )}
                  </td>
                  <td data-label={labels.used} className="mono">
                    {key.available
                      ? money(key.total_used, key.total_used_nano)
                      : "—"}
                  </td>
                  <td data-label={labels.available} className="mono">
                    {key.available
                      ? key.unlimited
                        ? text("无限", "Unlimited", "غير محدود")
                        : money(key.total_available, key.total_available_nano)
                      : "—"}
                  </td>
                  <td data-label={labels.expires}>
                    {key.available && key.expires_at > 0
                      ? new Date(key.expires_at * 1000).toLocaleDateString(lang)
                      : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="usage-note">
            {text(
              "上游金额按兼容 new-api 的相同额度单位换算为 USD。密钥累计可能包含其他来源及历史调用，且计价倍率、退款同步可能不同；差异仅供参考，不等同采购账单或计费错误。",
              "Upstream amounts assume the same new-api quota unit and are converted to USD. Key totals may include other callers and earlier usage; pricing and refund timing can differ. Differences are for reference and do not establish procurement costs or billing errors.",
              "تفترض مبالغ المزود وحدة حصة new-api نفسها وتحول إلى USD. قد تشمل إجماليات المفاتيح استدعاءات خارجية أو سابقة، وقد تختلف الأسعار وتوقيت الاسترداد. الفرق مرجعي ولا يثبت تكلفة الشراء أو خطأ في الفوترة.",
            )}
          </p>
        </>
      )}
    </div>
  );
}
