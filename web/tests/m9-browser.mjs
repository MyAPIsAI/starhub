import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile, readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";
import crypto from "node:crypto";
import net from "node:net";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url)),
  project = path.resolve(web, "..");
const out = path.join(project, "output", "m9-browser");
const require = createRequire(import.meta.url);
const { PNG } = require(
  path.join(
    path.dirname(require.resolve("playwright-core/package.json")),
    "lib/utilsBundle.js",
  ),
);
const baselines = path.join(web, "tests", "baselines", "m9");
await mkdir(baselines, { recursive: true });
const comparisons = [];
async function capture(page, name) {
  await page.waitForLoadState("networkidle");
  await page.evaluate(() => document.fonts.ready);
  const actual = await page.screenshot({
    path: path.join(out, name + ".png"),
    animations: "disabled",
  });
  const baseline = path.join(baselines, name + ".png");
  if (process.env.M9_UPDATE_SCREENSHOTS === "1" && !process.env.CI) {
    await writeFile(baseline, actual);
    return;
  }
  const expected = PNG.sync.read(await readFile(baseline)),
    current = PNG.sync.read(actual);
  assert.equal(current.width, expected.width, name + " width");
  assert.equal(current.height, expected.height, name + " height");
  let changed = 0;
  for (let i = 0; i < current.data.length; i += 4) {
    if (
      Math.max(
        ...[0, 1, 2].map((c) =>
          Math.abs(current.data[i + c] - expected.data[i + c]),
        ),
      ) > 55
    )
      changed++;
  }
  const ratio = changed / (current.width * current.height);
  comparisons.push({ name, changedRatio: ratio });
  assert.ok(
    ratio < 0.015,
    `${name}: ${(ratio * 100).toFixed(2)}% pixels changed; inspect current screenshot before updating baseline`,
  );
}
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-")),
  binary = path.join(temp, "announcement-check.exe");
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
  GOCACHE: path.join(project, "output", "go-build"),
  GOFLAGS: "-buildvcs=false",
  STARHUB_CONFIG: path.join(temp, "fixture.yaml"),
  STARHUB_DB: path.join(temp, "notices.db"),
  STARHUB_PORT: new URL(backend).port,
  STARHUB_COOKIE_SECURE: "false",
  STARHUB_STAFF_SETUP_MODE: "commercial",
  STARHUB_STAFF_ALLOWED_IPS: "127.0.0.1/32,::1/128",
  STARHUB_TRUSTED_PROXIES: "",
  STARHUB_STAFF_PASSWORD: "Synthetic-notice-root-123",
  STARHUB_ROOT_PASSWORD: "",
};
// The matrix shares one loopback IP; raise only the isolated fixture's request budget.
await writeFile(
  env.STARHUB_CONFIG,
  "version: 1\ninitial_options:\n  ip_rate_limit: 10000\n  user_rate_limit: 10000\n",
);
for (const name of [
  "SNAPSHOT_URL",
  "SNAPSHOT_TOKEN",
  "EVENTS_URL",
  "HEALTH_SNAPSHOT_URL",
  "INCIDENTS_URL",
  "CA_FILE",
  "CLIENT_CERT_FILE",
  "CLIENT_KEY_FILE",
])
  env[`STARHUB_RESOURCE_${name}`] = "";
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: project,
  env,
  stdio: "pipe",
  windowsHide: true,
});
execFileSync(binary, ["staff", "init", "--username", "root"], {
  cwd: temp,
  env,
  stdio: "pipe",
  windowsHide: true,
});
const seed = path.join(temp, "seed.go");
await writeFile(
  seed,
  `package main
import (
  "fmt"
  "os"
  "starhub/model"
)
func main() {
  if err := model.InitDB(os.Getenv("STARHUB_DB")); err != nil { panic(err) }
  defer model.CloseDB()
  rows := []model.StandardModel{
    {Model: "fixture-model", Name: "Fixture model", Vendor: "Fixture vendor", Kind: "chat", Protocols: []string{"OpenAI"}},
    {Model: "secondary-model", Name: "Secondary model", Vendor: "Second vendor", Kind: "chat", Protocols: []string{"OpenAI"}},
  }
  for i := 0; i < 42; i++ {
    rows = append(rows, model.StandardModel{Model: fmt.Sprintf("fixture-bulk-%02d", i), Name: fmt.Sprintf("Bulk fixture %02d", i), Vendor: "Fixture vendor", Kind: "chat", Protocols: []string{"OpenAI"}})
  }
  if err := model.DB.Create(&rows).Error; err != nil { panic(err) }
}
`,
);
execFileSync("go", ["run", seed], {
  cwd: project,
  env,
  stdio: "pipe",
  windowsHide: true,
});
for (const name of ["ADDRESS", "FROM", "USERNAME", "PASSWORD"])
  env[`STARHUB_SMTP_${name}`] = "";
const child = spawn(binary, [], {
  cwd: temp,
  env,
  windowsHide: true,
  stdio: ["ignore", "ignore", "pipe"],
});
let serverErrors = "";
child.stderr.on("data", (v) => (serverErrors += v));
function totp(secret) {
  let bits = "";
  for (const c of secret)
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
}
let vite, browser, lastPage;
const results = [],
  errors = [];
try {
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(backend + "/api/admin/install")).ok) break;
    } catch {}
    if (i === 99) throw Error("isolated backend failed to start");
    await new Promise((r) => setTimeout(r, 200));
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
  browser = await chromium.launch({
    channel: process.env.CI ? undefined : "chrome",
    headless: true,
  });
  const post = async (context, endpoint, data) => {
    const response = await context.request.post(backend + endpoint, {
      data,
      headers: { Origin: backend, "X-StarHub-Portal": "staff" },
    });
    assert.equal(response.status(), 200, await response.text());
    const value = await response.json();
    assert.equal(value.success, true);
    return value.data;
  };
  const enroll = async (context, username, password) => {
    await post(context, "/api/admin/auth/login", { username, password });
    await post(context, "/api/admin/auth/password", {
      current_password: password,
      new_password: "Changed-synthetic-notice-123",
    });
    const enrolled = await post(context, "/api/admin/auth/enroll", {});
    await post(context, "/api/admin/auth/verify", {
      code: totp(enrolled.secret),
    });
  };

  const api = async (context, method, url, data, expected = 200) => {
    const response = await context.request.fetch(backend + url, {
      method,
      data,
      headers: { Origin: backend, "X-StarHub-Portal": "workspace" },
    });
    assert.equal(response.status(), expected, await response.text());
    const value = await response.json();
    return value.data;
  };
  const pick = async (page, name) => {
    await page
      .locator('.product-preferences [role="combobox"]')
      .first()
      .click();
    await page.getByRole("option", { name, exact: true }).click();
  };
  for (const [locale, expected] of [
    ["fr-FR", "en"],
    ["ar-AE", "ar"],
    ["zh-CN", "zh"],
  ]) {
    const context = await browser.newContext({ locale });
    const page = await context.newPage();
    await page.goto(base + "docs");
    await page.locator(".product-preferences").first().waitFor();
    assert.equal(await page.locator("html").getAttribute("lang"), expected);
    assert.equal(
      await page.locator("html").getAttribute("dir"),
      expected === "ar" ? "rtl" : "ltr",
    );
    if (expected === "ar")
      await page.waitForFunction(() => {
        const el = document.querySelector("pre");
        return el && getComputedStyle(el).direction === "ltr";
      });
    results.push({
      case: locale === "fr-FR" ? "I18N-AC-01" : "I18N-AC-02",
      locale,
      passed: true,
    });
    await context.close();
  }
  const first = await browser.newContext({ locale: "en-US" });
  const user = await api(first, "POST", "/api/user/register", {
    username: "m9-customer",
    password: "Synthetic-customer-123",
    email: "m9@example.test",
  });
  await api(first, "POST", "/api/user/login", {
    username: "m9-customer",
    password: "Synthetic-customer-123",
  });
  const firstPage = await first.newPage();
  await firstPage.goto(base + "console");
  await firstPage.locator(".product-preferences").first().waitFor();
  await pick(firstPage, "中文");
  await firstPage.waitForFunction(() => document.documentElement.lang === "zh");
  assert.equal(
    (await api(first, "GET", "/api/user/self")).preferred_language,
    "zh",
  );
  const second = await browser.newContext({ locale: "fr-FR" });
  await api(second, "POST", "/api/user/login", {
    username: "m9-customer",
    password: "Synthetic-customer-123",
  });
  const secondPage = await second.newPage();
  await secondPage.goto(base + "console");
  await secondPage.waitForFunction(
    () => document.documentElement.lang === "zh",
  );
  await api(
    second,
    "PUT",
    "/api/user/language",
    { expected_user_id: user.id + 100, language: "en" },
    409,
  );
  await api(
    second,
    "PUT",
    "/api/user/language",
    { expected_user_id: user.id, language: "ko" },
    400,
  );
  results.push({
    case: "I18N-AC-03",
    checks: [
      "real account language write",
      "fresh second-device login",
      "owner and language validation",
    ],
    passed: true,
  });
  const root = await browser.newContext({ locale: "ar-AE" });
  await enroll(root, "root", env.STARHUB_STAFF_PASSWORD);
  const admin = await root.newPage();
  await admin.goto(base + "admin?tab=incidents");
  await admin.locator(".announcement-admin").waitFor();
  assert.equal(await admin.locator("html").getAttribute("lang"), "en");
  await admin.locator('.product-preferences [role="combobox"]').click();
  assert.deepEqual(await admin.getByRole("option").allTextContents(), [
    "English",
    "中文",
  ]);
  await admin.keyboard.press("Escape");
  await admin
    .getByRole("button", { name: "Publish notice", exact: true })
    .click();
  assert.equal(await admin.locator("#notice-title-ar").count(), 1);
  await admin.keyboard.press("Escape");
  const apiError = await second.request.get(backend + "/v1/models", {
    headers: { "Accept-Language": "ar" },
  });
  assert.equal(apiError.status(), 401);
  assert.equal(
    (await apiError.json()).error.message,
    "missing Authorization: Bearer header",
  );
  results.push({
    case: "I18N-AC-12",
    checks: [
      "two staff interface languages",
      "three announcement content languages",
      "English API errors",
    ],
    passed: true,
  });
  await api(second, "PUT", "/api/user/language", {
    expected_user_id: user.id,
    language: "ar",
  });
  const storageState = await second.storageState();
  const stats = (id, requests, amount) => ({
    id,
    name: id,
    requests,
    successful: requests,
    input_tokens: 12,
    output_tokens: 4,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    amount_nano: amount,
  });
  const report = {
    timezone: "Asia/Dubai",
    start_date: "2026-11-01",
    end_date: "2026-11-03",
    group_by: "model",
    granularity: "day",
    totals: stats("total", 6, "12340000000"),
    rows: [stats("fixture-model", 6, "12340000000")],
    by_key: [stats("Key one", 6, "12340000000")],
    by_model: [stats("fixture-model", 6, "12340000000")],
    series: [
      stats("2026-11-01T00:00:00Z", 1, "1000000000"),
      stats("2026-11-02T00:00:00Z", 2, "2340000000"),
      stats("2026-11-03T00:00:00Z", 3, "9000000000"),
    ],
    options: {},
  };
  for (const theme of ["light", "dark"])
    for (const width of [1440, 390]) {
      const context = await browser.newContext({
        storageState,
        locale: "ar-AE",
        viewport: { width, height: 1100 },
        reducedMotion: "reduce",
      });
      await context.addInitScript(
        (theme) => localStorage.setItem("xy-theme", theme),
        theme,
      );
      const page = await context.newPage();
      lastPage = page;
      page.on("pageerror", (error) => errors.push(error.message));
      await page.clock.install({ time: new Date("2026-11-06T12:00:00Z") });
      await page.route("**/api/user/reports**", (route) =>
        route.fulfill({ json: { success: true, data: report } }),
      );
      await page.route("**/api/log?**", (route) =>
        route.fulfill({
          json: {
            success: true,
            data: {
              items: [
                {
                  id: 1,
                  user_id: user.id,
                  token_id: 1,
                  token_name: "Fixture key",
                  model: "fixture-model",
                  trace_id: "trace_m9_0123456789",
                  created_at: 1793966400,
                  status: 1,
                  amount_nano: "12340000000",
                  prompt_tokens: 12,
                  completion_tokens: 4,
                  duration_ms: 250,
                  quota: 0,
                  unfunded_quota: 0,
                },
              ],
              total: 1,
              page: 1,
              page_size: 20,
            },
          },
        }),
      );
      await page.goto(base + "console/usage");
      await page.locator(".usage-trend .recharts-surface").first().waitFor();
      await page
        .locator(".recharts-xAxis .recharts-cartesian-axis-tick")
        .first()
        .waitFor();
      const ticks = await page
        .locator(".recharts-xAxis .recharts-cartesian-axis-tick")
        .evaluateAll((rows) =>
          rows.map((row) => row.getBoundingClientRect().x),
        );
      assert.ok(
        ticks.length >= 2 && ticks[0] > ticks.at(-1),
        "Arabic time axis must run from right to left",
      );
      assert.equal(await page.locator("html").getAttribute("dir"), "rtl");
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      );
      await page.evaluate(() => document.fonts.ready);
      await capture(page, "usage-ar-" + theme + "-" + width);
      await page.locator(".usage-trend").scrollIntoViewIfNeeded();
      await capture(page, "chart-ar-" + theme + "-" + width);
      await page.goto(base + "console/requests");
      await page.locator(".request-history-table").waitFor();
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      );
      assert.equal(
        await page
          .locator(".request-trace-button")
          .evaluate((e) => getComputedStyle(e).direction),
        "ltr",
      );
      await capture(page, "requests-ar-" + theme + "-" + width);
      await page.locator(".request-trace-button").click();
      await page.getByRole("dialog").waitFor();
      assert.equal(
        await page
          .getByRole("dialog")
          .evaluate((e) => getComputedStyle(e).direction),
        "rtl",
      );
      await capture(page, "drawer-ar-" + theme + "-" + width);
      await page.keyboard.press("Escape");
      await page.goto(base + "console/billing");
      await page.locator(".bc-page").waitFor();
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      );
      await capture(page, "balance-ar-" + theme + "-" + width);
      await page.locator(".bc-help-button").click();
      await page.getByRole("dialog").waitFor();
      assert.equal(
        await page
          .getByRole("dialog")
          .evaluate((e) => getComputedStyle(e).direction),
        "rtl",
      );
      await capture(page, "modal-ar-" + theme + "-" + width);
      await page.keyboard.press("Escape");
      await page.goto(base + "docs");
      await page.locator("pre").first().waitFor();
      await page.waitForFunction(() => {
        const pre = document.querySelector("pre");
        return pre && getComputedStyle(pre).direction === "ltr";
      });
      await capture(page, "docs-ar-" + theme + "-" + width);
      for (const route of ["", "keys", "connect", "service-quality", "smart-routing", "playground", "exports", "settings"]) {
        await page.goto(base + "console/" + route);
        await page.locator(".product-preferences").first().waitFor();
        await page.waitForLoadState("networkidle");
        assert.equal(await page.locator("html").getAttribute("dir"), "rtl", route);
        assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), route + " overflows viewport");
        await page.screenshot({path: path.join(out, `page-${route || "overview"}-ar-${theme}-${width}.png`), animations: "disabled"});
      }
      results.push({ case: "I18N-AC-04/05/13", theme, width, passed: true });
      await context.close();
    }
  await first.close();
  await second.close();
  await root.close();
  assert.deepEqual(errors, []);
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify({ results, comparisons, errors }, null, 2),
  );
  console.log(JSON.stringify({ passed: results.length, errors }));
} catch (error) {
  if (lastPage && !lastPage.isClosed())
    await lastPage
      .screenshot({ path: path.join(out, "failure.png") })
      .catch(() => {});
  throw error;
} finally {
  await browser?.close();
  await vite?.close();
  child.kill();
}
