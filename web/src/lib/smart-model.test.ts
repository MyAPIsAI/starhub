import { describe, expect, it } from "vitest";
import { canonicalModelId } from "./smart-model";
import {
  getDirectoryModel,
  getDirectoryModels,
} from "../config/model-directory";
import { createDefaultPricing } from "./model-pricing";
import { modelCatalog, upgradePlatform } from "./platform-catalog";

describe("smart directory and cache compatibility", () => {
  it("maps both directory entries to one canonical platform model", () => {
    const canonical = getDirectoryModel("myapis-auto")!;
    expect(getDirectoryModel("veyra-auto")).toEqual(canonical);
    expect(canonical).toMatchObject({
      id: "myapis-auto",
      name: "MyAPIs Auto",
      vendor: "MyAPIs",
      kind: "smart",
    });
    expect(
      getDirectoryModels().filter((model) => model.kind === "smart"),
    ).toHaveLength(1);
    for (const id of [
      "",
      "veyra-auto-preview",
      "myapis-auto-preview",
      "gpt-4.1",
    ])
      expect(canonicalModelId(id)).toBe(id);
  });
  it("migrates legacy configuration idempotently without expanding physical access", () => {
    const config = createDefaultPricing();
    config.catalog!["veyra-auto"] = {
      ...structuredClone(config.catalog!["myapis-auto"]),
      id: "veyra-auto",
    };
    config.models["veyra-auto"] = structuredClone(config.models["gpt-4.1"]);
    const resource = {
      ...config.resources!.instances[0],
      id: "legacy-route",
      canonicalModel: "veyra-auto",
      upstreamModel: "veyra-auto",
    };
    config.resources!.instances.push(resource);
    config.placements![resource.id] = {
      modelId: "veyra-auto",
      enabled: true,
      groupIds: ["pro"],
    };
    const original = structuredClone(config);
    const next = upgradePlatform(config);
    expect(next.catalog!["veyra-auto"]).toBeUndefined();
    expect(next.catalog!["myapis-auto"].id).toBe("myapis-auto");
    expect(next.models["veyra-auto"]).toBeUndefined();
    expect(next.resources!.instances).not.toContainEqual(resource);
    expect(next.placements![resource.id]).toBeUndefined();
    expect(modelCatalog(next).map((model) => model.id)).toEqual(
      modelCatalog(config).map((model) => model.id),
    );
    expect(upgradePlatform(next)).toEqual(next);
    expect(config).toEqual(original);
  });
});
