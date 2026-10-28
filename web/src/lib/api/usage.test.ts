import { describe, expect, it } from "vitest";
import { adaptLogRow, type LogRow } from "./usage";
import { filterRequestHistory } from "../request-history";
import type { PreviewRequest } from "../product-preview";

const log: LogRow = {
  id: 42,
  user_id: 7,
  token_id: 3,
  token_name: "review",
  channel_id: 9,
  model: "gpt-4.1",
  requested_model: "myapis-auto",
  prompt_tokens: 12,
  completion_tokens: 4,
  quota: 123,
  unfunded_quota: 0,
  used_channels: "9",
  duration_ms: 250,
  status: 1,
  created_at: 1793145600,
};

describe("backend log model attribution", () => {
  it("keeps the original routing entry while pricing and vendor use the actual model", () => {
    const row = adaptLogRow(log);
    expect(row).toMatchObject({
      model: "gpt-4.1",
      requested_model: "myapis-auto",
      actual_model: "gpt-4.1",
      vendor: "OpenAI",
      cost_nano: "246000",
      status: "success",
    });
    expect(log.requested_model).toBe("myapis-auto");
  });

  it.each([undefined, "", "gpt-4.1"])(
    "preserves the explicit-model fallback for a requested_model of %s",
    (requested_model) => {
      expect(adaptLogRow({ ...log, requested_model })).toMatchObject({
        requested_model: "gpt-4.1",
        actual_model: "gpt-4.1",
        model: "gpt-4.1",
      });
    },
  );

  it("allows the existing history search to find a backend row by either model", () => {
    const row = adaptLogRow(log) as PreviewRequest;
    for (const query of ["MYAPIS-AUTO", "gpt-4.1"]) {
      expect(
        filterRequestHistory([row], {
          range: "all",
          start: "",
          end: "",
          query,
          status: "all",
        }).rows,
      ).toEqual([row]);
    }
  });
});
