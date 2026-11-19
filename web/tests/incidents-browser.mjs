// All publication checks use an isolated server and temporary database.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-incidents-"));
const out = path.join(web, "test-results", "incidents");
await mkdir(out, { recursive: true });
const report = { checks: [], errors: [] };
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
      STARHUB_ROOT_PASSWORD: "IncidentInitial92", STARHUB_ENV: "development",
      STARHUB_CRYPTO_SECRET: "incident-browser-fixture-secret",
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
    if (!localStorage.getItem("xy-language")) localStorage.setItem("xy-language", "zh");
  });
  const api = async (method, route, data) => {
    const response = await context.request.fetch(base + route, { method, data });
    assert.equal(response.status(), 200, await response.text());
    const body = await response.json();
    assert.equal(body.success, true);
    return body.data;
  };
  await api("POST", "/api/user/login", { username: "root", password: "IncidentInitial92" });
  await api("PUT", "/api/user/self", { current_password: "IncidentInitial92", new_password: "IncidentChanged93" });
  await api("POST", "/api/user/2fa/skip");
  await api("PUT", "/api/option", { key: "ip_rate_limit", value: "10000" });
  const page = await context.newPage();
  page.setDefaultTimeout(30000);
  page.on("pageerror", (error) => report.errors.push(String(error)));
  await page.goto(base + "/admin?tab=incidents");
  assert.equal((await api("GET", "/api/incident") ?? []).length, 0);
  await page.getByRole("button", { name: "发布新公告", exact: true }).click();
  const dialog = page.getByRole("dialog");
  const title = "Persisted incident <fixture>";
  const description = "Description & workaround from the database.";
  await page.locator("#admin-incident-title").fill(` ${title} `);
  await page.locator("#admin-incident-content").fill(` ${description} `);
  await page.locator("#admin-incident-vendor").selectOption("Google");
  const submit = page.locator('button[form="admin-incident-form"]');
  let mode = "failure", writes = 0, release;
  const gate = new Promise((resolve) => { release = resolve; });
  await page.route(base + "/api/incident", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    writes++;
    if (mode === "failure") return route.fulfill({ status: 503, json: { success: false, message: "Fixture failure" } });
    await gate;
    await route.continue();
  });
  await submit.click();
  await dialog.getByRole("alert").waitFor();
  assert.equal(await page.locator("#admin-incident-title").inputValue(), ` ${title} `);
  assert.equal(await page.locator("#admin-incident-content").inputValue(), ` ${description} `);
  assert.equal((await api("GET", "/api/incident") ?? []).length, 0);
  report.checks.push("failed publication preserves the draft and never inserts a local-only announcement");
  mode = "success";
  await submit.click();
  await page.waitForFunction(() => document.querySelector('button[form="admin-incident-form"]')?.disabled);
  assert.equal(await dialog.isVisible(), true);
  assert.equal(await page.locator("#admin-incident-title").isDisabled(), true);
  await page.locator("#admin-incident-form").evaluate((form) => form.requestSubmit());
  assert.equal(writes, 2);
  assert.equal((await api("GET", "/api/incident") ?? []).length, 0);
  release();
  await dialog.waitFor({ state: "hidden" });
  await page.getByRole("heading", { name: title, exact: true }).waitFor();
  const persisted = await api("GET", "/api/incident");
  assert.equal(persisted.length, 1);
  const incident = persisted[0];
  assert.equal(incident.title, title);
  assert.equal(incident.description, description);
  assert.equal(incident.component, "Google");
  assert.equal(incident.status, "investigating");
  const card = () => page.locator(`[data-incident-id="${incident.id}"]`);
  await page.reload();
  await card().getByRole("heading", { name: title, exact: true }).waitFor();
  await page.goto(base + "/admin?tab=customers");
  await page.goto(base + "/admin?tab=incidents");
  await card().getByText(description, { exact: true }).waitFor();
  assert.equal((await api("GET", "/api/incident")).length, 1);
  report.checks.push("publication waits for server success, prevents duplicate submissions and survives refresh and navigation");

  const statusLabels = {
    zh: { investigating: "调查中", identified: "已定位", monitoring: "监测中", resolved: "已解决" },
    en: { investigating: "Investigating", identified: "Identified", monitoring: "Monitoring", resolved: "Resolved" },
    ar: { investigating: "قيد التحقيق", identified: "تم تحديد السبب", monitoring: "قيد المراقبة", resolved: "تم الحل" },
  };
  for (const status of ["identified", "monitoring", "resolved"]) {
    const entry = await api("POST", "/api/incident", {
      title: `${status} announcement`, description: "First line.\nSecond line & workaround.", component: "OpenAI",
    });
    await api("PUT", `/api/incident/${entry.id}`, { status, message: "Fixture status" });
  }
  const records = await api("GET", "/api/incident");
  for (const [language, theme, width] of [["zh", "light", 1440], ["en", "dark", 320], ["ar", "light", 390]]) {
    await page.evaluate(({ language, theme }) => { localStorage.setItem("xy-language", language); localStorage.setItem("xy-theme", theme); }, { language, theme });
    await page.setViewportSize({ width, height: 1000 });
    for (const [view, target] of [["admin", "/admin?tab=incidents"], ["public", "/status"]]) {
      await page.goto(base + target);
      await card().getByText(description, { exact: true }).waitFor();
      for (const record of records) {
        const item = page.locator(`[data-incident-id="${record.id}"]`);
        assert.equal(await item.locator("[data-incident-status]").innerText(), statusLabels[language][record.status]);
        assert.equal(await item.locator("time").getAttribute("datetime"), new Date(record.updated_at * 1000).toISOString());
        assert.ok((await item.locator("time").innerText()).endsWith(" UTC"));
      }
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${view} ${language} page overflow`);
      await page.screenshot({ path: path.join(out, `${view}-${language}-${theme}-${width}.png`), animations: "disabled", timeout: 30000 });
    }
  }
  report.checks.push("both pages localize all four statuses and show real UTC timestamps in three languages, both themes and narrow layouts");

  await page.evaluate(() => { localStorage.setItem("xy-language", "zh"); localStorage.setItem("xy-theme", "light"); });
  await page.setViewportSize({ width: 390, height: 1000 });
  for (const [view, target, endpoint] of [["admin", "/admin?tab=incidents", "/api/incident"], ["public", "/status", "/api/status/incidents"]]) {
    let readMode = "loading", releaseRead;
    const readGate = new Promise((resolve) => { releaseRead = resolve; });
    await page.route(base + endpoint, async (route) => {
      if (route.request().method() !== "GET") return route.continue();
      if (readMode === "loading") await readGate;
      if (readMode === "error") return route.fulfill({ status: 503, json: { success: false, message: "Fixture unavailable" } });
      if (readMode === "empty") return route.fulfill({ json: { success: true, data: null } });
      await route.continue();
    });
    await page.goto(base + target);
    const list = page.locator(".incident-list");
    await list.getByRole("status").getByText("正在读取公告…", { exact: true }).waitFor();
    assert.equal(await list.locator("[data-incident-id]").count(), 0);
    releaseRead();
    await card().waitFor();
    readMode = "error";
    await page.reload();
    await list.getByRole("alert").waitFor();
    assert.equal(await list.locator("[data-incident-id]").count(), 0);
    if (view === "public") {
      await page.getByRole("heading", { name: "服务状态待确认", exact: true }).waitFor();
      assert.equal(await page.locator("[data-status-state]").getAttribute("data-status-state"), "unknown");
    }
    await page.screenshot({ path: path.join(out, `${view}-error.png`), animations: "disabled", timeout: 30000 });
    readMode = "live";
    await list.getByRole("button", { name: "重试", exact: true }).focus();
    await page.keyboard.press("Enter");
    await card().waitFor();
    readMode = "empty";
    await page.reload();
    await list.getByRole("status").getByText("暂无公告。", { exact: true }).waitFor();
    assert.equal(await list.getByRole("alert").count(), 0);
    await page.screenshot({ path: path.join(out, `${view}-empty.png`), animations: "disabled", timeout: 30000 });
    await page.unroute(base + endpoint);
  }
  report.checks.push("both pages distinguish loading, empty and failed reads, support keyboard retry and never show demo announcements on failure");
  const anonymous = await browser.newContext();
  const publicResponse = await anonymous.request.get(base + "/api/status/incidents");
  assert.equal(publicResponse.status(), 200);
  assert.ok((await publicResponse.json()).data.some((entry) => entry.id === incident.id));
  const publicPage = await anonymous.newPage();
  publicPage.setDefaultTimeout(30000);
  publicPage.on("pageerror", (error) => report.errors.push(String(error)));
  const publicRequests = [];
  publicPage.on("request", (request) => publicRequests.push(new URL(request.url()).pathname));
  await publicPage.goto(base + "/status");
  await publicPage.locator(`[data-incident-id="${incident.id}"]`).getByText(description, { exact: true }).waitFor();
  assert.equal(await publicPage.locator("[data-incident-id]").count(), records.length);
  assert.equal(publicRequests.includes("/api/incident"), false);
  await publicPage.reload();
  await publicPage.locator(`[data-incident-id="${incident.id}"]`).getByText(title, { exact: true }).waitFor();
  await api("PUT", `/api/incident/${incident.id}`, { status: "resolved", message: "Fixture resolved" });
  await publicPage.reload();
  await publicPage.locator(`[data-incident-id="${incident.id}"] [data-incident-status="resolved"]`).waitFor();
  await publicPage.screenshot({ path: path.join(out, "public-anonymous.png"), animations: "disabled", timeout: 30000 });
  report.checks.push("the anonymous status page reads the persisted announcement and recent resolution through the public API");
  assert.equal((await anonymous.request.get(base + "/api/incident")).status(), 401);
  assert.equal((await anonymous.request.post(base + "/api/incident", { data: { title: "Denied" } })).status(), 401);
  await anonymous.request.post(base + "/api/user/register", { data: { username: "incident-reader", password: "IncidentReader93" } });
  await anonymous.request.post(base + "/api/user/login", { data: { username: "incident-reader", password: "IncidentReader93" } });
  assert.equal((await anonymous.request.post(base + "/api/incident", { data: { title: "Denied" } })).status(), 403);
  report.checks.push("anonymous and ordinary users cannot publish announcements");
  assert.deepEqual(report.errors, []);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.failure = String(error.stack || error);
  const page = browser?.contexts()[0]?.pages()[0];
  if (page) await page.screenshot({ path: path.join(out, "failure.png"), animations: "disabled", timeout: 30000 }).catch(() => {});
  throw error;
} finally {
  await writeFile(path.join(out, "report.json"), JSON.stringify(report, null, 2));
  await browser?.close();
  await vite?.close();
  backend?.kill();
}
