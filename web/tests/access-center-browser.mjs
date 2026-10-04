import { chromium } from "playwright";
import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
const base = process.env.VEYRA_TEST_URL ?? "http://127.0.0.1:5196";
const output = "docs/screenshots/access-center-20260926";
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ channel: "chrome", headless: true });
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  permissions: ["clipboard-read", "clipboard-write"],
});
const page = await context.newPage();
const failures = [];
const passed = [];
let keyId = "";
page.on("pageerror", (error) => failures.push(error.message));
async function check(name, fn) {
  try {
    await fn();
    passed.push(name);
    console.log("PASS", name);
  } catch (error) {
    failures.push(`${name}: ${error.message}`);
    console.error("FAIL", name, error.message);
  }
}
try {
  await page.goto(base + "/console/connect");
  await page.getByLabel("Key（密钥）", { exact: true }).waitFor();
  keyId = await page
    .locator("#connect-key option")
    .nth(1)
    .getAttribute("value");
  await check(
    "tool configuration filters protocol without substitution",
    async () => {
      await page.locator("#connect-key").selectOption(keyId);
      await page.locator("#connect-tool").selectOption("cc-switch");
      await page.locator("#connect-target").selectOption("claude-code");
      const options = await page
        .locator("#connect-model option")
        .allTextContents();
      assert(!options.some((value) => value.includes("GPT-4.1")));
      await page
        .locator("#connect-model")
        .selectOption("claude-sonnet-4-20250514");
      assert(
        (await page.locator(".access-code").innerText()).includes(
          "ANTHROPIC_BASE_URL",
        ),
      );
      assert(
        !(await page.locator(".access-code").innerText()).includes("••••"),
      );
      assert.equal(
        await page
          .getByRole("link", { name: "可选：验证配置", exact: true })
          .getAttribute("href"),
        `/console/playground?key=${keyId}&model=claude-sonnet-4-20250514&protocol=Anthropic`,
      );
    },
  );
  await check(
    "copy configuration does not create validation success",
    async () => {
      await page.getByRole("button", { name: "复制配置", exact: true }).click();
      await page
        .getByRole("button", { name: "配置已复制", exact: true })
        .waitFor();
      assert(
        (await page.locator(".access-status").innerText()).includes(
          "等待对应请求验证",
        ),
      );
      assert.equal(
        await page.evaluate(() => navigator.clipboard.readText()),
        await page.locator(".access-code").innerText(),
      );
    },
  );
  await check("refresh retains target, key and model", async () => {
    await page.reload();
    await page.locator("#connect-tool").waitFor();
    assert.equal(await page.locator("#connect-tool").inputValue(), "cc-switch");
    assert.equal(
      await page.locator("#connect-target").inputValue(),
      "claude-code",
    );
    assert.equal(
      await page.locator("#connect-model").inputValue(),
      "claude-sonnet-4-20250514",
    );
  });
  await check(
    "matching simulation persists after refresh and later failure stays visible",
    async () => {
      await page
        .getByRole("link", { name: "可选：验证配置", exact: true })
        .click();
      await page
        .getByRole("button", { name: "发送模拟请求", exact: true })
        .click();
      await page.getByText("模拟验证通过 · 已计费", { exact: true }).waitFor();
      await page
        .getByRole("link", { name: "返回接入中心", exact: true })
        .click();
      await page.getByText("已有对应的模拟成功请求", { exact: true }).waitFor();
      await page.reload();
      await page.getByText("已有对应的模拟成功请求", { exact: true }).waitFor();
      await page
        .getByRole("link", { name: "可选：验证配置", exact: true })
        .click();
      await page.locator(".verify-advanced summary").click();
      await page.locator("#verify-max").fill("0");
      await page
        .getByRole("button", { name: "发送模拟请求", exact: true })
        .click();
      await page.getByText("请求被拒绝 · 未计费", { exact: true }).waitFor();
      await page
        .getByRole("link", { name: "返回接入中心", exact: true })
        .click();
      await page
        .getByText(
          "最近一次模拟验证失败且未计费；请进入明细查看原因，配置选择已保留。",
          { exact: true },
        )
        .waitFor();
      assert(
        !(await page.locator(".access-status").innerText()).includes(
          "已有对应的模拟成功请求",
        ),
      );
    },
  );
  await page.screenshot({
    path: output + "/desktop-connect.png",
    fullPage: true,
  });
  await check(
    "Codex has honest unsupported state, no pretend config",
    async () => {
      await page.locator("#connect-target").selectOption("codex");
      await page
        .getByRole("heading", { name: "当前没有已确认适配的模型" })
        .waitFor();
      assert.equal(await page.locator(".access-code").count(), 0);
      assert.equal(
        await page
          .getByRole("button", { name: "复制配置", exact: true })
          .count(),
        0,
      );
    },
  );
  await check("nonexistent detail cannot show a different model", async () => {
    await page.goto(base + "/models/nonexistent-model");
    await page
      .getByRole("heading", { name: "模型不存在", exact: true })
      .waitFor();
    assert.equal(
      await page.getByRole("heading", { name: "GPT-4.1", exact: true }).count(),
      0,
    );
  });
  await check("model detail reuses compatible key", async () => {
    await page.goto(base + "/models/gpt-4.1");
    const href = await page
      .getByRole("link", { name: "复用密钥并接入", exact: true })
      .getAttribute("href");
    assert(href.includes(`key=${keyId}`) && href.includes("model=gpt-4.1"));
  });
  await check(
    "mobile connection and catalog have no page overflow",
    async () => {
      await page.setViewportSize({ width: 390, height: 844 });
      await page.goto(
        base + `/console/connect?key=${keyId}&model=claude-sonnet-4-20250514`,
      );
      await page.locator("#connect-tool").waitFor();
      await page.locator("#connect-tool").selectOption("claude-code");
      const connect = await page.evaluate(() => ({
        scroll: document.documentElement.scrollWidth,
        width: innerWidth,
      }));
      assert(connect.scroll <= connect.width + 1, JSON.stringify(connect));
      await page.screenshot({
        path: output + "/mobile-connect.png",
        fullPage: true,
      });
      await page.goto(base + "/models");
      await page.getByRole("heading", { name: "找到适合你的模型" }).waitFor();
      const catalog = await page.evaluate(() => ({
        scroll: document.documentElement.scrollWidth,
        width: innerWidth,
      }));
      assert(catalog.scroll <= catalog.width + 1, JSON.stringify(catalog));
      await page.screenshot({
        path: output + "/mobile-models.png",
        fullPage: true,
      });
    },
  );
  await check("Arabic dark detail remains usable at narrow width", async () => {
    await page.evaluate(() => {
      localStorage.setItem("xy-language", "ar");
      localStorage.setItem("xy-theme", "dark");
    });
    await page.goto(base + "/models/veyra-auto");
    await page.locator(".access-detail-heading").waitFor();
    const size = await page.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      width: innerWidth,
      direction: getComputedStyle(document.documentElement).direction,
    }));
    assert(size.scroll <= size.width + 1, JSON.stringify(size));
    assert.equal(size.direction, "rtl");
    await page.screenshot({
      path: output + "/mobile-arabic-dark-detail.png",
      fullPage: true,
    });
  });
} finally {
  await writeFile(
    output + "/report.json",
    JSON.stringify({ passed, failures }, null, 2),
  );
  await browser.close();
}
if (failures.length) process.exitCode = 1;
