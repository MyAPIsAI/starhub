import assert from "node:assert/strict";
import { mkdir, writeFile, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

// Isolated HTTP fixtures never write to the live accounting database.
const web = fileURLToPath(new URL("../", import.meta.url));
const out = path.resolve(web, "../output/reconciliation-redesign/browser");
await mkdir(out, { recursive: true });
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
const report = { results: [], errors: [] };
let browser;
try {
  await vite.listen();
  browser = await chromium.launch({ channel: "chrome", headless: true });
  for (const lang of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const labels = JSON.parse(
          await readFile(
            path.join(
              web,
              `src/i18n/messages.${lang === "ar" ? "en" : lang}.json`,
            ),
            "utf8",
          ),
        );
        const context = await browser.newContext({
          viewport: { width, height: 1050 },
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
          posted = false,
          runs = 0,
          summaryReads = 0,
          lastAccount = "";
        const difference = {
          kind: "balance_vs_ledger",
          funding_source: "api_quota",
          expected_nano: "1000000000",
          actual_nano: "1000000001",
          delta_nano: "1",
        };
        const rows = Array.from({ length: 14 }, (_, i) => ({
          id: i + 1,
          date: new Date(Date.UTC(2026, 10, 7 - i)).toISOString().slice(0, 10),
          user_id: 12,
          username: "noor_labs",
          timezone: "Asia/Dubai",
          usage_nano: "1245678000",
          api_ledger_nano: "1245678000",
          order_nano: i === 2 ? "100000000000" : "0",
          recharge_ledger_nano: i === 2 ? "100000000000" : "0",
          balance_expected_nano: "1000000000",
          balance_actual_nano: i === 0 ? "1000000001" : "1000000000",
          reserved_nano: "0",
          differences: i === 0 ? [difference] : [],
          provisional: i === 0,
          created_at: 1794016800,
          external_status: "not_imported",
          imported_sources: [],
          checks_version: i === 3 ? 0 : 2,
          admission_time_verified: true,
        }));
        rows.push({
          ...rows[0],
          id: 99,
          user_id: 13,
          username: "atlas_studio",
        });
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const request = route.request(),
              url = new URL(request.url());
            let data = {};
            if (
              ["/api/admin/auth/self", "/api/user/self"].includes(url.pathname)
            )
              data = {
                id: 1,
                username: "fixture",
                role: width === 390 ? 5 : 100,
                account_kind: "staff",
                platform_admin: true,
                status: 1,
                group: "default",
                quota: 0,
                used_quota: 0,
              };
            else if (url.pathname === "/api/finance/summary") {
              summaryReads++;
            } else if (url.pathname === "/api/admin/customers")
              data = {
                items: [
                  { id: 12, username: "noor_labs" },
                  { id: 13, username: "atlas_studio" },
                ],
                total: 2,
              };
            else if (url.pathname === "/api/admin/reconciliation/statements") {
              if (request.method() === "POST") {
                const body = request.postDataJSON();
                assert.equal(body.provider, "bank-a");
                assert.equal(body.expected_previous_id, 0);
                assert.ok(body.csv.includes("1000,USD"));
                posted = true;
                rows[0].external_status = "imported_scope";
                data = { id: 1 };
              } else data = { items: [] };
            } else if (url.pathname === "/api/admin/reconciliation") {
              if (request.method() === "POST") runs++;
              if (phase === "error") {
                await route.fulfill({
                  status: 500,
                  json: { success: false, message: "Synthetic report failure" },
                });
                return;
              }
              lastAccount = url.searchParams.get("user_id") || "";
              let items = rows.filter(
                (row) => !lastAccount || String(row.user_id) === lastAccount,
              );
              if (phase === "empty") items = [];
              if (phase === "paged")
                items = [
                  ...rows,
                  ...rows.map((row) => ({
                    ...row,
                    id: row.id + 100,
                    user_id: row.user_id + 100,
                  })),
                ];
              if (phase === "matched")
                items = rows.map((row) => ({
                  ...row,
                  differences: [],
                  checks_version: 2,
                  provisional: false,
                }));
              if (phase === "missing-end")
                items = rows.filter((row) => row.date !== "2026-11-07");
              data = {
                items,
                timezone: "Asia/Dubai",
                end: url.searchParams.get("end") || "2026-11-07",
                alert_webhook_configured: phase === "matched",
                pending_alerts: phase === "matched" ? 0 : 1,
              };
            }
            await route.fulfill({ json: { success: true, data } });
          },
        );
        const page = await context.newPage();
        page.setDefaultTimeout(15000);
        page.on("pageerror", (error) =>
          report.errors.push(`${lang}-${theme}-${width}: ${error.message}`),
        );
        await page.goto(vite.resolvedUrls.local[0] + "admin?tab=finance");
        const root = page.locator(".adm-reconciliation"),
          dialog = page.getByRole("dialog");
        const button = (key, parent = page) =>
          parent.getByRole("button", { name: labels[key], exact: true });
        await root.locator("tbody tr").first().waitFor();
        assert.equal(await page.locator(".finance-overview").count(), 0);
        assert.equal(summaryReads, 0);
        assert.equal(
          await page.getByTestId("difference-days").innerText(),
          "1",
        );
        assert.ok(
          (await page.getByTestId("end-result").innerText()).includes(
            labels.differences_found_384b9ede6f,
          ),
        );
        assert.equal(await page.locator("html").getAttribute("dir"), "ltr");
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
        );
        await page.screenshot({
          path: path.join(out, `${lang}-${theme}-${width}-reports.png`),
          fullPage: width === 1440,
        });
        const tones = await root
          .locator("tbody tr")
          .evaluateAll((rows) => [
            getComputedStyle(rows[0]).backgroundColor,
            getComputedStyle(rows[2]).backgroundColor,
          ]);
        assert.notEqual(tones[0], tones[1]);
        if (width === 390) {
          const table = root.locator(".adm-table-scroll");
          await table.focus();
          await table.press("ArrowRight");
          await page.waitForFunction(
            () =>
              document.querySelector(".adm-reconciliation .adm-table-scroll")
                .scrollLeft > 0,
          );
          await table.evaluate((element) =>
            element.scrollTo({ left: element.scrollWidth }),
          );
          await page.screenshot({
            path: path.join(out, `${lang}-${theme}-${width}-table.png`),
          });
        }
        const detail = root.locator("tbody tr").first().getByRole("button");
        await detail.click();
        await dialog.waitFor();
        const detailText = await dialog.innerText();
        assert.ok(
          detailText.includes("$1.000000001") &&
            detailText.includes("$0.000000001"),
        );
        assert.ok(!detailText.includes("balance_vs_ledger"));
        await page.screenshot({
          path: path.join(out, `${lang}-${theme}-${width}-detail.png`),
        });
        await page.keyboard.press("Escape");
        await dialog.waitFor({ state: "hidden" });
        await page.waitForFunction(
          (label) => document.activeElement?.textContent?.trim() === label,
          labels.details_a91e8f9bea,
        );
        await page
          .getByTestId("reconciliation-result-filter")
          .selectOption("difference");
        assert.equal(await root.locator("tbody tr").count(), 2);
        await page
          .getByTestId("reconciliation-result-filter")
          .selectOption("review");
        assert.equal(await root.locator("tbody tr").count(), 1);
        await page
          .getByTestId("reconciliation-result-filter")
          .selectOption("all");
        await page.getByTestId("reconciliation-account").selectOption("13");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-reconciliation tbody tr").length ===
            1,
        );
        assert.equal(lastAccount, "13");
        await page.getByTestId("reconciliation-account").selectOption("");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-reconciliation tbody tr").length ===
            15,
        );
        await page.getByTestId("run-reconciliation").click();
        await page.waitForFunction(
          (label) =>
            document
              .querySelector('[data-testid="run-reconciliation"]')
              ?.textContent?.includes(label),
          labels.reconciliation_run_again,
        );
        assert.equal(runs, 1);
        await page.getByTestId("import-statement").click();
        await dialog.waitFor();
        await dialog.locator("input[pattern]").fill("bank-a");
        await dialog.locator('input[type="date"]').fill("2026-11-04");
        await dialog.locator('input[type="file"]').setInputFiles({
          name: "statement.csv",
          mimeType: "text/csv",
          buffer: Buffer.from(
            "provider_order_id,platform_order_id,user_id,amount_cents,currency,paid_at\npay-1,ord-1,12,1000,USD,2026-11-04T12:00:00+04:00\n",
          ),
        });
        await page.screenshot({
          path: path.join(out, `${lang}-${theme}-${width}-import.png`),
        });
        await dialog.locator('button[type="submit"]').click();
        await dialog.waitFor({ state: "hidden" });
        assert.equal(posted, true);
        assert.ok(
          (await root.locator("tbody tr").first().innerText()).includes(
            labels.imported_statements_checked_1d6b628be5,
          ),
        );
        phase = "paged";
        await page.getByTestId("refresh-reconciliation").click();
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".adm-reconciliation tbody tr").length ===
            20,
        );
        await button("next_60869bad82", root).click();
        assert.equal(await root.locator("tbody tr").count(), 10);
        phase = "matched";
        await page.getByTestId("refresh-reconciliation").click();
        await page
          .getByTestId("end-result")
          .getByText(labels.internal_checks_match_5188b672dc, { exact: true })
          .waitFor();
        assert.equal(
          await page.getByTestId("difference-days").innerText(),
          "0",
        );
        assert.equal(await root.locator("tbody tr").count(), 15);
        phase = "missing-end";
        await page.getByTestId("refresh-reconciliation").click();
        await page
          .getByTestId("end-result")
          .getByText(labels.reconciliation_no_end_report, { exact: true })
          .waitFor();
        phase = "empty";
        await page.getByTestId("refresh-reconciliation").click();
        await root.locator(".reconciliation-empty").waitFor();
        await page.screenshot({
          path: path.join(out, `${lang}-${theme}-${width}-empty.png`),
        });
        phase = "error";
        await page.getByTestId("refresh-reconciliation").click();
        await page
          .getByText("Synthetic report failure", { exact: false })
          .waitFor();
        assert.equal(
          await page.getByTestId("reconciliation-summary").count(),
          0,
        );
        await page.screenshot({
          path: path.join(out, `${lang}-${theme}-${width}-error.png`),
        });
        assert.equal(summaryReads, 0);
        report.results.push({
          lang,
          theme,
          width,
          role: width === 390 ? "finance" : "root",
          passed: true,
        });
        await context.close();
      }
  assert.deepEqual(report.errors, []);
} catch (error) {
  report.errors.push(String(error.stack || error));
  process.exitCode = 1;
} finally {
  await browser?.close();
  await vite.close();
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify(report, null, 2),
  );
  console.log(JSON.stringify(report));
}
