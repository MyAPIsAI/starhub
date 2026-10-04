import type {
  OfficialPriceReference,
  PricingCatalogConfig,
} from "../config/models";

/** Point-in-time references verified against first-party sources, not platform selling rates. */
const defaults: Record<string, OfficialPriceReference> = {
  "gpt-4.1": {
    currency: "USD",
    status: "current",
    rate: {
      mode: "tokens",
      pricing: {
        input_per_million_usd: "2",
        output_per_million_usd: "8",
        cache_read_per_million_usd: "0.5",
      },
    },
    scope: {
      zh: "标准文本词元价格；非批处理，不含工具费。缓存读取单列，无独立缓存写入报价。",
      en: "Standard text tokens, not batch; excludes tool fees. Cache reads are separate; no standalone cache-write quote.",
      ar: "رموز نصية قياسية دون معالجة مجمعة أو رسوم أدوات. قراءة الذاكرة منفصلة ولا يوجد سعر مستقل لكتابتها.",
    },
    sourceUrl: "https://developers.openai.com/api/docs/models/gpt-4.1",
    verifiedAt: "2026-09-28",
  },
  "claude-sonnet-4-20250514": {
    currency: "USD",
    status: "legacy",
    rate: {
      mode: "tokens",
      pricing: {
        input_per_million_usd: "3",
        output_per_million_usd: "15",
        cache_read_per_million_usd: "0.3",
        cache_write_per_million_usd: "3.75",
      },
    },
    scope: {
      zh: "历史标准价，输入不超过 200,000 词元；缓存写入按 5 分钟。厂商直连接口已退役，非当前在售承诺；不含云渠道差异及工具费。",
      en: "Historical standard rate for prompts up to 200,000 tokens; 5-minute cache writes. Retired from the direct provider API, not a current availability promise; cloud-channel differences and tool fees excluded.",
      ar: "سعر قياسي تاريخي لمدخلات حتى 200,000 رمز وكتابة ذاكرة لمدة 5 دقائق. أوقف في واجهة المورد المباشرة ولا يعد بتوافر حالي؛ يستثني فروق القنوات السحابية ورسوم الأدوات.",
    },
    sourceUrl: "https://platform.claude.com/docs/en/about-claude/pricing",
    verifiedAt: "2026-09-28",
  },
  "gemini-2.5-pro": {
    currency: "USD",
    status: "current",
    rate: {
      mode: "tokens",
      pricing: {
        input_per_million_usd: "1.25",
        output_per_million_usd: "10",
        cache_read_per_million_usd: "0.125",
      },
    },
    scope: {
      zh: "标准付费，输入不超过 200,000 词元；输出包含思考词元。非批处理；不含缓存存储及搜索等工具费，长上下文另价。",
      en: "Standard paid tier, prompts up to 200,000 tokens; output includes thinking tokens. Not batch; excludes cache storage and tools such as search. Longer contexts use different rates.",
      ar: "الفئة المدفوعة القياسية لمدخلات حتى 200,000 رمز؛ يشمل الإخراج رموز التفكير. دون معالجة مجمعة أو رسوم تخزين الذاكرة وأدوات البحث. للسياقات الأطول أسعار مختلفة.",
    },
    sourceUrl: "https://ai.google.dev/gemini-api/docs/pricing#gemini-2.5-pro",
    verifiedAt: "2026-09-28",
  },
};

const validPrice = (value: unknown) =>
  typeof value === "string" &&
  /^(0|[1-9]\d*)(\.\d{1,6})?$/.test(value) &&
  Number(value) <= 1_000_000;

export function officialReferenceError(
  value: unknown,
): "shape" | "source" | "date" | "scope" | "price" | null {
  if (!value || typeof value !== "object") return "shape";
  const item = value as OfficialPriceReference;
  if (
    item.currency !== "USD" ||
    !["current", "legacy"].includes(item.status) ||
    !item.rate ||
    !["tokens", "unit"].includes(item.rate.mode)
  )
    return "shape";
  try {
    const url = new URL(item.sourceUrl);
    if (url.protocol !== "https:" || url.username || url.password)
      return "source";
  } catch {
    return "source";
  }
  if (
    typeof item.verifiedAt !== "string" ||
    !/^\d{4}-\d{2}-\d{2}$/.test(item.verifiedAt) ||
    !Number.isFinite(Date.parse(item.verifiedAt)) ||
    new Date(item.verifiedAt).toISOString().slice(0, 10) !== item.verifiedAt ||
    item.verifiedAt > new Date().toISOString().slice(0, 10)
  )
    return "date";
  if (
    !item.scope ||
    !["zh", "en", "ar"].every(
      (lang) =>
        typeof item.scope[lang as keyof typeof item.scope] === "string" &&
        item.scope[lang as keyof typeof item.scope].trim().length > 0 &&
        item.scope[lang as keyof typeof item.scope].length <= 1200,
    )
  )
    return "scope";
  if (item.rate.mode === "tokens") {
    const prices = item.rate.pricing;
    if (
      !prices ||
      !validPrice(prices.input_per_million_usd) ||
      !validPrice(prices.output_per_million_usd) ||
      [
        prices.cache_read_per_million_usd,
        prices.cache_write_per_million_usd,
      ].some((price) => price !== undefined && !validPrice(price))
    )
      return "price";
  } else if (
    !["request", "image", "second"].includes(item.rate.unit) ||
    !validPrice(item.rate.priceUsd)
  )
    return "price";
  return null;
}

export function validateOfficialReference(
  value: unknown,
): value is OfficialPriceReference {
  return officialReferenceError(value) === null;
}

/** Explicit null survives refresh/publication; existing old configs gain only verified display metadata. */
export function getOfficialReference(
  modelId: string,
  config: PricingCatalogConfig,
): OfficialPriceReference | null {
  const entry = config.models[modelId];
  if (!entry) return null;
  const reference =
    entry.officialReference === undefined
      ? defaults[modelId]
      : entry.officialReference;
  return validateOfficialReference(reference)
    ? structuredClone(reference)
    : null;
}
