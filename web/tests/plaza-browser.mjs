import { chromium, request } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

// Run against TestP6BrowserServer and a Vite server proxying to that fixture.
// All setup writes below affect only the fixture's temporary SQLite database.
const base = process.env.STARHUB_PLAZA_URL || 'http://127.0.0.1:5173';
assert.equal(process.env.STARHUB_PLAZA_FIXTURE, '1', 'Start the isolated TestP6BrowserServer fixture before running this test.');
const out = 'test-results/plaza';
await mkdir(out, { recursive: true });
const api = await request.newContext({ baseURL: base });
async function login(context) {
  const response = await context.post('/api/user/login', { data: { username: 'root', password: '123456' } });
  assert.equal(response.status(), 200);
}
async function put(path, data) {
  const response = await api.put(path, { data });
  assert.equal(response.status(), 200, await response.text());
}
await login(api);
await put('/api/option/', { key: 'group_ratio', value: JSON.stringify({ default: 1, vip: 0.6 }) });
await put('/api/option/', { key: 'model_ratio', value: JSON.stringify({ 'plaza-live-a': 123, 'publisher/model:latest': 123, 'plaza-conditional': 199 }) });
await put('/api/option/', { key: 'completion_ratio', value: JSON.stringify({ 'plaza-live-a': 2, 'publisher/model:latest': 2 }) });
await put('/api/option/', { key: 'model_price', value: JSON.stringify({ 'plaza-call': 0.04 }) });
await put('/api/option/', { key: 'model_price_expr', value: JSON.stringify({ 'plaza-conditional': 'prompt_tokens / 1000000 * 77' }) });
const meta = (model, vendor = 'openai', vendor_display = 'OpenAI', patch = {}) => ({
  model, vendor, vendor_display, tags: 'text,function_call', context_length: 1500000, max_output: 4096,
  description: 'Live fixture model', official_input_per_m: 456, official_output_per_m: 912, ...patch,
});
for (const data of [
  meta('plaza-live-a', 'openai', 'OpenAI', { description: 'description-only-needle' }),
  meta('publisher/model:latest'),
  meta('plaza-vip-only'),
  meta('plaza-google', 'google', 'Google'),
  meta('plaza-call', 'other', 'Other', { tags: 'image', context_length: 0, official_input_per_m: null, official_output_per_m: null, official_per_request: 0.08 }),
  meta('plaza-conditional', 'other', 'Other'),
]) await put('/api/plaza/meta', data);
for (let i = 0; i < 201; i++) {
  await put('/api/plaza/meta', meta(`plaza-filler-${String(i).padStart(3, '0')}`, 'other', 'Other', { context_length: 0, tags: '' }));
}
for (const data of [
  { name: 'Plaza text fixture', type: 1, models: 'plaza-live-a,publisher/model:latest,plaza-call,plaza-conditional', group: 'default,vip' },
  { name: 'Plaza VIP fixture', type: 1, models: 'plaza-vip-only', group: 'vip' },
  { name: 'Plaza Gemini fixture', type: 25, models: 'plaza-google', group: 'default' },
]) {
  const response = await api.post('/api/channel/', { data: { key: 'fixture-only-key', base_url: 'http://127.0.0.1:1', ...data } });
  assert.equal(response.status(), 200, await response.text());
}
const facets = (await (await api.get('/api/plaza/filters')).json()).data;
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = { checks: [], errors: [], screenshots: [], backend: 'real router with isolated temporary database', faultInjection: '500 responses and delayed list response only' };
let page;
let context;
async function newPage({ width = 1440, language = 'zh', theme = 'light' } = {}) {
  await context?.close();
  context = await browser.newContext({ baseURL: base, viewport: { width, height: 1000 }, reducedMotion: 'reduce' });
  await login(context.request);
  page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', (error) => report.errors.push(String(error)));
  await page.addInitScript(({ language, theme }) => {
    localStorage.setItem('xy-language', language);
    localStorage.setItem('xy-theme', theme);
  }, { language, theme });
}
async function go(path = '/models') {
  await page.goto(base + path);
  await page.locator('main h1').first().waitFor();
}
const cards = () => page.locator('[data-model-id]');
const card = (id) => page.locator(`[data-model-id="${id}"]`);
async function loaded() {
  await page.waitForFunction(() => !document.querySelector('.catalog-empty[role="status"]'));
}
async function check(name, run) {
  await run();
  report.checks.push(name);
  console.log('PASS ' + name);
}
async function capture(name) {
  const path = `${out}/${name}.png`;
  await page.screenshot({ path, fullPage: true });
  report.screenshots.push(path);
}
try {
  await newPage();
  await check('real backend catalog and global facets', async () => {
    await go();
    await card('plaza-live-a').waitFor();
    assert.equal(await cards().count(), 24);
    assert.equal(await page.locator('[data-catalog-vendor="all"] span').innerText(), String(facets.total));
    assert.equal(await page.locator('[data-catalog-vendor="openai"]').count(), 1);
    await capture('desktop-cards');
  });
  await check('vendor ID, display label and stable counts', async () => {
    const response = page.waitForResponse((r) => r.url().includes('/api/plaza/models?') && new URL(r.url()).searchParams.get('vendors') === 'openai');
    await page.locator('[data-catalog-vendor="openai"]').click();
    assert.equal((await response).status(), 200);
    await loaded();
    assert.ok(await card('plaza-live-a').count());
    assert.ok(await page.locator('[data-catalog-vendor="google"]').count());
    assert.equal(await page.locator('[data-catalog-vendor="all"] span').innerText(), String(facets.total));
  });
  await check('description search, real prices, context and connection link', async () => {
    await page.locator('.catalog-search input').fill('description-only-needle');
    await page.waitForFunction(() => document.querySelectorAll('[data-model-id]').length === 1);
    assert.equal(await card('plaza-live-a').count(), 1);
    assert.ok((await card('plaza-live-a').innerText()).includes('$123'));
    assert.ok((await card('plaza-live-a').innerText()).includes('$456'));
    assert.ok((await card('plaza-live-a').innerText()).includes('1.5M'));
    assert.equal(await card('plaza-live-a').locator('.catalog-try').getAttribute('href'), '/console/connect?model=plaza-live-a');
  });
  await check('empty search result without local fallback', async () => {
    await page.locator('.catalog-search input').fill('no-such-model-needle');
    await page.locator('.catalog-empty h2').waitFor();
    assert.equal(await cards().count(), 0);
    assert.equal(await card('gpt-4.1').count(), 0);
  });
  await check('pagination, view persistence and filter page reset', async () => {
    await go('/models?page=2&view=list');
    await page.locator('.catalog-list-row').first().waitFor();
    assert.equal(await cards().count(), 24);
    assert.ok((await page.locator('.plaza-pagination').innerText()).includes('2 / 9'));
    await page.locator('[data-catalog-view="cards"]').click();
    await page.locator('.catalog-card').first().waitFor();
    assert.equal(new URL(page.url()).searchParams.get('page'), '2');
    await page.locator('[data-catalog-vendor="openai"]').focus();
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => new URL(location.href).searchParams.get('vendor') === 'openai');
    assert.equal(new URL(page.url()).searchParams.get('page'), null);
    await loaded();
    await page.locator('[data-catalog-vendor="all"]').click();
    await card('plaza-live-a').waitFor();
    await page.locator('.plaza-pagination button').last().click();
    await page.waitForFunction(() => new URL(location.href).searchParams.get('page') === '2');
    await page.locator('.catalog-search input').fill('description-only-needle');
    await card('plaza-live-a').waitFor();
    assert.equal(new URL(page.url()).searchParams.get('page'), null);
  });
  await check('encoded model detail, group coverage and return destination', async () => {
    const returnTo = '/models?vendor=other&page=2&view=list';
    await go('/models/' + encodeURIComponent('publisher/model:latest') + '?from=' + encodeURIComponent(returnTo));
    await page.locator('.access-detail-heading h1').waitFor();
    assert.equal(await page.locator('.access-detail-heading h1').innerText(), 'publisher/model:latest');
    assert.ok((await page.locator('[data-price-axis="input"]').innerText()).includes('$123'));
    assert.ok((await page.locator('[data-price-axis="input"]').innerText()).includes('$456'));
    assert.equal(await page.locator('[data-current-group="true"]').innerText(), 'default（当前）');
    assert.ok((await page.locator('main').innerText()).includes('vip'));
    assert.ok((await page.locator('main').innerText()).includes('Anthropic'));
    await capture('desktop-detail');
    assert.equal(await page.locator('main > a').first().getAttribute('href'), returnTo);
    await page.locator('main > a').first().click();
    await page.locator('.catalog-list-row').first().waitFor();
    assert.equal(new URL(page.url()).searchParams.get('page'), '2');
  });
  await check('detail 404 does not display a sample model', async () => {
    await go('/models/gpt-4.1');
    assert.ok((await page.locator('main h1').innerText()).includes('模型不存在'));
    assert.equal(await page.locator('[data-pricing-mode]').count(), 0);
  });
  await check('VIP-only model remains unavailable in the default group', async () => {
    await go('/models/plaza-vip-only');
    await page.locator('.access-detail-heading h1').waitFor();
    assert.equal(await page.locator('main a[href^="/console/connect"]').count(), 0);
    assert.equal(await page.locator('[data-current-group="true"]').count(), 0);
    assert.equal(await page.locator('.plaza-group-list span').filter({ hasText: /^vip$/ }).count(), 1);
  });
  await check('per-request and conditional pricing', async () => {
    await go('/models/plaza-call');
    await page.locator('[data-pricing-mode="per_request"]').waitFor();
    assert.ok((await page.locator('[data-price-axis="request"]').innerText()).includes('$0.04'));
    assert.ok((await page.locator('[data-price-axis="request"]').innerText()).includes('$0.08'));
    await go('/models/plaza-conditional');
    await page.locator('[data-pricing-mode="tiered"]').waitFor();
    assert.ok(!(await page.locator('[data-pricing-mode="tiered"]').innerText()).includes('$199'));
  });
  await check('loading state, backend failure and retry', async () => {
    let release;
    const pending = new Promise((resolve) => { release = resolve; });
    await page.route('**/api/plaza/models?*', async (route) => { await pending; await route.continue(); });
    await page.goto(base + '/models');
    await page.locator('.catalog-empty[role="status"]').waitFor();
    assert.equal(await cards().count(), 0);
    release();
    await card('plaza-live-a').waitFor();
    await page.unroute('**/api/plaza/models?*');
    await page.route('**/api/plaza/models?*', (route) => route.fulfill({ status: 500, json: { success: false, message: 'Injected fixture failure' } }));
    await page.goto(base + '/models');
    await page.locator('.catalog-empty[role="alert"]').waitFor();
    assert.equal(await cards().count(), 0);
    await page.unroute('**/api/plaza/models?*');
    await page.locator('.catalog-empty button').click();
    await card('plaza-live-a').waitFor();
  });
  for (const config of [
    { width: 390, language: 'en', theme: 'light' },
    { width: 390, language: 'ar', theme: 'dark' },
    { width: 320, language: 'zh', theme: 'light' },
    { width: 1440, language: 'en', theme: 'dark' },
  ]) await check(`${config.language} ${config.theme} ${config.width}px layout`, async () => {
    await newPage(config);
    await go('/models?view=list');
    await card('plaza-live-a').waitFor();
    assert.equal(await page.evaluate(() => document.documentElement.dir), config.language === 'ar' ? 'rtl' : 'ltr');
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    await capture(`${config.language}-${config.theme}-${config.width}-list`);
    await card('plaza-live-a').locator('.catalog-model-name').click();
    await page.locator('.access-detail-heading').waitFor();
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    await capture(`${config.language}-${config.theme}-${config.width}-detail`);
  });
  assert.deepEqual(report.errors, []);
} finally {
  await writeFile(`${out}/report.json`, JSON.stringify(report, null, 2));
  await browser.close();
  await api.dispose();
}
