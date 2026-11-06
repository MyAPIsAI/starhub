import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

// Synthetic fixtures never modify the running deployment or real credentials.
const web = fileURLToPath(new URL("../", import.meta.url));
const out = path.resolve(web, "../output/prd-scope-browser");
await mkdir(out, { recursive: true });
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
const results = [],
  errors = [],
  retiredRequests = [];
let browser;
try {
  await vite.listen();
  const base = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ channel: "chrome", headless: true });
  for (const lang of ["zh", "en", "ar"])
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
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const p = new URL(route.request().url()).pathname;
            if (/task-plugin|redemption|user\/redeem/.test(p))
              retiredRequests.push(p);
            let data = [];
            if (p.endsWith("/auth/self") || p === "/api/user/self")
              data = {
                id: 1,
                username: "root",
                role: 100,
                account_kind: "staff",
                platform_admin: true,
                status: 1,
                group: "default",
                quota: 0,
                used_quota: 0,
              };
            else if (p === "/api/option/status")
              data = {
                database_version: 1,
                loaded_version: 1,
                in_sync: true,
                instances: [],
              };
            else if (p === "/api/metrics") data = { channel_stats: [] };
            else if (p === "/api/wallet")
              data = {
                user_id: 1,
                balance: 0,
                balance_nano: "0",
                api_balance_nano: "0",
                month_spend_nano: "0",
                timezone: "Asia/Dubai",
              };
            else if (p === "/api/admin/dashboard") {
              await route.fulfill({
                status: 503,
                json: {
                  success: false,
                  message: "Synthetic dashboard unavailable",
                },
              });
              return;
            }
            await route.fulfill({ json: { success: true, data } });
          },
        );
        const page = await context.newPage();
        page.setDefaultTimeout(10000);
        page.on("pageerror", (error) => errors.push(error.message));
        await page.goto(base + "admin?tab=resources");
        await page.locator('[data-staff-page="resources"]').waitFor();
        if (width < 600)
          await page.locator(".staff-shell-topbar .staff-icon-button").click();
        const nav = page.locator(
          width < 600 ? ".staff-navigation-dialog" : ".staff-shell-sidebar",
        );
        await nav.waitFor();
        const navText = await nav.innerText();
        assert.doesNotMatch(
          navText,
          /视频插件|兑换码|更多管理工具|Video plugins|Redemption codes|More management tools|إضافات الفيديو|رموز الاسترداد|أدوات الإدارة الإضافية/,
        );
        assert.equal(await nav.locator(".staff-shell-group").count(), 4);
        await page.screenshot({
          path: path.join(out, `navigation-${lang}-${theme}-${width}.png`),
        });
        if (width < 600) await page.keyboard.press("Escape");
        await page
          .getByRole("button", {
            name: { zh: "新建渠道", en: "New channel", ar: "قناة جديدة" }[lang],
            exact: true,
          })
          .click();
        await page.locator("#ch-type").waitFor();
        assert.deepEqual(
          await page
            .locator("#ch-type option")
            .evaluateAll((options) => options.map((o) => o.value)),
          ["1", "14", "15", "25"],
        );
        assert.equal(await page.locator("#ch-plugin").count(), 0);
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          true,
        );
        await page.screenshot({
          path: path.join(out, `channel-${lang}-${theme}-${width}.png`),
        });
        await page.keyboard.press("Escape");
        for (const tab of ["plugins", "redemption"]) {
          await page.goto(base + "admin?tab=" + tab);
          await page.locator('[data-staff-page="analytics"]').waitFor();
          await page.waitForURL((url) => !url.searchParams.has("tab"));
        }
        await page.goto(base + "console/billing");
        await page.locator(".bc-wallet-card").waitFor();
        await page.locator(".bc-month-card").waitFor();
        assert.equal(
          await page.locator("#redeem-code-input, .bc-redeem-card").count(),
          0,
        );
        assert.equal(
          await page.locator(".bc-summary-grid > section").count(),
          3,
        );
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          true,
        );
        await page.screenshot({
          path: path.join(out, `billing-${lang}-${theme}-${width}.png`),
        });
        results.push({ lang, theme, width, passed: true });
        await context.close();
      }
  assert.deepEqual(errors, []);
  assert.deepEqual(retiredRequests, []);
  console.log(
    JSON.stringify({ passed: results.length, errors, retiredRequests }),
  );
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify({ results, errors, retiredRequests }, null, 2),
  );
  await browser?.close();
  await vite.close();
}
