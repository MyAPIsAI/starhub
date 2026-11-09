import assert from "node:assert/strict";
import { mkdir, writeFile, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url));
const out = path.resolve(web, "../output/nfr6/browser");
await mkdir(out, { recursive: true });
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
const report = { cases: [], errors: [] };
let browser;
try {
  await vite.listen();
  browser = await chromium.launch({ channel: "chrome", headless: true });
  for (const lang of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const name = `${lang}-${theme}-${width}`;
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
        const now = Math.floor(Date.now() / 1000),
          traceID = "a".repeat(64);
        const metric = {
          protocol: "OpenAI",
          model: "fixture-model",
          vendor: "Example vendor",
          service_group: "default",
          requests: 100,
          eligible: 100,
          failures: 6,
          success_rate: 0.94,
          ttft_ms: 120,
          duration_ms: 350,
          gateway_ms: 12,
        };
        const summary = {
          at: now,
          all: [metric],
          dimensions: [metric],
          active: { critical: 1, serious: 2, general: 1 },
          pending_deliveries: 4,
          notifications: {
            phone_configured: false,
            im_configured: true,
            general_configured: true,
            observer_configured: false,
            timezone: "UTC",
            work_start: 9,
            work_end: 18,
          },
          observations: {
            precharge: { total: 100, failed: 1 },
            settlement: { total: 94, failed: 0 },
            ledger: { total: 94, failed: 0 },
            payment_callback: { total: 0, failed: 0 },
          },
          instances: [
            {
              gateway_id: "gateway-fixture",
              at: now,
              cpu_percent: 12,
              memory_percent: 81,
              disk_percent: 34,
            },
          ],
          routing_states: [
            { gateway_id: "gateway-fixture", at: now, open: 1, degraded: 2 },
          ],
          sync: { version_gap: 2, snapshot: { age_seconds: 181 } },
        };
        const alerts = Array.from({ length: 23 }, (_, i) => ({
          id: `alert-${i}`,
          rule:
            i === 0
              ? "platform_success"
              : i === 1
                ? "snapshot_age"
                : "reconciliation_difference",
          scope: i === 0 ? "platform" : `account-${i}`,
          severity: i === 0 ? "critical" : "serious",
          active: true,
          value: 0.94,
          threshold: 0.95,
          first_seen: now - 120,
          last_seen: now,
          resolved_at: 0,
        }));
        let failed = false,
          empty = false;
        const requests = [];
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const u = new URL(route.request().url()),
              p = u.pathname;
            requests.push(p + u.search);
            let data = {};
            if (["/api/admin/auth/self", "/api/user/self"].includes(p))
              data = {
                id: 1,
                username: "monitoring-fixture",
                role: width === 390 ? 20 : 100,
                account_kind: "staff",
                platform_admin: true,
                status: 1,
                group: "default",
                quota: 0,
                used_quota: 0,
              };
            else if (p === "/api/status")
              data = { title: "Fixture", setup: true };
            else if (p === "/api/option") data = [];
            else if (p === "/api/admin/observability") {
              if (failed)
                return route.fulfill({
                  status: 503,
                  json: { success: false, message: "Fixture unavailable" },
                });
              data = empty
                ? {
                    ...summary,
                    all: [],
                    dimensions: [],
                    active: { critical: 0, serious: 0, general: 0 },
                    instances: [],
                    pending_deliveries: 0,
                  }
                : summary;
            } else if (p === "/api/admin/alerts") {
              if (failed)
                return route.fulfill({
                  status: 503,
                  json: { success: false, message: "Fixture unavailable" },
                });
              const severity = u.searchParams.get("severity"),
                state = u.searchParams.get("state");
              const rows = empty
                ? []
                : alerts.filter(
                    (row) =>
                      (!severity || severity === row.severity) &&
                      state !== "resolved",
                  );
              const page = Math.min(
                Number(u.searchParams.get("page") || 1),
                Math.max(1, Math.ceil(rows.length / 20)),
              );
              data = {
                items: rows.slice((page - 1) * 20, page * 20),
                page,
                page_size: 20,
                total: rows.length,
                deliveries: [
                  {
                    id: "delivery-1",
                    alert_id: "alert-0",
                    channel: "phone",
                    severity: "critical",
                    transition: "firing",
                    created_at: now - 120,
                    delivered_at: 0,
                    attempts: 2,
                    last_error: "notification_unreachable",
                  },
                ],
              };
            } else if (p.startsWith("/api/admin/traces/")) {
              if (!p.endsWith(traceID))
                return route.fulfill({
                  status: 404,
                  json: { success: false, message: "Trace not found" },
                });
              data = {
                request: {
                  trace_id: traceID,
                  model: metric.model,
                  protocol: "OpenAI",
                  status: 200,
                },
                accounting_status: "posted",
                charged_nano: "123456",
                log_id: 12,
                ledger_id: 9,
                complete: true,
                steps: [
                  "gateway",
                  "authentication",
                  "precharge",
                  "routing",
                  "upstream",
                  "metering",
                  "settlement",
                  "ledger",
                ].map((stage, i) => ({
                  id: `step-${i}`,
                  stage,
                  status: "success",
                  started_at: (now - 1) * 1000 + i,
                  ended_at: now * 1000,
                  channel_id: stage === "upstream" ? 2 : 0,
                  upstream_id: stage === "upstream" ? "provider-fixture" : "",
                })),
              };
            }
            await route.fulfill({ json: { success: true, data } });
          },
        );
        const page = await context.newPage();
        page.on("pageerror", (e) =>
          report.errors.push(`${name}: ${e.message}`),
        );
        await page.goto(
          new URL("/admin?tab=observability", vite.resolvedUrls.local[0]).href,
        );
        const root = page.locator("section.adm-page.nfr-page"),
          dialog = page.getByRole("dialog");
        const button = (key, within = page) =>
          within.getByRole("button", { name: labels[key], exact: true });
        await root.getByText("94.00%", { exact: true }).first().waitFor();
        assert.equal(await page.locator("html").getAttribute("dir"), "ltr");
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          `${name} overflow`,
        );
        await page.screenshot({
          path: path.join(out, `${name}-page.png`),
          fullPage: true,
        });
        const table = root.getByRole("region", {
          name: labels.nfr_alert_records,
        });
        if (width === 390) {
          await table.focus();
          await page.keyboard.press("ArrowRight");
          await page.waitForFunction(
            () =>
              document.querySelector(".nfr-alert-list .adm-table-scroll")
                .scrollLeft > 0,
          );
        }
        await button("nfr_view_delivery", root).first().click();
        await dialog
          .getByText(labels.nfr_retry_pending, { exact: true })
          .waitFor();
        await dialog.screenshot({
          path: path.join(out, `${name}-delivery.png`),
        });
        await page.keyboard.press("Escape");
        await dialog.waitFor({ state: "hidden" });
        assert.equal(
          await page.evaluate(() =>
            document.activeElement?.textContent?.trim(),
          ),
          labels.nfr_view_delivery,
        );
        await button("next_60869bad82", root).click();
        await root.getByText("account-22", { exact: true }).waitFor();
        await root
          .getByLabel(labels.nfr_level, { exact: true })
          .selectOption("critical");
        await root.getByText("platform", { exact: true }).waitFor();
        assert.equal(await root.locator(".nfr-alert-list tbody tr").count(), 1);
        assert.ok(
          requests.some(
            (p) => p.includes("severity=critical") && p.includes("page=1"),
          ),
        );
        await root.locator("#nfr-trace").fill(traceID);
        await button("nfr_search", root).click();
        await dialog.getByText(labels.nfr_posted, { exact: true }).waitFor();
        assert.equal(await dialog.locator(".nfr-timeline li").count(), 8);
        await dialog.getByText("provider-fixture", { exact: true }).waitFor();
        assert.ok(
          await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth + 1),
          `${name} trace drawer overflow`,
        );
        await dialog.screenshot({ path: path.join(out, `${name}-trace.png`) });
        await page.keyboard.press("Escape");
        await dialog.waitFor({ state: "hidden" });
        await root.locator("#nfr-trace").fill("missing-trace-0001");
        await button("nfr_search", root).click();
        try {
          await dialog.getByRole("alert").waitFor();
        } catch (error) {
          await page.screenshot({
            path: path.join(out, `${name}-failure.png`),
            fullPage: true,
          });
          await writeFile(
            path.join(out, `${name}-failure.json`),
            JSON.stringify(
              {
                requests,
                errors: report.errors,
                input: await root.locator("#nfr-trace").inputValue(),
                html: await page.locator("body").innerText(),
              },
              null,
              2,
            ),
          );
          throw error;
        }
        assert.equal(await dialog.locator(".nfr-timeline").count(), 0);
        await page.keyboard.press("Escape");
        await root.locator("details > summary").click();
        await root.getByText("gateway-fixture", { exact: true }).waitFor();
        await root
          .locator("details")
          .screenshot({ path: path.join(out, `${name}-metrics.png`) });
        empty = true;
        await button("refresh_b1cae2bbd8", root).click();
        await root.getByText(labels.nfr_no_alerts, { exact: true }).waitFor();
        assert.equal(await root.locator(".nfr-alert-list tbody tr").count(), 0);
        failed = true;
        await button("refresh_b1cae2bbd8", root).click();
        await root.getByRole("alert").first().waitFor();
        assert.equal(await root.locator(".nfr-summary").count(), 0);
        await page.screenshot({
          path: path.join(out, `${name}-error.png`),
          fullPage: true,
        });
        report.cases.push({ name, passed: true });
        await context.close();
      }
  assert.deepEqual(report.errors, []);
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify(report, null, 2),
  );
  await browser?.close();
  await vite.close();
}
console.log(
  `Passed ${report.cases.length} browser combinations with ${report.errors.length} page errors.`,
);
