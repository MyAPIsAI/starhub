// UI fixtures are isolated from the real backend and complement controller tests.
import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url));
const out = path.resolve(web, "../output/admin-audit-browser");
await mkdir(out, { recursive: true });
const now = Math.floor(Date.now() / 1000);
const baseRow = {
  user_id: 1,
  username: "root",
  actor_role: 100,
  object_type: "staff",
  object_id: "1",
  object_name: "root",
  before_summary: "",
  after_summary: "",
  reason: "",
  ip: "10.20.0.12",
  method: "CHANGE",
  path: "",
  status: 200,
};
const fixtures = [
  { action: "staff_2fa_verified" },
  {
    action: "service_group_multiplier_changed",
    object_type: "service-group",
    object_id: "Partner",
    object_name: "",
    before_summary: '{"component":"group_ratio","value":0.75}',
    after_summary: '{"component":"group_ratio","value":0.7}',
  },
  {
    action: "balance_compensation",
    object_type: "customers",
    object_id: "12",
    object_name: "atlas_ai",
    before_summary: '{"balance_nano":"100000000000"}',
    after_summary:
      '{"balance_nano":"300000000000","amount_nano":"200000000000"}',
    reason: "9/30 上游故障补偿",
  },
  {
    action: "model_pricing_changed",
    object_type: "models",
    object_id: "deepseek-chat",
    object_name: "",
    before_summary: '{"component":"model_ratio","value":0.28}',
    after_summary: '{"component":"model_ratio","value":0.27}',
  },
  {
    action: "customer_disabled",
    object_type: "customers",
    object_id: "13",
    object_name: "palm_logistics",
    before_summary: '{"status":1}',
    after_summary: '{"status":2}',
    reason: "疑似滥用，等待客户回复",
  },
  {
    action: "staff_created",
    object_id: "3",
    object_name: "cwcw",
    after_summary:
      '{"platform_admin":true,"role":5,"status":1,"two_factor_bound":false,"username":"cwcw"}',
  },
  {
    action: "staff_password_authenticated",
    user_id: 3,
    username: "cwcw",
    actor_role: 5,
    object_id: "3",
    object_name: "cwcw",
  },
  ...Array.from({ length: 17 }, (_, i) => ({
    action: "staff_2fa_verified",
    created_at: now - (i + 8) * 86400,
  })),
].map((row, i) => ({
  ...baseRow,
  id: 24 - i,
  created_at: now - i * 3600,
  ...row,
}));
const actions = [...new Set(fixtures.map((row) => row.action))];
const actors = [
  { id: 1, username: "root" },
  { id: 3, username: "cwcw" },
];
const errors = [],
  results = [];
let browser, livePage;
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
try {
  await vite.listen();
  browser = await chromium.launch({ channel: "chrome", headless: true });
  const base = vite.resolvedUrls.local[0];
  for (const lang of (process.env.AUDIT_LANGS || "zh,en,ar").split(","))
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const context = await browser.newContext({
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
        let lastQuery;
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const url = new URL(route.request().url()),
              q = url.searchParams;
            let data = {};
            if (
              url.pathname === "/api/admin/auth/self" ||
              url.pathname === "/api/user/self"
            )
              data = {
                id: 1,
                username: "root",
                role: 100,
                account_kind: "staff",
                platform_admin: true,
                status: 1,
                group: "default",
                email: "",
                quota: 0,
                used_quota: 0,
              };
            else if (url.pathname === "/api/audit") {
              lastQuery = q;
              let rows = fixtures.filter(
                (row) =>
                  (!q.get("actor") || String(row.user_id) === q.get("actor")) &&
                  (!q.get("action") || row.action === q.get("action")) &&
                  (!q.get("from") || row.created_at >= Number(q.get("from"))) &&
                  (!q.get("until") ||
                    row.created_at < Number(q.get("until"))) &&
                  (!q.get("q") ||
                    JSON.stringify(row).includes(q.get("q")) ||
                    q.getAll("search_action").includes(row.action)),
              );
              data = {
                total: rows.length,
                items: rows.slice(
                  (Number(q.get("p")) - 1) * 20,
                  Number(q.get("p")) * 20,
                ),
                actions,
                actors,
              };
            }
            await route.fulfill({ json: { success: true, data } });
          },
        );
        const page = await context.newPage();
        livePage = page;
        page.setDefaultTimeout(10000);
        page.on("pageerror", (error) => errors.push(error.message));
        await page.goto(base + "admin?tab=audit");
        await page.locator(".adm-audit-results tbody tr").first().waitFor();
        assert.equal(await page.locator(".adm-audit-results th").count(), 6);
        assert.equal(
          await page.locator(".adm-audit-results tbody tr").count(),
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
        const text = await page.locator(".adm-audit-results").innerText();
        for (const raw of [
          "staff_2fa_verified",
          "staff_password_authenticated",
          "platform_admin:",
          "two_factor_bound:",
        ])
          assert.ok(!text.includes(raw));
        if (lang === "zh") {
          assert.ok(
            text.includes("超级管理员") &&
              text.includes("登录后台") &&
              text.includes("0.75 → 0.7"),
          );
          await page
            .locator(".adm-audit-results details summary")
            .first()
            .click();
          assert.ok(
            (
              await page.locator(".adm-audit-results details[open]").innerText()
            ).includes("未绑定"),
          );
          await page
            .locator(".adm-audit-results details summary")
            .first()
            .click();
        }
        const screenshot = `audit-${lang}-${theme}-${width}.png`;
        await page.locator(".adm-table-scroll").evaluate((element) => {
          element.scrollLeft = 0;
        });
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: path.join(out, screenshot),
          fullPage: false,
        });
        await page.locator(".adm-audit-pagination button").last().click();
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-audit-results tbody tr").length ===
            4,
        );
        assert.equal(lastQuery.get("p"), "2");
        await page.locator(".adm-audit-search input").fill("palm_logistics");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-audit-results tbody tr").length ===
            1,
        );
        assert.equal(lastQuery.get("p"), "1");
        assert.equal(lastQuery.get("q"), "palm_logistics");
        await page.locator(".adm-audit-search input").fill("no-match-object");
        await page.locator(".adm-audit-empty").waitFor();
        assert.equal(
          await page
            .locator(".adm-audit-pagination button:not([disabled])")
            .count(),
          0,
        );
        await page.locator(".adm-audit-search input").fill("");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-audit-results tbody tr").length ===
            20,
        );
        await page
          .locator(".adm-audit-filters select")
          .nth(1)
          .selectOption("3");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-audit-results tbody tr").length ===
            1,
        );
        assert.equal(lastQuery.get("actor"), "3");
        await page.locator(".adm-audit-filters select").nth(1).selectOption("");
        await page
          .locator(".adm-audit-filters select")
          .nth(0)
          .selectOption("staff_2fa_verified");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-audit-results tbody tr").length ===
            18,
        );
        await page
          .locator(".adm-audit-filters select")
          .nth(2)
          .selectOption("1");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-audit-results tbody tr").length ===
            1,
        );
        assert.ok(Number(lastQuery.get("from")) > now - 86500);
        await page.locator(".adm-table-scroll").focus();
        assert.equal(
          await page
            .locator(".adm-table-scroll")
            .evaluate((element) => element === document.activeElement),
          true,
        );
        results.push({ lang, theme, width, screenshot, passed: true });
        console.log(`PASS ${lang} ${theme} ${width}`);
        await context.close();
      }
  assert.deepEqual(errors, []);
} catch (error) {
  if (livePage && !livePage.isClosed()) {
    await livePage.screenshot({ path: path.join(out, "failure.png") });
    await writeFile(
      path.join(out, "failure.txt"),
      livePage.url() + "\n" + (await livePage.locator("body").innerText()),
    );
  }
  throw error;
} finally {
  await writeFile(
    path.join(out, process.env.AUDIT_LANGS ? `report-${process.env.AUDIT_LANGS}.json` : "report.json"),
    JSON.stringify(
      {
        dataSource:
          "Isolated API fixtures; backend search and permissions covered by Go tests",
        results,
        errors,
      },
      null,
      2,
    ),
  );
  await browser?.close();
  await vite.close();
}
console.log(JSON.stringify({ combinations: results.length, errors }));
