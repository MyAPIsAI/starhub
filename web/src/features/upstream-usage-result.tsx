import type { UpstreamUsageResult } from "../lib/api/admin";
import { formatMoney, quotaMoneyNano } from "../lib/money";
import { useSettingsText } from "./settings-text";
import "./upstream-usage-result.css";

const messages: Record<string, [string, string, string]> = {
  ark_usage_not_supported: [
    "尚未接入方舟官方用量查询，请到火山引擎控制台查看账单。此处仅展示本地消费；模型测试与用量查询是独立能力。",
    "Ark usage queries are not integrated. View bills in the Volcengine console. Only local usage is shown here; model tests and usage queries are separate capabilities.",
    "استعلام استخدام Ark غير مدمج. راجع الفواتير في لوحة Volcengine. يعرض هنا الاستخدام المحلي فقط؛ اختبار النماذج واستعلام الاستخدام قدرتان منفصلتان.",
  ],
  usage_not_supported: [
    "上游未提供兼容的用量查询接口，请检查地址或在上游控制台查看。",
    "The upstream does not expose a compatible usage endpoint. Check the URL or view usage in its console.",
    "لا يوفر المزود واجهة استخدام متوافقة. تحقق من العنوان أو راجع لوحة المزود.",
  ],
  usage_query_failed: [
    "未获取到有效上游用量，请查看各密钥的失败原因。",
    "No valid upstream usage was retrieved. See the failure reason for each key.",
    "لم يتم جلب استخدام صالح. راجع سبب الفشل لكل مفتاح.",
  ],
  usage_incomplete: [
    "仅部分密钥查询成功；展示已查部分，不计算与本地累计的差异。",
    "Only some keys were queried successfully. Retrieved usage is shown; no difference against the local total is calculated.",
    "نجح الاستعلام لبعض المفاتيح فقط. يعرض الجزء المتاح دون حساب الفرق مع الإجمالي المحلي.",
  ],
  usage_auth_failed: [
    "上游认证或权限不足，请检查密钥及用量查询权限。",
    "Upstream authentication or permission failed. Check the key and usage-query permission.",
    "فشلت المصادقة أو الصلاحية. تحقق من المفتاح وصلاحية استعلام الاستخدام.",
  ],
  usage_timeout: [
    "上游用量查询超时，请稍后重试。",
    "The upstream usage query timed out. Retry later.",
    "انتهت مهلة استعلام الاستخدام. أعد المحاولة لاحقًا.",
  ],
  usage_network_error: [
    "无法连接上游，请检查地址和网络。",
    "Cannot connect to the upstream. Check its URL and network.",
    "تعذر الاتصال بالمزود. تحقق من العنوان والشبكة.",
  ],
  usage_invalid_response: [
    "上游用量响应缺少有效金额或格式错误。",
    "The upstream usage response has missing or invalid amounts or format.",
    "استجابة الاستخدام تفتقر إلى مبالغ صالحة أو تنسيقها غير صالح.",
  ],
  usage_query_rejected: [
    "上游拒绝用量查询，请在上游检查权限和配置。",
    "The upstream rejected the usage query. Check permissions and configuration upstream.",
    "رفض المزود استعلام الاستخدام. تحقق من الصلاحيات والإعدادات لديه.",
  ],
  usage_http_error: [
    "上游用量接口返回异常，请稍后重试。",
    "The upstream usage endpoint returned an error. Retry later.",
    "أعادت واجهة الاستخدام خطأ. أعد المحاولة لاحقًا.",
  ],
  usage_invalid_url: [
    "上游地址无效，请检查渠道配置。",
    "The upstream URL is invalid. Check the channel configuration.",
    "عنوان المزود غير صالح. تحقق من إعداد القناة.",
  ],
  usage_base_url_required: [
    "缺少上游地址，请检查渠道配置。",
    "The upstream URL is missing. Check the channel configuration.",
    "عنوان المزود مفقود. تحقق من إعداد القناة.",
  ],
  usage_key_required: [
    "没有可查询的密钥，请检查渠道配置。",
    "No key is available for the usage query. Check the channel configuration.",
    "لا يوجد مفتاح للاستعلام. تحقق من إعداد القناة.",
  ],
};

export function UpstreamUsageResultView({
  result,
}: {
  result: UpstreamUsageResult;
}) {
  const { text, lang } = useSettingsText();
  const complete =
    result.supported &&
    (result.status
      ? result.status === "ok"
      : result.keys.length > 0 && result.keys.every((key) => key.available));
  const message = (code?: string) =>
    text(...(messages[code || ""] || messages.usage_query_failed));
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
            label: complete
              ? labels.upstream
              : text(
                  "上游已查部分",
                  "Retrieved upstream usage",
                  "الجزء المتاح من استخدام المزود",
                ),
            quota: result.upstream_total_used,
            exact: result.upstream_total_used_nano,
          },
          ...(complete
            ? [
                {
                  label: labels.diff,
                  quota: result.diff,
                  exact: result.diff_nano,
                },
              ]
            : []),
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
      {!complete && (
        <p role="status">
          {message(
            result.code ||
              (result.supported ? "usage_incomplete" : "usage_query_failed"),
          )}
        </p>
      )}
      {result.keys.length > 0 && (
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
                    {!key.available && <p>{message(key.code)}</p>}
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
