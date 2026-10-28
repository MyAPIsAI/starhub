import { describe, expect, it } from "vitest";
import {
  acknowledgementMatchesReview,
  publicationCommitRequest,
  readPublicationAcknowledgement,
  readPublicationReceipt,
  receiptMatchesReview,
} from "../src/lib/resource-publication-commit";
import type { PublicationPreview } from "../src/lib/resource-publication";

const review: PublicationPreview = {
  schema: "exact_model_pairs_preview_v1",
  profile: "openai_text_exact_v1",
  supported_endpoints: ["/v1/chat/completions", "/v1/messages"],
  resource_revision: 3,
  channel_revision: 8,
  option_version: 12,
  channels: [{ id: "9007199254740993", name: "Gateway", type: 1, status: 1 }],
  preview_only: true,
  publish_available: true,
  affects_gateway_routing: false,
  ready: true,
  bindings: [
    { source_channel_id: "source", gateway_channel_id: "9007199254740993" },
  ],
  proposed_pairs: [
    {
      gateway_channel_id: "9007199254740993",
      model: "alpha",
      group: "default",
    },
  ],
  proposed_routes: [
    {
      gateway_channel_id: "9007199254740993",
      model: "alpha",
      group: "default",
      capabilities: ["text"],
      context_length: 128000,
      group_definition_id: "0",
    },
  ],
  existing_pairs: [],
  review_digest: "a".repeat(64),
  issues: [],
};
const acknowledgement = {
  publication_id: "9007199254740993",
  review_digest: review.review_digest,
  resource_revision: 4,
  channel_revision: 9,
  option_version: 12,
  profile: "openai_text_exact_v1",
  binding_count: 1,
  route_count: 1,
  published: true,
  operation: "publish",
};
const receipt = () => ({
  ...acknowledgement,
  actor_id: "9007199254740994",
  created_at: 1790000000,
  historical: true,
  committed: true,
  bindings: structuredClone(review.bindings),
  routes: structuredClone(review.proposed_routes),
});

describe("guarded actual publication", () => {
  it("sends only captured guards, explicit bindings and digest after declared capability", () => {
    const request = publicationCommitRequest(review);
    expect(request).toEqual({
      expected_resource_revision: 3,
      expected_channel_revision: 8,
      expected_option_version: 12,
      bindings: review.bindings,
      review_digest: review.review_digest,
    });
    expect(request.bindings[0]).not.toBe(review.bindings[0]);
    for (const change of [
      { ready: false },
      { publish_available: false },
      { publish_available: undefined },
      { resource_revision: Number.MAX_SAFE_INTEGER },
      { channel_revision: Number.MAX_SAFE_INTEGER },
    ])
      expect(() =>
        publicationCommitRequest({ ...review, ...change }),
      ).toThrow();
  });
  it("preserves exact receipt IDs and verifies acknowledgement against the submitted review", () => {
    const value = readPublicationAcknowledgement(acknowledgement);
    expect(value.publication_id).toBe("9007199254740993");
    expect(acknowledgementMatchesReview(value, review)).toBe(true);
    for (const change of [
      { review_digest: "b".repeat(64) },
      { resource_revision: 5 },
      { channel_revision: 10 },
      { option_version: 13 },
      { binding_count: 2 },
      { route_count: 2 },
    ])
      expect(
        acknowledgementMatchesReview(
          readPublicationAcknowledgement({ ...acknowledgement, ...change }),
          review,
        ),
      ).toBe(false);
    for (const change of [
      { published: false },
      { operation: "unknown" },
      { publication_id: 9007199254740993 },
      { resource_revision: Number.MAX_SAFE_INTEGER + 1 },
      { route_count: 0 },
      { profile: "unknown" },
    ])
      expect(() =>
        readPublicationAcknowledgement({ ...acknowledgement, ...change }),
      ).toThrow();
  });
  it("requires committed historical receipts and complete matching route constraints", () => {
    const value = readPublicationReceipt(receipt());
    expect(value.actor_id).toBe("9007199254740994");
    expect(receiptMatchesReview(value, review)).toBe(true);
    const changed = receipt();
    changed.routes[0].context_length = 32000;
    expect(receiptMatchesReview(readPublicationReceipt(changed), review)).toBe(
      false,
    );
    for (const change of [
      { historical: false },
      { committed: false },
      { actor_id: 1 },
      { created_at: 0 },
      { routes: [] },
      { bindings: [] },
      { routes: [{ ...review.proposed_routes[0], group_definition_id: 1 }] },
      { routes: [{ ...review.proposed_routes[0], capabilities: ["vision"] }] },
    ])
      expect(() =>
        readPublicationReceipt({ ...receipt(), ...change }),
      ).toThrow();
  });
});
