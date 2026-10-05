import type { PlazaPricing } from "../lib/api/plaza";
import { usePreferences } from "../lib/preferences";
import "./catalog-price-summary.css";

export function PlazaPrice({
  pricing,
  detailed = false,
}: {
  pricing: PlazaPricing;
  detailed?: boolean;
}) {
  const lang = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const perRequest = pricing.mode === "per_request";
  const conditional = pricing.mode === "tiered";
  const rows = perRequest
    ? [
        {
          id: "request",
          label: text("每次请求", "Per request", "لكل طلب"),
          platform: pricing.per_request,
          official: pricing.official_per_request,
        },
      ]
    : [
        {
          id: "input",
          label: text("输入", "Input", "إدخال"),
          platform: pricing.input_per_m,
          official: pricing.official_input_per_m,
        },
        {
          id: "output",
          label: text("输出", "Output", "إخراج"),
          platform: pricing.output_per_m,
          official: pricing.official_output_per_m,
        },
        ...(detailed
          ? [
              {
                id: "cache-read",
                label: text("缓存读取", "Cache read", "قراءة التخزين"),
                platform: pricing.cache_read_per_m,
                official: pricing.official_cache_read_per_m,
              },
              {
                id: "cache-write",
                label: text("缓存写入", "Cache write", "كتابة التخزين"),
                platform: pricing.cache_write_per_m,
                official: pricing.official_cache_write_per_m,
              },
            ]
          : []),
      ];
  const amount = (value: number | null) =>
    value == null
      ? "—"
      : `${pricing.currency === "USD" ? "$" : `${pricing.currency} `}${value}`;

  return (
    <div
      className="catalog-model-price catalog-price-summary"
      data-pricing-mode={pricing.mode}
    >
      {conditional && (
        <span className="catalog-price-summary-note">
          {text(
            "平台按用量与条件计价",
            "Platform pricing depends on usage and conditions",
            "تسعير المنصة حسب الاستخدام والشروط",
          )}
        </span>
      )}
      <span className="catalog-price-summary-unit">
        {pricing.currency}{" "}
        {perRequest
          ? text("/ 次", "/ request", "/ طلب")
          : text("/ 百万词元", "/ million tokens", "/ مليون رمز")}
      </span>
      <table className="catalog-price-summary-table">
        <caption className="sr-only">
          {text(
            "模型价格对照",
            "Model price comparison",
            "مقارنة أسعار النموذج",
          )}
        </caption>
        <thead>
          <tr>
            <th scope="col">
              <span className="sr-only">
                {text("用量", "Usage", "الاستخدام")}
              </span>
            </th>
            <th scope="col">{text("官方参考", "Official", "مرجع رسمي")}</th>
            <th scope="col">{text("平台报价", "Platform", "المنصة")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id} data-price-axis={row.id}>
              <th scope="row">{row.label}</th>
              <td data-price-kind="official">
                <bdi dir="ltr">{amount(row.official)}</bdi>
              </td>
              <td data-price-kind="public">
                {conditional ? (
                  text("按条件计算", "Conditional", "حسب الشروط")
                ) : (
                  <bdi dir="ltr">{amount(row.platform)}</bdi>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!pricing.explicit && !conditional && (
        <span className="catalog-price-summary-note">
          {text(
            "未单独配置，显示默认报价",
            "Default quote; no model-specific price configured",
            "سعر افتراضي دون إعداد خاص للنموذج",
          )}
        </span>
      )}
      {detailed && (
        <span className="catalog-price-summary-note">
          {text(
            "— 表示未提供报价，不代表免费。官方参考价仅用于对照。",
            "— means no quote provided, not free. Official prices are for comparison.",
            "— تعني عدم توفر سعر وليس المجانية. الأسعار الرسمية للمقارنة فقط.",
          )}
        </span>
      )}
    </div>
  );
}
