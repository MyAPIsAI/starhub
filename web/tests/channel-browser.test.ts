import { describe, expect, it } from "vitest";
import { createDefaultPricing } from "../src/lib/model-pricing";
import {
  browseChannelResources,
  getResourceState,
} from "../src/lib/channel-browser";

function fixture() {
  const config = createDefaultPricing();
  config.resources!.channels.push({
    id: "empty-channel",
    name: "空渠道",
    provider: "待提供资源",
    enabled: true,
  });
  const template = config.resources!.instances[0];
  config.resources!.instances.push({
    ...structuredClone(template),
    id: "incoming-resource",
    channelId: "channel-2",
    upstreamModel: "vendor-new-model",
    canonicalModel: "new-canonical-model",
  });
  config.placements!["incoming-resource"] = {
    modelId: null,
    groupIds: [],
    enabled: false,
  };
  config.catalog!["empty-model"] = {
    ...structuredClone(config.catalog!["gpt-4.1"]),
    id: "empty-model",
    name: "尚无资源的标准模型",
  };
  return config;
}

describe("resource browsing in channel and standard-model views", () => {
  it("splits channels without dropping empty channels or changing the source", () => {
    const config = fixture();
    const original = structuredClone(config);
    const result = browseChannelResources(config, { view: "channel" });
    expect(result.groups.map((group) => group.id)).toEqual([
      "__all__",
      "channel-1",
      "channel-2",
      "empty-channel",
    ]);
    expect(result.selectedGroup?.id).toBe("__all__");
    expect(result.total).toBe(10);
    expect(new Set(result.rows.map((row) => row.channelId)).size).toBe(2);
    expect(
      browseChannelResources(config, {
        view: "channel",
        selectedId: "channel-1",
      }).rows.every((row) => row.channelId === "channel-1"),
    ).toBe(true);
    expect(config).toEqual(original);
    expect(
      browseChannelResources(config, {
        view: "channel",
        selectedId: "empty-channel",
      }).rows,
    ).toEqual([]);
  });

  it("aggregates the same standard model across channels once per instance", () => {
    const config = fixture();
    const result = browseChannelResources(config, {
      view: "model",
      selectedId: "gpt-4.1",
    });
    expect(result.rows.map((row) => row.id)).toEqual([
      "resource-1-a",
      "resource-1-b",
    ]);
    expect(new Set(result.rows.map((row) => row.channelId)).size).toBe(2);
    expect(new Set(result.rows.map((row) => row.id)).size).toBe(result.total);
    expect(
      result.groups.find((group) => group.id === "empty-model")?.total,
    ).toBe(0);
  });

  it("groups unbound, missing and stale mappings together without trusting canonical hints", () => {
    const config = fixture();
    delete config.placements!["resource-1-a"];
    config.placements!["resource-1-b"].modelId = "deleted-model";
    const result = browseChannelResources(config, {
      view: "model",
      selectedId: "__unmapped__",
    });
    expect(result.rows.map((row) => row.id)).toEqual([
      "resource-1-a",
      "resource-1-b",
      "incoming-resource",
    ]);
    expect(result.selectedGroup?.unmapped).toBe(3);
  });

  it("shows an empty unmapped group so a user can keep that selection after binding the last resource", () => {
    const config = createDefaultPricing();
    const result = browseChannelResources(config, {
      view: "model",
      selectedId: "__unmapped__",
    });
    expect(result.selectedGroup?.name).toBe("待映射资源");
    expect(result.rows).toEqual([]);
  });

  it.each([
    ["incoming-resource", "incoming-resource"],
    ["vendor-new-model", "incoming-resource"],
    ["NEW-CANONICAL-MODEL", "incoming-resource"],
    ["GPT-4.1", "resource-1-a"],
    ["OpenAI", "resource-1-a"],
    ["渠道供应商 2", "resource-1-b"],
  ])(
    "searches resource identity and mapped/source context: %s",
    (query, id) => {
      const result = browseChannelResources(fixture(), {
        view: "model",
        query,
      });
      expect(result.rows.some((row) => row.id === id)).toBe(true);
    },
  );

  it("combines group, status and text filters while preserving left-side counts", () => {
    const config = fixture();
    config.placements!["resource-1-a"].enabled = false;
    config.placements!["resource-1-b"].groupIds = ["lite"];
    const baseline = browseChannelResources(config, { view: "channel" });
    const result = browseChannelResources(config, {
      view: "channel",
      selectedId: "channel-1",
      query: "gpt-4.1",
      groupId: "pro",
      status: "paused",
    });
    expect(result.rows.map((row) => row.id)).toEqual(["resource-1-a"]);
    expect(result.total).toBe(7);
    expect(result.filteredTotal).toBe(1);
    expect(result.groups).toEqual(baseline.groups);
    expect(
      browseChannelResources(config, {
        view: "model",
        selectedId: "gpt-4.1",
        groupId: "lite",
        status: "ready",
      }).rows.map((row) => row.id),
    ).toEqual(["resource-1-b"]);
  });

  it("keeps valid explicit selections and repairs missing selections to all resources, independently of filters", () => {
    const config = fixture();
    for (const selectedId of [undefined, "removed-channel"]) {
      expect(
        browseChannelResources(config, {
          view: "channel",
          selectedId,
          query: "vendor-new-model",
        }).selectedGroup?.id,
      ).toBe("__all__");
    }
    const retained = browseChannelResources(config, {
      view: "channel",
      selectedId: "channel-1",
      query: "vendor-new-model",
    });
    expect(retained.selectedGroup?.id).toBe("channel-1");
    expect(retained.rows).toEqual([]);
    expect(
      browseChannelResources(config, {
        view: "channel",
        query: "no-such-resource",
      }).selectedGroup?.id,
    ).toBe("__all__");
  });

  it("handles a missing or empty snapshot without inventing resources", () => {
    const config = fixture();
    delete config.resources;
    expect(browseChannelResources(config, { view: "channel" })).toEqual({
      groups: [
        {
          id: "__all__",
          name: "全部渠道",
          subtitle: "全部资源实例",
          total: 0,
          ready: 0,
          unmapped: 0,
        },
      ],
      selectedGroup: {
        id: "__all__",
        name: "全部渠道",
        subtitle: "全部资源实例",
        total: 0,
        ready: 0,
        unmapped: 0,
      },
      rows: [],
      total: 0,
      filteredTotal: 0,
    });
    expect(
      browseChannelResources(config, { view: "model" }).groups.some(
        (group) => group.id === "gpt-4.1" && group.total === 0,
      ),
    ).toBe(true);
  });
});

describe("resource state priority and operational boundaries", () => {
  it("prioritizes unavailable source over an absent mapping or paused placement", () => {
    const config = fixture();
    const resource = config.resources!.instances.at(-1)!;
    resource.status = "unavailable";
    expect(getResourceState(config, resource).id).toBe("unavailable");
    resource.status = "retired";
    expect(getResourceState(config, resource)).toEqual({
      id: "unavailable",
      label: "来源已下架",
    });
    resource.status = "available";
    config.resources!.channels.find(
      (channel) => channel.id === "channel-2",
    )!.enabled = false;
    expect(getResourceState(config, resource).id).toBe("unavailable");
    resource.channelId = "removed-channel";
    expect(getResourceState(config, resource).id).toBe("unavailable");
  });

  it("prioritizes missing model mappings over local placement state", () => {
    const config = fixture();
    const resource = config.resources!.instances[0];
    for (const modelId of [null, "deleted-model"]) {
      config.placements![resource.id] = {
        modelId,
        groupIds: [],
        enabled: false,
      };
      expect(getResourceState(config, resource).id).toBe("unmapped");
    }
  });

  it("requires an enabled placement, online model and at least one enabled existing group", () => {
    const config = fixture();
    const resource = config.resources!.instances[0];
    const placement = config.placements![resource.id];
    expect(getResourceState(config, resource).id).toBe("ready");
    placement.enabled = false;
    expect(getResourceState(config, resource).id).toBe("paused");
    placement.enabled = true;
    placement.groupIds = [];
    expect(getResourceState(config, resource).id).toBe("paused");
    placement.groupIds = ["removed-group"];
    expect(getResourceState(config, resource).id).toBe("paused");
    placement.groupIds.push("pro");
    config.groups.pro.enabled = false;
    expect(getResourceState(config, resource).id).toBe("paused");
    placement.groupIds.push("standard");
    expect(getResourceState(config, resource).id).toBe("ready");
    config.catalog!["gpt-4.1"].online = false;
    expect(getResourceState(config, resource).id).toBe("paused");
  });
});
