export const RESOURCE_PUBLICATION_SCHEMA = "exact_model_pairs_preview_v1";

export interface PublicationChannel {
  id: string;
  name: string;
  type: number;
  status: number;
}
export interface PublicationContext {
  schema: typeof RESOURCE_PUBLICATION_SCHEMA;
  profile: "openai_text_exact_v1" | "exact_resource_policy_v2";
  supported_endpoints: string[];
  resource_revision: number;
  channel_revision: number;
  option_version: number;
  channels: PublicationChannel[];
  preview_only: true;
  publish_available?: boolean;
  affects_gateway_routing: false;
}
export interface PublicationBinding {
  source_channel_id: string;
  gateway_channel_id: string;
}
export interface PublicationPair {
  gateway_channel_id: string;
  model: string;
  group: string;
}
export interface PublicationRoute extends PublicationPair {
  upstream_model?: string;
  protocols?: string[];
  capabilities: string[];
  context_length: number;
  group_definition_id: string;
}
export interface PublicationIssue {
  resource_id?: string;
  source_channel_id?: string;
  group?: string;
  code: string;
}
export interface PublicationPreview extends PublicationContext {
  ready: boolean;
  bindings: PublicationBinding[];
  proposed_pairs: PublicationPair[];
  proposed_routes: PublicationRoute[];
  review_digest: string;
  existing_pairs: (PublicationPair & { enabled: boolean })[];
  issues: PublicationIssue[];
}
const object = (value: unknown): value is Record<string, unknown> =>
  Boolean(value && typeof value === "object" && !Array.isArray(value));
const text = (value: unknown, max: number): value is string =>
  typeof value === "string" &&
  value.trim() === value &&
  value.length > 0 &&
  new TextEncoder().encode(value).length <= max &&
  !/[\u0000-\u001f\u007f-\u009f]/.test(value);
const version = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const id = (value: unknown): value is string =>
  typeof value === "string" &&
  /^[1-9]\d{0,18}$/.test(value) &&
  BigInt(value) <= 9223372036854775807n;
const invalid = (): never => {
  throw Error("Unsupported or invalid resource publication response");
};

export function readPublicationContext(value: unknown): PublicationContext {
  if (
    !object(value) ||
    value.schema !== RESOURCE_PUBLICATION_SCHEMA ||
    !validPublicationProfile(value.profile) ||
    JSON.stringify(value.supported_endpoints) !==
      JSON.stringify(publicationEndpoints(value.profile)) ||
    value.preview_only !== true ||
    (value.publish_available !== undefined &&
      typeof value.publish_available !== "boolean") ||
    value.affects_gateway_routing !== false ||
    !version(value.resource_revision) ||
    !version(value.channel_revision) ||
    !version(value.option_version) ||
    !Array.isArray(value.channels) ||
    value.channels.length > 10000
  )
    return invalid();
  const ids = new Set<string>();
  for (const channel of value.channels) {
    if (
      !object(channel) ||
      !id(channel.id) ||
      ids.has(channel.id) ||
      typeof channel.name !== "string" ||
      !Number.isSafeInteger(channel.type) ||
      !Number.isSafeInteger(channel.status)
    )
      return invalid();
    ids.add(channel.id);
  }
  return value as unknown as PublicationContext;
}

export function publicationRequest(
  context: PublicationContext,
  bindings: PublicationBinding[],
) {
  readPublicationContext(context);
  if (bindings.length === 0 || bindings.length > 500) return invalid();
  const sources = new Set<string>(),
    targets = new Set<string>();
  for (const binding of bindings) {
    if (
      !text(binding.source_channel_id, 128) ||
      !id(binding.gateway_channel_id) ||
      sources.has(binding.source_channel_id) ||
      targets.has(binding.gateway_channel_id) ||
      !context.channels.some((ch) => ch.id === binding.gateway_channel_id)
    )
      return invalid();
    sources.add(binding.source_channel_id);
    targets.add(binding.gateway_channel_id);
  }
  return {
    expected_resource_revision: context.resource_revision,
    expected_channel_revision: context.channel_revision,
    expected_option_version: context.option_version,
    bindings: bindings.map((binding) => ({ ...binding })),
  };
}

export function readPublicationPreview(value: unknown): PublicationPreview {
  const context = readPublicationContext(value);
  if (
    !object(value) ||
    typeof value.ready !== "boolean" ||
    !Array.isArray(value.bindings) ||
    !Array.isArray(value.proposed_pairs) ||
    !Array.isArray(value.proposed_routes) ||
    value.proposed_routes.length !== value.proposed_pairs.length ||
    typeof value.review_digest !== "string" ||
    !/^[0-9a-f]{64}$/.test(value.review_digest) ||
    !Array.isArray(value.existing_pairs) ||
    !Array.isArray(value.issues) ||
    value.proposed_pairs.length > 10000 ||
    value.existing_pairs.length > 10000 ||
    value.issues.length > 11000
  )
    return invalid();
  publicationRequest(context, value.bindings as PublicationBinding[]);
  const targets = new Set(
    value.bindings.map(
      (binding: PublicationBinding) => binding.gateway_channel_id,
    ),
  );
  for (const entries of [value.proposed_pairs, value.existing_pairs]) {
    const pairs = new Set<string>();
    for (const pair of entries) {
      if (
        !object(pair) ||
        !id(pair.gateway_channel_id) ||
        !targets.has(pair.gateway_channel_id) ||
        !text(pair.model, 128) ||
        !text(pair.group, 64) ||
        (entries === value.existing_pairs && typeof pair.enabled !== "boolean")
      )
        return invalid();
      const key = JSON.stringify([
        pair.gateway_channel_id,
        pair.model,
        pair.group,
      ]);
      if (pairs.has(key)) return invalid();
      pairs.add(key);
    }
  }
  const proposed = new Set(
    value.proposed_pairs.map((pair: PublicationPair) =>
      JSON.stringify([pair.gateway_channel_id, pair.model, pair.group]),
    ),
  );
  const routes = new Set<string>();
  for (const route of value.proposed_routes) {
    if (
      !object(route) ||
      !id(route.gateway_channel_id) ||
      !text(route.model, 128) ||
      !text(route.group, 64) ||
      !version(route.context_length) ||
      !(route.group_definition_id === "0" || id(route.group_definition_id)) ||
      !validPublicationRoute(route, context.profile)
    )
      return invalid();
    const key = JSON.stringify([
      route.gateway_channel_id,
      route.model,
      route.group,
    ]);
    if (!proposed.has(key) || routes.has(key)) return invalid();
    routes.add(key);
  }
  for (const issue of value.issues) {
    if (
      !object(issue) ||
      !text(issue.code, 128) ||
      (issue.resource_id !== undefined && !text(issue.resource_id, 128)) ||
      (issue.source_channel_id !== undefined &&
        !text(issue.source_channel_id, 128)) ||
      (issue.group !== undefined && !text(issue.group, 128))
    )
      return invalid();
  }
  if (
    value.ready !== (value.issues.length === 0) ||
    (value.ready && value.proposed_pairs.length === 0)
  )
    return invalid();
  return value as unknown as PublicationPreview;
}

export const advancedResourceCapabilities = [
  "text",
  "code",
  "vision",
  "reasoning",
  "function_call",
  "responses",
  "image",
  "video",
  "audio",
  "embedding",
  "rerank",
];
export const validPublicationProfile = (
  value: unknown,
): value is PublicationContext["profile"] =>
  value === "openai_text_exact_v1" || value === "exact_resource_policy_v2";
export const publicationEndpoints = (profile: PublicationContext["profile"]) =>
  profile === "openai_text_exact_v1"
    ? ["/v1/chat/completions", "/v1/messages"]
    : [
        "/v1/chat/completions",
        "/v1/messages",
        "/v1/responses",
        "/v1/responses/compact",
        "/v1beta/models",
        "/v1/images/generations",
        "/v1/images/edits",
        "/v1/video/generations",
        "/v1/embeddings",
        "/v1/rerank",
        "/v1/audio/speech",
        "/v1/audio/transcriptions",
        "/v1/audio/translations",
      ];
export function validPublicationRoute(
  route: Record<string, unknown>,
  profile: PublicationContext["profile"],
): boolean {
  if (profile === "openai_text_exact_v1")
    return (
      route.upstream_model === undefined &&
      route.protocols === undefined &&
      ['["text"]', '["code"]', '["code","text"]'].includes(
        JSON.stringify(route.capabilities),
      )
    );
  return (
    text(route.upstream_model, 128) &&
    Array.isArray(route.protocols) &&
    route.protocols.length === 1 &&
    ["OpenAI", "Anthropic", "Gemini"].includes(route.protocols[0]) &&
    Array.isArray(route.capabilities) &&
    route.capabilities.length > 0 &&
    new Set(route.capabilities).size === route.capabilities.length &&
    route.capabilities.every(
      (cap: unknown) =>
        typeof cap === "string" && advancedResourceCapabilities.includes(cap),
    )
  );
}
