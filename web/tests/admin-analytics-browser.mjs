import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { chromium } from 'playwright';

const base = process.env.STARHUB_WEB_URL || 'http://127.0.0.1:5173';
const out = 'test-results/admin-analytics';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const errors = [];
let checks = 0;
const labels = {
  zh: ['总请求数', '请求成功率'],
  en: ['Total requests', 'Request success rate'],
  ar: ['إجمالي الطلبات', 'معدل نجاح الطلبات'],
};

// Isolate browser fixtures from real accounts and request logs.
async function open(language = 'zh', width = 1440, initialMode = 'history') {
  const context = await browser.newContext({ viewport: { width, height: 1000 } });
  await context.addInitScript(({ language }) => {
    localStorage.setItem('xy-language', language);
    localStorage.setItem('xy-theme', language === 'ar' ? 'dark' : 'light');
  }, { language });
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', error => errors.push(error.message));
  let mode = initialMode;
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    assert.equal(route.request().method(), 'GET');
    let data = [];
    let status = 200;
    if (path === '/api/user/self') {
      data = { id: 7, username: 'analytics-test', role: 100, group: 'default', status: 1, language };
    } else if (path === '/api/public/region') {
      data = { id: 'test', default_language: language, supported_languages: ['zh', 'en', 'ar'], feature_flags: {}, timezone: 'UTC' };
    } else if (path === '/api/audit') {
      data = { items: [], total: 0 };
    } else if (path === '/api/metrics') {
      if (mode === 'loading') await gate;
      if (mode === 'error') status = 503;
      data = {
        usage: {
          total_requests: mode === 'empty' ? 0 : 5,
          success_count: mode === 'empty' || mode === 'failed' ? 0 : 3,
          failed_count: mode === 'empty' ? 0 : mode === 'failed' ? 5 : 2,
        },
        requests: [{ class: 'success', count: 0 }, { class: 'upstream_error', count: 0 }],
        channel_stats: [], window: 100, affinity: [], generated_at: Math.floor(Date.now() / 1000),
      };
    }
    await route.fulfill({ status, json: status === 200 ? { success: true, data } : { success: false, message: 'Fixture unavailable' } });
  });
  await page.goto(`${base}/admin?tab=analytics`);
  const metric = index => page.getByText(labels[language][index], { exact: true }).locator('..').locator('.mono');
  await metric(0).waitFor().catch(async error => {
    await page.screenshot({ path: `${out}/failure.png`, fullPage: true });
    console.error(JSON.stringify({ url: page.url(), body: (await page.locator('body').innerText()).slice(0, 3000), errors }));
    throw error;
  });
  return { context, page, metric, setMode: value => { mode = value; }, release };
}

async function expectText(locator, expected) {
  await locator.page().waitForFunction(({ label, expected }) => {
    const node = [...document.querySelectorAll('div')].find(node => node.textContent === label);
    return node?.parentElement.querySelector('.mono')?.textContent === expected;
  }, { label: await locator.locator('..').locator('div').first().innerText(), expected });
  assert.equal(await locator.innerText(), expected);
}

try {
  for (const language of ['zh', 'en', 'ar']) {
    for (const width of [1440, 390]) {
      const { context, page, metric } = await open(language, width);
      await expectText(metric(0), '5');
      await expectText(metric(1), '60.00%');
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
      if (language === 'ar') {
        assert.equal(await page.locator('html').getAttribute('dir'), 'rtl');
        assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
      }
      await page.screenshot({ path: `${out}/${language}-${width}.png`, fullPage: true });
      checks++;
      await context.close();
    }
  }
  for (const [mode, total, rate] of [['empty', '0', '—'], ['failed', '5', '0.00%']]) {
    const { context, metric } = await open('zh', 1440, mode);
    await expectText(metric(0), total);
    await expectText(metric(1), rate);
    checks++;
    await context.close();
  }
  {
    const { context, metric, release } = await open('zh', 1440, 'loading');
    await expectText(metric(0), '加载中…');
    await expectText(metric(1), '加载中…');
    release();
    await expectText(metric(0), '5');
    await expectText(metric(1), '60.00%');
    checks++;
    await context.close();
  }
  {
    const { context, page, metric, setMode } = await open('zh', 390, 'error');
    await page.getByRole('alert').filter({ hasText: '请求统计加载失败' }).waitFor();
    await expectText(metric(0), '—');
    await expectText(metric(1), '—');
    setMode('history');
    await page.getByRole('button', { name: '重试', exact: true }).click();
    await expectText(metric(0), '5');
    await expectText(metric(1), '60.00%');
    assert.equal(await page.getByRole('alert').count(), 0);
    checks++;
    await context.close();
  }
  assert.deepEqual(errors, []);
  console.log(`PASS ${checks} admin analytics browser checks`);
} finally {
  await browser.close();
}
