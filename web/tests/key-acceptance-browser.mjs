// Uses isolated application processes, a temporary database and synthetic credentials.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const project = path.resolve(web, "..");
const out = path.join(project, "output", "key-acceptance-check");
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-"));
const binary = path.join(out, process.platform === "win32" ? "key-check.exe" : "key-check");
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], { cwd: project, stdio: "pipe" });
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
const backendPort = await freePort();
const backend = `http://127.0.0.1:${backendPort}`;
const password = "KeyAcceptanceOnly2026";
const child = spawn(binary, [], {
  cwd: temp,
  env: { ...process.env, STARHUB_CONFIG: "", STARHUB_PORT: String(backendPort), STARHUB_DB: path.join(temp, "starhub.db"), STARHUB_COOKIE_SECURE: "false", STARHUB_ROOT_PASSWORD: password },
  stdio: ["ignore", "ignore", "pipe"], windowsHide: true,
});
let backendLogs = "";
child.stderr.on("data", (data) => { backendLogs += data; });
let vite, browser;
const report = { checks: [], screenshots: [], errors: [], networkErrors: [] };
const words = {
  zh: { create: "创建密钥", allowed: "明确授权的模型", candidates: "智能候选模型", empty: "开启智能入口时须选择至少一个已授权的文本模型。", close: "关闭" },
  en: { create: "Create key", allowed: "Explicitly allowed models", candidates: "Smart candidate models", empty: "Select at least one authorized text model for smart routing.", close: "Close" },
  ar: { create: "إنشاء مفتاح", allowed: "النماذج المسموحة صراحةً", candidates: "النماذج المرشحة للتوجيه الذكي", empty: "اختر نموذجًا نصيًا مصرحًا به على الأقل للتوجيه الذكي.", close: "إغلاق" },
};
try {
  let ready = false;
  for (let i = 0; i < 80; i++) {
    try { ready = (await fetch(`${backend}/api/plaza/models`)).ok; } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, `isolated backend did not start: ${backendLogs}`);
  vite = await createViteServer({ root: web, server: { port: await freePort(), host: "127.0.0.1", proxy: { "/api": backend, "/v1": backend } }, logLevel: "error" });
  await vite.listen();
  const base = vite.resolvedUrls.local[0].replace(/\/$/, "");
  browser = await chromium.launch({ channel: "chrome", headless: true });
  const seed = await browser.newContext();
  const login = await seed.request.post(`${base}/api/user/login`, { data: { username: "root", password } });
  assert.equal(login.status(), 200);
  const rateLimit = await seed.request.put(`${base}/api/option`, { data: { key: "ip_rate_limit", value: "10000" } });
  assert.equal(rateLimit.status(), 200);
  const channel = await seed.request.post(`${base}/api/channel/`, { data: { name: "Key acceptance", type: 1, key: "synthetic-browser-key", base_url: "https://example.invalid", models: "alpha,beta,gamma", group: "default", status: 1 } });
  assert.equal(channel.status(), 200);
  const storageState = await seed.storageState();
  await seed.close();
  for (const language of ["zh", "en", "ar"]) for (const theme of ["light", "dark"]) for (const width of [1440, 390]) {
    const text = words[language];
    const context = await browser.newContext({ storageState, viewport: { width, height: width === 390 ? 844 : 1000 } });
    await context.addInitScript(({ language, theme }) => { localStorage.setItem("xy-language", language); localStorage.setItem("xy-theme", theme); }, { language, theme });
    const page = await context.newPage();
    page.setDefaultTimeout(12000);
    page.on("pageerror", (error) => report.errors.push(String(error)));
    page.on("response", (response) => {
      if (response.url().includes("/api/") && response.status() >= 400)
        report.networkErrors.push({ path: new URL(response.url()).pathname, status: response.status() });
    });
    await page.goto(`${base}/console/keys`);
    await page.locator(".key-manager-header").getByRole("button", { name: text.create, exact: true }).click();
    const dialog = page.getByRole("dialog");
    await page.locator("#new-key-name").fill("Key acceptance");
    const allowed = dialog.getByText(text.allowed, { exact: true }).locator("..");
    const candidates = dialog.getByRole("group", { name: text.candidates, exact: true });
    await allowed.getByLabel("gamma", { exact: true }).uncheck();
    assert.equal(await candidates.getByLabel("gamma", { exact: true }).count(), 0);
    await candidates.getByLabel("alpha", { exact: true }).uncheck();
    await candidates.getByLabel("beta", { exact: true }).uncheck();
    await dialog.getByRole("button", { name: text.create, exact: true }).click();
    await dialog.getByRole("alert").filter({ hasText: text.empty }).waitFor();
    await candidates.getByLabel("beta", { exact: true }).check();
    const box = await dialog.boundingBox();
    assert.ok(box && box.x >= 0 && box.x + box.width <= width + 1, "dialog exceeds viewport");
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), "horizontal page overflow");
    await page.locator(".task-dialog-body").evaluate((element) => { element.scrollTop = 200; });
    await page.waitForTimeout(180);
    const screenshot = `keys-${language}-${theme}-${width}.png`;
    await page.screenshot({ path: path.join(out, screenshot), fullPage: false });
    report.screenshots.push(screenshot);
    await page.locator("#native-key-smart-access").focus();
    await page.keyboard.press("Tab");
    assert.ok(await dialog.evaluate((element) => element.contains(document.activeElement)), "keyboard focus escaped dialog");
    if (language === "zh" && theme === "light" && width === 1440) {
      const responsePromise = page.waitForResponse((response) => response.request().method() === "POST" && response.url().endsWith("/api/token/owned-action"));
      await dialog.getByRole("button", { name: text.create, exact: true }).click();
      const response = await responsePromise;
      assert.equal(response.status(), 200);
      const body = response.request().postDataJSON();
      assert.equal(body.payload.model_limits, "alpha,beta");
      assert.equal(body.payload.smart_candidates, "beta");
      assert.equal(body.payload.smart_access, true);
      const list = await context.request.get(`${base}/api/token/owned`);
      const token = (await list.json()).data.find((row) => row.name === "Key acceptance");
      assert.equal(token.smart_candidates, "beta");
      assert.equal(token.smart_access, true);
      assert.equal(token.model_limits, "alpha,beta");
      report.checks.push("real API preserves separate authorization and smart candidate lists");
      await dialog.getByRole("button", { name: text.close, exact: true }).last().click();
    } else {
      await page.keyboard.press("Escape");
    }
    await dialog.waitFor({ state: "hidden" });
    report.checks.push(`${language}/${theme}/${width}: candidate boundary, empty validation, keyboard and viewport`);
    console.log(`PASS ${language}/${theme}/${width}`);
    await context.close();
  }
  assert.deepEqual(report.errors, []);
} catch (error) {
  report.errors.push(String(error));
  console.error(error);
  process.exitCode = 1;
} finally {
  await writeFile(path.join(out, "browser-report.json"), JSON.stringify(report, null, 2));
  await browser?.close();
  await vite?.close();
  child.kill();
}
