import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ i18n: { language: "zh" }, t: (key: string) => key }),
}));
vi.mock("../src/components/ui/overlay", () => ({
  Drawer: ({ children }: any) => <section>{children}</section>,
}));
vi.mock("../src/lib/api/key-adapter", () => ({
  useBackendKeys: () => ({ keys: [{ id: "7", name: "Renamed key" }] }),
}));
vi.mock("../src/lib/api/billing", () => ({
  useLedger: () => ({ data: [] }),
  adaptLedgerEntry: (value: unknown) => value,
}));
vi.mock("../src/lib/system-access", () => ({
  useSystemAccess: (read: any) => read({ role: 100 }),
  isAdmin: () => true,
}));
vi.mock("../src/lib/product-preview", () => ({
  useProductPreview: (read: any) => read({ demoSetUpstreamOutage: () => {} }),
}));
import RequestInspector from "../src/features/request-inspector";
import type { PreviewRequest } from "../src/lib/product-preview";
describe("customer history metadata", () => {
  it("shows switching and frozen billing but no content or internal instances", () => {
    const row = {
      trace_id: "a".repeat(64),
      source: "api",
      status: "success",
      model: "model-a",
      requested_model: "myapis-auto",
      actual_model: "model-a",
      vendor: "Captured Vendor",
      fallback: true,
      time: "2026-11-03T00:00:00Z",
      cost_nano: "1000000",
      key_id: "7",
      key_name: "Captured key",
      service_group: "captured-group",
      call_type: "chat",
      input_tokens: 100,
      output_tokens: 50,
      cache_read_tokens: 20,
      cache_write_tokens: 3,
      duration_ms: 300,
      ttft_ms: 40,
      error_code: "",
      resource_id: "PRIVATE_RESOURCE",
      channel_id: "PRIVATE_CHANNEL",
      response_text: "PRIVATE_CONTENT",
      billing_detail: {
        version: 1,
        mode: "request",
        priced_at: "2026-11-03T00:00:00Z",
        group_multiplier: "1",
        parts: [
          {
            name: "requests",
            quantity: "1",
            unit: "request",
            unit_price_usd: "0.001",
          },
        ],
      },
    } as PreviewRequest;
    const text = renderToStaticMarkup(
      <MemoryRouter>
        <RequestInspector record={row} onClose={() => {}} onRetry={() => {}} />
      </MemoryRouter>,
    );
    for (const value of [
      "已自动切换",
      "Captured Vendor",
      "Captured key",
      "captured-group",
      "$0.001000000",
      "0.001",
      "调用时计费明细",
    ])
      expect(text).toContain(value);
    for (const value of [
      "PRIVATE_RESOURCE",
      "PRIVATE_CHANNEL",
      "PRIVATE_CONTENT",
      "Renamed key",
    ])
      expect(text).not.toContain(value);
  });
});
