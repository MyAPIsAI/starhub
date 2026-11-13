// Exercise real key APIs and console interactions against an isolated SQLite
// server. Locale/theme preferences are the only client-side fixture settings.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { createServer } from "node:http";
import { execFileSync, spawn } from "node:child_process";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
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
const report = { checks: [], screenshots: [], pageErrors: [] };
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
      STARHUB_ROOT_PASSWORD: password,
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
  page.setDefaultTimeout(12000);
  page.on("pageerror", (e) => report.pageErrors.push(String(e)));
  async function api(method, url, data) {
    const r = await context.request.fetch(base + url, { method, data });
    const b = await r.json();
    assert.ok(r.ok() && b.success, b.message || `API rejected ${url}`);
    return b.data;
  }
  await api("POST", "/api/user/login", { username: "root", password });
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
      await page.locator("#new-key-quota").fill("1");
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
      created.period_quota_limit === 500000,
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
  if (backend && !backend.killed) {
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
