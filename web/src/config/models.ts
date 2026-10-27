import { placeholders, type TierId } from "./placeholders";

export interface ModelPricing {
  input_per_million_usd: string; // e.g. "2.50"
  output_per_million_usd: string; // e.g. "10.00"
  cache_read_per_million_usd?: string; // e.g. "0.25"
  cache_write_per_million_usd?: string; // e.g. "1.25"
}

export interface ModelItem {
  id: string;
  name: string;
  vendor: string;
  capabilities: (
    "text" | "code" | "vision" | "reasoning" | "image" | "video"
  )[];
  contextLength: number; // in tokens
  contextDisplay: string; // e.g. "128k"
  pricing: ModelPricing;
  protocols: ("OpenAI" | "Anthropic" | "Gemini")[];
  description: {
    en: string;
    zh: string;
    ar: string;
  };
  tierAvailability: Record<TierId, boolean>;
  online: boolean;
}

export const catalogModels: ModelItem[] = [
  {
    id: "gpt-4.1",
    name: "GPT-4.1",
    vendor: "OpenAI",
    capabilities: ["text", "code", "vision", "reasoning"],
    contextLength: 128000,
    contextDisplay: "128K",
    pricing: {
      input_per_million_usd: "2.00",
      output_per_million_usd: "8.00",
      cache_read_per_million_usd: "0.50",
      cache_write_per_million_usd: "2.00",
    },
    protocols: ["OpenAI"],
    description: {
      en: "Flagship general intelligence model with superior instruction following and multimodality.",
      zh: "通用智能旗舰模型，具备卓越的代码生成、复杂推理与多模态理解能力。",
      ar: "نموذج الذكاء العام الرائد مع قدرات فائقة في البرمجة والتفكير متعدد الوسائط.",
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: false },
    online: true,
  },
  {
    id: "claude-sonnet-4-20250514",
    name: "Claude Sonnet 4",
    vendor: "Anthropic",
    capabilities: ["text", "code", "vision", "reasoning"],
    contextLength: 200000,
    contextDisplay: "200K",
    pricing: {
      input_per_million_usd: "3.00",
      output_per_million_usd: "15.00",
      cache_read_per_million_usd: "0.30",
      cache_write_per_million_usd: "3.75",
    },
    protocols: ["Anthropic", "OpenAI"],
    description: {
      en: "State-of-the-art coding and agentic execution with 200K context window and prompt caching.",
      zh: "面向代码与智能体自动化的模型，超强结构化输出与超长上下文敏捷检索。",
      ar: "أحدث نموذج للبرمجة وتنفيذ الوكلاء مع نافذة سياق 200 ألف وتخزين مؤقت للأوامر.",
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "gemini-2.5-pro",
    name: "Gemini 2.5 Pro",
    vendor: "Google",
    capabilities: ["text", "code", "vision", "reasoning"],
    contextLength: 1000000,
    contextDisplay: "1M",
    pricing: {
      input_per_million_usd: "1.25",
      output_per_million_usd: "5.00",
      cache_read_per_million_usd: "0.31",
      cache_write_per_million_usd: "1.25",
    },
    protocols: ["Gemini", "OpenAI"],
    description: {
      en: "Massive 1M token context window model with extreme multi-modal ingestion efficiency.",
      zh: "百万级超长上下文多模态模型，支持海量文档、音视频与代码库即时分析。",
      ar: "نموذج بسياق هائل يبلغ مليون رمز مع كفاءة استيعاب وسائط متعددة متطرفة.",
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "deepseek-chat",
    name: "DeepSeek V3",
    vendor: "DeepSeek",
    capabilities: ["text", "code", "reasoning"],
    contextLength: 64000,
    contextDisplay: "64K",
    pricing: {
      input_per_million_usd: "0.27",
      output_per_million_usd: "1.10",
      cache_read_per_million_usd: "0.07",
      cache_write_per_million_usd: "0.27",
    },
    protocols: ["OpenAI"],
    description: {
      en: "High-efficiency open architecture model with unrivaled cost-performance ratio.",
      zh: "高性价比全能基座模型，在代码、中文及数学推理领域表现极具竞争力。",
      ar: "نموذج معماري مفتوح عالي الكفاءة مع نسبة أداء إلى تكلفة لا تضاهى.",
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "qwen-plus",
    name: "Qwen Plus",
    vendor: "Qwen",
    capabilities: ["text", "code", "vision"],
    contextLength: 128000,
    contextDisplay: "128K",
    pricing: {
      input_per_million_usd: "0.40",
      output_per_million_usd: "1.20",
      cache_read_per_million_usd: "0.10",
      cache_write_per_million_usd: "0.40",
    },
    protocols: ["OpenAI"],
    description: {
      en: "Versatile enterprise-grade model tailored for complex business workflows and tool calling.",
      zh: "专为企业级复杂任务打造的模型，工具调用敏锐，中英双语表达自然流畅。",
      ar: "نموذج متكامل للمؤسسات مصمم لمسارات العمل المعقدة واستدعاء الأدوات.",
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "veyra-demo-image",
    name: "Veyra Image（图片计费样例）",
    vendor: "Veyra",
    capabilities: ["image"],
    contextLength: 0,
    contextDisplay: "—",
    pricing: { input_per_million_usd: "0", output_per_million_usd: "0" },
    protocols: [],
    description: {
      zh: "按张计费演示；只验证用量和账本，不生成图片，也不提供客户端接口。",
      en: "Per-image billing demonstration only. No image generation or client API.",
      ar: "عرض فوترة الصور فقط؛ لا يُنشئ صورًا ولا يوفّر واجهة عميل.",
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "veyra-demo-video",
    name: "Veyra Video（视频计费样例）",
    vendor: "Veyra",
    capabilities: ["video"],
    contextLength: 0,
    contextDisplay: "—",
    pricing: { input_per_million_usd: "0", output_per_million_usd: "0" },
    protocols: [],
    description: {
      zh: "按秒计费演示；只验证时长和账本，不生成视频，也不提供客户端接口。",
      en: "Per-second billing demonstration only. No video generation or client API.",
      ar: "عرض فوترة الثواني فقط؛ لا يُنشئ فيديو ولا يوفّر واجهة عميل.",
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "myapis-auto",
    name: "MyAPIs Auto",
    vendor: "MyAPIs",
    capabilities: ["text", "code", "vision", "reasoning"],
    contextLength: 0,
    contextDisplay: "—",
    pricing: { input_per_million_usd: "0", output_per_million_usd: "0" },
    protocols: ["OpenAI", "Anthropic", "Gemini"],
    description: {
      zh: "智能路由模型。在密钥授权范围内自动选择成本最低的可用模型。填写 model 为 myapis-auto 即可；实际执行的模型在调用明细中记录。",
      en: "Smart routing model. Automatically selects the lowest-cost available model within the key's authorized scope. Set model to myapis-auto; the actual model used is recorded in request logs.",
      ar: "نموذج التوجيه الذكي. يختار تلقائيًا النموذج الأقل تكلفة المتاح ضمن نطاق المفتاح المصرّح به. اضبط model على myapis-auto؛ النموذج الفعلي يُسجّل في سجل الطلبات.",
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
];

export interface PricingTimeWindow {
  id: string;
  label: string;
  timezone: string;
  /** 0 = Sunday; overnight windows belong to their starting day. */
  days: number[];
  start: string;
  end: string;
  multiplier: string;
  tokenPrices?: Partial<ModelPricing>;
}
export interface PricingAttributes {
  resolution?: string;
  quality?: string;
  audio?: boolean;
}
export interface ConditionalUnitPrice {
  id: string;
  label: string;
  when: PricingAttributes;
  priceUsd: string;
}
export type PricingRule = (
  | { mode: "tokens" }
  | {
      mode: "unit";
      unit: "request" | "image" | "second";
      priceUsd: string;
      minimum: string;
      ceilTo: string;
      conditionRates?: ConditionalUnitPrice[];
    }
  | { mode: "expression"; expression: string }
) & { timeWindows?: PricingTimeWindow[] };
/** Independently verified provider reference; never participates in platform billing. */
export interface OfficialPriceReference {
  currency: "USD";
  status: "current" | "legacy";
  rate:
    | { mode: "tokens"; pricing: ModelPricing }
    | { mode: "unit"; unit: "request" | "image" | "second"; priceUsd: string };
  scope: { zh: string; en: string; ar: string };
  sourceUrl: string;
  verifiedAt: string;
}
export interface ModelPricingEntry {
  pricing: ModelPricing;
  rule: PricingRule;
  tierAvailability: Record<TierId, boolean>;
  source: "sample" | "manual-official";
  sourceUrl: string;
  verifiedAt: string | null;
  effectiveAt: string | null;
  version: string;
  /** undefined uses a vetted default, null explicitly removes the reference. */
  officialReference?: OfficialPriceReference | null;
}
export interface ServiceGroupConfig {
  visibility?: "common" | "custom";
  accountIds?: string[];
  name: string;
  description: string;
  enabled: boolean;
}

/** Reference price × service-group multiplier. Defaults are unverified examples. */
export interface ResourceChannel {
  id: string;
  name: string;
  provider: string;
  enabled: boolean;
}
export interface ResourceInstance {
  id: string;
  channelId: string;
  upstreamModel: string;
  canonicalModel: string;
  protocols: ModelItem["protocols"];
  capabilities: ModelItem["capabilities"];
  contextLength: number;
  region: string;
  status: "available" | "unavailable" | "retired";
  latencyMs: number;
}
export interface ResourceSnapshot {
  id: string;
  sequence: number;
  publishedAt: string;
  receivedAt: string;
  source: string;
  channels: ResourceChannel[];
  instances: ResourceInstance[];
}
export interface ResourcePlacement {
  modelId: string | null;
  groupIds: string[];
  enabled: boolean;
}
export interface PricingCatalogConfig {
  catalog?: Record<string, ModelItem>;
  resources?: ResourceSnapshot;
  placements?: Record<string, ResourcePlacement>;
  revision: string;
  updatedAt: string | null;
  models: Record<string, ModelPricingEntry>;
  multipliers: Record<TierId, string>;
  groups: Record<TierId, ServiceGroupConfig>;
}

/** Exact decimal multiplication; prices retain up to 10 decimal places. */
export function multiplyDecimalPrice(a: string, b: string): string {
  const toScaled = (value: string) => {
    const [whole, fraction = ""] = value.split(".");
    return { integer: BigInt(whole + fraction), scale: fraction.length };
  };
  const left = toScaled(a);
  const right = toScaled(b);
  const scale = left.scale + right.scale;
  const raw = (left.integer * right.integer)
    .toString()
    .padStart(scale + 1, "0");
  if (!scale) return raw;
  return `${raw.slice(0, -scale)}.${raw.slice(-scale)}`
    .replace(/0+$/, "")
    .replace(/\.$/, "");
}

export function getModelTierPrices(
  model: ModelItem,
  config?: PricingCatalogConfig,
) {
  const current = config?.models[model.id] ?? model;
  return (
    config
      ? Object.keys(config.groups).map((id) => ({
          ...(placeholders.tiers.find((item) => item.id === id) ?? {
            id,
            name: {
              zh: config.groups[id].name,
              en: config.groups[id].name,
              ar: config.groups[id].name,
            },
            mark: 1,
            multiplier: config.multipliers[id],
          }),
        }))
      : placeholders.tiers
  ).map((tier) => {
    const multiplier = config?.multipliers[tier.id] ?? tier.multiplier;
    return {
      tier: { ...tier, multiplier },
      available:
        model.online &&
        current.tierAvailability[tier.id] &&
        (config?.groups[tier.id]?.enabled ?? true),
      input: multiplyDecimalPrice(
        current.pricing.input_per_million_usd,
        multiplier,
      ),
      output: multiplyDecimalPrice(
        current.pricing.output_per_million_usd,
        multiplier,
      ),
      cacheRead:
        current.pricing.cache_read_per_million_usd !== undefined
          ? multiplyDecimalPrice(
              current.pricing.cache_read_per_million_usd,
              multiplier,
            )
          : undefined,
      cacheWrite:
        current.pricing.cache_write_per_million_usd !== undefined
          ? multiplyDecimalPrice(
              current.pricing.cache_write_per_million_usd,
              multiplier,
            )
          : undefined,
    };
  });
}

export function getModelLowestPrice(
  model: ModelItem,
  config?: PricingCatalogConfig,
): {
  asLowAsUsd: string;
  tierId: TierId;
  available: boolean;
} {
  const rates = getModelTierPrices(model, config).filter(
    (rate) => rate.available,
  );
  const lowest = rates.sort((a, b) => Number(a.input) - Number(b.input))[0];
  return {
    asLowAsUsd: lowest?.input ?? "—",
    tierId: lowest?.tier.id ?? "standard",
    available: Boolean(lowest),
  };
}
