import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";
const out = fileURLToPath(
  new URL("../../output/ui25/public-browser/", import.meta.url),
);
await mkdir(out, { recursive: true });
const vite = await createServer({
  root: fileURLToPath(new URL("../", import.meta.url)),
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
const report = { fixtureData: true, cases: [], errors: [] };
let browser, page;
const boundary = Date.parse("2027-01-01T00:00:00+04:00") / 1000;
try {
  await vite.listen();
  const base = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ channel: "chrome", headless: true });
  for (const language of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const name = `${language}-${theme}-${width}`;
        let at = boundary - 1,
          mode = "normal",
          style = "banner",
          consent = true;
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
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const url = new URL(route.request().url()),
              p = url.pathname;
            let data = {};
            if (p === "/api/user/self" || p === "/api/admin/auth/state")
              return route.fulfill({
                status: 401,
                json: { success: false, message: "Anonymous fixture" },
              });
            if (p === "/api/localization")
              data = { default_language: "en", languages: ["en", "zh", "ar"] };
            if (p === "/api/public/home") data = { models: [], groups: [] };
            if (p === "/api/legal-policy")
              data = {
                region_id: "fixture-me",
                timezone: "Asia/Dubai",
                cookie_consent: consent,
                cookie_consent_mode: style,
                consent_version: "necessary-v1",
              };
            if (p.startsWith("/api/legal/")) {
              if (mode === "missing" || p.endsWith("/unknown"))
                return route.fulfill({
                  status: 404,
                  json: { success: false, message: "not found" },
                });
              if (mode === "error")
                return route.fulfill({
                  status: 503,
                  json: { success: false, message: "unavailable" },
                });
              const kind = p.split("/").at(-1),
                lang = url.searchParams.get("language");
              data = {
                kind,
                language: lang,
                version: at < boundary ? "v2" : "v3",
                content:
                  `Synthetic ${kind} ${lang}\n` +
                  "Long-document-content-".repeat(25) +
                  "\n<script>window.legalInjected=true</script>",
                effective_at:
                  at < boundary
                    ? Date.parse("2026-12-01T00:00:00+04:00") / 1000
                    : boundary,
                timezone: "Asia/Dubai",
                now: at,
                next_effective_at: at < boundary ? boundary : 0,
              };
            }
            await route.fulfill({ json: { success: true, data } });
          },
        );
        page = await context.newPage();
        page.setDefaultTimeout(12000);
        page.on("pageerror", (e) =>
          report.errors.push({ name, error: e.message }),
        );
        await page.clock.install({ time: new Date((boundary - 1) * 1000) });
        const fit = async () =>
          assert.ok(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
            name + " page overflow",
          );
        await page.goto(base + "legal/terms");
        await page.locator('[data-legal-version="v2"]').waitFor();
        await page.locator(".cookie-banner").waitFor();
        await fit();
        assert.equal(
          await page.locator("html").getAttribute("dir"),
          language === "ar" ? "rtl" : "ltr",
        );
        assert.deepEqual(await context.cookies(), []);
        assert.equal(
          await page.evaluate(() => window.legalInjected),
          undefined,
        );
        await page.screenshot({
          path: out + name + "-consent.png",
          fullPage: true,
        });
        await page.locator(".cookie-banner button").click();
        assert.equal(await page.locator(".cookie-banner").count(), 0);
        const receipt = await page.evaluate(() =>
          JSON.parse(localStorage.getItem("myapis-cookie-consent-v1")),
        );
        assert.equal(receipt.choice, "necessary");
        await page.reload();
        await page.locator('[data-legal-version="v2"]').waitFor();
        assert.equal(await page.locator(".cookie-banner").count(), 0);
        assert.deepEqual(await context.cookies(), []);
        at = boundary;
        await page.clock.fastForward(1100);
        await page.locator('[data-legal-version="v3"]').waitFor();
        assert.match(await page.locator(".legal-metadata").innerText(), /2027/);
        for (const kind of ["privacy", "cookies", "refund"]) {
          await page.locator(`.legal-nav a[href='/legal/${kind}']`).click();
          await page.locator('[data-legal-version="v3"]').waitFor();
          await page
            .locator(".legal-document-body")
            .filter({ hasText: `Synthetic ${kind} ${language}` })
            .waitFor();
          await fit();
        }
        await page.screenshot({
          path: out + name + "-legal.png",
          fullPage: true,
        });
        await page.goto(base + "legal/cookies");
        await page.locator('[data-legal-version="v3"]').waitFor();
        await page.locator(".cookie-settings-button").click();
        await page.locator(".cookie-dialog").waitFor();
        await fit();
        await page.keyboard.press("Escape");
        await page.locator(".cookie-dialog").waitFor({ state: "hidden" });
        await page.waitForFunction(() =>
          document.activeElement?.classList.contains("cookie-settings-button"),
        );
        if (language === "zh" && theme === "light" && width === 1440) {
          mode = "missing";
          for (const kind of ["terms", "privacy", "cookies", "refund"]) {
            await page.goto(base + "legal/" + kind);
            await page.locator(".legal-feedback[role='status']").waitFor();
            assert.equal(await page.locator(".legal-nav a").count(), 4);
            assert.equal(await page.locator(".legal-document-body").count(), 0);
            assert.equal(
              await page
                .locator('.legal-nav a[aria-current="page"]')
                .getAttribute("href"),
              "/legal/" + kind,
            );
          }
          mode = "error";
          await page.reload();
          await page.locator(".legal-feedback[role='alert']").waitFor();
          mode = "normal";
          await page.locator(".legal-feedback button").click();
          await page.locator('[data-legal-version="v3"]').waitFor();
          await page.goto(base + "legal/unknown");
          await page.getByRole("heading", { name: "未找到法律文本" }).waitFor();
          assert.equal(await page.locator(".legal-document-body").count(), 0);
          style = "dialog";
          await page.evaluate(() =>
            localStorage.removeItem("myapis-cookie-consent-v1"),
          );
          await page.goto(base + "legal/terms");
          await page.locator(".cookie-dialog").waitFor();
          await page.locator('.cookie-dialog a[href="/legal/cookies"]').click();
          await page
            .locator('.legal-nav a[aria-current="page"][href="/legal/cookies"]')
            .waitFor();
          assert.equal(await page.locator(".cookie-dialog").count(), 0);
          await page.locator(".cookie-banner").waitFor();
          await page.locator(".cookie-banner button").click();
          await page.reload();
          await page.locator('[data-legal-version="v3"]').waitFor();
          assert.equal(await page.locator(".cookie-dialog").count(), 0);
          consent = false;
          await page.evaluate(() =>
            localStorage.removeItem("myapis-cookie-consent-v1"),
          );
          await page.reload();
          await page.locator('[data-legal-version="v3"]').waitFor();
          assert.equal(
            await page.locator(".cookie-dialog,.cookie-banner").count(),
            0,
          );
          await page.goto(base + "login");
          await page.locator("[data-auth-policy=terms]").click();
          await page
            .locator('.auth-policy [data-legal-version="v3"]')
            .waitFor();
          await page.locator(".auth-back").click();
          await page.locator("[data-auth-policy=privacy]").waitFor();
        }
        report.cases.push({ name, passed: true });
        await context.close();
      }
  assert.deepEqual(report.errors, []);
} catch (e) {
  if (page && !page.isClosed())
    await page
      .screenshot({ path: out + "failure.png", fullPage: true })
      .catch(() => {});
  throw e;
} finally {
  await writeFile(out + "report.json", JSON.stringify(report, null, 2));
  await browser?.close();
  await vite.close();
}
console.log(JSON.stringify(report));
