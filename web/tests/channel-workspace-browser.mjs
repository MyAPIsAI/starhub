import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-26-round4';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage();
page.setDefaultTimeout(8000);
const report = { startedAt: new Date().toISOString(), base, independentBrowserContext: true, checks: [], errors: [], screenshots: [] };
page.on('pageerror', e => report.errors.push(String(e)));
page.on('dialog', d => d.accept());
const config = () => page.evaluate(() => JSON.parse(localStorage.getItem('veyra-platform-pricing-v1')));
const rows = () => page.locator('.channel-resource-table:visible tbody tr[data-resource-id]');
const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
const rowIds = async () => { await settle(); return rows().evaluateAll(elements => elements.map(e => e.dataset.resourceId)); };
const directory = view => page.getByRole('navigation', { name: view === 'model' ? '标准模型目录' : '渠道目录', exact: true });
const choose = async (view, id) => { await directory(view).locator(`button[data-group-id="${id}"]`).click(); await settle(); };
const mode = async view => { await page.getByRole('button', { name: view === 'model' ? '按标准模型' : '按渠道', exact: true }).click(); await settle(); };
async function go(search = '') { await page.goto(`${base}/console/channels${search}`); await page.locator('.channel-workspace').waitFor(); await settle(); }
async function capture(name) {
  await page.screenshot({ path: `${out}/${name}.png`, fullPage: !(await page.getByRole('dialog').count()) });
  report.screenshots.push(`${name}.png`);
}
async function check(name, fn) {
  try { await fn(); report.checks.push({ name, passed: true }); console.log('PASS', name); }
  catch (error) {
    report.checks.push({ name, passed: false, error: String(error) }); console.log('FAIL', name, error.message);
    await capture(`channel-failure-${report.checks.length}`);
    await writeFile(`${out}/channel-failure-${report.checks.length}.html`, await page.content());
    await page.keyboard.press('Escape'); await settle();
  }
}
async function openSource() { await page.getByRole('button', { name: '同步资源', exact: true }).click(); await settle(); }
async function sync(value) {
  await openSource(); await page.getByLabel('同步演示场景', { exact: true }).selectOption(value); await settle();
  await page.getByRole('button', { name: '预览模拟同步', exact: true }).click(); await settle();
  if (value !== 'fail') {
    await page.getByRole('button', { name: '确认同步快照', exact: true }).click(); await settle();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
  } else { await page.keyboard.press('Escape'); await settle(); await page.getByRole('dialog').waitFor({ state: 'hidden' }); }
}
async function importSnapshot(snapshot) {
  await openSource(); await page.getByRole('button', { name: '导入快照数据', exact: true }).click(); await settle();
  await page.getByLabel('资源快照数据', { exact: true }).fill(JSON.stringify(snapshot, null, 2)); await settle();
  await page.getByRole('button', { name: '校验并预览同步', exact: true }).click(); await settle();
  await page.getByRole('button', { name: '确认同步快照', exact: true }).click(); await settle();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
}
const noDemoOverlap = async () => {
  const workspace = await page.locator('.channel-workspace').boundingBox();
  const controls = await page.locator('.demo-controls').boundingBox();
  assert.ok(controls && workspace && controls.y >= workspace.y + workspace.height - 1, '演示操作应在工作区下方，不得悬浮遮挡资源操作或分页');
  assert.notEqual(await page.locator('.demo-controls').evaluate(e => getComputedStyle(e).position), 'fixed');
};
const noOverflow = async () => assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), '页面不得横向溢出');
await page.goto(base + '/console');
await page.locator('.xy-main').waitFor();
await page.evaluate(async () => {
  const load = async file => import(performance.getEntriesByType('resource').filter(r => r.name.includes(`/src/lib/${file}.ts`)).at(-1).name);
  (await load('product-preview')).useProductPreview.getState().resetScenario('enterprise');
  (await load('system-access')).useSystemAccess.getState().setRole('superAdmin');
});

await check('左右主从工作区只有一个资源表；来源同步降为次级入口', async () => {
  await go();
  const list = await directory('channel').boundingBox(), table = await page.locator('.channel-resource-table:visible').boundingBox();
  assert.ok(list.x + list.width <= table.x + 2, '目录应位于资源表左侧');
  assert.equal(await page.locator('.channel-resource-table:visible').count(), 1);
  assert.equal(await page.getByLabel('同步演示场景').isVisible(), false);
  assert.equal(await page.getByRole('button', { name: '按渠道', exact: true }).getAttribute('aria-pressed'), 'true');
  assert.equal((await rowIds()).length, 8);
  await noOverflow(); await noDemoOverlap(); await capture('channels-workspace-desktop');
});
await check('资源分页连续、刷新保留当前页，切换目录后回到第一页', async () => {
  const firstIds = await rowIds();
  await page.getByRole('button', { name: '下一页资源', exact: true }).click(); await settle();
  const secondIds = await rowIds();
  assert.equal(secondIds.length, 1); assert.equal(new Set([...firstIds, ...secondIds]).size, 9);
  assert.equal(new URL(page.url()).searchParams.get('page'), '2');
  await page.reload(); await page.locator('.channel-workspace').waitFor(); assert.deepEqual(await rowIds(), secondIds);
  await choose('channel', 'channel-1'); assert.equal(new URL(page.url()).searchParams.has('page'), false);
  assert.equal((await rowIds()).length, 7);
});
await check('渠道目录选择、目录搜索与资源搜索互相独立', async () => {
  await choose('channel', 'channel-2');
  assert.deepEqual(new Set(await rowIds()), new Set(['resource-1-b', 'resource-2-b']));
  assert.ok(await directory('channel').locator('[data-group-id="channel-2"]').getAttribute('aria-current'));
  await page.getByLabel('搜索渠道', { exact: true }).fill('供应商 1'); await settle();
  assert.equal(await directory('channel').locator('[data-group-id="channel-1"]').count(), 1);
  assert.equal(await directory('channel').locator('[data-group-id="channel-2"]').count(), 0);
  assert.deepEqual(new Set(await rowIds()), new Set(['resource-1-b', 'resource-2-b']), '目录搜索不应偷偷切换右侧作用范围');
  await page.getByLabel('搜索渠道', { exact: true }).fill(''); await settle();
  await page.getByLabel('搜索资源实例', { exact: true }).fill('gpt-4.1'); await settle();
  assert.deepEqual(await rowIds(), ['resource-1-b']);
  await page.getByLabel('搜索资源实例', { exact: true }).fill('不存在的资源'); await settle();
  assert.equal((await rowIds()).length, 0);
  assert.equal(new URL(page.url()).searchParams.get('channel'), 'channel-2');
  await page.getByRole('button', { name: '清空筛选', exact: true }).first().click(); await settle();
  assert.deepEqual(new Set(await rowIds()), new Set(['resource-1-b', 'resource-2-b']));
});
await check('按标准模型聚合同一模型跨渠道实例；视角分别记忆选中项', async () => {
  await mode('model'); await choose('model', '__all__');
  const allHeadings = await page.locator('.channel-resource-table:visible thead').innerText();
  assert.ok(allHeadings.includes('标准模型') && allHeadings.includes('来源渠道'), '全部模型应同时展示模型身份与来源渠道');
  await choose('model', 'gpt-4.1');
  assert.deepEqual(new Set(await rowIds()), new Set(['resource-1-a', 'resource-1-b']));
  const headings = await page.locator('.channel-resource-table:visible thead').innerText();
  assert.ok(headings.includes('来源渠道')); assert.ok(!headings.includes('标准模型'));
  await mode('channel'); assert.equal(new URL(page.url()).searchParams.get('channel'), 'channel-2');
  assert.deepEqual(new Set(await rowIds()), new Set(['resource-1-b', 'resource-2-b']));
  await mode('model'); assert.equal(new URL(page.url()).searchParams.get('model'), 'gpt-4.1');
  assert.deepEqual(new Set(await rowIds()), new Set(['resource-1-a', 'resource-1-b']));
  await capture('channels-by-model');
});
await check('分组、状态、搜索筛选与刷新、浏览器返回保持一致', async () => {
  await mode('channel'); await choose('channel', '__all__');
  await page.getByLabel('按投放分组筛选', { exact: true }).selectOption('lite'); await settle();
  const c = await config();
  assert.deepEqual(new Set(await rowIds()), new Set(c.resources.instances.filter(r => c.placements[r.id].groupIds.includes('lite')).map(r => r.id)));
  await page.getByLabel('按资源状态筛选', { exact: true }).selectOption('ready'); await settle();
  await page.getByLabel('搜索资源实例', { exact: true }).fill('claude'); await settle();
  const url = page.url(), ids = await rowIds();
  await page.reload(); await page.locator('.channel-workspace').waitFor();
  assert.equal(page.url(), url); assert.deepEqual(await rowIds(), ids);
  assert.equal(await page.getByLabel('按投放分组筛选').inputValue(), 'lite');
  assert.equal(await page.getByLabel('按资源状态筛选').inputValue(), 'ready');
  assert.equal(await page.getByLabel('搜索资源实例').inputValue(), 'claude');
  await choose('channel', 'channel-1'); await page.goBack();
  await page.locator('.channel-workspace').waitFor(); assert.equal(page.url(), url);
  assert.deepEqual(await rowIds(), ids);
  await page.getByRole('button', { name: '清空筛选', exact: true }).first().click(); await settle();
});
await check('未映射入口可发现新资源；同步取消与失败均保留现有状态', async () => {
  await go(); const previous = await config();
  await openSource(); await page.getByRole('button', { name: '预览模拟同步', exact: true }).click(); await settle();
  await page.getByRole('button', { name: '取消', exact: true }).click(); await settle();
  assert.equal((await config()).resources.sequence, previous.resources.sequence);
  await sync('fail'); assert.equal((await config()).resources.sequence, previous.resources.sequence);
  await sync('add');
  const c = await config(), id = `resource-import-${c.resources.sequence}`;
  assert.deepEqual(c.placements[id], { modelId: null, groupIds: [], enabled: false });
  await mode('model'); await choose('model', '__unmapped__'); assert.deepEqual(await rowIds(), [id]);
  await page.getByLabel('按资源状态筛选').selectOption('unmapped'); await settle(); assert.deepEqual(await rowIds(), [id]);
  await capture('channels-unmapped');
});
await check('真实映射与投放保存后两视角立即一致，取消不改数据', async () => {
  await go('?view=channel&channel=channel-1');
  const id = 'resource-1-a', before = (await config()).placements[id];
  const row = page.locator(`.channel-resource-table:visible tr[data-resource-id="${id}"]`);
  await row.getByRole('button', { name: '映射与投放', exact: true }).click(); await settle();
  await page.getByLabel('启用本地投放', { exact: true }).uncheck();
  await page.getByRole('button', { name: '取消', exact: true }).click(); await settle();
  assert.deepEqual((await config()).placements[id], before);
  await row.getByRole('button', { name: '映射与投放', exact: true }).click(); await settle();
  await page.getByLabel('启用本地投放', { exact: true }).uncheck();
  await page.getByRole('button', { name: '预览投放影响', exact: true }).click(); await settle();
  await page.getByRole('button', { name: '确认发布投放', exact: true }).click(); await settle();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  assert.equal((await config()).placements[id].enabled, false);
  await page.getByLabel('按资源状态筛选').selectOption('paused'); await settle(); assert.deepEqual(await rowIds(), [id]);
  await page.getByRole('button', { name: '清空筛选', exact: true }).first().click(); await settle();
  await mode('model'); await choose('model', 'gpt-4.1');
  await page.getByLabel('按资源状态筛选').selectOption('paused'); await settle(); assert.deepEqual(await rowIds(), [id]);
  await page.reload(); await page.locator('.channel-workspace').waitFor(); assert.deepEqual(await rowIds(), [id]);
});
await check('来源故障可筛选、同步恢复后列表状态更新', async () => {
  await go(); await sync('outage');
  await page.getByLabel('按资源状态筛选').selectOption('unavailable'); await settle();
  const firstIds = await rowIds();
  await page.getByRole('button', { name: '下一页资源', exact: true }).click(); await settle();
  assert.equal(new Set([...firstIds, ...(await rowIds())]).size, (await config()).resources.instances.length);
  await capture('channels-source-unavailable');
  await sync('restore'); assert.equal((await rowIds()).length, 0);
  await page.getByRole('button', { name: '清空筛选', exact: true }).first().click(); await settle();
});
await check('来源已移除实例有独立入口并可清除本地投放引用', async () => {
  await go(); const c = await config(), id = 'resource-2-b';
  const source = structuredClone(c.resources); source.sequence++; source.id = `resource-test-${source.sequence}`;
  source.publishedAt = new Date().toISOString(); source.instances = source.instances.filter(r => r.id !== id);
  await importSnapshot(source);
  assert.ok((await config()).placements[id]);
  const trigger = page.locator('button:visible').filter({ hasText: /已离开来源|已移除|移出来源/ }).first();
  await trigger.click(); await settle();
  const cleanup = page.getByRole('button', { name: '检查并清除投放', exact: true }).first();
  await cleanup.click(); await settle();
  assert.ok((await page.getByRole('dialog').innerText()).includes(id));
  await page.getByRole('button', { name: '预览投放影响', exact: true }).click(); await settle();
  await page.getByRole('button', { name: '确认发布投放', exact: true }).click(); await settle();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  assert.equal((await config()).placements[id], undefined);
});
await check('键盘可选择视角和目录，关闭编辑后焦点回到操作按钮', async () => {
  await go(); const switcher = page.getByRole('button', { name: '按标准模型', exact: true });
  await switcher.focus(); await page.keyboard.press('Enter'); await settle();
  assert.equal(await switcher.getAttribute('aria-pressed'), 'true');
  const item = directory('model').locator('button[data-group-id="gpt-4.1"]');
  await item.focus(); await page.keyboard.press('Enter'); await settle();
  assert.deepEqual(new Set(await rowIds()), new Set(['resource-1-a', 'resource-1-b']));
  const edit = rows().first().getByRole('button', { name: '映射与投放', exact: true });
  await edit.focus(); await page.keyboard.press('Enter'); await settle(); await page.getByRole('dialog').waitFor();
  await page.keyboard.press('Escape'); await settle(); await page.getByRole('dialog').waitFor({ state: 'hidden' });
  assert.equal(await edit.evaluate(e => document.activeElement === e), true);
});
await check('390像素窄屏使用目录下拉和资源卡片；编辑滚动与退出有效', async () => {
  await page.setViewportSize({ width: 390, height: 844 }); await go('?view=channel&channel=channel-2');
  await noOverflow(); await noDemoOverlap();
  assert.equal(await page.getByLabel('选择渠道', { exact: true }).isVisible(), true);
  assert.equal(await page.locator('.channel-resource-table:visible').count(), 0);
  assert.equal(await page.locator('.channel-resource-card:visible').count(), 1);
  await page.getByLabel('选择渠道', { exact: true }).selectOption('channel-1'); await settle();
  assert.equal(await page.locator('.channel-resource-card:visible').count(), 8);
  await capture('channels-workspace-mobile');
  await page.locator('.channel-resource-card:visible').first().getByRole('button', { name: '映射与投放', exact: true }).click(); await settle();
  const rect = await page.getByRole('dialog').boundingBox();
  assert.ok(rect.x >= 0 && rect.x + rect.width <= 391 && rect.y + rect.height <= 845);
  await page.getByLabel('启用本地投放', { exact: true }).scrollIntoViewIfNeeded();
  await capture('channels-placement-mobile');
  await page.keyboard.press('Escape'); await settle(); await page.getByRole('dialog').waitFor({ state: 'hidden' });
  await mode('model'); assert.equal(await page.getByLabel('选择标准模型', { exact: true }).isVisible(), true);
  await page.getByLabel('选择标准模型', { exact: true }).selectOption('gpt-4.1'); await settle();
  assert.equal(await page.locator('.channel-resource-card:visible').count(), 2); await noOverflow();
});
await check('深色与从右向左布局无溢出，目录和资源仍可选择', async () => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => { localStorage.setItem('xy-theme', 'dark'); localStorage.setItem('xy-language', 'ar'); });
  await go('?view=model&model=gpt-4.1');
  await page.waitForFunction(() => document.documentElement.dir === 'rtl' && document.documentElement.dataset.theme === 'dark');
  await noOverflow();
  const nav = await page.locator('.channel-index nav').boundingBox(), table = await page.locator('.channel-resource-table:visible').boundingBox();
  assert.ok(nav.x >= table.x + table.width - 2, '从右向左时目录应位于资源表右側');
  await capture('channels-workspace-dark-rtl');
  await page.setViewportSize({ width: 390, height: 844 }); await noOverflow(); await capture('channels-workspace-mobile-rtl');
});

report.completedAt = new Date().toISOString();
await writeFile(`${out}/channel-workspace-report.json`, JSON.stringify(report, null, 2));
await browser.close();
console.log(JSON.stringify(report.checks, null, 2));
if (report.errors.length || report.checks.some(c => !c.passed)) { console.log(report.errors); process.exitCode = 1; }
