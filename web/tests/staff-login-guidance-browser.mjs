// Verify the separate login forms using isolated accounts and the actual backend.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { DatabaseSync } from "node:sqlite";
import path from "node:path";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const project = path.resolve(web, "..");
const out = path.join(project, "output", "staff-login-separation");
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "fixture-"));
const binary = path.join(out, "staff-check.exe");
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], { cwd: project, stdio: "pipe" });
const freePort = async () => {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
};
const backend = `http://127.0.0.1:${await freePort()}`;
const configuration = path.join(temp, "config.yml");
await writeFile(
  configuration,
  "version: 1\ninitial_options:\n  ip_rate_limit: 10000\n",
);
const password = "S".repeat(60) + "-Fixture-123";
const customerPassword = "Synthetic-customer-password-123";
const env = {
  ...process.env,
  STARHUB_CONFIG: configuration,
  STARHUB_DB: path.join(temp, "staff.db"),
  STARHUB_PORT: new URL(backend).port,
  STARHUB_COOKIE_SECURE: "false",
  STARHUB_STAFF_ALLOWED_IPS: "127.0.0.0/8,::1/128",
  STARHUB_STAFF_SETUP_MODE: "commercial",
  STARHUB_STAFF_PASSWORD: password,
  STARHUB_TRUSTED_PROXIES: "127.0.0.1/32,::1/128",
  STARHUB_RESOURCE_SNAPSHOT_URL: "",
  STARHUB_RESOURCE_SNAPSHOT_TOKEN: "",
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
  stdio: "ignore",
});
let vite, browser;
const report = {
  passed: false,
  combinations: [],
  checks: [],
  pageErrors: [],
  scope:
    "Isolated fixture database; no real account login or credential changes.",
};
try {
  for (let attempt = 0; attempt < 80; attempt++) {
    try {
      if ((await fetch(backend + "/api/admin/install")).ok) break;
    } catch {}
    await new Promise((resolve) => setTimeout(resolve, 250));
    if (attempt === 79) throw Error("Isolated backend did not start");
  }
  for (const username of ["customer", "disabled-customer"]) {
    const response = await fetch(backend + "/api/user/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password: customerPassword }),
    });
    assert.equal(response.status, 200);
  }
  const database = new DatabaseSync(env.STARHUB_DB);
  database.exec("PRAGMA busy_timeout=10000");
  database
    .prepare(
      "UPDATE users SET status=2 WHERE username=? AND account_kind='customer'",
    )
    .run("disabled-customer");
  database.close();
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
  let clientIndex = 10;
  const open = async (language, theme, width) => {
    // Model distinct clients through the fixture's trusted loopback proxy.
    // Keep the production ten-per-minute staff login limit enabled.
    const context = await browser.newContext({
      viewport: { width, height: 1000 },
      extraHTTPHeaders: { "X-Forwarded-For": `127.0.0.${clientIndex++}` },
    });
    await context.addInitScript(
      ({ language, theme }) => {
        localStorage.setItem("xy-language", language);
        localStorage.setItem("xy-theme", theme);
      },
      { language, theme },
    );
    const page = await context.newPage();
    page.on("pageerror", (error) => report.pageErrors.push(error.message));
    await page.goto(base + "login");
    await page.locator("#auth-account").waitFor();
    return { context, page };
  };
  const signIn = async (page, username, secret) => {
    await page.locator("#auth-account").fill(username);
    await page.locator("#auth-password").fill(secret);
    assert.equal(await page.locator("#auth-password").inputValue(), secret);
    await page.locator("#auth-consent").check();
    const response = page.waitForResponse(
      (value) =>
        value.url().endsWith("/api/user/login") &&
        value.request().method() === "POST",
    );
    await page.locator(".auth-form button[type=submit]").click();
    return response;
  };
  const expected = {
    zh: {
      hint: "这是内部员工账号，请使用后台登录。",
      disabled: "该账户已被禁用，请联系管理员。",
      heading: "欢迎回来",
    },
    en: {
      hint: "This is an internal staff account. Sign in to the back office.",
      disabled: "This account is disabled. Contact the administrator.",
      heading: "Welcome back",
    },
    ar: {
      hint: "هذا حساب موظف داخلي. استخدم تسجيل دخول الإدارة.",
      disabled: "هذا الحساب معطل. اتصل بالمسؤول.",
      heading: "مرحبًا بعودتك",
    },
  };
  for (const language of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const { context, page } = await open(language, theme, width);
        assert.equal(
          await page.locator(".auth-helper-row a[href='/admin/login']").count(),
          0,
        );
        assert.equal(
          await page
            .getByRole("button", {
              name: /填入管理员账号|Use admin account|استخدم حساب المسؤول/,
            })
            .count(),
          0,
        );
        await page
          .getByRole("heading", {
            name: expected[language].heading,
            exact: true,
          })
          .waitFor();
        await page.locator(".auth-dialog").screenshot({
          path: path.join(out, `login-${language}-${theme}-${width}.png`),
          animations: "disabled",
        });
        let staffRequests = 0;
        page.on("request", (request) => {
          if (request.url().endsWith("/api/admin/auth/login")) staffRequests++;
        });
        const response = await signIn(page, "root", password);
        assert.equal(response.status(), 401);
        await page.locator(".auth-error").waitFor();
        assert.equal(staffRequests, 0);
        await page.goto(base + "admin/login");
        await page.locator("#staff-account").fill("root");
        const employeePassword = page.locator("input[autocomplete=current-password]");
        await employeePassword.fill(password);
        assert.equal(await employeePassword.inputValue(), password);
        const staffResponse = page.waitForResponse((value) => value.url().endsWith("/api/admin/auth/login") && value.request().method() === "POST");
        await page.locator(".auth-form button[type=submit]").click();
        assert.equal((await staffResponse).status(), 200);
        await page.locator(".auth-staff-verification").waitFor();
        assert.equal(new URL(page.url()).pathname, "/admin/login");
        assert.equal(await page.locator(".auth-dialog").count(), 1);
        assert.equal(await page.getByRole("alert").count(), 0);
        assert.equal(await page.locator("a[href='/admin/login']").count(), 0);
        assert.equal(
          (await page.request.get(base + "api/admin/auth/self")).status(),
          403,
        );
        assert.equal(
          (await page.request.get(base + "api/user/self")).status(),
          401,
        );
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          ),
          false,
        );
        await page.locator(".auth-dialog").screenshot({
          path: path.join(
            out,
            `verification-${language}-${theme}-${width}.png`,
          ),
          animations: "disabled",
        });
        await page.reload();
        await page.locator(".auth-staff-verification").waitFor();
        assert.equal(
          await page
            .locator("input[autocomplete=current-password]")
            .inputValue(),
          "",
        );
        const back = page.locator(
          ".auth-staff-verification .auth-helper-row button",
        );
        await back.focus();
        await page.keyboard.press("Enter");
        await page.locator("#staff-account").waitFor();
        assert.equal(await page.locator("input[autocomplete=current-password]").inputValue(), "");
        assert.equal(await page.locator("#staff-account").evaluate((el) => el === document.activeElement), true);
        await page.locator(".auth-wordmark").click();
        await page.waitForURL(base);
        report.combinations.push({
          language,
          theme,
          width,
          separateLoginForms: true,
          staffVerificationRequired: true,
          pendingSessionResumed: true,
          keyboardNavigation: true,
          passwordNotPersisted: true,
        });
        await context.close();
      }
  {
    const { context, page } = await open("zh", "dark", 390);
    assert.equal(
      (await signIn(page, "disabled-customer", customerPassword)).status(),
      403,
    );
    await page
      .getByRole("alert")
      .getByText(expected.zh.disabled, { exact: true })
      .waitFor();
    assert.equal(await page.getByRole("alert").getByRole("link").count(), 0);
    report.checks.push(
      "Actual disabled customers retain the correct disabled-account message",
    );
    await page.route("**/api/user/login", (route) =>
      route.fulfill({
        status: 403,
        contentType: "application/json",
        body: JSON.stringify({
          success: false,
          message: "Sign-in denied by policy",
        }),
      }),
    );
    await signIn(page, "customer", customerPassword);
    await page
      .getByRole("alert")
      .getByText("登录请求被拒绝，请联系管理员。", { exact: true })
      .waitFor();
    report.checks.push(
      "A simulated unrelated 403 is not mislabeled as account disablement",
    );
    await page.unroute("**/api/user/login");
    assert.equal(
      (await signIn(page, "customer", customerPassword)).status(),
      200,
    );
    await page.waitForURL("**/console");
    await page.locator(".xy-sidebar-nav").waitFor({ state: "attached" });
    assert.equal(await page.locator("[data-platform-management]").count(), 0);
    report.checks.push("Actual enabled customers still sign in normally");
    report.checks.push("Ordinary customers do not see Platform management");
    assert.equal(
      (await page.request.get(base + "api/admin/staff")).status(),
      403,
    );
    await page.goto(base + "admin");
    await page.waitForURL("**/admin/login?continue=admin");
    await page.locator("#staff-account").waitFor();
    assert.equal(await page.locator(".auth-dialog").count(), 1);
    report.checks.push(
      "Customer direct access to management stays forbidden and returns to the employee login page",
    );
    await context.close();
  }
  {
    const { context, page } = await open("en", "light", 1440);
    let staffLoginRequests = 0;
    page.on("request", (request) => {
      if (request.url().endsWith("/api/admin/auth/login")) staffLoginRequests++;
    });
    assert.equal(
      (await signIn(page, "root", "Incorrect-fixture-password")).status(),
      401,
    );
    await page
      .getByRole("alert")
      .getByText("The account or password is incorrect. Please try again.", {
        exact: true,
      })
      .waitFor();
    assert.equal(staffLoginRequests, 0);
    assert.equal(await page.locator(".auth-staff-verification").count(), 0);
    report.checks.push(
      "Invalid credentials never select a staff session or verification flow",
    );
    await context.close();
  }
  assert.equal(report.pageErrors.length, 0);
  report.passed = true;
  await writeFile(
    path.join(out, "browser-report.json"),
    JSON.stringify(report, null, 2),
  );
  console.log(
    JSON.stringify({
      passed: true,
      combinations: report.combinations.length,
      checks: report.checks.length,
    }),
  );
} finally {
  await browser?.close();
  await vite?.close();
  child.kill();
}
