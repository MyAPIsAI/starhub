import { execFileSync } from "node:child_process";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { BILLING_STORAGE_KEY, parseBillingSnapshot } from "../src/lib/billing";
import {
  keyPeriodUsed,
  seedRequestCount,
  useProductPreview,
} from "../src/lib/product-preview";
import {
  buildCurlRequest,
  defaultDemoRequest,
  demoInputTokens,
  serializeRequest,
  type DemoRequest,
} from "../src/lib/request-contract";
import {
  estimateModelCharge,
  getPricingSnapshot,
} from "../src/lib/model-pricing";

const isUnix = process.platform !== "win32";
const itUnix = isUnix ? it : it.skip;

let storage: Map<string, string>;
beforeEach(() => {
  storage = new Map();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
  });
  useProductPreview.getState().resetScenario("enterprise");
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
const store = () => useProductPreview.getState();
const request = (overrides: Partial<DemoRequest> = {}): DemoRequest => ({
  ...defaultDemoRequest(),
  ...overrides,
});
const run = (data: DemoRequest) =>
  store().recordCall(store().selectedKey, false, data.model, data)!;
function reconciled() {
  let balance = 0n;
  for (const row of [...store().ledger].reverse()) {
    balance += BigInt(row.amount_nano);
    expect(row.balance_after_nano).toBe(balance.toString());
  }
  expect(store().balance).toBe(balance.toString());
  expect(BigInt(store().opening)).toBe(
    balance +
      store().requests.reduce((sum, call) => sum + BigInt(call.cost_nano), 0n),
  );
}
describe("canonical request, recovery, persisted scenarios", () => {
  itUnix("D: exports the same escaped model/messages/parameters used by the simulator", () => {
    const data = request({
      messages: [
        { role: "system", content: 'You are "careful".' },
        {
          role: "user",
          content: "翻译：It's a test\n第二行 \\\n $HOME `literal`",
        },
      ],
      temperature: 0.25,
      max_tokens: 200,
    });
    const payload = JSON.parse(serializeRequest(data));
    const row = run(data);
    expect(payload).toEqual({
      model: data.model,
      messages: data.messages,
      temperature: 0.25,
      max_tokens: 200,
    });
    expect(row.request).toEqual(data);
    expect(row.response_text).toContain("翻译任务");
    expect(row.response_text).toContain(data.messages[1].content);
    expect(row.input_tokens).toBe(demoInputTokens(data));
    expect(row.output_tokens).toBeLessThanOrEqual(200);
    const code = buildCurlRequest({
      ...data,
      chainId: "tool-quoted",
      capability: "code",
    });
    const argv = execFileSync(
      "/bin/sh",
      ["-c", `curl() { printf '%s\\0' "$@"; }; ${code}`],
      { env: { MYAPIS_API_KEY: "test-demo-credential" } },
    )
      .toString()
      .split("\0");
    expect(JSON.parse(argv[argv.indexOf("--data-raw") + 1])).toEqual(payload);
    expect(argv).toContain("X-MyAPIs-Capability: code");
    expect(argv).toContain("X-MyAPIs-Chain-Id: tool-quoted");
    expect(argv).toContain("Authorization: Bearer test-demo-credential");
    expect(code).toContain("'\"'\"'");
    expect(code).toContain("\\n");
    expect(code).toContain('"Authorization: Bearer $MYAPIS_API_KEY"');
    expect(buildCurlRequest(data, undefined, "sk-••••mask")).not.toContain(
      "••••",
    );
    expect(
      store().recordCall(store().selectedKey, false, "deepseek-chat", data),
    ).toMatchObject({ error_code: "invalid_request", cost_nano: "0" });
  });
  it("C: estimates maximum spend before any direct or smart request and allows recovery without restarting", () => {
    const data = request({ max_tokens: 100 });
    const estimated = estimateModelCharge({
      modelId: data.model,
      tierId: "pro",
      inputTokens: demoInputTokens(data),
      outputTokens: data.max_tokens,
    }).amountNano;
    store().updateKey(store().selectedKey, {
      quotaLimit_usd: Number(BigInt(estimated) - 1n) / 1e9,
    });
    const before = store().balance;
    expect(run(data)).toMatchObject({
      error_code: "key_quota_exceeded",
      billing_status: "not_charged",
      request: data,
    });
    expect(store().balance).toBe(before);
    store().updateKey(store().selectedKey, { quotaLimit_usd: 0 });
    expect(run(request({ model: "veyra-auto" }))).toMatchObject({
      error_code: "key_quota_exceeded",
    });
    store().updateKey(store().selectedKey, { quotaLimit_usd: 1 });
    expect(run(data)).toMatchObject({ status: "success" });
    reconciled();
  });
  it("resets daily and monthly usage at Dubai calendar boundaries while lifetime usage remains", () => {
    const key = store().keys[0];
    const fields = {
      ...key,
      totalUsed_nano: "900000000",
      currentPeriodUsed_nano: "900000000",
      usagePeriodStartedAt: "2026-09-30T19:59:59Z",
    };
    expect(
      keyPeriodUsed(
        { ...fields, quotaPeriod: "daily" },
        "2026-09-30T19:59:59Z",
      ),
    ).toBe(900000000n);
    expect(
      keyPeriodUsed(
        { ...fields, quotaPeriod: "daily" },
        "2026-09-30T20:00:00Z",
      ),
    ).toBe(0n);
    expect(
      keyPeriodUsed(
        { ...fields, quotaPeriod: "monthly" },
        "2026-09-30T20:00:00Z",
      ),
    ).toBe(0n);
    expect(
      keyPeriodUsed(
        { ...fields, quotaPeriod: "never" },
        "2026-09-30T20:00:00Z",
      ),
    ).toBe(900000000n);
  });
  it("changing quota cycles rebuilds the correct bucket and never erases lifetime spend", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-29T12:00:00Z"));
    store().updateKey(store().selectedKey, { quotaPeriod: "daily" });
    const first = run(request());
    vi.setSystemTime(new Date("2026-09-30T12:00:00Z"));
    const second = run(request());
    expect(store().keys[0].currentPeriodUsed_nano).toBe(second.cost_nano);
    store().updateKey(store().selectedKey, { quotaPeriod: "monthly" });
    const total = BigInt(first.cost_nano) + BigInt(second.cost_nano);
    expect(store().keys[0].currentPeriodUsed_nano).toBe(total.toString());
    store().updateKey(store().selectedKey, { quotaPeriod: "never" });
    expect(keyPeriodUsed(store().keys[0])).toBe(total);
  });
  itUnix("exports system and generation settings in each selected protocol's payload", () => {
    for (const protocol of ["Anthropic", "Gemini"] as const) {
      const data = request({
        protocol,
        model:
          protocol === "Anthropic"
            ? "claude-sonnet-4-20250514"
            : "gemini-2.5-pro",
        messages: [
          { role: "system", content: "系统指令" },
          { role: "user", content: 'a "quote"\nnext' },
        ],
        temperature: 0.25,
        max_tokens: 123,
      });
      const argv = execFileSync("/bin/sh", [
        "-c",
        `curl() { printf '%s\\0' "$@"; }; ${buildCurlRequest(data)}`,
      ])
        .toString()
        .split("\0");
      const body = JSON.parse(argv[argv.indexOf("--data-raw") + 1]);
      if (protocol === "Anthropic")
        expect(body).toMatchObject({
          model: data.model,
          system: "系统指令",
          messages: [data.messages[1]],
          temperature: 0.25,
          max_tokens: 123,
        });
      else
        expect(body).toMatchObject({
          contents: [
            { role: "user", parts: [{ text: data.messages[1].content }] },
          ],
          systemInstruction: { parts: [{ text: "系统指令" }] },
          generationConfig: { temperature: 0.25, maxOutputTokens: 123 },
        });
    }
  });
  it("filters routing by protocol and capability; pins a tool chain instead of silently substituting", () => {
    const first = run(
      request({
        model: "veyra-auto",
        protocol: "Anthropic",
        capability: "vision",
        chainId: "tool-1",
      }),
    );
    expect(first).toMatchObject({
      status: "success",
      actual_model: "claude-sonnet-4-20250514",
    });
    const incompatible = run(
      request({ model: "veyra-auto", protocol: "Gemini", chainId: "tool-1" }),
    );
    expect(incompatible).toMatchObject({
      error_code: "tool_chain_incompatible",
      cost_nano: "0",
    });
    expect(run(request({ protocol: "Anthropic" }))).toMatchObject({
      error_code: "protocol_incompatible",
      cost_nano: "0",
    });
    expect(
      run(request({ model: "deepseek-chat", capability: "vision" })),
    ).toMatchObject({ error_code: "capability_incompatible", cost_nano: "0" });
    expect(
      run(
        request({ model: "veyra-auto", protocol: "Gemini", chainId: "tool-2" }),
      ),
    ).toMatchObject({ status: "success", actual_model: "gemini-2.5-pro" });
  });
  it("F: separates balance, authorization, upstream and disabled-key errors; each failure is free and recoverable", () => {
    store().demoSetBalanceState("zero");
    expect(run(request())).toMatchObject({
      error_code: "insufficient_balance",
      billing_status: "not_charged",
    });
    store().rechargeAccount(10);
    store().demoSetUpstreamOutage(true);
    expect(run(request())).toMatchObject({
      error_code: "upstream_unavailable",
      cost_nano: "0",
    });
    store().demoSetUpstreamOutage(false);
    store().toggleKey(store().selectedKey);
    expect(run(request())).toMatchObject({
      error_code: "key_disabled",
      cost_nano: "0",
    });
    store().toggleKey(store().selectedKey);
    expect(run(request())).toMatchObject({ status: "success" });
    reconciled();
  });
  it("retains existing IPv4/CIDR restrictions with an explicit simulated request source", () => {
    store().updateKey(store().selectedKey, { ipWhitelist: ["10.0.0.0/8"] });
    expect(run(request())).toMatchObject({
      error_code: "ip_unauthorized",
      cost_nano: "0",
    });
    store().updateKey(store().selectedKey, { ipWhitelist: ["127.0.0.0/24"] });
    expect(run(request())).toMatchObject({ status: "success" });
    expect(() =>
      store().updateKey(store().selectedKey, {
        ipWhitelist: ["127.0.0.999/99"],
      }),
    ).toThrow();
  });
  it("G/H: all scenarios reset consistently, refresh restores keys/quotas/chain and no prompt or credential is persisted", () => {
    const key = store().keys[0];
    const originalCredential = key.keySecret;
    const data = request({
      messages: [{ role: "user", content: "PRIVATE_PROMPT_KEEP_IN_MEMORY" }],
      chainId: "chain-preserved",
    });
    run(data);
    const used = store().keys[0].currentPeriodUsed_nano;
    const raw = storage.get(BILLING_STORAGE_KEY)!;
    expect(parseBillingSnapshot(raw)).not.toBeNull();
    expect(raw).not.toContain("PRIVATE_PROMPT_KEEP_IN_MEMORY");
    expect(raw).not.toContain("keySecret");
    expect(raw).not.toContain(originalCredential);
    store().resetScenario("first");
    storage.set(BILLING_STORAGE_KEY, raw);
    expect(store().syncBillingStorage()).toBe(true);
    expect(store().keys[0]).toMatchObject({
      keySecret: originalCredential,
      currentPeriodUsed_nano: used,
    });
    expect(store().toolChains[`${key.id}:chain-preserved`]).toBe("gpt-4.1");
    reconciled();
    for (const scenario of ["first", "enterprise", "pricing"] as const) {
      store().resetScenario(scenario);
      expect(store().scenario).toBe(scenario);
      expect(store().rechargeOrders).toHaveLength(0);
      expect(store().keys).toHaveLength(
        scenario === "first" ? 0 : scenario === "enterprise" ? 2 : 1,
      );
      // first 为新用户首屏，无调用记录；enterprise / pricing 恢复种子数据，
      // pricing 场景在种子数据之外再补一次调价验证调用。
      expect(store().requests).toHaveLength(
        scenario === "first"
          ? 0
          : seedRequestCount + (scenario === "pricing" ? 1 : 0),
      );
      expect(getPricingSnapshot().revision).toBe("sample-1");
      reconciled();
    }
  });
});
