import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-28-click-target';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || 'catalog-click';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = { base, startedAt: new Date().toISOString(), independentBrowserContext: true, businessStateInjection: false,
  setup: 'Only language/theme preferences are initialized. Navigation, selection, copying, modifier/middle clicks, keyboard and touch inputs use browser interactions. Mobile scrolling sends native touch input through the browser input protocol.',
  modifier: process.platform === 'darwin' ? 'Meta' : 'Control', checks: [], scans: [], screenshots: [], errors: [] };
let page, context;
async function fresh({ width = 1440, height = 1000, language = 'zh', theme = 'light', mobile = false } = {}) {
  if (context) await context.close();
  context = await browser.newContext({ viewport: { width, height }, reducedMotion: 'reduce', isMobile: mobile, hasTouch: mobile, permissions: ['clipboard-read', 'clipboard-write'] });
  page = await context.newPage(); page.setDefaultTimeout(7000);
  context.on('page', p => p.on('pageerror', e => report.errors.push(String(e))));
  page.on('pageerror', e => report.errors.push(String(e)));
  await page.addInitScript(({ language, theme }) => { localStorage.setItem('xy-language', language); localStorage.setItem('xy-theme', theme); }, { language, theme });
}
const item = (id = 'gpt-4.1') => page.locator(`[data-model-id="${id}"]`);
const title = () => item().locator('.catalog-model-name');
const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
async function go(view = 'cards', filtered = false) {
  await page.goto(`${base}/models?view=${view}${filtered ? '&vendor=OpenAI&q=gpt' : ''}`);
  await item().waitFor(); await settle();
}
async function detail(id = 'gpt-4.1') {
  await page.waitForURL(url => url.pathname === `/models/${id}`);
  await page.locator('.access-detail-heading h1').waitFor();
}
async function capture(name) { const file = `${prefix}-${name}.png`; await page.screenshot({ path: `${out}/${file}`, fullPage: false }); report.screenshots.push(file); }
async function check(name, fn) {
  try { await fn(); report.checks.push({ name, passed: true }); console.log('PASS', name); }
  catch (error) { report.checks.push({ name, passed: false, error: String(error) }); console.log('FAIL', name, error.stack); await capture(`failure-${report.checks.length}`).catch(() => {}); }
}
async function scan(name) {
  const r = await new AxeBuilder({ page }).include('main').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
  const violations = r.violations.map(v => ({ id: v.id, impact: v.impact, help: v.help, nodes: v.nodes.map(n => ({ target: n.target, failureSummary: n.failureSummary })) }));
  report.scans.push({ name, violations }); assert.equal(violations.length, 0, JSON.stringify(violations));
}
async function blankPoint() {
  await item().scrollIntoViewIfNeeded();
  const b = await item().boundingBox();
  return { x: b.x + 8, y: b.y + 8 };
}
const currentMoney = () => page.evaluate(() => { const s = JSON.parse(localStorage.getItem('veyra-billing-demo-v1')); return s && Object.fromEntries(['balance', 'ledger', 'liveRequests', 'rechargeOrders', 'keys'].map(k => [k, s[k]])); });
async function assertUnchangedRoute(before) { await settle(); assert.equal(page.url(), before); }
async function dragText(locator) {
  await locator.scrollIntoViewIfNeeded(); const box = await locator.boundingBox();
  await page.mouse.move(box.x + 1, box.y + box.height / 2);
  await page.mouse.down(); await page.mouse.move(box.x + box.width - 1, box.y + box.height / 2, { steps: 16 }); await page.mouse.up();
}
async function newTabClick(view, area, options) {
  console.log('TRY newtab', view, area, JSON.stringify(options));
  await go(view, true); const old = page.url();
  const popupPromise = context.waitForEvent('page', { timeout: 7000 });
  if (area === 'blank') {
    const p = await blankPoint();
    for (const key of options.modifiers || []) await page.keyboard.down(key);
    try { await page.mouse.click(p.x, p.y, { button: options.button || 'left' }); }
    finally { for (const key of [...(options.modifiers || [])].reverse()) await page.keyboard.up(key); }
  }
  else await item().locator(area === 'price' ? '[data-price-kind="public"]' : '.catalog-capabilities span').first().click(options);
  const popup = await popupPromise; await popup.waitForLoadState('domcontentloaded');
  await popup.waitForURL(url => url.pathname === '/models/gpt-4.1');
  assert.equal(new URL(popup.url()).searchParams.get('from'), '/models?view=' + view + '&vendor=OpenAI&q=gpt');
  assert.equal(page.url(), old, '原标签仍保留原筛选'); await popup.close();
}
try {
  await fresh();
  for (const view of ['cards', 'list']) {
    await check(`${view}整块空白、能力文字及价格单元格进入对应详情并返回原筛选`, async () => {
      for (const area of ['blank', 'capability', 'price']) {
        await go(view, true); const old = page.url(); const before = await currentMoney();
        if (area === 'blank') { const p = await blankPoint(); await page.mouse.click(p.x, p.y); }
        else if (area === 'capability') await item().locator('.catalog-capabilities span').first().click();
        else await item().locator('[data-price-kind="public"]').first().click();
        await detail(); assert.equal(new URL(page.url()).searchParams.get('from'), new URL(old).pathname + new URL(old).search);
        await page.goBack(); await item().waitFor(); assert.equal(page.url(), old); assert.equal(await page.locator('.catalog-search input').inputValue(), 'gpt');
        assert.deepEqual(await currentMoney(), before);
      }
    });
    await check(`${view}复制按钮保持独立且真实复制，不打开详情或接入页`, async () => {
      await go(view); const old = page.url(); await item().locator('[data-copy-model-id="gpt-4.1"]').click();
      await item().locator('.model-id-copy-feedback').filter({ hasText: '已复制' }).waitFor();
      assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'gpt-4.1'); await assertUnchangedRoute(old);
      const before = await currentMoney(); await item().locator('.catalog-try').click();
      await page.waitForURL(url => url.pathname === '/console/connect');
      assert.equal(new URL(page.url()).searchParams.get('model'), 'gpt-4.1'); assert.deepEqual(await currentMoney(), before);
    });
    await check(`${view}真实鼠标拖选标识与能力文字不会误跳详情，选择后复制仍可用`, async () => {
      await go(view); const old = page.url();
      await dragText(item().locator('code')); await assertUnchangedRoute(old);
      assert.match(await page.evaluate(() => getSelection()?.toString() || ''), /gpt-4\.1/);
      await capture(`${view}-text-selected`);
      await item().locator('[data-copy-model-id="gpt-4.1"]').click(); await assertUnchangedRoute(old);
      assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'gpt-4.1');
      await dragText(item().locator('.catalog-capabilities'));
      await assertUnchangedRoute(old); assert.match(await page.evaluate(() => getSelection()?.toString() || ''), /文本|编程|视觉|推理/);
    });
    await check(`${view}详情是可聚焦真实链接，Enter进入，Tab可独立到复制和接入`, async () => {
      await go(view, true); const link = title();
      assert.equal(await link.evaluate(n => n.tagName), 'A'); assert.match(await link.getAttribute('href'), /^\/models\/gpt-4\.1\?from=/);
      assert.equal(await item().locator('a a, a button').count(), 0, '不存在交互元素嵌套');
      for (let tab = 0; tab < 35 && !(await link.evaluate(n => n === document.activeElement)); tab++) await page.keyboard.press('Tab');
      assert.equal(await link.evaluate(n => n === document.activeElement), true);
      const focus = await link.evaluate(n => ({ outline: getComputedStyle(n, '::after').outlineStyle, width: getComputedStyle(n, '::after').outlineWidth }));
      assert.equal(focus.outline, 'solid', '整块伪元素获得键盘焦点提示');
      assert.ok(parseFloat(focus.width) >= 2, '焦点轮廓至少2像素可见');
      await capture(`${view}-keyboard-focus`);
      await page.keyboard.press('Tab'); assert.equal(await page.locator('[data-copy-model-id="gpt-4.1"]').evaluate(n => n === document.activeElement), true);
      await page.keyboard.press('Tab'); assert.equal(await item().locator('.catalog-try').evaluate(n => n === document.activeElement), true);
      await dragText(item().locator('code'));
      assert.match(await page.evaluate(() => getSelection()?.toString() || ''), /gpt-4\.1/);
      await link.focus(); await page.keyboard.press('Enter'); await detail();
      await page.goBack(); await item().waitFor(); const p = await blankPoint(); await page.mouse.move(p.x, p.y); await capture(`${view}-hover`);
      await scan(`中文桌面${view}扩大点击目标`);
    });
    await check(`${view}系统修饰键点击空白、能力和价格分别打开新标签，原目录不变化`, async () => {
      for (const area of ['blank', 'content', 'price']) await newTabClick(view, area, { modifiers: [report.modifier] });
    });
    await check(`${view}中键点击空白、能力和价格分别打开新标签，原目录不变化`, async () => {
      for (const area of ['blank', 'content', 'price']) await newTabClick(view, area, { button: 'middle' });
    });
  }
  for (const view of ['cards', 'list']) {
    await fresh({ width: 390, height: 844, mobile: true });
    await check(`手机${view}轻触价格可进入，触摸滑动保持目录且不触发跳转`, async () => {
      await go(view); await item().scrollIntoViewIfNeeded();
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
      const old = page.url(), y0 = await page.evaluate(() => scrollY), p = await blankPoint();
      const client = await context.newCDPSession(page);
      const x = Math.min(350, p.x + 140), y = Math.min(680, Math.max(250, p.y + 150));
      await client.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
      for (let step = 1; step <= 8; step++) await client.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: y - step * 22 }] });
      await client.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await settle(); assert.equal(page.url(), old); assert.ok(await page.evaluate(() => scrollY) > y0 + 30, '真实触摸滑动应滚动');
      await item().scrollIntoViewIfNeeded(); await capture(`${view}-mobile`);
      await item().locator('[data-price-kind="public"]').first().tap(); await detail();
      await page.goBack(); await item().waitFor(); assert.equal(page.url(), old);
    });
  }
  for (const [language, theme, view] of [['en', 'dark', 'cards'], ['ar', 'light', 'list']]) {
    await fresh({ width: 390, height: 844, language, theme });
    await check(`${language}/${theme}窄屏扩大点击目标、复制、焦点方向和无障碍仍正确`, async () => {
      await go(view); await item().scrollIntoViewIfNeeded();
      assert.equal(await page.locator('html').getAttribute('dir'), language === 'ar' ? 'rtl' : 'ltr');
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
      await page.keyboard.press('Tab'); await title().focus(); await item().scrollIntoViewIfNeeded(); await capture(`${view}-${language}-${theme}-focus`); await scan(`${language}/${theme}/${view}`);
      const old = page.url(); await item().locator('[data-copy-model-id="gpt-4.1"]').click();
      assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'gpt-4.1'); await assertUnchangedRoute(old);
      await item().locator('.catalog-capabilities span').first().click(); await detail();
    });
  }
} finally {
  report.completedAt = new Date().toISOString(); report.passed = report.checks.filter(c => c.passed).length; report.total = report.checks.length;
  await writeFile(`${out}/${prefix}-browser-report.json`, JSON.stringify(report, null, 2)); await browser.close();
}
console.log(`${report.passed}/${report.total} checks passed; ${report.scans.length} scans; ${report.errors.length} page errors`);
if (report.checks.some(c => !c.passed) || report.errors.length || report.scans.some(s => s.violations.length)) process.exitCode = 1;
