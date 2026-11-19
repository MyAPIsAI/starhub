// All availability changes use an isolated database and a local mock upstream.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { createServer } from "node:http";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-channel-status-"));
const out = path.join(web, "test-results", "channel-status");
await mkdir(out, { recursive: true });
const report = { checks: [], pageErrors: [] };
let backend, vite, browser, upstream;
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
      STARHUB_ROOT_PASSWORD: "StatusInitial92", STARHUB_ENV: "development",
      STARHUB_CRYPTO_SECRET: "channel-status-fixture-secret",
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
    server: { host: "127.0.0.1", port: await freePort(), proxy: { "/api": `http://127.0.0.1:${port}`, "/v1": `http://127.0.0.1:${port}` } },
  });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  upstream = createServer((req, res) => {
    res.setHeader("Content-Type", "application/json");
    if (req.headers.authorization === "Bearer bad-fixture-key") {
      res.writeHead(401);
      return res.end(JSON.stringify({ error: { message: "Incorrect API key provided", type: "invalid_request_error", code: "invalid_api_key" } }));
    }
    res.end(JSON.stringify({ id: "mock", model: "gpt-4o-mini", choices: [{ message: { role: "assistant", content: "OK" } }], usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 } }));
  });
  await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  const upstreamURL = `http://127.0.0.1:${upstream.address().port}`;
  browser = await chromium.launch({ headless: true, ...(process.platform === "win32" ? { channel: "msedge" } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: "reduce" });
  await context.addInitScript(() => {
    if (!localStorage.getItem("xy-language")) localStorage.setItem("xy-language", "zh");
  });
  const api = async (method, route, data) => {
    const response = await context.request.fetch(base + route, { method, data });
    assert.equal(response.status(), 200, await response.text());
    const body = await response.json();
    assert.equal(body.success, true);
    return body.data;
  };
  await api("POST", "/api/user/login", { username: "root", password: "StatusInitial92" });
  await api("PUT", "/api/user/self", { current_password: "StatusInitial92", new_password: "StatusChanged93" });
  await api("POST", "/api/user/2fa/skip");
  // Repeated layout reloads must not consume the fixture's management API limit.
  await api("PUT", "/api/option", { key: "ip_rate_limit", value: "10000" });
  const partial = await api("POST", "/api/channel", { name: "Partial fixture", type: 1, models: "gpt-4o-mini,gpt-4o", key: "single-good", base_url: upstreamURL });
  const full = await api("POST", "/api/channel", { name: "Full fixture", type: 1, models: "gpt-4", key: "bad-fixture-key\ngood-fixture-key", key_mode: "round_robin", base_url: upstreamURL });
  const closed = await api("POST", "/api/channel", { name: "Closed fixture", type: 1, models: "closed-model", key: "closed-key", status: 2, base_url: upstreamURL });
  const token = await api("POST", "/api/token", { name: "status-test", unlimited_quota: true });
  const relay = (model) => context.request.post(base + "/v1/chat/completions", {
    headers: { Authorization: `Bearer ${token.key}` }, data: { model, messages: [{ role: "user", content: "hello" }] },
  });
  assert.equal((await relay("gpt-4")).status(), 401);
  assert.deepEqual((await api("GET", `/api/channel/${full.id}`)).multi_key_status, [2, 1]);
  assert.equal((await api("GET", `/api/channel/${full.id}`)).resource_state.state, "enabled");
  const anonymous = await browser.newContext();
  assert.equal((await anonymous.request.put(base + `/api/channel/${partial.id}/resource-status`, {data:{model:"gpt-4o",enabled:false}})).status(), 401);
  await anonymous.close();
  const page = await context.newPage();
  page.setDefaultTimeout(30000);
  page.on("pageerror", (error) => report.pageErrors.push(String(error)));
  let managementWrites = 0;
  page.on("request", (request) => {
    if (request.method() === "PUT" && /\/api\/channel\/\d+\/(?:resource-)?status$/.test(new URL(request.url()).pathname)) managementWrites++;
  });
  const row = (name) => page.locator("tr").filter({ hasText: name });
  const status = (name) => row(name).locator("[data-channel-status]");
  const dialog = page.getByRole("dialog");
  const modelSwitch = (model) => dialog.locator(`[data-resource-model="${model}"]`).getByRole("switch");
  const resource = () => modelSwitch("gpt-4o-mini");
  const openResources = () => status("Partial fixture").click();
  const closeResources = async () => {
    await dialog.locator(".task-dialog-footer").getByRole("button").click();
    await dialog.waitFor({ state: "hidden" });
  };
  const waitForFocus = (name = "Partial fixture") => page.waitForFunction((name) => document.activeElement?.hasAttribute("data-channel-status") && document.activeElement.closest("tr")?.textContent.includes(name), name);
  const enableAll = () => dialog.locator('[data-channel-bulk="enable"]');
  const closeAll = () => dialog.locator('[data-channel-bulk="close"]');
  const waitState = (state) => page.waitForFunction((state) => [...document.querySelectorAll("tr")].find((el) => el.textContent.includes("Partial fixture"))?.querySelector("[data-channel-status]")?.dataset.channelStatus === state, state);
  await page.goto(base + "/admin?tab=resources#regions", {waitUntil:"domcontentloaded"});
  await status("Partial fixture").waitFor();
  assert.equal(await status("Partial fixture").innerText(), "启用");
  assert.equal(await status("Full fixture").innerText(), "启用");
  assert.equal(await status("Closed fixture").innerText(), "关闭");
  assert.equal(await row("Partial fixture").locator("td").nth(2).getByRole("button").count(), 1);
  assert.equal(await status("Partial fixture").getAttribute("aria-haspopup"), "dialog");
  assert.equal(await status("Partial fixture").getAttribute("aria-expanded"), "false");
  const openingWrites = managementWrites;
  await status("Partial fixture").focus();
  await page.keyboard.press("Enter");
  await dialog.waitFor();
  assert.equal(await status("Partial fixture").getAttribute("aria-expanded"), "true");
  await closeResources();
  await waitForFocus();
  assert.equal(await status("Partial fixture").getAttribute("aria-expanded"), "false");
  await page.keyboard.press("Space");
  await dialog.waitFor();
  await page.keyboard.press("Escape");
  await dialog.waitFor({ state: "hidden" });
  await waitForFocus();
  assert.equal(managementWrites, openingWrites);
  assert.equal((await api("GET", `/api/channel/${partial.id}`)).resource_state.state, "enabled");
  report.checks.push("one accessible status entry opens with Enter/Space, Done/Escape return focus, and opening or dismissing never writes availability");
  await openResources();
  await resource().waitFor();
  assert.equal(await dialog.getByRole("switch").count(), 2);
  let release, writes = 0;
  const gate = new Promise((resolve) => { release = resolve; });
  const resourceEndpoint = `**/api/channel/${partial.id}/resource-status`;
  await page.route(resourceEndpoint, async (route) => { writes++; await gate; await route.continue(); });
  await resource().click();
  await page.waitForFunction(() => document.querySelector('[data-resource-model="gpt-4o-mini"] [role="switch"]')?.disabled);
  assert.equal((await api("GET", `/api/channel/${partial.id}`)).resource_state.enabled, 2);
  release();
  await waitState("partial");
  assert.equal(writes, 1);
  await page.unroute(resourceEndpoint);
  assert.equal(await resource().getAttribute("aria-checked"), "false");
  assert.equal(await status("Partial fixture").innerText(), "部分启用 (1/2)");
  assert.notEqual((await relay("gpt-4o-mini")).status(), 200);
  assert.equal((await relay("gpt-4o")).status(), 200);
  await closeResources();
  await page.reload();
  await waitState("partial");
  report.checks.push("single-key multiple-model resources reach partial state through real UI changes, persist after refresh, prevent duplicate writes and stop only the selected model");

  await openResources();
  await page.route(resourceEndpoint, (route) => route.fulfill({status:503,json:{success:false,message:"Fixture failure"}}));
  await resource().click();
  await dialog.getByRole("alert").waitFor();
  assert.equal(await resource().getAttribute("aria-checked"), "false");
  assert.equal((await api("GET", `/api/channel/${partial.id}`)).resource_state.enabled, 1);
  await page.unroute(resourceEndpoint);
  await resource().focus();
  await page.keyboard.press("Enter");
  await waitState("enabled");
  await closeResources();
  report.checks.push("failed resource writes preserve the real state and support keyboard retry without closing the editor");

  await openResources();
  await resource().click();
  await waitState("partial");
  assert.equal(await enableAll().isDisabled(), false);
  assert.equal(await closeAll().isDisabled(), false);
  await enableAll().click();
  await waitState("enabled");
  assert.equal(await resource().getAttribute("aria-checked"), "true");
  await resource().click();
  await waitState("partial");
  const bulkEndpoint = `**/api/channel/${partial.id}/status`;
  await page.route(bulkEndpoint, (route) => route.fulfill({status:503,json:{success:false,message:"Fixture bulk close failure"}}));
  await closeAll().click();
  await dialog.getByRole("alert").waitFor();
  assert.equal((await api("GET", `/api/channel/${partial.id}`)).resource_state.state, "partial");
  await page.unroute(bulkEndpoint);
  await resource().click();
  await waitState("enabled");
  assert.equal(await dialog.getByRole("alert").count(), 0);
  await resource().click();
  await waitState("partial");
  let releaseBulk, bulkWrites = 0;
  const bulkGate = new Promise((resolve) => { releaseBulk = resolve; });
  await page.route(bulkEndpoint, async (route) => { bulkWrites++; await bulkGate; await route.continue(); });
  await closeAll().click();
  await page.waitForFunction(() => document.querySelector('[data-channel-bulk="close"]')?.disabled);
  assert.equal(await enableAll().isDisabled(), true);
  assert.equal(await resource().isDisabled(), true);
  assert.equal(await dialog.locator(".task-dialog-footer button").isDisabled(), true);
  await closeAll().evaluate((button) => button.click());
  await page.keyboard.press("Escape");
  await dialog.locator(".task-dialog-header button").click();
  assert.equal(await dialog.isVisible(), true);
  assert.equal((await api("GET", `/api/channel/${partial.id}`)).resource_state.state, "partial");
  releaseBulk();
  await waitState("closed");
  assert.equal(bulkWrites, 1);
  await page.unroute(bulkEndpoint);
  await closeResources();
  await page.reload();
  await waitState("closed");
  await openResources();
  assert.equal(await resource().isDisabled(), true);
  assert.ok((await dialog.innerText()).includes("请先点击“全部启用”"));
  await page.route(bulkEndpoint, (route) => route.fulfill({status:503,json:{success:false,message:"Fixture bulk failure"}}));
  await enableAll().click();
  await dialog.getByRole("alert").waitFor();
  assert.equal((await api("GET", `/api/channel/${partial.id}`)).status, 2);
  assert.equal(await resource().isDisabled(), true);
  await page.unroute(bulkEndpoint);
  await enableAll().focus();
  await page.keyboard.press("Enter");
  await waitState("enabled");
  assert.equal(await dialog.getByRole("alert").count(), 0);
  await modelSwitch("gpt-4o").click();
  await waitState("partial");
  await resource().click();
  await waitState("closed");
  assert.equal((await api("GET", `/api/channel/${partial.id}`)).status, 1);
  assert.equal(await resource().isDisabled(), false);
  assert.equal(await closeAll().isDisabled(), false);
  await resource().click();
  await waitState("partial");
  assert.equal(await modelSwitch("gpt-4o").getAttribute("aria-checked"), "false");
  await enableAll().click();
  await waitState("enabled");
  await closeResources();
  assert.equal((await api("GET", `/api/channel/${full.id}`)).status, 1);
  assert.deepEqual((await api("GET", `/api/channel/${full.id}`)).multi_key_status, [2,1]);
  await api("PUT", `/api/channel/${full.id}/status`, {enabled:false});
  await api("PUT", `/api/channel/${full.id}/status`, {enabled:true});
  assert.deepEqual((await api("GET", `/api/channel/${full.id}`)).multi_key_status, [2,1]);
  report.checks.push("partial state exposes both bulk actions, pending writes prevent duplicates and dismissal, bulk failures support retry or individual recovery without stale errors, and all-disabled models can recover individually while a closed channel requires Enable all");

  // A revoked managed channel can retain configured models but have no published resources.
  const listEndpoint = (url) => url.pathname.replace(/\/$/, "") === "/api/channel";
  await page.route(listEndpoint, async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.data.find((entry) => entry.id === closed.id).resource_state = {state:"closed",total:0,enabled:0,resources:[]};
    await route.fulfill({response,json:body});
  });
  await page.setViewportSize({width:320,height:1000});
  await page.reload();
  await status("Closed fixture").waitFor();
  const emptyWrites = managementWrites;
  await status("Closed fixture").click();
  await dialog.waitFor();
  assert.equal(await enableAll().isDisabled(), true);
  assert.equal(await closeAll().isDisabled(), true);
  assert.equal(await dialog.getByRole("switch").count(), 0);
  assert.ok((await dialog.innerText()).includes("暂无可操作资源"));
  assert.ok(!(await dialog.innerText()).includes("请先点击“全部启用”"));
  await page.screenshot({path:path.join(out,"resources-empty-zh-light-320.png")});
  await closeResources();
  await waitForFocus("Closed fixture");
  assert.equal(managementWrites, emptyWrites);
  await page.unroute(listEndpoint);
  report.checks.push("zero-resource response fixtures remain inspectable at 320px, disable bulk writes, explain the empty state and do not suggest an unavailable action");

  await api("PUT", `/api/channel/${partial.id}/resource-status`, {model:"gpt-4o-mini",enabled:false});
  for (const [language, labels] of [["zh", ["部分启用", "关闭", "全部启用", "全部关闭", "完成"]], ["en", ["Partially enabled", "Closed", "Enable all", "Close all", "Done"]], ["ar", ["مُفعّل جزئيًا", "متوقف", "تفعيل الكل", "إيقاف الكل", "تم"]]]) {
    for (const theme of ["light", "dark"]) {
      for (const width of [1440, 320]) {
        await page.evaluate(({ language, theme }) => { localStorage.setItem("xy-language", language); localStorage.setItem("xy-theme", theme); }, { language, theme });
        await page.setViewportSize({ width, height: 1000 });
        await page.reload();
        await status("Partial fixture").waitFor();
        assert.equal(await status("Partial fixture").innerText(), labels[0] + " (1/2)");
        assert.equal(await status("Closed fixture").innerText(), labels[1]);
        await status("Partial fixture").scrollIntoViewIfNeeded();
        assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
        await page.screenshot({ path: path.join(out, `${language}-${theme}-${width}.png`) });
        await openResources();
        await resource().waitFor();
        assert.equal(await enableAll().innerText(), labels[2]);
        assert.equal(await closeAll().innerText(), labels[3]);
        assert.equal(await dialog.locator(".task-dialog-footer button").innerText(), labels[4]);
        await resource().focus();
        await page.keyboard.press("Tab");
        assert.ok(await dialog.evaluate((node) => node.contains(document.activeElement)));
        assert.ok(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth + 1));
        assert.deepEqual((await new AxeBuilder({ page }).include('[role="dialog"]').withTags(["wcag2a", "wcag2aa"]).analyze()).violations, []);
        await page.screenshot({ path: path.join(out, `resources-${language}-${theme}-${width}.png`) });
        await closeResources();
      }
    }
  }
  report.checks.push("resource counts, switches and state labels work in three languages, both themes, desktop/320px and keyboard navigation without accessibility violations");
  assert.deepEqual(report.pageErrors, []);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.failure = String(error.stack || error);
  const page = browser?.contexts()[0]?.pages()[0];
  if (page) await page.screenshot({ path: path.join(out, "failure.png") });
  throw error;
} finally {
  await writeFile(path.join(out, "report.json"), JSON.stringify(report, null, 2));
  await browser?.close();
  await vite?.close();
  backend?.kill();
  if (upstream) await new Promise((resolve) => upstream.close(resolve));
}
