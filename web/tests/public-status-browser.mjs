// All status observations use an isolated database and a local mock upstream.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { createServer } from "node:http";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-public-status-"));
const out = path.join(web, "test-results", "public-status");
await mkdir(out, { recursive: true });
const report = { checks: [], errors: [] };
let backend, vite, browser, upstream;
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
try {
  const binary = path.join(temp, process.platform === "win32" ? "starhub.exe" : "starhub");
  execFileSync("go", ["build", "-o", binary, "."], { cwd: path.resolve(web, ".."), stdio: "pipe" });
  const port = await freePort();
  backend = spawn(binary, [], {
    cwd: temp, windowsHide: true, stdio: "ignore",
    env: {
      ...process.env, STARHUB_CONFIG: "", STARHUB_PORT: String(port), STARHUB_BIND_ADDRESS: "127.0.0.1",
      STARHUB_DB: path.join(temp, "starhub.db"), STARHUB_COOKIE_SECURE: "false",
      STARHUB_ROOT_PASSWORD: "StatusInitial92", STARHUB_ENV: "development",
      STARHUB_CRYPTO_SECRET: "public-status-fixture-secret",
      STARHUB_CREDENTIAL_KEYS: JSON.stringify({ fixture: "11".repeat(32) }), STARHUB_CREDENTIAL_ACTIVE_KEY: "fixture",
      RESOURCE_CATALOG_NOTIFY_URL: "", STARHUB_REGION_PROFILE: "",
    },
  });
  let ready = false;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { ready = (await fetch(`http://127.0.0.1:${port}/api/health`)).ok; } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, "isolated backend did not start");
  vite = await createViteServer({
    root: web, cacheDir: path.join(temp, "vite-cache"), logLevel: "error",
    server: { host: "127.0.0.1", port: await freePort(), proxy: { "/api": `http://127.0.0.1:${port}`, "/v1": `http://127.0.0.1:${port}` } },
  });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  upstream = createServer(async (request, response) => {
    let raw = "";
    for await (const chunk of request) raw += chunk;
    const input = JSON.parse(raw);
    response.setHeader("Content-Type", "application/json");
    setTimeout(() => {
      if (input.model === "status-failed") {
        response.writeHead(500);
        response.end(JSON.stringify({error:{message:"Fixture provider failure",type:"server_error"}}));
      } else {
        response.end(JSON.stringify({id:"fixture-status",model:input.model,choices:[{index:0,message:{role:"assistant",content:"ok"}}],usage:{prompt_tokens:1,completion_tokens:1,total_tokens:2}}));
      }
    }, 20);
  });
  await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  browser = await chromium.launch({ headless: true, ...(process.platform === "win32" ? { channel: "msedge" } : {}) });
  const admin = await browser.newContext();
  const api = async (method, route, data) => {
    const response = await admin.request.fetch(base + route, { method, data });
    assert.equal(response.status(), 200, await response.text());
    const body = await response.json();
    assert.equal(body.success, true);
    return body.data;
  };
  await api("POST", "/api/user/login", {username:"root",password:"StatusInitial92"});
  await api("PUT", "/api/user/self", {current_password:"StatusInitial92",new_password:"StatusChanged93"});
  await api("POST", "/api/user/2fa/skip");
  await api("PUT", "/api/option", {key:"ip_rate_limit",value:"10000"});
  const anonymous = await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:"reduce"});
  await anonymous.addInitScript(() => { if (!localStorage.getItem("xy-language")) localStorage.setItem("xy-language","zh"); });
  const page = await anonymous.newPage();
  page.setDefaultTimeout(30000);
  page.on("pageerror", (error) => report.errors.push(String(error)));
  const requests = [];
  page.on("request", (request) => requests.push(new URL(request.url()).pathname));
  const card = (id) => page.locator(`[data-status-model="${id}"]`);
  const state = () => page.locator("[data-status-state]").getAttribute("data-status-state");
  await page.goto(base + "/status",{waitUntil:"domcontentloaded"});
  await page.getByText("暂无已配置的公开模型。",{exact:true}).waitFor();
  assert.equal(await card("gpt-4.1").count(),0);
  assert.equal(await state(),"unknown");
  report.checks.push("anonymous empty databases show no configured public models and no static catalog or fabricated healthy state");
  const ch = await api("POST","/api/channel",{name:"Public fixture",type:1,key:"fixture-only",models:"status-good",base_url:`http://127.0.0.1:${upstream.address().port}`});
  const token = await api("POST","/api/token",{name:"status-fixture",unlimited_quota:true});
  const relay = (model) => admin.request.post(base + "/v1/chat/completions",{headers:{Authorization:`Bearer ${token.key}`},data:{model,messages:[{role:"user",content:"hello"}],max_tokens:8}});
  assert.equal((await relay("status-good")).status(),200);
  await api("POST","/api/incident",{title:"Stored info notice",description:"Real database notice",severity:"info",component:"Other"});
  await page.reload({waitUntil:"domcontentloaded"});
  await card("status-good").waitFor();
  await page.getByText("Stored info notice",{exact:true}).waitFor();
  await page.waitForFunction(() => document.querySelector("[data-status-state]")?.dataset.statusState === "operational");
  assert.equal(await card("status-good").locator('[data-metric="recent-rate"]').innerText(),"100.00%");
  const incident = await api("POST","/api/incident",{title:"Confirmed outage notice",description:"Critical fixture",severity:"critical",component:"Other"});
  await page.reload({waitUntil:"domcontentloaded"});
  await page.waitForFunction(() => document.querySelector("[data-status-state]")?.dataset.statusState === "degraded");
  await api("PUT",`/api/channel/${ch.id}/status`,{enabled:false});
  await page.reload({waitUntil:"domcontentloaded"});
  await page.waitForFunction(() => document.querySelector("[data-status-state]")?.dataset.statusState === "unavailable");
  await api("PUT",`/api/channel/${ch.id}/status`,{enabled:true});
  await api("PUT",`/api/incident/${incident.id}`,{status:"resolved",message:"Fixture restored"});
  await page.reload({waitUntil:"domcontentloaded"});
  await page.waitForFunction(() => document.querySelector("[data-status-state]")?.dataset.statusState === "operational");
  report.checks.push("real successful relay logs drive public health, info notices do not invent outages, critical notices affect the banner until resolved and cannot downgrade a known total resource outage");
  await api("PUT","/api/channel",{id:ch.id,name:"Public fixture",key:"",models:"status-good,status-failed,status-unknown,status-closed"});
  await api("PUT",`/api/channel/${ch.id}/resource-status`,{model:"status-closed",enabled:false});
  await api("POST","/api/channel",{name:"Private fixture",type:1,key:"private-fixture-key",models:"private-fixture-model",group:"private"});
  assert.equal((await relay("status-failed")).status(),502);
  const view = await api("GET","/api/status/overview");
  assert.equal(view.models.length,4);
  assert.equal(view.models.find((model) => model.id === "status-failed").recent.success_rate,0);
  assert.equal(view.models.find((model) => model.id === "status-unknown").recent.success_rate,null);
  await page.reload({waitUntil:"domcontentloaded"});
  await card("status-failed").waitFor();
  assert.equal(await card("status-failed").locator('[data-metric="recent-rate"]').innerText(),"0.00%");
  assert.equal(await card("status-unknown").locator('[data-metric="recent-rate"]').innerText(),"—");
  assert.equal(await card("status-closed").locator("[data-state]").getAttribute("data-state"),"unavailable");
  assert.equal(await card("private-fixture-model").count(),0);
  assert.equal(await card("status-good").locator(".public-status-bars span").count(),90);
  assert.equal(await card("status-good").locator(".public-status-bars span[data-day-state=unknown]").count(),89);
  assert.ok(!(await page.locator("main").innerText()).includes("99.98%"));
  assert.ok(!requests.includes("/api/service-quality"));
  report.checks.push("real failed requests remain 0%, no samples remain null, closed resources stay visible, private models stay hidden and actual UTC daily history replaces synthetic bars");
  await page.screenshot({path:path.join(out,"overview-zh-light-1440.png"),fullPage:true});
  await page.evaluate(async () => {
    const { useProductPreview } = await import("/src/lib/product-preview.ts");
    useProductPreview.getState().demoSetUpstreamOutage(true);
  });
  assert.equal(await card("status-good").locator("[data-state]").getAttribute("data-state"),"operational");
  const endpoint = "**/api/status/overview";
  let mode = "hold", release;
  const gate = new Promise((resolve) => {release=resolve;});
  await page.route(endpoint,async (route) => {
    if (mode === "hold") {await gate; return route.continue();}
    if (mode === "error") return route.fulfill({status:503,json:{success:false,message:"Fixture status unavailable"}});
    if (mode === "malformed") return route.fulfill({status:200,json:{success:true,data:{}}});
    await route.continue();
  });
  await page.reload({waitUntil:"domcontentloaded"});
  await page.getByText("正在读取模型指标…",{exact:true}).waitFor();
  assert.equal(await page.locator("[data-status-model]").count(),0);
  assert.equal(await state(),"unknown");
  release();
  await card("status-good").waitFor();
  mode = "error";
  await page.reload({waitUntil:"domcontentloaded"});
  const metricError = page.locator("[data-status-models]").getByRole("alert");
  await metricError.waitFor();
  assert.equal(await state(),"unknown");
  assert.equal(await page.locator("[data-status-model]").count(),0);
  await page.getByText("Stored info notice",{exact:true}).waitFor();
  mode = "success";
  await metricError.getByRole("button").focus();
  await page.keyboard.press("Enter");
  await card("status-good").waitFor();
  mode = "malformed";
  await page.reload({waitUntil:"domcontentloaded"});
  await metricError.waitFor();
  assert.equal(await state(),"unknown");
  await page.unroute(endpoint);
  await metricError.getByRole("button").click();
  await card("status-good").waitFor();
  report.checks.push("loading, unavailable and malformed public responses never restore demo or stale healthy cards, keep real notices independent, and recover through keyboard retry; local demo outage settings do not affect real model health");
  for (const language of ["zh","en","ar"]) for (const theme of ["light","dark"]) for (const width of [1440,320]) {
    await page.evaluate(({language,theme}) => {localStorage.setItem("xy-language",language);localStorage.setItem("xy-theme",theme);},{language,theme});
    await page.setViewportSize({width,height:1000});
    await page.reload({waitUntil:"domcontentloaded"});
    await card("status-good").waitFor();
    const summary = card("status-good").locator("summary");
    await summary.focus();
    await page.keyboard.press("Enter");
    await card("status-good").locator("table").waitFor();
    assert.equal(await card("status-good").locator("tbody tr").count(),90);
    await page.keyboard.press("Tab");
    assert.equal(await card("status-good").locator(".public-status-history").evaluate((node) => node === document.activeElement),true);
    await page.keyboard.press("End");
    await page.waitForFunction(() => document.activeElement?.classList.contains("public-status-history") && document.activeElement.scrollTop > 0);
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    assert.deepEqual((await new AxeBuilder({page}).include("main").withTags(["wcag2a","wcag2aa"]).analyze()).violations,[]);
    await page.screenshot({path:path.join(out,`${language}-${theme}-${width}.png`)});
  }
  report.checks.push("three languages, both themes, desktop/320px, keyboard daily-history tables and accessibility checks pass without horizontal page overflow");
  assert.deepEqual(report.errors,[]);
  console.log(JSON.stringify(report,null,2));
} catch (error) {
  report.failure = String(error.stack || error);
  const page = browser?.contexts().flatMap((context) => context.pages()).at(-1);
  if (page) await page.screenshot({path:path.join(out,"failure.png")});
  throw error;
} finally {
  await writeFile(path.join(out,"report.json"),JSON.stringify(report,null,2));
  await browser?.close();
  await vite?.close();
  backend?.kill();
  if (upstream) await new Promise((resolve) => upstream.close(resolve));
}
