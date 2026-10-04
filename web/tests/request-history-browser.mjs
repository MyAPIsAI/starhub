import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-27';
await mkdir(out, {recursive:true});
const browser = await chromium.launch({channel:'chrome',headless:true});
const context = await browser.newContext({viewport:{width:1440,height:1000}});
const page = await context.newPage(); page.setDefaultTimeout(8000);
const report = {base,startedAt:new Date().toISOString(),independentBrowserContext:true,checks:[],errors:[],screenshots:[]};
page.on('pageerror', error => report.errors.push(String(error)));
const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
async function go(query='?range=all') { await page.goto(`${base}/console/requests${query}`); await page.locator('.request-history').waitFor(); await settle(); }
async function bind() { await page.evaluate(async () => { const resource = performance.getEntriesByType('resource').filter(item => item.name.includes('/src/lib/product-preview.ts')).at(-1); window.__historyStore = (await import(resource.name)).useProductPreview; }); }
async function capture(name) { await page.screenshot({path:`${out}/${name}.png`,fullPage:!(await page.getByRole('dialog').count())}); report.screenshots.push(`${name}.png`); }
async function check(name, fn) { try { await fn(); report.checks.push({name,passed:true}); console.log('PASS',name); } catch(error) { report.checks.push({name,passed:false,error:String(error)}); console.log('FAIL',name,error.message); await capture(`request-failure-${report.checks.length}`); await page.keyboard.press('Escape'); } }
const ids = () => page.locator('.request-history-table .request-trace-button').allTextContents();
const overflow = async () => assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),'page overflow');
await go(); await bind();
await page.evaluate(() => {
 const store = window.__historyStore; store.getState().resetScenario('enterprise');
 store.getState().recordCall(store.getState().keys[0].id, false);
 const template = store.getState().requests[0];
 if(!template) throw new Error('Enterprise fixture must contain a request');
 store.getState().resetScenario('enterprise');
 // Date/filter fixtures have zero cost so the unchanged ledger remains valid on reload.
 const rows = Array.from({length:125}, (_,i) => ({...template,trace_id:`history-test-${String(i+1).padStart(3,'0')}`,time:new Date(Date.parse('2026-09-26T05:00:00Z')+i*1000).toISOString(),status:i%5===0?'failed':'success',error_code:i%5===0?'MODEL_NOT_AUTHORIZED':null,cost_nano:'0',billing_status:i%5===0?'not_charged':'charged',requested_model:'veyra-auto',actual_model:i%5===0?null:'gpt-4.1'}));
 const edges=['2026-08-31T19:59:59.999Z','2026-08-31T20:00:00.000Z','2026-09-01T19:59:59.999Z','2026-09-01T20:00:00.000Z'].map((time,i)=>({...template,time,trace_id:`date-edge-${i}`,status:'success',error_code:null,cost_nano:'0'}));
 window.__historyFixture=[...rows,...edges]; store.setState({requests:window.__historyFixture});
}); await settle();
await check('桌面搜索限宽且日期、状态位于同一紧凑工具栏', async()=>{
 const search=await page.locator('.request-history-search').boundingBox(); const range=await page.getByLabel('调用时间',{exact:true}).boundingBox();
 assert.ok(search.width>=300&&search.width<=322); assert.ok(Math.abs(search.y-range.y)<5); assert.equal(await page.getByLabel('调用时间',{exact:true}).inputValue(),'all');
 const panel=await page.locator('.request-history-panel').boundingBox(),controls=await page.locator('.request-history .demo-controls').boundingBox();assert.ok(controls.y>=panel.y+panel.height-1,'demo controls must follow panel without overlap');assert.notEqual(await page.locator('.request-history .demo-controls').evaluate(e=>getComputedStyle(e).position),'fixed');
 await overflow(); await capture('requests-desktop');
});
await check('自定义按迪拜整日筛选，边界前后请求不混入', async()=>{
 await page.getByLabel('调用时间',{exact:true}).selectOption('custom');
 await page.getByLabel('开始日期',{exact:true}).fill('2026-09-01'); await page.getByLabel('结束日期',{exact:true}).fill('2026-09-01'); await settle();
 assert.deepEqual(await ids(),['date-edge-2','date-edge-1']); assert.ok((await page.locator('.request-history-summary').innerText()).includes('共 2 条调用')); await capture('requests-custom-dates');
});
await check('倒序与缺失日期保留输入并提示，可直接修复',async()=>{
 await page.getByLabel('开始日期',{exact:true}).fill('2026-09-03'); await settle(); assert.ok((await page.getByRole('alert').innerText()).includes('结束日期不能早于')); assert.equal((await ids()).length,0);
 assert.equal(await page.getByLabel('开始日期',{exact:true}).inputValue(),'2026-09-03');
 await page.getByLabel('开始日期',{exact:true}).fill(''); await settle(); assert.ok((await page.getByRole('alert').innerText()).includes('请选择完整'));
 await page.getByLabel('开始日期',{exact:true}).fill('2026-09-01'); await settle(); assert.equal(await page.getByRole('alert').count(),0); assert.equal((await ids()).length,2);
});
await check('日期、状态与大小写不敏感搜索共同过滤',async()=>{
 await page.getByLabel('开始日期',{exact:true}).fill('2026-09-26'); await page.getByLabel('结束日期',{exact:true}).fill('2026-09-26');
 await page.getByLabel('调用状态',{exact:true}).selectOption('failed'); await page.getByLabel('搜索调用',{exact:true}).fill('authorized'); await settle();
 assert.equal((await ids()).length,20); assert.ok((await page.locator('.request-history-summary').innerText()).includes('共 25 条调用'));
 assert.ok((await page.locator('.request-history-table tbody').innerText()).includes('失败'));
});
await check('筛选无结果与从未调用分开，并可清空恢复全部记录',async()=>{
 await page.getByLabel('搜索调用',{exact:true}).fill('no-such-request'); await settle(); assert.equal((await ids()).length,0); assert.ok(await page.getByText('没有符合筛选条件的调用',{exact:true}).isVisible()); assert.equal(await page.getByText('还没有调用记录',{exact:true}).count(),0);
 await page.getByRole('button',{name:'清空筛选，查看全部',exact:true}).click(); await settle(); assert.equal((await ids()).length,20); assert.ok((await page.locator('.request-history-summary').innerText()).includes('共 129 条调用'));
 await bind(); await page.evaluate(()=>{window.__historyFixture=window.__historyStore.getState().requests;window.__historyStore.setState({requests:[]});}); await settle(); assert.ok(await page.getByText('还没有调用记录',{exact:true}).isVisible()); assert.ok(await page.getByRole('link',{name:'前往接入',exact:true}).isVisible());
 await page.evaluate(()=>window.__historyStore.setState({requests:window.__historyFixture})); await settle();
});
await check('查询参数刷新及浏览器返回恢复日期状态搜索',async()=>{
 await go('?range=custom&start=2026-09-26&end=2026-09-26&status=failed&query=authorized'); const before=await ids(); assert.equal(before.length,20); await page.reload(); await page.locator('.request-history').waitFor();
 assert.deepEqual(await ids(),before); assert.equal(await page.getByLabel('开始日期',{exact:true}).inputValue(),'2026-09-26'); assert.equal(await page.getByLabel('调用状态',{exact:true}).inputValue(),'failed'); assert.equal(await page.getByLabel('搜索调用',{exact:true}).inputValue(),'authorized');
 await page.getByLabel('调用时间',{exact:true}).selectOption('30d'); await settle(); await page.goBack(); await settle(); assert.equal(await page.getByLabel('调用时间',{exact:true}).inputValue(),'custom'); assert.deepEqual(await ids(),before);
});
await check('20 条真实分页可连续访问 129 条，超过旧 100 条限制，刷新保留页码',async()=>{
 await go(); const seen=new Set(); for(let current=1;current<=7;current++){const currentIds=await ids(); assert.equal(currentIds.length,current===7?9:20); currentIds.forEach(id=>{assert.ok(!seen.has(id));seen.add(id);});if(current===2){await page.reload();await page.locator('.request-history').waitFor();assert.deepEqual(await ids(),currentIds);assert.equal(new URL(page.url()).searchParams.get('page'),'2');}if(current<7){await page.getByRole('button',{name:'下一页',exact:true}).click();await settle();}}
 assert.equal(seen.size,129); assert.equal(await page.getByRole('button',{name:'下一页',exact:true}).isDisabled(),true);
});
await check('键盘打开详情并退出后，当前筛选及页码保持',async()=>{
 await go('?range=all&status=success&page=2');const url=page.url(),before=await ids(); const trigger=page.locator('.request-trace-button').first();await trigger.focus();await page.keyboard.press('Enter');await page.getByRole('dialog').waitFor();assert.ok((await page.getByRole('dialog').innerText()).includes(before[0]));await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(page.url(),url);assert.deepEqual(await ids(),before);
});
await check('深链接详情关闭后移除业务参数，保留筛选且不随筛选重新弹出',async()=>{
 await go('?range=all&status=success&page=2');const trace=(await ids())[0];
 await go(`?trace=${encodeURIComponent(trace)}&range=all&status=success&page=2`);await page.getByRole('dialog').waitFor();assert.ok((await page.getByRole('dialog').innerText()).includes(trace));
 await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'hidden'});const params=new URL(page.url()).searchParams;assert.equal(params.has('trace'),false);assert.equal(params.get('range'),'all');assert.equal(params.get('status'),'success');assert.equal(params.get('page'),'2');
 await page.getByLabel('搜索调用',{exact:true}).fill('history-test');await settle();assert.equal(await page.getByRole('dialog').count(),0);await page.getByLabel('调用时间',{exact:true}).selectOption('30d');await settle();assert.equal(await page.getByRole('dialog').count(),0);assert.equal(new URL(page.url()).searchParams.has('trace'),false);
 // Use the existing issue-list link for an actual client-side navigation into a trace URL.
 await page.locator('.xy-sidebar-nav').getByRole('link',{name:'概览',exact:true}).click();await page.locator('.overview-actionable').waitFor();await page.locator('.overview-actionable a[href^="/console/requests?trace="]').first().click();await page.getByRole('dialog').waitFor();assert.ok(new URL(page.url()).searchParams.has('trace'));
 await page.goBack();await settle();await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(new URL(page.url()).searchParams.has('trace'),false);assert.ok(await page.locator('.overview-actionable').isVisible());
});
await check('导出明确独立范围，错误恢复不丢失调用筛选且生成任务',async()=>{
 await go('?range=all&status=failed&query=authorized');const url=page.url();assert.ok((await page.locator('.request-export-action').innerText()).includes('不沿用本页筛选'));await bind();const count=await page.evaluate(()=>window.__historyStore.getState().exports.length);
 await page.getByRole('button',{name:'创建导出任务',exact:true}).click();await page.getByRole('dialog').waitFor();await page.getByLabel('时间跨度（天）',{exact:true}).fill('32');assert.ok((await page.getByRole('dialog').innerText()).includes('31'));await page.getByLabel('时间跨度（天）',{exact:true}).fill('7');await page.getByRole('button',{name:'生成 CSV（表格文件）',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(page.url(),url);assert.equal(await page.evaluate(()=>window.__historyStore.getState().exports.length),count+1);assert.equal(await page.getByLabel('搜索调用',{exact:true}).inputValue(),'authorized');
});
await check('窄屏自定义日期与分页可操作，无页面横向溢出',async()=>{
 await page.setViewportSize({width:390,height:844});await go('?range=custom&start=2026-09-26&end=2026-09-26');await overflow();const bounds=await page.getByLabel('结束日期',{exact:true}).boundingBox();assert.ok(bounds.x>=0&&bounds.x+bounds.width<=390);await page.getByLabel('结束日期',{exact:true}).fill('2026-09-27');await page.getByRole('button',{name:'下一页',exact:true}).click();await settle();assert.equal(new URL(page.url()).searchParams.get('page'),'2');await capture('requests-mobile');
});
await check('英文和深色阿拉伯文从右到左界面可操作',async()=>{
 await page.setViewportSize({width:1440,height:1000});await page.evaluate(()=>{localStorage.setItem('xy-language','en');localStorage.setItem('xy-theme','light');});await go('?range=all');assert.ok(await page.getByLabel('Request date range',{exact:true}).isVisible());await capture('requests-english');
 await page.evaluate(()=>{localStorage.setItem('xy-language','ar');localStorage.setItem('xy-theme','dark');});await go('?range=custom&start=2026-09-26&end=2026-09-27');assert.equal(await page.locator('html').getAttribute('dir'),'rtl');assert.equal(await page.locator('html').getAttribute('data-theme'),'dark');assert.ok(await page.getByLabel('تاريخ البداية',{exact:true}).isVisible());await overflow();await capture('requests-dark-rtl');
 await page.setViewportSize({width:390,height:844});await overflow();await capture('requests-dark-rtl-mobile');
});
report.completedAt=new Date().toISOString();await writeFile(`${out}/request-history-browser-report.json`,JSON.stringify(report,null,2));await browser.close();if(report.errors.length||report.checks.some(check=>!check.passed)){console.log(report.errors);process.exitCode=1;}console.log(`${report.checks.filter(check=>check.passed).length}/${report.checks.length} checks passed`);
