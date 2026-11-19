// Provider checks use an isolated backend, temporary database and local upstream.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { createServer } from "node:http";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-providers-"));
const out = path.join(web, "test-results", "channel-providers");
await mkdir(out, { recursive: true });
const report = { checks: [], errors: [] };
let backend, vite, browser, upstream;
const hits = [];
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
try {
  upstream = createServer((req, res) => {
    hits.push({ method: req.method, path: req.url, auth: req.headers.authorization });
    res.setHeader("Content-Type", "application/json");
    if (req.headers.authorization === "Bearer rejected-key") {
      res.writeHead(401);
      return res.end('{"error":{"message":"Fixture denied"}}');
    }
    res.end('{"data":[{"id":"model-a"},{"id":"model-b"}]}');
  });
  await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  const upstreamBase = `http://127.0.0.1:${upstream.address().port}`;
  const binary = path.join(temp, process.platform === "win32" ? "starhub.exe" : "starhub");
  execFileSync("go", ["build", "-o", binary, "."], { cwd: path.resolve(web, ".."), stdio: "pipe" });
  const port = await freePort();
  backend = spawn(binary, [], {
    cwd: temp, windowsHide: true, stdio: "ignore",
    env: {
      ...process.env, STARHUB_CONFIG: "", STARHUB_PORT: String(port), STARHUB_BIND_ADDRESS: "127.0.0.1",
      STARHUB_DB: path.join(temp, "starhub.db"), STARHUB_COOKIE_SECURE: "false",
      STARHUB_ROOT_PASSWORD: "ProviderInitial92", STARHUB_ENV: "development",
      STARHUB_CRYPTO_SECRET: "provider-browser-fixture-secret",
      STARHUB_CREDENTIAL_KEYS: JSON.stringify({ fixture: "11".repeat(32) }), STARHUB_CREDENTIAL_ACTIVE_KEY: "fixture",
      RESOURCE_CATALOG_NOTIFY_URL: "", STARHUB_REGION_PROFILE: "",
    },
  });
  let ready = false;
  for (let attempt = 0; attempt < 80; attempt++) {
    try { ready = (await fetch(`http://127.0.0.1:${port}/api/health`)).ok; } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, "isolated backend did not start");
  vite = await createViteServer({
    root: web, cacheDir: path.join(temp, "vite-cache"), logLevel: "error",
    server: { host: "127.0.0.1", port: await freePort(), proxy: { "/api": `http://127.0.0.1:${port}` } },
  });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  browser = await chromium.launch({ headless: true, ...(process.platform === "win32" ? { channel: "chrome" } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: "reduce" });
  await context.addInitScript(() => {
    if (!localStorage.getItem("xy-language")) localStorage.setItem("xy-language", "en");
  });
  const api = async (method, route, data) => {
    const response = await context.request.fetch(base + route, { method, data });
    assert.equal(response.status(), 200, await response.text());
    const body = await response.json();
    assert.equal(body.success, true);
    return body.data;
  };
  assert.equal((await context.request.post(base + "/api/channel", { data: { type: 26 } })).status(), 401);
  await api("POST", "/api/user/login", { username: "root", password: "ProviderInitial92" });
  await api("PUT", "/api/user/self", { current_password: "ProviderInitial92", new_password: "ProviderChanged93" });
  await api("POST", "/api/user/2fa/skip");
  await api("PUT", "/api/option", { key: "ip_rate_limit", value: "10000" });
  const page = await context.newPage();
  page.setDefaultTimeout(30000);
  page.on("pageerror", (error) => report.errors.push(String(error)));
  const dialog = page.getByRole("dialog");
  for (const [type, name, defaultBase, catalogPath] of [
    [26, "Zhipu (GLM)", "https://open.bigmodel.cn/api/paas/v4", null],
    [24, "Kimi (Moonshot)", "https://api.moonshot.cn/v1", "/v1/models"],
    [45, "DeepSeek", "https://api.deepseek.com", "/models"],
  ]) {
    await page.goto(base + "/admin?tab=resources#regions", { waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "New channel", exact: true }).click();
    assert.equal(await dialog.locator("#ch-type option").count(), 9);
    await dialog.locator("#ch-name").fill(name);
    await dialog.locator("#ch-key").fill("fixture-key");
    await dialog.locator("#ch-models").fill("manual-model");
    await dialog.locator("#ch-base-url").fill(upstreamBase);
    await dialog.locator("#ch-type").selectOption(String(type));
    assert.equal(await dialog.locator("#ch-models").inputValue(), "manual-model");
    assert.equal(await dialog.locator("#ch-base-url").inputValue(), upstreamBase);
    assert.equal(await dialog.locator("#ch-base-url").getAttribute("placeholder"), defaultBase);
    const discover = dialog.getByRole("button", { name: "Fetch upstream models", exact: true });
    if (catalogPath) {
      await discover.click();
      await dialog.getByRole("checkbox", { name: "model-a", exact: true }).check();
      await dialog.getByRole("button", { name: "Add selected models", exact: true }).click();
      assert.equal(await dialog.locator("#ch-models").inputValue(), "manual-model,model-a");
      assert.deepEqual(hits.at(-1), { method: "GET", path: catalogPath, auth: "Bearer fixture-key" });
    } else {
      assert.equal(await discover.count(), 0);
      await dialog.getByText("Model discovery is not configured for this type. Enter model IDs from the provider’s console.", { exact: true }).waitFor();
    }
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await dialog.waitFor({ state: "hidden" });
    const saved = (await api("GET", "/api/channel")).find((entry) => entry.name === name);
    assert.equal(saved.type, type);
    assert.equal(saved.base_url, upstreamBase);
    await page.reload();
    await page.getByRole("row").filter({ hasText: name }).getByRole("button", { name: "Edit", exact: true }).click();
    assert.equal(await dialog.locator("#ch-type").inputValue(), String(type));
    assert.equal(await dialog.locator("#ch-key").inputValue(), "");
    assert.equal(await dialog.locator("#ch-models").inputValue(), saved.models);
    if (catalogPath) {
      await discover.click();
      await dialog.getByRole("checkbox", { name: "model-a", exact: true }).waitFor();
      assert.equal(hits.at(-1).auth, "Bearer fixture-key");
      await dialog.locator("#ch-key").fill("rejected-key");
      await discover.click();
      await dialog.getByRole("alert").filter({ hasText: "Upstream returned 401" }).waitFor();
      assert.equal(await dialog.locator("#ch-models").inputValue(), saved.models);
      assert.equal((await api("GET", "/api/user/self")).username, "root");
      await dialog.locator("#ch-key").fill("");
    }
    await dialog.getByRole("button", { name: "Save", exact: true }).click();
    await dialog.waitFor({ state: "hidden" });
    const updated = (await api("GET", "/api/channel")).find((entry) => entry.id === saved.id);
    assert.equal(updated.type, type);
    assert.equal(updated.models, saved.models);
    assert.equal(updated.key_count, 1);
    report.checks.push(`${name}: create, refresh, edit, saved credentials and ${catalogPath ? "real model discovery with authentication errors" : "manual model entry"}`);
  }
  await page.getByRole("button", { name: "New channel", exact: true }).click();
  for (const language of ["zh", "en", "ar"]) {
    for (const [theme, width] of [["light", 1440], ["dark", 320]]) {
      await page.evaluate(async ({ language, theme }) => {
        const { usePreferences } = await import("/src/lib/preferences.ts");
        usePreferences.getState().setLanguage(language);
        usePreferences.getState().setTheme(theme);
      }, { language, theme });
      await page.setViewportSize({ width, height: 1000 });
      for (const type of [26, 24, 45]) {
        await dialog.locator("#ch-type").selectOption(String(type));
        await dialog.locator("#ch-type").focus();
        await page.keyboard.press("Tab");
        assert.ok(await dialog.evaluate((node) => node.contains(document.activeElement)));
        await dialog.locator("#ch-type").scrollIntoViewIfNeeded();
        assert.ok(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth + 1));
        assert.ok(await dialog.locator(".task-dialog-body").evaluate((node) => node.scrollWidth <= node.clientWidth + 1));
        await page.screenshot({ path: path.join(out, `provider-${type}-${language}-${theme}-${width}.png`) });
        await dialog.locator("#ch-base-url").scrollIntoViewIfNeeded();
        assert.ok(await dialog.locator("#ch-base-url").isVisible());
      }
    }
  }
  const accessibility = await new AxeBuilder({ page }).include('[role="dialog"]').withTags(["wcag2a", "wcag2aa"]).analyze();
  assert.deepEqual(accessibility.violations, []);
  report.checks.push("all three provider options support three languages, light/dark themes, desktop/320px, keyboard focus, modal scrolling and accessibility checks");
  assert.deepEqual(report.errors, []);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.failure = String(error.stack || error);
  const page = browser?.contexts()[0]?.pages()[0];
  if (page) await page.screenshot({ path: path.join(out, "failure.png"), mask: [page.locator('input[type="password"]')] });
  throw error;
} finally {
  await writeFile(path.join(out, "report.json"), JSON.stringify(report, null, 2));
  await browser?.close();
  await vite?.close();
  backend?.kill();
  await new Promise((resolve) => upstream ? upstream.close(resolve) : resolve());
}
