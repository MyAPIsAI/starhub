import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import net from "node:net";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url)),
  project = path.resolve(web, "..");
const out = path.join(project, "output", "ui25", "mobile-browser");
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-")),
  binary = path.join(temp, "mobile-check.exe");
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
  STARHUB_DB: path.join(temp, "mobile.db"),
  STARHUB_PORT: new URL(backend).port,
  STARHUB_COOKIE_SECURE: "false",
  STARHUB_STAFF_SETUP_MODE: "commercial",
  STARHUB_STAFF_ALLOWED_IPS: "127.0.0.1/32,::1/128",
  STARHUB_TRUSTED_PROXIES: "",
  STARHUB_STAFF_PASSWORD: "Synthetic-mobile-root-123",
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
let vite, browser, page;
const report = {
  fixtureData: true,
  realBackend: true,
  mockedUsagePaginationAndRetry: true,
  pages: [],
  errors: [],
};
try {
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(backend + "/api/admin/install")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 200));
    if (i === 99) throw Error("backend startup");
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
  const fixtures = {};
  for (const language of ["zh", "en", "ar"]) {
    const context = await browser.newContext();
    const api = async (method, url, data) => {
      const response = await context.request.fetch(backend + url, {
        method,
        data,
        headers: { Origin: backend, "X-StarHub-Portal": "workspace" },
      });
      assert.equal(response.status(), 200, await response.text());
      return (await response.json()).data;
    };
    const user = await api("POST", "/api/user/register", {
      username: "mobile-" + language,
      password: "Synthetic-mobile-password-123",
    });
    await api("POST", "/api/user/login", {
      username: user.username,
      password: "Synthetic-mobile-password-123",
    });
    await api("PUT", "/api/user/language", {
      expected_user_id: user.id,
      language,
    });
    for (let i = 0; i < 23; i++)
      await api("POST", "/api/token", {
        name: (i === 0 ? "Long-key-".repeat(5) : "Mobile key ") + i,
        group: "default",
        model_limits: "",
        smart_access: false,
        unlimited_quota: true,
      });
    fixtures[language] = await context.storageState();
    await context.close();
  }
  const routes = [
    "",
    "keys",
    "connect",
    "playground",
    "service-quality",
    "usage",
    "requests",
    "billing",
    "billing/recharge",
    "exports",
    "settings",
    "smart-routing",
  ];
  for (const language of process.env.UI25_LANGUAGE
    ? [process.env.UI25_LANGUAGE]
    : ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [320, 390]) {
        const name = `${language}-${theme}-${width}`;
        const context = await browser.newContext({
          storageState: fixtures[language],
          viewport: { width, height: 900 },
          reducedMotion: "reduce",
        });
        await context.addInitScript(
          ({ language, theme }) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          { language, theme },
        );
        page = await context.newPage();
        page.setDefaultTimeout(12000);
        page.on("pageerror", (e) =>
          report.errors.push({ name, error: e.message }),
        );
        const fit = async (label) => {
          const size = await page.evaluate(() => ({
            width: innerWidth,
            scroll: document.documentElement.scrollWidth,
            overflow: [...document.querySelectorAll("body *")]
              .filter((e) => {
                const r = e.getBoundingClientRect();
                return (
                  getComputedStyle(e).visibility !== "hidden" &&
                  !e.closest(".xy-sidebar") &&
                  r.width > 0 &&
                  (r.left < -1 || r.right > innerWidth + 1)
                );
              })
              .slice(0, 30)
              .map((e) => ({
                tag: e.tagName,
                cls: e.className,
                left: e.getBoundingClientRect().left,
                right: e.getBoundingClientRect().right,
                position: getComputedStyle(e).position,
                parent: e.parentElement?.outerHTML.slice(0, 600),
              })),
          }));
          assert.ok(
            size.scroll <= size.width + 1,
            `${name} ${label}: ${JSON.stringify(size)}`,
          );
        };
        for (const route of routes) {
          await page.goto(base + "console/" + route, {
            waitUntil: "domcontentloaded",
          });
          await page.locator(".xy-main").waitFor();
          await page.waitForLoadState("networkidle");
          await fit(route);
          await page.locator(".xy-menu-toggle").click();
          await page.locator(".xy-sidebar.is-open").waitFor();
          await fit("navigation");
          await page.locator(".xy-mobile-close").click();
          assert.equal(
            await page.locator("html").getAttribute("dir"),
            language === "ar" ? "rtl" : "ltr",
          );
          if (route === "keys") {
            await page.locator(".key-list-row").first().waitFor();
            assert.equal(await page.locator(".key-list-row").count(), 20);
            await page.locator(".list-pagination button").last().click();
            assert.equal(await page.locator(".key-list-row").count(), 3);
            await fit("keys second page");
            await page
              .locator('.key-list-row [data-key-action="details"]')
              .first()
              .click();
            await page.locator("[data-key-detail]").waitFor();
            await fit("key details");
            await page.keyboard.press("Escape");
            await page.locator(".list-pagination button").first().click();
            assert.equal(await page.locator(".key-list-row").count(), 20);
            await page.goto(base + "console/keys?create=1");
            await page.getByRole("dialog").waitFor();
            await fit("key form");
            await page.screenshot({
              path: path.join(out, name + "-key-form.png"),
              fullPage: true,
            });
            await page.keyboard.press("Escape");
          }
          if (route === "usage") {
            let fail = true;
            const stats = (i) => ({
              id: String(i),
              name: "Synthetic usage " + i,
              requests: 2,
              successful: 1,
              input_tokens: 10,
              output_tokens: 10,
              cache_read_tokens: 0,
              cache_write_tokens: 0,
              amount_nano: "10000",
            });
            await page.route("**/api/user/reports?*", async (route) => {
              const params = new URL(route.request().url()).searchParams;
              const rows = Array.from({ length: 23 }, (_, i) => stats(i));
              await route.fulfill({
                status: fail ? 503 : 200,
                contentType: "application/json",
                body: JSON.stringify(
                  fail
                    ? { success: false, message: "Synthetic retry condition" }
                    : {
                        success: true,
                        data: {
                          timezone: "Asia/Dubai",
                          start_date: "2026-11-01",
                          end_date: "2026-11-07",
                          group_by: params.get("group_by") || "key",
                          granularity: "day",
                          totals: {
                            ...stats("all"),
                            amount_nano: "230000",
                            requests: 46,
                            successful: 23,
                          },
                          rows,
                          series: [],
                          by_key: rows,
                          by_model: rows,
                          options: {},
                        },
                      },
                ),
              });
            });
            await page.reload();
            await page.locator(".usage-error").waitFor();
            fail = false;
            await page.locator(".usage-error button").click();
            await page
              .locator(".usage-table-scroll tbody tr")
              .first()
              .waitFor();
            assert.equal(
              await page.locator(".usage-table-scroll tbody tr").count(),
              20,
            );
            const tablePager = page
              .locator(".usage-reports .list-pagination")
              .last();
            await tablePager.locator("button").last().click();
            assert.equal(
              await page.locator(".usage-table-scroll tbody tr").count(),
              3,
            );
            assert.equal(
              await page.locator(".usage-table-scroll td[data-label]").count(),
              18,
            );
            await fit("usage populated pagination");
            assert.ok(
              await page
                .locator(".usage-table-scroll")
                .evaluate((el) => el.scrollWidth <= el.clientWidth + 1),
            );
            const groupSelect = page
              .locator(".usage-filters select")
              .filter({ has: page.locator('option[value="vendor"]') });
            await groupSelect.selectOption("model");
            await page.waitForResponse(
              (r) =>
                r.url().includes("/api/user/reports?") &&
                r.url().includes("group_by=model"),
            );
            await page
              .locator(".usage-table-scroll tbody tr")
              .nth(19)
              .waitFor();
            assert.match(await tablePager.innerText(), /1 \/ 2/);
            const sharePager = page
              .locator(".usage-shares .list-pagination")
              .first();
            await sharePager.locator("button").last().click();
            assert.equal(
              await page
                .locator(".usage-shares .usage-card")
                .first()
                .locator(".usage-share")
                .count(),
              3,
            );
            await page.screenshot({
              path: path.join(out, name + "-usage-populated.png"),
              fullPage: true,
            });
            await page.unroute("**/api/user/reports?*");
          }
          if (route === "settings") {
            const tabs = page.locator('[role="tab"]');
            for (let i = 0; i < (await tabs.count()); i++) {
              await tabs.nth(i).click();
              await fit("settings tab " + i);
            }
          }
          await page.screenshot({
            path: path.join(
              out,
              name + "-" + (route || "overview").replaceAll("/", "-") + ".png",
            ),
            fullPage: true,
          });
          report.pages.push({ name, route, passed: true });
        }
        await context.close();
      }
  assert.deepEqual(report.errors, []);
} catch (e) {
  if (page && !page.isClosed())
    await page
      .screenshot({ path: path.join(out, "failure.png"), fullPage: true })
      .catch(() => {});
  throw e;
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify(report, null, 2),
  );
  await browser?.close();
  await vite?.close();
  child.kill();
}
console.log(
  JSON.stringify({ pages: report.pages.length, errors: report.errors }),
);
