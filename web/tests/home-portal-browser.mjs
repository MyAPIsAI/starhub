import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import { preview } from "vite";

const root = fileURLToPath(new URL("../", import.meta.url));
const out = fileURLToPath(
  new URL("../test-results/home-portal/", import.meta.url),
);
await mkdir(out, { recursive: true });
let server;
let browser;
const report = {
  checks: [],
  errors: [],
  screenshots: [],
  api: "Mocked for visual testing; no live relay calls",
  startedAt: new Date().toISOString(),
};
async function check(name, run) {
  try {
    await run();
    report.checks.push({ name, passed: true });
    console.log("PASS", name);
  } catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.error("FAIL", name, error);
  }
}
try {
  if (!process.env.VEYRA_URL) {
    server = await preview({
      root,
      logLevel: "error",
      preview: { host: "127.0.0.1", port: 0, strictPort: false },
    });
  }
  const base =
    process.env.VEYRA_URL ||
    `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch({
    channel: process.env.PLAYWRIGHT_CHANNEL || "chrome",
    headless: true,
  });
  async function open({
    width = 1440,
    height = 1000,
    language = "zh",
    theme = "light",
    reducedMotion = "no-preference",
  } = {}) {
    const context = await browser.newContext({
      viewport: { width, height },
      reducedMotion,
    });
    await context.addInitScript(
      ({ language, theme }) => {
        localStorage.setItem("xy-language", language);
        localStorage.setItem("xy-theme", theme);
      },
      { language, theme },
    );
    await context.route(
      (url) =>
        url.origin === new URL(base).origin && url.pathname.startsWith("/api/"),
      (route) => {
        const path = new URL(route.request().url()).pathname;
        if (path === "/api/user/self")
          return route.fulfill({
            status: 401,
            json: { success: false, message: "login required" },
          });
        const data =
          path === "/api/plaza/models"
            ? { total: 0, page: 1, page_size: 20, group: "default", items: [] }
            : path === "/api/plaza/filters"
              ? {
                  total: 0,
                  group: "default",
                  vendors: [],
                  tags: [],
                  price_histogram: [],
                }
              : [];
        return route.fulfill({ json: { success: true, data } });
      },
    );
    const page = await context.newPage();
    page.setDefaultTimeout(10000);
    page.on("pageerror", (error) => report.errors.push(String(error)));
    await page.goto(base);
    await page.locator(".portal-hero").waitFor();
    await page.waitForFunction(
      () =>
        document.querySelector(".home-portal").dataset.heroProgress !==
        undefined,
    );
    return { page, context };
  }
  async function snap(page, name) {
    await page.screenshot({ path: `${out}/${name}.png` });
    report.screenshots.push(`${name}.png`);
  }
  async function scroll(page, ratio) {
    await page.evaluate((ratio) => {
      const scene = document.querySelector(".portal-scene");
      const height = document.querySelector(".portal-sticky").offsetHeight;
      window.scrollTo({
        top:
          scene.getBoundingClientRect().top +
          scrollY +
          (scene.offsetHeight - height) * ratio,
        behavior: "instant",
      });
    }, ratio);
    await page.waitForFunction(
      (ratio) =>
        Math.abs(
          Number(document.querySelector(".home-portal").dataset.heroProgress) -
            ratio,
        ) < 0.02,
      ratio,
    );
  }
  async function fits(page) {
    const issues = await page.evaluate(() => {
      const issues = [];
      if (document.documentElement.scrollWidth > innerWidth + 1)
        issues.push("horizontal overflow");
      for (const el of document.querySelectorAll(
        ".portal-model, .portal-actions a, .portal-actions button, .portal-motion",
      )) {
        const rect = el.getBoundingClientRect();
        if (rect.left < -1 || rect.right > innerWidth + 1)
          issues.push(`clipped control: ${el.textContent}`);
      }
      const stage = document
        .querySelector(".portal-stage")
        .getBoundingClientRect();
      const caption = document
        .querySelector(".portal-caption")
        .getBoundingClientRect();
      if (stage.bottom > caption.top + 1)
        issues.push("diagram overlaps caption");
      const actions = document
        .querySelector(".portal-actions")
        .getBoundingClientRect();
      const request = document
        .querySelector(".portal-request")
        .getBoundingClientRect();
      const gate = document
        .querySelector(".portal-gate-wrap")
        .getBoundingClientRect();
      if (request.top < actions.bottom + 8 || gate.top < actions.bottom + 8)
        issues.push("diagram overlaps hero actions");
      const models = [...document.querySelectorAll(".portal-model")].map((el) =>
        el.getBoundingClientRect(),
      );
      for (let i = 1; i < models.length; i++) {
        if (
          innerWidth > 680
            ? models[i - 1].bottom > models[i].top - 2
            : models[i - 1].right > models[i].left - 2
        )
          issues.push("protocol controls overlap");
      }
      return issues;
    });
    assert.deepEqual(issues, []);
  }
  const { page, context } = await open();
  await check("desktop full-screen scene and retained CTA", async () => {
    assert.equal(await page.locator(".portal-model").count(), 3);
    assert.equal(
      await page.locator(".portal-actions a").getAttribute("href"),
      "/console/connect",
    );
    const bottom = await page
      .locator(".portal-hero")
      .evaluate((el) => el.getBoundingClientRect().bottom);
    assert.ok(Math.abs(bottom - 1000) < 2);
    await fits(page);
    await snap(page, "desktop-expanded");
  });
  await check("protocol autoplay changes the selected route", async () => {
    const initial = await page
      .locator('.portal-model[aria-pressed="true"]')
      .getAttribute("aria-label");
    await page.waitForFunction(
      (initial) =>
        document
          .querySelector('.portal-model[aria-pressed="true"]')
          .getAttribute("aria-label") !== initial,
      initial,
      { timeout: 7000 },
    );
  });
  await check(
    "keyboard protocol selection updates the route and caption",
    async () => {
      await page.locator(".portal-model").nth(1).focus();
      await page.keyboard.press("Enter");
      assert.equal(
        await page.locator(".portal-model").nth(1).getAttribute("aria-pressed"),
        "true",
      );
      await page.keyboard.press("Tab");
      await page.keyboard.press("Space");
      assert.equal(
        await page.locator(".portal-model").nth(2).getAttribute("aria-pressed"),
        "true",
      );
      assert.equal(
        await page.locator(".portal-caption b").innerText(),
        "Gemini",
      );
    },
  );
  await check(
    "pause removes scroll motion and pauses decorative animation",
    async () => {
      await page.locator(".portal-motion").click();
      assert.equal(
        await page.locator(".home-portal").getAttribute("data-experience"),
        "quiet",
      );
      assert.equal(
        await page.locator(".portal-motion").getAttribute("aria-pressed"),
        "true",
      );
      assert.equal(
        await page
          .locator(".portal-path-flow")
          .first()
          .evaluate((el) => getComputedStyle(el).animationPlayState),
        "paused",
      );
      await page.locator(".portal-model").first().click();
      assert.equal(
        await page
          .locator(".portal-model")
          .first()
          .getAttribute("aria-pressed"),
        "true",
      );
      await page.locator(".portal-motion").click();
      assert.equal(
        await page.locator(".home-portal").getAttribute("data-experience"),
        "full",
      );
    },
  );
  await check(
    "native scrolling scales, shifts and fades the hero",
    async () => {
      await scroll(page, 0.4);
      const state = await page.locator(".home-portal").evaluate((el) => ({
        scale: Number(el.style.getPropertyValue("--hero-scale")),
        opacity: Number(el.style.getPropertyValue("--hero-opacity")),
        shift: parseFloat(el.style.getPropertyValue("--hero-shift")),
        sticky: document.querySelector(".portal-sticky").getBoundingClientRect()
          .top,
      }));
      assert.ok(state.scale < 1 && state.scale > 0.88);
      assert.ok(state.opacity < 1 && state.opacity > 0);
      assert.ok(state.shift < 0);
      assert.ok(Math.abs(state.sticky) < 2);
      await snap(page, "desktop-transition");
    },
  );
  await check(
    "collapsed hero is inert and moves focus into visible content",
    async () => {
      await scroll(page, 0);
      await page.locator(".portal-model").first().focus();
      await scroll(page, 1);
      assert.equal(
        await page.locator(".portal-visual").evaluate((el) => el.inert),
        true,
      );
      assert.equal(
        await page.locator(".portal-visual").getAttribute("aria-hidden"),
        "true",
      );
      assert.ok(
        await page.evaluate(() =>
          document
            .querySelector(".home-portal-content")
            .contains(document.activeElement),
        ),
      );
      await snap(page, "desktop-collapsed");
    },
  );
  await check("reverse scrolling restores the hero", async () => {
    await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
    await page.waitForFunction(
      () => document.querySelector(".portal-visual").inert === false,
    );
    assert.equal(
      await page
        .locator(".portal-visual")
        .evaluate((el) => getComputedStyle(el).opacity),
      "1",
    );
  });
  await check(
    "scroll cue opens the content without trapping input",
    async () => {
      await page.locator(".portal-scroll-cue").click();
      await page.waitForFunction(
        () =>
          Number(document.querySelector(".home-portal").dataset.heroProgress) >
          0.98,
      );
      const top = await page
        .locator(".home-portal-content")
        .evaluate((el) => el.getBoundingClientRect().top);
      assert.ok(Math.abs(top) < 3);
    },
  );
  await check("resize recalculates the scene for mobile", async () => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
    await page.waitForFunction(
      () => !document.querySelector(".portal-visual").inert,
    );
    await fits(page);
    await snap(page, "mobile-zh-light");
  });
  await check("existing demo dialog remains usable", async () => {
    await page.locator(".portal-demo").click();
    await page.getByRole("dialog").waitFor();
    await page.keyboard.press("Escape");
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    assert.ok(
      await page
        .locator(".portal-demo")
        .evaluate((el) => el === document.activeElement),
    );
  });
  await check("navigation away unmounts the animation cleanly", async () => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.locator('.veyra-public-nav a[href="/models"]').click();
    await page.waitForURL("**/models");
    await page.locator(".home-portal").waitFor({ state: "detached" });
    await page.locator(".catalog-browser").waitFor();
    assert.equal(await page.locator(".home-portal").count(), 0);
    await page.evaluate(() => window.scrollTo(0, 100));
    await page.goBack();
    await page.locator(".portal-hero").waitFor();
  });
  await context.close();
  for (const scenario of [
    {
      name: "desktop-720p-light",
      width: 1280,
      height: 720,
      language: "zh",
      theme: "light",
    },
    {
      name: "tablet-en-light",
      width: 768,
      height: 1024,
      language: "en",
      theme: "light",
    },
    {
      name: "desktop-en-dark",
      width: 1440,
      height: 900,
      language: "en",
      theme: "dark",
    },
    {
      name: "mobile-ar-dark",
      width: 390,
      height: 844,
      language: "ar",
      theme: "dark",
    },
    {
      name: "compact-en-light",
      width: 320,
      height: 568,
      language: "en",
      theme: "light",
    },
  ]) {
    await check(scenario.name, async () => {
      const { page, context } = await open(scenario);
      try {
        await fits(page);
        assert.equal(
          await page.locator("html").getAttribute("dir"),
          scenario.language === "ar" ? "rtl" : "ltr",
        );
        await snap(page, scenario.name);
      } finally {
        await context.close();
      }
    });
  }
  await check(
    "reduced motion is static, selectable and responds to preference changes",
    async () => {
      const { page, context } = await open({
        width: 390,
        height: 844,
        reducedMotion: "reduce",
      });
      try {
        assert.equal(
          await page.locator(".home-portal").getAttribute("data-experience"),
          "quiet",
        );
        assert.ok(await page.locator(".portal-motion").isDisabled());
        assert.equal(
          await page
            .locator(".portal-path-flow")
            .first()
            .evaluate((el) => getComputedStyle(el).animationName),
          "none",
        );
        await page.locator(".portal-model").nth(1).click();
        assert.equal(
          await page
            .locator(".portal-model")
            .nth(1)
            .getAttribute("aria-pressed"),
          "true",
        );
        await fits(page);
        await snap(page, "mobile-reduced-motion");
        await page.emulateMedia({ reducedMotion: "no-preference" });
        await page.waitForFunction(
          () =>
            document.querySelector(".home-portal").dataset.experience ===
            "full",
        );
        assert.ok(await page.locator(".portal-motion").isEnabled());
      } finally {
        await context.close();
      }
    },
  );
  assert.deepEqual(report.errors, []);
} catch (error) {
  report.errors.push(String(error));
} finally {
  await browser?.close();
  if (server)
    await new Promise((resolve, reject) =>
      server.httpServer.close((error) => (error ? reject(error) : resolve())),
    );
  report.finishedAt = new Date().toISOString();
  await writeFile(`${out}/report.json`, JSON.stringify(report, null, 2));
}
const failed = report.checks.filter((item) => !item.passed).length;
console.log(
  `${report.checks.length - failed}/${report.checks.length} checks passed; ${report.errors.length} page/runtime errors. Report: ${out}/report.json`,
);
if (failed || report.errors.length) process.exitCode = 1;
