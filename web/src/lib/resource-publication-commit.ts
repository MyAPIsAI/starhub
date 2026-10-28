import {
  readPublicationPreview,
  type PublicationPreview,
  type PublicationBinding,
  type PublicationRoute,
} from "./resource-publication";

export interface PublicationAcknowledgement {
  publication_id: string;
  review_digest: string;
  resource_revision: number;
  channel_revision: number;
  option_version: number;
  profile: "openai_text_exact_v1";
  binding_count: number;
  route_count: number;
  published: true;
  operation: "publish";
}
export interface PublicationReceipt extends PublicationAcknowledgement {
  actor_id: string;
  created_at: number;
  bindings: PublicationBinding[];
  routes: PublicationRoute[];
  historical: true;
  committed: true;
}
const object = (value: unknown): value is Record<string, unknown> =>
  Boolean(value && typeof value === "object" && !Array.isArray(value));
const version = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const id = (value: unknown): value is string =>
  typeof value === "string" &&
  /^[1-9]\d{0,18}$/.test(value) &&
  BigInt(value) <= 9223372036854775807n;
const text = (value: unknown, max: number): value is string =>
  typeof value === "string" &&
  value.length > 0 &&
  value.trim() === value &&
  new TextEncoder().encode(value).length <= max &&
  !/[\u0000-\u001f\u007f-\u009f]/.test(value);
const invalid = (): never => {
  throw Error("Invalid resource publication acknowledgement or receipt");
};

export function readPublicationAcknowledgement(
  value: unknown,
): PublicationAcknowledgement {
  if (
    !object(value) ||
    !id(value.publication_id) ||
    typeof value.review_digest !== "string" ||
    !/^[0-9a-f]{64}$/.test(value.review_digest) ||
    !version(value.resource_revision) ||
    value.resource_revision < 1 ||
    !version(value.channel_revision) ||
    value.channel_revision < 1 ||
    !version(value.option_version) ||
    value.profile !== "openai_text_exact_v1" ||
    value.published !== true ||
    value.operation !== "publish" ||
    !version(value.binding_count) ||
    value.binding_count < 1 ||
    value.binding_count > 500 ||
    !version(value.route_count) ||
    value.route_count < 1 ||
    value.route_count > 10000
  )
    return invalid();
  return value as unknown as PublicationAcknowledgement;
}

export function acknowledgementMatchesReview(
  value: PublicationAcknowledgement,
  review: PublicationPreview,
): boolean {
  return (
    value.review_digest === review.review_digest &&
    value.resource_revision === review.resource_revision + 1 &&
    value.channel_revision === review.channel_revision + 1 &&
    value.option_version === review.option_version &&
    value.binding_count === review.bindings.length &&
    value.route_count === review.proposed_routes.length
  );
}

export function publicationCommitRequest(review: PublicationPreview) {
  readPublicationPreview(review);
  if (
    !review.ready ||
    review.publish_available !== true ||
    review.resource_revision >= Number.MAX_SAFE_INTEGER ||
    review.channel_revision >= Number.MAX_SAFE_INTEGER
  )
    return invalid();
  return {
    expected_resource_revision: review.resource_revision,
    expected_channel_revision: review.channel_revision,
    expected_option_version: review.option_version,
    bindings: review.bindings.map((binding) => ({ ...binding })),
    review_digest: review.review_digest,
  };
}

export function readPublicationReceipt(value: unknown): PublicationReceipt {
  readPublicationAcknowledgement(value);
  if (
    !object(value) ||
    value.historical !== true ||
    value.committed !== true ||
    !id(value.actor_id) ||
    !version(value.created_at) ||
    value.created_at < 1 ||
    !Number.isFinite(new Date(value.created_at * 1000).getTime()) ||
    !Array.isArray(value.bindings) ||
    !Array.isArray(value.routes) ||
    value.bindings.length !== value.binding_count ||
    value.routes.length !== value.route_count
  )
    return invalid();
  const targets = new Set<string>(),
    sources = new Set<string>();
  for (const binding of value.bindings) {
    if (
      !object(binding) ||
      !id(binding.gateway_channel_id) ||
      !text(binding.source_channel_id, 128) ||
      targets.has(binding.gateway_channel_id) ||
      sources.has(binding.source_channel_id)
    )
      return invalid();
    targets.add(binding.gateway_channel_id);
    sources.add(binding.source_channel_id);
  }
  const pairs = new Set<string>(),
    used = new Set<string>();
  for (const route of value.routes) {
    if (
      !object(route) ||
      !id(route.gateway_channel_id) ||
      !targets.has(route.gateway_channel_id) ||
      !text(route.model, 128) ||
      !text(route.group, 64) ||
      !version(route.context_length) ||
      !(route.group_definition_id === "0" || id(route.group_definition_id)) ||
      ![
        JSON.stringify(["text"]),
        JSON.stringify(["code"]),
        JSON.stringify(["code", "text"]),
      ].includes(JSON.stringify(route.capabilities))
    )
      return invalid();
    const key = JSON.stringify([
      route.gateway_channel_id,
      route.model,
      route.group,
    ]);
    if (pairs.has(key)) return invalid();
    pairs.add(key);
    used.add(route.gateway_channel_id);
  }
  if (used.size !== targets.size) return invalid();
  return value as unknown as PublicationReceipt;
}

export function receiptMatchesReview(
  receipt: PublicationReceipt,
  review: PublicationPreview,
): boolean {
  if (!acknowledgementMatchesReview(receipt, review)) return false;
  const bindings = (items: PublicationBinding[]) =>
    JSON.stringify(
      items
        .map((item) => [item.source_channel_id, item.gateway_channel_id])
        .sort((a, b) => a[0].localeCompare(b[0])),
    );
  const routes = (items: PublicationRoute[]) =>
    JSON.stringify(
      items
        .map((item) => [
          item.gateway_channel_id,
          item.model,
          item.group,
          item.capabilities,
          item.context_length,
          item.group_definition_id,
        ])
        .sort((a, b) =>
          JSON.stringify(a.slice(0, 3)).localeCompare(
            JSON.stringify(b.slice(0, 3)),
          ),
        ),
    );
  return (
    bindings(receipt.bindings) === bindings(review.bindings) &&
    routes(receipt.routes) === routes(review.proposed_routes)
  );
}
