// Uses an isolated database and fixture credentials; no real SMS is sent.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const repo = path.resolve(web, "..");
const out = path.join(web, "playwright-report", "sms-configuration");
await mkdir(out, { recursive: true });
await mkdir(path.join(repo, ".p6-work"), { recursive: true });
const temp = await mkdtemp(path.join(repo, ".p6-work", "sms-browser-"));
const report = { checks: [], screenshots: [], pageErrors: [] };
const secret = "BrowserSMSFixtureSecret92";
const password = "BrowserSMSChanged94";
let backend, vite, browser;
let logs = "";
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
const backendPort = await freePort();
const backendBase = `http://127.0.0.1:${backendPort}`;
const binary = path.join(temp, process.platform === "win32" ? "starhub.exe" : "starhub");
async function startBackend(bridge = false) {
  backend = spawn(binary, [], {
    cwd: temp, windowsHide: true, stdio: ["ignore", "pipe", "pipe"],
    env: {
      ...Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("STARHUB_"))),
      STARHUB_PORT: String(backendPort), STARHUB_BIND_ADDRESS: "127.0.0.1", STARHUB_DB: path.join(temp, "starhub.db"),
      STARHUB_ENV: "development", STARHUB_ROOT_PASSWORD: "BrowserSMSInitial93", STARHUB_CRYPTO_SECRET: "browser-sms-persistent-fixture",
      STARHUB_CHANNEL_TEST_INTERVAL: "0", STARHUB_TASK_POLL_INTERVAL: "0",
      STARHUB_SMS_BRIDGE_URL: bridge ? "http://127.0.0.1:1/sms" : "", STARHUB_SMS_BRIDGE_TOKEN: bridge ? "bridge-fixture" : "",
      RESOURCE_CATALOG_NOTIFY_URL: "",
    },
  });
  for (const stream of [backend.stdout, backend.stderr]) stream.on("data", (chunk) => { logs = (logs + chunk).slice(-16000); });
  for (let i = 0; i < 100; i++) {
    if (backend.exitCode !== null) throw Error("isolated SMS server exited: " + logs);
    try { if ((await fetch(backendBase + "/api/health")).ok) return; } catch {}
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
  throw Error("isolated SMS server did not become healthy");
}
async function stopBackend() {
  if (!backend || backend.exitCode !== null) return;
  const stopped = new Promise((resolve) => backend.once("exit", resolve));
  backend.kill();
  await stopped;
}
try {
  execFileSync("go", ["build", "-o", binary, "."], { cwd: repo, stdio: "pipe" });
  await startBackend();
  vite = await createViteServer({ root: web, cacheDir: path.join(temp, "vite-cache"), logLevel: "error",
    server: { host: "127.0.0.1", port: await freePort(), proxy: { "/api": backendBase } } });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  browser = await chromium.launch({ headless: true, ...(process.platform === "win32" ? { channel: "msedge" } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: "reduce" });
  await context.addInitScript(() => localStorage.setItem("xy-language", "en"));
  const api = async (method, route, data) => {
    const response = await context.request.fetch(base + route, { method, data });
    const value = await response.json();
    assert.equal(response.status(), 200, JSON.stringify(value));
    assert.equal(value.success, true);
    assert.equal(JSON.stringify(value).includes(secret), false);
    return value.data;
  };
  await api("POST", "/api/user/login", { username: "root", password: "BrowserSMSInitial93" });
  await api("PUT", "/api/user/self", { current_password: "BrowserSMSInitial93", new_password: password });
  await api("POST", "/api/user/2fa/skip");
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on("pageerror", (error) => report.pageErrors.push(String(error)));
  await page.goto(base + "/console/settings");
  const button = page.locator('[data-account-action="sms-config"]');
  await button.scrollIntoViewIfNeeded();
  await page.locator(".ac-security-row").filter({ has: button }).screenshot({ path: path.join(out, "phone-row-desktop.png") });
  report.screenshots.push("phone-row-desktop.png");
  const dialog = page.getByRole("dialog");
  const save = () => dialog.locator('button[form="sms-config-form"]');
  const open = async () => { await button.click(); await dialog.locator("#sms-access-id").waitFor(); };
  const reload = async (expectedSign) => {
    const loaded = page.waitForResponse((response) => response.request().method() === "GET" && response.url().includes("/api/system/sms-config?"));
    await dialog.getByRole("button", { name: "Reload", exact: true }).click();
    await loaded;
    await page.waitForFunction((sign) => document.querySelector("#sms-sign-name")?.value === sign, expectedSign);
  };
  const state = () => api("GET", "/api/system/sms-config?expected_user_id=1");
  await open();
  await dialog.locator("#sms-access-id").fill("BrowserSMSAccess123");
  await dialog.locator("#sms-access-secret").fill(secret);
  await dialog.locator("#sms-sign-name").fill("SMS Fixture Sign");
  await dialog.locator("#sms-template-code").fill("SMS_123456");
  await dialog.getByRole("switch").click();
  await dialog.locator("#sms-current-password").fill("wrong-password");
  await save().click();
  await dialog.getByRole("alert").filter({ hasText: "current password" }).waitFor();
  const successToast = page.locator('[data-sonner-toast][data-type="success"]');
  assert.equal(await successToast.count(), 0);
  assert.equal((await state()).version, 0);
  await dialog.locator("#sms-access-secret").fill(secret);
  await dialog.locator("#sms-current-password").fill(password);
  await save().click();
  await dialog.getByRole("status").filter({ hasText: "saved and applied" }).waitFor();
  assert.equal((await state()).secret_configured, true);
  assert.equal((await state()).effective_enabled, true);
  await dialog.locator("#sms-access-secret").waitFor();
  assert.equal(await dialog.locator("#sms-access-secret").inputValue(), "");
  await successToast.filter({ hasText: "Saved successfully" }).waitFor();
  await page.screenshot({ path: path.join(out, "save-success-desktop.png"), mask: [page.locator('input[type="password"]')] });
  report.screenshots.push("save-success-desktop.png");
  await page.setViewportSize({ width: 390, height: 1000 });
  await successToast.waitFor();
  const toastBox = await successToast.boundingBox();
  assert.ok(toastBox.x >= 0 && toastBox.x + toastBox.width <= 391);
  await page.screenshot({ path: path.join(out, "save-success-mobile.png"), mask: [page.locator('input[type="password"]')] });
  report.screenshots.push("save-success-mobile.png");
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.keyboard.press("Escape");
  await dialog.waitFor({ state: "hidden" });
  assert.equal(await button.evaluate((element) => element === document.activeElement), true);
  await open();
  assert.equal(await dialog.locator("#sms-access-secret").inputValue(), "");
  assert.equal(await dialog.locator("#sms-sign-name").inputValue(), "SMS Fixture Sign");
  await dialog.locator("#sms-sign-name").fill("Updated Fixture Sign");
  await dialog.locator("#sms-current-password").fill(password);
  await save().click();
  await dialog.getByRole("status").filter({ hasText: "saved and applied" }).waitFor();
  assert.equal((await state()).secret_configured, true);
  report.checks.push("administrator button opens; wrong password is rejected; save applies; closing restores focus; blank secret preserves the stored credential");

  const current = await state();
  await api("PUT", "/api/system/sms-config", { expected_user_id: 1, expected_version: current.version, current_password: password,
    enabled: true, access_key_id: current.access_key_id, sign_name: "Concurrent Fixture Sign", template_code: current.template_code, access_secret: "" });
  await dialog.locator("#sms-current-password").fill(password);
  await save().click();
  await dialog.getByRole("alert").filter({ hasText: "Reload and check" }).waitFor();
  await successToast.waitFor({ state: "hidden" });
  assert.equal(await save().isDisabled(), true);
  await reload("Concurrent Fixture Sign");
  assert.equal(await dialog.locator("#sms-sign-name").inputValue(), "Concurrent Fixture Sign");
  report.checks.push("stale dialog cannot overwrite concurrent configuration; explicit reload recovers");
  report.checks.push("confirmed saves show a visible desktop/mobile success toast; password and version failures show no success toast");

  for (const [language, theme, width] of [["en", "light", 1440], ["zh", "light", 390], ["ar", "dark", 320]]) {
    await page.evaluate(async ([language, theme]) => {
      const { usePreferences } = await import("/src/lib/preferences.ts");
      usePreferences.getState().setLanguage(language); usePreferences.getState().setTheme(theme);
    }, [language, theme]);
    await page.setViewportSize({ width, height: 1000 });
    await page.evaluate(() => document.fonts.ready);
    await page.waitForFunction(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth), true);
    const box = await dialog.boundingBox();
    assert.ok(box.x >= 0 && box.x + box.width <= width + 1);
    await page.screenshot({ path: path.join(out, `${language}-${theme}-${width}.png`), mask: [page.locator('input[type="password"]')] });
    report.screenshots.push(`${language}-${theme}-${width}.png`);
  }
  const scan = await new AxeBuilder({ page }).include(".sms-config-dialog").withTags(["wcag2a", "wcag2aa"]).analyze();
  assert.deepEqual(scan.violations, []);
  report.checks.push("English, Chinese and Arabic dialogs; light and dark themes; 320px layout and dialog accessibility pass");
  await page.keyboard.press("Escape");
  await stopBackend();
  await startBackend();
  await api("POST", "/api/user/login", { username: "root", password });
  await api("POST", "/api/user/2fa/skip");
  assert.equal((await state()).effective_enabled, true);
  assert.equal((await state()).sign_name, "Concurrent Fixture Sign");
  await stopBackend();
  await startBackend(true);
  await api("POST", "/api/user/login", { username: "root", password });
  await api("POST", "/api/user/2fa/skip");
  await page.reload();
  await open();
  assert.equal(await dialog.locator("#sms-access-id").isDisabled(), true);
  assert.equal(await dialog.locator('button[form="sms-config-form"]').count(), 0);
  assert.equal((await state()).source, "environment_bridge");
  report.checks.push("configuration survives actual service restart; deployment-managed SMS is read-only");
  const memberContext = await browser.newContext();
  for (const [route, data] of [
    ["/api/user/register", { username: "smsmember", password: "SMSMemberFixture93", email: "" }],
    ["/api/user/login", { username: "smsmember", password: "SMSMemberFixture93" }],
  ]) {
    const response = await memberContext.request.post(base + route, { data });
    assert.equal(response.status(), 200, await response.text());
  }
  const memberPage = await memberContext.newPage();
  await memberPage.goto(base + "/console/settings");
  await memberPage.locator("#account-username").waitFor();
  assert.equal(await memberPage.locator('[data-account-action="sms-config"]').count(), 0);
  const forbidden = await memberContext.request.get(base + "/api/system/sms-config?expected_user_id=2");
  assert.equal(forbidden.status(), 403);
  await memberContext.close();
  report.checks.push("a real ordinary account has no SMS configuration entry and cannot call its API");
  assert.equal(logs.includes(secret), false);
  assert.deepEqual(report.pageErrors, []);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.failure = String(error.stack || error);
  throw error;
} finally {
  await writeFile(path.join(out, "report.json"), JSON.stringify(report, null, 2));
  await browser?.close();
  await vite?.close();
  await stopBackend();
}
