import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { chromium, request } from 'playwright';
import { createServer } from 'vite';

// Writes are restricted to TestP6BrowserServer's temporary database.
assert.equal(process.env.STARHUB_PLAZA_FIXTURE, '1');
const backend = process.env.STARHUB_PLAZA_BACKEND || 'http://127.0.0.1:3011';
const out = 'test-results/plaza-meta';
await mkdir(out, { recursive: true });
const server = await createServer({ server: {
  port: 5181, strictPort: true, host: '127.0.0.1',
  proxy: { '/api': backend, '/v1': backend },
} });
await server.listen();
const base = 'http://127.0.0.1:5181';
const api = await request.newContext({ baseURL: base });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = { checks: [], errors: [], screenshots: [], backend: 'real router with isolated temporary database' };
let context;
let page;
async function login(client) {
  const response = await client.post('/api/user/login', { data: { username: 'root', password: '123456' } });
  assert.equal(response.status(), 200);
}
async function get(path) {
  const response = await api.get(path);
  assert.equal(response.status(), 200);
  return (await response.json()).data;
}
async function open({ language = 'zh', theme = 'light', width = 1440 } = {}) {
  await context?.close();
  context = await browser.newContext({ baseURL: base, viewport: { width, height: 1000 }, reducedMotion: 'reduce' });
  await login(context.request);
  page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', error => report.errors.push(String(error)));
  await page.addInitScript(({ language, theme }) => {
    localStorage.setItem('xy-language', language);
    localStorage.setItem('xy-theme', theme);
    localStorage.setItem('veyra-demo-account-role-v1', '100');
  }, { language, theme });
  await page.goto(base + '/console/model-pricing');
  await page.getByRole('row').filter({ hasText: 'vendor-fixture-a' }).getByRole('button', { name: '编辑', exact: true }).click();
  await page.getByRole('dialog').waitFor();
}
async function capture(name) {
  const file = `${name}.png`;
  await page.screenshot({ path: `${out}/${file}` });
  report.screenshots.push(file);
}
async function check(name, run) {
  try { await run(); report.checks.push({ name, passed: true }); console.log('PASS', name); }
  catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.error('FAIL', name, String(error));
    await capture(`failure-${report.checks.length}`).catch(() => {});
  }
}
try {
  await login(api);
  for (const model of ['vendor-fixture-a', 'vendor-fixture-b']) {
    const response = await api.put('/api/plaza/meta', { data: {
      model, name: model === 'vendor-fixture-a' ? 'GPT-4o Chat' : 'GPT Mini',
      vendor: 'openai', vendor_display: 'OpenAI', description: 'Identity regression fixture',
      tags: 'text,vision,function_call', context_length: 128000, max_output: 4096,
      official_input_per_m: 2.500000123, official_output_per_m: null,
      official_cache_read_per_m: 0, official_cache_write_per_m: null,
      official_per_request: 0.02, per_request_usd: 0.01,
      score: 85, compatible_groups: 'default,vip', sort_weight: 12,
    } });
    assert.equal(response.status(), 200);
  }
  const original = (await get('/api/plaza/meta')).find(row => row.model === 'vendor-fixture-a');
  await check('editing a model name leaves the vendor and other metadata intact', async () => {
    await open();
    assert.equal(await page.getByLabel('模型名称', { exact: true }).inputValue(), 'GPT-4o Chat');
    assert.equal(await page.getByLabel('厂商标识', { exact: true }).inputValue(), 'openai');
    assert.equal(await page.getByLabel('厂商显示名', { exact: true }).inputValue(), 'OpenAI');
    await page.getByLabel('模型名称', { exact: true }).fill('GPT-4o Renamed');
    await capture('edit-desktop-zh');
    const saved = page.waitForResponse(response => response.url().endsWith('/api/plaza/meta') && response.request().method() === 'PUT');
    await page.getByRole('button', { name: '保存标准模型', exact: true }).click();
    assert.equal((await saved).status(), 200);
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    const current = (await get('/api/plaza/meta')).find(row => row.model === original.model);
    assert.deepEqual(current, { ...original, name: 'GPT-4o Renamed' });
    await page.reload();
    await page.getByRole('button', { name: '编辑模型信息', exact: true }).click();
    assert.equal(await page.getByLabel('模型名称', { exact: true }).inputValue(), 'GPT-4o Renamed');
    assert.equal(await page.getByLabel('厂商显示名', { exact: true }).inputValue(), 'OpenAI');
  });
  await check('plaza cards, details and vendor tabs use separate identities', async () => {
    await page.goto(base + '/models');
    await page.locator('[data-model-id="vendor-fixture-a"]').waitFor();
    assert.ok((await page.locator('[data-catalog-vendor="openai"]').innerText()).includes('OpenAI'));
    assert.ok(!(await page.locator('[data-catalog-vendor="openai"]').innerText()).includes('Renamed'));
    assert.ok((await page.locator('[data-model-id="vendor-fixture-a"]').innerText()).includes('GPT-4o Renamed'));
    await page.goto(base + '/models/vendor-fixture-a');
    await page.getByRole('heading', { name: 'GPT-4o Renamed', exact: true }).waitFor();
    await capture('plaza-detail');
  });
  for (const [language, theme, width, labels] of [
    ['zh', 'dark', 390, ['模型名称', '厂商标识', '厂商显示名']],
    ['en', 'light', 320, ['Model name', 'Vendor ID', 'Vendor display name']],
    ['ar', 'dark', 390, ['اسم النموذج', 'معرّف المورّد', 'اسم المورّد المعروض']],
  ]) {
    await check(`${language} ${theme} ${width}px identity fields fit the dialog`, async () => {
      await open({ language, theme, width });
      const dialog = page.getByRole('dialog');
      for (const [i, value] of ['GPT-4o Renamed', 'openai', 'OpenAI'].entries()) {
        const field = dialog.getByLabel(labels[i], { exact: true });
        assert.equal(await field.inputValue(), value);
        const bounds = await field.boundingBox();
        assert.ok(bounds && bounds.x >= 0 && bounds.x + bounds.width <= width + 1);
      }
      assert.ok(await dialog.evaluate(node => node.scrollWidth <= node.clientWidth + 1));
      if (language === 'ar') assert.equal(await page.locator('html').getAttribute('dir'), 'rtl');
      await capture(`edit-${language}-${theme}-${width}`);
      await page.keyboard.press('Escape');
      await dialog.waitFor({ state: 'hidden' });
    });
  }
} finally {
  await context?.close();
  await browser.close();
  await api.dispose();
  await server.close();
  await writeFile(`${out}/report.json`, JSON.stringify(report, null, 2));
}
assert.ok(report.checks.every(check => check.passed) && report.errors.length === 0, JSON.stringify(report));
