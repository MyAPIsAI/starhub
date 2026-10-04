import { chromium } from "playwright";
import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";
import { mkdir, writeFile } from "node:fs/promises";
const url = process.env.VEYRA_URL || "http://127.0.0.1:5196";
const out = new URL("../docs/verification-2026-09-26/", import.meta.url);
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: "chrome", headless: true });
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  permissions: ["clipboard-read", "clipboard-write"],
});
const page = await context.newPage();
const report = {
  checks: [],
  errors: [],
  screenshots: [],
  startedAt: new Date().toISOString(),
};
page.on("pageerror", (e) => report.errors.push(String(e)));
async function check(name, fn) {
  try {
    await fn();
    report.checks.push({ name, passed: true });
    console.log("PASS", name);
  } catch (e) {
    report.checks.push({ name, passed: false, error: String(e) });
    console.log("FAIL", name, e.message);
  }
}
const state = () =>
  page.evaluate(async () => {
    const useProductPreview = window.__veyraStore;
    return useProductPreview.getState();
  });
async function snap(name) {
  await page.screenshot({
    path: fileURLToPath(new URL(name + ".png", out)),
    fullPage: true,
  });
  report.screenshots.push(name + ".png");
}
async function reset(scenario) {
  await page.evaluate(async (value) => {
    const useProductPreview = window.__veyraStore;
    useProductPreview.getState().resetScenario(value);
  }, scenario);
}
async function bind() {
  await page.evaluate(async () => {
    const loaded = performance
      .getEntriesByType("resource")
      .filter((e) => e.name.includes("/src/lib/product-preview.ts"))
      .at(-1);
    window.__veyraStore = (await import(loaded.name)).useProductPreview;
  });
}
async function go(path) {
  await page.goto(url + path);
  await page.locator(".xy-main").waitFor();
  await bind();
}
async function send() {
  const count = (await state()).requests.length;
  await page.getByRole("button", { name: "发送模拟请求", exact: true }).click();
  await page.waitForFunction(
    (n) => window.__veyraStore.getState().requests.length > n,
    count,
  );
}
async function call(model) {
  await page.locator("#verify-model").selectOption(model);
  await send();
}
await go("/console");
await check("A 同一密钥 A、B、智能模型连续调用与账本一致", async () => {
  await reset("enterprise");
  const before = await state();
  await go("/console/playground?key=" + before.keys[0].id);
  await page
    .locator("#verify-prompt")
    .fill("包含 \"双引号\"、'单引号' 和\n换行的测试输入");
  for (const model of ["gpt-4.1", "deepseek-chat", "veyra-auto"])
    await call(model);
  const after = await state();
  assert.equal(after.requests.length, 3);
  assert.ok(
    after.requests.every(
      (r) => r.key_id === before.keys[0].id && r.status === "success",
    ),
  );
  assert.equal(new Set(after.requests.map((r) => r.trace_id)).size, 3);
  assert.equal(after.requests[0].requested_model, "veyra-auto");
  assert.ok(after.requests[0].actual_model);
  assert.equal(
    BigInt(before.balance) - BigInt(after.balance),
    after.requests.reduce((n, r) => n + BigInt(r.cost_nano), 0n),
  );
  assert.equal(after.ledger.filter((l) => l.trace_id).length, 3);
  await snap("desktop-verification");
});
await check("D 请求导出处理引号与换行且参数一致", async () => {
  const code = await page.locator(".verify-code pre").innerText();
  assert.ok(code.includes("veyra-auto"));
  assert.ok(code.includes("\\n"));
  assert.ok(code.includes("max_tokens"));
  assert.ok(!code.includes("••"));
  await page.getByRole("button", { name: "复制命令", exact: true }).click();
  assert.equal(await page.evaluate(() => navigator.clipboard.readText()), code);
  const after = await state();
  assert.equal(after.requests.length, 3);
});
await check("C 受限密钥智能路由保持白名单与能力边界", async () => {
  const s = await state();
  await go("/console/playground?key=" + s.keys[1].id + "&model=veyra-auto");
  await call("veyra-auto");
  let current = await state();
  assert.ok(
    ["deepseek-chat", "qwen-plus"].includes(current.requests[0].actual_model),
  );
  await page.locator(".verify-advanced summary").click();
  await page.locator("#verify-custom-model").fill("gpt-4.1");
  await send();
  current = await state();
  assert.equal(current.requests[0].error_code, "model_unauthorized");
  assert.equal(current.requests[0].cost_nano, "0");
});
await check("F 额度失败保留输入，编辑额度后继续", async () => {
  const s = await state();
  const key = s.keys[1];
  await page.evaluate(async (id) => {
    const useProductPreview = window.__veyraStore;
    useProductPreview.getState().updateKey(id, { quotaLimit_usd: 0 });
  }, key.id);
  await page.locator("#verify-custom-model").fill("veyra-auto");
  const input = await page.locator("#verify-prompt").inputValue();
  await send();
  assert.equal((await state()).requests[0].error_code, "key_quota_exceeded");
  await page.getByRole("link", { name: "检查密钥与额度", exact: true }).click();
  const card = page.locator(".xy-key-card").filter({ hasText: key.name });
  const edit = (await card.count())
    ? card.getByRole("button", { name: "编辑", exact: true })
    : page.getByRole("button", { name: "编辑", exact: true }).nth(1);
  await edit.click();
  await page.locator("#edit-key-quota").fill("10");
  await page.getByRole("button", { name: "保存修改", exact: true }).click();
  await page.goBack();
  await page.locator("#verify-prompt").waitFor();
  assert.equal(await page.locator("#verify-prompt").inputValue(), input);
  await call("veyra-auto");
  assert.equal((await state()).requests[0].status, "success");
});
await check("B 模型详情创建保留分组全模型授权且刷新保留密钥", async () => {
  await go("/console/keys?create=1&model=gpt-4.1");
  await page.locator("#new-key-name").fill("从模型详情接入");
  assert.equal(
    await page.locator("#new-key-form details").getAttribute("open"),
    null,
  );
  await page.locator("#new-key-form").evaluate((form) => form.requestSubmit());
  await page.getByRole("heading", { name: "密钥已创建" }).waitFor();
  const s = await state();
  const created = s.keys.find((k) => k.name === "从模型详情接入");
  assert.equal(created.modelAccess, "all");
  assert.equal(created.smartAccess, true);
  assert.equal(created.allowedModels, undefined);
  await page.getByRole("button", { name: "接入工具", exact: true }).click();
  await page.locator("#connect-key").waitFor();
  assert.equal(await page.locator("#connect-key").inputValue(), created.id);
  await page.reload();
  await page.locator("#connect-key").waitFor();
  await bind();
  assert.ok((await state()).keys.some((k) => k.id === created.id));
});
await check("D 接入配置复制不生成请求，工具过滤与协议不兼容状态", async () => {
  await page.locator("#connect-tool").selectOption("cc-switch");
  await page.locator("#connect-target").selectOption("claude-code");
  await page.locator("#connect-model").selectOption("claude-sonnet-4-20250514");
  assert.equal(
    await page.locator('#connect-model option[value="gpt-4.1"]').count(),
    0,
  );
  const count = (await state()).requests.length;
  await page.getByRole("button", { name: "复制配置", exact: true }).click();
  assert.equal((await state()).requests.length, count);
  assert.ok(
    (await page.locator(".access-status").innerText()).includes(
      "等待对应请求验证",
    ),
  );
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  assert.ok(copied.includes("ANTHROPIC_DEFAULT_SONNET_MODEL"));
  assert.ok(!copied.includes("••"));
  await page.locator("#connect-target").selectOption("codex");
  assert.ok(
    (await page.locator(".access-empty").innerText()).includes("Responses"),
  );
  await page.locator("#connect-target").selectOption("openai");
  await page.locator("#connect-model").selectOption("veyra-auto");
  await snap("desktop-connect");
});
await check("F 上游失败、余额失败明确未计费并能恢复", async () => {
  await reset("enterprise");
  const s = await state();
  await go("/console/playground?key=" + s.keys[0].id + "&model=gpt-4.1");
  await page.evaluate(async () => {
    const useProductPreview = window.__veyraStore;
    useProductPreview.getState().demoSetUpstreamOutage(true);
  });
  await call("gpt-4.1");
  assert.equal((await state()).requests[0].error_code, "upstream_unavailable");
  await page.getByRole("button", { name: "恢复模拟上游", exact: true }).click();
  await call("gpt-4.1");
  assert.equal((await state()).requests[0].status, "success");
});
await check("E 记录可进入对应资金流水并返回调用", async () => {
  await page.getByRole("button", { name: "查看本次明细", exact: true }).click();
  const s = await state();
  const ledger = s.ledger.find((l) => l.trace_id === s.requests[0].trace_id);
  await page.getByRole("link", { name: ledger.id, exact: true }).click();
  await page.getByRole("link", { name: "查看关联调用", exact: true }).click();
  await page.getByRole("heading", { name: "调用明细", exact: true }).waitFor();
  assert.ok(
    (await page.getByRole("dialog").innerText()).includes(
      s.requests[0].trace_id,
    ),
  );
  await page.keyboard.press("Escape");
});
await check("G 首次接入从零余额到充值、调用与刷新连续操作", async () => {
  await reset('first'); await go('/console');
  assert.equal((await state()).balance,'0'); assert.equal((await state()).keys.length,0);
  assert.ok((await page.locator('.xy-metrics').innerText()).includes('—'));
  await go('/console/keys?create=1'); await page.locator('#new-key-name').fill('首次接入验证');
  await page.locator('#new-key-form').evaluate(form=>form.requestSubmit());
  await page.getByRole('heading',{name:'密钥已创建'}).waitFor();
  const key=(await state()).keys[0];
  await page.getByRole('button',{name:'验证配置（可选）',exact:true}).click();
  await page.locator('#verify-model').waitFor(); await call('gpt-4.1');
  assert.equal((await state()).requests[0].error_code,'insufficient_balance');
  await page.getByRole('link',{name:'补充演示余额',exact:true}).click();
  await page.getByLabel('或输入其他金额',{exact:true}).fill('10');
  await page.getByRole('button',{name:'确认金额并创建订单',exact:true}).click();
  assert.equal((await state()).balance,'0');
  await page.reload(); await page.locator('.xy-main').waitFor(); await bind();
  await page.getByRole('button',{name:/进入演示收银台|继续演示支付/}).click();
  await page.getByRole('button',{name:'模拟支付成功',exact:true}).click();
  await page.getByRole('heading',{name:'充值成功',exact:true}).waitFor();
  const paid=await state(); assert.equal(paid.balance,'10000000000');assert.equal(paid.rechargeOrders.length,1);
  await go('/console/playground?key='+key.id+'&model=gpt-4.1');await call('gpt-4.1');
  const after=await state();assert.equal(after.requests[0].status,'success');
  assert.equal(10000000000n-BigInt(after.balance),BigInt(after.requests[0].cost_nano));
  await page.reload();await page.locator('.xy-main').waitFor();await bind();
  assert.equal((await state()).balance,after.balance);assert.equal((await state()).ledger.length,2);
});
await check("H 创建取消、刷新保留草稿，重置清除草稿", async()=>{
 await go('/console/keys?create=1');await page.locator('#new-key-name').fill('未提交草稿');
 await page.getByRole('button',{name:'取消，保留草稿',exact:true}).click();assert.ok(!(await state()).keys.some(k=>k.name==='未提交草稿'));
 await go('/console/keys?create=1');assert.equal(await page.locator('#new-key-name').inputValue(),'未提交草稿');
 await page.reload();await page.locator('#new-key-name').waitFor();await bind();assert.equal(await page.locator('#new-key-name').inputValue(),'未提交草稿');
 await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(await page.getByRole('dialog').count(),0);
 await reset('enterprise');await go('/console/keys?create=1');assert.equal(await page.locator('#new-key-name').inputValue(),'');await page.keyboard.press('Escape');
});
await check("H 窄屏、深色与三种语言无页面横向溢出", async () => {
  await reset("enterprise");
  for (const [lang, theme] of [
    ["zh", "light"],
    ["en", "dark"],
    ["ar", "dark"],
  ]) {
    await page.evaluate(
      async ({ lang, theme }) => {
        const mod = performance
          .getEntriesByType("resource")
          .filter((e) => e.name.includes("/src/lib/preferences.ts"))
          .at(-1);
        const { usePreferences } = await import(mod.name);
        usePreferences.getState().setLanguage(lang);
        usePreferences.getState().setTheme(theme);
      },
      { lang, theme },
    );
    await page.setViewportSize({ width: 390, height: 844 });
    for (const path of [
      "/console/connect",
      "/console/playground",
      "/console/keys",
      "/console/requests",
    ]) {
      await go(path);
      await page.waitForTimeout(100);
      const size = await page.evaluate(() => [
        document.documentElement.scrollWidth,
        innerWidth,
      ]);
      assert.ok(size[0] <= size[1] + 1, `${lang} ${path}: ${size}`);
    }
    await snap("mobile-" + lang + "-" + theme);
  }
});
await writeFile(
  new URL("browser-results.json", out),
  JSON.stringify(report, null, 2),
);
await browser.close();
if (report.errors.length || report.checks.some((c) => !c.passed))
  process.exitCode = 1;
