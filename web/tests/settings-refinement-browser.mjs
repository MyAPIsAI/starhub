import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = 'docs/verification-2026-09-26-round3';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage();
const report = { checks: [], errors: [], screenshots: [], startedAt: new Date().toISOString() };
page.on('pageerror', error => report.errors.push(String(error)));
page.on('dialog', dialog => dialog.accept());
const check = async (name, fn) => { try { await fn(); report.checks.push({ name, passed: true }); console.log('PASS', name); } catch (error) { report.checks.push({ name, passed: false, error: String(error) }); console.log('FAIL', name, error.message); await page.screenshot({path: `${out}/settings-failure-${report.checks.length}.png`, fullPage: true}); await writeFile(`${out}/settings-failure-${report.checks.length}.html`, await page.content()); } };
async function bind() {
  await page.evaluate(async () => {
    const load = async (file) => import(performance.getEntriesByType('resource').filter(row => row.name.includes('/src/lib/' + file + '.ts')).at(-1).name);
    window.__s = (await load('product-preview')).useProductPreview;
    window.__p = (await load('model-pricing')).useModelPricing;
    window.__a = (await load('system-access')).useSystemAccess;
    window.__prefs = (await load('preferences')).usePreferences;
  });
}
async function go(path) { await page.goto(base + path); await page.locator('.xy-main').waitFor(); await bind(); }
const state = () => page.evaluate(() => window.__s.getState());
const snap = async name => { await page.screenshot({ path: `${out}/${name}.png`, fullPage: true }); report.screenshots.push(name + '.png'); };
async function publish() { await page.getByRole('button', { name: '检查并发布', exact: true }).click(); await page.getByRole('dialog').waitFor(); await page.getByRole('button', { name: '确认发布', exact: true }).click(); await page.getByRole('dialog').waitFor({ state: 'hidden' }); }
async function submit() { const n = (await state()).requests.length; await page.getByRole('button', { name: '提交模拟用量并计费', exact: true }).click(); await page.waitForFunction(count => window.__s.getState().requests.length > count, n); return (await state()).requests[0]; }
await go('/console');
await page.evaluate(() => { window.__a.getState().setRole('superAdmin'); window.__s.getState().resetScenario('pricing'); });
await go('/console/model-pricing');
await check('系统设置在工作台；普通用户隐藏入口且直接地址受限；旧后台地址重定向', async () => {
  assert.equal(await page.locator('.xy-sidebar-nav').getByRole('link', { name: '标准模型', exact: true }).count(), 1);
  assert.equal(await page.locator('.xy-sidebar-nav').getByRole('link', { name: '服务分组', exact: true }).count(), 1);
  assert.equal(await page.getByRole('heading', { name: '标准模型', exact: true }).count(), 1);
  assert.equal(await page.getByLabel('分组倍率', { exact: true }).count(), 0);
  await page.getByRole('button', { name: '演示场景与重置' }).click();
  await page.getByRole('button', { name: '普通用户', exact: true }).click();
  await page.getByRole('button', { name: '演示场景与重置' }).click();
  await page.getByRole('heading', { name: '需要系统管理员权限', exact: true }).waitFor();
  assert.equal(await page.locator('.xy-sidebar-nav').getByRole('link', { name: '标准模型', exact: true }).count(), 0);
  await go('/console/service-groups');
  await page.getByRole('heading', { name: '需要系统管理员权限', exact: true }).waitFor();
  await page.evaluate(() => window.__a.getState().setRole('superAdmin'));
  await go('/admin?tab=models');
  await page.getByRole('heading', { name: '标准模型', exact: true }).waitFor();
  assert.equal(new URL(page.url()).pathname, '/console/model-pricing');
});
await check('模型基准改价有影响预览，取消保留草稿，发布后新价结算且历史快照不变', async () => {
  await go('/console/model-pricing?model=gpt-4.1');
  const before = await state();
  const first = await submit(); assert.equal(first.status, 'success'); assert.equal(first.cost_nano, '4800000');
  await page.getByLabel('GPT-4.1 普通输入', { exact: true }).fill('4');
  await page.getByRole('button', { name: '检查并发布', exact: true }).click();
  assert.ok((await page.getByRole('dialog').innerText()).includes('受影响'));
  await snap('settings-pricing-impact');
  await page.getByRole('button', { name: '继续编辑', exact: true }).click();
  assert.equal(await page.getByLabel('GPT-4.1 普通输入', { exact: true }).inputValue(), '4');
  await page.reload(); await page.locator('.settings-pricing').waitFor(); await bind();
  assert.equal(await page.getByLabel('GPT-4.1 普通输入', { exact: true }).inputValue(), '4');
  await publish();
  const second = await submit(); assert.equal(second.cost_nano, '6400000');
  const after = await state();
  const historical = after.requests.find(row => row.trace_id === first.trace_id); assert.equal(historical.cost_nano, first.cost_nano); assert.equal(historical.time, first.time); assert.deepEqual(historical.pricing_snapshot, JSON.parse(JSON.stringify(first.pricing_snapshot))); 
  assert.equal(BigInt(before.balance) - BigInt(after.balance), 11200000n);
  assert.equal(after.ledger.filter(row => [first.trace_id, second.trace_id].includes(row.trace_id)).length, 2);
  await snap('settings-pricing-desktop');
});
await check('分组模块独立管理名称、倍率与可用状态，并实际限制后续调用', async () => {
  async function publishGroup(){ await page.getByRole('button',{name:'预览影响',exact:true}).click(); await page.getByRole('button',{name:'确认发布分组',exact:true}).click(); await page.getByRole('dialog').waitFor({state:'hidden'}); }
  await go('/console/service-groups');
  await page.getByRole('row').filter({has:page.locator('code').filter({hasText:/^pro$/})}).getByRole('button',{name:'编辑',exact:true}).click();
  await page.getByLabel('分组名称',{exact:true}).fill('生产服务组');
  await page.getByLabel('分组倍率',{exact:true}).fill('0.5'); await publishGroup();
  await go('/console/model-pricing?model=gpt-4.1');
  const third=await submit(); assert.equal(third.cost_nano,'4000000'); assert.equal(third.pricing_snapshot.groupMultiplier,'0.5');
  await go('/console/service-groups');
  await page.getByRole('row').filter({hasText:'生产服务组'}).getByRole('button',{name:'编辑',exact:true}).click();
  await page.getByLabel('启用分组',{exact:true}).uncheck(); await publishGroup();
  const failed=await page.evaluate(()=>{const s=window.__s.getState();return s.recordMeteredCall(s.keys[0].id,'gpt-4.1',{input_tokens:1000,output_tokens:500});});
  assert.equal(failed.error_code,'group_unauthorized'); assert.equal(failed.cost_nano,'0');
  await page.getByRole('row').filter({hasText:'生产服务组'}).getByRole('button',{name:'编辑',exact:true}).click();
  await page.getByLabel('启用分组',{exact:true}).check(); await publishGroup(); await snap('settings-groups-desktop');
});
await check('图片按张、视频按秒与取整、表达式及分时规则均产生真实演示账目', async () => {
  await go('/console/model-pricing?model=veyra-demo-image');
  await page.getByLabel('图片张数', { exact: true }).fill('3');
  const image = await submit(); assert.equal(image.cost_nano, '60000000');
  assert.equal(image.pricing_snapshot.rule.unit, 'image');
  await go('/console/model-pricing?model=veyra-demo-video');
  await page.getByLabel('时长（秒）', { exact: true }).fill('2.1');
  const video = await submit(); assert.equal(video.cost_nano, '120000000'); assert.equal(video.pricing_snapshot.billedQuantity, '3');
  await page.getByRole('button', { name: '表达式 · 高级', exact: true }).click();
  await page.getByLabel('计费表达式（结果单位：美元）', { exact: true }).fill('ceil(seconds) * 0.1 + requests * 0.02');
  await page.getByRole('button', { name: '添加时段', exact: true }).click();
  await page.getByLabel('时区', { exact: true }).selectOption('UTC');
  await page.getByLabel('计价模拟时间（本地时区）', { exact: true }).fill('2026-09-25T17:30');
  await publish();
  // Use a known instant for the rule while preserving actual billing/quota time.
  const instant = '2026-09-26T01:30:00Z';
  const local = await page.evaluate(at => { const d = new Date(at); return new Date(d.getTime() - d.getTimezoneOffset()*60000).toISOString().slice(0,16); }, instant);
  await page.getByLabel('计价模拟时间（本地时区）', { exact: true }).fill(local);
  const expression = await submit(); assert.equal(expression.cost_nano, '80000000'); assert.equal(expression.pricing_snapshot.timeMultiplier, '0.5');
  assert.equal(expression.pricing_snapshot.rule.mode, 'expression');
  await page.getByRole('link', { name: '查看关联调用明细', exact: true }).click();
  await page.getByRole('dialog').waitFor(); assert.ok((await page.getByRole('dialog').innerText()).includes('结算系数'));
  await snap('settings-metered-detail');
  await page.getByRole('button', { name: '复用请求验证', exact: true }).click();
  await page.getByLabel('时长（秒）', { exact: true }).waitFor();
  assert.equal(await page.getByLabel('时长（秒）', { exact: true }).inputValue(), '2.1');
});
await check('余额不足明确未计费，输入保留，恢复余额后可直接重试', async () => {
  const before = await state();
  await page.evaluate(() => window.__s.getState().demoSetBalanceState('zero'));
  const failed = await submit(); assert.equal(failed.error_code, 'insufficient_balance'); assert.equal(failed.cost_nano, '0');
  assert.equal(await page.getByLabel('时长（秒）', { exact: true }).inputValue(), '2.1');
  assert.equal((await state()).ledger.length, before.ledger.length + 1); // balance fixture adjustment only
  await page.evaluate(() => window.__s.getState().demoSetBalanceState('normal'));
  const success = await submit(); assert.equal(success.status, 'success');
});
await check('手机深色中文、英文、阿拉伯语页面可操作且无横向溢出', async () => {
  await page.setViewportSize({ width: 390, height: 844 });
  for (const language of ['zh', 'en', 'ar']) {
    await page.evaluate(lang => { window.__prefs.getState().setLanguage(lang); window.__prefs.getState().setTheme('dark'); }, language);
    for (const path of ['/console/model-pricing?model=gpt-4.1', '/console/service-groups', '/console/channels']) {
      await go(path);
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1));
      await snap(`settings-${path.split('/').at(-1)}-${language}-mobile`);
    }
  }
});
report.finishedAt = new Date().toISOString();
await writeFile(`${out}/settings-results.json`, JSON.stringify(report, null, 2));
await browser.close();
if (report.errors.length || report.checks.some(item => !item.passed)) process.exitCode = 1;
