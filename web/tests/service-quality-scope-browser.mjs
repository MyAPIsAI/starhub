import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, readFile } from 'node:fs/promises';

const base = process.env.STARHUB_WEB_URL || 'http://127.0.0.1:5173';
const out = 'test-results/service-quality-scope';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const errors = [];
let checks = 0;

// API fixtures isolate presentation and request scoping from real accounts and logs.
async function open(role, language = 'zh', width = 1440, periods = false) {
  const context = await browser.newContext({ viewport: { width, height: 1000 } });
  await context.addInitScript(({ language }) => {
    localStorage.setItem('xy-language', language);
    localStorage.setItem('xy-theme', language === 'ar' ? 'dark' : 'light');
  }, { language });
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  const requests = [];
  const qualityRequests = [];
  let failQuality = false;
  let trendMode = 'ok';
  let trendGate = Promise.resolve();
  let releaseTrend;
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) return route.continue();
    assert.equal(route.request().method(), 'GET', 'Browser checks must not mutate real data');
    let data;
    let status = 200;
    if (url.pathname === '/api/user/self') {
      data = { id: 7, username: 'quality-test', role, group: 'default', status: 1, quota: 0, used_quota: 0, language };
    } else if (url.pathname === '/api/public/region') {
      data = { id: 'test', default_language: language, supported_languages: ['zh', 'en', 'ar'], feature_flags: {}, timezone: 'UTC' };
    } else if (url.pathname === '/api/service-quality') {
      requests.push(url.searchParams.get('user_id'));
      qualityRequests.push(Object.fromEntries(url.searchParams));
      if (failQuality) status = 503;
      const model = url.searchParams.get('model');
      if (model && trendMode === 'loading') await trendGate;
      if (model && trendMode === 'error') status = 503;
      const user = url.searchParams.get('user_id');
      const currentMonth = new Date().toISOString().slice(0, 7);
      const month = url.searchParams.get('month') || currentMonth;
      const start = new Date(`${month}-01T00:00:00Z`);
      const next = new Date(start);
      next.setUTCMonth(next.getUTCMonth() + 1);
      const end = Math.min(next.getTime() / 1000, Math.floor(Date.now() / 1000) + 1);
      const count = (user === '99' ? 2 : user === '999999' ? 0 : 4) * (periods && month !== currentMonth ? 3 : 1);
      const metric = { model: 'glm-5.2', total_requests: count, success_count: count, failed_count: 0, success_rate: 1, avg_duration_ms: 8000, p95_duration_ms: 12000, total_quota: 1, error_rate: 0 };
      const other = { ...metric, model: 'gpt-4.1', success_count: 0, failed_count: count, success_rate: 0 };
      const models = model === 'gpt-4.1' ? [other] : model || !periods ? [metric] : [metric, other];
      const rate = model === 'gpt-4.1' ? 0 : model ? 1 : 0.5;
      data = {
        window_hours: Math.ceil((end - start.getTime() / 1000) / 3600),
        window_start: start.getTime() / 1000,
        window_end: end,
        overall: metric,
        by_model: count ? models : [],
        by_day: !periods || trendMode === 'empty' ? [] : [{ date: new Date((end - 1) * 1000).toISOString().slice(0, 10), total_requests: count, success_count: count * rate, failed_count: count * (1 - rate), success_rate: rate, avg_duration_ms: 8000, p95_duration_ms: 12000 }],
      };
    } else if (['/api/token', '/api/export'].includes(url.pathname)) {
      data = [];
    } else if (url.pathname === '/api/log') {
      data = { items: [], total: 0 };
    } else {
      status = 503;
    }
    await route.fulfill({ status, json: status === 200 ? { success: true, data } : { success: false, message: 'Fixture unavailable' } });
  });
  await page.goto(`${base}/console/service-quality?view=admin&customer=99`);
  await page.locator('.sq-page').waitFor().catch(async error => {
    await page.screenshot({ path: `${out}/failure.png`, fullPage: true });
    console.error(JSON.stringify({ url: page.url(), body: (await page.locator('body').innerText()).slice(0, 2500), errors }));
    throw error;
  });
  await page.locator('.sq-row-view').first().waitFor({ state: 'attached' });
  return { context, page, requests, qualityRequests, setFailQuality: value => { failQuality = value; }, setTrendMode: value => { trendMode = value; if (value === 'loading') trendGate = new Promise(resolve => { releaseTrend = resolve; }); }, releaseTrend: () => releaseTrend?.() };
}

try {
  for (const role of [1, 5, 7, 10, 100]) {
    const { context, page, requests } = await open(role);
    const input = page.getByRole('textbox', { name: '用户编号', exact: true });
    const admin = role === 10 || role === 100;
    assert.equal(await input.isDisabled(), !admin);
    assert.equal(await input.inputValue(), admin ? '99' : '7');
    assert.equal(requests.at(-1), admin ? '99' : null);
    const sample = page.locator('.sq-table-scroll tbody .sq-numeric').first();
    assert.equal(await sample.innerText(), admin ? '2' : '4');
    if (admin) {
      await input.fill('');
      await page.waitForFunction(() => document.querySelector('.sq-numeric')?.textContent === '4');
      assert.equal(requests.at(-1), null);
      await input.fill('99');
      await page.waitForFunction(() => document.querySelector('.sq-numeric')?.textContent === '2');
      await input.fill('999999');
      await page.getByRole('heading', { name: '当前筛选下没有核算单元' }).waitFor();
      await input.fill('99');
      await page.locator('.sq-row-view').first().waitFor();
    }
    await page.locator('.sq-row-view').first().click();
    await page.locator('.sq-detail-identity').waitFor();
    assert.ok((await page.locator('.sq-detail-identity').innerText()).includes(admin ? '#99' : '#7'));
    await page.getByRole('button', { name: '返回交付列表', exact: true }).click();
    await page.locator('.sq-table-card').waitFor();
    await page.screenshot({ path: `${out}/role-${role}.png`, fullPage: true });
    checks++;
    await context.close();
  }

  for (const [language, label] of [['zh', '用户编号'], ['en', 'User ID'], ['ar', 'معرّف المستخدم']]) {
    const { context, page } = await open(100, language, 390);
    const input = page.getByRole('textbox', { name: label, exact: true });
    await input.fill('');
    await page.waitForFunction(() => document.querySelector('.sq-mobile-unit')?.textContent.includes('4'));
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    await input.focus();
    await page.keyboard.type('99');
    await page.waitForFunction(() => document.querySelector('.sq-mobile-unit')?.textContent.includes('#99')).catch(async error => {
      await page.screenshot({ path: `${out}/mobile-failure.png`, fullPage: true });
      console.error(JSON.stringify({ language, input: await input.inputValue(), url: page.url(), body: (await page.locator('.sq-page').innerText()).slice(0, 1600) }));
      throw error;
    });
    await page.screenshot({ path: `${out}/mobile-${language}.png`, fullPage: true });
    checks++;
    await context.close();
  }

  const { context, page, setFailQuality } = await open(100);
  setFailQuality(true);
  await page.getByRole('textbox', { name: '用户编号', exact: true }).fill('123');
  await page.getByRole('alert').filter({ hasText: '服务质量数据加载失败' }).waitFor();
  assert.equal(await page.locator('.sq-row-view').count(), 0);
  setFailQuality(false);
  await page.getByRole('button', { name: '重试', exact: true }).click();
  await page.locator('.sq-row-view').first().waitFor();
  checks++;
  await context.close();

  for (const role of [1, 100]) {
    const { context, page, qualityRequests, setTrendMode, releaseTrend } = await open(role, 'en', 1440, true);
    const period = page.getByRole('combobox', { name: 'Accounting period', exact: true });
    const months = await period.locator('option').evaluateAll(nodes => nodes.map(node => node.value));
    const past = months.at(-1);
    await period.selectOption(past);
    const expectedCount = role === 100 ? '6' : '12';
    await page.waitForFunction(count => document.querySelector('.sq-table-scroll tbody .sq-numeric')?.textContent === count, expectedCount);
    assert.equal(qualityRequests.at(-1).month, past);
    assert.equal(qualityRequests.at(-1).user_id, role === 100 ? '99' : undefined);
    assert.equal(qualityRequests.at(-1).hours, undefined);
    const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Export monthly report', exact: true }).click()]);
    const csv = await readFile(await download.path(), 'utf8');
    const start = new Date(`${past}-01T00:00:00Z`);
    const end = new Date(start);
    end.setUTCMonth(end.getUTCMonth() + 1);
    const lastDay = new Date(end.getTime() - 1000).toISOString().slice(0, 10);
    assert.ok(csv.includes(`"${past}","${past}-01","${lastDay}"`));
    assert.ok(csv.includes(`"${expectedCount}"`));
    assert.ok(!csv.includes('last 7 days'));
    checks++;

    setTrendMode('loading');
    await page.locator('.sq-row-view').first().click();
    await page.getByRole('status').filter({ hasText: 'Loading model trend' }).waitFor();
    assert.equal(await page.locator('.sq-trend-chart').count(), 0);
    setTrendMode('ok');
    releaseTrend();
    await page.waitForFunction(() => document.querySelector('.sq-trend-current strong')?.textContent === '100.00%');
    assert.equal(qualityRequests.at(-1).model, 'glm-5.2');
    assert.equal(qualityRequests.at(-1).month, past);
    assert.equal(qualityRequests.at(-1).user_id, role === 100 ? '99' : undefined);
    await page.screenshot({ path: `${out}/monthly-model-role-${role}.png`, fullPage: true });
    checks++;

    await page.getByRole('button', { name: 'Back to delivery list', exact: true }).click();
    await page.locator('.sq-row-view').nth(1).click();
    await page.waitForFunction(() => document.querySelector('.sq-trend-current strong')?.textContent === '0.00%');
    assert.equal(qualityRequests.at(-1).model, 'gpt-4.1');
    checks++;

    await page.getByRole('button', { name: 'Back to delivery list', exact: true }).click();
    await period.selectOption(months[0]);
    await page.waitForFunction(month => new URL(location.href).searchParams.get('month') === month, months[0]);
    setTrendMode('error');
    await page.locator('.sq-row-view').first().click();
    await page.getByRole('alert').filter({ hasText: "Could not load this model's trend" }).waitFor();
    assert.equal(await page.locator('.sq-trend-chart').count(), 0);
    setTrendMode('ok');
    await page.getByRole('button', { name: 'Retry', exact: true }).click();
    await page.waitForFunction(() => document.querySelector('.sq-trend-current strong')?.textContent === '100.00%');
    checks++;

    await page.getByRole('button', { name: 'Back to delivery list', exact: true }).click();
    setTrendMode('empty');
    await page.locator('.sq-row-view').nth(1).click();
    await page.getByRole('status').filter({ hasText: 'No trend data for this model' }).waitFor();
    assert.equal(await page.locator('.sq-trend-chart').count(), 0);
    checks++;
    await context.close();
  }
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ checks, pageErrors: errors, evidence: out, fixtureApis: true }));
} finally {
  await browser.close();
}
