// Real isolated backend and SQLite fixture; no production sessions or data.
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile, rm } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import crypto from "node:crypto";
import net from "node:net";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url)),
  project = path.resolve(web, "..");
const out = path.join(project, "output", "operating-dashboard-browser");
const languages = process.env.DASHBOARD_LANG
  ? [process.env.DASHBOARD_LANG]
  : ["zh", "en", "ar"];
assert.ok(languages.every((language) => ["zh", "en", "ar"].includes(language)));
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-")),
  binary = path.join(temp, "dashboard-check.exe"),
  db = path.join(temp, "dashboard.db");
const freePort = async () => {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
};
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: project,
  stdio: "pipe",
});
const backend = `http://127.0.0.1:${await freePort()}`;
const env = {
  ...process.env,
  STARHUB_CONFIG: "",
  STARHUB_DB: db,
  STARHUB_PORT: new URL(backend).port,
  STARHUB_COOKIE_SECURE: "false",
  STARHUB_STAFF_SETUP_MODE: "commercial",
  STARHUB_STAFF_ALLOWED_IPS: "127.0.0.1/32,::1/128",
  STARHUB_TRUSTED_PROXIES: "",
  STARHUB_RESOURCE_SNAPSHOT_URL: "",
  STARHUB_RESOURCE_SNAPSHOT_TOKEN: "",
  STARHUB_STAFF_PASSWORD: "Synthetic-dashboard-root-123",
  STARHUB_ROOT_PASSWORD: "",
};
execFileSync(binary, ["staff", "init", "--username", "root"], {
  cwd: temp,
  env,
  stdio: "pipe",
});
execFileSync(
  "go",
  ["run", path.join(web, "tests", "operating-dashboard-seed.go"), db],
  { cwd: project, env, stdio: "pipe" },
);
const child = spawn(binary, [], {
  cwd: temp,
  env,
  windowsHide: true,
  stdio: ["ignore", "ignore", "pipe"],
});
let backendErrors = "";
child.stderr.on("data", (value) => (backendErrors += value));
const totp = (secret) => {
  let bits = "";
  for (const char of secret)
    bits += "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
      .indexOf(char)
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
const results = [],
  errors = [];
try {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      if ((await fetch(backend + "/api/admin/install")).ok) break;
    } catch {}
    if (attempt === 99) throw Error("isolated backend did not start");
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  vite = await createServer({
    root: web,
    server: {
      host: "127.0.0.1",
      port: await freePort(),
      proxy: { "/api": backend, "/v1": backend },
    },
  });
  await vite.listen();
  const base = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ channel: "chrome", headless: true });
  const auth = await browser.newContext();
  const post = async (endpoint, data) => {
    const response = await auth.request.post(backend + "/" + endpoint, {
      data,
      headers: { Origin: backend, "X-StarHub-Portal": "staff" },
    });
    assert.equal(response.status(), 200);
    const body = await response.json();
    assert.equal(body.success, true);
    return body.data;
  };
  await post("api/admin/auth/login", {
    username: "root",
    password: env.STARHUB_STAFF_PASSWORD,
  });
  await post("api/admin/auth/password", {
    current_password: env.STARHUB_STAFF_PASSWORD,
    new_password: "Changed-synthetic-dashboard-123",
  });
  const enrolled = await post("api/admin/auth/enroll", {});
  await post("api/admin/auth/verify", { code: totp(enrolled.secret) });
  const storageState = await auth.storageState();
  const apiResponse = await auth.request.get(
    base + "api/admin/dashboard?range=7d",
    { headers: { "X-StarHub-Portal": "staff" } },
  );
  assert.equal(apiResponse.status(), 200);
  const report = (await apiResponse.json()).data;
  assert.equal(report.top_models.length, 20);
  assert.equal(report.models.length, 25);
  assert.equal(report.top_accounts.length, 20);
  assert.equal(report.totals.active_accounts, 25);
  assert.equal(report.totals.paying_accounts, 8);
  for (const lang of languages)
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const context = await browser.newContext({
          storageState,
          viewport: { width, height: 1000 },
          reducedMotion: "reduce",
        });
        await context.addInitScript(
          ({ lang, theme }) => {
            localStorage.setItem("xy-language", lang);
            localStorage.setItem("xy-theme", theme);
          },
          { lang, theme },
        );
        const page = await context.newPage();
        livePage = page;
        page.setDefaultTimeout(15000);
        page.on("pageerror", (error) => errors.push(error.message));
        if (lang === "zh" && theme === "light" && width === 1440)
          await page.clock.install();
        let requests = 0;
        page.on("request", (request) => {
          if (new URL(request.url()).pathname === "/api/admin/dashboard")
            requests++;
        });
        await page.goto(base + "admin?tab=analytics");
        await page.locator(".op-kpis").waitFor();
        assert.equal(await page.locator(".op-kpi").count(), 7);
        assert.equal(
          await page.locator(".op-ranking").first().locator("li").count(),
          20,
        );
        assert.equal(
          await page.locator(".op-ranking").last().locator("li").count(),
          20,
        );
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          true,
        );
        assert.equal(
          await page.locator("html").getAttribute("dir"),
          lang === "ar" ? "rtl" : "ltr",
        );
        const screenshot = `dashboard-${lang}-${theme}-${width}.png`;
        await page.screenshot({
          path: path.join(out, screenshot),
          fullPage: true,
        });
        const vendor = page.locator(".op-distribution").nth(1);
        await vendor.getByRole("button").click();
        assert.equal(await vendor.locator(".op-distribution-row").count(), 8);
        const modelToggle = page.locator(".op-rankings > div > button");
        await modelToggle.click();
        const modelDistribution = page.locator(
          ".op-dashboard > .op-distribution",
        );
        await modelDistribution.getByRole("button").click();
        assert.equal(
          await modelDistribution.locator(".op-distribution-row").count(),
          25,
        );
        await modelToggle.click();
        const todayResponse = page.waitForResponse(
          (response) =>
            response.url().includes("/api/admin/dashboard?range=today") &&
            response.status() === 200,
        );
        await page.locator(".op-periods button").first().click();
        await todayResponse;
        await page.locator(".op-kpis").waitFor();
        const monthResponse = page.waitForResponse(
          (response) =>
            response.url().includes("/api/admin/dashboard?range=30d") &&
            response.status() === 200,
        );
        await page.locator(".op-periods button").nth(2).click();
        await monthResponse;
        await page.locator(".op-kpis").waitFor();
        await page.locator(".op-periods button").last().click();
        await page.locator(".op-custom input").first().fill(report.start_date);
        await page.locator(".op-custom input").last().fill(report.end_date);
        await page.locator(".op-custom button").click();
        await page.locator(".op-kpis").waitFor();
        assert.ok(page.url().includes("range=custom"));
        await page.locator(".op-method summary").click();
        assert.equal(await page.locator(".op-method dl > div").count(), 4);
        const before = requests;
        await page.locator(".op-toolbar > button").click();
        await page.waitForFunction(
          () => !document.querySelector(".op-toolbar > button").disabled,
        );
        assert.ok(requests > before);
        if (lang === "zh" && theme === "light" && width === 1440) {
          const refresh = page.waitForRequest(
            (request) =>
              new URL(request.url()).pathname === "/api/admin/dashboard",
          );
          await page.clock.fastForward(300001);
          await refresh;
          await page.locator(".op-kpis").waitFor();
          await page.clock.resume();
          await page.locator(".op-ranking a").first().click();
          await page.waitForURL(/tab=customers.*customer=/);
          await page.locator(".adm-heading").waitFor();
        }
        results.push({ lang, theme, width, screenshot, passed: true });
        console.log(`PASS ${lang} ${theme} ${width}`);
        await context.close();
      }
  const edge = await browser.newContext({ storageState });
  const page = await edge.newPage();
  livePage = page;
  await edge.addInitScript(() => {
    localStorage.setItem("xy-language", "zh");
    localStorage.setItem("xy-theme", "light");
  });
  await page.goto(
    base +
      "admin?tab=analytics&range=custom&start_date=2020-01-01&end_date=2020-01-01",
  );
  await page.locator(".op-kpis").waitFor();
  assert.equal(await page.locator(".op-kpi strong").nth(3).innerText(), "—");
  assert.equal(await page.locator(".op-ranking li").count(), 0);
  await page.route(
    (url) => url.pathname === "/api/admin/dashboard",
    (route) =>
      route.fulfill({
        status: 500,
        json: { success: false, message: "synthetic server failure" },
      }),
  );
  await page.locator(".op-toolbar > button").click();
  await page.getByRole("alert").waitFor();
  assert.equal(await page.locator(".op-kpis").count(), 0);
  await page.screenshot({
    path: path.join(out, "dashboard-error.png"),
    fullPage: true,
  });
  await edge.close();
  assert.deepEqual(errors, []);
} catch (error) {
  if (livePage && !livePage.isClosed()) {
    await livePage.screenshot({
      path: path.join(out, "failure.png"),
      fullPage: true,
    });
    await writeFile(
      path.join(out, "failure.txt"),
      livePage.url() + "\n" + (await livePage.locator("body").innerText()),
    );
  }
  throw error;
} finally {
  await writeFile(
    path.join(
      out,
      process.env.DASHBOARD_LANG
        ? `report-${process.env.DASHBOARD_LANG}.json`
        : "report.json",
    ),
    JSON.stringify(
      {
        source:
          "Actual isolated backend and SQLite with synthetic test accounts, receipts and settled-call fixtures",
        results,
        errors,
      },
      null,
      2,
    ),
  );
  await browser?.close();
  await vite?.close();
  child.kill();
  await new Promise((resolve) => child.once("exit", resolve));
  const resolved = path.resolve(temp);
  if (resolved.startsWith(path.resolve(out) + path.sep))
    await rm(resolved, {
      recursive: true,
      force: true,
      maxRetries: 5,
      retryDelay: 200,
    });
}
console.log(JSON.stringify({ combinations: results.length, errors }));
