import { createServer as createViteServer } from "vite";
import { chromium, firefox, webkit } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { execFileSync, spawn } from "node:child_process";
import { createHmac } from "node:crypto";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const root = fileURLToPath(new URL("../", import.meta.url));
const repo = path.resolve(root, "..");
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-staff-security-"));
const engine = process.env.STARHUB_TEST_BROWSER || "chromium";
const out = path.join(root, "playwright-report", "staff-security", engine);
await mkdir(out, { recursive: true });
const report = { engine, checks: [], screenshots: [], pageErrors: [] };
let backend, vite, browser;
async function freePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const port = server.address().port;
      server.close(() => resolve(port));
    });
  });
}
function authenticatorCode(secret) {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const c of secret)
    bits += alphabet.indexOf(c).toString(2).padStart(5, "0");
  const bytes = [];
  for (let i = 0; i + 8 <= bits.length; i += 8)
    bytes.push(parseInt(bits.slice(i, i + 8), 2));
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000)));
  const digest = createHmac("sha1", Buffer.from(bytes))
    .update(counter)
    .digest();
  return String(
    (digest.readUInt32BE(digest[19] & 15) & 0x7fffffff) % 1000000,
  ).padStart(6, "0");
}
try {
  const binary = path.join(
    temp,
    process.platform === "win32" ? "starhub.exe" : "starhub",
  );
  execFileSync("go", ["build", "-o", binary, "."], {
    cwd: repo,
    stdio: "pipe",
  });
  const port = await freePort();
  backend = spawn(binary, [], {
    cwd: temp,
    windowsHide: true,
    stdio: "ignore",
    env: {
      ...process.env,
      STARHUB_PORT: String(port),
      STARHUB_BIND_ADDRESS: "127.0.0.1",
      STARHUB_ROOT_PASSWORD: "BrowserInitial92",
      STARHUB_CRYPTO_SECRET: "browser-staff-only-secret",
      RESOURCE_CATALOG_NOTIFY_URL: "",
    },
  });
  let ready = false;
  for (let i = 0; i < 80; i++) {
    try {
      if ((await fetch(`http://127.0.0.1:${port}/api/health`)).ok) {
        ready = true;
        break;
      }
    } catch {}
    await new Promise((r) => setTimeout(r, 250));
  }
  assert.ok(ready, "backend did not start");
  vite = await createViteServer({
    root,
    logLevel: "error",
    server: {
      host: "127.0.0.1",
      port: await freePort(),
      proxy: { "/api": `http://127.0.0.1:${port}` },
    },
  });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  const browserType = { chromium, firefox, webkit }[engine];
  assert.ok(browserType, "unsupported browser engine");
  browser = await browserType.launch({
    headless: true,
    ...(engine === "chromium" && process.platform === "win32"
      ? { channel: "msedge" }
      : {}),
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    reducedMotion: "reduce",
  });
  await context.addInitScript(() => {
    localStorage.setItem("xy-language", "en");
  });
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on("pageerror", (error) => report.pageErrors.push(String(error)));
  async function check(name, run) {
    await run();
    report.checks.push(name);
    console.log("PASS", engine, name);
  }
  async function capture(name) {
    await page.screenshot({
      path: path.join(out, name + ".png"),
      mask: [
        page.locator("[data-private]"),
        page.locator("input[type=password]"),
        page.locator("#auth-factor"),
      ],
    });
    report.screenshots.push(name + ".png");
  }
  async function login(password) {
    await page.goto(base + "/login");
    await page.locator("#auth-account").fill("root");
    await page.locator("#auth-password").fill(password);
    await page.locator("#auth-consent").check();
    await page.locator(".auth-form button[type=submit]").click();
  }
  await check(
    "staff cannot use management APIs before onboarding",
    async () => {
      await login("BrowserInitial92");
      await page.locator(".staff-security").waitFor();
      const denied = await context.request.get(base + "/api/option/");
      assert.equal(denied.status(), 403);
      const scan = await new AxeBuilder({ page })
        .include(".staff-security")
        .withTags(["wcag2a", "wcag2aa"])
        .analyze();
      assert.deepEqual(scan.violations, []);
      await capture("initial-password-desktop");
    },
  );
  await check(
    "change password and enroll a real TOTP factor through the UI",
    async () => {
      await page
        .getByLabel("Current password", { exact: true })
        .fill("BrowserInitial92");
      await page
        .getByLabel("New password", { exact: true })
        .fill("BrowserChanged93");
      await page
        .getByLabel("Confirm new password", { exact: true })
        .fill("BrowserChanged93");
      await page
        .getByRole("button", { name: "Change password", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Set up authenticator", exact: true })
        .waitFor();
      await page
        .getByLabel("Current password", { exact: true })
        .fill("BrowserChanged93");
      await page
        .getByRole("button", { name: "Set up authenticator", exact: true })
        .click();
      const secret = await page.locator("textarea[data-private]").inputValue();
      await page
        .getByLabel("Six-digit authenticator code", { exact: true })
        .fill(authenticatorCode(secret));
      await page
        .getByRole("button", {
          name: "Enable two-factor authentication",
          exact: true,
        })
        .click();
      await page.locator("pre[data-private]").waitFor();
      assert.equal(
        (await page.locator("pre[data-private]").innerText()).trim().split("\n")
          .length,
        4,
      );
      assert.equal(
        (await context.request.get(base + "/api/option/")).status(),
        403,
      );
      await page.getByLabel("I have saved the recovery codes").check();
      await capture("recovery-codes-masked");
      await page
        .getByRole("button", { name: "Sign in again", exact: true })
        .click();
      await login("BrowserChanged93");
      await page.locator("#auth-factor").fill(authenticatorCode(secret));
      await page.locator(".auth-form button[type=submit]").click();
      await page.waitForURL("**/console");
      assert.equal(
        (await context.request.get(base + "/api/option/")).status(),
        200,
      );
      assert.equal(
        (
          await context.request.post(base + "/api/user/login/2fa", {
            data: { username: "root", code: authenticatorCode(secret) },
          })
        ).status(),
        401,
      );
    },
  );
  await check(
    "mobile Arabic dark mode keeps security setup accessible",
    async () => {
      await context.request.post(base + "/api/user/logout");
      execFileSync(
        "go",
        [
          "run",
          "./cmd/staff-account",
          "-db",
          path.join(temp, "starhub.db"),
          "-action",
          "reset-2fa",
          "-username",
          "root",
          "-reason",
          "Browser recovery fixture",
        ],
        { cwd: repo, stdio: "pipe", windowsHide: true },
      );
      const mobile = await browser.newContext({
        viewport: { width: 320, height: 900 },
        reducedMotion: "reduce",
      });
      await mobile.addInitScript(() => {
        localStorage.setItem("xy-language", "ar");
        localStorage.setItem("xy-theme", "dark");
      });
      const p = await mobile.newPage();
      p.on("pageerror", (error) => report.pageErrors.push(String(error)));
      await mobile.request.post(base + "/api/user/login", {
        data: { username: "root", password: "BrowserChanged93" },
      });
      await p.goto(base + "/console");
      await p.locator(".staff-security").waitFor();
      assert.ok(
        await p.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      );
      const scan = await new AxeBuilder({ page: p })
        .include(".staff-security")
        .withTags(["wcag2a", "wcag2aa"])
        .analyze();
      assert.deepEqual(scan.violations, []);
      await p.screenshot({
        path: path.join(out, "arabic-dark-320.png"),
        mask: [p.locator("input[type=password]")],
      });
      report.screenshots.push("arabic-dark-320.png");
      await mobile.close();
    },
  );
  assert.deepEqual(report.pageErrors, []);
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify(report, null, 2),
  );
  await browser?.close();
  await vite?.close();
  if (backend) {
    const exited = new Promise((resolve) => backend.once("exit", resolve));
    backend.kill();
    await exited;
  }
  await rm(temp, { recursive: true, force: true });
}
