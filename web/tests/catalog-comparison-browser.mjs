import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-28-simplification';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || 'catalog-comparison';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = {
  base, startedAt: new Date().toISOString(), independentBrowserContext: true,
  businessStateInjection: false,
  setup: 'Presentation preferences only are set in storage. Group creation, placement, base-price and expression changes use rendered administrative controls. Clipboard rejection and delayed completion override only the browser clipboard method in isolated pages; successful copies read the real clipboard.',
  checks: [], scans: [], errors: [], screenshots: [], layout: [],
};
let page, context;
async function newPage({ width = 1440, height = 1000, language = 'zh', theme = 'light' } = {}) {
  if (context) await context.close();
  context = await browser.newContext({ viewport: { width, height }, reducedMotion: 'reduce', permissions: ['clipboard-read', 'clipboard-write'] });
  page = await context.newPage();
  page.setDefaultTimeout(12000);
  page.on('pageerror', error => report.errors.push(String(error)));
  await page.addInitScript(({ language, theme }) => {
    localStorage.setItem('xy-language', language);
    localStorage.setItem('xy-theme', theme);
  }, { language, theme });
}
const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
async function go(route = '/models?view=cards') {
  await page.goto(`${base}${route}`);
  await page.locator('main h1').first().waitFor();
  if (new URL(page.url()).pathname === '/models') await page.locator('[data-model-id]').first().waitFor();
  await settle();
}
const models = () => page.locator('[data-model-id]');
const model = id => page.locator(`[data-model-id="${id}"]`);
const publicPrice = id => model(id).locator('.catalog-model-price');
const officialPrice = id => model(id).locator('[data-price-kind="official"]');
const officialText = async id => (await model(id).locator('[data-price-kind="official"], .price-official-meta').allTextContents()).join(' ');
const detailPrice = () => page.locator('.price-comparison');
const normalize = text => text.replace(/\s+/g, ' ').trim();
const ids = () => models().evaluateAll(nodes => nodes.map(node => node.getAttribute('data-model-id')));
const stored = () => page.evaluate(() => JSON.parse(localStorage.getItem('veyra-billing-demo-v1')));
const config = () => page.evaluate(() => JSON.parse(localStorage.getItem('veyra-platform-pricing-v1')));
const moneyState = state => state && Object.fromEntries(['balance', 'ledger', 'liveRequests', 'rechargeOrders', 'keys'].map(key => [key, state[key]]));
const prices = () => models().evaluateAll(nodes => nodes.map(node => ({ id: node.getAttribute('data-model-id'), price: node.querySelector('.catalog-model-price')?.textContent.replace(/\s+/g, ' ').trim() })).sort((a, b) => a.id.localeCompare(b.id)));
async function switchView(view) {
  await page.locator(`[data-catalog-view="${view}"]`).click();
  await page.waitForFunction(value => document.querySelector(`[data-catalog-view="${value}"]`)?.getAttribute('aria-pressed') === 'true', view);
  await settle();
}
async function capture(name) {
  const file = `${prefix}-${name}.png`;
  await page.screenshot({ path: `${out}/${file}`, fullPage: false });
  report.screenshots.push(file);
}
async function check(name, fn) {
  try { await fn(); report.checks.push({ name, passed: true }); console.log('PASS', name); }
  catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.log('FAIL', name, error.stack);
    await capture(`failure-${report.checks.length}`).catch(() => {});
    await page.keyboard.press('Escape').catch(() => {});
  }
}
async function noOverflow() {
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), '整页不得横向溢出');
  for (const selector of ['.catalog-search', '.catalog-toolbar', '[data-model-id]', '[data-copy-model-id]']) {
    for (const item of await page.locator(selector).all()) {
      const box = await item.boundingBox();
      assert.ok(box && box.x >= -1 && box.x + box.width <= page.viewportSize().width + 1, `${selector} 不得超出视口`);
    }
  }
}
async function scan(name) {
  const result = await new AxeBuilder({ page }).include('main').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
  const violations = result.violations.map(v => ({ id: v.id, impact: v.impact, help: v.help, nodes: v.nodes.map(n => ({ target: n.target, failureSummary: n.failureSummary })) }));
  report.scans.push({ name, violations });
  assert.equal(violations.length, 0, JSON.stringify(violations));
}
async function publishPrice() {
  await page.getByRole('button', { name: '检查并发布', exact: true }).click();
  await page.getByRole('button', { name: '确认发布', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  await settle();
}
async function createGroup({ id, name, multiplier, custom = false, enabled = true }) {
  await go('/console/service-groups');
  await page.getByRole('button', { name: '创建分组', exact: true }).click();
  await page.getByLabel('分组名称', { exact: true }).fill(name);
  await page.getByLabel('分组标识', { exact: true }).fill(id);
  await page.getByLabel('分组倍率', { exact: true }).fill(multiplier);
  if (custom) {
    await page.getByLabel('分组类型', { exact: true }).selectOption('custom');
    await page.getByRole('dialog').locator('label').filter({ hasText: 'account-demo' }).getByRole('checkbox').check();
  }
  if (!enabled) await page.getByLabel('启用分组', { exact: true }).uncheck();
  await page.getByRole('button', { name: '预览影响', exact: true }).click();
  await page.getByRole('button', { name: '确认发布分组', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
}
async function addPlacement(groupName) {
  await go('/console/channels?view=channel&channel=channel-1');
  await page.locator('.channel-resource-table:visible tr[data-resource-id="resource-1-a"]').getByRole('button', { name: '映射与投放', exact: true }).click();
  await page.getByRole('dialog').getByLabel(new RegExp(groupName)).check();
  await page.getByRole('button', { name: '预览投放影响', exact: true }).click();
  await page.getByRole('button', { name: '确认发布投放', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
}

try {
  await newPage();
  let initialIds, initialMoney, cardPrices, originalPublic, originalOfficial;
  await check('目录保留厂商分类，明确分开展示官方参考价与平台公开最低价', async () => {
    await go();
    initialIds = await ids(); initialMoney = moneyState(await stored()); cardPrices = await prices();
    assert.equal(initialIds.length, 8);
    assert.equal(await page.locator('main select').count(), 0);
    assert.equal(await page.locator('.catalog-tabs').getByRole('button', { name: /指定模型|智能路由/ }).count(), 0);
    for (const id of initialIds) {
      if (id !== 'veyra-auto') assert.ok(await officialPrice(id).count() >= 1, `${id} 有官方参考状态`);
      else assert.equal(await officialPrice(id).count(), 0, '路由不假设独立官价');
      assert.equal(await publicPrice(id).count(), 1, `${id} 有公开价格或不可报价状态`);
      assert.equal(await model(id).locator(`[data-copy-model-id="${id}"]`).count(), 1);
    }
    originalPublic = normalize(await publicPrice('gpt-4.1').innerText());
    originalOfficial = normalize(await officialText('gpt-4.1'));
    assert.match(originalPublic, /平台最低/);
    assert.equal(await publicPrice('gpt-4.1').getAttribute('data-public-group'), 'standard');
    assert.match(originalPublic, /\$1\.2\b/); assert.match(originalPublic, /\$4\.8\b/);
    assert.match(originalOfficial, /\$2\b/); assert.match(originalOfficial, /\$8\b/);
    assert.match(await page.locator('main').innerText(), /模拟/);
    await noOverflow(); await capture('cards-desktop'); await scan('中文桌面卡片');
  });
  await check('列表信息分层：来源日期、完整官价条件、缓存和路由估算在详情保留', async () => {
    assert.equal(await page.locator('.price-official-meta, .price-conditions, .price-public-source, .catalog-description, .access-model-protocols, .catalog-footer-note').count(), 0);
    assert.doesNotMatch(await model('gpt-4.1').innerText(), /缓存读|缓存写|核验|官方来源|官价适用条件|Standard|标准|指定模型/);
    assert.match(await model('claude-sonnet-4-20250514').innerText(), /历史/);
    assert.match(await model('deepseek-chat').innerText(), /未核实|未核验/);
    assert.match(await publicPrice('veyra-auto').innerText(), /按实际模型计费/);
    assert.doesNotMatch(await publicPrice('veyra-auto').innerText(), /1,000|1000|\$\d/);
    await go('/models/gpt-4.1');
    const links = await detailPrice().locator('.price-official-meta').getByRole('link').evaluateAll(nodes => nodes.map(n => n.href));
    assert.ok(links.some(href => /https:\/\/.*openai\.com\//.test(href)), '详情参考来源为官方域名');
    assert.match(await detailPrice().innerText(), /20\d\d[-年/]\d{1,2}[-月/]\d{1,2}/);
    assert.match(await detailPrice().innerText(), /缓存读/);
    assert.equal(await detailPrice().locator('.price-reference-scope').count() > 0, true);
    for (const id of ['veyra-auto', 'veyra-demo-image', 'veyra-demo-video']) {
      await go(`/models/${id}`);
      const text = (await detailPrice().locator('[data-price-kind="official"], .price-official-meta').allTextContents()).join(' ');
      assert.doesNotMatch(text, /\$\d/);
      assert.match(text, /无固定|无独立|不适用|样例|暂无|未核验/);
    }
    await go('/models/veyra-auto');
    assert.match(await detailPrice().innerText(), /1,000|1000/);
    await go();
  });
  await check('桌面卡片与列表在一层完成选择，信息去重且高度受控', async () => {
    for (const view of ['cards', 'list']) {
      if (view === 'list') await switchView('list');
      const dimensions = await models().evaluateAll(nodes => nodes.map(n => ({ id: n.dataset.modelId, height: n.getBoundingClientRect().height, textLength: n.innerText.length })));
      report.layout.push({ viewport: page.viewportSize(), view, models: dimensions });
      assert.ok(dimensions.every(n => n.height <= (view === 'cards' ? 500 : 240)), `${view} 高度不应被说明文字拉长：${JSON.stringify(dimensions)}`);
      assert.equal(await page.locator('.catalog-price-context').count(), 1, '统一计价提示只保留一处');
      assert.equal(await page.locator('.catalog-description, .access-model-protocols, .price-official-meta, .catalog-footer-note').count(), 0);
      assert.equal(await model('gpt-4.1').locator('[data-copy-model-id]').count(), 1);
      assert.equal(await model('gpt-4.1').getByRole('link', { name: '接入此模型', exact: true }).count(), 1);
    }
    await switchView('cards');
  });
  await check('卡片真实复制调用标识，成功反馈不夹带展示名称', async () => {
    for (const id of ['gpt-4.1', 'claude-sonnet-4-20250514', 'veyra-auto']) {
      const button = model(id).locator(`[data-copy-model-id="${id}"]`);
      await button.click();
      await page.waitForFunction(value => document.querySelector(`[data-model-id="${value}"] .model-id-copy-feedback`)?.textContent.includes('已复制'), id);
      assert.equal(await page.evaluate(() => navigator.clipboard.readText()), id);
      assert.equal(new URL(page.url()).pathname, '/models', '复制不得意外进入详情');
    }
    await capture('copy-success');
  });
  await check('剪贴板写入被拒绝时明确提示失败，不出现假成功且标识仍可手动选择', async () => {
    await page.evaluate(() => { window.__clipboardWrite = navigator.clipboard.writeText.bind(navigator.clipboard); navigator.clipboard.writeText = async () => { throw new DOMException('Clipboard permission denied', 'NotAllowedError'); }; });
    const id = 'gemini-2.5-pro';
    await model(id).locator(`[data-copy-model-id="${id}"]`).click();
    const alert = model(id).getByRole('alert');
    await alert.waitFor(); assert.match(await alert.innerText(), /复制失败/);
    assert.equal(await model(id).locator('.model-id-copy-feedback').filter({ hasText: '已复制' }).count(), 0);
    assert.match(await model(id).innerText(), /gemini-2\.5-pro/);
    await capture('copy-rejected');
    await page.evaluate(() => { navigator.clipboard.writeText = window.__clipboardWrite; });
  });
  await check('列表与卡片保留完全相同的模型、官方价格、公开价格和计价单位，并可复制', async () => {
    await switchView('list');
    assert.deepEqual((await ids()).sort(), [...initialIds].sort());
    assert.deepEqual(await prices(), cardPrices);
    const copy = model('gpt-4.1').locator('[data-copy-model-id="gpt-4.1"]');
    await copy.focus(); await page.keyboard.press('Enter');
    await page.waitForFunction(() => document.querySelector('[data-model-id="gpt-4.1"] .model-id-copy-feedback')?.textContent.includes('已复制'));
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'gpt-4.1');
    await noOverflow(); await capture('list-desktop'); await scan('中文桌面列表');
  });
  await check('广场固定媒体显示单位价格，复杂规格与路由估算下沉详情', async () => {
    for (const [id, unit] of [['veyra-demo-image', /美元 \/ 张/], ['veyra-demo-video', /美元 \/ 秒/]]) {
      assert.match(await publicPrice(id).innerText(), id === 'veyra-demo-image' ? /\$0\.016/ : /\$0\.032/);
      assert.match(await publicPrice(id).innerText(), unit);
      assert.match(await model(id).innerText(), /样例/);
      assert.equal(await model(id).getByRole('link', { name: '接入此模型', exact: true }).count(), 0);
      await go(`/models/${id}`);
      assert.match(await detailPrice().innerText(), unit);
      assert.match(await detailPrice().innerText(), /\$\d/);
      assert.match(await detailPrice().innerText(), /规格|1024|720|秒|张/);
      await go('/models?view=list');
    }
    assert.match(await model('veyra-auto').innerText(), /按实际模型计费/);
    await go('/models/veyra-auto');
    assert.match(await detailPrice().innerText(), /1,000 输入 \+ 300 输出词元/);
    await go('/models?view=list');
  });
  await check('筛选、视图刷新与详情返回保持一致，详情也能复制实际调用标识', async () => {
    await page.locator('[data-catalog-vendor="OpenAI"]').click();
    await page.locator('.catalog-search input').fill('gpt'); await settle();
    const filteredUrl = page.url();
    await page.reload(); await model('gpt-4.1').waitFor();
    assert.equal(await page.locator('.catalog-search input').inputValue(), 'gpt');
    await model('gpt-4.1').getByRole('link', { name: 'GPT-4.1', exact: true }).click();
    await page.locator('.access-detail-heading h1').waitFor();
    await page.locator('[data-copy-model-id="gpt-4.1"]').click();
    await page.waitForFunction(() => document.querySelector('.model-id-copy-feedback')?.textContent.includes('已复制'));
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'gpt-4.1');
    await page.reload(); await page.locator('.access-detail-heading h1').waitFor();
    await page.getByRole('link', { name: '返回模型目录', exact: true }).click();
    await model('gpt-4.1').waitFor(); assert.equal(page.url(), filteredUrl);
    assert.equal(await page.locator('[data-catalog-view="list"]').getAttribute('aria-pressed'), 'true');
    await model('gpt-4.1').getByRole('link', { name: 'GPT-4.1', exact: true }).click();
    await page.locator('.access-detail-heading h1').waitFor(); await page.goBack(); await model('gpt-4.1').waitFor();
    assert.equal(page.url(), filteredUrl);
    assert.deepEqual(moneyState(await stored()), initialMoney);
  });
  await check('详情快速切换模型不继承已复制状态，旧模型的延迟复制结果不误报到新模型', async () => {
    await go('/models/veyra-auto');
    await page.locator('[data-copy-model-id="veyra-auto"]').click();
    await page.waitForFunction(() => document.querySelector('.model-id-copy-feedback')?.textContent.includes('已复制'));
    await page.getByRole('link', { name: 'GPT-4.1', exact: true }).first().click();
    await page.locator('[data-copy-model-id="gpt-4.1"]').waitFor(); await settle();
    assert.equal(await page.locator('.model-id-copy-feedback').innerText(), '', '新模型不能继承旧模型的成功反馈');
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'veyra-auto');
    await go('/models/veyra-auto');
    await page.evaluate(() => {
      window.__clipboardWrite = navigator.clipboard.writeText.bind(navigator.clipboard);
      navigator.clipboard.writeText = value => new Promise((resolve, reject) => {
        window.__completePendingCopy = async () => { try { await window.__clipboardWrite(value); resolve(); } catch (error) { reject(error); } };
      });
    });
    await page.locator('[data-copy-model-id="veyra-auto"]').click();
    await page.getByRole('link', { name: 'GPT-4.1', exact: true }).first().click();
    await page.locator('[data-copy-model-id="gpt-4.1"]').waitFor();
    await page.evaluate(async () => { await window.__completePendingCopy(); navigator.clipboard.writeText = window.__clipboardWrite; });
    await settle();
    assert.equal(await page.locator('.model-id-copy-feedback').innerText(), '', '旧模型异步完成不能把新模型标成已复制');
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'veyra-auto');
  });
  await check('切换密钥不改变公开最低价；详情仍按当前密钥的真实分组计价', async () => {
    await go('/console/connect'); await page.locator('#connect-key').waitFor();
    const before = moneyState(await stored()); const keys = (await stored()).keys;
    const standardKey = keys.find(k => k.tier === 'standard'), proKey = keys.find(k => k.tier === 'pro');
    assert.ok(standardKey && proKey);
    for (const [key, input, output] of [[standardKey, '1.2', '4.8'], [proKey, '1.6', '6.4']]) {
      await go('/console/connect'); await page.locator('#connect-key').selectOption(key.id);
      await go('/models?view=list');
      assert.equal(normalize(await publicPrice('gpt-4.1').innerText()), originalPublic);
      await go('/models/gpt-4.1');
      assert.equal(await page.locator('.access-inline-label select').inputValue(), key.tier);
      const current = await page.locator('.directory-detail-single .directory-price').innerText();
      assert.ok(current.includes(`$${input} / $${output}`));
    }
    assert.deepEqual(moneyState(await stored()), before);
  });

  await newPage();
  let baselinePublic, baselineMoney;
  await check('低价定制分组有资源投放也不会进入公开最低价范围', async () => {
    await go(); baselinePublic = normalize(await publicPrice('gpt-4.1').innerText()); baselineMoney = moneyState(await stored());
    await createGroup({ id: 'qa-private-floor', name: '验收专属折扣', multiplier: '0.01', custom: true });
    await addPlacement('验收专属折扣');
    const c = await config(); assert.equal(c.groups['qa-private-floor'].visibility, 'custom'); assert.ok(c.placements['resource-1-a'].groupIds.includes('qa-private-floor'));
    await go(); assert.equal(normalize(await publicPrice('gpt-4.1').innerText()), baselinePublic);
    assert.doesNotMatch(await page.locator('main').innerText(), /验收专属折扣/);
    assert.deepEqual(moneyState(await stored()), baselineMoney);
  });
  await check('便宜通用分组没有资源投放时不作为可售最低价', async () => {
    await createGroup({ id: 'qa-empty-floor', name: '验收未投放组', multiplier: '0.02' });
    assert.equal(Object.values((await config()).placements).some(p => p.groupIds.includes('qa-empty-floor')), false);
    await go(); assert.equal(normalize(await publicPrice('gpt-4.1').innerText()), baselinePublic);
    assert.doesNotMatch(await publicPrice('gpt-4.1').innerText(), /验收未投放组/);
  });
  await check('通用分组投放后真实更新公开最低价，停用后立即撤销该报价', async () => {
    await createGroup({ id: 'qa-public-floor', name: '验收公开优惠', multiplier: '0.1' });
    await addPlacement('验收公开优惠');
    await go();
    assert.equal(await publicPrice('gpt-4.1').getAttribute('data-public-group'), 'qa-public-floor');
    assert.match(await publicPrice('gpt-4.1').innerText(), /\$0\.2\b/);
    assert.match(await publicPrice('gpt-4.1').innerText(), /\$0\.8\b/);
    await capture('public-floor-updated');
    await go('/console/service-groups');
    await page.getByRole('row').filter({ hasText: 'qa-public-floor' }).getByRole('button', { name: '编辑', exact: true }).click();
    await page.getByLabel('启用分组', { exact: true }).uncheck();
    await page.getByRole('button', { name: '预览影响', exact: true }).click();
    await page.getByRole('button', { name: '确认发布分组', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await go(); assert.equal(normalize(await publicPrice('gpt-4.1').innerText()), baselinePublic);
    assert.deepEqual(moneyState(await stored()), baselineMoney);
  });

  await newPage();
  await check('平台改价更新新报价但不改官方参考价格、已有账本、密钥授权', async () => {
    await go(); const officialBefore = normalize(await officialText('gpt-4.1'));
    await go('/console/model-pricing?model=gpt-4.1');
    const priorCount = (await stored()).liveRequests.length;
    await page.getByRole('button', { name: '提交模拟用量并计费', exact: true }).click();
    await page.locator('.settings-result').waitFor();
    await page.waitForFunction(count => JSON.parse(localStorage.getItem('veyra-billing-demo-v1')).liveRequests.length === count + 1, priorCount);
    const before = moneyState(await stored()); assert.equal(before.liveRequests[0].status, 'success', '先产生可核对的真实演示账单');
    await page.getByLabel('GPT-4.1 普通输入', { exact: true }).fill('5');
    await publishPrice();
    await go(); assert.match(await publicPrice('gpt-4.1').innerText(), /\$3\b/);
    assert.equal(normalize(await officialText('gpt-4.1')), officialBefore);
    assert.deepEqual(moneyState(await stored()), before);
    await capture('base-vs-official');
  });
  await check('表达式展示示例费用而非输入输出单价，与词元官价不冒充同口径比较', async () => {
    await go('/console/model-pricing?model=gpt-4.1');
    await page.getByRole('button', { name: '表达式 · 高级', exact: true }).click();
    await page.getByLabel('计费表达式（结果单位：美元）', { exact: true }).fill('requests * 0.1');
    await publishPrice();
    await go('/models?view=list&vendor=OpenAI');
    assert.match(await publicPrice('gpt-4.1').innerText(), /按用量|计价/);
    assert.doesNotMatch(await publicPrice('gpt-4.1').innerText(), /\$0\.060000|1 次请求/);
    await model('gpt-4.1').getByRole('link', { name: 'GPT-4.1', exact: true }).click();
    await page.locator('.access-detail-heading h1').waitFor();
    const text = await detailPrice().innerText();
    assert.match(text, /表达式|示例/); assert.match(text, /非固定单价|非固定|示例费用/); assert.match(text, /\$0\.060000/);
    assert.match(text, /1 次请求/);
    assert.doesNotMatch(await detailPrice().locator('[data-price-kind="public"]').innerText(), /输入 \/ 输出 · 美元 \/ 百万词元/);
    assert.match(text, /两侧计费方式或单位不同/);
    assert.match((await detailPrice().locator('[data-price-kind="official"]').allTextContents()).join(' '), /不同|不适用|不一致|词元|参考/);
    await capture('expression-example');
  });

  await newPage();
  await check('规格条件价不以某一规格冒充统一最低价，详情保留实际假设和金额', async () => {
    await go('/console/model-pricing?model=veyra-demo-video');
    const beforeMoney = moneyState(await stored());
    await page.getByText('按规格设置条件价（可选）', { exact: false }).click();
    await page.getByRole('button', { name: '添加条件价', exact: true }).click();
    await page.getByLabel('条件单位价 1', { exact: true }).fill('0.2');
    await page.getByLabel('条件音频 1', { exact: true }).selectOption('true');
    await publishPrice();
    await go('/models?view=list&vendor=Veyra');
    assert.match(await publicPrice('veyra-demo-video').innerText(), /按规格计价/);
    assert.doesNotMatch(await publicPrice('veyra-demo-video').innerText(), /\$\d/);
    assert.equal(await model('veyra-demo-video').getByRole('link', { name: '接入此模型', exact: true }).count(), 0);
    await model('veyra-demo-video').locator('.catalog-model-name').click();
    await page.locator('.access-detail-heading h1').waitFor();
    assert.equal(await detailPrice().getAttribute('data-public-group'), 'lite');
    assert.match(await detailPrice().innerText(), /\$0\.08/);
    assert.match(await detailPrice().innerText(), /1080p/);
    assert.match(await detailPrice().innerText(), /音频|audio/);
    assert.deepEqual(moneyState(await stored()), beforeMoney);
    await capture('specification-detail');
  });
  await newPage();
  await check('官方按次而平台按词元时各自保留金额与单位，参考价修改不改变计费规则和账本', async () => {
    await go('/console/model-pricing?model=gpt-4.1');
    const beforeMoney = moneyState(await stored()), beforeConfig = await config();
    await page.locator('[data-official-reference-editor="gpt-4.1"] summary').click();
    await page.getByLabel('官方计价单位', { exact: true }).selectOption('request');
    await page.getByLabel('官方每单位价格（美元）', { exact: true }).fill('0.77');
    await page.getByRole('textbox', { name: '适用条件（修改内容按原文展示）', exact: true }).fill('验收用的计价单位差异样例，不是实际官方报价。');
    await publishPrice();
    const afterConfig = await config();
    assert.deepEqual(afterConfig.models['gpt-4.1'].rule, beforeConfig.models['gpt-4.1'].rule);
    assert.deepEqual(afterConfig.models['gpt-4.1'].pricing, beforeConfig.models['gpt-4.1'].pricing);
    assert.deepEqual(moneyState(await stored()), beforeMoney);
    await go('/models?view=list&vendor=OpenAI');
    assert.match(await model('gpt-4.1').locator('[data-price-kind="official"]').innerText(), /\$0\.77/);
    assert.match(await model('gpt-4.1').locator('[data-price-kind="official"]').innerText(), /美元 \/ 次/);
    assert.match(await model('gpt-4.1').locator('[data-price-kind="public"]').innerText(), /\$1\.2 \/ \$4\.8/);
    assert.match(await model('gpt-4.1').locator('[data-price-kind="public"]').innerText(), /美元 \/ 百万词元/);
    await go('/models/gpt-4.1');
    const official = await detailPrice().locator('[data-price-kind="official"]').innerText();
    const platform = await detailPrice().locator('[data-price-kind="public"]').innerText();
    assert.match(official, /\$0\.77/); assert.match(official, /美元 \/ 次/);
    assert.match(platform, /\$1\.2 \/ \$4\.8/); assert.match(platform, /美元 \/ 百万词元/);
    assert.match(await detailPrice().innerText(), /两侧计费方式或单位不同/);
    await page.reload(); await page.locator('.access-detail-heading h1').waitFor();
    assert.match(await detailPrice().locator('[data-price-kind="official"]').innerText(), /\$0\.77/);
  });
  await newPage();
  await check('新增未投放标准模型显示无公开价格和未核验官价，不回退到其他模型', async () => {
    await go('/console/model-pricing');
    await page.getByRole('button', { name: '创建标准模型', exact: true }).click();
    await page.getByLabel('模型名称', { exact: true }).fill('验收价格未就绪模型');
    await page.getByLabel('调用标识', { exact: true }).fill('qa-no-public-price');
    await page.getByLabel('模型厂商', { exact: true }).fill('验收厂商');
    await page.getByLabel('输入基准价', { exact: true }).fill('0.01');
    await page.getByRole('button', { name: '保存标准模型', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await go('/models?view=list&vendor=验收厂商');
    assert.deepEqual(await ids(), ['qa-no-public-price']);
    assert.equal(await publicPrice('qa-no-public-price').getAttribute('data-public-status'), 'no-resource');
    assert.match(await publicPrice('qa-no-public-price').innerText(), /不可用|暂无|未提供/);
    assert.equal(await model('qa-no-public-price').getByRole('link', { name: '接入此模型', exact: true }).count(), 0);
    await go('/models/qa-no-public-price');
    const unavailableOfficial = (await detailPrice().locator('[data-price-kind="official"], .price-official-meta').allTextContents()).join(' ');
    assert.match(unavailableOfficial, /未核验|尚未核实|暂无|未收录/);
    assert.doesNotMatch(unavailableOfficial, /\$\d/);
  });

  for (const width of [320, 390]) {
    await newPage({ width, height: width === 320 ? 568 : 844 });
    await check(`${width}像素卡片和列表价格、复制及筛选操作没有横向溢出或相互遮挡`, async () => {
      await go(); await noOverflow();
      const cardDimensions = await models().evaluateAll(nodes => nodes.map(n => ({ id: n.dataset.modelId, height: n.getBoundingClientRect().height })));
      report.layout.push({ viewport: page.viewportSize(), view: 'cards', models: cardDimensions });
      assert.ok(cardDimensions.every(n => n.height <= 580), '窄屏卡片高度应可连续浏览');
      await capture(`cards-mobile-${width}`);
      await model('gpt-4.1').scrollIntoViewIfNeeded(); await capture(`cards-mobile-${width}-price`);
      await model('gpt-4.1').locator('[data-copy-model-id="gpt-4.1"]').click();
      assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'gpt-4.1');
      await switchView('list'); await noOverflow();
      await model('gpt-4.1').scrollIntoViewIfNeeded(); await capture(`list-mobile-${width}-price`);
      await page.locator('[data-catalog-vendor="Veyra"]').click(); await settle();
      assert.deepEqual((await ids()).sort(), ['veyra-auto', 'veyra-demo-image', 'veyra-demo-video']);
      await noOverflow();
    });
  }
  for (const [language, theme] of [['zh', 'dark'], ['en', 'light'], ['en', 'dark'], ['ar', 'light'], ['ar', 'dark']]) {
    await newPage({ width: 390, height: 844, language, theme });
    await check(`${language}/${theme}窄屏两个视图均提供官价与公开价格，方向正确且无溢出`, async () => {
      await go();
      assert.equal(await page.locator('html').getAttribute('dir'), language === 'ar' ? 'rtl' : 'ltr');
      assert.equal(await page.locator('html').getAttribute('data-theme'), theme);
      for (const view of ['cards', 'list']) {
        if (view === 'list') await switchView('list');
        await noOverflow();
        assert.equal(await publicPrice('gpt-4.1').count(), 1); assert.ok(await officialPrice('gpt-4.1').count() >= 1);
        await model('gpt-4.1').scrollIntoViewIfNeeded();
        await capture(`${view}-${language}-${theme}`);
        if (language === 'ar' && theme === 'dark') await scan(`阿拉伯文深色窄屏${view}`);
      }
    });
  }
} finally {
  report.completedAt = new Date().toISOString(); report.passed = report.checks.filter(c => c.passed).length; report.total = report.checks.length;
  await writeFile(`${out}/${prefix}-browser-report.json`, JSON.stringify(report, null, 2));
  await browser.close();
}
console.log(`${report.passed}/${report.total} checks passed; ${report.errors.length} page errors; ${report.scans.length} accessibility scans`);
if (report.errors.length || report.checks.some(c => !c.passed) || report.scans.some(scan => scan.violations.length)) process.exitCode = 1;
