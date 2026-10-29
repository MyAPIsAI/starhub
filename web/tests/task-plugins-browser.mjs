// Uses an isolated backend and database; no real provider requests are sent.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const project = path.resolve(web, "..");
const out = path.join(web, "test-results", "p23");
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-plugins-"));
const binary = path.join(out, process.platform === "win32" ? "plugin-test.exe" : "plugin-test");
execFileSync("go", ["build", "-o", binary, "."], { cwd: project, stdio: "pipe" });
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
const backendPort = await freePort();
const backend = `http://127.0.0.1:${backendPort}`;
const password = "PluginTestOnly-2026";
const processHandle = spawn(binary, [], {
  cwd: temp,
  env: { ...process.env, STARHUB_CONFIG: "", STARHUB_PORT: String(backendPort), STARHUB_DB: path.join(temp, "starhub.db"), STARHUB_COOKIE_SECURE: "false", STARHUB_ROOT_PASSWORD: password },
  stdio: ["ignore", "ignore", "pipe"],
  windowsHide: true,
});
let logs = "";
processHandle.stderr.on("data", (data) => { logs += data; });
let vite, browser;
const report = { checks: [], screenshots: [], errors: [] };
try {
  let ready = false;
  for (let i = 0; i < 60; i++) {
    try { ready = (await fetch(`${backend}/api/plaza/models`)).ok; } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, "isolated backend did not start");
  const port = await freePort();
  vite = await createViteServer({ root: web, server: { port, host: "127.0.0.1", proxy: { "/api": backend, "/v1": backend } }, logLevel: "error" });
  await vite.listen();
  const base = `http://127.0.0.1:${port}`;
  browser = await chromium.launch({ headless: true, channel: process.env.P23_BROWSER_CHANNEL || "chrome" });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  page.on("pageerror", (error) => report.errors.push(String(error)));
  const login = await context.request.post(`${base}/api/user/login`, { data: { username: "root", password } });
  assert.equal(login.status(), 200);
  assert.equal((await login.json()).success, true);
  await page.goto(`${base}/admin?tab=plugins`);
  await page.locator('.task-plugins input[type="file"]').waitFor();
  await page.locator('.task-plugins input[type="file"]').setInputFiles(path.join(project, "plugins/tasks/openai-video.js"));
  await page.locator(".task-plugins li").filter({ hasText: "openai-video" }).waitFor();
  report.checks.push("upload and list immutable plugin version");
  const listResponse = await context.request.get(`${base}/api/task-plugin`);
  const plugin = (await listResponse.json()).data[0];
  await page.getByRole("button", { name: "解析测试", exact: true }).click();
  await page.getByRole("button", { name: "运行测试", exact: true }).click();
  await page.locator('[role="dialog"] pre').waitFor();
  await page.waitForFunction(() => Number(getComputedStyle(document.querySelector('[role="dialog"]')).opacity) >= 0.99);
  const result = JSON.parse(await page.locator('[role="dialog"] pre').innerText());
  assert.equal(result.request.path, "/v1/videos");
  assert.equal(result.usage.seconds, 4);
  await page.screenshot({ path: path.join(out, "transform-desktop.png"), fullPage: true });
  report.screenshots.push("transform-desktop.png");
  await page.keyboard.press("Escape");
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  assert.equal(await page.getByRole("button", { name: "解析测试", exact: true }).evaluate((el) => el === document.activeElement), true);
  report.checks.push("local transform preview and keyboard focus return");
  await page.getByRole("button", { name: "禁用", exact: true }).click();
  await page.getByRole("button", { name: "启用", exact: true }).waitFor();
  await page.getByRole("button", { name: "启用", exact: true }).click();
  await page.getByRole("button", { name: "禁用", exact: true }).waitFor();
  report.checks.push("disable and enable version");
  const changed = (await readFile(path.join(project, "plugins/tasks/openai-video.js"), "utf8")) + "\n// Different source under the same version.\n";
  const conflict = await context.request.post(`${base}/api/task-plugin`, { data: { source: changed } });
  assert.equal(conflict.status(), 409);
  report.checks.push("reject source overwrite");
  await page.goto(`${base}/admin?tab=resources`);
  await page.getByRole("button", { name: "新建渠道", exact: true }).click();
  await page.getByLabel("渠道类型", { exact: true }).selectOption("61");
  await page.getByLabel("插件版本", { exact: true }).selectOption(String(plugin.id));
  await page.waitForFunction(() => Number(getComputedStyle(document.querySelector('[role="dialog"]')).opacity) >= 0.99);
  await page.screenshot({ path: path.join(out, "channel-plugin-binding.png"), fullPage: true });
  report.screenshots.push("channel-plugin-binding.png");
  await page.locator("#ch-name").fill("Plugin browser channel");
  await page.locator("#ch-key").fill("test-upstream-placeholder");
  await page.locator("#ch-models").fill("sora-2");
  await page.locator("#ch-base-url").fill("http://127.0.0.1:1");
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page.getByText("Plugin browser channel", { exact: true }).waitFor();
  const channels = (await (await context.request.get(`${base}/api/channel`)).json()).data;
  const bound = channels.find((item) => item.name === "Plugin browser channel");
  assert.equal(bound.task_plugin_id, plugin.id);
  const probe = await context.request.post(`${base}/api/channel/test/${bound.id}`);
  assert.equal((await probe.json()).data.ok, true);
  report.checks.push("bind a fixed plugin version using channel form");
  for (const [language, theme, width] of [["zh", "light", 1440], ["en", "dark", 390], ["ar", "light", 390]]) {
    await page.evaluate(({ language, theme }) => { localStorage.setItem("xy-language", language); localStorage.setItem("xy-theme", theme); }, { language, theme });
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`${base}/admin?tab=plugins`);
    await page.locator(".task-plugins li").waitFor();
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${language} page overflow`);
    await page.locator(".task-plugins li button").last().click();
    await page.getByRole("dialog").waitFor();
    await page.waitForFunction(() => Number(getComputedStyle(document.querySelector('[role="dialog"]')).opacity) >= 0.99);
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${language} dialog overflow`);
    const name = `plugins-${language}-${theme}-${width}.png`;
    await page.screenshot({ path: path.join(out, name), fullPage: true });
    report.screenshots.push(name);
    await page.keyboard.press("Escape");
  }
  report.checks.push("desktop, narrow screen, light/dark, Chinese/English/Arabic and dialogs");
  const anonymous = await browser.newContext();
  assert.equal((await anonymous.request.get(`${base}/api/task-plugin`)).status(), 401);
  await anonymous.request.post(`${base}/api/user/register`, { data: { username: "plugin-reader", password: "ReaderTestOnly2026" } });
  await anonymous.request.post(`${base}/api/user/login`, { data: { username: "plugin-reader", password: "ReaderTestOnly2026" } });
  assert.equal((await anonymous.request.post(`${base}/api/task-plugin`, { data: { source: changed } })).status(), 403);
  report.checks.push("anonymous and ordinary users cannot manage plugins");
  assert.deepEqual(report.errors, []);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.errors.push(String(error));
  console.error(error);
  process.exitCode = 1;
} finally {
  await writeFile(path.join(out, "report.json"), JSON.stringify(report, null, 2));
  await writeFile(path.join(out, "backend.log"), logs);
  await browser?.close();
  await vite?.close();
  processHandle.kill();
}
