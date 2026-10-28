import { describe, expect, it } from "vitest";
import { readResourceBindingState } from "../src/lib/resource-binding-state";

const fixture = () => ({
  schema: "managed_gateway_bindings_v1",
  resource_revision: 4,
  channel_revision: 9,
  option_version: 12,
  page: 1,
  page_size: 20,
  total: 1,
  read_only: true,
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
describe("persistent gateway binding status", () => {
  it("preserves exact identifiers and distinguishes configuration from routing gates", () => {
    const state = readResourceBindingState(fixture(), 1, 20);
    expect(state.bindings[0].gateway_channel_id).toBe("9007199254740993");
    const blocked = fixture();
    blocked.bindings[0].route_eligible_pair_count = 0;
    blocked.bindings[0].profile = "unknown";
    expect(
      readResourceBindingState(blocked, 1, 20).bindings[0].configured_active,
    ).toBe(true);
    const released = fixture();
    Object.assign(released.bindings[0], {
      source_channel_id: "",
      source_present: false,
      source_enabled: false,
      gateway_present: false,
      gateway_status: 0,
      gateway_type: 0,
      gateway_name: "",
      configured_active: false,
      stored_pair_count: 0,
      route_eligible_pair_count: 0,
      publication_id: "0",
    });
    expect(
      readResourceBindingState(released, 1, 20).bindings[0].source_channel_id,
    ).toBe("");
    expect(
      readResourceBindingState({ ...fixture(), page: 2, bindings: [] }, 2, 20)
        .bindings,
    ).toEqual([]);
  });
  it("rejects mismatched pages, unsafe numbers, duplicate and unordered IDs", () => {
    for (const change of [
      { schema: "other" },
      { read_only: false },
      { resource_revision: -1 },
      { channel_revision: 2 ** 53 },
      { option_version: null },
      { page: 2 },
      { page_size: 100 },
      { total: 0 },
      { bindings: [] },
    ])
      expect(() =>
        readResourceBindingState({ ...fixture(), ...change }, 1, 20),
      ).toThrow();
    const state = fixture();
    state.bindings.push({ ...state.bindings[0] });
    state.total = 2;
    expect(() => readResourceBindingState(state, 1, 20)).toThrow();
    state.bindings[1].gateway_channel_id = "9007199254740992";
    expect(() => readResourceBindingState(state, 1, 20)).toThrow();
    state.bindings[1].gateway_channel_id = "9007199254740994";
    expect(readResourceBindingState(state, 1, 20).bindings).toHaveLength(2);
  });
  it("rejects inconsistent eligibility and malformed ownership facts", () => {
    for (const change of [
      { gateway_channel_id: 9007199254740992 },
      { gateway_channel_id: "01" },
      { gateway_channel_id: "9223372036854775808" },
      { publication_id: 1 },
      { publication_id: "-1" },
      { source_channel_id: " source " },
      { source_channel_id: "a\n" },
      { source_channel_id: "x".repeat(129) },
      { source_present: false, source_enabled: true },
      { source_channel_id: "", source_present: true },
      { stored_pair_count: 1 },
      { route_eligible_pair_count: 2 ** 53 },
      { configured_active: false },
      { profile: "unknown" },
      { gateway_present: false },
      { gateway_status: 2 },
      { gateway_type: 99 },
    ]) {
      const state = fixture();
      Object.assign(state.bindings[0], change);
      expect(() => readResourceBindingState(state, 1, 20)).toThrow();
    }
  });
});
