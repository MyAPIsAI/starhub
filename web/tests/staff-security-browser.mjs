import { createServer as createViteServer } from "vite";
import { chromium, firefox, webkit } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { execFileSync, spawn } from "node:child_process";
import { authenticatorCode } from "./browser-totp.mjs";
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
const cryptoEnv = {
  STARHUB_CRYPTO_SECRET: "browser-staff-only-secret",
  STARHUB_CREDENTIAL_KEYS: JSON.stringify({
    old: "11".repeat(32),
    new: "22".repeat(32),
  }),
  STARHUB_CREDENTIAL_ACTIVE_KEY: "old",
};
const out = path.join(root, "playwright-report", "staff-security", engine);
await mkdir(out, { recursive: true });
const report = { engine, checks: [], screenshots: [], pageErrors: [] };
let backend, vite, browser;
let backendOutput = "";
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
    stdio: ["ignore", "pipe", "pipe"],
    env: {
      ...process.env,
      STARHUB_PORT: String(port),
      STARHUB_BIND_ADDRESS: "127.0.0.1",
      STARHUB_ROOT_PASSWORD: "BrowserInitial92",
      ...cryptoEnv,
      RESOURCE_CATALOG_NOTIFY_URL: "",
      STARHUB_REGION_PROFILE: "",
    },
  });
  for (const stream of [backend.stdout, backend.stderr])
    stream.on("data", (chunk) => {
      backendOutput = (backendOutput + chunk.toString()).slice(-16384);
    });
  let ready = false;
  for (let i = 0; i < 80; i++) {
    if (backend.exitCode !== null || backend.signalCode !== null) break;
    try {
      if ((await fetch(`http://127.0.0.1:${port}/api/health`)).ok) {
        ready = true;
        break;
      }
    } catch {}
    await new Promise((r) => setTimeout(r, 250));
  }
  assert.ok(ready, "backend did not start: " + backendOutput);
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
  let factorSecret = "";
  page.setDefaultTimeout(45000);
  page.setDefaultNavigationTimeout(45000);
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
    await page.goto(base + "/login", { waitUntil: "domcontentloaded" });
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
      factorSecret = secret;
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
    "cross-process credential rotation preserves customer and channel identity",
    async () => {
      const api = async (method, url, data) => {
        const response = await context.request.fetch(base + url, {
          method,
          data,
        });
        const result = await response.json();
        assert.ok(response.ok() && result.success, result.message);
        return result.data;
      };
      await api("PUT", "/api/option/", {
        key: "resource_provider_mode",
        value: "local",
      });
      const channel = await api("POST", "/api/channel", {
        name: "Rotation fixture",
        key: "test-upstream-key",
        base_url: "http://127.0.0.1:1",
        models: "rotation-fixture",
        group: "default",
        type: 1,
      });
      const token = await api("POST", "/api/token", {
        name: "Rotation key",
        unlimited_quota: true,
      });
      const rotationEnv = {
        ...process.env,
        ...cryptoEnv,
        STARHUB_CREDENTIAL_ACTIVE_KEY: "new",
      };
      execFileSync(
        "go",
        [
          "run",
          "./cmd/credential-keys",
          "-db",
          path.join(temp, "starhub.db"),
          "-rotate",
          "-reason",
          "Browser rotation fixture",
        ],
        { cwd: repo, stdio: "pipe", windowsHide: true, env: rotationEnv },
      );
      execFileSync(
        "go",
        ["run", "./cmd/credential-keys", "-db", path.join(temp, "starhub.db")],
        {
          cwd: repo,
          stdio: "pipe",
          windowsHide: true,
          env: {
            ...rotationEnv,
            STARHUB_CREDENTIAL_KEYS: JSON.stringify({ new: "22".repeat(32) }),
          },
        },
      );
      const storedChannel = await api("GET", `/api/channel/${channel.id}`);
      assert.equal(storedChannel.id, channel.id);
      assert.equal(
        (
          await context.request.get(`http://127.0.0.1:${port}/v1/models`, {
            headers: { Authorization: `Bearer ${token.key}` },
          })
        ).status(),
        200,
      );
    },
  );
  await check(
    "failed account language saves preserve local and server preferences",
    async () => {
      const before = (
        await (await context.request.get(base + "/api/user/self")).json()
      ).data.language;
      const localBefore = await page.evaluate(() =>
        localStorage.getItem("xy-language"),
      );
      await page.route("**/api/user/self", async (route) => {
        if (route.request().method() === "PUT") {
          await route.fulfill({
            status: 503,
            contentType: "application/json",
            body: JSON.stringify({
              success: false,
              message: "Fixture write unavailable",
            }),
          });
        } else await route.continue();
      });
      try {
        await page
          .locator(".product-preferences [role=combobox]")
          .first()
          .click();
        await page
          .getByRole("option", { name: "العربية", exact: true })
          .click();
        await page
          .getByText("Unable to save your language preference. Please retry.", {
            exact: true,
          })
          .waitFor();
        assert.equal(
          await page.evaluate(() => document.documentElement.lang),
          "en",
        );
        assert.equal(
          await page.evaluate(() => localStorage.getItem("xy-language")),
          localBefore,
        );
        const after = (
          await (await context.request.get(base + "/api/user/self")).json()
        ).data.language;
        assert.equal(after, before);
        await capture("account-language-save-error");
      } finally {
        await page.unroute("**/api/user/self");
      }
    },
  );
  await check(
    "account language is saved through the console and restored on another device",
    async () => {
      await page
        .locator(".product-preferences [role=combobox]")
        .first()
        .click();
      await page.getByRole("option", { name: "العربية", exact: true }).click();
      await page.waitForFunction(() => document.documentElement.lang === "ar");
      const self = await (
        await context.request.get(base + "/api/user/self")
      ).json();
      assert.equal(self.data.language, "ar");
      const device = await browser.newContext({
        viewport: { width: 390, height: 900 },
        locale: "en-US",
        reducedMotion: "reduce",
      });
      const first = await device.request.post(base + "/api/user/login", {
        data: { username: "root", password: "BrowserChanged93" },
      });
      assert.equal((await first.json()).data["2fa_required"], true);
      const second = await device.request.post(base + "/api/user/login/2fa", {
        data: { username: "root", code: authenticatorCode(factorSecret) },
      });
      assert.equal(second.status(), 200);
      const p = await device.newPage();
      p.on("pageerror", (error) => report.pageErrors.push(String(error)));
      await p.goto(base + "/console", {
        waitUntil: "domcontentloaded",
        timeout: 45000,
      });
      await p.waitForFunction(
        () =>
          document.documentElement.lang === "ar" &&
          document.documentElement.dir === "rtl",
      );
      assert.equal(
        await p.evaluate(() => localStorage.getItem("xy-language")),
        null,
      );
      await p.screenshot({
        path: path.join(out, "account-language-second-device-390.png"),
        mask: [p.locator("input[type=password]")],
      });
      report.screenshots.push("account-language-second-device-390.png");
      await device.close();
    },
  );
  await check(
    "regional legal changes require renewed consent and preserve registration input",
    async () => {
      const api = async (method, url, data) => {
        const response = await context.request.fetch(base + url, {
          method,
          data,
        });
        const body = await response.json();
        assert.ok(response.ok() && body.success, body.message);
        return body.data;
      };
      const profile = {
        id: "default",
        name: "Browser region",
        default_language: "ar",
        supported_languages: "ar,en",
        country_code: "AE",
        timezone: "Asia/Dubai",
        legal_version: "browser-v1",
        allowed_models: "rotation-fixture",
        feature_flags: JSON.stringify({ payments: false }),
        enabled: true,
      };
      const saved = await api("PUT", "/api/region-profile", profile);
      async function publish(version) {
        for (const type of ["user_agreement", "privacy_policy"])
          await api("PUT", "/api/legal", {
            type,
            version,
            language: "en",
            region_id: "default",
            content: `Published regional fixture ${type} ${version}.`,
          });
      }
      await publish("browser-v1");
      assert.equal(
        (
          await context.request.post(base + "/api/recharge", {
            data: { amount_usd: "10", channel: "card" },
          })
        ).status(),
        403,
      );
      const guest = await browser.newContext({
        viewport: { width: 390, height: 900 },
        locale: "fr-FR",
        reducedMotion: "reduce",
      });
      const p = await guest.newPage();
      p.setDefaultTimeout(45000);
      p.on("pageerror", (error) => report.pageErrors.push(String(error)));
      await p.goto(base + "/legal/terms", {
        waitUntil: "domcontentloaded",
        timeout: 45000,
      });
      await p
        .locator(".regional-legal-document")
        .getByText("Published regional fixture user_agreement browser-v1.", {
          exact: true,
        })
        .waitFor();
      await p.waitForFunction(() => document.documentElement.lang === "ar");
      await p.screenshot({ path: path.join(out, "regional-legal-ar-390.png") });
      report.screenshots.push("regional-legal-ar-390.png");
      await p.goto(base + "/register", {
        waitUntil: "domcontentloaded",
        timeout: 45000,
      });
      await p.locator("#auth-username").fill("regional-member");
      await p.locator("#auth-new-password").fill("RegionalMember94");
      await p.locator("#auth-confirm-password").fill("RegionalMember94");
      await p.locator('[data-auth-policy="terms"]').click();
      await p
        .locator(".regional-legal-document")
        .getByText("Published regional fixture user_agreement browser-v1.", {
          exact: true,
        })
        .waitFor();
      await p.locator(".auth-back").click();
      await p.locator("#auth-consent").check();
      await publish("browser-v2");
      await api("PUT", "/api/region-profile", {
        ...profile,
        legal_version: "browser-v2",
        expected_revision: saved.revision,
      });
      await p.locator(".auth-submit").click();
      await p.locator(".auth-error").waitFor();
      assert.equal(await p.locator("#auth-consent").isChecked(), false);
      assert.equal(
        await p.locator("#auth-username").inputValue(),
        "regional-member",
      );
      assert.equal(
        JSON.stringify(await api("GET", "/api/user")).includes(
          "regional-member",
        ),
        false,
      );
      await p.locator('[data-auth-policy="terms"]').click();
      await p
        .locator(".regional-legal-document")
        .getByText("Published regional fixture user_agreement browser-v2.", {
          exact: true,
        })
        .waitFor();
      await p.locator(".auth-back").click();
      await p.locator("#auth-consent").check();
      await p.locator(".auth-submit").click();
      await p.waitForURL("**/login");
      const login = await guest.request.post(base + "/api/user/login", {
        data: { username: "regional-member", password: "RegionalMember94" },
      });
      assert.equal((await login.json()).data.user.language, "ar");
      await guest.close();
    },
  );
  await check(
    "unpublished regional legal documents block registration without creating an account",
    async () => {
      const list = await (
        await context.request.get(base + "/api/region-profile")
      ).json();
      const profile = list.data.find((entry) => entry.id === "default");
      assert.ok(profile);
      const update = await context.request.put(base + "/api/region-profile", {
        data: {
          ...profile,
          legal_version: "browser-unpublished",
          expected_revision: profile.revision,
        },
      });
      assert.equal(update.status(), 200);
      const guest = await browser.newContext({
        viewport: { width: 390, height: 900 },
        locale: "en-US",
        reducedMotion: "reduce",
      });
      try {
        const p = await guest.newPage();
        p.setDefaultTimeout(45000);
        p.on("pageerror", (error) => report.pageErrors.push(String(error)));
        await p.goto(base + "/register", {
          waitUntil: "domcontentloaded",
          timeout: 45000,
        });
        await p.locator("#auth-username").fill("unpublished-legal-member");
        await p.locator("#auth-new-password").fill("RegionalMember95");
        await p.locator("#auth-confirm-password").fill("RegionalMember95");
        await p.locator('[data-auth-policy="terms"]').click();
        await p.locator(".regional-legal-document [role=alert]").waitFor();
        assert.equal(
          await p.locator(".regional-legal-document [lang]").count(),
          0,
        );
        await p.locator(".auth-back").click();
        await p.locator("#auth-consent").check();
        await p.locator(".auth-submit").click();
        await p.locator(".auth-error").waitFor();
        assert.equal(
          await p.locator("#auth-username").inputValue(),
          "unpublished-legal-member",
        );
        const users = await (
          await context.request.get(base + "/api/user")
        ).json();
        assert.equal(
          JSON.stringify(users.data).includes("unpublished-legal-member"),
          false,
        );
        await p.screenshot({
          path: path.join(out, "unpublished-legal-registration-error-390.png"),
          mask: [p.locator("input[type=password]")],
        });
        report.screenshots.push("unpublished-legal-registration-error-390.png");
      } finally {
        await guest.close();
      }
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
        {
          cwd: repo,
          stdio: "pipe",
          windowsHide: true,
          env: { ...process.env, ...cryptoEnv },
        },
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
      await p.goto(base + "/console", {
        waitUntil: "domcontentloaded",
        timeout: 45000,
      });
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
  if (backend && backend.exitCode === null && backend.signalCode === null) {
    const exited = new Promise((resolve) => backend.once("exit", resolve));
    backend.kill();
    await exited;
  }
  assert.equal(path.dirname(path.resolve(temp)), path.resolve(os.tmpdir()));
  assert.ok(path.basename(temp).startsWith("starhub-staff-security-"));
  await rm(temp, { recursive: true, force: true });
}
