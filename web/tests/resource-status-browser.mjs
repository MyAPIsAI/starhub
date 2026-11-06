import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

// Synthetic HTTP responses isolate presentation from the live database.
const web = fileURLToPath(new URL("../", import.meta.url));
const out = path.resolve(web, "../output/resource-monitor-cards-browser");
await mkdir(out, { recursive: true });
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
let browser;
const results = [],
  errors = [];
try {
  await vite.listen();
  const base = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ channel: "chrome", headless: true });
  for (const lang of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const context = await browser.newContext({
          viewport: { width, height: 1100 },
          reducedMotion: "reduce",
        });
        await context.addInitScript(
          ({ lang, theme }) => {
            localStorage.setItem("xy-language", lang);
            localStorage.setItem("xy-theme", theme);
          },
          { lang, theme },
        );
        let phase = "alert",
          posts = 0,
          reads = 0,
          mismatchRecovered = false;
        const now = Math.floor(Date.now() / 1000);
        const resources = Array.from({ length: 21 }, (_, i) => ({
          id: `resource-${String(i + 1).padStart(2, "0")}`,
          vendor: i === 20 ? "vendor-b" : "vendor-a",
          canonicalModel: "fixture-model",
          upstreamModel: "fixture-upstream",
          capabilities: ["text"],
          price_tier: 4,
          quality_level: "S0",
          state: "hot",
          status: "available",
          trend: "stable",
          score: 90,
          calls_5m: 200,
          success_rate: 0.975,
          tiers: [4, 6, 8, 10],
          gateways: [
            {
              gateway_id: "gateway-a",
              state: "open",
              last_seen: now,
              consecutive_failures: 5,
            },
            {
              gateway_id: "gateway-b",
              state: "normal",
              last_seen: now,
              consecutive_failures: 0,
            },
            {
              gateway_id: "gateway-old",
              state: "unknown",
              last_seen: now - 30,
              consecutive_failures: 0,
            },
          ],
        }));
        Object.assign(resources[19], {
          state: "cold",
          trend: "degraded",
          price_tier: undefined,
          quality_level: undefined,
          score: undefined,
          calls_5m: 0,
          success_rate: null,
        });
        resources[20].gateways.push({
          gateway_id: "gateway-extra-with-a-long-unique-identifier",
          state: "degraded",
          last_seen: now,
          consecutive_failures: 0,
        });
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const req = route.request(),
              url = new URL(req.url());
            let data = {};
            if (
              url.pathname === "/api/admin/auth/self" ||
              url.pathname === "/api/user/self"
            )
              data = {
                id: 1,
                username: "technical",
                role: 20,
                account_kind: "staff",
                platform_admin: true,
                status: 1,
                group: "default",
                quota: 0,
                used_quota: 0,
              };
            else if (url.pathname === "/api/admin/resource-sync") {
              reads++;
              if (phase === "error") {
                await route.fulfill({
                  status: 500,
                  json: { success: false, message: "Synthetic sync failure" },
                });
                return;
              }
              if (phase === "legacy") {
                await route.fulfill({
                  json: {
                    success: true,
                    data: {
                      configured: true,
                      revision: 1,
                      sequence: 1,
                      updated_at: now,
                    },
                  },
                });
                return;
              }
              if (req.method() === "POST") {
                assert.equal(req.postDataJSON().confirm, true);
                posts++;
                phase = "aligned";
              }
              data = {
                configured: true,
                status: phase === "alert" ? "alert" : "aligned",
                mode: "versioned_snapshot",
                applied_version: 12,
                remote_version: phase === "alert" ? 14 : 12,
                version_gap: phase === "alert" ? 2 : 0,
                checksum: "b".repeat(64),
                sequence: 105,
                snapshot: {
                  sequence: phase === "alert" ? 300 : 301,
                  published_at: now - (phase === "alert" ? 190 : 1),
                  age_seconds: phase === "alert" ? 190 : 1,
                  status: phase === "alert" ? "expired" : "fresh",
                },
                last_aligned_at: phase === "aligned" ? now + 1 : now - 600,
                last_attempt_at: now,
                observed_at: now,
                failed_cycles: phase === "alert" ? 2 : 0,
                failures:
                  phase === "empty"
                    ? []
                    : [
                        { id: 1, at: now - 300, code: "source_unreachable" },
                        { id: 2, at: now, code: "source_rejected" },
                      ],
              };
            } else if (url.pathname === "/api/admin/resource-monitoring") {
              if (phase === "error") {
                await route.fulfill({
                  status: 500,
                  json: {
                    success: false,
                    message: "Synthetic monitoring failure",
                  },
                });
                return;
              }
              data = {
                events_configured: phase !== "empty",
                snapshot_configured: phase !== "empty",
                reports_configured: phase !== "empty",
                feeds: [],
                mismatches:
                  phase === "empty"
                    ? []
                    : (phase === "paged"
                        ? Array.from(
                            {
                              length:
                                Number(
                                  url.searchParams.get("mismatch_page") || 1,
                                ) === 1
                                  ? 20
                                  : 5,
                            },
                            (_, i) =>
                              i +
                              1 +
                              (Number(
                                url.searchParams.get("mismatch_page") || 1,
                              ) -
                                1) *
                                20,
                          )
                        : [1, 2]
                      ).map((id) => ({
                        id: `mismatch-${id}`,
                        resource_id:
                          id === 2 ? "retired-resource" : "resource-21",
                        snapshot_sequence: 298 + id,
                        snapshot_price: 4,
                        event_price: id === 1 ? 6 : 8,
                        event_version: id + 10,
                        at: now - 100 * id,
                        resolved_at: mismatchRecovered ? now : 0,
                      })),
                mismatch_total:
                  phase === "empty" ? 0 : phase === "paged" ? 25 : 2,
                incidents:
                  phase === "empty"
                    ? []
                    : ["half_open_failed", "ewma_breach_twice"].map(
                        (type, id) => ({
                          event_id: `incident-${id}`,
                          type,
                          resource_id: "resource-21",
                          gateway_id: "gw-2",
                          occurred_at: now - 60,
                          delivered_at: id === 0 ? now : 0,
                          attempts: id === 0 ? 1 : 2,
                          delivery_status: id === 0 ? "delivered" : "retrying",
                        }),
                      ),
                incident_total: phase === "empty" ? 0 : 2,
              };
            } else if (url.pathname === "/api/admin/resource-instances") {
              if (phase === "error") {
                await route.fulfill({
                  status: 500,
                  json: { success: false, message: "Synthetic failure" },
                });
                return;
              }
              const matching =
                phase === "empty"
                  ? []
                  : resources.filter(
                      (r) =>
                        (!url.searchParams.get("q") ||
                          `${r.id} ${r.canonicalModel}`
                            .toLowerCase()
                            .includes(
                              url.searchParams.get("q").toLowerCase(),
                            )) &&
                        (!url.searchParams.get("resource_id") ||
                          r.id === url.searchParams.get("resource_id")) &&
                        (!url.searchParams.get("vendor") ||
                          r.vendor === url.searchParams.get("vendor")) &&
                        (!url.searchParams.get("status") ||
                          r.state === url.searchParams.get("status")),
                    );
              const page = Number(url.searchParams.get("page") || 1);
              data = {
                items: matching.slice((page - 1) * 20, page * 20),
                total: matching.length,
                vendors: ["vendor-a", "vendor-b"],
                models: ["fixture-model"],
                observed_at: now,
              };
            } else if (url.pathname === "/api/log") {
              assert.ok(
                ["resource-21", "retired-resource"].includes(
                  url.searchParams.get("resource_id"),
                ),
              );
              data = {
                items: [
                  {
                    id: 1,
                    user_id: 2,
                    model: "fixture-model",
                    trace_id: "related-resource-trace",
                    status: 1,
                  },
                ],
                total: 1,
              };
            }
            await route.fulfill({ json: { success: true, data } });
          },
        );
        const page = await context.newPage();
        page.setDefaultTimeout(12000);
        page.on("pageerror", (e) => errors.push(e.message));
        await page.goto(base + "admin?tab=sync");
        await page.locator(".resource-status-table tbody tr").first().waitFor();
        assert.equal(
          await page.locator(".resource-status-table tbody tr").count(),
          20,
        );
        assert.equal(
          await page
            .locator('.resource-status-table [data-state="open"]')
            .count(),
          20,
        );
        assert.equal(
          await page
            .locator('.resource-status-table [data-state="normal"]')
            .count(),
          20,
        );
        assert.equal(
          await page
            .locator('.resource-status-table [data-state="unknown"]')
            .count(),
          20,
        );
        await page.locator('[data-snapshot-state="expired"]').waitFor();
        await page
          .locator('.resource-monitor-list:not([data-history="sync"]) > li')
          .nth(3)
          .waitFor();
        assert.equal(
          await page
            .locator('.resource-monitor-list:not([data-history="sync"]) > li')
            .count(),
          4,
        );
        await page
          .locator(".resource-snapshot-panel")
          .evaluate((e) =>
            window.scrollTo(0, scrollY + e.getBoundingClientRect().top - 60),
          );
        await page.screenshot({
          path: path.join(
            out,
            `snapshot-history-${lang}-${theme}-${width}.png`,
          ),
        });
        assert.equal(await page.locator("html").getAttribute("dir"), "ltr");
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
        );
        await page.screenshot({
          path: path.join(out, `status-${lang}-${theme}-${width}.png`),
        });
        await page
          .locator(".resource-monitor-history")
          .first()
          .evaluate((e) =>
            window.scrollTo(0, scrollY + e.getBoundingClientRect().top - 70),
          );
        await page.screenshot({
          path: path.join(out, `histories-${lang}-${theme}-${width}.png`),
        });
        const cards = page.locator(".resource-monitor-grid > section");
        assert.equal(await cards.count(), 3);
        const boxes = await cards.evaluateAll((nodes) =>
          nodes.map((node) => ({
            x: node.getBoundingClientRect().x,
            y: node.getBoundingClientRect().y,
            width: node.clientWidth,
            scrollWidth: node.scrollWidth,
          })),
        );
        assert.ok(boxes.every((box) => box.scrollWidth <= box.width + 1));
        if (width === 1440)
          assert.ok(boxes.every((box) => Math.abs(box.y - boxes[0].y) < 2));
        else assert.ok(boxes[0].y < boxes[1].y && boxes[1].y < boxes[2].y);
        assert.equal(
          await page.locator("html").getAttribute("lang"),
          lang === "ar" ? "en" : lang,
        );
        const mismatchRows = page.locator('[data-history="mismatches"] > li');
        const recordDetails = mismatchRows.first().locator("summary");
        await recordDetails.focus();
        await page.keyboard.press("Enter");
        await mismatchRows.first().locator("details[open]").waitFor();
        assert.ok(
          (await mismatchRows.first().locator("dl").textContent()).includes(
            "299",
          ),
        );
        await page.keyboard.press("Enter");
        const missingTrigger = mismatchRows
          .nth(1)
          .locator('[data-action="history-resource-detail"]');
        await missingTrigger.click();
        await page.locator('.resource-status-detail [role="status"]').waitFor();
        await page.keyboard.press("Escape");
        await page.getByRole("dialog").waitFor({ state: "hidden" });
        await mismatchRows
          .nth(1)
          .locator(".resource-monitor-missing")
          .waitFor();
        await page.waitForFunction(
          () =>
            document.activeElement?.getAttribute("data-action") ===
            "history-resource-detail",
        );
        await mismatchRows.nth(1).locator("a").click();
        await page.getByTestId("calls-resource-filter").waitFor();
        assert.equal(
          new URL(page.url()).searchParams.get("resource_id"),
          "retired-resource",
        );
        await page.goBack();
        await page.locator(".resource-status-table tbody tr").first().waitFor();

        await page
          .locator(".resource-status-inventory")
          .evaluate((e) =>
            window.scrollTo(0, scrollY + e.getBoundingClientRect().top - 90),
          );
        assert.ok(
          await page
            .locator(".resource-status-table tbody tr")
            .first()
            .evaluate((e) => e.getBoundingClientRect().height < 75),
        );
        await page.screenshot({
          path: path.join(out, `inventory-${lang}-${theme}-${width}.png`),
        });
        await page.locator(".resource-status-table").evaluate((e, rtl) => {
          e.scrollLeft = rtl ? -e.scrollWidth : e.scrollWidth;
        }, false);
        await page.screenshot({
          path: path.join(out, `resources-${lang}-${theme}-${width}.png`),
        });

        const search = page.locator(".resource-status-search input");
        await search.fill("RESOURCE-21");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".resource-status-table tbody tr")
              .length === 1,
        );
        assert.ok(
          (
            await page.locator(".resource-status-table tbody").textContent()
          ).includes("resource-21"),
        );
        assert.equal(
          await page
            .locator(".resource-status-gateways")
            .textContent()
            .then((v) => v.includes("+1")),
          true,
        );
        const detailTrigger = page
          .locator('[data-action="resource-detail"]')
          .first();
        await detailTrigger.click();
        await page
          .locator(".resource-status-gateway-details li")
          .nth(3)
          .waitFor();
        assert.ok(
          (
            await page.locator(".resource-status-detail").textContent()
          ).includes("gateway-extra-with-a-long-unique-identifier"),
        );
        assert.ok(
          await page
            .getByRole("dialog")
            .evaluate((e) => e.scrollWidth <= e.clientWidth + 1),
        );
        await page.screenshot({
          path: path.join(out, `detail-${lang}-${theme}-${width}.png`),
        });
        await page.keyboard.press("Escape");
        await page.getByRole("dialog").waitFor({ state: "hidden" });
        await page.waitForFunction(
          () =>
            document.activeElement ===
            document.querySelector('[data-action="resource-detail"]'),
        );
        await page.locator(".resource-status-row-actions a").first().click();
        await page.getByTestId("calls-resource-filter").waitFor();
        await page
          .getByText("related-resource-trace", { exact: true })
          .waitFor();
        assert.equal(
          new URL(page.url()).searchParams.get("resource_id"),
          "resource-21",
        );
        await page.goBack();
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".resource-status-table tbody tr")
              .length === 20,
        );
        await page
          .locator(".resource-status-search input")
          .fill("no-matching-resource");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".resource-status-table tbody tr")
              .length === 0,
        );
        await page.locator(".resource-status-search input").fill("");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".resource-status-table tbody tr")
              .length === 20,
        );

        await page.locator('[data-filter="vendor"]').selectOption("vendor-b");
        await page.waitForFunction(() =>
          document
            .querySelector(".resource-status-table tbody")
            ?.textContent.includes("resource-21"),
        );
        assert.equal(
          await page.locator(".resource-status-table tbody tr").count(),
          1,
        );
        await page.locator('[data-filter="vendor"]').selectOption("");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".resource-status-table tbody tr")
              .length === 20,
        );
        await page
          .locator(
            ".resource-status-inventory .resource-status-pagination button",
          )
          .last()
          .click();
        await page.waitForFunction(() =>
          document
            .querySelector(".resource-status-table tbody")
            ?.textContent.includes("resource-21"),
        );
        const trigger = page.getByTestId("history-force-resource-sync");
        await trigger.click();
        const dialog = page.getByRole("dialog");
        await dialog.waitFor();
        assert.equal(posts, 0);
        await page.screenshot({
          path: path.join(out, `confirm-${lang}-${theme}-${width}.png`),
        });
        await page.keyboard.press("Escape");
        await dialog.waitFor({ state: "hidden" });
        assert.equal(posts, 0);
        await page.waitForFunction(
          () =>
            document.activeElement ===
            document.querySelector(
              '[data-testid="history-force-resource-sync"]',
            ),
        );
        await trigger.click();
        await dialog.waitFor();
        await dialog.locator(".resource-status-actions button").last().click();
        await dialog.waitFor({ state: "hidden" });
        assert.equal(posts, 1);
        await page.locator('[data-state="aligned"]').waitFor();
        await page.locator('[data-snapshot-state="fresh"]').waitFor();
        assert.equal(
          await page
            .locator('.resource-monitor-list:not([data-history="sync"]) > li')
            .count(),
          4,
        );
        assert.equal(
          await page
            .locator('[data-history="mismatches"] [data-state="alert"]')
            .count(),
          2,
        );
        await page
          .locator('[data-history="sync"] [data-state="normal"]')
          .first()
          .waitFor();
        mismatchRecovered = true;
        await page.locator(".resource-monitor-toolbar button").last().click();
        await page
          .locator('[data-history="mismatches"] [data-state="normal"]')
          .nth(1)
          .waitFor();
        assert.equal(
          await page.locator('[data-history="sync"] > li').count(),
          2,
        );
        await page.locator(".resource-monitor-grid").scrollIntoViewIfNeeded();
        await page.screenshot({
          path: path.join(out, `recovered-${lang}-${theme}-${width}.png`),
        });
        if (lang === "zh" && theme === "light" && width === 1440) {
          const prior = reads;
          await page.waitForTimeout(5500);
          assert.ok(reads > prior);
        }
        if (lang === "zh" && theme === "light" && width === 1440) {
          phase = "paged";
          await page.reload();
          await page
            .locator('[data-history="mismatches"] > li')
            .nth(19)
            .waitFor();
          const list = page.locator('[data-history="mismatches"]');
          assert.ok(
            await list.evaluate(
              (node) => node.scrollHeight > node.clientHeight,
            ),
          );
          await list.focus();
          await page.keyboard.press("End");
          await page.waitForFunction(
            () =>
              document.querySelector('[data-history="mismatches"]').scrollTop >
              0,
          );
          await page
            .locator(
              '[aria-labelledby="resource-mismatch-title"] .resource-status-pagination button',
            )
            .last()
            .click();
          await page.waitForFunction(
            () =>
              document.querySelectorAll('[data-history="mismatches"] > li')
                .length === 5,
          );
          await page
            .locator(
              '[aria-labelledby="resource-mismatch-title"] .resource-status-pagination button',
            )
            .first()
            .click();
          await page
            .locator('[data-history="mismatches"] > li')
            .nth(19)
            .waitFor();
        }
        phase = "empty";
        await page.reload();
        await page
          .locator(".resource-status-inventory .resource-status-pagination")
          .waitFor();
        assert.equal(
          await page.locator(".resource-status-table tbody tr").count(),
          0,
        );
        await page.locator(".resource-monitor-empty").nth(2).waitFor();
        await page.locator(".resource-monitor-grid").scrollIntoViewIfNeeded();
        await page.screenshot({
          path: path.join(out, `empty-${lang}-${theme}-${width}.png`),
        });
        phase = "error";
        await page.reload();
        await page
          .locator(".resource-status-panel")
          .last()
          .getByRole("alert")
          .waitFor();
        assert.equal(
          await page.locator(".resource-status-table tbody tr").count(),
          0,
        );
        assert.equal(await page.locator(".resource-monitor-empty").count(), 0);
        assert.equal(
          await page.locator('.resource-monitor-grid [role="alert"]').count(),
          3,
        );
        await page.locator(".resource-monitor-grid").scrollIntoViewIfNeeded();
        await page.screenshot({
          path: path.join(out, `error-${lang}-${theme}-${width}.png`),
        });
        phase = "legacy";
        await page.reload();
        await page
          .locator(".resource-status-panel")
          .first()
          .getByRole("alert")
          .waitFor();
        assert.equal(
          await page.getByTestId("force-resource-sync").isDisabled(),
          true,
        );
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
