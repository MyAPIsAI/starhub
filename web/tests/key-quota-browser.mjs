// Exercise real key APIs and console interactions against an isolated SQLite
// server. Locale/theme preferences are the only client-side fixture settings.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { authenticatorCode } from "./browser-totp.mjs";
import AxeBuilder from "@axe-core/playwright";
import { createServer } from "node:http";
import { execFileSync, spawn } from "node:child_process";
import { mkdtemp, mkdir, writeFile, rm, readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const root = fileURLToPath(new URL("../", import.meta.url)),
  repo = path.resolve(root, "..");
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-key-quota-"));
const out = path.join(root, "playwright-report", "key-quota");
await mkdir(out, { recursive: true });
const password = "Browser-test-only-quota-942!";
let backend, vite, browser, upstream;
const report = {
  checks: [],
  screenshots: [],
  pageErrors: [],
  failedRequests: [],
};
async function freePort() {
  return new Promise((resolve, reject) => {
    const s = net.createServer();
    s.once("error", reject);
    s.listen(0, "127.0.0.1", () => {
      const p = s.address().port;
      s.close(() => resolve(p));
    });
  });
}
async function ready(url) {
  for (let i = 0; i < 60; i++) {
    try {
      if ((await fetch(url)).ok) return;
    } catch {}
    await new Promise((r) => setTimeout(r, 250));
  }
  throw Error("Test server did not start");
}
async function check(name, fn) {
  await fn();
  report.checks.push(name);
  console.log("PASS", name);
}
async function nextReadWindow() {
  console.log("Waiting for the next real console rate-limit window");
  await new Promise((resolve) =>
    setTimeout(resolve, 60100 - (Date.now() % 60000)),
  );
}
try {
  const binary = path.join(
    temp,
    process.platform === "win32" ? "starhub.exe" : "starhub",
  );
  execFileSync("go", ["build", "-o", binary, "."], {
    cwd: repo,
    stdio: "pipe",
  });
  const port = await freePort();
  backend = spawn(binary, [], {
    cwd: temp,
    windowsHide: true,
    stdio: "ignore",
    env: {
      ...process.env,
      STARHUB_PORT: String(port),
      STARHUB_ROOT_PASSWORD: "BrowserInitialQuota92",
      STARHUB_BIND_ADDRESS: "127.0.0.1",
      STARHUB_REGION_PROFILE: "",
      STARHUB_CRYPTO_SECRET: "test-only-key-quota-encryption-fixture",
      RESOURCE_CATALOG_NOTIFY_URL: "",
    },
  });
  await ready(`http://127.0.0.1:${port}/api/plaza/models`);
  vite = await createViteServer({
    root,
    logLevel: "error",
    server: {
      host: "127.0.0.1",
      port: await freePort(),
      proxy: {
        "/api": `http://127.0.0.1:${port}`,
        "/v1": `http://127.0.0.1:${port}`,
      },
    },
  });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  browser = await chromium.launch({
    channel: process.platform === "win32" ? "msedge" : undefined,
    headless: true,
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    reducedMotion: "reduce",
    permissions: ["clipboard-read", "clipboard-write"],
  });
  await context.addInitScript(() => {
    if (!localStorage.getItem("xy-language"))
      localStorage.setItem("xy-language", "en");
  });
  const page = await context.newPage();
  page.setDefaultTimeout(45000);
  page.on("pageerror", (e) => report.pageErrors.push(String(e)));
  page.on("response", (response) => {
    if (response.status() >= 400)
      report.failedRequests.push({
        path: new URL(response.url()).pathname,
        status: response.status(),
        retryAfter: response.headers()["retry-after"],
      });
  });
  async function api(method, url, data) {
    const r = await context.request.fetch(base + url, { method, data });
    const b = await r.json();
    assert.ok(r.ok() && b.success, b.message || `API rejected ${url}`);
    return b.data;
  }
  await check(
    "staff fixture completes real password and factor initialization",
    async () => {
      await api("POST", "/api/user/login", {
        username: "root",
        password: "BrowserInitialQuota92",
      });
      assert.equal(
        (await context.request.get(base + "/api/option/")).status(),
        403,
      );
      await api("PUT", "/api/user/self", {
        current_password: "BrowserInitialQuota92",
        new_password: password,
      });
      const factor = await api("POST", "/api/user/2fa/setup", { password });
      await api("POST", "/api/user/2fa/enable", {
        code: authenticatorCode(factor.secret),
      });
      assert.equal(
        (await context.request.get(base + "/api/option/")).status(),
        403,
      );
      await api("POST", "/api/user/logout");
      const proof = await api("POST", "/api/user/login", {
        username: "root",
        password,
      });
      assert.equal(proof["2fa_required"], true);
      await api("POST", "/api/user/login/2fa", {
        username: "root",
        code: authenticatorCode(factor.secret),
      });
      assert.equal(
        (await context.request.get(base + "/api/option/")).status(),
        200,
      );
    },
  );
  upstream = createServer((r, w) => {
    r.resume();
    w.writeHead(200, { "Content-Type": "application/json" });
    w.end(
      JSON.stringify({
        choices: [
          {
            message: { role: "assistant", content: "ok" },
            finish_reason: "stop",
          },
        ],
        usage: { prompt_tokens: 10, completion_tokens: 2 },
      }),
    );
  });
  await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  await api("POST", "/api/channel", {
    name: "Quota browser fixture",
    type: 1,
    status: 1,
    key: "fixture-only",
    base_url: `http://127.0.0.1:${upstream.address().port}`,
    models: "browser-period-model",
    group: "default",
    weight: 1,
  });
  await api("PUT", "/api/region-profile", {
    id: "uae",
    name: "United Arab Emirates",
    timezone: "Asia/Dubai",
    enabled: true,
  });
  const existing = await api("POST", "/api/token", {
    name: "Existing key",
    remain_quota: 1000,
    model_limits: "browser-period-model",
    group: "default",
  });
  const row = (id) => page.locator(`[data-key-id="${id}"]`).first();
  async function keys() {
    await page.goto(base + "/console/keys");
    await row(existing.id).waitFor();
  }
  async function capture(name) {
    const file = path.join(out, name + ".png");
    await page.screenshot({
      path: file,
      mask: [
        page.locator(".key-list-credential code"),
        page.locator("input[type=password]"),
        page.locator(".created-secret"),
      ],
    });
    report.screenshots.push(name + ".png");
  }
  await check(
    "one-nano redemption credits the canonical wallet through the console",
    async () => {
      const before = await api("GET", "/api/wallet/v2");
      const codes = await api("POST", "/api/redemption", {
        quota_nano: "1",
        count: 1,
      });
      await nextReadWindow();
      await page.goto(base + "/console/billing");
      await page.locator("#redeem-code-input").fill(codes.codes[0]);
      const response = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/user/redeem") &&
          r.request().method() === "POST",
      );
      await page.getByRole("button", { name: "Redeem", exact: true }).click();
      const redeemed = await (await response).json();
      assert.ok(redeemed.success, redeemed.message);
      assert.equal(redeemed.data.quota_nano, "1");
      await page.getByText("Redeemed $0.000000001", { exact: true }).waitFor();
      assert.equal(
        BigInt((await api("GET", "/api/wallet/v2")).balance_nano),
        BigInt(before.balance_nano) + 1n,
      );
    },
  );
  await keys();
  await check(
    "existing masked key can be recovered after reload and a password check",
    async () => {
      assert.ok(
        (
          await row(existing.id)
            .locator(".key-list-credential code")
            .innerText()
        ).includes("*"),
      );
      await row(existing.id).locator('[data-key-action="reveal"]').click();
      await page.locator("#key-auth-password").fill(password);
      await page.locator("#key-auth-password").press("Enter");
      await page.waitForFunction(
        (id) =>
          document
            .querySelector(`[data-key-id="${id}"] .key-list-credential code`)
            ?.textContent?.includes("*") === false,
        existing.id,
      );
      assert.ok(
        (await row(existing.id)
          .locator(".key-list-credential code")
          .innerText()) === existing.key,
        "verified key differs",
      );
      await row(existing.id).locator('[data-key-action="reveal"]').click();
      await row(existing.id).locator('[data-key-action="reveal"]').click();
      await page.waitForFunction(
        () => !document.querySelector("#key-auth-password"),
      );
      await page.reload();
      await row(existing.id).waitFor();
      assert.ok(
        (
          await row(existing.id)
            .locator(".key-list-credential code")
            .innerText()
        ).includes("*"),
        "reload retained plaintext",
      );
    },
  );
  await check(
    "daily key can be created through the console with a regional timezone",
    async () => {
      await page
        .locator(".key-manager-header")
        .getByRole("button", { name: "Create key", exact: true })
        .click();
      await page.locator("#new-key-name").fill("Regional daily key");
      await page.locator("#new-key-quota").fill("1.000000001");
      await page.locator("#native-key-period").selectOption("daily");
      await page.locator("#native-key-quota-region").selectOption("uae");
      await capture("create-daily-desktop");
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Create key", exact: true })
        .click();
      await page.locator(".created-secret").waitFor();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Close", exact: true })
        .last()
        .click();
    },
  );
  const created = (await api("GET", "/api/token/owned")).find(
    (t) => t.name === "Regional daily key",
  );
  assert.ok(
    created &&
      created.quota_period === "daily" &&
      created.quota_timezone === "Asia/Dubai" &&
      created.period_quota_limit === 500000 &&
      created.period_quota_limit_nano === "1000000001",
  );
  await api("POST", `/api/token/${created.id}/unlock`, {
    expected_user_id: created.user_id,
    password,
  });
  const secret = await api("POST", `/api/token/${created.id}/reveal`, {
    expected_user_id: created.user_id,
  });
  const relay = await context.request.post(base + "/v1/chat/completions", {
    headers: { Authorization: `Bearer ${secret.key}` },
    data: {
      model: "browser-period-model",
      messages: [{ role: "user", content: "hello" }],
      max_tokens: 10,
    },
  });
  assert.ok(relay.ok(), "real gateway fixture call failed");
  await check(
    "monthly policy edit retains spending and shows both period records",
    async () => {
      await keys();
      await row(created.id).locator('[data-key-action="edit"]').click();
      await page.locator("#native-key-period").selectOption("monthly");
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Save", exact: true })
        .click();
      await page.locator("#native-key-period").waitFor({ state: "hidden" });
      await row(created.id)
        .locator('[data-key-action="details"]')
        .first()
        .click();
      await page
        .locator(`[data-token-quota-details="${created.id}"]`)
        .waitFor();
      const data = await api("GET", `/api/token/${created.id}/usage`);
      assert.ok(
        data.current.period === "monthly" &&
          data.current.used_quota > 0 &&
          data.current.used_quota === data.current.lifetime_used_quota &&
          data.period_total === 2 &&
          data.models[0].model === "browser-period-model",
      );
      assert.equal(await page.locator(".key-usage-bars > div").count(), 30);
      assert.equal(
        await page.locator("[data-key-period-history] tbody tr").count(),
        2,
      );
      await capture("usage-desktop");
      const accessibility = await new AxeBuilder({ page })
        .include(".drawer")
        .withTags(["wcag2a", "wcag2aa"])
        .analyze();
      assert.equal(
        accessibility.violations.length,
        0,
        accessibility.violations.map((v) => v.id).join(","),
      );
    },
  );
  await check(
    "Chinese light and Arabic dark mobile details remain usable",
    async () => {
      for (const [language, theme] of [
        ["zh", "light"],
        ["ar", "dark"],
      ]) {
        await page.evaluate(
          ([language, theme]) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          [language, theme],
        );
        await page.setViewportSize({ width: 390, height: 844 });
        await keys();
        await row(created.id)
          .locator('[data-key-action="details"]')
          .first()
          .click();
        await page
          .locator(`[data-token-quota-details="${created.id}"]`)
          .waitFor();
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          "page overflow",
        );
        await page.locator("[data-key-model-usage]").scrollIntoViewIfNeeded();
        await capture(`usage-mobile-${language}-${theme}`);
        if (language === "ar")
          assert.equal(await page.locator("html").getAttribute("dir"), "rtl");
      }
    },
  );
  await check(
    "canonical wallet keeps one-nano grants and simulated payments separate",
    async () => {
      const before = await api("GET", "/api/wallet/v2");
      const adjustment = {
        schema: "money_adjustment_v2",
        expected_actor_id: created.user_id,
        user_id: created.user_id,
        operation_id: "browser-one-nano",
        amount_nano: "1",
        expected_available_nano: before.balance_nano,
        reason: "Browser precision verification",
      };
      const receipt = await api("POST", "/api/wallet/adjustments", adjustment);
      const replay = await api("POST", "/api/wallet/adjustments", adjustment);
      assert.equal(replay.entry.id, receipt.entry.id);
      const credited = await api("GET", "/api/wallet/v2");
      assert.equal(
        BigInt(credited.balance_nano),
        BigInt(before.balance_nano) + 1n,
      );
      const simulated = await api("POST", "/api/recharge", {
        amount_usd: "10.00",
        channel: "card",
      });
      await api("POST", `/api/recharge/${simulated.id}/resolve`, {
        outcome: "success",
      });
      assert.equal(
        (await api("GET", "/api/wallet/v2")).balance_nano,
        credited.balance_nano,
      );
      await api("PUT", "/api/option", {
        key: "model_price",
        value: JSON.stringify({ "browser-period-model": 0.000000001 }),
      });
      const response = await context.request.post(
        base + "/v1/chat/completions",
        {
          headers: { Authorization: `Bearer ${secret.key}` },
          data: {
            model: "browser-period-model",
            messages: [{ role: "user", content: "one nano" }],
            max_tokens: 10,
          },
        },
      );
      assert.ok(response.ok());
      const charged = await api("GET", "/api/wallet/v2");
      assert.equal(
        BigInt(charged.balance_nano),
        BigInt(credited.balance_nano) - 1n,
      );
      assert.equal(
        BigInt(charged.spent_nano),
        BigInt(credited.spent_nano) + 1n,
      );
      assert.equal(charged.reserved_nano, "0");
      const ledger = await api("GET", "/api/ledger/v2?page=1&page_size=10");
      assert.ok(
        ledger.items.some(
          (row) => row.type === "consumption" && row.spent_delta_nano === "1",
        ),
      );
      assert.ok(
        (
          await api(
            "GET",
            `/api/wallet/reconciliation?user_id=${created.user_id}`,
          )
        ).verified,
      );
      await page.evaluate(() => {
        localStorage.setItem("xy-language", "en");
        localStorage.setItem("xy-theme", "light");
      });
      await page.setViewportSize({ width: 1440, height: 1000 });
      await nextReadWindow();
      await page.goto(base + "/console/billing");
      await page.locator("[data-money-entry]").first().waitFor();
      const journal = page.locator("[data-money-ledger]");
      assert.ok((await journal.innerText()).includes("$0.000000001"));
      assert.ok(
        (await page.locator(".bc-wallet-card").innerText()).includes(
          "Available API balance",
        ),
      );
      await capture("canonical-wallet-desktop");
      const a11y = await new AxeBuilder({ page })
        .include("[data-money-ledger]")
        .withTags(["wcag2a", "wcag2aa"])
        .analyze();
      assert.equal(
        a11y.violations.length,
        0,
        a11y.violations.map((v) => v.id).join(","),
      );
      for (const [language, theme, width] of [
        ["zh", "light", 390],
        ["ar", "dark", 320],
      ]) {
        await page.evaluate(
          ([language, theme]) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          [language, theme],
        );
        await page.setViewportSize({ width, height: 844 });
        await nextReadWindow();
        await page.reload();
        await page.locator("[data-money-entry]").first().waitFor();
        await page.locator("[data-money-ledger]").scrollIntoViewIfNeeded();
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          "money ledger overflow",
        );
        await capture(`canonical-wallet-${language}-${theme}-${width}`);
      }
    },
  );
  // Let the real per-minute API limit replenish before testing more writes.
  await nextReadWindow();
  await check(
    "Root console grants one nano without legacy quota scaling",
    async () => {
      await page.evaluate(() => {
        localStorage.setItem("xy-language", "en");
        localStorage.setItem("xy-theme", "light");
      });
      await page.setViewportSize({ width: 1440, height: 1000 });
      const before = await api("GET", "/api/wallet/v2");
      await page.goto(base + "/admin?tab=customers");
      await page
        .locator(`[data-admin-money-user="${created.user_id}"]`)
        .click();
      await page.locator("#admin-top-up-amount").fill("0.000000001");
      await page.locator("#admin-top-up-type").selectOption("gift");
      await page.locator("#admin-top-up-reason").fill("Browser nano grant");
      await capture("root-nano-adjustment-desktop");
      await page.locator('button[form="admin-top-up-form"]').click();
      await page.locator("#admin-top-up-amount").waitFor({ state: "hidden" });
      assert.equal(
        BigInt((await api("GET", "/api/wallet/v2")).balance_nano),
        BigInt(before.balance_nano) + 1n,
      );
      await page.goto(base + "/console/billing");
      const download = page.waitForEvent("download");
      await page
        .getByRole("button", { name: "Export last 31 days", exact: true })
        .click();
      const exported = await download;
      const file = path.join(temp, "nano-ledger.csv");
      await exported.saveAs(file);
      const csv = await readFile(file, "utf8");
      assert.ok(
        csv.includes("Spent Delta (nano-USD)") &&
          csv.includes("Browser nano grant") &&
          csv.includes("n2:"),
        "canonical CSV lost audit evidence",
      );
    },
  );
  await check(
    "finance view separates spendable API funds from legacy payment balances",
    async () => {
      const wallet = await api("GET", "/api/wallet/v2");
      const summary = await api("GET", "/api/finance/summary?days=30");
      assert.equal(summary.api_available_nano, wallet.balance_nano);
      assert.equal(summary.api_recorded_reserved_nano, "0");
      assert.equal(summary.wallet_balance_nano, "10000000000");
      assert.equal(summary.verified_payments_available, false);
      await page.goto(base + "/admin?tab=finance");
      await page.locator('[data-metric="api-available"]').waitFor();
      await page.locator('[data-metric="api-available"] summary').click();
      assert.equal(
        await page.locator('[data-metric="api-available"] code').innerText(),
        wallet.balance_nano,
      );
      await capture("finance-canonical-desktop");
      for (const [language, theme, width] of [
        ["zh", "light", 390],
        ["ar", "dark", 320],
      ]) {
        await page.evaluate(
          ([language, theme]) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          [language, theme],
        );
        await page.setViewportSize({ width, height: 844 });
        await nextReadWindow();
        await page.reload();
        await page.locator('[data-metric="api-available"]').waitFor();
        await page
          .locator('[data-metric="api-available"]')
          .scrollIntoViewIfNeeded();
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          "finance viewport overflow",
        );
        await capture(`finance-canonical-${language}-${theme}-${width}`);
      }
    },
  );
  await check("logout invalidates the server-side reveal grant", async () => {
    await api("POST", "/api/user/logout-current", {
      schema: "logout_current_v1",
      expected_user_id: created.user_id,
    });
    const response = await context.request.post(
      base + `/api/token/${created.id}/reveal`,
      { data: { expected_user_id: created.user_id } },
    );
    assert.equal(response.status(), 401);
  });
  assert.deepEqual(report.pageErrors, []);
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify(report, null, 2),
  );
  await browser?.close();
  await vite?.close();
  if (upstream) await new Promise((resolve) => upstream.close(resolve));
  if (backend && !backend.killed && backend.exitCode === null) {
    const exited = new Promise((resolve) => backend.once("exit", resolve));
    backend.kill();
    await exited;
  }
  if (
    path.dirname(path.resolve(temp)) !== path.resolve(os.tmpdir()) ||
    !path.basename(temp).startsWith("starhub-key-quota-")
  )
    throw Error("Unsafe test cleanup target");
  await rm(temp, { recursive: true, force: true });
}
