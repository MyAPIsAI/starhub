import { usePreferences } from "../lib/preferences";
import { writtenLanguage } from "../lib/localize";
import { formatTimeQuoteInstant, readTimeQuote } from "../lib/time-quote";
import "./time-pricing-note.css";

export function TimePricingNote({
  value,
  compact = false,
}: {
  value: unknown;
  compact?: boolean;
}) {
  const lang = writtenLanguage(usePreferences((state) => state.language));
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const quote = readTimeQuote(value);
  if (quote === undefined) return null;
  if (quote === null)
    return (
      <p role="alert" className="time-pricing-note">
        {text(
          "分时信息暂不可用，请刷新报价。",
          "Time-pricing information is unavailable. Refresh the quote.",
          "معلومات التسعير الزمني غير متاحة. حدّث عرض السعر.",
        )}
      </p>
    );
  const at = formatTimeQuoteInstant(quote.quoted_at, quote.timezone, lang),
    until = formatTimeQuoteInstant(quote.valid_until, quote.timezone, lang);
  return (
    <div
      className={`time-pricing-note${compact ? " is-compact" : ""}`}
      data-time-quote-window={quote.window_id}
      data-token-price-window={quote.token_window?.window_id}
    >
      <p>
        {text("已含分时倍率", "Time multiplier included", "معامل الوقت مشمول")}:{" "}
        <bdi>{quote.multiplier}×</bdi> · <bdi>{quote.timezone}</bdi>
        {quote.window_id && (
          <>
            {" "}
            · <bdi>{quote.window_id}</bdi>
          </>
        )}
      </p>
      {quote.token_window && (
        <p>
          {quote.token_window.window_id
            ? text(
                "已含独立时段词元单价",
                "Independent window token prices included",
                "أسعار رموز الفترة المستقلة مشمولة",
              )
            : text(
                "独立单价时段外，使用基础词元单价",
                "Outside independent price windows; base token prices apply",
                "خارج فترات الأسعار المستقلة؛ تُطبق أسعار الرموز الأساسية",
              )}
          {" · "}
          <bdi>{quote.token_window.timezone}</bdi>
          {quote.token_window.window_id && (
            <>
              {" "}
              · <bdi>{quote.token_window.window_id}</bdi>
            </>
          )}
        </p>
      )}
      {compact ? (
        <p>
          <time dateTime={quote.quoted_at}>{at}</time>
        </p>
      ) : (
        <>
          <p>
            {text("报价时间", "Quoted at", "وقت عرض السعر")}:{" "}
            <time dateTime={quote.quoted_at}>{at}</time>
          </p>
          <p>
            {text(
              "本次报价有效至",
              "This quote is valid until",
              "هذا العرض صالح حتى",
            )}
            : <time dateTime={quote.valid_until}>{until}</time>
          </p>
          <p>
            {text(
              "页面按服务器报价时效自动刷新；未命中时段使用基础倍率。实际费用使用请求准入时捕获的定价，长请求不会按结束时段重算。",
              "The page refreshes using the server quote lifetime. Unmatched windows use the base multiplier. Charges use pricing captured at request admission; long requests are not repriced at completion.",
              "تُحدّث الصفحة وفق مدة عرض الخادم. خارج الفترات يُستخدم المعامل الأساسي. تُحسب الرسوم بالسعر الملتقط عند قبول الطلب دون إعادة تسعير الطلبات الطويلة عند انتهائها.",
            )}
          </p>
        </>
      )}
    </div>
  );
}
