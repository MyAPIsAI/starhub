import { describe, expect, it } from "vitest";
import {
  bindingChangeRequest,
  bindingReceiptMatchesReview,
  prepareBindingChange,
  readBindingChangeReceipt,
  type BindingChangeReview,
} from "../src/lib/resource-binding-change";
import type { ResourceBindingState } from "../src/lib/resource-binding-state";

const state = (): ResourceBindingState => ({
  schema: "managed_gateway_bindings_v1",
  resource_revision: 4,
  channel_revision: 9,
  option_version: 12,
  page: 1,
  page_size: 20,
  total: 1,
  read_only: true,
  binding_changes_available: true,
  bindings: [
    {
      gateway_channel_id: "9007199254740993",
      gateway_name: "Gateway",
      gateway_present: true,
      gateway_status: 1,
      gateway_type: 1,
      source_channel_id: "source",
      source_present: true,
      source_enabled: true,
      configured_active: true,
      profile: "openai_text_exact_v1",
      publication_id: "9007199254740994",
      stored_pair_count: 2,
      route_eligible_pair_count: 2,
    },
  ],
});
const review = (operation: BindingChangeReview["operation"] = "release") => {
  const value = state();
  return prepareBindingChange(
    value,
    value.bindings[0],
    operation,
    "a".repeat(32),
  );
};
const receipt = (operation: BindingChangeReview["operation"] = "release") => ({
  schema: "managed_binding_change_v1",
  change_id: "9007199254740995",
  request_id: "a".repeat(32),
  actor_id: "9007199254740996",
  operation,
  gateway_channel_id: "9007199254740993",
  source_channel_id: "source",
  retained_source_channel_id: operation === "release" ? "" : "source",
  publication_id: "9007199254740994",
  profile: "openai_text_exact_v1",
  was_configured_active: true,
  removed_route_count: 2,
  removed_ability_count: 2,
  resource_revision: 5,
  channel_revision: 10,
  created_at: 1793232000,
  historical: true,
  committed: true,
  protective_marker_retained: true,
});

describe("guarded binding reduction commands and receipts", () => {
  it("sends only captured guards and explicit operation with exact IDs", () => {
    const command = bindingChangeRequest(review());
    expect(command).toEqual({
      expected_resource_revision: 4,
      expected_channel_revision: 9,
      expected_source_channel_id: "source",
      expected_publication_id: "9007199254740994",
      operation: "release",
      request_id: "a".repeat(32),
    });
    expect(command).not.toHaveProperty("actor_id");
    expect(command).not.toHaveProperty("removed_route_count");
    expect(command).not.toHaveProperty("profile");
    const value = state(),
      captured = prepareBindingChange(
        value,
        value.bindings[0],
        "revoke",
        "b".repeat(32),
      );
    value.bindings[0].source_channel_id = "changed";
    expect(captured.row.source_channel_id).toBe("source");
    expect(captured.row).not.toBe(value.bindings[0]);
  });
  it("requires declared server capability, safe versions and a bound captured row", () => {
    for (const change of [
      { binding_changes_available: undefined },
      { binding_changes_available: false },
      { resource_revision: Number.MAX_SAFE_INTEGER },
      { channel_revision: Number.MAX_SAFE_INTEGER },
    ]) {
      const value = { ...state(), ...change };
      expect(() =>
        prepareBindingChange(
          value,
          value.bindings[0],
          "revoke",
          "a".repeat(32),
        ),
      ).toThrow();
    }
    const value = state();
    expect(() =>
      prepareBindingChange(
        value,
        { ...value.bindings[0], source_channel_id: "other" },
        "release",
        "a".repeat(32),
      ),
    ).toThrow();
    for (const key of ["bad", "A".repeat(32), "a".repeat(31)])
      expect(() =>
        prepareBindingChange(value, value.bindings[0], "release", key),
      ).toThrow();
    for (const change of [
      { request_id: "bad" },
      { resource_revision: -1 },
      { channel_revision: 2 ** 53 },
      { operation: "delete" },
      { row: { ...value.bindings[0], gateway_channel_id: "01" } },
      { row: { ...value.bindings[0], publication_id: "01" } },
      { row: { ...value.bindings[0], source_channel_id: "" } },
    ])
      expect(() =>
        bindingChangeRequest({ ...review(), ...change } as BindingChangeReview),
      ).toThrow();
  });
  it("matches complete historical receipts to the captured action, not just success flags", () => {
    for (const operation of ["revoke", "release"] as const) {
      const result = readBindingChangeReceipt(receipt(operation));
      expect(bindingReceiptMatchesReview(result, review(operation))).toBe(true);
      expect(result.change_id).toBe("9007199254740995");
    }
    for (const change of [
      { request_id: "b".repeat(32) },
      { source_channel_id: "other", retained_source_channel_id: "" },
      { gateway_channel_id: "2" },
      { publication_id: "0" },
      { profile: "unknown" },
      { was_configured_active: false },
      { removed_route_count: 1 },
      { resource_revision: 6 },
      { channel_revision: 11 },
    ])
      expect(
        bindingReceiptMatchesReview(
          readBindingChangeReceipt({ ...receipt(), ...change }),
          review(),
        ),
      ).toBe(false);
  });
  it("rejects malformed or nonhistorical receipts and inconsistent release semantics", () => {
    for (const change of [
      { schema: "unknown" },
      { change_id: 9007199254740992 },
      { actor_id: "0" },
      { request_id: "BAD" },
      { gateway_channel_id: "9223372036854775808" },
      { publication_id: "01" },
      { operation: ["release"] },
      { operation: "delete" },
      { source_channel_id: "" },
      { retained_source_channel_id: "source" },
      { profile: "x".repeat(65) },
      { was_configured_active: 1 },
      { removed_route_count: -1 },
      { removed_ability_count: 2 ** 53 },
      { resource_revision: 0 },
      { channel_revision: 2 ** 53 },
      { created_at: Number.MAX_SAFE_INTEGER },
      { historical: false },
      { committed: false },
      { protective_marker_retained: false },
    ])
      expect(() =>
        readBindingChangeReceipt({ ...receipt(), ...change }),
      ).toThrow();
    expect(() =>
      readBindingChangeReceipt({
        ...receipt("revoke"),
        retained_source_channel_id: "",
      }),
    ).toThrow();
    expect(() =>
      readBindingChangeReceipt({ ...receipt("revoke"), operation: ["revoke"] }),
    ).toThrow();
  });
});
