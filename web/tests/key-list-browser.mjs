import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";

const base = process.env.VEYRA_URL || "http://127.0.0.1:5196";
const out =
  process.env.VEYRA_EVIDENCE_DIR || "docs/verification-2026-09-28-key-list";
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || "key-list";
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: "chrome", headless: true });
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  reducedMotion: "reduce",
  permissions: ["clipboard-read", "clipboard-write"],
});
const page = await context.newPage();
page.setDefaultTimeout(10000);
const report = {
  base,
  startedAt: new Date().toISOString(),
  independentBrowserContext: true,
  businessStateInjection: false,
  clockSimulation:
    "Advance 5 minutes to verify credential re-lock, then 25 hours to verify daily quota presentation; all account/key/payment/request changes use actual UI.",
  checks: [],
  errors: [],
  screenshots: [],
  accessibility: [],
};
page.on("pageerror", (error) => report.errors.push(String(error)));
const state = () =>
  page.evaluate(() =>
    JSON.parse(localStorage.getItem("veyra-billing-demo-v1")),
  );
const currentKey = async (id) =>
  (await state()).keys.find((key) => key.id === id);
const row = (id) => page.locator(`[data-key-id="${id}"]`).first();
const action = (id, name) => row(id).locator(`[data-key-action="${name}"]`);
const menu = () => page.getByRole("menu");
const advanced = () => page.locator(".key-task-advanced > summary");
const password = "KeyListTest94725";
let mainId,
  unlimitedId,
  initialSecret,
  keyName = "qa-key-purpose";
async function check(name, run) {
  try {
    await run();
    report.checks.push({ name, passed: true });
    console.log("PASS", name);
  } catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.log("FAIL", name, error.message);
  }
}
async function go(route = "/console/keys") {
  await page.goto(base + route);
  await page.locator("main").first().waitFor();
}
async function capture(name) {
  const file = `${prefix}-${name}.png`;
  await page.screenshot({
    path: `${out}/${file}`,
    fullPage: false,
    mask: [page.locator(".created-secret, input[type=password]")],
  });
  report.screenshots.push(file);
}
async function chooseMenu(id, name) {
  await action(id, "more").click();
  await menu().waitFor();
  await menu().locator(`[data-key-action="${name}"]`).click();
}
async function edit(id, { name, quota, period } = {}) {
  await action(id, "edit").click();
  await page.locator("#edit-key-name").waitFor();
  if (name !== undefined) await page.locator("#edit-key-name").fill(name);
  if (quota !== undefined) await page.locator("#edit-key-quota").fill(quota);
  if (period) await page.locator("#edit-key-period").selectOption(period);
  await page.getByRole("button", { name: "保存修改", exact: true }).click();
  await page.locator("#edit-key-form").waitFor({ state: "hidden" });
}
async function noOverflow() {
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
    "页面不得横向溢出",
  );
  const outside = await page.locator("[data-key-id]").evaluateAll(
    (rows) =>
      rows.filter((node) => {
        const rect = node.getBoundingClientRect();
        return rect.left < -1 || rect.right > innerWidth + 1;
      }).length,
  );
  assert.equal(outside, 0, "密钥行不得超出屏幕");
}
async function preference(lang, theme) {
  await page.evaluate(
    ({ lang, theme }) => {
      localStorage.setItem("xy-language", lang);
      localStorage.setItem("xy-theme", theme);
    },
    { lang, theme },
  );
  await page.reload();
  await page.locator("main").first().waitFor();
}
async function call() {
  await go(
    `/console/playground?key=${encodeURIComponent(mainId)}&model=gpt-4.1`,
  );
  await page.locator("#verify-model").selectOption("gpt-4.1");
  await page.locator("#verify-prompt").fill("请把接入验证拆成三个步骤。");
  await page.locator(".verify-advanced > summary").click();
  await page.locator("#verify-max").fill("100");
  const count = (await state()).liveRequests.length;
  await page.getByRole("button", { name: "发送模拟请求", exact: true }).click();
  await page.waitForFunction(
    (before) =>
      JSON.parse(localStorage.getItem("veyra-billing-demo-v1")).liveRequests
        .length > before,
    count,
  );
  const request = (await state()).liveRequests[0];
  assert.equal(
    request.status,
    "success",
    `模拟调用应成功：${request.error_code || ""}`,
  );
  return request;
}
try {
  await go("/");
  await check(
    "公共页桌面导航移除服务优势，首页介绍及首页选中保留",
    async () => {
      for (const route of ["/", "/models", "/docs", "/status", "/#why"]) {
        await go(route);
        const header = page.locator(".veyra-public-header");
        assert.equal(
          await header.locator('a[href="/#why"], a[href="#why"]').count(),
          0,
        );
        assert.equal(
          await header.getByText("服务优势", { exact: true }).count(),
          0,
        );
        if (route === "/#why") {
          await page.locator("#why").waitFor();
          assert.equal(
            await header.locator('a[href="/"][aria-current="page"]').count(),
            1,
          );
        }
      }
    },
  );
  await check(
    "注册独立测试账号并创建有限额、模型白名单、地址白名单密钥",
    async () => {
      await go("/register");
      await page.locator("#auth-username").fill("qa_key_list");
      await page.locator("#auth-register-phone").fill("13988227419");
      await page.locator("#auth-new-password").fill(password);
      await page.locator("#auth-confirm-password").fill(password);
      await page
        .getByRole("button", { name: "获取验证码", exact: true })
        .click();
      await page.locator("#auth-code").fill("829104");
      await page.locator("#auth-consent").check();
      await page.locator(".auth-human input").check();
      await page.locator('.auth-form button[type="submit"]').click();
      await page.waitForURL("**/console/connect");
      await go("/console/keys?create=1");
      await page.locator("#new-key-name").fill(keyName);
      await page.locator("#new-key-quota").fill("0.01");
      assert.equal(await page.locator("#new-key-period").inputValue(), "never");
      await advanced().click();
      await page
        .getByRole("switch", { name: "启用模型白名单", exact: true })
        .click();
      const box = page
        .locator(".key-model-list label")
        .filter({ hasText: "gpt-4.1" })
        .locator("input");
      await box.check();
      await page
        .getByRole("switch", { name: "启用 IP（网络地址）白名单", exact: true })
        .click();
      await page.locator("#new-key-ips").fill("127.0.0.1");
      await page
        .locator("#new-key-form")
        .evaluate((form) => form.requestSubmit());
      await page
        .getByRole("heading", { name: "密钥已创建", exact: true })
        .waitFor();
      initialSecret = await page.locator(".created-secret code").innerText();
      const key = (await state()).keys.find((key) => key.name === keyName);
      mainId = key.id;
      assert.equal(key.modelAccess, "custom");
      assert.deepEqual(key.allowedModels, ["gpt-4.1"]);
      assert.deepEqual(key.ipWhitelist, ["127.0.0.1"]);
      await page.getByRole("button", { name: "关闭", exact: true }).click();
      await row(mainId).waitFor();
      assert.ok(
        !(await row(mainId).innerText()).includes(initialSecret),
        "列表默认仅显示掩码",
      );
      assert.match(await action(mainId, "quota").innerText(), /0%/);
    },
  );
  await check("更多菜单支持外点、Escape、键盘与焦点返回", async () => {
    await action(mainId, "more").click();
    await menu().waitFor();
    await page.keyboard.press("Escape");
    await menu().waitFor({ state: "hidden" });
    assert.ok(
      await action(mainId, "more").evaluate(
        (node) => document.activeElement === node,
      ),
    );
    await action(mainId, "more").click();
    await page.locator("main h1").click();
    await menu().waitFor({ state: "hidden" });
    await action(mainId, "more").focus();
    await page.keyboard.press("Enter");
    await menu().waitFor();
    assert.ok(
      await menu()
        .getByRole("menuitem")
        .first()
        .evaluate((node) => document.activeElement === node),
    );
    await page.keyboard.press("ArrowDown");
    assert.ok(
      await menu()
        .getByRole("menuitem")
        .nth(1)
        .evaluate((node) => document.activeElement === node),
    );
    await page.keyboard.press("Escape");
  });
  await check(
    "详情展示完整授权与地址限制，关闭/接入/返回不扩大权限",
    async () => {
      const before = await currentKey(mainId);
      await chooseMenu(mainId, "details");
      await page.locator("[data-key-detail]").waitFor();
      const details = await page.locator("[data-key-detail]").innerText();
      assert.ok(details.includes("gpt-4.1"));
      assert.ok(details.includes("127.0.0.1"));
      await page.keyboard.press("Escape");
      await page.locator("[data-key-detail]").waitFor({ state: "hidden" });
      await action(mainId, "connect").click();
      await page.waitForURL("**/console/connect?key=*");
      assert.equal(new URL(page.url()).searchParams.get("key"), mainId);
      await page.goBack();
      await row(mainId).waitFor();
      assert.deepEqual(await currentKey(mainId), before);
    },
  );
  await check("编辑取消保留数据，保存名称与周期生效且保留授权", async () => {
    const before = await currentKey(mainId);
    await action(mainId, "edit").click();
    await page.locator("#edit-key-name").fill("cancelled-change");
    await page.getByRole("button", { name: "取消", exact: true }).click();
    assert.deepEqual(await currentKey(mainId), before);
    keyName = "qa-key-production";
    await edit(mainId, { name: keyName, period: "daily" });
    const saved = await currentKey(mainId);
    assert.equal(saved.name, keyName);
    assert.equal(saved.quotaPeriod, "daily");
    assert.deepEqual(saved.allowedModels, before.allowedModels);
    assert.deepEqual(saved.ipWhitelist, before.ipWhitelist);
    assert.equal(saved.smartAccess, before.smartAccess);
    await page.reload();
    await row(mainId).waitFor();
    assert.ok((await row(mainId).innerText()).includes(keyName));
  });
  await check("停用与启用产生真实状态，停用时接入不可操作", async () => {
    await chooseMenu(mainId, "toggle");
    assert.equal((await currentKey(mainId)).active, false);
    assert.match(await action(mainId, "status").innerText(), /停用/);
    assert.ok(await action(mainId, "connect").isDisabled());
    await chooseMenu(mainId, "toggle");
    assert.equal((await currentKey(mainId)).active, true);
    assert.ok(await action(mainId, "connect").isEnabled());
  });
  await check(
    "复制要求当前账号密码，错误密码不解锁，成功复制真实凭证且到期重新锁定",
    async () => {
      await page.clock.install();
      await action(mainId, "reveal").click();
      await page.locator("#key-auth-password").fill("wrong-password-000");
      await page.getByRole("dialog").locator('button[type="submit"]').click();
      await page
        .getByRole("dialog")
        .getByText("密码错误，请重试。", { exact: true })
        .waitFor();
      assert.ok(!(await row(mainId).innerText()).includes(initialSecret));
      await page.locator("#key-auth-password").fill(password);
      await page.getByRole("dialog").locator('button[type="submit"]').click();
      await page.locator("#key-auth-password").waitFor({ state: "hidden" });
      await action(mainId, "copy").click();
      const copied = await page.evaluate(() => navigator.clipboard.readText());
      assert.ok(copied === initialSecret, "剪贴板必须是创建时的真实测试凭证");
      assert.ok(!copied.includes("•"), "不得复制掩码");
      await page.clock.fastForward(5 * 60 * 1000 + 1000);
      assert.ok(
        !(await row(mainId).innerText()).includes(initialSecret),
        "验证到期应自动隐藏明文",
      );
      await page.evaluate(() =>
        navigator.clipboard.writeText("qa-before-copy"),
      );
      await action(mainId, "copy").click();
      await page.locator("#key-auth-password").fill(password);
      await page.getByRole("dialog").locator('button[type="submit"]').click();
      await page.locator("#key-auth-password").waitFor({ state: "hidden" });
      const directCopy = await page.evaluate(() =>
        navigator.clipboard.readText(),
      );
      assert.ok(
        directCopy === initialSecret,
        "锁定态复制应在验证后直接写入真实凭证",
      );
      assert.ok(
        !(await row(mainId).innerText()).includes(initialSecret),
        "复制动作不应展开明文",
      );
      await page.reload();
      await row(mainId).waitFor();
    },
  );
  await check(
    "真实模拟充值与调用后额度金额、百分比、余额保持一致",
    async () => {
      await go("/console/billing/recharge");
      if (!(await page.getByLabel("或输入其他金额", { exact: true }).count()))
        await page
          .getByRole("button", { name: "充值", exact: true })
          .first()
          .click();
      await page.getByLabel("或输入其他金额", { exact: true }).fill("10");
      await page
        .getByRole("button", { name: "确认金额并创建订单", exact: true })
        .click();
      await page
        .getByRole("button", { name: /进入演示收银台|继续演示支付/ })
        .click();
      await page
        .getByRole("button", { name: "模拟支付成功", exact: true })
        .click();
      await page
        .getByRole("heading", { name: "充值成功", exact: true })
        .waitFor();
      const before = await state();
      const request = await call();
      const after = await state();
      const key = after.keys.find((key) => key.id === mainId);
      assert.equal(
        BigInt(before.balance) - BigInt(after.balance),
        BigInt(request.cost_nano),
      );
      assert.equal(
        BigInt(key.currentPeriodUsed_nano),
        BigInt(request.cost_nano),
      );
      await go();
      const progress = row(mainId).getByRole("progressbar");
      assert.ok(await progress.count());
      const actual = Number(await progress.getAttribute("aria-valuenow"));
      const expected =
        (Number(key.currentPeriodUsed_nano) / (key.quotaLimit_usd * 1e9)) * 100;
      assert.ok(
        Math.abs(actual - expected) <= 0.1,
        "进度应由本周期金额除以额度计算",
      );
      assert.ok(actual > 0 && actual < 100);
      await capture("desktop-used");
    },
  );
  await check("额度耗尽与将尽有明确状态，无限额不显示百分比", async () => {
    await edit(mainId, { quota: "0" });
    assert.match(
      await action(mainId, "status").innerText(),
      /额度.*耗尽|额度已用尽|已超当前额度|额度不足/,
    );
    await edit(mainId, { quota: "0.01" });
    for (let count = 0; count < 25; count++) {
      const current = await currentKey(mainId);
      if (BigInt(current.currentPeriodUsed_nano) >= 9_000_000n) break;
      await call();
    }
    await go();
    assert.match(await action(mainId, "status").innerText(), /额度将尽/);
    assert.ok(
      Number(
        await row(mainId)
          .getByRole("progressbar")
          .getAttribute("aria-valuenow"),
      ) >= 90,
    );
    await go("/console/keys?create=1");
    await page.locator("#new-key-name").fill("qa-key-unlimited");
    await page.locator("#new-key-quota").fill("");
    await page
      .locator("#new-key-form")
      .evaluate((form) => form.requestSubmit());
    await page
      .getByRole("heading", { name: "密钥已创建", exact: true })
      .waitFor();
    unlimitedId = (await state()).keys.find(
      (key) => key.name === "qa-key-unlimited",
    ).id;
    await page.getByRole("button", { name: "关闭", exact: true }).click();
    assert.match(await action(unlimitedId, "quota").innerText(), /不限额/);
    assert.equal(await row(unlimitedId).getByRole("progressbar").count(), 0);
    assert.ok(!(await action(unlimitedId, "quota").innerText()).includes("%"));
    await capture("desktop-two-keys");
    await chooseMenu(mainId, "details");
    await page.locator("[data-key-detail]").waitFor();
    await capture("detail-desktop");
    await page.keyboard.press("Escape");
  });
  await check(
    "重置与删除保留确认，取消不变更，确认实际更新且不伤及其他密钥",
    async () => {
      const before = await currentKey(mainId);
      await chooseMenu(mainId, "regenerate");
      await page
        .getByRole("alertdialog")
        .getByRole("button", { name: "取消", exact: true })
        .click();
      assert.deepEqual(await currentKey(mainId), before);
      await chooseMenu(mainId, "regenerate");
      await page
        .getByRole("alertdialog")
        .getByRole("button", { name: "重置密钥", exact: true })
        .click();
      const after = await currentKey(mainId);
      assert.ok(
        after.credentialVersion !== before.credentialVersion,
        "确认重置应改变凭证版本",
      );
      assert.deepEqual(after.allowedModels, before.allowedModels);
      assert.equal(after.currentPeriodUsed_nano, before.currentPeriodUsed_nano);
      await chooseMenu(unlimitedId, "delete");
      await page
        .getByRole("alertdialog")
        .getByRole("button", { name: "取消", exact: true })
        .click();
      assert.ok(await currentKey(unlimitedId));
      await chooseMenu(unlimitedId, "delete");
      await page
        .getByRole("alertdialog")
        .getByRole("button", { name: "删除密钥", exact: true })
        .click();
      assert.equal(await currentKey(unlimitedId), undefined);
      assert.equal(await row(unlimitedId).count(), 0);
      assert.deepEqual(await currentKey(mainId), after);
    },
  );
  await check("日额度跨周期展示归零而累计消费保留", async () => {
    const before = await currentKey(mainId);
    assert.ok(BigInt(before.totalUsed_nano) > 0n);
    await page.clock.fastForward(25 * 60 * 60 * 1000);
    await page.reload();
    await row(mainId).waitFor();
    assert.equal(
      Number(
        await row(mainId)
          .getByRole("progressbar")
          .getAttribute("aria-valuenow"),
      ),
      0,
    );
    assert.equal(
      (await currentKey(mainId)).totalUsed_nano,
      before.totalUsed_nano,
    );
    await chooseMenu(mainId, "details");
    const detail = await page.locator("[data-key-detail]").innerText();
    assert.match(detail, /累计/);
    await page.keyboard.press("Escape");
  });
  for (const lang of ["zh", "en", "ar"]) {
    await check(
      `${lang}中英阿布局：320/390窄屏、深浅主题及菜单详情均可操作`,
      async () => {
        for (const width of [320, 390])
          for (const theme of ["light", "dark"]) {
            await page.setViewportSize({ width, height: 844 });
            await preference(lang, theme);
            await row(mainId).waitFor();
            await noOverflow();
            if (lang === "ar") {
              const used = await row(mainId).locator(".key-quota-values > strong").boundingBox();
              const limit = await row(mainId).locator(".key-quota-values > span").boundingBox();
              assert.ok(used && limit && used.x < limit.x, "阿语金额比值也必须按已用/上限从左至右排列");
            }
            await action(mainId, "more").click();
            await menu().waitFor();
            const rect = await menu().boundingBox();
            assert.ok(
              rect.x >= -1 && rect.x + rect.width <= width + 1,
              "更多菜单不能超出窄屏",
            );
            await menu().locator('[data-key-action="details"]').click();
            await page.locator("[data-key-detail]").waitFor();
            await noOverflow();
            if (width === 390 && theme === "dark")
              await capture(`detail-${lang}-dark-390`);
            await page.keyboard.press("Escape");
            if (
              (width === 320 && theme === "light") ||
              (width === 390 && theme === "dark")
            )
              await capture(`${lang}-${theme}-${width}`);
          }
      },
    );
  }
  await check(
    "公共导航手机端没有服务优势且首页锚点可访问，菜单可收起",
    async () => {
      await page.setViewportSize({ width: 390, height: 844 });
      await preference("zh", "light");
      for (const route of ["/models", "/#why"]) {
        await go(route);
        await page.locator(".veyra-public-menu").click();
        const nav = page.locator(".veyra-public-nav.is-open");
        await nav.waitFor();
        assert.equal(
          await nav.getByText("服务优势", { exact: true }).count(),
          0,
        );
        assert.equal(await nav.locator('a[href*="#why"]').count(), 0);
        if (route.includes("#why")) {
          assert.equal(
            await nav.locator('a[href="/"][aria-current="page"]').count(),
            1,
          );
          await page.locator("#why").waitFor();
        }
        await page.keyboard.press("Escape");
        assert.equal(
          await page
            .locator(".veyra-public-menu")
            .getAttribute("aria-expanded"),
          "false",
        );
      }
      await capture("public-mobile-navigation");
    },
  );
  for (const width of [1440, 390])
    await check(`${width}px密钥列表无障碍扫描`, async () => {
      await page.setViewportSize({
        width,
        height: width === 1440 ? 1000 : 844,
      });
      await go();
      await preference("zh", "light");
      const result = await new AxeBuilder({ page }).include("main").analyze();
      report.accessibility.push({
        width,
        violations: result.violations.map(({ id, impact, nodes }) => ({
          id,
          impact,
          count: nodes.length,
          nodes: nodes.map(({ target, failureSummary }) => ({
            target,
            failureSummary,
          })),
        })),
      });
      assert.equal(
        result.violations.length,
        0,
        JSON.stringify(report.accessibility.at(-1)),
      );
      if (width === 1440) await capture("desktop-final");
    });
} finally {
  report.finishedAt = new Date().toISOString();
  await writeFile(
    `${out}/${prefix}-results.json`,
    JSON.stringify(report, null, 2),
  );
  await browser.close();
}
if (report.errors.length || report.checks.some((check) => !check.passed))
  process.exitCode = 1;
