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
const out = path.join(
  project,
  "output",
  "unified-login-20261104",
  "staff-browser",
);
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
let vite, browser;
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
  const root = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
  });
  await root.addInitScript(() => {
    localStorage.setItem("xy-language", "en");
    localStorage.setItem("xy-theme", "light");
  });
  const page = await root.newPage(),
    errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(base + "admin/login?continue=admin");
  await page.locator("#staff-account").fill("root");
  await page.locator("input[autocomplete=current-password]").fill(env.STARHUB_STAFF_PASSWORD);
  await page.locator(".auth-form button[type=submit]").click();
  await page
    .getByLabel("New password (12–72 bytes)", { exact: true })
    .fill("Changed-Synthetic-root-123");
  await page
    .getByRole("button", { name: "Change password and continue", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Get setup key", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Show setup key", exact: true })
    .click();
  const secret = await page.locator(".staff-secret").textContent();
  await page
    .getByRole("button", { name: "Hide setup key", exact: true })
    .click();
  assert.equal(
    (
      await page.request.get(new URL("/api/user", base).href, {
        headers: { "X-StarHub-Portal": "staff" },
      })
    ).status(),
    403,
  );
  await page
    .getByLabel("6-digit verification code", { exact: true })
    .fill(totp(secret));
  await page
    .getByRole("button", { name: "Verify and enter", exact: true })
    .click();
  await page.waitForURL(/\/admin$/);
  await page.locator("[data-staff-page=analytics]").waitFor();
  const post = async (context, endpoint, data, want = 200) => {
    const response = await context.request.post(new URL(endpoint, base).href, {
      data,
      headers: { "X-StarHub-Portal": "staff" },
    });
    assert.equal(response.status(), want, endpoint);
    return (await response.json()).data;
  };
  await post(root, "/api/user/register", {
    username: "synthetic-customer",
    password: "Customer-fixture-123",
    email: "synthetic@example.invalid",
  });
  const roleStates = new Map([[100, await root.storageState()]]);
  for (const role of [10, 5, 20]) {
    const username = `staff-${role}`,
      password = `Temporary-staff-${role}`;
    await post(root, "/api/admin/staff", {
      username,
      temporary_password: password,
      role,
      reason: "Browser acceptance fixture",
    });
    const context = await browser.newContext();
    await post(context, "/api/admin/auth/login", { username, password });
    await post(context, "/api/admin/auth/password", {
      current_password: password,
      new_password: `Changed-${password}`,
    });
    const setup = await post(context, "/api/admin/auth/enroll");
    await post(context, "/api/admin/auth/verify", { code: totp(setup.secret) });
    await post(
      context,
      "/api/admin/resource-sync",
      undefined,
      role === 20 ? 503 : 403,
    );
    roleStates.set(role, await context.storageState());
    await context.close();
  }
  const users = await (
    await root.request.get(new URL("/api/user", base).href, {
      headers: { "X-StarHub-Portal": "staff" },
    })
  ).json();
  const customer = users.data.find((u) => u.username === "synthetic-customer");
  await post(root, `/api/admin/customers/${customer.id}/adjustments`, {
    amount_usd: "0.000000001",
    kind: "gift",
    reason: "Exact nano fixture",
    operation_id: "browser-adjustment-001",
  });
  const rate = await root.request.put(new URL("/api/option", base).href, {
    data: { key: "ip_rate_limit", value: "10000" },
    headers: { "X-StarHub-Portal": "staff" },
  });
  assert.equal(rate.status(), 200);
  await page.goto(base + "admin?tab=staff");
  await page.getByRole("button", { name: "Add staff", exact: true }).click();
  const form = page.locator("#staff-account-form");
  await form.getByLabel("Username", { exact: true }).fill("ui-staff");
  await form.locator("input[type=password]").fill("Synthetic-UI-password-123");
  await form.getByLabel("Role", { exact: true }).selectOption("10");
  await form
    .getByLabel("Reason", { exact: true })
    .fill("Browser account creation fixture");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  const uiRow = page
    .locator(".staff-table tbody tr")
    .filter({ hasText: "ui-staff" });
  await uiRow.waitFor();
  await uiRow.getByRole("button", { name: "Manage", exact: true }).click();
  await form.getByLabel("Status", { exact: true }).selectOption("2");
  await form
    .getByLabel("Reason", { exact: true })
    .fill("Browser departure fixture");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await uiRow.getByText("Disabled", { exact: true }).waitFor();
  // Keep the verified accounts and backend assertions while exercising the supplied Demo layout.
  const navigationNames = {
    zh: "打开导航",
    en: "Open navigation",
    ar: "افتح التنقل",
  };
  const openNavigation = async (p, language = "en") => {
    const trigger = p.getByRole("button", {
      name: navigationNames[language],
      exact: true,
    });
    if (await trigger.count()) await trigger.click();
  };
  const selectPage = async (p, id, language = "en") => {
    await openNavigation(p, language);
    const link = p.locator(`[data-staff-tab=${id}]`);
    if (!(await link.isVisible()))
      await link
        .locator("xpath=ancestor::section[1]")
        .getByRole("button")
        .click();
    await link.click();
    await p.locator(`[data-staff-page=${id}]`).waitFor();
  };
  for (const language of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const context = await browser.newContext({
          storageState: roleStates.get(100),
          viewport: { width, height: 1000 },
        });
        await context.addInitScript(
          ({ language, theme }) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          { language, theme },
        );
        const p = await context.newPage(),
          issues = [];
        p.on("pageerror", (error) => issues.push(error.message));
        await p.goto(base + "admin?tab=staff");
        await p
          .locator(".staff-table tbody tr")
          .filter({ hasText: "staff-10" })
          .waitFor();
        assert.equal(await p.locator(".admin-tabbar").count(), 0);
        const brand = await p
          .locator(".staff-workspace")
          .first()
          .evaluate((element) =>
            getComputedStyle(element).getPropertyValue("--brand").trim(),
          );
        assert.equal(brand, theme === "dark" ? "#82a6ff" : "#3068fa");
        await openNavigation(p, language);
        const nav = p
          .getByRole("navigation")
          .filter({ has: p.locator("[data-staff-tab=staff]") });
        await nav
          .locator("[data-staff-tab=staff][aria-current=page]")
          .waitFor();
        await p.screenshot({
          path: path.join(out, `navigation-${language}-${theme}-${width}.png`),
          fullPage: false,
        });
        if (width === 390) {
          await p.keyboard.press("Escape");
          await p.waitForFunction(
            (label) =>
              document.activeElement?.getAttribute("aria-label") === label,
            navigationNames[language],
          );
          assert.equal(
            await p
              .getByRole("button", {
                name: navigationNames[language],
                exact: true,
              })
              .evaluate((element) => element === document.activeElement),
            true,
          );
        }
        const toolbar = p.locator(".staff-list-toolbar");
        await toolbar.locator("input").fill("staff-10");
        await p
          .locator(".staff-table tbody tr")
          .filter({ hasText: "staff-10" })
          .waitFor();
        assert.equal(await p.locator(".staff-table tbody tr").count(), 1);
        await toolbar.locator("input").fill("");
        await toolbar.locator("select").selectOption("2");
        await p
          .locator(".staff-table tbody tr")
          .filter({ hasText: "ui-staff" })
          .waitFor();
        assert.equal(await p.locator(".staff-table tbody tr").count(), 1);
        await toolbar.locator("select").selectOption("all");
        assert.equal(
          await p.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          ),
          false,
        );
        assert.equal(await p.locator("input[type=password]").count(), 0);
        await p.screenshot({
          path: path.join(out, `staff-${language}-${theme}-${width}.png`),
          fullPage: true,
        });
        await selectPage(p, "customers", language);
        await p
          .locator(".staff-table tbody tr")
          .filter({ hasText: "synthetic-customer" })
          .waitFor();
        await p.goBack();
        await p.locator("[data-staff-page=staff]").waitFor();
        await p.goto(base + "admin/login?continue=admin");
        await p.waitForURL(/\/admin$/);
        await p.locator("[data-staff-page=analytics]").waitFor();
        assert.equal(issues.length, 0, issues.join("\n"));
        results.push({
          language,
          theme,
          width,
          themeMatchesCurrentCode: true,
          demoSidebar: true,
          mobileKeyboardFocus: true,
          staffFilters: true,
          noPageErrors: true,
        });
        await context.close();
      }
  const expected = {
    10: ["analytics", "customers", "models", "groups", "incidents", "audit"],
    5: ["analytics", "customers", "finance", "audit"],
    20: ["customers", "calls", "resources", "sync", "audit"],
  };
  const defaults = { 10: "analytics", 5: "finance", 20: "sync" };
  for (const role of [10, 5, 20]) {
    const context = await browser.newContext({
      storageState: roleStates.get(role),
      viewport: { width: 390, height: 1000 },
    });
    await context.addInitScript(() => {
      localStorage.setItem("xy-language", "en");
      localStorage.setItem("xy-theme", "dark");
    });
    const p = await context.newPage();
    await p.goto(base + "admin");
    await p.locator(`[data-staff-page=${defaults[role]}]`).waitFor();
    await openNavigation(p);
    assert.equal(
      await p.locator("[data-platform-management]").count(),
      1,
    );
    const ids = await p
      .locator("[data-staff-tab]")
      .evaluateAll((nodes) =>
        nodes.map((node) => node.getAttribute("data-staff-tab")),
      );
    assert.deepEqual(ids.sort(), expected[role].sort());
    await p.keyboard.press("Escape");
    await selectPage(p, "customers");
    await p
      .locator(".staff-table tbody tr")
      .filter({ hasText: "synthetic-customer" })
      .waitFor();
    assert.equal(
      await p
        .getByRole("button", { name: "Adjust balance", exact: true })
        .count(),
      role === 5 ? 1 : 0,
    );
    if (role === 10) {
      const pricing = p.waitForResponse(
        (response) =>
          response.url().endsWith("/api/option/pricing") &&
          response.request().method() === "GET",
      );
      await selectPage(p, "models");
      assert.equal((await pricing).status(), 200);
      await p.locator(".server-token-pricing").waitFor();
      await selectPage(p, "customers");
    }
    if (role === 5) {
      await p
        .getByRole("button", { name: "Adjust balance", exact: true })
        .click();
      await p.getByLabel("Type", { exact: true }).selectOption("deduction");
      await p
        .getByLabel("Signed amount (USD, up to 9 decimal places)", {
          exact: true,
        })
        .fill("-1.000000001");
      await p
        .getByLabel("Reason (required)", { exact: true })
        .fill("Synthetic finance adjustment");
      await p.getByRole("button", { name: "Confirm", exact: true }).click();
      await p.getByText("−$1.000000000", { exact: true }).waitFor();
    }
    await p.goto(base + "admin?tab=sync");
    await p
      .locator(`[data-staff-page=${role === 20 ? "sync" : defaults[role]}]`)
      .waitFor();
    if (role === 20)
      assert.equal(
        await p
          .getByRole("button", { name: "Force full sync", exact: true })
          .isDisabled(),
        true,
      );
    else
      assert.equal(
        await p
          .getByRole("button", { name: "Force full sync", exact: true })
          .count(),
        0,
      );
    await p.screenshot({
      path: path.join(out, `role-${role}-dark-390.png`),
      fullPage: true,
    });
    await context.close();
  }
  assert.equal(errors.length, 0, errors.join("\n"));
  await writeFile(
    path.join(out, "browser-report.json"),
    JSON.stringify(
      {
        passed: true,
        cases: results,
        roles: [10, 5, 20, 100],
        realHTTPOnboarding: true,
        credentialsInArtifacts: false,
        scope:
          "Isolated database and local browser. No real authenticator or production provider was used.",
      },
      null,
      2,
    ),
  );
  console.log(
    JSON.stringify({
      passed: true,
      combinations: results.length,
      roles: 4,
      report: path.join(out, "browser-report.json"),
    }),
  );
} finally {
  await browser?.close();
  await vite?.close();
  child.kill();
}
