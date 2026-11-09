// Isolated staff onboarding and ADM role acceptance against the real backend.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import net from "node:net";
import crypto from "node:crypto";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url)),
  project = path.resolve(web, "..");
const out = path.join(project, "output", "ui21", "admin-browser");
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-")),
  binary = path.join(out, "staff-check.exe");
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: project,
  stdio: "pipe",
});
const freePort = async () => {
  const s = net.createServer();
  await new Promise((r) => s.listen(0, "127.0.0.1", r));
  const port = s.address().port;
  await new Promise((r) => s.close(r));
  return port;
};
const backend = `http://127.0.0.1:${await freePort()}`;
const env = {
  ...process.env,
  STARHUB_CONFIG: "",
  STARHUB_DB: path.join(temp, "staff.db"),
  STARHUB_PORT: new URL(backend).port,
  STARHUB_COOKIE_SECURE: "false",
  STARHUB_STAFF_SETUP_MODE: "commercial",
  STARHUB_STAFF_ALLOWED_IPS: "127.0.0.1/32,::1/128",
  STARHUB_TRUSTED_PROXIES: "",
  STARHUB_RESOURCE_SNAPSHOT_URL: "",
  STARHUB_RESOURCE_SNAPSHOT_TOKEN: "",
  STARHUB_STAFF_PASSWORD: "Synthetic-root-password-123",
  STARHUB_ROOT_PASSWORD: "",
};
execFileSync(binary, ["staff", "init", "--username", "root"], {
  cwd: temp,
  env,
  stdio: "pipe",
});
const child = spawn(binary, [], {
  cwd: temp,
  env,
  windowsHide: true,
  stdio: ["ignore", "ignore", "pipe"],
});
let logs = "";
child.stderr.on("data", (x) => (logs += x));
const totp = (secret) => {
  let bits = "";
  for (const c of secret.trim())
    bits += "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
      .indexOf(c)
      .toString(2)
      .padStart(5, "0");
  const bytes = [];
  for (let i = 0; i + 8 <= bits.length; i += 8)
    bytes.push(parseInt(bits.slice(i, i + 8), 2));
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000)));
  const sum = crypto
      .createHmac("sha1", Buffer.from(bytes))
      .update(counter)
      .digest(),
    offset = sum.at(-1) & 15;
  return String((sum.readUInt32BE(offset) & 0x7fffffff) % 1000000).padStart(
    6,
    "0",
  );
};
let vite, browser, livePage;
const errors = [];
const results = [];
try {
  for (let i = 0; i < 120; i++) {
    try {
      if ((await fetch(backend + "/api/admin/install")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 250));
    if (i === 119) throw Error("isolated staff backend did not start");
  }
  vite = await createViteServer({
    root: web,
    configFile: path.join(web, "vite.config.ts"),
    server: {
      port: await freePort(),
      host: "127.0.0.1",
      proxy: { "/api": backend, "/v1": backend },
    },
  });
  await vite.listen();
  const base = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ channel: "chrome", headless: true });
  const root = await browser.newContext();
  const api = async (method, url, data) => {
    const response = await root.request.fetch(backend + url, {
      method,
      data,
      headers: { Origin: backend, "X-StarHub-Portal": "staff" },
    });
    assert.equal(response.status(), 200, await response.text());
    return (await response.json()).data;
  };
  await api("POST", "/api/admin/auth/login", {
    username: "root",
    password: env.STARHUB_STAFF_PASSWORD,
  });
  await api("POST", "/api/admin/auth/password", {
    current_password: env.STARHUB_STAFF_PASSWORD,
    new_password: "Changed-synthetic-ui21-123",
  });
  const setup = await api("POST", "/api/admin/auth/enroll", {});
  await api("POST", "/api/admin/auth/verify", { code: totp(setup.secret) });
  await api("PUT", "/api/service-group", {
    group_id: "status-review",
    name: "Status review",
    multiplier: "1",
    enabled: true,
    visibility: "common",
  });
  const state = await root.storageState();
  for (const [language, theme, width] of [
    ["zh", "light", 1440],
    ["en", "dark", 390],
  ]) {
    const name = `${language}-${theme}-${width}`;
    const context = await browser.newContext({
      storageState: state,
      viewport: { width, height: 1000 },
      reducedMotion: "reduce",
    });
    await context.addInitScript(
      ({ language, theme }) => {
        localStorage.setItem("xy-language", language);
        localStorage.setItem("xy-theme", theme);
      },
      { language, theme },
    );
    const page = await context.newPage();
    livePage = page;
    page.setDefaultTimeout(15000);
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto(base + "admin?tab=groups");
    const row = page.locator('[data-config-group="status-review"]');
    await row.waitFor();
    await row.locator("button").nth(1).click();
    const dialog = page.getByRole("dialog"),
      inputs = dialog.locator(".public-status-settings input");
    if (await inputs.first().isChecked()) await inputs.first().uncheck();
    await inputs.nth(1).check();
    assert.ok(await inputs.nth(1).isChecked());
    await page.screenshot({
      path: path.join(out, name + "-scope.png"),
      fullPage: true,
    });
    await dialog.locator(".task-dialog-footer button").last().click();
    await dialog.locator(".task-dialog-footer button").last().click();
    await dialog.waitFor({ state: "hidden" });
    let saved = await api("GET", "/api/service-group/configuration");
    assert.equal(
      saved.groups.find((g) => g.group_id === "status-review").status_page_mode,
      "include",
    );
    await row.locator("button").nth(1).click();
    assert.ok(await inputs.nth(1).isChecked());
    await inputs.nth(1).uncheck();
    await dialog.locator(".task-dialog-footer button").last().click();
    await dialog.locator(".task-dialog-footer button").last().click();
    await dialog.waitFor({ state: "hidden" });
    saved = await api("GET", "/api/service-group/configuration");
    assert.equal(
      saved.groups.find((g) => g.group_id === "status-review").status_page_mode,
      "exclude",
    );
    await row.locator("button").nth(1).click();
    await inputs.first().check();
    await dialog.locator(".task-dialog-footer button").last().click();
    await dialog.locator(".task-dialog-footer button").last().click();
    await dialog.waitFor({ state: "hidden" });
    saved = await api("GET", "/api/service-group/configuration");
    assert.equal(
      saved.groups.find((g) => g.group_id === "status-review").status_page_mode,
      "auto",
    );
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    );
    await context.close();
    results.push({ name, passed: true });
  }
  await root.close();
  assert.deepEqual(errors, []);
} catch (error) {
  if (livePage && !livePage.isClosed())
    await livePage
      .screenshot({ path: path.join(out, "failure.png"), fullPage: true })
      .catch(() => {});
  throw error;
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify({ results, errors }, null, 2),
  );
  await browser?.close();
  await vite?.close();
  child.kill();
}
console.log(JSON.stringify({ results, errors }));
