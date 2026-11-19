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
  env: { ...process.env, STARHUB_CONFIG: "", STARHUB_PORT: String(backendPort), STARHUB_DB: path.join(temp, "starhub.db"), STARHUB_COOKIE_SECURE: "false", STARHUB_ROOT_PASSWORD: password,
    STARHUB_ENV: "development", STARHUB_CRYPTO_SECRET: "plugin-browser-fixture-secret",
    STARHUB_CREDENTIAL_KEYS: JSON.stringify({ fixture: "11".repeat(32) }), STARHUB_CREDENTIAL_ACTIVE_KEY: "fixture",
    RESOURCE_CATALOG_NOTIFY_URL: "", STARHUB_REGION_PROFILE: "" },
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
  for (const [route, data] of [
    ["/api/user/self", { current_password: password, new_password: "PluginChangedOnly-2026" }],
    ["/api/user/2fa/skip", undefined],
    ["/api/option", { key: "ip_rate_limit", value: "10000" }],
  ]) {
    const response = await context.request.fetch(`${base}${route}`, { method: route === "/api/user/2fa/skip" ? "POST" : "PUT", data });
    assert.equal(response.status(), 200, await response.text());
  }
  await page.goto(`${base}/admin?tab=plugins`);
  await page.locator('.task-plugins input[type="file"]').waitFor();
  const native = page.locator(".task-plugin-native");
  await native.getByText("豆包 Seedance", { exact: true }).waitFor();
  assert.equal(await native.getByRole("button").count(), 0);
  assert.equal(await native.getByRole("link", { name: "配置渠道", exact: true }).getAttribute("href"), "/admin?tab=resources&create_channel=ark#channels");
  assert.deepEqual((await (await context.request.get(`${base}/api/task-plugin`)).json()).data, []);
  report.checks.push("Doubao native integration remains discoverable without installing a plugin");

  await native.getByRole("link", { name: "配置渠道", exact: true }).focus();
  await page.keyboard.press("Enter");
  const channelDialog = page.getByRole("dialog");
  await channelDialog.waitFor();
  assert.equal(await channelDialog.locator("#ch-type").inputValue(), "32");
  assert.equal(await channelDialog.locator("#ch-plugin").count(), 0);
  assert.equal(new URL(page.url()).searchParams.has("create_channel"), false);
  assert.deepEqual((await (await context.request.get(`${base}/api/channel`)).json()).data, []);
  await channelDialog.getByRole("button", { name: "取消", exact: true }).click();
  await channelDialog.waitFor({ state: "hidden" });
  await page.waitForFunction(() => document.activeElement?.id === "channels");
  await page.reload();
  await page.locator("#channels").waitFor();
  assert.equal(await page.getByRole("dialog").count(), 0);
  await page.goto(`${base}/admin?tab=plugins`);
  await native.getByRole("link", { name: "配置渠道", exact: true }).click();
  await channelDialog.locator("#ch-name").fill("Doubao enabled");
  await channelDialog.locator("#ch-key").fill("native-test-placeholder");
  await channelDialog.locator("#ch-models").fill("doubao-seedance-2-0-260128");
  await channelDialog.locator("#ch-base-url").fill("http://127.0.0.1:1");
  await channelDialog.getByRole("button", { name: "创建", exact: true }).click();
  await channelDialog.waitFor({ state: "hidden" });
  await page.getByText("Doubao enabled", { exact: true }).waitFor();
  const createNative = async (name, input) => {
    const response = await context.request.post(`${base}/api/channel`, { data: {
      type: 32, name, models: "doubao-seedance-2-0-260128", key: "native-fixture-key", base_url: "http://127.0.0.1:1", ...input,
    } });
    assert.equal(response.status(), 200, await response.text());
    return (await response.json()).data;
  };
  const closed = await createNative("Doubao closed", { status: 2 });
  const partial = await createNative("Endpoint video", { models: "ep-video-fixture", key: "native-first\nnative-second", model_capabilities: { "ep-video-fixture": "VideoGeneration" } });
  await createNative("Ark text only", { models: "text-only-fixture" });
  let failChannels = false;
  // Present an isolated partial-key state without submitting a provider task.
  await page.route(`${base}/api/channel`, async route => {
    if (route.request().method() !== "GET") return route.continue();
    if (failChannels) return route.fulfill({ status: 503, json: { success: false, message: "Fixture unavailable" } });
    const response = await route.fetch();
    const body = await response.json();
    body.data = body.data.map(entry => entry.id === partial.id ? { ...entry, multi_key_status: [2, 1] } : entry);
    await route.fulfill({ response, json: body });
  });
  await page.goto(`${base}/admin?tab=plugins`);
  await native.locator(`[data-native-channel="${partial.id}"] dd`).getByText("部分启用", { exact: true }).waitFor();
  assert.equal(await native.locator("[data-native-channel]").count(), 3);
  assert.equal(await native.locator(`[data-native-channel="${closed.id}"] dd`).innerText(), "关闭");
  assert.equal(await native.getByText("Ark text only", { exact: true }).count(), 0);
  await native.getByRole("link", { name: "管理渠道", exact: true }).click();
  await page.waitForFunction(() => document.activeElement?.id === "channels");
  assert.equal(await page.getByRole("dialog").count(), 0);
  await page.goto(`${base}/admin?tab=plugins`);
  failChannels = true;
  await page.reload();
  await native.getByRole("alert").waitFor();
  assert.equal(await native.locator("[data-native-channel]").count(), 0);
  failChannels = false;
  await native.getByRole("button", { name: "重试", exact: true }).click();
  await native.locator(`[data-native-channel="${partial.id}"] dd`).getByText("部分启用", { exact: true }).waitFor();
  report.checks.push("native channel shortcut selects Ark once, saves only on confirmation, lists video availability, focuses channel management and retries failed reads");
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
  for (const [language, theme, width] of [["zh", "light", 1440], ["en", "dark", 320], ["ar", "light", 390]]) {
    await page.evaluate(({ language, theme }) => { localStorage.setItem("xy-language", language); localStorage.setItem("xy-theme", theme); }, { language, theme });
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`${base}/admin?tab=plugins`);
    await page.locator(".task-plugins li").waitFor();
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${language} page overflow`);
    const nativeName = `native-${language}-${theme}-${width}.png`;
    await page.locator(".task-plugin-native").scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(out, nativeName), fullPage: true });
    report.screenshots.push(nativeName);
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
