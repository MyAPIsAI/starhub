import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { chromium } from "playwright";

// Run against the isolated TestP6BrowserServer fixture and the current frontend.
const base = process.env.STARHUB_AUTH_E2E_URL || "http://127.0.0.1:39781";
const out =
  process.env.STARHUB_AUTH_EVIDENCE_DIR || "test-results/console-entry";
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: "chrome", headless: true });
const report = { checks: [], errors: [] };
async function newPage(language = "zh", width = 1440) {
  const context = await browser.newContext({
    viewport: { width, height: 900 },
    reducedMotion: "reduce",
  });
  await context.addInitScript(
    ({ language, width }) => {
      localStorage.setItem("xy-language", language);
      localStorage.setItem("xy-theme", width === 1440 ? "light" : "dark");
    },
    { language, width },
  );
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on("pageerror", (error) => report.errors.push(String(error)));
  return page;
}
async function enter(page) {
  if (page.viewportSize().width < 768) {
    await page.locator(".veyra-public-menu").click();
    await page
      .locator(".veyra-public-mobile-actions .veyra-public-console")
      .click();
  } else await page.locator(".veyra-public-desktop-console").click();
}
async function expectLogin(page) {
  await page.waitForURL(`${base}/login`);
  await page.getByRole("dialog").waitFor();
  assert.ok(await page.locator("#auth-account").isVisible());
  assert.equal(await page.locator(".xy-sidebar").count(), 0);
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  );
}
try {
  for (const language of ["zh", "en", "ar"]) {
    for (const width of [1440, 390, 320]) {
      const page = await newPage(language, width);
      for (const route of ["/", "/models", "/docs", "/status"]) {
        await page.goto(base + route);
        await page
          .locator(".veyra-public-login")
          .waitFor({ state: "attached" });
        await enter(page);
        await expectLogin(page);
        if (route === "/")
          await page.screenshot({
            path: `${out}/${language}-${width}-login.png`,
          });
        await page.keyboard.press("Escape");
        await page.waitForURL(`${base}/`);
        await page.getByRole("dialog").waitFor({ state: "detached" });
      }
      await page.goto(base + "/console/keys");
      await expectLogin(page);
      await page.locator("#auth-account").fill("member");
      await page.locator("#auth-password").fill("p6-password");
      await page.locator("#auth-consent").check();
      await page.locator('.auth-form button[type="submit"]').click();
      await page.waitForURL(`${base}/console`);
      await page.locator(".xy-sidebar").waitFor({ state: "attached" });
      await page.goto(base + "/");
      await page
        .locator('.veyra-public-header a[href="/console/settings"]')
        .first()
        .waitFor({ state: "attached" });
      await enter(page);
      await page.waitForURL(`${base}/console`);
      assert.equal(await page.locator(".auth-dialog").count(), 0);
      await page.context().clearCookies();
      await page.goto(base + "/console");
      await expectLogin(page);
      report.checks.push(
        `${language}/${width}: public console links, Escape, protected deep link, login, signed-in entry and expired session`,
      );
      console.log("PASS", report.checks.at(-1));
      await page.context().close();
    }
  }
  const checking = await newPage();
  let release;
  const gate = new Promise((resolve) => {
    release = resolve;
  });
  await checking.route("**/api/user/self", async (route) => {
    await gate;
    await route.continue();
  });
  await checking.goto(base + "/");
  await checking.locator('.veyra-public-actions [role="status"]').waitFor();
  await enter(checking);
  await checking.locator('.route-loading[role="status"]').waitFor();
  assert.equal(await checking.locator(".auth-dialog").count(), 0);
  release();
  await expectLogin(checking);
  report.checks.push(
    "Session checking finishes before showing anonymous login",
  );
  await checking.context().close();

  const unavailable = await newPage();
  await unavailable.route("**/api/user/self", (route) =>
    route.fulfill({
      status: 503,
      json: { success: false, message: "Temporary fixture failure" },
    }),
  );
  await unavailable.goto(base + "/");
  await enter(unavailable);
  await unavailable.locator('.route-loading[role="alert"]').waitFor();
  assert.equal(await unavailable.locator(".auth-dialog").count(), 0);
  await unavailable.unroute("**/api/user/self");
  await unavailable.locator(".route-loading button").click();
  await expectLogin(unavailable);
  report.checks.push(
    "An unavailable session remains gated and retry opens login after a confirmed 401",
  );
  await unavailable.context().close();
  assert.deepEqual(report.errors, []);
} finally {
  await writeFile(`${out}/report.json`, JSON.stringify(report, null, 2));
  await browser.close();
}
