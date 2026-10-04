import type { DirectoryRate } from "../config/model-directory";
import type { Language } from "../config/placeholders";
import { multiplyDecimalPrice } from "../config/models";
import { formatMoney } from "../lib/money";
import "./directory-price.css";

export function describeDirectoryRate(rate: DirectoryRate, lang: Language) {
  const t = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const rule = rate.rule;
  const mode =
    rule.mode === "tokens"
      ? t("按词元计价", "Token pricing", "التسعير بالرموز")
      : rule.mode === "expression"
        ? t("表达式计价", "Expression pricing", "التسعير بالتعبير")
        : {
            request: t("按次计价", "Per request", "لكل طلب"),
            image: t("按张计价", "Per image", "لكل صورة"),
            second: t("按秒计价", "Per second", "لكل ثانية"),
          }[rule.unit];
  const usage = rate.usage;
  const basicAssumption =
    rule.mode === "expression"
      ? t(
          `示例用量：输入 ${usage.input_tokens}、输出 ${usage.output_tokens}、缓存读写均为 0 词元，${usage.images} 张图片、${usage.seconds} 秒、1 次请求。`,
          `Example usage: ${usage.input_tokens} input, ${usage.output_tokens} output, 0 cache tokens, ${usage.images} images, ${usage.seconds} seconds, 1 request.`,
          `مثال الاستخدام: ${usage.input_tokens} إدخال، ${usage.output_tokens} إخراج، صفر تخزين، ${usage.images} صور، ${usage.seconds} ثانية، طلب واحد.`,
        )
      : rule.mode === "tokens"
        ? t(
            "示例：1,000 输入 + 300 输出词元，无缓存。",
            "Example: 1,000 input + 300 output tokens, no cache.",
            "مثال: 1000 إدخال + 300 إخراج بلا تخزين.",
          )
        : rule.unit === "request"
          ? t("示例：1 次请求。", "Example: 1 request.", "مثال: طلب واحد.")
          : rule.unit === "image"
            ? t("示例：1 张图片。", "Example: 1 image.", "مثال: صورة واحدة.")
            : t(
                "示例：1 秒视频。",
                "Example: 1 second of video.",
                "مثال: ثانية واحدة من الفيديو.",
              );
  const assumption =
    basicAssumption +
    (rate.attributes
      ? t(
          ` 示例规格：${JSON.stringify(rate.attributes)}；其他规格可能不同。`,
          ` Example specification: ${JSON.stringify(rate.attributes)}; other specifications may differ.`,
          ` مواصفات المثال: ${JSON.stringify(rate.attributes)}؛ قد تختلف المواصفات الأخرى.`,
        )
      : "");
  if (!rate.quote)
    return {
      mode,
      value: t("暂不可估算", "Estimate unavailable", "التقدير غير متاح"),
      unit:
        rate.error ||
        t("计价配置不可用", "Pricing unavailable", "التسعير غير متاح"),
      assumption,
      estimate: "—",
      schedule: "",
      details: "",
    };
  const quote = rate.quote;
  const schedule = quote.snapshot.matchedWindow
    ? t(
        `当前命中「${quote.snapshot.matchedWindow.label}」时段（${quote.snapshot.matchedWindow.timezone}），时段系数 ${quote.snapshot.timeMultiplier}× 已计入。`,
        `Current window: ${quote.snapshot.matchedWindow.label} (${quote.snapshot.matchedWindow.timezone}); ${quote.snapshot.timeMultiplier}× included.`,
        `الفترة الحالية: ${quote.snapshot.matchedWindow.label} (${quote.snapshot.matchedWindow.timezone})؛ المعامل ${quote.snapshot.timeMultiplier} محسوب.`,
      )
    : rule.timeWindows?.length
      ? t(
          "当前未命中特殊时段，按标准时段计价。",
          "Current time uses the standard rate.",
          "السعر القياسي للوقت الحالي.",
        )
      : "";
  const unitPrice =
    rule.mode === "unit"
      ? multiplyDecimalPrice(
          multiplyDecimalPrice(
            quote.snapshot.matchedCondition?.priceUsd ?? rule.priceUsd,
            quote.snapshot.groupMultiplier,
          ),
          quote.snapshot.timeMultiplier,
        )
      : "";
  return {
    mode,
    value:
      rule.mode === "tokens"
        ? `$${quote.rate.input} / $${quote.rate.output}`
        : rule.mode === "unit"
          ? `$${unitPrice}`
          : formatMoney(quote.amountNano, "call"),
    unit:
      rule.mode === "tokens"
        ? t(
            "输入 / 输出 · 美元 / 百万词元",
            "Input / output · USD / million tokens",
            "إدخال / إخراج · دولار / مليون رمز",
          )
        : rule.mode === "expression"
          ? t(
              "上述示例费用，非固定单价",
              "Example cost, not a fixed unit price",
              "تكلفة المثال وليست سعر وحدة ثابتًا",
            )
          : {
              request: t("美元 / 次", "USD / request", "دولار / طلب"),
              image: t("美元 / 张", "USD / image", "دولار / صورة"),
              second: t("美元 / 秒", "USD / second", "دولار / ثانية"),
            }[rule.unit],
    assumption,
    estimate: formatMoney(quote.amountNano, "call"),
    schedule,
    details:
      rule.mode === "unit"
        ? t(
            `最低计费量 ${rule.minimum}；${Number(rule.ceilTo) > 0 ? `按 ${rule.ceilTo} 向上取整` : "不取整"}。本例计费量 ${quote.snapshot.billedQuantity}。`,
            `Minimum quantity ${rule.minimum}; rounding step ${rule.ceilTo}. Example billed quantity ${quote.snapshot.billedQuantity}.`,
            `الحد الأدنى ${rule.minimum}؛ خطوة التقريب ${rule.ceilTo}. كمية المثال ${quote.snapshot.billedQuantity}.`,
          )
        : rule.mode === "expression"
          ? rule.expression
          : "",
  };
}

export function DirectoryPrice({
  rate,
  lang,
  compact = false,
}: {
  rate: DirectoryRate;
  lang: Language;
  compact?: boolean;
}) {
  const info = describeDirectoryRate(rate, lang);
  return (
    <div className={`directory-price${rate.error ? " has-error" : ""}`}>
      <small>{info.mode}</small>
      <strong dir={rate.quote ? "ltr" : undefined}>
        <bdi>{info.value}</bdi>
      </strong>
      <small>{info.unit}</small>
      {!compact && (
        <>
          <small>
            {info.assumption}{" "}
            {lang === "zh"
              ? "示例费用："
              : lang === "ar"
                ? "تكلفة المثال: "
                : "Example cost: "}
            {info.estimate}
          </small>
          {info.details && (
            <small
              className={
                rate.rule.mode === "expression"
                  ? "directory-price-expression"
                  : ""
              }
            >
              {info.details}
            </small>
          )}
          {info.schedule && <small>{info.schedule}</small>}
        </>
      )}
      {compact && rate.rule.mode === "expression" && (
        <small>{info.assumption}</small>
      )}
    </div>
  );
}
