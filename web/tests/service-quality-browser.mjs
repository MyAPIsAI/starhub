import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-27';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || 'service-quality';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, acceptDownloads: true });
const page = await context.newPage();
page.setDefaultTimeout(8000);
const report = { startedAt: new Date().toISOString(), base, independentBrowserContext: true, checks: [], errors: [], screenshots: [] };
page.on('pageerror', error => report.errors.push(String(error)));
const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
const params = () => new URL(page.url()).searchParams;
const desktopRows = () => page.locator('.sq-table-scroll:visible tbody tr');
const detail = () => page.locator('.sq-detail-identity');
async function go(query = '') {
  await page.goto(`${base}/console/service-quality${query ? `?${query}` : ''}`);
  await page.locator('.sq-page').waitFor();
  await settle();
}
async function capture(name, fullPage = true) {
  const file = `${prefix}-${name}.png`;
  await page.screenshot({ path: `${out}/${file}`, fullPage });
  report.screenshots.push(file);
}
async function check(name, fn) {
  try { await fn(); report.checks.push({ name, passed: true }); console.log('PASS', name); }
  catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.log('FAIL', name, error.message);
    await capture(`failure-${report.checks.length}`).catch(() => {});
  }
}
async function noOverflow() {
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), '整页不得横向溢出');
}
async function noDemoOverlap() {
  const work = await page.locator('.sq-table-card:visible, .sq-analysis-grid:visible').last().boundingBox();
  const footer = await page.locator('.demo-inline-footer').boundingBox();
  assert.ok(work && footer && footer.y >= work.y + work.height - 1, '演示控制应位于服务质量内容之后');
  assert.notEqual(await page.locator('.demo-inline-footer .demo-controls').evaluate(e => getComputedStyle(e).position), 'fixed');
}
async function openUnit(id, keyboard = false) {
  const button = page.locator(`.sq-row-view[data-unit-id="${id}"]:visible, .sq-mobile-unit button[data-unit-id="${id}"]:visible`);
  if (keyboard) { await button.focus(); await page.keyboard.press('Enter'); }
  else await button.click();
  await detail().waitFor(); await settle();
  await page.waitForFunction(() => window.scrollY === 0);
  assert.equal(params().get('unit'), id);
  assert.ok(params().get('month'));
}
async function backToList() {
  await page.getByRole('button', { name: '返回交付列表', exact: true }).click();
  await page.locator('.sq-table-card').waitFor(); await settle();
}
function csvRows(text) {
  const rows = []; let row = [], value = '', quoted = false;
  const csv = text.replace(/^\uFEFF/, '');
  for (let i = 0; i < csv.length; i++) {
    const c = csv[i];
    if (c === '"') { if (quoted && csv[i + 1] === '"') { value += '"'; i++; } else quoted = !quoted; }
    else if (c === ',' && !quoted) { row.push(value); value = ''; }
    else if ((c === '\n' || c === '\r') && !quoted) {
      if (c === '\r' && csv[i + 1] === '\n') i++;
      row.push(value); if (row.some(Boolean)) rows.push(row); row = []; value = '';
    } else value += c;
  }
  if (value || row.length) { row.push(value); rows.push(row); }
  return rows;
}
async function download(name) {
  const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name, exact: true }).click()]);
  return { name: download.suggestedFilename(), rows: csvRows(await readFile(await download.path(), 'utf8')) };
}
await page.goto(base);
await page.evaluate(() => { localStorage.setItem('xy-language', 'zh'); localStorage.setItem('xy-theme', 'light'); });
await go();
const monthOptions = await page.locator('.sq-filters label').filter({ hasText: '核算账期' }).locator('select').locator('option').evaluateAll(options => options.map(option => option.value));
const [month, previousMonth] = monthOptions;

await check('列表不预选详情；明确入口进入含账期与单元的独立详情', async () => {
  await go();
  await page.getByRole('heading', { name: '合同核算单元', exact: true }).waitFor();
  assert.equal(await detail().count(), 0);
  assert.equal(await page.locator('.sq-bucket-section, .sq-analysis-grid, .sq-unit-select').count(), 0);
  assert.equal(await desktopRows().count(), 5);
  await noOverflow(); await noDemoOverlap(); await capture('list-desktop');
  await openUnit('sq-2');
  assert.equal(params().get('month'), month);
  assert.equal(await page.locator('.sq-table-card, .sq-filter-card, .sq-summary-grid, .sq-unit-select').count(), 0);
  const identity = await detail().innerText();
  for (const value of ['#1001', 'claude-sonnet-4-20250514', '流式', month, `DEMO-U-${month.replace('-', '.')}`]) assert.ok(identity.includes(value), value);
  assert.ok((await page.locator('.sq-demo-banner').innerText()).includes('前端演示'));
  assert.equal(await page.locator('.sq-heading h1').evaluate(e => e === document.activeElement), true);
  await noDemoOverlap(); await capture('detail-desktop');
});

await check('非默认账期与流式筛选第二页：刷新、后退、前进、返回保留位置和焦点', async () => {
  await go();
  await page.locator('.sq-filters label').filter({ hasText: '核算账期' }).locator('select').selectOption(previousMonth);
  await page.locator('.sq-filters label').filter({ hasText: '请求类型' }).locator('select').selectOption('streaming');
  await page.getByRole('button', { name: '下一页', exact: true }).click(); await settle();
  assert.equal(params().get('page'), '2');
  const listUrl = page.url();
  const button = page.locator('.sq-row-view:visible').first();
  const id = await button.getAttribute('data-unit-id');
  await button.scrollIntoViewIfNeeded(); await settle();
  const scroll = await page.evaluate(() => scrollY);
  await openUnit(id);
  const detailUrl = page.url();
  await page.reload(); await detail().waitFor(); await settle();
  assert.equal(page.url(), detailUrl);
  assert.ok((await detail().innerText()).includes(previousMonth));
  await page.goBack(); await page.locator('.sq-table-card').waitFor(); await settle();
  assert.equal(page.url(), listUrl);
  assert.equal(await page.locator('.sq-filters label').filter({ hasText: '核算账期' }).locator('select').inputValue(), previousMonth);
  assert.equal(await page.locator('.sq-filters label').filter({ hasText: '请求类型' }).locator('select').inputValue(), 'streaming');
  assert.equal(await page.locator('.sq-pagination select').inputValue(), '5');
  assert.ok(Math.abs(await page.evaluate(() => scrollY) - scroll) <= 2, '浏览器返回应恢复列表滚动');
  assert.equal(await page.locator(`.sq-row-view[data-unit-id="${id}"]`).evaluate(e => e === document.activeElement), true);
  await page.goForward(); await detail().waitFor(); await settle(); assert.equal(page.url(), detailUrl);
  await backToList(); assert.equal(page.url(), listUrl);
  assert.equal(await page.locator(`.sq-row-view[data-unit-id="${id}"]`).evaluate(e => e === document.activeElement), true);
  assert.ok(Math.abs(await page.evaluate(() => scrollY) - scroll) <= 2);
  await page.locator('.sq-pagination select').selectOption('10'); await settle();
  await page.reload(); await page.locator('.sq-table-card').waitFor();
  assert.equal(await page.locator('.sq-pagination select').inputValue(), '10');
  assert.equal(await desktopRows().count(), 6);
});

await check('直接打开详情可返回同账期列表；未知单元和其他客户不可替换展示', async () => {
  await go(`month=${previousMonth}&unit=sq-2`); await detail().waitFor();
  await backToList(); assert.equal(params().get('month'), previousMonth); assert.equal(params().has('unit'), false);
  for (const unit of ['does-not-exist', 'sq-9', 'sq-10']) {
    await go(`month=${month}&unit=${unit}&view=enterprise`);
    await page.getByRole('heading', { name: '无法找到该核算单元', exact: true }).waitFor();
    assert.equal(await detail().count(), 0); assert.equal(await page.locator('.sq-analysis-grid').count(), 0);
    assert.equal(await page.getByRole('button', { name: '导出本单元', exact: true }).isDisabled(), true);
    assert.ok(!(await page.locator('.sq-page').innerText()).includes('#1002'));
    await page.getByRole('button', { name: '重置并返回列表', exact: true }).click(); await settle();
    assert.equal(await desktopRows().count(), 5);
  }
});

await check('失效与畸形账期呈现可恢复空态，不报运行错误或替换月份', async () => {
  await go('unit=sq-1');
  await page.getByRole('heading', { name: '无法找到该核算单元', exact: true }).waitFor();
  assert.equal(await detail().count(), 0, '没有显式账期不能猜测单元所属月份');
  for (const invalid of ['1900-01', '2026-99', 'not-a-month']) {
    await go(`month=${invalid}&unit=sq-1`);
    await page.getByRole('heading', { name: '无法找到该核算单元', exact: true }).waitFor();
    assert.equal(await detail().count(), 0);
    assert.equal(params().get('month'), invalid);
    await page.getByRole('button', { name: '重置并返回列表', exact: true }).click(); await settle();
    assert.equal(await desktopRows().count(), 5);
  }
});

await check('流式趋势与非流式切换不遗留指标；未配置和待核验保持真实状态', async () => {
  await go(); await openUnit('sq-2');
  await page.getByRole('button', { name: 'G2', exact: true }).click(); await settle();
  assert.equal(params().get('metric'), 'g2');
  await backToList(); await openUnit('sq-4');
  assert.equal(params().has('metric'), false);
  assert.ok((await detail().innerText()).includes('非流式'));
  await page.getByRole('heading', { name: '非流式请求不计算首词时延', exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: '成功率', exact: true }).getAttribute('aria-pressed'), 'true');
  for (const name of ['G0', 'G1', 'G2']) assert.equal(await page.getByRole('button', { name, exact: true }).isDisabled(), true);
  assert.equal(await page.locator('.sq-bucket-grid').count(), 0);
  await go(`month=${month}&unit=sq-8`);
  assert.ok((await detail().innerText()).includes('未配置合同目标'));
  assert.equal(await page.locator('.sq-chart-target').count(), 0);
  assert.equal(await page.locator('.sq-bucket-grid .sq-status.is-pass').count(), 0);
  await go(`month=${month}&unit=sq-7`);
  assert.ok((await detail().innerText()).includes('数据待核验'));
  assert.equal(await page.locator('.sq-bucket-grid .sq-status.is-pass').count(), 0);
});

await check('详情导出仅本单元，列表导出筛选全量且保留账期身份与演示口径', async () => {
  await go(`month=${previousMonth}&unit=sq-2`);
  const one = await download('导出本单元');
  assert.equal(one.rows.length, 2); assert.ok(one.name.includes(`${previousMonth}-sq-2`));
  const [header, data] = one.rows;
  const column = name => data[header.indexOf(name)];
  assert.equal(column('账期'), previousMonth);
  assert.equal(column('用户编号'), '1001');
  assert.equal(column('标准模型'), 'claude-sonnet-4-20250514');
  assert.equal(column('合同版本'), `DEMO-U-${previousMonth.replace('-', '.')}`);
  assert.ok(column('核验状态').includes('未最终核验'));
  assert.ok(column('数据来源').includes('前端演示数据，非真实交付记录'));
  assert.equal(params().get('unit'), 'sq-2');
  await go(`month=${previousMonth}&type=streaming&page=2`);
  assert.equal(await desktopRows().count(), 1);
  const all = await download('下载月报');
  assert.equal(all.rows.length, 7, '6 个流式核算单元加表头，不限于第二页 1 条');
  assert.ok(all.rows.slice(1).every(row => row[all.rows[0].indexOf('账期')] === previousMonth));
  assert.ok(all.rows.slice(1).every(row => row[all.rows[0].indexOf('请求类型')] === '流式'));
  assert.equal(params().get('page'), '2');
});

await check('键盘进入及返回；390像素手机列表卡片和详情无横溢且演示控制不遮挡', async () => {
  await go(); await openUnit('sq-2', true);
  const back = page.getByRole('button', { name: '返回交付列表', exact: true });
  await back.focus(); await page.keyboard.press('Enter'); await page.locator('.sq-table-card').waitFor(); await settle();
  assert.equal(await page.locator('.sq-row-view[data-unit-id="sq-2"]').evaluate(e => e === document.activeElement), true);
  await page.setViewportSize({ width: 390, height: 844 }); await go();
  assert.equal(await page.locator('.sq-table-scroll:visible').count(), 0);
  assert.equal(await page.locator('.sq-mobile-unit:visible').count(), 5);
  await noOverflow(); await noDemoOverlap(); await capture('list-mobile');
  await page.getByRole('button', { name: '下一页', exact: true }).click(); await settle();
  const id = await page.locator('.sq-mobile-unit:visible button[data-unit-id]').first().getAttribute('data-unit-id');
  await openUnit(id); await noOverflow(); await noDemoOverlap();
  const backBox = await page.getByRole('button', { name: '返回交付列表', exact: true }).boundingBox();
  assert.ok(backBox.x >= 0 && backBox.x + backBox.width <= 391 && backBox.y >= 0 && backBox.y + backBox.height <= 844);
  await capture('detail-mobile-viewport', false);
  await capture('detail-mobile'); await backToList(); assert.equal(params().get('page'), '2');
});

await check('英文、阿拉伯文深色与手机从右向左详情可读可操作', async () => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => { localStorage.setItem('xy-language', 'en'); localStorage.setItem('xy-theme', 'light'); });
  await go(`month=${month}&unit=sq-2`);
  await page.waitForFunction(() => document.documentElement.dir === 'ltr');
  assert.ok((await detail().innerText()).includes('Contract version'));
  assert.equal(await page.getByRole('button', { name: 'Export this unit', exact: true }).isVisible(), true);
  await noOverflow(); await capture('detail-english');
  await page.evaluate(() => { localStorage.setItem('xy-language', 'ar'); localStorage.setItem('xy-theme', 'dark'); });
  await go(`month=${month}&unit=sq-2`);
  await page.waitForFunction(() => document.documentElement.dir === 'rtl' && document.documentElement.dataset.theme === 'dark');
  await noOverflow(); await noDemoOverlap();
  const transform = await page.locator('.sq-detail-breadcrumb button svg').evaluate(e => getComputedStyle(e).transform);
  assert.notEqual(transform, 'none', '从右向左布局应翻转返回箭头');
  await capture('detail-dark-rtl');
  await page.setViewportSize({ width: 390, height: 844 }); await settle(); await noOverflow(); await noDemoOverlap();
  await capture('detail-mobile-rtl');
  await page.locator('.sq-detail-breadcrumb button').click(); await page.locator('.sq-table-card').waitFor(); await settle();
  assert.equal(await page.locator('.sq-mobile-unit:visible').count(), 5);
  await noOverflow(); await noDemoOverlap(); await capture('list-mobile-rtl');
});

report.completedAt = new Date().toISOString();
await writeFile(`${out}/${prefix}-report.json`, JSON.stringify(report, null, 2));
await browser.close();
console.log(JSON.stringify(report.checks, null, 2));
if (report.errors.length || report.checks.some(check => !check.passed)) { console.log(report.errors); process.exitCode = 1; }
