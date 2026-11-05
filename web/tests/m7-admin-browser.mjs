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
const out = path.join(project, "output", "staff-separation-browser");
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
  await page.goto(base + "admin/login");
  await page.locator("#staff-account").fill("root");
  await page
    .getByLabel("Password", { exact: true })
    .fill(env.STARHUB_STAFF_PASSWORD);
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
  await page.waitForURL(/\/console$/);
  await page
    .locator(".xy-profile")
    .getByText("Platform administrator", { exact: true })
    .waitFor();
  const workspaceSelf = await page.request.get(base + "api/user/self", {
    headers: { "X-StarHub-Portal": "workspace" },
  });
  const principal = (await workspaceSelf.json()).data;
  assert.equal(principal.role, 1);
  assert.equal(principal.platform_admin, true);
  const workspaceGroups = (scope) =>
    scope.locator(".workspace-nav-group").evaluateAll((groups) =>
      groups.map((group) => ({
        label: group.getAttribute("aria-label"),
        paths: Array.from(group.querySelectorAll("a"), (link) =>
          link.getAttribute("href"),
        ),
      })),
    );
  const consoleGroups = await workspaceGroups(page.locator(".xy-sidebar-nav"));
  assert.deepEqual(consoleGroups, [
    {
      label: "CONSOLE",
      paths: [
        "/console",
        "/models",
        "/console/keys",
        "/console/connect",
        "/console/playground",
      ],
    },
    {
      label: "Quality & routing",
      paths: [
        "/console/service-quality",
        "/console/usage",
        "/console/requests",
      ],
    },
    {
      label: "ACCOUNT",
      paths: ["/console/billing", "/console/exports", "/console/settings"],
    },
  ]);
  await page.locator("[data-platform-management]").click();
  await page.waitForURL(/\/admin$/);
  await page.locator(".staff-shell-workspace-switch summary").press("Enter");
  assert.deepEqual(
    await workspaceGroups(page.locator(".staff-shell-workspace-switch")),
    consoleGroups,
  );
  assert.deepEqual(
    await page
      .locator(".staff-shell-workspace-switch .xy-nav-label")
      .allTextContents(),
    ["Quality & routing", "ACCOUNT"],
  );
  await page
    .locator('.staff-shell-workspace-switch a[href="/console/keys"]')
    .click();
  await page.waitForURL(/\/console\/keys$/);
  await page.reload();
  await page
    .locator(".xy-profile")
    .getByText("Platform administrator", { exact: true })
    .waitFor();
  await page.screenshot({ path: path.join(out, "workspace-keys.png") });
  await page.locator("[data-platform-management]").click();
  await page.locator("[data-staff-page=analytics]").waitFor();
  await page.goto(base + "admin?tab=staff");
  const staffForm = page.locator("#staff-account-form");
  const createAttempts = [];
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      new URL(request.url()).pathname === "/api/admin/staff"
    )
      createAttempts.push(request.method());
  });
  const fillStaff = async (username, password) => {
    await page.getByRole("button", { name: "Add staff", exact: true }).click();
    await staffForm.getByLabel("Username", { exact: true }).fill(username);
    await staffForm.locator("[name=temporary_password]").fill(password);
    await staffForm.locator("[name=reason]").fill("Form validation fixture");
  };
  const saveStaff = () =>
    page.getByRole("button", { name: "Save", exact: true }).click();
  await fillStaff("cw", "Synthetic-temporary-123");
  await saveStaff();
  await staffForm.getByRole("alert").filter({ hasText: /3–64/ }).waitFor();
  assert.equal(createAttempts.length, 0);
  await staffForm.getByLabel("Username", { exact: true }).fill("cw_ops");
  await staffForm.locator("[name=temporary_password]").fill("界".repeat(25));
  await saveStaff();
  await staffForm
    .getByRole("alert")
    .filter({ hasText: /UTF-8 bytes/ })
    .waitFor();
  assert.equal(createAttempts.length, 0);
  await staffForm.locator("[name=temporary_password]").fill("界".repeat(4));
  await saveStaff();
  await staffForm.waitFor({ state: "hidden" });
  await page.getByRole("cell", { name: "cw_ops", exact: true }).waitFor();
  assert.equal(createAttempts.length, 1);
  await fillStaff("cw_ops", "Synthetic-temporary-123");
  await saveStaff();
  await staffForm
    .getByRole("alert")
    .filter({ hasText: /username may already be in use/i })
    .waitFor();
  assert.equal(createAttempts.length, 2);
  assert.equal(await staffForm.getByRole("checkbox").count(), 0);
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page
    .getByRole("row")
    .filter({ hasText: "cw_ops" })
    .getByRole("button", { name: "Manage", exact: true })
    .click();
  assert.equal(
    await staffForm
      .getByRole("checkbox", { name: /Platform administrator/i })
      .count(),
    0,
  );
  await staffForm.getByRole("checkbox", { name: /reset.*2FA/i }).waitFor();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
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
  const isolated = await browser.newContext();
  await isolated.addInitScript(() => localStorage.setItem("xy-language", "en"));
  const authPage = await isolated.newPage();
  const authRequests = [];
  authPage.on("request", (request) => {
    if (request.method() === "POST" && request.url().includes("/auth/login"))
      authRequests.push(request.url());
  });
  await authPage.goto(base + "login");
  await authPage.locator("#auth-account").fill("root");
  await authPage.locator("#auth-password").fill("Changed-Synthetic-root-123");
  await authPage.locator("#auth-consent").check();
  await authPage.locator(".auth-form button[type=submit]").click();
  await authPage.locator(".auth-error").waitFor();
  assert.equal(authRequests.length, 0, "customer form attempted staff login");
  assert.equal(await authPage.locator(".auth-staff-verification").count(), 0);
  await authPage.goto(base + "admin/login");
  await authPage.locator("#staff-account").fill("synthetic-customer");
  await authPage
    .getByLabel("Password", { exact: true })
    .fill("Customer-fixture-123");
  await authPage.locator(".auth-form button[type=submit]").click();
  await authPage.locator(".auth-error").waitFor();
  assert.match(authPage.url(), /admin\/login$/);
  assert.equal(
    (await isolated.cookies()).filter((cookie) =>
      ["session", "staff_session"].includes(cookie.name),
    ).length,
    0,
  );
  await authPage.locator("#staff-account").fill("root");
  await authPage
    .getByLabel("Password", { exact: true })
    .fill("Changed-Synthetic-root-123");
  await authPage.locator(".auth-form button[type=submit]").click();
  await authPage
    .getByLabel("6-digit verification code", { exact: true })
    .waitFor();
  await authPage.reload();
  await authPage
    .getByLabel("6-digit verification code", { exact: true })
    .waitFor();
  assert.equal(
    (await isolated.request.get(base + "api/admin/auth/self")).status(),
    403,
  );
  await isolated.close();
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
  const rate = await root.request.put(base + "api/option", {
    data: { key: "ip_rate_limit", value: "10000" },
    headers: { "X-StarHub-Portal": "staff" },
  });
  assert.equal(rate.status(), 200);
  livePage = page;
  await page.goto(base + "admin?tab=customers");
  await page
    .getByRole("link", { name: "synthetic-customer", exact: true })
    .click();
  await page
    .getByRole("heading", { name: "Account information", exact: true })
    .waitFor();
  await page.getByRole("button", { name: "Change limit", exact: true }).click();
  await page.getByLabel("New concurrent request limit").fill("51");
  await page
    .getByLabel("Reason", { exact: true })
    .fill("Synthetic concurrency review");
  await page.getByRole("button", { name: "Confirm", exact: true }).click();
  await page.locator(".adm-count").getByText("51", { exact: true }).waitFor();
  const detailURL = page.url();
  await page.screenshot({
    path: path.join(out, "account-detail-en-light.png"),
    fullPage: true,
  });
  await page.goto(base + "admin?tab=models");
  await page
    .getByRole("button", { name: "New standard model", exact: true })
    .click();
  await page.getByLabel("Display name", { exact: true }).fill("Browser model");
  await page.getByLabel("Stable model ID").fill("browser-model");
  await page.getByLabel("Vendor", { exact: true }).fill("OpenAI");
  await page.getByLabel("Capabilities (comma separated)").fill("text,code");
  assert.equal(
    await page.getByLabel("Capabilities (comma separated)").inputValue(),
    "text,code",
  );
  await page
    .getByRole("tab", { name: "Platform base price", exact: true })
    .click();
  await page
    .getByLabel("Input USD / million tokens", { exact: true })
    .fill("1");
  await page
    .getByLabel("Output USD / million tokens", { exact: true })
    .fill("2");
  await page
    .getByRole("tab", { name: "Official reference", exact: true })
    .click();
  await page
    .getByLabel("Reference input USD / million", { exact: true })
    .fill("5");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page
    .getByRole("alert")
    .getByText(/source URL and verification date/)
    .waitFor();
  await page
    .getByLabel("Source URL", { exact: true })
    .fill("https://example.invalid/pricing");
  await page
    .getByLabel("Verification date", { exact: true })
    .fill("2026-11-04");
  await page
    .getByLabel("Applicable conditions", { exact: true })
    .fill("Synthetic browser reference");
  await page.screenshot({
    path: path.join(out, "model-reference-drawer-en-light.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page
    .locator(".adm-table tbody tr")
    .filter({ hasText: "Browser model" })
    .waitFor();
  await page.goto(base + "admin?tab=regions");
  await page
    .getByRole("button", { name: "Edit region profile", exact: true })
    .click();
  await page.getByLabel("Region ID", { exact: true }).fill("browser-region");
  await page.getByLabel("Allowed vendors (one ID per line)").fill("OpenAI");
  await page.getByLabel("IANA timezone", { exact: true }).fill("Asia/Shanghai");
  await page
    .getByRole("button", { name: "Save new version", exact: true })
    .click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page
    .getByRole("button", { name: "Upload new version", exact: true })
    .click();
  await page.getByLabel("Document type", { exact: true }).selectOption("terms");
  await page.getByLabel("Version ID", { exact: true }).fill("browser-v1");
  await page
    .getByLabel("Effective time (regional timezone)")
    .fill("2026-01-01T00:00");
  await page
    .getByLabel("Document content", { exact: true })
    .fill("Synthetic published legal text.");
  await page.getByRole("button", { name: "Save version", exact: true }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page.getByRole("cell", { name: "browser-v1", exact: true }).waitFor();
  const publicPage = await root.newPage();
  await publicPage.goto(base + "legal/terms");
  await publicPage
    .getByText("Synthetic published legal text.", { exact: true })
    .waitFor();
  await publicPage.close();
  await page.goto(base + "admin?tab=finance");
  await page.getByRole("button", { name: "Reconcile", exact: true }).click();
  await page.locator(".adm-table tbody tr").first().waitFor();
  await page.goto(base + "admin?tab=audit");
  await page.locator(".adm-table tbody tr").first().waitFor();
  await page.screenshot({
    path: path.join(out, "audit-en-light.png"),
    fullPage: true,
  });
  const headings = {
    customers: ["账号管理", "Customer accounts"],
    models: ["标准模型与价格", "Models and pricing"],
    regions: ["地区档案", "Region profile"],
    finance: ["账务对账", "Financial reconciliation"],
    audit: ["审计日志", "Audit log"],
    system: ["系统信息", "System information"],
  };
  for (const language of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const loginContext = await browser.newContext({
          viewport: { width, height: 1000 },
        });
        await loginContext.addInitScript(
          ({ language, theme }) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          { language, theme },
        );
        const loginPage = await loginContext.newPage();
        loginPage.on("pageerror", (e) => errors.push(e.message));
        await loginPage.goto(base + "admin/login");
        await loginPage.locator("#staff-account").waitFor();
        assert.equal(
          await loginPage.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          ),
          false,
        );
        const card = await loginPage.locator(".staff-login-card").boundingBox();
        assert.ok(
          card &&
            card.x >= 0 &&
            card.x + card.width <= width + 1 &&
            card.y >= 0,
          "login card must remain entirely on screen in both text directions",
        );
        await loginPage.locator("#staff-account").focus();
        await loginPage.keyboard.press("Tab");
        assert.equal(
          await loginPage
            .locator("input[autocomplete=current-password]")
            .evaluate((el) => el === document.activeElement),
          true,
        );
        await loginPage.screenshot({
          path: path.join(out, `login-${language}-${theme}-${width}.png`),
          fullPage: true,
        });
        await loginContext.close();
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
        const p = await context.newPage();
        p.on("pageerror", (e) => errors.push(e.message));
        for (const tab of Object.keys(headings)) {
          livePage = p;
          await p.goto(base + `admin?tab=${tab}`);
          await p
            .getByRole("heading", {
              name: headings[tab][language === "zh" ? 0 : 1],
              exact: true,
            })
            .waitFor();
          await p.waitForFunction(
            () =>
              ![...document.querySelectorAll(".adm-page [role=status]")].some(
                (e) => e.getBoundingClientRect().height > 0,
              ),
          );
          assert.equal(
            await p.locator(".adm-error").count(),
            0,
            `${tab} ${language} ${theme} ${width}`,
          );
          const overflow = await p.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          );
          assert.equal(
            overflow,
            false,
            `page overflow: ${tab} ${language} ${width}`,
          );
          await p.screenshot({
            path: path.join(out, `${tab}-${language}-${theme}-${width}.png`),
            fullPage: false,
          });
        }
        await p.goto(detailURL);
        await p
          .locator(".adm-count")
          .getByText("51", { exact: true })
          .waitFor();
        if (width === 390)
          await p.screenshot({
            path: path.join(out, `detail-${language}-${theme}-${width}.png`),
            fullPage: true,
          });
        if (width === 390)
          await p.locator(".staff-shell-breadcrumb .staff-icon-button").click();
        await p.locator(".staff-shell-workspace-switch summary").click();
        if (language === "zh")
          await p.screenshot({
            path: path.join(out, `workspace-menu-${theme}-${width}.png`),
          });
        await p
          .locator('.staff-shell-workspace-switch a[href="/console/keys"]')
          .click();
        await p.waitForURL(/\/console\/keys$/);
        await p.locator(".key-manager").waitFor();
        if (width === 390) await p.locator(".xy-menu-toggle").click();
        const management = p.locator("[data-platform-management]");
        await management.evaluate((link) =>
          link.scrollIntoView({ block: "nearest" }),
        );
        if (language === "zh") {
          await p.screenshot({
            path: path.join(out, `platform-entry-${theme}-${width}.png`),
          });
        }
        await management.click();
        await p.locator("[data-staff-page=analytics]").waitFor();
        results.push({
          language,
          theme,
          width,
          pages: 8,
          workspaceRoundTrip: true,
          passed: true,
        });
        await context.close();
      }
  for (const role of [10, 5, 20]) {
    const context = await browser.newContext({
      storageState: roleStates.get(role),
      viewport: { width: 390, height: 900 },
    });
    await context.addInitScript(() => {
      localStorage.setItem("xy-language", "en");
      localStorage.setItem("xy-theme", "dark");
    });
    const p = await context.newPage();
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(base + "admin?tab=customers");
    await p
      .getByRole("link", { name: "synthetic-customer", exact: true })
      .click();
    await p
      .getByRole("heading", { name: "Account information", exact: true })
      .waitFor();
    assert.equal(
      await p
        .getByRole("button", { name: "Edit region profile", exact: true })
        .count(),
      0,
    );
    assert.equal(
      await p
        .getByRole("button", { name: "Adjust balance", exact: true })
        .count(),
      role === 5 ? 1 : 0,
    );
    assert.equal(
      await p
        .getByRole("button", { name: "Change limit", exact: true })
        .count(),
      role === 10 ? 1 : 0,
    );
    const response = await context.request.put(
      base + "api/admin/region-profile",
      { data: {}, headers: { "X-StarHub-Portal": "staff" } },
    );
    assert.equal(response.status(), 403);
    if (role === 20) {
      await p.goto(base + "admin?tab=system");
      await p
        .getByRole("heading", { name: "System information", exact: true })
        .waitFor();
      await p
        .getByRole("region", { name: "Running instances", exact: true })
        .waitFor();
    }
    await p.locator(".staff-shell-breadcrumb .staff-icon-button").click();
    await p.locator(".staff-shell-workspace-switch summary").click();
    await p
      .locator('.staff-shell-workspace-switch a[href="/console/keys"]')
      .click();
    await p.locator(".key-manager").waitFor();
    await p.locator(".xy-menu-toggle").click();
    await p.locator("[data-platform-management]").click();
    const landingTab = { 10: "analytics", 5: "finance", 20: "sync" }[role];
    await p.locator(`[data-staff-page=${landingTab}]`).waitFor();
    await context.close();
  }
  assert.deepEqual(errors, []);
  await writeFile(
    path.join(out, "browser-report.json"),
    JSON.stringify(
      {
        passed: true,
        cases: results,
        roles: [100, 10, 5, 20],
        realHTTP: true,
        independentLoginForms: true,
        pendingOTPResumesOnRefresh: true,
        isolatedDatabase: true,
        credentialsInArtifacts: false,
      },
      null,
      2,
    ),
  );
  console.log(
    JSON.stringify({
      passed: true,
      combinations: results.length,
      pagesPerCombination: 8,
      roles: 4,
      report: path.join(out, "browser-report.json"),
    }),
  );
} catch (error) {
  console.error(error);
  if (livePage && !livePage.isClosed()) {
    await livePage.screenshot({
      path: path.join(out, "failure.png"),
      fullPage: true,
    });
    console.error((await livePage.locator("body").innerText()).slice(-5000));
  }
  throw error;
} finally {
  await browser?.close();
  await vite?.close();
  child.kill();
}
