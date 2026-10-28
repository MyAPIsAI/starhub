import { describe, expect, it } from "vitest";
import {
  emptyModelMetadata,
  readModelMetadata,
  readModelMetadataList,
} from "../src/lib/model-metadata";
describe("actual model metadata", () => {
  it("keeps unknown references distinct from zero and retains unedited metadata", () => {
    const original = {
      ...emptyModelMetadata(),
      model: "server/custom",
      tags: "function_call,unknown-extension",
      max_output: 2048,
      official_cache_read_per_m: 0,
      score: 91,
      compatible_groups: "private",
      sort_weight: 8,
    };
    const parsed = readModelMetadata(original),
      edited = { ...parsed, description: "New description" };
    expect(edited.official_input_per_m).toBeNull();
    expect(edited.official_cache_read_per_m).toBe(0);
    expect(edited.max_output).toBe(2048);
    expect(edited.tags).toBe(original.tags);
    expect(edited.score).toBe(91);
    expect(edited.compatible_groups).toBe("private");
    expect(edited.sort_weight).toBe(8);
    expect(original.description).toBe("");
  });
  it("does not create sample model rows from empty server metadata", () => {
    expect(readModelMetadataList([])).toEqual([]);
    expect(emptyModelMetadata().context_length).toBe(0);
    expect(emptyModelMetadata().official_input_per_m).toBeNull();
  });
  it("rejects malformed, unsafe and duplicate metadata instead of falling back", () => {
    const row = { ...emptyModelMetadata(), model: "actual" };
    for (const value of [
      null,
      {},
      { ...row, model: "" },
      { ...row, tags: null },
      { ...row, max_output: -1 },
      { ...row, context_length: Number.MAX_SAFE_INTEGER + 1 },
      { ...row, official_input_per_m: undefined },
      { ...row, official_output_per_m: NaN },
      { ...row, score: 101 },
    ])
      expect(() => readModelMetadata(value)).toThrow();
    expect(() => readModelMetadataList(null)).toThrow();
    expect(() => readModelMetadataList([row, row])).toThrow();
  });
});
