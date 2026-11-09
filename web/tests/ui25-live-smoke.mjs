import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
const out = new URL("../../output/ui25/live-browser/", import.meta.url);
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: "chrome", headless: true });
const report = { anonymous: true, fixtureData: false, pages: [], errors: [] };
try {
  const response = await fetch("http://127.0.0.1:3000/api/legal-policy");
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("cache-control"), "no-store");
  const policy = (await response.json()).data;
  assert.deepEqual(
    Object.keys(policy).sort(),
    [
      "region_id",
      "timezone",
      "cookie_consent",
      "cookie_consent_mode",
      "consent_version",
    ].sort(),
  );
  report.policy = policy;
  for (const language of ["zh", "en", "ar"]) {
    const context = await browser.newContext({
      viewport: { width: language === "ar" ? 390 : 1440, height: 950 },
    });
    await context.addInitScript(
      ({ language }) => {
        localStorage.setItem("xy-language", language);
        localStorage.setItem("xy-theme", language === "ar" ? "dark" : "light");
      },
      { language },
    );
    const page = await context.newPage();
    page.on("pageerror", (e) => report.errors.push(e.message));
    for (const kind of ["terms", "privacy", "cookies", "refund"]) {
      const result = await context.request.get(
        `http://127.0.0.1:5173/api/legal/${kind}?language=${language}`,
      );
      assert.ok([200, 404].includes(result.status()));
      await page.goto(`http://127.0.0.1:5173/legal/${kind}`, {
        waitUntil: "domcontentloaded",
      });
      await page
        .locator(
          result.status() === 404
            ? ".legal-feedback[role=status]"
            : ".published-legal",
        )
        .waitFor();
      assert.equal(await page.locator(".legal-nav a").count(), 4);
      assert.equal(
        await page
          .locator('.legal-nav a[aria-current="page"]')
          .getAttribute("href"),
        "/legal/" + kind,
      );
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      );
      if (!policy.cookie_consent)
        assert.equal(
          await page.locator(".cookie-banner,.cookie-dialog").count(),
          0,
        );
      report.pages.push({
        language,
        kind,
        status: result.status(),
        passed: true,
      });
    }
    await page.screenshot({
      path: fileURLToPath(new URL(language + "-refund.png", out)),
      fullPage: true,
    });
    await page.goto("http://127.0.0.1:5173/legal/cookies", {
      waitUntil: "domcontentloaded",
    });
    await page.locator(".cookie-settings-button").click();
    await page.locator(".cookie-dialog").waitFor();
    await page.keyboard.press("Escape");
    await page.locator(".cookie-dialog").waitFor({ state: "hidden" });
    assert.deepEqual(await context.cookies(), []);
    await context.close();
  }
  for (const route of ["/api/admin/legal", "/api/admin/region-profile"]) {
    assert.equal((await fetch("http://127.0.0.1:3000" + route)).status, 401);
  }
  assert.deepEqual(report.errors, []);
} finally {
  await browser.close();
  await writeFile(new URL("report.json", out), JSON.stringify(report, null, 2));
}
console.log(JSON.stringify(report));
