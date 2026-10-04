import { chromium } from "playwright";
import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";

const base = process.env.VEYRA_URL || "http://127.0.0.1:5196";
const out = process.env.VEYRA_EVIDENCE_DIR || "docs/verification-2026-09-28";
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || "official-reference";
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: "chrome", headless: true });
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  reducedMotion: "reduce",
});
const page = await context.newPage();
page.setDefaultTimeout(10000);
page.on("dialog", (dialog) => dialog.accept());
const report = {
  base,
  startedAt: new Date().toISOString(),
  checks: [],
  errors: [],
  screenshots: [],
  independentBrowserContext: true,
  businessStateInjection: false,
};
page.on("pageerror", (error) => report.errors.push(String(error)));
const state = () =>
  page.evaluate(() =>
    JSON.parse(localStorage.getItem("veyra-billing-demo-v1")),
  );
const pricing = () =>
  page.evaluate(() =>
    JSON.parse(localStorage.getItem("veyra-platform-pricing-v1")),
  );
async function go(id = "gpt-4.1") {
  await page.goto(`${base}/console/model-pricing?model=${id}`);
  await page.locator(".settings-pricing").waitFor();
}
const editor = () => page.locator(".official-reference-editor");
async function expand() {
  if (!(await editor().getAttribute("open")))
    await editor().locator("summary").click();
}
async function publish() {
  await page.getByRole("button", { name: "检查并发布", exact: true }).click();
  await page.getByRole("dialog").waitFor();
  await page.getByRole("button", { name: "确认发布", exact: true }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
}
async function check(name, fn) {
  try {
    await fn();
    report.checks.push({ name, passed: true });
    console.log("PASS", name);
  } catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.log("FAIL", name, error.stack);
  }
}
async function capture(name) {
  const file = `${prefix}-${name}.png`;
  await page.screenshot({ path: `${out}/${file}`, fullPage: false });
  report.screenshots.push(file);
}
await go();
await check("核验种子独立于基准报价，展开收起可操作", async () => {
  assert.equal(await editor().getAttribute("open"), null);
  await expand();
  assert.equal(
    await editor().getByRole("switch").getAttribute("aria-checked"),
    "true",
  );
  assert.equal(
    await editor().getByLabel("官方普通输入", { exact: true }).inputValue(),
    "2",
  );
  assert.equal(
    await editor().getByLabel("官方模型输出", { exact: true }).inputValue(),
    "8",
  );
  assert.equal(
    await editor().getByLabel("官方缓存写入", { exact: true }).inputValue(),
    "",
  );
  await editor().scrollIntoViewIfNeeded();
  await capture("desktop");
});
await check("参考价编辑有草稿/确认，发布不改基准、余额或历史调用", async () => {
  const before = await state();
  const basePrice = await page
    .getByLabel("GPT-4.1 普通输入", { exact: true })
    .inputValue();
  await editor().getByLabel("官方普通输入", { exact: true }).fill("2.4");
  await page.getByRole("button", { name: "检查并发布", exact: true }).click();
  const dialog = await page.getByRole("dialog").innerText();
  assert.ok(dialog.includes("0 个模型的计费规则变化"));
  assert.ok(dialog.includes("0 把启用密钥受影响"));
  await page.getByRole("button", { name: "继续编辑", exact: true }).click();
  await page.reload();
  await page.locator(".settings-pricing").waitFor();
  await expand();
  assert.equal(
    await editor().getByLabel("官方普通输入", { exact: true }).inputValue(),
    "2.4",
  );
  await publish();
  assert.equal(
    (await pricing()).models["gpt-4.1"].officialReference.rate.pricing
      .input_per_million_usd,
    "2.4",
  );
  assert.equal(
    await page.getByLabel("GPT-4.1 普通输入", { exact: true }).inputValue(),
    basePrice,
  );
  const after = await state();
  for (const key of [
    "balance",
    "ledger",
    "liveRequests",
    "keys",
    "rechargeOrders",
  ])
    assert.deepEqual(after?.[key], before?.[key]);
});
await check(
  "停用经发布和刷新保持，重新启用须填可靠来源不得复制基准",
  async () => {
    await editor().getByRole("switch").click();
    await publish();
    await page.reload();
    await page.locator(".settings-pricing").waitFor();
    await expand();
    assert.equal(
      await editor().getByRole("switch").getAttribute("aria-checked"),
      "false",
    );
    assert.equal((await pricing()).models["gpt-4.1"].officialReference, null);
    await editor().getByRole("switch").click();
    assert.equal(
      await editor().getByLabel("官方普通输入", { exact: true }).inputValue(),
      "",
    );
    assert.equal(
      await page
        .getByRole("button", { name: "检查并发布", exact: true })
        .isDisabled(),
      true,
    );
    await page.getByRole("button", { name: "放弃修改", exact: true }).click();
  },
);
await check("媒体参考按单位维护，非法来源拒绝发布，取消保留原样", async () => {
  await go("veyra-demo-image");
  await expand();
  await editor().getByRole("switch").click();
  assert.equal(
    await editor().getByLabel("官方计价单位", { exact: true }).inputValue(),
    "image",
  );
  await editor()
    .getByLabel("官方每单位价格（美元）", { exact: true })
    .fill("0.04");
  await editor()
    .getByLabel("官方参考来源地址", { exact: true })
    .fill("javascript:alert(1)");
  await editor()
    .getByLabel("官方参考核验日期", { exact: true })
    .fill("2026-09-28");
  await editor()
    .getByLabel("适用条件（修改内容按原文展示）", { exact: true })
    .fill("仅本地验证，1024像素标准图片。");
  assert.equal(
    await page
      .getByRole("button", { name: "检查并发布", exact: true })
      .isDisabled(),
    true,
  );
  assert.ok((await editor().getByRole("alert").innerText()).includes("https"));
  await page.getByRole("button", { name: "放弃修改", exact: true }).click();
  assert.equal(
    await editor().getByRole("switch").getAttribute("aria-checked"),
    "false",
  );
});
await check("390像素与深色三语展开表单无页面横溢出", async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  for (const language of ["zh", "en", "ar"]) {
    await page.evaluate((language) => {
      localStorage.setItem("xy-language", language);
      localStorage.setItem("xy-theme", "dark");
    }, language);
    await go("gemini-2.5-pro");
    await expand();
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    );
    await editor().scrollIntoViewIfNeeded();
    await capture(`mobile-${language}`);
  }
});
report.finishedAt = new Date().toISOString();
await writeFile(
  `${out}/${prefix}-results.json`,
  JSON.stringify(report, null, 2),
);
await browser.close();
if (report.errors.length || report.checks.some((check) => !check.passed))
  process.exitCode = 1;
