// Uses an isolated database and never requests an upstream model or generation.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-model-registration-"));
const out = path.join(web, "playwright-report", "channel-model-registration");
await mkdir(out, { recursive: true });
const report = { checks: [], screenshots: [], pageErrors: [] };
let backend, vite, browser;
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
try {
  const binary = path.join(temp, process.platform === "win32" ? "starhub.exe" : "starhub");
  execFileSync("go", ["build", "-o", binary, "."], { cwd: path.resolve(web, ".."), stdio: "pipe" });
  const port = await freePort();
  backend = spawn(binary, [], {
    cwd: temp, windowsHide: true, stdio: "ignore",
    env: {
      ...process.env, STARHUB_CONFIG: "", STARHUB_PORT: String(port), STARHUB_BIND_ADDRESS: "127.0.0.1",
      STARHUB_DB: path.join(temp, "starhub.db"), STARHUB_COOKIE_SECURE: "false",
      STARHUB_ROOT_PASSWORD: "RegistrationInitial92", STARHUB_ENV: "development",
      STARHUB_CRYPTO_SECRET: "model-registration-fixture-secret",
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
  browser = await chromium.launch({ headless: true, ...(process.platform === "win32" ? { channel: "msedge" } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: "reduce" });
  await context.addInitScript(() => localStorage.setItem("xy-language", "en"));
  const api = async (method, route, data) => {
    const response = await context.request.fetch(base + route, { method, data });
    assert.equal(response.status(), 200);
    const body = await response.json();
    assert.equal(body.success, true);
    return body.data;
  };
  await api("POST", "/api/user/login", { username: "root", password: "RegistrationInitial92" });
  await api("PUT", "/api/user/self", { current_password: "RegistrationInitial92", new_password: "RegistrationChanged93" });
  await api("POST", "/api/user/2fa/skip");
  const initialPricing = await api("GET", "/api/option/pricing");
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on("pageerror", (error) => report.pageErrors.push(String(error)));
  let catalogReads = 0;
  page.on("request", (request) => { if (new URL(request.url()).pathname === "/api/plaza/meta/configuration") catalogReads++; });
  await page.goto(base + "/admin?tab=resources");
  const pricing = page.locator(".server-token-pricing");
  await pricing.getByText("No matching registered models.", { exact: false }).waitFor();
  assert.equal(await pricing.locator("[data-server-priced-model]").count(), 0);
  assert.ok(Object.keys(JSON.parse(initialPricing.options.find((option) => option.key === "model_ratio").value)).length > 0);
  assert.equal(await pricing.getByRole("button", { name: "Configure model pricing", exact: true }).isDisabled(), true);
  report.checks.push("seeded pricing defaults do not create display rows when no platform models are registered");
  await pricing.getByLabel("Search models", { exact: true }).fill("catalog-case");
  const readsBefore = catalogReads;
  await page.getByRole("button", { name: "New channel", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.locator("#ch-name").fill("Catalog fixture");
  await dialog.locator("#ch-key").fill("fixture-key");
  await dialog.locator("#ch-base-url").fill("http://127.0.0.1:9");
  await dialog.locator("#ch-models").fill("catalog-case, catalog-case, catalog-case-video");
  await dialog.getByRole("button", { name: "Create", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  const row = (id) => pricing.locator(`tr[data-server-priced-model="${id}"]`);
  await row("catalog-case").getByText("Awaiting pricing", { exact: true }).waitFor();
  assert.ok(catalogReads > readsBefore, "channel save must invalidate the active catalog cache");
  assert.deepEqual(await api("GET", "/api/option/pricing"), initialPricing);
  const catalog = await api("GET", "/api/plaza/meta/configuration");
  assert.equal(catalog.models.length, 2);
  for (const model of catalog.models) {
    assert.equal(model.official_input_per_m, null);
    assert.equal(model.per_request_usd, null);
  }
  report.checks.push("creating a channel registers distinct models and refreshes pricing rows immediately without writing prices");
  await page.getByRole("row").filter({ hasText: "Catalog fixture" }).getByRole("button", { name: "Edit", exact: true }).click();
  await dialog.locator("#ch-models").fill("catalog-case,catalog-case-video,catalog-case-added");
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  await row("catalog-case-added").getByRole("button", { name: "Set pricing", exact: true }).waitFor();
  await pricing.getByLabel("Search models", { exact: true }).fill("");
  assert.equal(await pricing.locator("[data-server-priced-model]").count(), 3);
  await pricing.getByRole("button", { name: "Configure model pricing", exact: true }).click();
  await dialog.getByLabel("Server model ID", { exact: true }).fill("unregistered-model");
  await dialog.getByLabel("Input base USD/million tokens", { exact: true }).fill("1");
  await dialog.getByLabel("Output/input ratio", { exact: true }).fill("4");
  assert.equal(await dialog.getByRole("button", { name: "Review server changes", exact: true }).isDisabled(), true);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  report.checks.push("editing a channel registers additional models and refreshes the active catalog");
  await row("catalog-case").getByRole("button", { name: "Set pricing", exact: true }).click();
  assert.equal(await dialog.getByLabel("Server model ID", { exact: true }).inputValue(), "catalog-case");
  assert.equal(await dialog.getByLabel("Server model ID", { exact: true }).getAttribute("readonly"), "");
  assert.equal(await dialog.getByLabel("Input base USD/million tokens", { exact: true }).inputValue(), "");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  assert.deepEqual(await api("GET", "/api/option/pricing"), initialPricing);
  await row("catalog-case").getByRole("button", { name: "Set pricing", exact: true }).click();
  await dialog.getByLabel("Pricing mode", { exact: true }).selectOption("per_request");
  await dialog.getByLabel("Per request USD/request", { exact: true }).fill("0.025");
  await dialog.getByRole("button", { name: "Review server changes", exact: true }).click();
  assert.ok((await dialog.innerText()).includes("Unconfigured"));
  await dialog.getByRole("button", { name: "Confirm server pricing", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  await row("catalog-case").getByRole("button", { name: "Edit request price", exact: true }).waitFor();
  const priced = await api("GET", "/api/option/pricing");
  assert.equal(JSON.parse(priced.options.find((option) => option.key === "model_price").value)["catalog-case"], 0.025);
  for (const option of initialPricing.options.filter((option) => option.key !== "model_price"))
    assert.equal(priced.options.find((value) => value.key === option.key).value, option.value);
  report.checks.push("initial prices stay blank, cancel writes nothing, and explicit review saves only the selected model price");
  await page.route("**/api/plaza/meta/configuration", (route) => route.fulfill({ status: 503, json: { success: false, message: "Fixture unavailable" } }));
  await pricing.getByRole("button", { name: "Refresh pricing", exact: true }).click();
  await pricing.getByRole("alert").filter({ hasText: "Model catalog unavailable" }).waitFor();
  assert.equal(await row("catalog-case-video").count(), 0);
  assert.equal(await row("catalog-case").count(), 0);
  await page.unroute("**/api/plaza/meta/configuration");
  await pricing.getByRole("button", { name: "Refresh pricing", exact: true }).click();
  await row("catalog-case-video").waitFor();
  report.checks.push("failed catalog reads hide stale rows without falling back to defaults, and recover on refresh");
  for (const language of ["zh", "en", "ar"]) {
    for (const theme of ["light", "dark"]) {
      await page.evaluate(async ({ language, theme }) => {
        const entry = performance.getEntriesByType("resource").find((entry) => entry.name.includes("/src/lib/preferences.ts"));
        const { usePreferences } = await import(entry.name);
        usePreferences.getState().setLanguage(language);
        usePreferences.getState().setTheme(theme);
      }, { language, theme });
      for (const width of [1440, 320]) {
        await page.setViewportSize({ width, height: 1000 });
        await row("catalog-case-video").scrollIntoViewIfNeeded();
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
        if (width === 320) {
          const action = row("catalog-case-video").getByRole("button");
          await action.focus();
          await page.keyboard.press("Enter");
          await dialog.waitFor();
          assert.equal(await dialog.locator('input[readonly]').inputValue(), "catalog-case-video");
          await page.keyboard.press("Escape");
          await dialog.waitFor({ state: "hidden" });
          assert.deepEqual((await new AxeBuilder({ page }).include(".server-token-pricing").withTags(["wcag2a", "wcag2aa"]).analyze()).violations, []);
        }
        const name = `${language}-${theme}-${width}.png`;
        await page.screenshot({ path: path.join(out, name) });
        report.screenshots.push(name);
      }
    }
  }
  report.checks.push("three languages, light/dark themes and desktop/320px views keep pricing actions usable with no accessibility violations");
  assert.deepEqual(report.pageErrors, []);
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
}
