import { message as translatedMessage } from "../i18n/messages";
// All provisional product choices are centralized here. No live endpoints or secrets.
export const placeholders = {
  brand: {
    en: "MyAPIs",
    zh: "MyAPIs",
    ar: "MyAPIs",
    domain: "myapis.ai",
  },
  region: {
    id: "me",
    country: "AE",
    currency: "USD",
    timeZone: "Asia/Dubai",
    defaultLanguage: "en",
  },
  tiers: [
    {
      id: "ultra",
      grade: 10,
      name: {
        en: translatedMessage("ui_ultra_c53348c4")[1],
        zh: translatedMessage("ui_ultra_c53348c4")[0],
        ar: translatedMessage("ui_ultra_c53348c4")[2],
      },
      multiplier: "1.0000",
      success: "99.5",
      p95: 800,
      mark: 4,
    },
    {
      id: "pro",
      grade: 8,
      name: {
        en: translatedMessage("ui_pro_b9ce22e1")[1],
        zh: translatedMessage("ui_pro_b9ce22e1")[0],
        ar: translatedMessage("ui_pro_b9ce22e1")[2],
      },
      multiplier: "0.8000",
      success: "99.0",
      p95: 1500,
      mark: 3,
    },
    {
      id: "standard",
      grade: 6,
      name: {
        en: translatedMessage("ui_standard_3a68c577")[1],
        zh: translatedMessage("ui_standard_3a68c577")[0],
        ar: translatedMessage("ui_standard_3a68c577")[2],
      },
      multiplier: "0.6000",
      success: "97.5",
      p95: 3000,
      mark: 2,
    },
    {
      id: "lite",
      grade: 4,
      name: {
        en: translatedMessage("ui_lite_c5c821f6")[1],
        zh: translatedMessage("ui_lite_c5c821f6")[0],
        ar: translatedMessage("ui_lite_c5c821f6")[2],
      },
      multiplier: "0.4000",
      success: "97.5",
      p95: 3000,
      mark: 1,
    },
  ],
  apiBase: "https://api.myapis.ai/v1",
  launchModels: [
    "gpt-4.1",
    "claude-sonnet-4-20250514",
    "gemini-2.5-pro",
    "deepseek-chat",
    "qwen-plus",
  ],
  payment: {
    provider: "mock",
    minimum_nano: "10000000000",
    maximum_nano: "10000000000000",
  },
  balanceAlert_nano: "5000000000",
  closureDust_nano: "10000000",
  legal: {
    version: "demo-1",
    effectiveAt: "2026-09-18T00:00:00+04:00",
    chineseEnabled: true,
    retentionYears: null,
    taxRate: null,
  },
  cookieConsent: "necessary-only",
  emailHourlyLimit: 10,
  features: {
    arabic: true,
    videoCatalog: true,
    textPlaygroundOnly: true,
    countTokens: false,
  },
  priorities: ["reliability", "control", "compatibility", "price"],
  externalInputs: {
    toolVersions: null,
    productionModelPrices: null,
    successTargets: null,
    minimumDeployment: null,
    interfaceContract: null,
  },
  mock: {
    minimumDelay: 300,
    maximumDelay: 800,
    now: "2026-09-18T14:42:00+04:00",
  },
} as const;

export type TierId = string;
export type Language = "en" | "zh" | "ar" | "ko";
