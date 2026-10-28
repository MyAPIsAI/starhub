import {
  readResourceBindingState,
  type ResourceBindingState,
  type ResourceBindingStatus,
} from "./resource-binding-state";

export interface BindingChangeReview {
  resource_revision: number;
  channel_revision: number;
  row: ResourceBindingStatus;
  operation: "revoke" | "release";
  request_id: string;
}
export interface BindingChangeReceipt {
  schema: "managed_binding_change_v1";
  change_id: string;
  request_id: string;
  actor_id: string;
  operation: "revoke" | "release";
  gateway_channel_id: string;
  source_channel_id: string;
  retained_source_channel_id: string;
  publication_id: string;
  profile: string;
  was_configured_active: boolean;
  removed_route_count: number;
  removed_ability_count: number;
  resource_revision: number;
  channel_revision: number;
  created_at: number;
  historical: true;
  committed: true;
  protective_marker_retained: true;
}
const object = (value: unknown): value is Record<string, unknown> =>
  Boolean(value && typeof value === "object" && !Array.isArray(value));
const integer = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const id = (value: unknown): value is string =>
  typeof value === "string" &&
  /^[1-9]\d{0,18}$/.test(value) &&
  BigInt(value) <= 9223372036854775807n;
const requestID = (value: unknown): value is string =>
  typeof value === "string" && /^[0-9a-f]{32}$/.test(value);
const text = (value: unknown, max: number, required = true): value is string =>
  typeof value === "string" &&
  (!required || value.length > 0) &&
  value.trim() === value &&
  new TextEncoder().encode(value).length <= max &&
  !/[\u0000-\u001f\u007f-\u009f]/.test(value);
const invalid = (): never => {
  throw Error("Invalid binding change review or receipt");
};

export function prepareBindingChange(
  state: ResourceBindingState,
  row: ResourceBindingStatus,
  operation: BindingChangeReview["operation"],
  key: string,
): BindingChangeReview {
  readResourceBindingState(state, state.page, state.page_size);
  if (
    state.binding_changes_available !== true ||
    state.resource_revision >= Number.MAX_SAFE_INTEGER ||
    state.channel_revision >= Number.MAX_SAFE_INTEGER ||
    !requestID(key) ||
    !text(row.source_channel_id, 128) ||
    !text(row.profile, 64, false) ||
    !["revoke", "release"].includes(operation) ||
    !state.bindings.some((item) => JSON.stringify(item) === JSON.stringify(row))
  )
    return invalid();
  return {
    resource_revision: state.resource_revision,
    channel_revision: state.channel_revision,
    row: { ...row },
    operation,
    request_id: key,
  };
}

export function bindingChangeRequest(review: BindingChangeReview) {
  if (
    !integer(review.resource_revision) ||
    review.resource_revision >= Number.MAX_SAFE_INTEGER ||
    !integer(review.channel_revision) ||
    review.channel_revision >= Number.MAX_SAFE_INTEGER ||
    !requestID(review.request_id) ||
    !id(review.row.gateway_channel_id) ||
    !(review.row.publication_id === "0" || id(review.row.publication_id)) ||
    !text(review.row.source_channel_id, 128) ||
    !["revoke", "release"].includes(review.operation)
  )
    return invalid();
  return {
    expected_resource_revision: review.resource_revision,
    expected_channel_revision: review.channel_revision,
    expected_source_channel_id: review.row.source_channel_id,
    expected_publication_id: review.row.publication_id,
    operation: review.operation,
    request_id: review.request_id,
  };
}

export function readBindingChangeReceipt(value: unknown): BindingChangeReceipt {
  if (
    !object(value) ||
    value.schema !== "managed_binding_change_v1" ||
    !id(value.change_id) ||
    !id(value.actor_id) ||
    !id(value.gateway_channel_id) ||
    !(value.publication_id === "0" || id(value.publication_id)) ||
    !requestID(value.request_id) ||
    !text(value.source_channel_id, 128) ||
    !text(value.profile, 64, false) ||
    (value.operation !== "revoke" && value.operation !== "release") ||
    value.retained_source_channel_id !==
      (value.operation === "release" ? "" : value.source_channel_id) ||
    typeof value.was_configured_active !== "boolean" ||
    !integer(value.removed_route_count) ||
    !integer(value.removed_ability_count) ||
    !integer(value.resource_revision) ||
    value.resource_revision < 1 ||
    !integer(value.channel_revision) ||
    value.channel_revision < 1 ||
    !integer(value.created_at) ||
    value.created_at < 1 ||
    !Number.isFinite(new Date(value.created_at * 1000).getTime()) ||
    value.historical !== true ||
    value.committed !== true ||
    value.protective_marker_retained !== true
  )
    return invalid();
  return value as unknown as BindingChangeReceipt;
}

export function bindingReceiptMatchesReview(
  receipt: BindingChangeReceipt,
  review: BindingChangeReview,
): boolean {
  return (
    receipt.request_id === review.request_id &&
    receipt.operation === review.operation &&
    receipt.gateway_channel_id === review.row.gateway_channel_id &&
    receipt.source_channel_id === review.row.source_channel_id &&
    receipt.publication_id === review.row.publication_id &&
    receipt.profile === review.row.profile &&
    receipt.was_configured_active === review.row.configured_active &&
    receipt.removed_route_count === review.row.stored_pair_count &&
    receipt.resource_revision === review.resource_revision + 1 &&
    receipt.channel_revision === review.channel_revision + 1
  );
}
