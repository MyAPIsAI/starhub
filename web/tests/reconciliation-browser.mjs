import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

// Synthetic HTTP fixtures exercise the UI independently of real accounts.
const web = fileURLToPath(new URL("../", import.meta.url));
const out = path.resolve(web, "../output/reconciliation-browser");
await mkdir(out, { recursive: true });
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
const results = [],
  errors = [];
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
        let phase = "normal",
          posted = false;
        const row = {
          id: 1,
          date: "2026-11-04",
          user_id: 12,
          username: "reconciliation_customer",
          timezone: "Asia/Dubai",
          usage_nano: "1000000000",
          api_ledger_nano: "1000000000",
          order_nano: "0",
          recharge_ledger_nano: "0",
          balance_expected_nano: "1000000000",
          balance_actual_nano: "1000000001",
          reserved_nano: "0",
          differences: [
            {
              kind: "balance_vs_ledger",
              funding_source: "api_quota",
              expected_nano: "1000000000",
              actual_nano: "1000000001",
              delta_nano: "1",
            },
          ],
          provisional: true,
          created_at: 1793847600,
          external_status: "not_imported",
          imported_sources: [],
          checks_version: 2,
        };
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const request = route.request(),
              url = new URL(request.url());
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
                quota: 0,
                used_quota: 0,
              };
            else if (url.pathname === "/api/admin/customers")
              data = { items: [{ id: 12, username: row.username }], total: 1 };
            else if (url.pathname === "/api/admin/reconciliation/statements") {
              if (request.method() === "POST") {
                const body = request.postDataJSON();
                assert.equal(body.provider, "bank-a");
                assert.ok(body.csv.includes("1000,USD"));
                posted = true;
                row.external_status = "imported_scope";
                data = { id: 1 };
              } else data = { items: [] };
            } else if (url.pathname === "/api/admin/reconciliation") {
              if (phase === "error") {
                await route.fulfill({
                  status: 500,
                  json: { success: false, message: "Synthetic report failure" },
                });
                return;
              }
              data = {
                items: phase === "empty" ? [] : [row],
                timezone: "Asia/Dubai",
                end: "2026-11-05",
                alert_webhook_configured: false,
                pending_alerts: 1,
              };
            }
            await route.fulfill({ json: { success: true, data } });
          },
        );
        const page = await context.newPage();
        page.setDefaultTimeout(12000);
        page.on("pageerror", (e) => errors.push(e.message));
        await page.goto(base + "admin?tab=finance");
        await page.locator(".adm-reconciliation tbody tr").waitFor();
        assert.ok(
          (
            await page.locator(".adm-reconciliation tbody").innerText()
          ).includes(
            { zh: "存在差异", en: "Differences found", ar: "توجد فروقات" }[
              lang
            ],
          ),
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
        await page.screenshot({
          path: path.join(out, `reports-${lang}-${theme}-${width}.png`),
        });
        const detail = page.locator(".adm-reconciliation tbody button");
        await detail.click();
        const dialog = page.getByRole("dialog");
        await dialog.waitFor();
        const text = await dialog.innerText();
        assert.ok(
          text.includes("$1.000000001") && text.includes("$0.000000001"),
        );
        assert.ok(!text.includes("balance_vs_ledger"));
        await page.screenshot({
          path: path.join(out, `details-${lang}-${theme}-${width}.png`),
        });
        await page.keyboard.press("Escape");
        await dialog.waitFor({ state: "hidden" });
        assert.equal(
          await detail.evaluate((e) => document.activeElement === e),
          true,
        );
        await page.locator(".reconciliation-actions button").first().click();
        await dialog.waitFor();
        await dialog.locator("input[pattern]").fill("bank-a");
        await dialog.locator('input[type="date"]').fill("2026-11-04");
        await dialog
          .locator('input[type="file"]')
          .setInputFiles({
            name: "statement.csv",
            mimeType: "text/csv",
            buffer: Buffer.from(
              "provider_order_id,platform_order_id,user_id,amount_cents,currency,paid_at\npay-1,ord-1,12,1000,USD,2026-11-04T12:00:00+04:00\n",
            ),
          });
        await page.screenshot({
          path: path.join(out, `import-${lang}-${theme}-${width}.png`),
        });
        await dialog.locator('button[type="submit"]').click();
        await dialog.waitFor({ state: "hidden" });
        assert.equal(posted, true);
        phase = "empty";
        await page.reload();
        await page.locator(".adm-reconciliation").waitFor();
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-reconciliation tbody tr").length ===
            0,
        );
        phase = "error";
        await page.reload();
        await page
          .getByText("Synthetic report failure", { exact: false })
          .waitFor();
        results.push({ lang, theme, width, passed: true });
        await context.close();
      }
  assert.deepEqual(errors, []);
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify({ results, errors }, null, 2),
  );
  console.log(JSON.stringify({ passed: results.length, errors }));
} finally {
  await browser?.close();
  await vite.close();
}
