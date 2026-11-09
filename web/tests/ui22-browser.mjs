import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url));
const out = fileURLToPath(
  new URL("../../output/catalog-demo-refresh/browser/", import.meta.url),
);
await mkdir(out, { recursive: true });
const pricing = {
  mode: "per_token",
  unit: "million_tokens",
  input_per_m: 1.2,
  output_per_m: 4.8,
  cache_read_per_m: 0.6,
  cache_write_per_m: 1.2,
  official_input_per_m: 2,
  official_output_per_m: 8,
  group_ratio: 0.6,
  explicit: true,
  currency: "USD",
};
const sla = {
  targets: { success_rate_ppm: 999000, p95_latency_ms: 800 },
  revision: 1,
  updated_at: 1794205390,
};
const groups = [
  {
    id: "lite",
    name: "Lite",
    multiplier: 0.4,
    available: false,
    callable: false,
    pricing: { ...pricing, input_per_m: 0.8, output_per_m: 3.2 },
    protocols: [],
    service_level: sla,
  },
  {
    id: "standard",
    name: "Standard",
    multiplier: 0.6,
    available: true,
    callable: true,
    pricing,
    protocols: ["OpenAI", "Anthropic"],
    service_level: sla,
  },
];
const row = {
  id: "catalog-chat",
  name: "Catalog Chat",
  vendor: "openai",
  vendor_display: "OpenAI",
  description: "",
  tags: ["text", "vision"],
  context_length: 128000,
  max_output: 4096,
  protocols: ["OpenAI", "Anthropic"],
  pricing,
  public_catalog: true,
  kind: "specified",
  price_group: "standard",
  callable: true,
  group_prices: groups,
  available_groups: ["standard"],
  is_available: true,
  official_source: "https://example.com/pricing",
  official_verified: "2026-11-09",
  score: null,
  calls_7d: null,
  created_at: 0,
  sort_weight: 0,
};
const smart = {
  ...row,
  id: "myapis-auto",
  name: "MyAPIs Auto",
  vendor: "myapis",
  vendor_display: "MyAPIs",
  tags: ["text"],
  kind: "smart",
  pricing: {
    ...pricing,
    mode: "tiered",
    input_per_m: null,
    output_per_m: null,
  },
};
const audio = {
  ...row,
  id: "audio-fixture",
  name: "Audio Model",
  vendor: "audio-vendor",
  vendor_display: "Audio Vendor",
  tags: ["audio"],
};
const report = { fixtureData: true, cases: [], errors: [] };
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
let browser;
try {
  await vite.listen();
  const origin = vite.resolvedUrls.local[0].replace(/\/$/, "");
  browser = await chromium.launch({ channel: "chrome", headless: true });
  for (const lang of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const name = `${lang}-${theme}-${width}`;
        const context = await browser.newContext({
          viewport: { width, height: 1000 },
          reducedMotion: "reduce",
          permissions: ["clipboard-read", "clipboard-write"],
        });
        await context.addInitScript(
          ({ lang, theme }) => {
            localStorage.setItem("xy-language", lang);
            localStorage.setItem("xy-theme", theme);
          },
          { lang, theme },
        );
        let mode = "normal",
          listRequests = 0;
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const url = new URL(route.request().url()),
              p = url.pathname;
            let data = {};
            if (p === "/api/catalog/filters")
              data = {
                vendors: [
                  { value: "myapis", label: "MyAPIs", count: 1 },
                  { value: "openai", label: "OpenAI", count: 1 },
                  { value: "audio-vendor", label: "Audio Vendor", count: 1 },
                ],
                tags: [
                  { value: "text", label: "text", count: 2 },
                  { value: "audio", label: "audio", count: 1 },
                ],
                total: 3,
                price_histogram: [],
                group: "",
              };
            else if (p === "/api/catalog/models") {
              listRequests++;
              if (mode === "error")
                return route.fulfill({
                  status: 503,
                  json: { success: false, message: "catalog unavailable" },
                });
              const items = (
                mode === "empty" || mode === "hidden" ? [] : [smart, row, audio]
              ).filter(
                (r) =>
                  (!url.searchParams.get("vendors") ||
                    r.vendor === url.searchParams.get("vendors")) &&
                  (!url.searchParams.get("tags") ||
                    r.tags.includes(url.searchParams.get("tags"))) &&
                  (!url.searchParams.get("q") ||
                    r.name
                      .toLowerCase()
                      .includes(url.searchParams.get("q").toLowerCase())),
              );
              data = {
                items,
                total: items.length,
                page: 1,
                page_size: 200,
                group: "",
              };
            } else if (p.startsWith("/api/catalog/models/")) {
              const id = decodeURIComponent(
                p.slice("/api/catalog/models/".length),
              );
              let model = id === "myapis-auto" ? smart : row;
              if (mode === "hidden")
                model = {
                  ...row,
                  is_available: false,
                  callable: false,
                  group_prices: groups.map((g) => ({
                    ...g,
                    available: false,
                    callable: false,
                  })),
                };
              data = {
                model,
                group: "standard",
                group_sla: sla,
                max_output: 4096,
                status: {
                  state: "insufficient_data",
                  samples: 0,
                  success_rate: null,
                  ttft_p95_ms: null,
                  window_minutes: 15,
                },
              };
            }
            assert.ok(
              !p.endsWith("/rank"),
              "retired ranking must not be requested",
            );
            await route.fulfill({ json: { success: true, data } });
          },
        );
        const page = await context.newPage();
        page.setDefaultTimeout(12000);
        page.on("pageerror", (e) =>
          report.errors.push({ name, error: e.message }),
        );
        const noOverflow = async () =>
          assert.ok(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
            name + " overflow",
          );
        await page.clock.install();
        await page.goto(origin + "/models");
        await page.locator('[data-model-id="catalog-chat"]').waitFor();
        assert.equal(
          await page
            .locator("[data-model-id]")
            .first()
            .getAttribute("data-model-id"),
          "myapis-auto",
        );
        assert.equal(
          await page.locator("html").getAttribute("dir"),
          lang === "ar" ? "rtl" : "ltr",
        );
        assert.equal(await page.locator(".catalog-ranking").count(), 0);
        assert.equal(
          await page.getByText("Veyra Auto", { exact: true }).count(),
          0,
        );
        await noOverflow();
        await page.screenshot({
          path: out + name + "-list.png",
          fullPage: true,
        });
        const card = page.locator('[data-model-id="catalog-chat"]');
        await card.locator(".catalog-copy-prices").click();
        assert.match(
          await page.evaluate(() => navigator.clipboard.readText()),
          /Input: \$1.2\r?\nOutput: \$4.8/,
        );
        assert.equal(new URL(page.url()).pathname, "/models");
        await page
          .locator(".catalog-capability-filter select")
          .selectOption("text");
        await page.locator('[data-catalog-vendor="openai"]').click();
        await page.locator(".catalog-search input").fill("Catalog");
        await page.locator('[data-catalog-view="list"]').click();
        await page.waitForFunction(
          () => document.querySelectorAll("[data-model-id]").length === 1,
        );
        await page.reload();
        await page.locator('[data-model-id="catalog-chat"]').waitFor();
        assert.equal(
          await page.locator(".catalog-capability-filter select").inputValue(),
          "text",
        );
        assert.equal(
          await page.locator(".catalog-search input").inputValue(),
          "Catalog",
        );
        await page
          .locator('[data-model-id="catalog-chat"]')
          .click({ position: { x: 8, y: 8 } });
        await page.locator(".catalog-group-prices").waitFor();
        await noOverflow();
        assert.equal(await page.locator("[data-group-id]").count(), 2);
        assert.equal(
          await page.locator('[data-group-id="lite"]').getAttribute("class"),
          "detail-group-unavailable",
        );
        await page.locator(".catalog-group-select select").selectOption("lite");
        assert.match(
          await page.locator(".access-public-comparison").innerText(),
          /\$0.8/,
        );
        await page
          .locator(".catalog-examples select")
          .first()
          .selectOption("anthropic");
        await page
          .locator(".catalog-examples select")
          .nth(1)
          .selectOption("python");
        assert.match(
          await page.locator(".catalog-examples pre").innerText(),
          /api.myapis.ai\/v1\/messages/,
        );
        const table = page.locator('.catalog-group-prices [role="region"]');
        await table.focus();
        await page.keyboard.press(lang === "ar" ? "ArrowLeft" : "ArrowRight");
        await page.screenshot({
          path: out + name + "-detail.png",
          fullPage: true,
        });
        await page.locator(".access-model-detail > a").first().click();
        await page.locator('[data-model-id="catalog-chat"]').waitFor();
        assert.equal(
          new URL(page.url()).searchParams.get("capability"),
          "text",
        );
        assert.equal(new URL(page.url()).searchParams.get("view"), "list");
        if (lang === "zh" && theme === "light" && width === 1440) {
          const before = listRequests;
          await page.clock.fastForward(61_000);
          await page.waitForFunction(() => true);
          await page.waitForTimeout(200);
          assert.ok(listRequests > before, "one-minute refresh");
          mode = "hidden";
          await page.goto(origin + "/models/catalog-chat");
          await page.locator(".catalog-group-prices").waitFor();
          assert.equal(
            await page.locator(".detail-group-unavailable").count(),
            2,
          );
          assert.equal(await page.locator(".access-empty").count(), 0);
          mode = "empty";
          await page.goto(origin + "/models");
          await page
            .locator('.catalog-empty:not([aria-busy="true"])')
            .waitFor();
          mode = "error";
          await page.reload();
          await page.clock.fastForward(30_000);
          await page.locator('.catalog-empty[role="alert"]').waitFor();
          await page.screenshot({ path: out + "error.png" });
        }
        report.cases.push({ name, passed: true });
        await context.close();
      }
  assert.deepEqual(report.errors, []);
} finally {
  await writeFile(out + "report.json", JSON.stringify(report, null, 2));
  await browser?.close();
  await vite.close();
}
console.log(JSON.stringify(report));
