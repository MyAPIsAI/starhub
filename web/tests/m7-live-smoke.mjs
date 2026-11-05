import { chromium } from "playwright";
import { mkdir,writeFile } from "node:fs/promises";
import path from "node:path";
import assert from "node:assert/strict";
const out=path.resolve("../output/adm-m7-review-20261104");
await mkdir(out,{recursive:true});
const checks=[];
for(const port of [3000,5173])for(const route of ["/api/admin/models","/api/admin/region-profile","/api/admin/reconciliation","/api/admin/system"]){
  const response=await fetch(`http://127.0.0.1:${port}${route}`);
  assert.equal(response.status,401,`${port} ${route}`);
  checks.push({port,route,status:response.status,passed:true});
}
const browser=await chromium.launch({channel:"chrome",headless:true});
try {
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addInitScript(()=>{localStorage.setItem("xy-language","zh");localStorage.setItem("xy-theme","dark");});
  const page=await context.newPage(),errors=[];
  page.on("pageerror",error=>errors.push(error.message));
  await page.goto("http://127.0.0.1:5173/admin");
  await page.getByRole("heading",{name:"欢迎回来",exact:true}).waitFor();
  assert.equal(await page.locator("#auth-account").inputValue(),"");
  assert.equal(await page.locator("#auth-password").inputValue(),"");
  await page.screenshot({path:path.join(out,"live-login-ready.png"),fullPage:true});
  assert.deepEqual(errors,[]);
  await writeFile(path.join(out,"live-smoke.json"),JSON.stringify({passed:true,endpointChecks:checks,pageErrors:errors,anonymousBrowser:true,realCredentialsUsed:false},null,2));
  console.log(JSON.stringify({passed:true,endpointChecks:checks.length,anonymousLoginReady:true}));
} finally {await browser.close();}
