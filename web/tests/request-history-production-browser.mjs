import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
const base=process.env.VEYRA_PRODUCTION_URL||'http://127.0.0.1:5197';
const out=process.env.VEYRA_EVIDENCE_DIR||'docs/verification-2026-09-27';await mkdir(out,{recursive:true});
const browser=await chromium.launch({channel:'chrome',headless:true});const context=await browser.newContext({viewport:{width:1440,height:1000}});const page=await context.newPage();page.setDefaultTimeout(8000);
const report={base,startedAt:new Date().toISOString(),independentBrowserContext:true,usesBuiltAssetsOnly:true,checks:[],errors:[],screenshots:[]};page.on('pageerror',error=>report.errors.push(String(error)));
const settle=()=>page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
const snapshot=()=>page.evaluate(()=>JSON.parse(localStorage.getItem('veyra-billing-demo-v1')));
const ids=()=>page.locator('.request-history-table .request-trace-button').allTextContents();
async function go(query='?range=today'){await page.goto(`${base}/console/requests${query}`);await page.locator('.request-history').waitFor();await settle();}
async function capture(name){await page.screenshot({path:`${out}/${name}.png`,fullPage:!(await page.getByRole('dialog').count())});report.screenshots.push(`${name}.png`);}
async function check(name,fn){try{await fn();report.checks.push({name,passed:true});console.log('PASS',name);}catch(error){report.checks.push({name,passed:false,error:String(error)});console.log('FAIL',name,error.message);await capture(`production-requests-failure-${report.checks.length}`);await page.keyboard.press('Escape');}}
let call,date;
await check('生产构建通过界面完成模拟调用，今日列表可查且搜索限宽',async()=>{
 await page.goto(`${base}/console/playground`);await page.locator('#verify-prompt').waitFor();await page.locator('#verify-prompt').fill('请用一句话确认这是调用明细的本地模拟验证。');await page.locator('#verify-model').selectOption('gpt-4.1');await page.getByRole('button',{name:'发送模拟请求',exact:true}).click();
 await page.waitForFunction(()=>JSON.parse(localStorage.getItem('veyra-billing-demo-v1'))?.liveRequests?.length>0);call=(await snapshot()).liveRequests[0];assert.equal(call.status,'success');date=new Date(Date.parse(call.time)+4*3600000).toISOString().slice(0,10);
 await go();assert.deepEqual(await ids(),[call.trace_id]);const search=await page.locator('.request-history-search').boundingBox();assert.ok(search.width>=300&&search.width<=322);await capture('production-requests-desktop');
});
await check('生产自定义日期含完整当天，倒序可原位恢复',async()=>{
 await page.getByLabel('调用时间',{exact:true}).selectOption('custom');await page.getByLabel('开始日期',{exact:true}).fill(date);await page.getByLabel('结束日期',{exact:true}).fill(date);await settle();assert.deepEqual(await ids(),[call.trace_id]);
 const next=new Date(Date.parse(`${date}T00:00:00Z`)+86400000).toISOString().slice(0,10);await page.getByLabel('开始日期',{exact:true}).fill(next);await settle();assert.ok((await page.getByRole('alert').innerText()).includes('结束日期不能早于'));assert.equal((await ids()).length,0);await page.getByLabel('开始日期',{exact:true}).fill(date);await settle();assert.deepEqual(await ids(),[call.trace_id]);assert.equal(await page.getByRole('alert').count(),0);
});
await check('生产搜索状态联合过滤，刷新和返回保存筛选',async()=>{
 await page.getByLabel('搜索调用',{exact:true}).fill(call.trace_id);await page.getByLabel('调用状态',{exact:true}).selectOption('failed');await settle();assert.equal((await ids()).length,0);assert.ok(await page.getByText('没有符合筛选条件的调用',{exact:true}).isVisible());
 await page.getByLabel('调用状态',{exact:true}).selectOption('success');await settle();assert.deepEqual(await ids(),[call.trace_id]);await page.reload();await page.locator('.request-history').waitFor();assert.equal(await page.getByLabel('搜索调用',{exact:true}).inputValue(),call.trace_id);assert.equal(await page.getByLabel('开始日期',{exact:true}).inputValue(),date);assert.deepEqual(await ids(),[call.trace_id]);await page.getByLabel('调用时间',{exact:true}).selectOption('all');await settle();await page.goBack();await settle();assert.equal(await page.getByLabel('调用时间',{exact:true}).inputValue(),'custom');assert.deepEqual(await ids(),[call.trace_id]);
});
await check('生产深链接详情关闭后保留筛选，后续操作不重开',async()=>{
 await go(`?trace=${encodeURIComponent(call.trace_id)}&range=all&status=success`);await page.getByRole('dialog').waitFor();assert.ok((await page.getByRole('dialog').innerText()).includes(call.trace_id));await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(new URL(page.url()).searchParams.has('trace'),false);assert.equal(new URL(page.url()).searchParams.get('status'),'success');assert.equal(new URL(page.url()).searchParams.get('range'),'all');await page.getByLabel('搜索调用',{exact:true}).fill(call.trace_id);await settle();assert.equal(await page.getByRole('dialog').count(),0);assert.deepEqual(await ids(),[call.trace_id]);
});
await check('生产导出明确独立时区范围，31天错误可修复且保留调用筛选',async()=>{
 assert.ok((await page.locator('.request-export-action').innerText()).includes('不沿用本页筛选'));const url=page.url();await page.getByRole('button',{name:'创建导出任务',exact:true}).click();await page.getByRole('dialog').waitFor();assert.ok((await page.getByRole('dialog').innerText()).includes('协调世界时'));await page.getByLabel('时间跨度（天）',{exact:true}).fill('32');assert.ok((await page.getByRole('dialog').innerText()).includes('31'));await page.getByLabel('时间跨度（天）',{exact:true}).fill('7');await page.getByRole('button',{name:'生成 CSV（表格文件）',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(page.url(),url);assert.deepEqual(await ids(),[call.trace_id]);
});
await check('生产手机日期可操作，页面无横向溢出且演示控制不遮挡',async()=>{
 await page.setViewportSize({width:390,height:844});await go(`?range=custom&start=${date}&end=${date}`);assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));const end=await page.getByLabel('结束日期',{exact:true}).boundingBox();assert.ok(end.x>=0&&end.x+end.width<=390);const panel=await page.locator('.request-history-panel').boundingBox(),controls=await page.locator('.request-history .demo-controls').boundingBox();assert.ok(controls.y>=panel.y+panel.height-1);assert.notEqual(await page.locator('.request-history .demo-controls').evaluate(e=>getComputedStyle(e).position),'fixed');assert.deepEqual(await ids(),[call.trace_id]);await capture('production-requests-mobile');
});
await check('手机空态和恢复按钮始终在屏幕内，英文与阿语无溢出', async()=>{
 for(const [lang,width] of [['zh',390],['ar',390],['en',320]]) {
  await page.evaluate(lang=>{localStorage.setItem('xy-language',lang);localStorage.setItem('xy-theme',lang==='zh'?'light':'dark');},lang);
  await page.setViewportSize({width,height:844}); await go('?range=all&query=no-such-request-mobile-check');
  const empty=await page.locator('.request-empty').boundingBox();assert.ok(empty.x>=0&&empty.x+empty.width<=width);
  const action=page.locator('.request-empty .button');const box=await action.boundingBox();assert.ok(box.x>=0&&box.x+box.width<=width);
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
  await capture(`production-requests-${lang}-empty-mobile`);
  await action.click();await settle();assert.deepEqual(await ids(),[call.trace_id]);
 }
});
assert.equal(await page.evaluate(()=>performance.getEntriesByType('resource').some(resource=>resource.name.includes('/src/'))),false);
report.completedAt=new Date().toISOString();await writeFile(`${out}/production-requests-report.json`,JSON.stringify(report,null,2));await browser.close();if(report.errors.length||report.checks.some(check=>!check.passed)){console.log(report.errors);process.exitCode=1;}console.log(`${report.checks.filter(check=>check.passed).length}/${report.checks.length} checks passed`);
