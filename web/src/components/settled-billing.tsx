import { useTranslation } from "react-i18next";
import type { StoredBillingDetail } from "../lib/request-metadata";
import "./settled-billing.css";

export function SettledBilling({
  detail,
  timezone = "Asia/Dubai",
}: {
  detail: StoredBillingDetail;
  timezone?: string;
}) {
  const { i18n } = useTranslation();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language.startsWith("zh")
      ? zh
      : i18n.language.startsWith("ar")
        ? ar
        : en;
  const labels: Record<string, string> = {
    billable_units: c("计费量", "Billable quantity", "الكمية المفوترة"),
    input_tokens: c("普通输入 Token", "Uncached input", "رموز الإدخال"),
    output_tokens: c("输出 Token", "Output tokens", "رموز الإخراج"),
    cache_read_tokens: c(
      "缓存读取 Token",
      "Cache read tokens",
      "رموز قراءة التخزين",
    ),
    cache_write_tokens: c(
      "缓存写入 Token",
      "Cache write tokens",
      "رموز كتابة التخزين",
    ),
    input_audio_tokens: c(
      "音频输入 Token",
      "Audio input tokens",
      "رموز إدخال الصوت",
    ),
    output_audio_tokens: c(
      "音频输出 Token",
      "Audio output tokens",
      "رموز إخراج الصوت",
    ),
    requests: c("请求", "Requests", "الطلبات"),
    images: c("图片", "Images", "الصور"),
    seconds: c("秒", "Seconds", "الثواني"),
  };
  const units: Record<string, string> = {
    request: c("次", "request", "طلب"),
    image: c("张", "image", "صورة"),
    second: c("秒", "second", "ثانية"),
    unit: c("单位", "unit", "وحدة"),
  };
  return (
    <div className="settled-billing">
      <dt>
        {c(
          "调用时计费明细",
          "Billing at request time",
          "تفاصيل الفوترة وقت الطلب",
        )}
      </dt>
      <dd>
        <div className="usage-table-scroll">
          <table>
            <thead>
              <tr>
                <th>{c("计费单位", "Meter", "الوحدة")}</th>
                <th>{c("数量", "Quantity", "الكمية")}</th>
                <th>
                  {c(
                    "调用时单价（美元）",
                    "Captured price (USD)",
                    "السعر المسجل (دولار)",
                  )}
                </th>
              </tr>
            </thead>
            <tbody>
              {detail.parts.map((part, index) => (
                <tr key={index}>
                  <td>{labels[part.name] ?? part.name}</td>
                  <td className="mono">{part.quantity}</td>
                  <td className="mono">
                    {part.unit_price_usd ? `$${part.unit_price_usd}` : "—"}
                    {part.unit === "million_tokens"
                      ? c(" / 百万 Token", " / 1M tokens", " / مليون رمز")
                      : part.unit_price_usd
                        ? ` / ${units[part.unit] ?? part.unit}`
                        : ""}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <small className="request-rate-note">
          {c(
            "单价已包含本次系数；金额按完整用量舍入结算。",
            "Rates include captured multipliers; the complete usage is rounded at settlement.",
            "تشمل الأسعار المعاملات المسجلة؛ يتم تقريب إجمالي الاستخدام عند التسوية.",
          )}
        </small>
        <small>
          {new Date(detail.priced_at).toLocaleString(i18n.language, {
            timeZone: timezone,
            timeZoneName: "short",
          })}{" "}
          · {c("系数", "Multiplier", "المعامل")} {detail.group_multiplier}
        </small>
        {detail.formula && (
          <code style={{ display: "block", overflowWrap: "anywhere" }}>
            {detail.formula}
          </code>
        )}
      </dd>
    </div>
  );
}
