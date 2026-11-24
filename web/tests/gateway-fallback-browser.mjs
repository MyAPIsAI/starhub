import { createServer } from "vite";
import { chromium } from "playwright";
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";

const vite = await createServer({
  server: { host: "127.0.0.1", port: 5299, strictPort: false },
  logLevel: "error",
});
await vite.listen();
const browser = await chromium.launch({ channel: "chrome", headless: true });
const out = "playwright-report/gateway-fallback";
await mkdir(out, { recursive: true });
try {
  for (const [language, width, theme] of [
    ["zh", 1440, "light"],
    ["en", 390, "light"],
    ["ar", 390, "dark"],
  ]) {
    const page = await browser.newPage({ viewport: { width, height: 1000 } });
    await page.addInitScript(
      (theme) => localStorage.setItem("xy-theme", theme),
      theme,
    );
    const errors = [];
    page.on("pageerror", (error) => errors.push(String(error)));
    let version = 1;
    let policy = {
      enabled: true,
      retry: true,
      ordinary_fallback: true,
      fallback_interval_seconds: 30,
      max_attempts: 4,
      retry_window_seconds: 60,
      failure_threshold: 5,
      open_seconds: 30,
      max_open_seconds: 300,
      rate_limit_seconds: 30,
      isolation_seconds: 900,
      probe_concurrency: 4,
      probe_timeout_seconds: 30,
      retry_status_codes: "",
      no_retry_status_codes: "400",
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (!path.startsWith("/api/")) return route.continue();
      let data = {};
      if (path === "/api/user/self")
        data = {
          id: 1,
          role: 100,
          username: "browser-root",
          group: "default",
          language,
          status: 1,
          quota: 1000,
        };
      else if (path === "/api/option/upstream") {
        if (route.request().method() === "PUT") {
          const input = route.request().postDataJSON();
          assert.equal(input.expected_version, version);
          policy = JSON.parse(input.value);
          version++;
          data = { applied: true, version };
        } else data = { version, value: JSON.stringify(policy), history: [] };
      } else if (path === "/api/option/gateway")
        data = {
          version,
          options: [
            "ip_rate_limit",
            "user_rate_limit",
            "token_rate_limit",
            "model_rate_limit",
          ].map((key) => ({ key, value: "100" })),
          history: [],
        };
      else if (path === "/api/option/channel-traffic")
        data = {
          version,
          channel_revision: 1,
          mode: "local",
          policy: { rules: [] },
          candidates: [],
          history: [],
        };
      else if (path === "/api/option/status")
        data = { in_sync: true, database_version: version };
      else
        return route.fulfill({
          status: 404,
          json: { success: false, message: "Not used by this fixture" },
        });
      await route.fulfill({ json: { success: true, data } });
    });
    await page.goto(
      `http://127.0.0.1:${vite.httpServer.address().port}/console/gateway-settings`,
    );
    const panel = page.locator(".upstream-panel");
    const toggle = panel.locator('input[type="checkbox"]').first();
    await toggle.waitFor({ timeout: 10000 }).catch(async (error) => {
      console.log(page.url(), await page.locator("body").innerText(), errors);
      throw error;
    });
    assert.equal(await toggle.isChecked(), true);
    await toggle.focus();
    await page.keyboard.press("Space");
    assert.equal(await toggle.isChecked(), false);
    await page.locator("#upstream-fallback_interval_seconds").fill("45");
    await panel.locator('button[type="submit"]').click();
    await page.waitForFunction(
      () =>
        document.querySelector('.upstream-panel button[type="submit"]')
          ?.disabled,
    );
    assert.equal(policy.ordinary_fallback, false);
    assert.equal(policy.fallback_interval_seconds, 45);
    await page.reload();
    await toggle.waitFor();
    assert.equal(await toggle.isChecked(), false);
    assert.equal(
      await page.locator("#upstream-fallback_interval_seconds").inputValue(),
      "45",
    );
    await page.addStyleTag({ content: "* { scroll-behavior: auto !important; }" });
    await panel.evaluate((element) =>
      element.scrollIntoView({ block: "start" }),
    );
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
      "horizontal overflow",
    );
    await page.screenshot({ path: `${out}/${language}-${width}.png` });
    await panel.locator('button[type="submit"]').scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${out}/${language}-${width}-actions.png` });
    assert.deepEqual(errors, []);
    console.log(
      `PASS ${language} ${width}: keyboard toggle, save, reload, layout`,
    );
    await page.close();
  }
} finally {
  await browser.close();
  await vite.close();
}
