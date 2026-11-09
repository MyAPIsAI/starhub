import assert from "node:assert/strict";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

const out = fileURLToPath(
  new URL("../../output/status-demo/browser/", import.meta.url),
);
await mkdir(out, { recursive: true });
const vite = await createServer({
  root: fileURLToPath(new URL("../", import.meta.url)),
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
const report = { fixtureData: true, cases: [], errors: [] };
const at = Date.parse("2026-11-09T08:00:00Z") / 1000;
const models = Array.from({ length: 23 }, (_, i) => ({
  id: `model-${String(i).padStart(2, "0")}`,
  name: `Published model ${i}`,
  vendor: i < 20 ? "Vendor A" : "Vendor B",
  tags: i === 4 ? ["audio"] : ["text"],
  protocols: ["OpenAI"],
  current: {
    state: ["outage", "insufficient_data", "degraded", "normal"][i % 4],
    samples: i === 1 ? 40 : 50,
    window_minutes: 15,
    at,
  },
  last_24_hours: {
    samples: 50,
    success_rate: i === 0 ? 0 : 0.999,
    ttft_samples: 50,
    ttft_p50_ms: i === 4 ? null : 250,
    ttft_p95_ms: i === 4 ? null : 700,
  },
  daily: Array.from({ length: 90 }, (_, d) => ({
    date: new Date((at - (89 - d) * 86400) * 1000).toISOString().slice(0, 10),
    state: ["normal", "degraded", "outage", "insufficient_data"][d % 4],
    samples: d % 4 === 3 ? 40 : 1000,
    successful: d % 4 === 3 ? 40 : [1000, 995, 900][d % 4],
    success_rate: d % 4 === 3 ? null : [1, 0.995, 0.9][d % 4],
  })),
}));
let browser, page;
try {
  await vite.listen();
  const base = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ channel: "chrome", headless: true });
  for (const language of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const name = `${language}-${theme}-${width}`;
        const translations = JSON.parse(
          await readFile(
            new URL(`../src/i18n/${language}.json`, import.meta.url),
            "utf8",
          ),
        );
        const context = await browser.newContext({
          viewport: { width, height: 1000 },
          reducedMotion: "reduce",
          permissions: ["clipboard-read", "clipboard-write"],
        });
        await context.addInitScript(
          ({ language, theme }) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          { language, theme },
        );
        let mode = "normal",
          statusRequests = 0;
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const url = new URL(route.request().url()),
              p = url.pathname;
            if (["/api/user/self", "/api/admin/auth/state"].includes(p))
              return route.fulfill({
                status: 401,
                json: { success: false, message: "Anonymous fixture" },
              });
            let data = {};
            if (p === "/api/localization")
              data = { default_language: "en", languages: ["en", "zh", "ar"] };
            if (p === "/api/legal-policy")
              data = { timezone: "Asia/Dubai", cookie_consent: false };
            if (p === "/api/status/announcements")
              data = {
                total: 1,
                items: [
                  {
                    id: 1,
                    version: 1,
                    title: {
                      zh: "Synthetic incident",
                      en: "Synthetic incident",
                      ar: "Synthetic incident",
                    },
                    content: {
                      zh: "Fixture update",
                      en: "Fixture update",
                      ar: "Fixture update",
                    },
                    vendors: ["Vendor A"],
                    models: ["model-00"],
                    status: "monitoring",
                    created_at: at,
                    updated_at: at,
                    resolved_at: 0,
                  },
                ],
              };
            if (p === "/api/public/home") {
              if (mode === "error")
                return route.fulfill({
                  status: 503,
                  json: { success: false, message: "Synthetic outage" },
                });
              data = {
                models: mode === "empty" ? [] : models,
                groups:
                  mode === "empty"
                    ? []
                    : Array.from({ length: 7 }, (_, i) => ({
                        id: `group-${i}`,
                        name: `Published group ${i}`,
                        description: `Configured group description ${i}`,
                      })),
              };
            }
            if (p === "/api/status/models") {
              statusRequests++;
              if (mode === "error")
                return route.fulfill({
                  status: 503,
                  json: { success: false, message: "Synthetic outage" },
                });
              const q = url.searchParams.get("q"),
                vendor = url.searchParams.get("vendor"),
                type = url.searchParams.get("type"),
                state = url.searchParams.get("state"),
                number = Number(url.searchParams.get("page") || 1);
              const rows =
                mode === "empty"
                  ? []
                  : models.filter(
                      (m) =>
                        (!q || (m.id + m.name).includes(q)) &&
                        (!vendor || m.vendor === vendor) &&
                        (!type || m.tags.includes(type)) &&
                        (!state ||
                          (state === "unknown"
                            ? m.current.state === "insufficient_data"
                            : ["outage", "degraded"].includes(
                                m.current.state,
                              ))),
                    );
              data = {
                items: rows.slice((number - 1) * 20, number * 20),
                total: rows.length,
                page: number,
                page_size: 20,
                vendors: ["Vendor A", "Vendor B"],
                types: [
                  { id: "text", count: 22 },
                  { id: "audio", count: 1 },
                ],
                catalog_total: 23,
                at,
                timezone: "Asia/Dubai",
                refresh_seconds: 300,
                selected_group_count: 2,
                latency_target_ms: 1000,
              };
            }
            if (p.endsWith("/history") && p.startsWith("/api/status/models/")) {
              if (mode === "hourError")
                return route.fulfill({
                  status: 503,
                  json: { success: false, message: "Synthetic hourly error" },
                });
              const hours = Array.from({ length: 24 }, (_, i) => ({
                start: at - (24 - i) * 3600,
                end: at - (23 - i) * 3600,
                samples: i === 2 ? 0 : 100,
                successful: i === 0 ? 0 : i === 2 ? 0 : 99,
                success_rate: i === 2 ? null : i === 0 ? 0 : 0.99,
              }));
              data = {
                model: p.split("/")[4],
                timezone: "Asia/Dubai",
                at,
                history: {
                  start: at - 86400,
                  end: at,
                  samples: 2300,
                  success_rate: 2178 / 2300,
                  hours,
                },
              };
            }
            await route.fulfill({ json: { success: true, data } });
          },
        );
        page = await context.newPage();
        page.setDefaultTimeout(15000);
        await page.clock.install();
        page.on("pageerror", (e) =>
          report.errors.push({ name, error: e.message }),
        );
        const fit = async () =>
          assert.ok(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
            `${name} overflow`,
          );
        await page.goto(base);
        await page.locator("#home-models-title").waitFor();
        await page.getByText("Published model 0", { exact: true }).waitFor();
        assert.equal(
          await page.locator("html").getAttribute("dir"),
          language === "ar" ? "rtl" : "ltr",
        );
        assert.equal(
          await page.locator(".portal-actions a").first().getAttribute("href"),
          "/register",
        );
        assert.equal(
          await page.locator(".portal-demo").getAttribute("href"),
          "/docs",
        );
        assert.equal(
          await page
            .locator(".public-home-card")
            .filter({ hasText: "Published model" })
            .count(),
          6,
        );
        assert.match(await page.locator(".public-home").innerText(), /23/);
        assert.doesNotMatch(
          await page.locator(".public-home").innerText(),
          /38\.2045|24,892|99\.82%|528\.40/,
        );
        await page
          .locator(".public-home .list-pagination button")
          .last()
          .click();
        await page.getByText("Published group 6", { exact: true }).waitFor();
        await page.locator(".public-code button").click();
        const copied = (
          await page.evaluate(() => navigator.clipboard.readText())
        ).replaceAll("\r\n", "\n");
        assert.match(
          copied,
          /https:\/\/api\.myapis\.ai\/v1\/chat\/completions/,
        );
        assert.match(copied, /"model": "model-00"/);
        assert.ok(
          copied.includes(String.fromCharCode(92, 10)),
          JSON.stringify(copied),
        );
        await page.keyboard.press("Tab");
        await fit();
        await page.screenshot({
          path: out + name + "-home.png",
          fullPage: true,
        });
        await page.goto(base + "status");
        await page.locator('[data-status-model="model-00"]').waitFor();
        assert.equal(await page.locator("[data-status-model]").count(), 20);
        assert.equal(await page.locator(".public-announcements").count(), 1);
        for (const [id, state] of [
          ["00", "outage"],
          ["01", "insufficient_data"],
          ["02", "degraded"],
          ["03", "normal"],
        ])
          assert.equal(
            await page
              .locator(`[data-status-model="model-${id}"]`)
              .getAttribute("data-state"),
            state,
          );
        const first = page.locator('[data-status-model="model-00"]');
        assert.equal(
          await first.locator(".public-daily-bars button").count(),
          30,
        );
        assert.equal(await first.locator("dd").first().textContent(), "0.00%");
        assert.equal(
          await page.locator('[data-status-model="model-04"] dd').count(),
          1,
        );
        await page
          .getByRole("button", { name: translations.statusDays90, exact: true })
          .click();
        assert.equal(
          await first.locator(".public-daily-bars button").count(),
          90,
        );
        await first.locator(".public-daily-bars button").last().focus();
        await page.keyboard.press("Home");
        await page.keyboard.press("Enter");
        const dialog = page.getByRole("dialog");
        await dialog.locator(".status-hour-bars button").first().waitFor();
        assert.equal(
          await dialog.locator(".status-hour-bars button").count(),
          24,
        );
        assert.match(
          await dialog.locator(".status-hour-selected").innerText(),
          /0.00%/,
        );
        await dialog.locator(".status-hour-bars button").nth(2).click();
        await dialog
          .getByText(translations.statusHourNoData, { exact: true })
          .waitFor();
        await dialog.screenshot({ path: out + name + "-hours.png" });
        await page.keyboard.press("Escape");
        await dialog.waitFor({ state: "hidden" });
        assert.ok(
          await first
            .locator(".public-daily-bars button")
            .first()
            .evaluate((el) => el === document.activeElement),
        );
        mode = "hourError";
        await first
          .getByRole("button", { name: translations.statusLast24, exact: true })
          .click();
        await dialog.getByRole("alert").waitFor();
        mode = "normal";
        await dialog
          .getByRole("button", { name: translations.retry, exact: true })
          .click();
        await dialog.locator(".status-hour-bars button").first().waitFor();
        await page.keyboard.press("Escape");
        await page
          .getByRole("button", { name: translations.statusDays30, exact: true })
          .click();
        await fit();
        await page.locator(".list-pagination button").last().click();
        await page.locator('[data-status-model="model-20"]').waitFor();
        assert.equal(await page.locator("[data-status-model]").count(), 3);
        await page
          .locator(".public-status-filters select")
          .selectOption("Vendor A");
        await page.locator('[data-status-model="model-00"]').waitFor();
        await page
          .getByRole("button", {
            name: translations.statusAbnormal,
            exact: true,
          })
          .click();
        await page.waitForFunction(
          () => document.querySelectorAll("[data-status-model]").length === 10,
        );
        assert.ok(
          await page
            .locator("[data-status-model]")
            .evaluateAll((rows) =>
              rows.every((row) =>
                ["outage", "degraded"].includes(row.dataset.state),
              ),
            ),
        );
        await page
          .getByRole("button", {
            name: translations.statusPending,
            exact: true,
          })
          .click();
        await page.waitForFunction(
          () => document.querySelectorAll("[data-status-model]").length === 5,
        );
        await page
          .getByRole("button", {
            name: translations.statusAllModels,
            exact: true,
          })
          .click();
        await page.locator(".status-type-filters button").last().click();
        await page.waitForFunction(
          () => document.querySelectorAll("[data-status-model]").length === 1,
        );
        assert.equal(
          await page
            .locator("[data-status-model]")
            .getAttribute("data-status-model"),
          "model-04",
        );
        await page.locator(".status-type-filters button").first().click();
        await page.locator('[data-status-model="model-00"]').waitFor();
        await page.screenshot({
          path: out + name + "-list.png",
          fullPage: false,
        });
        await page.locator(".public-status-filters input").fill("model-00");
        await page.waitForFunction(
          () => document.querySelectorAll("[data-status-model]").length === 1,
        );
        await page.screenshot({
          path: out + name + "-status.png",
          fullPage: true,
        });
        const before = statusRequests;
        await Promise.all([
          page.waitForResponse(
            (r) => new URL(r.url()).pathname === "/api/status/models",
          ),
          page.clock.fastForward(300001),
        ]);
        await page.waitForFunction(
          () => !document.querySelector(".status-update button:disabled"),
        );
        assert.ok(statusRequests > before, "five-minute refresh did not run");
        mode = "error";
        await page
          .getByRole("button", {
            name: translations.publicStatusRefresh,
            exact: true,
          })
          .click();
        await page
          .getByRole("alert")
          .filter({ hasText: translations.publicStatusError })
          .waitFor();
        assert.equal(await page.locator("[data-status-model]").count(), 0);
        mode = "empty";
        await page
          .getByRole("button", { name: translations.retry, exact: true })
          .click();
        await page
          .getByText(translations.publicStatusEmpty, { exact: true })
          .waitFor();
        await fit();
        mode = "normal";
        await page
          .getByRole("button", {
            name: translations.publicStatusRefresh,
            exact: true,
          })
          .click();
        await first.waitFor();
        mode = "error";
        await page.goto(base);
        await page
          .getByText(translations.publicHomeLoadError, { exact: true })
          .waitFor();
        assert.equal(
          await page
            .locator(".public-home-card")
            .filter({ hasText: "Published model" })
            .count(),
          0,
        );
        mode = "empty";
        await page
          .getByRole("button", { name: translations.retry, exact: true })
          .click();
        await page
          .getByText(translations.publicHomeNoModels, { exact: true })
          .waitFor();
        await page
          .getByText(translations.publicHomeNoGroups, { exact: true })
          .waitFor();
        await fit();
        if (width === 390) {
          await page.setViewportSize({ width: 320, height: 900 });
          await fit();
        }
        report.cases.push({ name, passed: true, statusRequests });
        await context.close();
      }
  assert.deepEqual(report.errors, []);
} catch (error) {
  report.failure = String(error.stack || error);
  if (page && !page.isClosed())
    await page
      .screenshot({ path: out + "failure.png", fullPage: true })
      .catch(() => {});
  process.exitCode = 1;
} finally {
  await writeFile(out + "report.json", JSON.stringify(report, null, 2));
  await browser?.close();
  await vite.close();
}
console.log(JSON.stringify(report));
