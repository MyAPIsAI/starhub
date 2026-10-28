import { placeholders } from "../config/placeholders";
import {
  getKeyModelOptions,
  isSmartModelId,
  SMART_MODEL_ID,
  type KeyModelPolicy,
} from "./key-model-policy";
import type { PricingCatalogConfig } from "../config/models";
import type { DemoProtocol } from "./request-contract";

export type ClientTarget = "openai" | "claude-code" | "codex";
export type ConnectionTool = ClientTarget | "cc-switch";
export const clientProtocols: Record<ClientTarget, DemoProtocol | null> = {
  openai: "OpenAI",
  "claude-code": "Anthropic",
  codex: null,
};
export function getClientModels(
  key: KeyModelPolicy,
  target: ClientTarget,
  config?: PricingCatalogConfig,
) {
  const protocol = clientProtocols[target];
  return protocol ? getKeyModelOptions(key, protocol, config) : [];
}
export function findClientModel(
  options: ReturnType<typeof getClientModels>,
  requestedModel: string,
) {
  return options.find(
    (option) =>
      option.id === requestedModel ||
      (option.kind === "smart" &&
        isSmartModelId(option.id) &&
        isSmartModelId(requestedModel)),
  );
}
export function shellQuote(value: string) {
  return `'${value.replace(/'/g, `'"'"'`)}'`;
}
export function validDemoSecret(value?: string) {
  return Boolean(value && value.startsWith("sk-") && !/[•●*\s]/.test(value));
}
export function buildClientConfiguration(input: {
  target: ClientTarget;
  model: string;
  secret?: string;
  apiBase?: string;
}) {
  if (input.target === "codex") return null;
  if (!input.model) return null;
  if (input.secret && !validDemoSecret(input.secret))
    throw new Error("A masked key is not a credential");
  const model = isSmartModelId(input.model) ? SMART_MODEL_ID : input.model;
  const secret = input.secret ? shellQuote(input.secret) : '"$MYAPIS_API_KEY"';
  const baseUrl =
    input.target === "claude-code"
      ? (input.apiBase ?? placeholders.apiBase).replace(/\/v1\/?$/, "")
      : (input.apiBase ?? placeholders.apiBase);
  const protocol = clientProtocols[input.target]!;
  const mapping =
    input.target === "claude-code"
      ? {
          ANTHROPIC_MODEL: model,
          ANTHROPIC_DEFAULT_OPUS_MODEL: model,
          ANTHROPIC_DEFAULT_SONNET_MODEL: model,
          ANTHROPIC_DEFAULT_HAIKU_MODEL: model,
          CLAUDE_CODE_SUBAGENT_MODEL: model,
        }
      : { OPENAI_MODEL: model };
  const variables =
    input.target === "claude-code"
      ? { ANTHROPIC_BASE_URL: baseUrl, ...mapping }
      : { OPENAI_BASE_URL: baseUrl, ...mapping };
  const secretName =
    input.target === "claude-code" ? "ANTHROPIC_AUTH_TOKEN" : "OPENAI_API_KEY";
  const snippet = [
    "# MyAPIs gateway configuration; verify connectivity in your client.",
    ...(!input.secret
      ? [
          "# Set MYAPIS_API_KEY to your full gateway credential before using these values.",
        ]
      : []),
    `export ${secretName}=${secret}`,
    ...Object.entries(variables).map(
      ([name, value]) => `export ${name}=${shellQuote(value)}`,
    ),
  ].join("\n");
  return {
    baseUrl,
    protocol,
    model,
    secretName,
    credential: input.secret ?? "$MYAPIS_API_KEY",
    mapping,
    snippet,
  };
}
