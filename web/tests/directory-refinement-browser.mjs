import { chromium } from "playwright";
import assert from "node:assert/strict";
import { writeFile } from "node:fs/promises";
const browser = await chromium.launch({ channel: "chrome", headless: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const report = { checks: [], errors: [], screenshots: [] };
const out = "docs/verification-2026-09-26-round2";
page.on("pageerror", (e) => report.errors.push(String(e)));
async function go(path) {
  await page.goto("http://127.0.0.1:5196" + path);
  await page.locator("h1").waitFor();
}
async function snap(name) {
  await page.waitForTimeout(180);
  await page.screenshot({ path: `${out}/${name}.png`, fullPage: true });
  report.screenshots.push(`${name}.png`);
}
async function check(name, fn) {
  try {
    await fn();
    report.checks.push({ name, passed: true });
    console.log("PASS", name);
  } catch (e) {
    report.checks.push({ name, passed: false, error: String(e) });
    console.log("FAIL", name, e.stack);
  }
}
await check(
  "媒体目录及详情显示按张/秒价格，没有错误客户端接入入口",
  async () => {
    await go("/models");
    const card = page
      .locator(".catalog-card")
      .filter({ hasText: "veyra-demo-image" });
    assert.ok((await card.innerText()).includes("美元 / 张"));
    assert.equal(
      await card.getByRole("link", { name: "接入此模型", exact: true }).count(),
      0,
    );
    await go("/models/veyra-demo-image");
    await page.locator(".access-inline-label select").selectOption("standard");
    assert.ok(
      (await page.locator("main").innerText()).includes("未提供客户端接口"),
    );
    assert.ok(
      (await page.locator(".directory-price").first().innerText()).includes(
        "$0.024",
      ),
    );
    assert.equal(
      await page.getByRole("link", { name: "开始接入", exact: true }).count(),
      0,
    );
    assert.equal(
      await page
        .getByRole("button", { name: "客户端接入未提供", exact: true })
        .isDisabled(),
      true,
    );
    await snap("directory-image-pricing");
    await go("/models/veyra-demo-video");
    assert.ok(
      (await page.locator(".directory-price").first().innerText()).includes(
        "美元 / 秒",
      ),
    );
    assert.ok(
      (await page.locator(".directory-price").first().innerText()).includes(
        "最低计费量 1",
      ),
    );
    await snap("directory-video-pricing");
  },
);
await check("智能路由候选表排除媒体，仅呈现客户端文本模型", async () => {
  await go("/models/veyra-auto");
  const rows = await page.locator(".access-table tbody tr").allTextContents();
  assert.equal(rows.length, 5);
  assert.ok(
    rows.every(
      (r) => !r.includes("图片计费样例") && !r.includes("视频计费样例"),
    ),
  );
  await snap("directory-smart-pricing");
});
await check(
  "词元模型改表达式后目录和详情同步显示示例费用，失败不冒充免费",
  async () => {
    await page.evaluate(async () => {
      const r = performance
        .getEntriesByType("resource")
        .find((r) => r.name.includes("/src/lib/model-pricing.ts"));
      const m = await import(r.name);
      const c = structuredClone(m.useModelPricing.getState().config);
      c.models["gpt-4.1"].rule = {
        mode: "expression",
        expression: "requests * 0.1",
      };
      const result = m.useModelPricing
        .getState()
        .save(c, "superAdmin", c.revision);
      if (!result.ok) throw new Error(result.reason);
    });
    await go("/models/gpt-4.1");
    await page.locator(".access-inline-label select").selectOption("standard");
    const block = page.locator(".directory-price").first();
    assert.ok((await block.innerText()).includes("表达式计价"));
    assert.ok((await block.innerText()).includes("$0.060000"));
    assert.ok((await block.innerText()).includes("0 张图片、0 秒、1 次请求"));
    await snap("directory-expression-pricing");
    await page.evaluate(async () => {
      const r = performance
        .getEntriesByType("resource")
        .find((r) => r.name.includes("/src/lib/model-pricing.ts"));
      const m = await import(r.name);
      const c = structuredClone(m.useModelPricing.getState().config);
      c.models["gpt-4.1"].rule = {
        mode: "expression",
        expression: "requests / images",
      };
      m.useModelPricing.setState({ config: c });
    });
    assert.ok((await block.innerText()).includes("暂不可估算"));
    assert.ok((await block.innerText()).includes("除数不能为零"));
  },
);
await check("390px媒体详情无横向页面溢出，可查看完整计费规则", async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  await go("/models/veyra-demo-video");
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
    false,
  );
  await snap("directory-mobile-media");
});
await writeFile(
  `${out}/directory-browser-results.json`,
  JSON.stringify(report, null, 2),
);
await browser.close();
if (report.errors.length || report.checks.some((c) => !c.passed))
  process.exitCode = 1;
