// E2E browser test runner — starts a Go backend with isolated DB,
// launches a Vite dev server that proxies to it, and runs browser tests
// against the real frontend+backend stack. Used by CI (npm run test:e2e).
//
// Phases:
//   1. Build and start Go backend (isolated DB + random port)
//   2. Start Vite dev server (proxies /api and /v1 to Go backend)
//   3. Run browser checks against the full stack
//   4. Cleanup (kill Go process, remove temp DB)
import { createServer } from "vite";
import { chromium } from "playwright";
import { mkdir, writeFile, mkdtemp, rm } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { execSync, spawn } from "node:child_process";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const root = fileURLToPath(new URL("../", import.meta.url));
const projectRoot = path.resolve(root, "..");
const outDir = path.join(root, "playwright-report");

// --- Utility: find a free port ---
function findFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      const port = server.address().port;
      server.close(() => resolve(port));
    });
    server.on("error", reject);
  });
}

// --- Utility: wait for a URL to return 200 ---
async function waitForReady(url, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const resp = await fetch(url);
      if (resp.ok) return true;
    } catch { /* not ready yet */ }
    await new Promise((r) => setTimeout(r, 500));
  }
  return false;
}

// --- Phase 1: Build and start Go backend ---
console.log("E2E: Building Go backend...");
const binaryName = process.platform === "win32" ? "starhub-e2e.exe" : "starhub-e2e";
const binaryPath = path.join(projectRoot, binaryName);
try { execSync(`go build -o "${binaryPath}" .`, { cwd: projectRoot, stdio: "pipe" }); }
catch (e) { console.error("FAIL: go build failed"); process.exit(1); }

const backendPort = await findFreePort();
const backendUrl = `http://127.0.0.1:${backendPort}`;
const dbDir = await mkdtemp(path.join(os.tmpdir(), "starhub-e2e-"));
console.log(`E2E: Go backend at ${backendUrl} (DB: ${dbDir})`);

const backendProc = spawn(binaryPath, ["-port", String(backendPort)], {
  cwd: dbDir,
  stdio: ["ignore", "pipe", "pipe"],
  detached: false,
});
let backendLogs = [];
backendProc.stdout.on("data", (d) => backendLogs.push(d.toString()));
backendProc.stderr.on("data", (d) => backendLogs.push(d.toString()));

const backendReady = await waitForReady(`${backendUrl}/api/plaza/models`);
if (!backendReady) {
  console.error("FAIL: Go backend did not become ready");
  console.error(backendLogs.join("\n"));
  backendProc.kill();
  process.exit(1);
}
console.log("E2E: Go backend ready");

// --- Phase 2: Start Vite dev server with proxy to Go backend ---
const vitePort = await findFreePort();
const server = await createServer({
  root,
  server: {
    port: vitePort,
    host: "127.0.0.1",
    proxy: {
      "/api": backendUrl,
      "/v1": backendUrl,
    },
  },
  logLevel: "error",
});
await server.listen();
const url = `http://127.0.0.1:${vitePort}`;
console.log(`E2E: Vite dev server at ${url} (proxy -> ${backendUrl})`);

// --- Phase 3: Run browser checks ---
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage();

const report = {
  startedAt: new Date().toISOString(),
  frontendUrl: url,
  backendUrl,
  checks: [],
  failures: [],
  consoleErrors: [],
};

page.on("pageerror", (e) => report.consoleErrors.push(String(e)));

async function check(name, fn) {
  try {
    await fn();
    report.checks.push({ name, passed: true });
    console.log(`PASS  ${name}`);
  } catch (e) {
    report.checks.push({ name, passed: false, error: e.message });
    report.failures.push({ name, error: e.message });
    console.log(`FAIL  ${name} — ${e.message}`);
  }
}

// Frontend smoke: pages render.
await check("homepage loads", async () => {
  await page.goto(url, { waitUntil: "networkidle", timeout: 15000 });
  const title = await page.title();
  assert.ok(title.length > 0, "page title should not be empty");
});

await check("models page loads with backend data", async () => {
  await page.goto(`${url}/models`, { waitUntil: "networkidle", timeout: 15000 });
  const body = await page.textContent("body");
  assert.ok(body.length > 100, "models page body should have content");
});

await check("console redirects when not logged in", async () => {
  await page.goto(`${url}/console`, { waitUntil: "networkidle", timeout: 15000 });
  const finalUrl = page.url();
  assert.ok(
    !finalUrl.endsWith("/console") && !finalUrl.endsWith("/console/"),
    `expected redirect away from /console, but URL is ${finalUrl}`,
  );
  const body = await page.textContent("body");
  assert.ok(body.length > 0, "console redirect page should have content");
});

// Backend integration: real API responses through the Vite proxy.
await check("backend plaza API returns models via proxy", async () => {
  const resp = await page.evaluate(async () => {
    const r = await fetch("/api/plaza/models");
    return { status: r.status, body: await r.text() };
  });
  assert.equal(resp.status, 200, `plaza API status should be 200, got ${resp.status}`);
  const data = JSON.parse(resp.body);
  assert.ok(data.success === true, "plaza API should return success=true");
});

await check("backend plaza rank API returns items via proxy", async () => {
  const resp = await page.evaluate(async () => {
    const r = await fetch("/api/plaza/rank?type=value");
    return { status: r.status, body: await r.text() };
  });
  assert.equal(resp.status, 200, `rank API status should be 200, got ${resp.status}`);
  const data = JSON.parse(resp.body);
  assert.ok(data.success === true, "rank API should return success=true");
  assert.ok(Array.isArray(data.data?.items), "rank API should return items array");
});

await check("user API requires auth (401 via proxy)", async () => {
  const resp = await page.evaluate(async () => {
    const r = await fetch("/api/user/self");
    return { status: r.status, body: await r.text() };
  });
  assert.equal(resp.status, 401, `user API should return 401 without auth, got ${resp.status}`);
  const data = JSON.parse(resp.body);
  assert.ok(data.success === false, "401 response should have success=false");
  assert.ok(typeof data.message === "string" && data.message.length > 0, "401 response should have message");
});

await check("register+login+logout via proxy", async () => {
  // Register.
  const regResp = await page.evaluate(async () => {
    const r = await fetch("/api/user/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: "e2etest", password: "secret123", email: "e2e@test.com" }),
    });
    return r.status;
  });
  assert.ok(regResp === 200 || regResp === 409, `register should return 200 or 409, got ${regResp}`);

  // Login.
  const loginResp = await page.evaluate(async () => {
    const r = await fetch("/api/user/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: "e2etest", password: "secret123" }),
    });
    return { status: r.status, body: await r.text() };
  });
  assert.equal(loginResp.status, 200, `login should return 200, got ${loginResp.status}`);

  // Get self with session cookie (automatically sent by browser).
  const selfResp = await page.evaluate(async () => {
    const r = await fetch("/api/user/self", { credentials: "same-origin" });
    return { status: r.status, body: await r.text() };
  });
  assert.equal(selfResp.status, 200, `self should return 200 after login, got ${selfResp.status}`);
  const selfData = JSON.parse(selfResp.body);
  assert.ok(selfData.success === true, "self should return success=true");
  assert.ok(selfData.data?.username === "e2etest", "self should return correct username");

  // Logout.
  const logoutResp = await page.evaluate(async () => {
    const r = await fetch("/api/user/logout", { method: "POST" });
    return r.status;
  });
  assert.equal(logoutResp.status, 200, `logout should return 200, got ${logoutResp}`);

  // Verify old session is invalidated.
  const afterLogoutResp = await page.evaluate(async () => {
    const r = await fetch("/api/user/self", { credentials: "same-origin" });
    return r.status;
  });
  assert.equal(afterLogoutResp, 401, `self should return 401 after logout, got ${afterLogoutResp}`);
});

await check("no JS console errors on homepage", async () => {
  report.consoleErrors.length = 0;
  await page.goto(url, { waitUntil: "networkidle", timeout: 15000 });
  if (report.consoleErrors.length > 0) {
    throw new Error(`console errors: ${report.consoleErrors.join("; ")}`);
  }
});

// --- Phase 4: Cleanup ---
await browser.close();
await server.close();
backendProc.kill();
backendProc.on("exit", async () => {
  try { await rm(dbDir, { recursive: true, force: true }); } catch { /* best effort */ }
  try { await rm(binaryPath, { force: true }); } catch { /* best effort */ }
});

report.finishedAt = new Date().toISOString();
report.passed = report.failures.length === 0;

await mkdir(outDir, { recursive: true });
await writeFile(
  path.join(outDir, "e2e-report.json"),
  JSON.stringify(report, null, 2),
);

console.log(`\nE2E: ${report.checks.filter(c => c.passed).length}/${report.checks.length} passed`);
if (!report.passed) {
  process.exit(1);
}
