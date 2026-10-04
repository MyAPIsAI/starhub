import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { createServer } from "vite";
import { mkdir, writeFile, readFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import assert from "node:assert/strict";

const root = fileURLToPath(new URL("../", import.meta.url));
const output = path.join(root, "docs/screenshots");
await mkdir(output, { recursive: true });
const report = {
  startedAt: new Date().toISOString(),
  status: "running",
  screenshots: [],
  checks: [],
  failures: [],
  consoleErrors: [],
};
const translations = Object.fromEntries(
  await Promise.all(
    ["en", "zh", "ar"].map(async (lang) => [
      lang,
      JSON.parse(
        await readFile(path.join(root, `src/i18n/${lang}.json`), "utf8"),
      ),
    ]),
  ),
);
let browser;
let server;

async function check(name, action) {
  try {
    await action();
    report.checks.push({ name, passed: true });
    console.log(`PASS ${name}`);
  } catch (error) {
    report.checks.push({ name, passed: false });
    report.failures.push({ name, message: String(error) });
    console.error(`FAIL ${name}: ${error.message}`);
  }
}
async function capture(page, name, fullPage = true) {
  await page.screenshot({
    path: path.join(output, `${name}.png`),
    fullPage,
    animations: "disabled",
  });
  report.screenshots.push(`${name}.png`);
}
async function noOverflow(page) {
  const dimensions = await page.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    viewport: innerWidth,
  }));
  assert.ok(
    dimensions.scroll <= dimensions.viewport + 1,
    JSON.stringify(dimensions),
  );
}
async function choose(page, label, value) {
  await page.getByRole("combobox", { name: label, exact: true }).click();
  await page.getByRole("option", { name: value, exact: true }).click();
}
async function audit(page, name) {
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  const violations = result.violations.map((v) => ({
    id: v.id,
    impact: v.impact,
    help: v.help,
    nodes: v.nodes.map((n) => n.target),
  }));
  await writeFile(
    path.join(output, `${name}-accessibility.json`),
    JSON.stringify(violations, null, 2),
  );
  assert.equal(violations.length, 0, JSON.stringify(violations));
}

try {
  server = await createServer({
    root,
    server: { host: "127.0.0.1", port: 0, strictPort: false, open: false },
  });
  await server.listen();
  const address = server.httpServer.address();
  const url = `http://127.0.0.1:${address.port}/design-system`;
  const launch = { headless: true };
  if (process.env.PLAYWRIGHT_EXECUTABLE_PATH)
    launch.executablePath = process.env.PLAYWRIGHT_EXECUTABLE_PATH;
  else if (
    existsSync("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
  )
    launch.channel = "chrome";
  browser = await chromium.launch(launch);
  report.browser = browser.version();

  for (const language of ["en", "ar"]) {
    for (const theme of ["light", "dark"]) {
      for (const size of ["desktop", "mobile"]) {
        const t = translations[language];
        const name = `${language}-${theme}-${size}`;
        const context = await browser.newContext({
          viewport:
            size === "desktop"
              ? { width: 1440, height: 1080 }
              : { width: 390, height: 844 },
          deviceScaleFactor: 1,
          locale: language,
          reducedMotion: "reduce",
          hasTouch: size === "mobile",
        });
        await context.addInitScript(
          ({ language, theme }) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          { language, theme },
        );
        const page = await context.newPage();
        page.on("pageerror", (error) =>
          report.consoleErrors.push({ view: name, message: error.message }),
        );
        page.on("console", (message) => {
          if (message.type() === "error")
            report.consoleErrors.push({ view: name, message: message.text() });
        });
        await page.goto(url);
        await page.locator(".data-table tbody tr").first().waitFor();
        await page.evaluate(() => document.fonts.ready);
        await page.waitForTimeout(400);
        await capture(page, name);
        await capture(page, `${name}-viewport`, false);

        await check(`${name}: no horizontal overflow`, () => noOverflow(page));
        await check(`${name}: language and direction`, async () => {
          assert.equal(
            await page.locator("html").getAttribute("lang"),
            language,
          );
          assert.equal(
            await page.locator("html").getAttribute("dir"),
            language === "ar" ? "rtl" : "ltr",
          );
          assert.equal(
            await page.locator("html").getAttribute("data-theme"),
            theme,
          );
          assert.equal(
            await page
              .locator(".model-cell b")
              .first()
              .evaluate((el) => getComputedStyle(el).direction),
            "ltr",
          );
        });
        await check(`${name}: accessibility`, () => audit(page, name));
        await check(`${name}: drawer direction, close and focus`, async () => {
          const opener = page.getByRole("button", {
            name: t.openDrawer,
            exact: true,
          });
          await opener.click();
          const drawer = page.getByRole("dialog", { name: t.drawerTitle });
          await drawer.waitFor();
          await page.waitForTimeout(350);
          const box = await drawer.boundingBox();
          const width = page.viewportSize().width;
          assert.ok(
            language === "ar"
              ? box.x < 1
              : Math.abs(box.x + box.width - width) < 1,
          );
          await capture(page, `${name}-drawer`, false);
          await page.keyboard.press("Escape");
          await drawer.waitFor({ state: "hidden" });
          await page.waitForTimeout(350);
          assert.ok(
            await opener.evaluate((el) => el === document.activeElement),
            "Drawer opener must regain focus",
          );
        });
        if (size === "desktop" && language === "en" && theme === "light") {
          await check(
            "table: search, empty recovery, pagination and numeric sorting",
            async () => {
              const search = page.getByRole("textbox", {
                name: t.searchRequests,
              });
              await search.fill("does-not-exist");
              await page
                .locator(".request-panel")
                .getByText(t.emptyTitle, { exact: true })
                .waitFor();
              await page
                .locator(".request-panel")
                .getByRole("button", { name: t.clearFilters })
                .click();
              await page.locator(".data-table tbody tr").first().waitFor();
              await page
                .getByRole("button", { name: t.nextPage, exact: true })
                .click();
              assert.ok(
                (await page.locator(".pagination").innerText()).includes(
                  "Page 2",
                ),
              );
              await page
                .getByRole("button", { name: "Sort by Cost", exact: true })
                .click();
              assert.equal(
                await page
                  .locator('th[data-column="cost_nano"]')
                  .getAttribute("aria-sort"),
                "ascending",
              );
              await choose(page, t.filterStatus, t.failed);
              const rows = await page
                .locator(".data-table tbody tr")
                .allTextContents();
              assert.ok(
                rows.length > 0 && rows.every((row) => row.includes(t.failed)),
              );
              await choose(page, t.filterStatus, t.allStatuses);
            },
          );
          await check("form: required error, switch and slider", async () => {
            await page
              .getByRole("button", { name: t.validate, exact: true })
              .click();
            assert.equal(
              await page
                .locator("#configuration-name")
                .getAttribute("aria-invalid"),
              "true",
            );
            await page
              .locator("#configuration-name")
              .fill("Support configuration");
            await page
              .getByRole("button", { name: t.validate, exact: true })
              .click();
            assert.equal(
              await page
                .locator("#configuration-name")
                .getAttribute("aria-invalid"),
              "false",
            );
            const toggle = page.getByRole("switch", { name: t.streaming });
            await toggle.click();
            assert.equal(await toggle.getAttribute("aria-checked"), "false");
            const slider = page.getByRole("slider", { name: t.temperature });
            await slider.focus();
            await page.keyboard.press("ArrowRight");
            assert.equal(await slider.getAttribute("aria-valuenow"), "0.8");
          });
          await check(
            "date range: reject reverse and over 31 days; accept exactly 31",
            async () => {
              await page.locator("#from").fill("2026-08-01");
              await page.locator("#to").fill("2026-09-01");
              await page.getByRole("button", { name: t.applyRange }).click();
              assert.equal(
                await page.locator("#date-error").innerText(),
                t.exportLimit,
              );
              await page.locator("#to").fill("2026-07-30");
              await page.getByRole("button", { name: t.applyRange }).click();
              assert.equal(
                await page.locator("#date-error").innerText(),
                t.rangeInvalid,
              );
              await page.locator("#to").fill("2026-08-31");
              await page.getByRole("button", { name: t.applyRange }).click();
              assert.equal(await page.locator("#date-error").count(), 0);
            },
          );
          await check(
            "code tabs: keyboard selection and valid shell continuation",
            async () => {
              const curl = page.getByRole("tab", {
                name: t.codeCurl,
                exact: true,
              });
              await curl.focus();
              await page.keyboard.press("ArrowRight");
              assert.equal(
                await page
                  .getByRole("tab", { name: t.codePython, exact: true })
                  .getAttribute("aria-selected"),
                "true",
              );
              assert.ok(
                (await page.getByRole("tabpanel").innerText()).includes(
                  "from openai import OpenAI",
                ),
              );
              await curl.click();
              assert.ok(
                !(await page.getByRole("tabpanel").innerText()).includes("\n+"),
              );
            },
          );
          await check("command palette: search and Escape", async () => {
            await page.keyboard.press("Control+k");
            const dialog = page.getByRole("dialog", { name: t.commandTitle });
            await dialog.waitFor();
            await dialog.getByRole("combobox").fill("gpt-4.1");
            assert.ok((await dialog.getByRole("option").count()) > 0);
            await capture(page, "en-light-command", false);
            await page.keyboard.press("Escape");
            await dialog.waitFor({ state: "hidden" });
          });
          await check(
            "demo controls: loading, empty, error and reset",
            async () => {
              await page.locator(".demo-trigger").click();
              for (const state of ["loading", "empty", "error"]) {
                await choose(page, t.stateLabel, t[`${state}Tab`]);
                if (state === "loading")
                  assert.ok(
                    (await page
                      .locator('.request-panel [role="status"]')
                      .count()) > 0,
                  );
                else
                  await page
                    .locator(".request-panel")
                    .getByText(t[`${state}Title`], { exact: true })
                    .waitFor();
                await page.locator(".request-panel").scrollIntoViewIfNeeded();
                await capture(page, `en-light-${state}`, false);
              }
              await page
                .getByRole("button", { name: t.reset, exact: true })
                .click();
              await page.locator(".data-table tbody tr").first().waitFor();
              await page.locator(".demo-trigger").click();
            },
          );
          await check(
            "confirmation: initial cancel focus and focus return",
            async () => {
              const opener = page.getByRole("button", { name: t.openModal });
              await opener.click();
              const dialog = page.getByRole("alertdialog");
              await dialog.waitFor();
              assert.ok(
                await dialog
                  .getByRole("button", { name: t.cancel })
                  .evaluate((el) => el === document.activeElement),
              );
              await capture(page, "en-light-confirmation", false);
              await page.keyboard.press("Escape");
              await dialog.waitFor({ state: "hidden" });
              await page.waitForTimeout(350);
              assert.ok(
                await opener.evaluate((el) => el === document.activeElement),
              );
            },
          );
        }
        if (size === "mobile") {
          await check(`${name}: mobile navigation`, async () => {
            await page
              .getByRole("button", { name: t.menu, exact: true })
              .click();
            const dialog = page.getByRole("dialog", {
              name: t.navigationLabel,
            });
            await dialog.waitFor();
            await capture(page, `${name}-navigation`, false);
            await dialog
              .getByRole("link", { name: t.components, exact: true })
              .click();
            await dialog.waitFor({ state: "hidden" });
          });
          for (const width of [320, 360, 768, 1024]) {
            await check(`${name}: ${width}px overflow`, async () => {
              await page.setViewportSize({ width, height: 900 });
              await page.waitForTimeout(100);
              await noOverflow(page);
            });
          }
        }
        await context.close();
      }
    }
  }
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1080 },
    locale: "zh-CN",
    reducedMotion: "reduce",
  });
  await page.goto(url);
  await page.locator(".data-table tbody tr").first().waitFor();
  await page.evaluate(() => document.fonts.ready);
  await capture(page, "zh-light-desktop");
  await check(
    "Chinese: browser preference and language switching",
    async () => {
      assert.equal(await page.locator("html").getAttribute("lang"), "zh");
      await choose(
        page,
        translations.zh.language,
        translations.zh.languageArabic,
      );
      await page.waitForFunction(() => document.documentElement.dir === "rtl");
      await page.reload();
      assert.equal(await page.locator("html").getAttribute("lang"), "ar");
    },
  );
  await check("browser: no runtime errors", async () =>
    assert.deepEqual(report.consoleErrors, []),
  );
  report.status = report.failures.length ? "failed" : "passed";
} catch (error) {
  report.status = browser ? "failed" : "blocked";
  report.failures.push({
    name: browser ? "runner" : "browser launch",
    message: String(error),
  });
  console.error(String(error));
} finally {
  await browser?.close();
  await server?.close();
  report.finishedAt = new Date().toISOString();
  await writeFile(
    path.join(output, "browser-report.json"),
    JSON.stringify(report, null, 2),
  );
  console.log(
    `Browser QA: ${report.status}; ${report.screenshots.length} screenshots; ${report.failures.length} failures.`,
  );
  if (report.status !== "passed") process.exitCode = 1;
}
