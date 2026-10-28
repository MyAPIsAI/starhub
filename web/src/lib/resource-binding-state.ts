export interface ResourceBindingStatus {
  gateway_channel_id: string;
  gateway_name: string;
  gateway_present: boolean;
  gateway_status: number;
  gateway_type: number;
  source_channel_id: string;
  source_present: boolean;
  source_enabled: boolean;
  configured_active: boolean;
  profile: string;
  publication_id: string;
  stored_pair_count: number;
  route_eligible_pair_count: number;
}
export interface ResourceBindingState {
  schema: "managed_gateway_bindings_v1";
  resource_revision: number;
  channel_revision: number;
  option_version: number;
  page: number;
  page_size: number;
  total: number;
  bindings: ResourceBindingStatus[];
  read_only: true;
}
const object = (value: unknown): value is Record<string, unknown> =>
  Boolean(value && typeof value === "object" && !Array.isArray(value));
const integer = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const id = (value: unknown): value is string =>
  typeof value === "string" &&
  /^[1-9]\d{0,18}$/.test(value) &&
  BigInt(value) <= 9223372036854775807n;
const invalid = (): never => {
  throw Error("Invalid managed gateway binding state");
};
export function readResourceBindingState(
  value: unknown,
  page: number,
  pageSize: number,
): ResourceBindingState {
  if (
    !object(value) ||
    value.schema !== "managed_gateway_bindings_v1" ||
    value.read_only !== true ||
    !integer(value.resource_revision) ||
    !integer(value.channel_revision) ||
    !integer(value.option_version) ||
    !integer(value.total) ||
    value.page !== page ||
    value.page_size !== pageSize ||
    !integer(page) ||
    page < 1 ||
    page > 1000000 ||
    !integer(pageSize) ||
    pageSize < 1 ||
    pageSize > 100 ||
    !Array.isArray(value.bindings) ||
    value.bindings.length !==
      Math.min(pageSize, Math.max(0, value.total - (page - 1) * pageSize))
  )
    return invalid();
  const seen = new Set<string>();
  let previous = 0n;
  for (const row of value.bindings) {
    if (
      !object(row) ||
      !id(row.gateway_channel_id) ||
      seen.has(row.gateway_channel_id) ||
      BigInt(row.gateway_channel_id) <= previous ||
      !(row.publication_id === "0" || id(row.publication_id)) ||
      typeof row.gateway_name !== "string" ||
      !Number.isSafeInteger(row.gateway_status) ||
      !Number.isSafeInteger(row.gateway_type) ||
      typeof row.source_channel_id !== "string" ||
      new TextEncoder().encode(row.source_channel_id).length > 128 ||
      row.source_channel_id.trim() !== row.source_channel_id ||
      /[\u0000-\u001f\u007f-\u009f]/.test(row.source_channel_id) ||
      typeof row.profile !== "string" ||
      [
        row.gateway_present,
        row.source_present,
        row.source_enabled,
        row.configured_active,
      ].some((flag) => typeof flag !== "boolean") ||
      (row.source_enabled && !row.source_present) ||
      (row.source_present && row.source_channel_id === "") ||
      !integer(row.stored_pair_count) ||
      !integer(row.route_eligible_pair_count) ||
      row.route_eligible_pair_count > row.stored_pair_count ||
      (row.route_eligible_pair_count > 0 &&
        (!row.gateway_present ||
          !row.configured_active ||
          row.gateway_status !== 1 ||
          row.gateway_type !== 1 ||
          row.profile !== "openai_text_exact_v1"))
    )
      return invalid();
    previous = BigInt(row.gateway_channel_id);
    seen.add(row.gateway_channel_id);
  }
  return value as unknown as ResourceBindingState;
}
