// Uses an isolated database and a loopback SMS bridge. No real SMS is sent.
import { createServer as createViteServer } from "vite";
import { chromium, expect } from "playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { createHmac } from "node:crypto";
import { fileURLToPath } from "node:url";
import path from "node:path";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const repo = path.resolve(web, "..");
const out = path.join(web, "playwright-report", "phone-authentication");
await mkdir(out, { recursive: true });
await mkdir(path.join(repo, ".p6-work"), { recursive: true });
const temp = await mkdtemp(path.join(repo, ".p6-work", "phone-browser-"));
const binary = path.join(temp, process.platform === "win32" ? "starhub.exe" : "starhub");
const report = { checks: [], screenshots: [], pageErrors: [] };
const password = "PhoneBrowserFixture93";
const phone = "13812345678";
const codes = new Map();
let lastSend = 0;
let backend, vite, browser, bridge;
let logs = "";
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
function totp(secret) {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const char of secret.replace(/=+$/, "")) bits += alphabet.indexOf(char).toString(2).padStart(5, "0");
  const key = Buffer.from(bits.match(/.{8}/g).map((byte) => parseInt(byte, 2)));
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000)));
  const hash = createHmac("sha1", key).update(counter).digest();
  const offset = hash[hash.length - 1] & 15;
  return String((hash.readUInt32BE(offset) & 0x7fffffff) % 1000000).padStart(6, "0");
}
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function waitForSendWindow() {
  const delay = lastSend + 61000 - Date.now();
  if (delay > 0) await sleep(delay);
}
try {
  bridge = createServer(async (request, response) => {
    assert.equal(request.headers.authorization, "Bearer phone-browser-fixture");
    let body = ""; for await (const chunk of request) body += chunk;
    const message = JSON.parse(body);
    assert.equal(message.kind, "sms_verification");
    codes.set(message.to, message.code); lastSend = Date.now();
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(JSON.stringify({ accepted: true, id: "phone-fixture-accepted" }));
  });
  await new Promise((resolve) => bridge.listen(0, "127.0.0.1", resolve));
  execFileSync("go", ["build", "-o", binary, "."], { cwd: repo, stdio: "pipe" });
  const backendPort = await freePort();
  const backendBase = `http://127.0.0.1:${backendPort}`;
  backend = spawn(binary, [], { cwd: temp, windowsHide: true, env: {
    ...Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("STARHUB_"))),
    STARHUB_PORT: String(backendPort), STARHUB_BIND_ADDRESS: "127.0.0.1", STARHUB_DB: path.join(temp, "starhub.db"),
    STARHUB_ENV: "development", STARHUB_ROOT_PASSWORD: "PhoneRootFixture92", STARHUB_CRYPTO_SECRET: "phone-browser-persistent-fixture",
    STARHUB_CHANNEL_TEST_INTERVAL: "0", STARHUB_TASK_POLL_INTERVAL: "0", RESOURCE_CATALOG_NOTIFY_URL: "",
    STARHUB_SMS_BRIDGE_URL: `http://127.0.0.1:${bridge.address().port}/sms`, STARHUB_SMS_BRIDGE_TOKEN: "phone-browser-fixture",
  } });
  for (const stream of [backend.stdout, backend.stderr]) stream.on("data", (chunk) => { logs = (logs + chunk).slice(-16000); });
  let healthy = false;
  for (let i = 0; i < 100; i++) {
    if (backend.exitCode != null) throw Error("isolated phone server exited");
    try { if ((await fetch(backendBase + "/api/health")).ok) { healthy = true; break; } } catch {}
    await sleep(150);
  }
  assert.equal(healthy, true);
  vite = await createViteServer({ root: web, cacheDir: path.join(temp, "vite-cache"), logLevel: "error",
    server: { host: "127.0.0.1", port: await freePort(), proxy: { "/api": backendBase } } });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  browser = await chromium.launch({ headless: true, ...(process.platform === "win32" ? { channel: "msedge" } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: "reduce" });
  await context.addInitScript(() => localStorage.setItem("xy-language", "en"));
  const api = async (method, route, data) => {
    const response = await context.request.fetch(base + route, { method, data });
    const value = await response.json(); assert.equal(response.status(), 200, JSON.stringify(value)); assert.equal(value.success, true);
    return value.data;
  };
  await api("POST", "/api/user/register", { username: "phonebrowser", password, email: "" });
  await api("POST", "/api/user/login", { username: "phonebrowser", password });
  const self = await api("GET", "/api/user/self");
  const page = await context.newPage(); page.setDefaultTimeout(20000);
  page.on("pageerror", (error) => report.pageErrors.push(String(error)));
  await page.goto(base + "/console/settings");
  const link = page.locator('[data-account-action="phone"]');
  await link.waitFor(); await link.click();
  const dialog = page.locator(".phone-binding-dialog");
  await dialog.locator("#phone-binding-password").fill("wrong-password");
  await dialog.locator("#bind-phone-number").fill(phone);
  await dialog.getByRole("button", { name: "Get code", exact: true }).click();
  await dialog.getByRole("alert").filter({ hasText: "current password" }).waitFor();
  assert.equal(codes.size, 0);
  const errorScan = await new AxeBuilder({ page }).include(".phone-binding-dialog").withTags(["wcag2a", "wcag2aa"]).analyze();
  assert.deepEqual(errorScan.violations, []);
  await dialog.locator("#phone-binding-password").fill(password);
  await dialog.getByRole("button", { name: "Get code", exact: true }).click();
  await dialog.getByRole("status").filter({ hasText: "Request accepted" }).waitFor();
  assert.equal(await dialog.getByRole("button", { name: /Retry in/ }).isDisabled(), true);
  await dialog.locator("#bind-phone-number").fill("13912345678");
  assert.equal(await dialog.getByRole("button", { name: "Get code", exact: true }).isEnabled(), true);
  await dialog.locator("#bind-phone-number").fill("+86 " + phone);
  assert.equal(await dialog.getByRole("button", { name: /Retry in/ }).isDisabled(), true);
  await dialog.locator("#bind-phone-number").fill("0086" + phone);
  assert.equal(await dialog.getByRole("button", { name: /Retry in/ }).isDisabled(), true);
  await dialog.locator("#bind-phone-number").fill(phone);
  const capture = async (name, target = page) => {
    await page.evaluate(() => document.fonts.ready);
    await page.waitForFunction(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);
    await target.screenshot({ path: path.join(out, name), mask: [page.locator('input[type="password"]'), page.locator('#bind-phone-number'), page.locator('#bind-phone-code'), page.locator('#auth-phone-number'), page.locator('#auth-phone-code'), page.locator('#auth-factor')] });
    report.screenshots.push(name);
  };
  await capture("binding-en-desktop.png");
  for (const [language, theme, width] of [["zh", "light", 390], ["ar", "dark", 320]]) {
    await page.evaluate(async ([language, theme]) => {
      const { usePreferences } = await import("/src/lib/preferences.ts");
      usePreferences.getState().setLanguage(language); usePreferences.getState().setTheme(theme);
    }, [language, theme]);
    await page.setViewportSize({ width, height: 1000 });
    await capture(`binding-${language}-${width}.png`);
  }
  const bindingScan = await new AxeBuilder({ page }).include(".phone-binding-dialog").withTags(["wcag2a", "wcag2aa"]).analyze();
  assert.deepEqual(bindingScan.violations, []);
  await page.evaluate(async () => { const { usePreferences } = await import("/src/lib/preferences.ts"); usePreferences.getState().setLanguage("en"); usePreferences.getState().setTheme("light"); });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await dialog.locator("#bind-phone-code").fill(codes.get("+86" + phone));
  await dialog.getByRole("button", { name: "Confirm link", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  await page.locator(".ac-events").getByText("Phone linked", { exact: true }).waitFor();
  const state = await api("GET", `/api/user/phone?expected_user_id=${self.id}`);
  assert.equal(state.bound, true); assert.equal(state.masked_phone.includes(phone), false);
  assert.equal(await page.locator('[data-account-action="phone"]').count(), 0);
  report.checks.push("wrong password sends no SMS; verified first binding succeeds; masked phone and security history refresh; countdown prevents immediate resend");
  await api("POST", "/api/user/logout");
  await page.goto(base + "/login?method=phone");
  await page.locator("#auth-phone-number").waitFor();
  const assertLoginCodeAlignment = async () => {
    await expect.poll(() => page.locator(".auth-dialog .phone-code-input").evaluate((row) => {
      const input = row.querySelector("input").getBoundingClientRect();
      const button = row.querySelector("button").getBoundingClientRect();
      return {
        inputHeight: Math.round(input.height),
        buttonHeight: Math.round(button.height),
        aligned: Math.abs(input.y - button.y) < 1,
      };
    })).toEqual({ inputHeight: 44, buttonHeight: 44, aligned: true });
  };
  await expect(page.locator(".phone-code-guidance")).toContainText("New users should register first");
  await assertLoginCodeAlignment();
  const unknownPhone = "13712345678";
  const dispatchedBeforeUnknown = codes.size;
  let releaseUnknown;
  const unknownGate = new Promise((resolve) => { releaseUnknown = resolve; });
  await page.route("**/api/user/phone-verification", async (route) => {
    await unknownGate;
    await route.continue();
  });
  await page.locator("#auth-phone-number").fill(unknownPhone);
  await page.getByRole("button", { name: "Get code", exact: true }).click();
  await expect(page.getByRole("button", { name: "Requesting…", exact: true })).toBeDisabled();
  await assertLoginCodeAlignment();
  const unknownAcknowledgement = page.waitForResponse((response) => response.url().endsWith("/api/user/phone-verification"));
  releaseUnknown();
  const unknownResponse = await unknownAcknowledgement;
  assert.equal(unknownResponse.status(), 200, await unknownResponse.text());
  await expect(page.locator(".phone-code-notice")).toContainText("does not confirm SMS was sent");
  await page.unroute("**/api/user/phone-verification");
  assert.equal(codes.has("+86" + unknownPhone), false);
  assert.equal(codes.size, dispatchedBeforeUnknown);
  await expect(page.getByRole("button", { name: /Retry in/ })).toBeDisabled();
  await assertLoginCodeAlignment();
  report.checks.push("unlinked login numbers show registration/linking guidance and an accurate acknowledgement without dispatching SMS; idle, requesting and cooldown controls stay aligned at 44px");
  for (const [language, theme, width] of [["en", "light", 1440], ["zh", "light", 390], ["ar", "dark", 320]]) {
    await page.evaluate(async ([language, theme]) => { const { usePreferences } = await import("/src/lib/preferences.ts"); usePreferences.getState().setLanguage(language); usePreferences.getState().setTheme(theme); }, [language, theme]);
    await page.setViewportSize({ width, height: 1000 });
    await assertLoginCodeAlignment();
    await capture(`login-${language}-${width}.png`);
  }
  const loginScan = await new AxeBuilder({ page }).include(".auth-dialog").withTags(["wcag2a", "wcag2aa"]).analyze();
  assert.deepEqual(loginScan.violations, []);
  await page.evaluate(async () => { const { usePreferences } = await import("/src/lib/preferences.ts"); usePreferences.getState().setLanguage("en"); usePreferences.getState().setTheme("light"); });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.locator("#auth-phone-number").fill(phone);
  await page.locator("#auth-phone-code").fill("111111");
  await page.locator("#auth-consent").check();
  await page.locator(".auth-submit").click();
  await page.locator(".auth-error").filter({ hasText: "invalid" }).waitFor();
  await waitForSendWindow();
  await page.getByRole("button", { name: "Get code", exact: true }).click();
  await page.locator(".phone-code-notice").waitFor();
  await page.locator("#auth-phone-code").fill(codes.get("+86" + phone));
  await page.locator(".auth-submit").click();
  await page.waitForURL("**/console");
  assert.equal((await api("GET", "/api/user/self")).id, self.id);
  report.checks.push("SMS login rejects invalid code, accepts bridge-delivered code, restores the real server session and opens console");
  const factor = await api("POST", "/api/user/2fa/setup", { password });
  await api("POST", "/api/user/2fa/enable", { code: totp(factor.secret) });
  await api("POST", "/api/user/logout");
  await page.goto(base + "/login?method=phone");
  await page.locator("#auth-phone-number").fill(phone);
  await waitForSendWindow();
  await page.getByRole("button", { name: "Get code", exact: true }).click();
  await page.locator(".phone-code-notice").waitFor();
  await page.locator("#auth-phone-code").fill(codes.get("+86" + phone));
  await page.locator("#auth-consent").check();
  await page.locator(".auth-submit").click();
  await page.locator("#auth-factor").waitFor();
  const beforeFactor = await context.request.get(base + "/api/user/self");
  assert.equal(beforeFactor.status(), 401);
  await capture("login-second-factor.png");
  await page.locator("#auth-factor").fill(totp(factor.secret));
  await page.locator(".auth-submit").click();
  await page.waitForURL("**/console");
  assert.equal((await api("GET", "/api/user/self")).id, self.id);
  report.checks.push("SMS first factor does not establish a session for an MFA account; the canonical username and protected challenge complete TOTP login");
  await api("POST", "/api/user/logout");
  await page.route("**/api/public/region", (route) => route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ success: false, message: "fixture unavailable" }) }));
  await page.goto(base + "/login?method=phone");
  await page.getByText("SMS sign-in availability could not be loaded. Refresh to retry.", { exact: true }).waitFor();
  assert.equal(await page.locator("#auth-phone-number").count(), 0);
  await page.unroute("**/api/public/region");
  await page.getByRole("button", { name: "Refresh availability", exact: true }).click();
  await page.locator("#auth-phone-number").waitFor();
  let release, received;
  const responseReady = new Promise((resolve) => { received = resolve; });
  const resume = new Promise((resolve) => { release = resolve; });
  await page.route("**/api/user/phone-verification", async (route) => {
    const response = await route.fetch(); received(); await resume; await route.fulfill({ response });
  });
  await page.locator("#auth-phone-number").fill("13912345678");
  await page.getByRole("button", { name: "Get code", exact: true }).click();
  await responseReady;
  await page.locator(".auth-methods").getByRole("button", { name: "Password", exact: true }).click();
  release();
  await page.locator("#auth-account").waitFor();
  assert.equal(await page.locator(".phone-code-notice").count(), 0);
  await page.unroute("**/api/user/phone-verification");
  await page.locator(".auth-methods").getByRole("button", { name: "Phone code", exact: true }).click();
  await page.locator("#auth-phone-number").waitFor();
  assert.equal(await page.locator("#auth-phone-number").inputValue(), "");
  assert.equal(await page.locator("#auth-phone-code").inputValue(), "");
  assert.equal(await page.locator(".phone-code-notice").count(), 0);
  report.checks.push("switching login method clears phone/code and ignores a delayed send response");
  await page.route("**/api/user/phone-verification", (route) => route.fulfill({ status: 429,
    headers: { "Content-Type": "application/json", "Retry-After": "3547" },
    body: JSON.stringify({ success: false, message: "fixture hourly limit", retry_after: 3547 }),
  }));
  await page.locator("#auth-phone-number").fill(phone);
  await page.getByRole("button", { name: "Get code", exact: true }).click();
  const hourlyRetry = page.getByRole("button", { name: /Retry in 35\d\ds/ });
  await hourlyRetry.waitFor();
  assert.equal(await hourlyRetry.isDisabled(), true);
  await page.locator("#auth-phone-number").fill("13712345678");
  assert.equal(await page.getByRole("button", { name: "Get code", exact: true }).isEnabled(), true);
  await page.locator("#auth-phone-number").fill("+86" + phone);
  await hourlyRetry.waitFor();
  assert.equal(await hourlyRetry.isDisabled(), true);
  for (const [language, theme, width] of [["en", "light", 1440], ["zh", "light", 390], ["ar", "dark", 320]]) {
    await page.evaluate(async ([language, theme]) => { const { usePreferences } = await import("/src/lib/preferences.ts"); usePreferences.getState().setLanguage(language); usePreferences.getState().setTheme(theme); }, [language, theme]);
    await page.setViewportSize({ width, height: 1000 });
    await assertLoginCodeAlignment();
    await capture(`hourly-retry-${language}-${width}.png`);
  }
  await page.evaluate(async () => { const { usePreferences } = await import("/src/lib/preferences.ts"); usePreferences.getState().setLanguage("en"); usePreferences.getState().setTheme("light"); });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.reload();
  await page.locator("#auth-phone-number").fill(phone);
  await page.getByRole("button", { name: "Get code", exact: true }).click();
  await hourlyRetry.waitFor();
  await page.unroute("**/api/user/phone-verification");
  report.checks.push("hourly Retry-After reaches the countdown; another number has its own timer; China aliases restore the existing timer; reload recovers the server interval without local phone storage");
  await page.route("**/api/public/region", async (route) => {
    const response = await route.fetch(); const value = await response.json(); value.data.phone_login_enabled = false;
    await route.fulfill({ response, json: value });
  });
  await page.reload();
  await page.getByText("SMS sign-in is not enabled for this region. Use password login.", { exact: true }).waitFor();
  assert.equal(await page.locator("#auth-phone-number").count(), 0);
  report.checks.push("unavailable capability fails closed with retry; disabled capability shows an accurate password fallback; Chinese/English/Arabic desktop/mobile and modal accessibility pass");
  assert.deepEqual(report.pageErrors, []);
  assert.equal(logs.includes(password), false);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.failure = String(error.stack || error); throw error;
} finally {
  await writeFile(path.join(out, "report.json"), JSON.stringify(report, null, 2));
  await browser?.close(); await vite?.close();
  if (backend && backend.exitCode == null) { const stopped = new Promise((resolve) => backend.once("exit", resolve)); backend.kill(); await stopped; }
  if (bridge) await new Promise((resolve) => bridge.close(resolve));
}
