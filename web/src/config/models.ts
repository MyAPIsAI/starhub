import { message as translatedMessage } from "../i18n/messages";
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
    | "text"
    | "code"
    | "vision"
    | "reasoning"
    | "image"
    | "video"
    | "audio"
    | "embedding"
    | "rerank"
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
      en: translatedMessage(
        "ui_flagship_general_intelligence_model_with_supe_c5e4f938",
      )[1],
      zh: translatedMessage(
        "ui_flagship_general_intelligence_model_with_supe_c5e4f938",
      )[0],
      ar: translatedMessage(
        "ui_flagship_general_intelligence_model_with_supe_c5e4f938",
      )[2],
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
      en: translatedMessage(
        "ui_state_of_the_art_coding_and_agentic_execution_b8e97b6f",
      )[1],
      zh: translatedMessage(
        "ui_state_of_the_art_coding_and_agentic_execution_b8e97b6f",
      )[0],
      ar: translatedMessage(
        "ui_state_of_the_art_coding_and_agentic_execution_b8e97b6f",
      )[2],
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
      en: translatedMessage(
        "ui_massive_1m_token_context_window_model_with_ex_a974c0bf",
      )[1],
      zh: translatedMessage(
        "ui_massive_1m_token_context_window_model_with_ex_a974c0bf",
      )[0],
      ar: translatedMessage(
        "ui_massive_1m_token_context_window_model_with_ex_a974c0bf",
      )[2],
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
      en: translatedMessage(
        "ui_high_efficiency_open_architecture_model_with__7608da99",
      )[1],
      zh: translatedMessage(
        "ui_high_efficiency_open_architecture_model_with__7608da99",
      )[0],
      ar: translatedMessage(
        "ui_high_efficiency_open_architecture_model_with__7608da99",
      )[2],
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
      en: translatedMessage(
        "ui_versatile_enterprise_grade_model_tailored_for_11b2b934",
      )[1],
      zh: translatedMessage(
        "ui_versatile_enterprise_grade_model_tailored_for_11b2b934",
      )[0],
      ar: translatedMessage(
        "ui_versatile_enterprise_grade_model_tailored_for_11b2b934",
      )[2],
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "veyra-demo-image",
    name: "MyAPIs Image（图片计费样例）",
    vendor: "MyAPIs",
    capabilities: ["image"],
    contextLength: 0,
    contextDisplay: "—",
    pricing: { input_per_million_usd: "0", output_per_million_usd: "0" },
    protocols: [],
    description: {
      zh: translatedMessage(
        "ui_per_image_billing_demonstration_only_no_image_9c60c98b",
      )[0],
      en: translatedMessage(
        "ui_per_image_billing_demonstration_only_no_image_9c60c98b",
      )[1],
      ar: translatedMessage(
        "ui_per_image_billing_demonstration_only_no_image_9c60c98b",
      )[2],
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "veyra-demo-video",
    name: "MyAPIs Video（视频计费样例）",
    vendor: "MyAPIs",
    capabilities: ["video"],
    contextLength: 0,
    contextDisplay: "—",
    pricing: { input_per_million_usd: "0", output_per_million_usd: "0" },
    protocols: [],
    description: {
      zh: translatedMessage(
        "ui_per_second_billing_demonstration_only_no_vide_04556804",
      )[0],
      en: translatedMessage(
        "ui_per_second_billing_demonstration_only_no_vide_04556804",
      )[1],
      ar: translatedMessage(
        "ui_per_second_billing_demonstration_only_no_vide_04556804",
      )[2],
    },
    tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
    online: true,
  },
  {
    id: "myapis-auto",
    name: "MyAPIs Auto",
    vendor: "MyAPIs",
    capabilities: ["text"],
    contextLength: 0,
    contextDisplay: "—",
    pricing: { input_per_million_usd: "0", output_per_million_usd: "0" },
    protocols: ["OpenAI", "Anthropic", "Gemini"],
    description: {
      zh: translatedMessage(
        "ui_smart_routing_model_automatically_selects_the_b7a3d474",
      )[0],
      en: translatedMessage(
        "ui_smart_routing_model_automatically_selects_the_b7a3d474",
      )[1],
      ar: translatedMessage(
        "ui_smart_routing_model_automatically_selects_the_b7a3d474",
      )[2],
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
  price_tier?: 4 | 6 | 8 | 10;
  quality_level?: "S0" | "S1" | "S2" | "U";
  state?: "hot" | "cold" | "bad" | "disabled";
  trend?: "stable" | "degraded";
  score?: number;
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
  confirmIdentity?: boolean;
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
