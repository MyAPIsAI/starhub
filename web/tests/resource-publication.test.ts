import { describe, expect, it } from "vitest";
import {
  publicationRequest,
  readPublicationContext,
  readPublicationPreview,
  type PublicationContext,
} from "../src/lib/resource-publication";

const context: PublicationContext = {
  schema: "exact_model_pairs_preview_v1",
  profile: "openai_text_exact_v1",
  supported_endpoints: ["/v1/chat/completions", "/v1/messages"],
  resource_revision: 3,
  channel_revision: 8,
  option_version: 12,
  preview_only: true,
  affects_gateway_routing: false,
  channels: [{ id: "9007199254740993", name: "Gateway", type: 1, status: 1 }],
};
const binding = {
  source_channel_id: "9007199254740993",
  gateway_channel_id: "9007199254740993",
};
const pair = {
  gateway_channel_id: binding.gateway_channel_id,
  model: "alpha",
  group: "default",
};
const preview = () => ({
  ...structuredClone(context),
  ready: true,
  bindings: [{ ...binding }],
  proposed_pairs: [{ ...pair }],
  existing_pairs: [{ ...pair, enabled: false }],
  issues: [],
});

describe("actual resource publication preview", () => {
  it("retains exact int64 strings and requires explicit unique bindings", () => {
    expect(readPublicationContext(context).channels[0].id).toBe(
      "9007199254740993",
    );
    const request = publicationRequest(context, [binding]);
    expect(request).toEqual({
      expected_resource_revision: 3,
      expected_channel_revision: 8,
      expected_option_version: 12,
      bindings: [binding],
    });
    expect(request.bindings[0]).not.toBe(binding);
    for (const bindings of [
      [],
      [binding, binding],
      [binding, { ...binding, source_channel_id: "other" }],
      [{ ...binding, gateway_channel_id: "09007199254740993" }],
      [{ ...binding, gateway_channel_id: "9223372036854775808" }],
      [{ ...binding, gateway_channel_id: "2" }],
    ])
      expect(() => publicationRequest(context, bindings)).toThrow();
  });
  it("rejects unsupported active publication capabilities and unsafe versions", () => {
    for (const overrides of [
      { preview_only: false },
      { affects_gateway_routing: true },
      { schema: "publish_v2" },
      { profile: "unknown" },
      { supported_endpoints: ["/v1/responses"] },
      { resource_revision: -1 },
      { channel_revision: Number.MAX_SAFE_INTEGER + 1 },
      { option_version: 0.5 },
      { channels: [{ ...context.channels[0], id: 9007199254740993 }] },
      { channels: [context.channels[0], context.channels[0]] },
    ])
      expect(() =>
        readPublicationContext({ ...context, ...overrides }),
      ).toThrow();
    expect(readPublicationContext({ ...context, channels: [] })).toMatchObject({
      preview_only: true,
      affects_gateway_routing: false,
    });
  });
  it("validates exact pairs, enabled flags and consistent readiness", () => {
    expect(readPublicationPreview(preview()).existing_pairs[0].enabled).toBe(
      false,
    );
    for (const overrides of [
      { ready: false },
      { issues: [{ code: "no_supported_pairs" }] },
      { proposed_pairs: [] },
      { proposed_pairs: [pair, pair] },
      { proposed_pairs: [{ ...pair, gateway_channel_id: "2" }] },
      { existing_pairs: [pair] },
      { bindings: [] },
      { proposed_pairs: [{ ...pair, group: "\n" }] },
    ])
      expect(() =>
        readPublicationPreview({ ...preview(), ...overrides }),
      ).toThrow();
    const blocked = {
      ...preview(),
      ready: false,
      issues: [{ code: "unbound_source_channel", source_channel_id: "other" }],
    };
    expect(readPublicationPreview(blocked).ready).toBe(false);
  });
});
