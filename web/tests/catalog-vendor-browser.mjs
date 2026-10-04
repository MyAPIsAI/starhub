import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-28-simplification';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || 'catalog-vendor';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = {
  base, startedAt: new Date().toISOString(), independentBrowserContext: true,
  businessStateInjection: false,
  preferencesOnly: 'Language and theme are presentation preferences. All filters, view changes and key selections use the rendered UI; no pricing, model or ledger state is injected.',
  checks: [], errors: [], screenshots: [],
};
let page;
async function newPage({ width = 1440, height = 1000, language = 'zh', theme = 'light' } = {}) {
  const context = await browser.newContext({ viewport: { width, height }, reducedMotion: 'reduce' });
  page = await context.newPage();
  page.setDefaultTimeout(12000);
  page.on('pageerror', error => report.errors.push(String(error)));
  await page.addInitScript(({ language, theme }) => {
    localStorage.setItem('xy-language', language);
    localStorage.setItem('xy-theme', theme);
  }, { language, theme });
}
async function go(route = '/models') {
  await page.goto(`${base}${route}`);
  await page.locator('main h1').first().waitFor();
  if (new URL(page.url()).pathname === '/models') await page.locator('.catalog-search input').waitFor();
}
const models = () => page.locator('[data-model-id]');
const model = id => page.locator(`[data-model-id="${id}"]`);
const filters = () => page.locator('.catalog-tabs');
const vendorButton = vendor => page.locator(`[data-catalog-vendor="${vendor}"]`);
const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
const chooseVendor = async vendor => {
  await vendorButton(vendor).click();
  await page.waitForFunction(value => document.querySelector(`[data-catalog-vendor="${value}"]`)?.getAttribute('aria-pressed') === 'true', vendor);
  await settle();
};
const switchView = async name => {
  await page.locator(`[data-catalog-view="${name === 'list' ? 'list' : 'cards'}"]`).click();
  await assertView(name); await settle();
};
const search = () => page.locator('.catalog-search input');
const fillSearch = async query => {
  await search().fill(query);
  await page.waitForFunction(value => (new URL(location.href).searchParams.get('q') ?? '') === value, query);
  await settle();
};
const ids = () => models().evaluateAll(nodes => nodes.map(node => node.getAttribute('data-model-id')));
const snapshot = () => page.evaluate(() => JSON.parse(localStorage.getItem('veyra-billing-demo-v1')));
const moneyState = state => state && Object.fromEntries(['balance', 'ledger', 'liveRequests', 'rechargeOrders', 'keys'].map(key => [key, state[key]]));
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
  }
}
async function noOverflow() {
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), '页面不能产生横向滚动');
  for (const selector of ['.catalog-search', '.catalog-toolbar', '[data-model-id]']) {
    for (const item of await page.locator(selector).all()) {
      const rect = await item.boundingBox();
      assert.ok(rect && rect.x >= -1 && rect.x + rect.width <= page.viewportSize().width + 1, `${selector} 应位于视口内`);
    }
  }
  assert.equal(await page.locator('.demo-controls').count(), 1, '模型广场只提供一个演示入口');
  assert.equal(await page.locator('.demo-controls-inline').count(), 1, '模型广场演示入口放在内容之后');
  assert.notEqual(await page.locator('.demo-controls').evaluate(node => getComputedStyle(node).position), 'fixed', '演示入口不得悬浮遮挡筛选或模型操作');
}
const priceRows = () => models().evaluateAll(nodes => nodes.map(node => ({
  id: node.getAttribute('data-model-id'),
  price: node.querySelector('.catalog-model-price')?.textContent.replace(/\s+/g, ' ').trim(),
})).sort((a, b) => a.id.localeCompare(b.id)));
async function assertView(expected) {
  await page.waitForFunction(value => document.querySelector(`[data-catalog-view="${value}"]`)?.getAttribute('aria-pressed') === 'true', expected === 'list' ? 'list' : 'cards');
  await settle();
  const active = page.locator(`[data-catalog-view="${expected === 'list' ? 'list' : 'cards'}"]`);
  assert.equal(await active.getAttribute('aria-pressed'), 'true');
}

try {
  await newPage();
  let initialIds, initialMoney, cardPrices;
  await check('默认广场按厂商分类，搜索宽度克制，无类型及价格分组选择器', async () => {
    await go();
    initialIds = await ids();
    initialMoney = moneyState(await snapshot());
    cardPrices = await priceRows();
    assert.equal(initialIds.length, 8);
    assert.equal(new Set(initialIds).size, initialIds.length);
    assert.equal(await filters().getByRole('button', { name: /指定模型|智能路由|Specific models|Smart routing/ }).count(), 0);
    assert.equal(await page.locator('main select').count(), 0, '厂商与分组均不得再通过下拉组成多层目录分类');
    for (const vendor of ['all', 'OpenAI', 'Anthropic', 'Google', 'DeepSeek', 'Qwen', 'Veyra']) {
      assert.equal(await vendorButton(vendor).count(), 1);
    }
    assert.ok((await page.locator('.catalog-search').boundingBox()).width <= 361);
    await assertView('grid'); await noOverflow();
    assert.ok(cardPrices.every(row => row.price), '每条模型应有明确价格或不可用状态');
    await capture('cards-desktop');
  });
  await check('各厂商标签真实筛选，Veyra（平台）含智能路由及既有媒体计费样例', async () => {
    const expectations = {
      OpenAI: ['gpt-4.1'], Anthropic: ['claude-sonnet-4-20250514'], Google: ['gemini-2.5-pro'],
      DeepSeek: ['deepseek-chat'], Qwen: ['qwen-plus'], Veyra: ['veyra-auto', 'veyra-demo-image', 'veyra-demo-video'],
    };
    for (const [vendor, expected] of Object.entries(expectations)) {
      await chooseVendor(vendor);
      assert.deepEqual((await ids()).sort(), [...expected].sort());
      assert.equal(await vendorButton(vendor).getAttribute('aria-pressed'), 'true');
      assert.equal(new URL(page.url()).searchParams.get('vendor'), vendor);
    }
    assert.match(await model('veyra-auto').innerText(), /智能路由/);
    assert.match(await model('veyra-auto').innerText(), /按实际模型计费/);
    await capture('veyra-vendor');
  });
  await check('卡片和列表使用同一组模型、价格摘要、单位及路由计价类型', async () => {
    await chooseVendor('all');
    await switchView('list'); await assertView('list');
    assert.deepEqual((await ids()).sort(), [...initialIds].sort());
    assert.deepEqual(await priceRows(), cardPrices);
    assert.ok(await model('gpt-4.1').getByRole('link', { name: /接入此模型/ }).isVisible());
    assert.match(await model('gpt-4.1').innerText(), /OpenAI/);
    assert.match(await model('gpt-4.1').innerText(), /文本|编程/);
    await noOverflow(); await capture('list-desktop');
  });
  await check('模型名称、标识和厂商搜索与标签组合，空结果可清除后继续操作', async () => {
    for (const [query, expected] of [['GPT-4.1', ['gpt-4.1']], ['deepseek-chat', ['deepseek-chat']], ['Anthropic', ['claude-sonnet-4-20250514']]]) {
      await fillSearch(query);
      assert.deepEqual(await ids(), expected);
    }
    await chooseVendor('OpenAI');
    await fillSearch('does-not-exist');
    assert.equal(await models().count(), 0);
    assert.ok(await page.getByRole('heading', { name: '没有符合条件的模型' }).isVisible());
    await page.getByRole('button', { name: '清除筛选', exact: true }).click(); await settle();
    assert.equal(await search().inputValue(), '');
    assert.equal(await vendorButton('all').getAttribute('aria-pressed'), 'true');
    assert.deepEqual((await ids()).sort(), [...initialIds].sort());
    await assertView('list');
  });
  await check('筛选和列表模式刷新后恢复，详情返回与浏览器后退保持原筛选', async () => {
    await chooseVendor('OpenAI'); await fillSearch('gpt');
    const filteredUrl = page.url();
    await page.reload(); await page.locator('.catalog-search input').waitFor();
    assert.equal(await search().inputValue(), 'gpt');
    assert.equal(await vendorButton('OpenAI').getAttribute('aria-pressed'), 'true');
    await assertView('list');
    await model('gpt-4.1').getByRole('link', { name: 'GPT-4.1', exact: true }).click();
    await page.locator('.access-detail-heading h1').waitFor();
    await page.reload(); await page.locator('.access-detail-heading h1').waitFor();
    await page.getByRole('link', { name: '返回模型目录', exact: true }).click();
    assert.equal(page.url(), filteredUrl);
    assert.equal(await search().inputValue(), 'gpt'); await assertView('list');
    await model('gpt-4.1').getByRole('link', { name: 'GPT-4.1', exact: true }).click();
    await page.locator('.access-detail-heading h1').waitFor();
    await page.goBack(); await page.locator('.catalog-search input').waitFor();
    assert.equal(page.url(), filteredUrl);
    assert.equal(await search().inputValue(), 'gpt'); await assertView('list');
  });
  await check('无视图参数时记住本地列表偏好，地址显式视图优先且可键盘切换', async () => {
    await go('/models'); await assertView('list');
    await go('/models?view=cards'); await assertView('grid');
    const list = page.locator('[data-catalog-view="list"]');
    await list.focus(); await page.keyboard.press('Enter'); await assertView('list');
    const vendor = vendorButton('OpenAI'); await vendor.focus(); await page.keyboard.press('Enter'); await settle();
    assert.deepEqual(await ids(), ['gpt-4.1']);
    const grid = page.locator('[data-catalog-view="cards"]');
    await grid.focus(); await page.keyboard.press('Space'); await assertView('grid');
  });
  await check('浏览目录、切换样式及详情不修改余额、流水、密钥或调用', async () => {
    assert.deepEqual(moneyState(await snapshot()), initialMoney);
  });
  await check('切换真实已有密钥不改变广场公开最低价，详情仍按该密钥分组计价且不改授权账本', async () => {
    await go('/console/connect'); await page.locator('#connect-key').waitFor();
    const before = moneyState(await snapshot());
    const availableKeys = (await snapshot()).keys;
    const standardKey = availableKeys.find(key => key.tier === 'standard');
    const proKey = availableKeys.find(key => key.tier === 'pro');
    assert.ok(standardKey && proKey);
    await page.locator('#connect-key').selectOption(standardKey.id);
    await go('/models?view=list');
    const standard = await model('gpt-4.1').locator('.catalog-model-price').innerText();
    assert.match(await model('gpt-4.1').locator('[data-price-kind="public"]').first().innerText(), /\$1\.2/);
    await go('/models/gpt-4.1');
    assert.equal(await page.locator('.access-inline-label select').inputValue(), 'standard');
    assert.match(await page.locator('.directory-detail-single .directory-price').innerText(), /\$1\.2\s*\/\s*\$4\.8/);
    await go('/console/connect'); await page.locator('#connect-key').selectOption(proKey.id);
    await go('/models?view=list');
    const pro = await model('gpt-4.1').locator('.catalog-model-price').innerText();
    assert.equal(standard, pro, '广场公开最低价不随当前密钥分组改变');
    await go('/models/gpt-4.1');
    assert.equal(await page.locator('.access-inline-label select').inputValue(), 'pro');
    assert.match(await page.locator('.directory-detail-single .directory-price').innerText(), /\$1\.6\s*\/\s*\$6\.4/);
    assert.deepEqual(moneyState(await snapshot()), before);
  });
  await check('媒体和智能条目保留清晰类型，完整计费条件下沉至详情', async () => {
    await go('/models?view=list&vendor=Veyra');
    for (const [id, unit] of [['veyra-demo-image', /美元 \/ 张/], ['veyra-demo-video', /美元 \/ 秒/]]) {
      assert.match(await model(id).locator('.catalog-model-price').innerText(), id === 'veyra-demo-image' ? /\$0\.016/ : /\$0\.032/);
      assert.match(await model(id).locator('.catalog-model-price').innerText(), unit);
      assert.match(await model(id).innerText(), /样例/);
      assert.equal(await model(id).getByRole('link', { name: '接入此模型', exact: true }).count(), 0);
      await model(id).getByRole('link').first().click();
      await page.locator('.access-detail-heading h1').waitFor();
      assert.match(await page.locator('.price-comparison').innerText(), unit);
      assert.match(await page.locator('main').innerText(), /未提供客户端接口|不提供客户端|计费样例|仅模拟/);
      await page.getByRole('link', { name: '返回模型目录', exact: true }).click();
      await model(id).waitFor();
    }
    assert.match(await model('veyra-auto').innerText(), /按实际模型计费/);
    assert.doesNotMatch(await model('veyra-auto').innerText(), /1,000 输入/);
    await model('veyra-auto').getByRole('link', { name: /Veyra Auto/ }).first().click();
    await page.locator('.access-detail-heading h1').waitFor();
    assert.match(await page.locator('.price-comparison').innerText(), /1,000 输入 \+ 300 输出词元/);
    await page.getByRole('link', { name: '返回模型目录', exact: true }).click();
    await model('veyra-auto').waitFor(); await capture('veyra-list');
  });
  await check('未知模型详情明确不存在，不替换为另一个模型且可返回目录', async () => {
    await go('/models/not-a-real-model');
    assert.equal(await page.locator('main h1').innerText(), '模型不存在');
    assert.match(await page.locator('main').innerText(), /not-a-real-model/);
    assert.equal(await page.locator('.access-detail-heading').count(), 0);
    await page.getByRole('link', { name: '返回模型目录', exact: true }).click();
    await search().waitFor(); assert.ok(await models().count());
  });
  await newPage();
  await check('真实创建标准模型后厂商标签自动出现，未投放不会冒充可调用', async () => {
    await go('/console/model-pricing');
    await page.getByRole('button', { name: '创建标准模型', exact: true }).click();
    await page.getByLabel('模型名称', { exact: true }).fill('目录厂商验证模型');
    await page.getByLabel('调用标识', { exact: true }).fill('catalog-new-provider-demo');
    await page.getByLabel('模型厂商', { exact: true }).fill('目录测试厂商');
    await page.getByLabel('输入基准价', { exact: true }).fill('2');
    await page.getByRole('button', { name: '保存标准模型', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await go('/models?view=list');
    await chooseVendor('目录测试厂商');
    assert.deepEqual(await ids(), ['catalog-new-provider-demo']);
    assert.match(await model('catalog-new-provider-demo').locator('.catalog-model-price').innerText(), /不可用|暂无公开可用资源/);
    await page.reload(); await search().waitFor();
    assert.deepEqual(await ids(), ['catalog-new-provider-demo']);
    await model('catalog-new-provider-demo').getByRole('link', { name: '目录厂商验证模型', exact: true }).click();
    await page.locator('.access-detail-heading h1').waitFor();
    assert.equal(await page.getByRole('button', { name: '当前分组不可接入', exact: true }).isDisabled(), true);
  });
  await check('通过标准模型管理停用后保留准确详情和下架状态，不替换模型', async () => {
    await go('/console/model-pricing?model=catalog-new-provider-demo');
    await page.getByRole('button', { name: '编辑模型信息', exact: true }).click();
    await page.getByRole('checkbox', { name: '启用此标准模型', exact: true }).uncheck();
    await page.getByRole('button', { name: '保存标准模型', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await go('/models/catalog-new-provider-demo');
    assert.equal(await page.locator('.access-detail-heading h1').innerText(), '目录厂商验证模型');
    assert.match(await page.locator('main').innerText(), /此模型已下架/);
    assert.equal(await page.getByRole('link', { name: '开始接入', exact: true }).count(), 0);
  });
  for (const width of [390, 320]) {
    await newPage({ width, height: width === 320 ? 568 : 844 });
    await check(`${width}像素卡片与列表无整页横向溢出，厂商筛选及模型入口可操作`, async () => {
      await go(); await noOverflow(); await capture(`cards-mobile-${width}`);
      await switchView('list'); await noOverflow();
      await chooseVendor('Veyra');
      assert.deepEqual((await ids()).sort(), ['veyra-auto', 'veyra-demo-image', 'veyra-demo-video']);
      await noOverflow(); await capture(`list-mobile-${width}`);
      await model('veyra-auto').scrollIntoViewIfNeeded();
      await capture(`list-mobile-${width}-first-model`);
      await model('veyra-auto').getByRole('link', { name: /Veyra Auto/ }).click();
      await page.locator('.access-detail-heading h1').waitFor();
      await page.getByRole('link', { name: '返回模型目录', exact: true }).click();
      await assertView('list'); await noOverflow();
    });
  }
  for (const language of ['en', 'ar']) {
    await newPage({ width: 390, height: 844, language, theme: 'dark' });
    await check(`${language === 'en' ? '英文' : '阿拉伯文'}深色窄屏厂商分类与两种视图可用`, async () => {
      await go();
      assert.equal(await page.locator('html').getAttribute('dir'), language === 'ar' ? 'rtl' : 'ltr');
      assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
      await noOverflow(); await capture(`cards-${language}-dark`);
      await switchView('list'); await chooseVendor('Veyra');
      await assertView('list'); await noOverflow();
      assert.equal(await models().count(), 3);
      await capture(`list-${language}-dark`);
      await model('veyra-auto').scrollIntoViewIfNeeded();
      await capture(`list-${language}-dark-first-model`);
    });
  }
} finally {
  report.completedAt = new Date().toISOString();
  report.passed = report.checks.filter(item => item.passed).length;
  report.total = report.checks.length;
  await writeFile(`${out}/${prefix}-browser-report.json`, JSON.stringify(report, null, 2));
  await browser.close();
}
if (report.errors.length || report.checks.some(item => !item.passed)) process.exitCode = 1;
console.log(`${report.passed}/${report.total} checks passed; ${report.errors.length} page errors`);
