import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
const out = fileURLToPath(
  new URL("../../output/status-demo/live/", import.meta.url),
);
await mkdir(out, { recursive: true });
const report = { fixtureData: false, cases: [], errors: [] };
const browser = await chromium.launch({ channel: "chrome", headless: true });
try {
  for (const [language, theme, width] of [
    ["zh", "light", 1440],
    ["ar", "dark", 390],
  ]) {
    const context = await browser.newContext({
      viewport: { width, height: 1000 },
      reducedMotion: "reduce",
    });
    await context.addInitScript(
      ({ language, theme }) => {
        localStorage.setItem("xy-language", language);
        localStorage.setItem("xy-theme", theme);
      },
      { language, theme },
    );
    const page = await context.newPage();
    page.on("pageerror", (e) => report.errors.push(e.message));
    const get = async (path) => {
      const response = await context.request.get(
        "http://127.0.0.1:5173" + path,
      );
      assert.equal(response.status(), 200);
      const body = await response.json();
      assert.equal(body.success, true);
      return body.data;
    };
    const home = await get("/api/public/home"),
      status = await get("/api/status/models");
    assert.ok(Array.isArray(home.models) && Array.isArray(home.groups));
    assert.equal(status.refresh_seconds, 300);
    for (const item of status.items) {
      assert.equal(item.daily.length, 90);
      assert.deepEqual(
        Object.keys(item).sort(),
        [
          "current",
          "daily",
          "id",
          "last_24_hours",
          "name",
          "protocols",
          "tags",
          "vendor",
        ].sort(),
      );
      if (!item.current.samples)
        assert.equal(item.current.state, "insufficient_data");
      if (!item.last_24_hours.samples)
        assert.equal(item.last_24_hours.success_rate, null);
    }
    assert.equal(
      (
        await context.request.get("http://127.0.0.1:5173/api/service-quality")
      ).status(),
      401,
    );
    await page.goto("http://127.0.0.1:5173/");
    await page.locator("#home-models-title").waitFor();
    if (home.models.length)
      await page
        .locator(".public-home-card")
        .filter({ hasText: home.models[0].id })
        .waitFor();
    assert.doesNotMatch(
      await page.locator(".public-home").innerText(),
      /38\.2045|24,892|99\.82%|528\.40/,
    );
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    );
    await page.screenshot({
      path: out + `${language}-${width}-home.png`,
      fullPage: true,
    });
    await page.goto("http://127.0.0.1:5173/status");
    if (status.items.length)
      await page.locator("[data-status-model]").first().waitFor();
    await page.locator(".status-update time").waitFor();
    assert.equal(
      await page.locator("[data-status-model]").count(),
      status.items.length,
    );
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    );
    await page.screenshot({ path: out + `${language}-${width}-status.png` });
    if (status.items.length)
      await page
        .locator("[data-status-model]")
        .first()
        .screenshot({ path: out + `${language}-${width}-model.png` });
    if (status.items.length) {
      const hourly = await get(
        `/api/status/models/${encodeURIComponent(status.items[0].id)}/history`,
      );
      assert.equal(hourly.history.hours.length, 24);
      await page
        .locator("[data-status-model] .status-text-button")
        .first()
        .click();
      const dialog = page.getByRole("dialog");
      await dialog.locator(".status-hour-bars button").first().waitFor();
      assert.equal(
        await dialog.locator(".status-hour-bars button").count(),
        24,
      );
      await dialog.screenshot({ path: out + `${language}-${width}-hours.png` });
      await page.keyboard.press("Escape");
      await dialog.waitFor({ state: "hidden" });
    }
    report.cases.push({
      language,
      width,
      modelCount: home.models.length,
      groupCount: home.groups.length,
      statusCount: status.total,
      timezone: status.timezone,
      passed: true,
    });
    await context.close();
  }
  assert.deepEqual(report.errors, []);
} finally {
  await browser.close();
  await writeFile(out + "report.json", JSON.stringify(report, null, 2));
}
console.log(JSON.stringify(report));
