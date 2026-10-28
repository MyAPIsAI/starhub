import { describe, it, expect } from "vitest";
import {
  parseResourceCatalog,
  parseResourceSnapshot,
  resourceImportDiff,
} from "../src/lib/resource-catalog";

const snapshot = () => ({
  id: "inventory",
  sequence: 1,
  publishedAt: "2026-10-28T00:00:00Z",
  receivedAt: "2026-10-28T00:00:01Z",
  source: "fixture",
  channels: [{ id: "supplier", name: "Supplier", provider: "", enabled: true }],
  instances: [
    {
      id: "resource",
      channelId: "supplier",
      upstreamModel: "upstream-text",
      canonicalModel: "standard-text",
      protocols: ["OpenAI"],
      capabilities: ["text"],
      contextLength: 128000,
      region: "",
      status: "available",
      latencyMs: 0,
    },
  ],
});
const catalog = () => ({
  revision: 1,
  snapshot: snapshot(),
  placements: { resource: { modelId: null, groupIds: [], enabled: false } },
  updated_at: 1793145601,
  affects_gateway_routing: false,
});

describe("server resource inventory boundaries", () => {
  it("preserves empty server state without seeding demo resources", () => {
    expect(
      parseResourceCatalog({
        revision: 0,
        snapshot: null,
        placements: {},
        updated_at: 0,
        affects_gateway_routing: false,
      }).snapshot,
    ).toBeNull();
  });
  it("preserves unknown regions and zero source latency without inventing observations", () => {
    const result = parseResourceCatalog(catalog());
    expect(result.snapshot?.instances[0]).toMatchObject({
      region: "",
      latencyMs: 0,
    });
    expect(result.placements.resource).toEqual({
      modelId: null,
      groupIds: [],
      enabled: false,
    });
  });
  it.each([
    null,
    [],
    {},
    { ...catalog(), revision: Number.MAX_SAFE_INTEGER + 1 },
    { ...catalog(), affects_gateway_routing: true },
    { ...catalog(), placements: {} },
    {
      ...catalog(),
      placements: { unknown: { modelId: null, groupIds: [], enabled: false } },
    },
    {
      ...catalog(),
      placements: { resource: { modelId: null, groupIds: [], enabled: true } },
    },
    {
      ...catalog(),
      placements: {
        resource: { modelId: "myapis-auto", groupIds: [], enabled: false },
      },
    },
    {
      ...catalog(),
      placements: {
        resource: {
          modelId: "standard-text",
          groupIds: ["default", "default"],
          enabled: true,
        },
      },
    },
  ])("rejects malformed or ambiguous server catalogs %#", (value) => {
    expect(() => parseResourceCatalog(value)).toThrow();
  });
  it.each([
    (s: ReturnType<typeof snapshot>) => {
      s.instances[0].region = null as unknown as string;
    },
    (s: ReturnType<typeof snapshot>) => {
      s.instances[0].region = " region ";
    },
    (s: ReturnType<typeof snapshot>) => {
      s.instances[0].channelId = "missing";
    },
    (s: ReturnType<typeof snapshot>) => {
      s.instances[0].canonicalModel = "veyra-auto";
    },
    (s: ReturnType<typeof snapshot>) => {
      s.instances[0].protocols = ["unknown"];
    },
    (s: ReturnType<typeof snapshot>) => {
      s.instances[0].contextLength = 1.5;
    },
    (s: ReturnType<typeof snapshot>) => {
      s.instances[0].latencyMs = Infinity;
    },
    (s: ReturnType<typeof snapshot>) => {
      s.instances.push(s.instances[0]);
    },
  ])("rejects malformed inventory metadata before preview %#", (mutate) => {
    const s = snapshot();
    mutate(s);
    expect(() => parseResourceSnapshot(s)).toThrow();
  });
  it("compares stable IDs for import review without counting every object as new", () => {
    const before = snapshot();
    const after = snapshot();
    after.instances[0].upstreamModel = "changed";
    after.instances.push({ ...after.instances[0], id: "added" });
    expect(resourceImportDiff(before, after)).toEqual({
      added: 1,
      changed: 1,
      removed: 0,
    });
    expect(resourceImportDiff(after, before)).toEqual({
      added: 0,
      changed: 1,
      removed: 1,
    });
  });
});
